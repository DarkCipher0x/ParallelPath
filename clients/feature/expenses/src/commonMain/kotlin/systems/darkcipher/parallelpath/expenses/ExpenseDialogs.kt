package systems.darkcipher.parallelpath.expenses

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.datetime.LocalDate
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Expense
import systems.darkcipher.parallelpath.model.ExpenseData
import systems.darkcipher.parallelpath.model.ExpensesView
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.ui.Choice
import systems.darkcipher.parallelpath.ui.DateField
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.FormDialog
import systems.darkcipher.parallelpath.ui.HistoryList
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.ReasonDialog
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.formatMoney
import systems.darkcipher.parallelpath.ui.parseMoney
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.todayLocal

internal sealed interface ExpenseDialog {
    data class Edit(val x: Expense?) : ExpenseDialog
    data class Answer(val x: Expense, val accept: Boolean) : ExpenseDialog
    data class Pay(val x: Expense) : ExpenseDialog
    data class Confirm(val x: Expense, val person: String, val name: String) : ExpenseDialog
    data class Withdraw(val x: Expense) : ExpenseDialog
    data class History(val x: Expense) : ExpenseDialog
}

@Composable
internal fun ExpenseDialogs(
    d: ExpenseDialog, v: ExpensesView, busy: Boolean, engine: Engine, space: String, act: (suspend () -> Unit) -> Unit, onClose: () -> Unit,
) {
    when (d) {
        is ExpenseDialog.Edit -> ExpenseEditor(d.x, v, busy, onClose) { data, note, files, reason ->
            act { engine.saveExpense(space, data, note, files, d.x?.id, reason) }
        }
        is ExpenseDialog.Answer -> {
            var comment by remember { mutableStateOf("") }
            FormDialog(if (d.accept) "Agree to this expense?" else "Decline this expense?", if (d.accept) "Agree" else "Decline",
                d.accept || comment.isNotBlank(), busy, onClose, { act { engine.answerExpense(space, d.x.id, d.accept, comment.trim()) } },
                danger = !d.accept, toneText = comment) {
                Text("${d.x.data.title}: ${d.x.authorName} paid ${formatMoney(d.x.data.amount, d.x.data.currency)} and asks you for " +
                    "${formatMoney(d.x.owed, d.x.data.currency)} (${d.x.data.share}%).", color = pp.fg)
                Text(if (d.accept) "Your answer is signed and kept with this version. If it's changed, you'll be asked again."
                    else "Say why - it's kept with the expense so there's a clear record.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                Field(comment, { comment = it }, if (d.accept) "Comment (optional)" else "Reason", singleLine = false, minLines = 2)
            }
        }
        is ExpenseDialog.Pay -> {
            var method by remember { mutableStateOf("Bank transfer") }
            var reference by remember { mutableStateOf("") }
            var note by remember { mutableStateOf("") }
            FormDialog("Record your payment", "Record payment", method.isNotBlank(), busy, onClose,
                { act { engine.payExpense(space, d.x.id, method.trim(), reference.trim(), note.trim()) } }) {
                Text("You're recording that you paid ${formatMoney(d.x.owed, d.x.data.currency)} to ${d.x.authorName} for ${d.x.data.title}. " +
                    "ParallelPath doesn't move money - pay as you normally would, then record it here. ${d.x.authorName} confirms when it arrives.",
                    color = pp.fgDim)
                FlowButtons { listOf("Bank transfer", "Cash", "Card", "Other").forEach { m -> Choice(method == m, m, { method = m }) } }
                Field(reference, { reference = it }, "Payment reference (optional)")
                Field(note, { note = it }, "Note (optional)")
            }
        }
        is ExpenseDialog.Confirm -> FormDialog("Confirm the payment arrived?", "Confirm", true, busy, onClose,
            { act { engine.confirmPayment(space, d.x.id, d.person) } }) {
            Text("This confirms ${d.name}'s payment for ${d.x.data.title} reached you. It's signed and kept in the record.", color = pp.fgDim)
        }
        is ExpenseDialog.Withdraw -> ReasonDialog("Withdraw ${d.x.data.title}?",
            "It stays in the record, marked withdrawn, with your reason. Any answers and payments stay too.", "Withdraw", busy, onClose) { r ->
            act { engine.withdrawExpense(space, d.x.id, r) }
        }
        is ExpenseDialog.History -> AlertDialog(
            onDismissRequest = onClose, containerColor = pp.panel, title = { Text("History of ${d.x.data.title}") },
            text = {
                Column(Modifier.widthIn(min = 300.dp, max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text("Every version is kept, encrypted and signed by the person who made it.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                    HistoryList(d.x.history)
                }
            },
            confirmButton = { PrimaryButton("Close", onClick = onClose) },
        )
    }
}

@Composable
private fun ExpenseEditor(
    edit: Expense?, v: ExpensesView, busy: Boolean, onClose: () -> Unit, onSave: (ExpenseData, String, List<FileInput>, String?) -> Unit,
) {
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    val old = edit?.data
    var title by remember { mutableStateOf(old?.title ?: "") }
    var amount by remember { mutableStateOf(old?.let { formatMoney(it.amount, "XXX").removeSuffix(" XXX") } ?: "") }
    var currency by remember { mutableStateOf(old?.currency ?: "GBP") }
    var date by remember { mutableStateOf(old?.date?.let { runCatching { LocalDate.parse(it) }.getOrNull() } ?: todayLocal()) }
    var category by remember { mutableStateOf(old?.category?.ifBlank { "other" } ?: "other") }
    var share by remember { mutableStateOf(old?.share?.toString() ?: "50") }
    var children by remember { mutableStateOf(old?.children?.joinToString() ?: "") }
    var note by remember { mutableStateOf(edit?.note ?: "") }
    var files by remember { mutableStateOf<List<FileInput>>(emptyList()) }
    var reason by remember { mutableStateOf("") }
    val minor = parseMoney(amount)
    val pct = share.trim().removeSuffix("%").toIntOrNull()
    val others = v.parents.filter { !it.me }.joinToString { it.name }.ifBlank { "the other parent" }
    val ok = title.isNotBlank() && minor != null && minor > 0 && pct != null && pct in 0..100 && (edit == null || reason.isNotBlank())

    FormDialog(if (edit == null) "Add an expense" else "Change this expense", if (edit == null) "Add" else "Save", ok, busy, onClose, {
        onSave(ExpenseData(title.trim(), minor!!, currency, date.toString(), category,
            children.split(',').map { it.trim() }.filter { it.isNotEmpty() }, pct!!), note.trim(), files,
            if (edit == null) null else reason.trim())
    }) {
        Text("Something you paid for the children. $others is asked to agree to the share; nothing is owed until they do.",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        Field(title, { title = it }, "What was it?")
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
            Field(amount, { amount = it }, "Amount paid", modifier = Modifier.weight(1f), keyboard = KeyboardType.Decimal,
                supporting = if (amount.isNotBlank() && minor == null) "e.g. 24.50" else null)
            FlowButtons { listOf("GBP", "EUR", "USD").forEach { c -> Choice(currency == c, c, { currency = c }) } }
        }
        DateField("Paid on", date, { date = it })
        FlowButtons { expenseCategories.forEach { (id, label) -> Choice(category == id, label, { category = id }) } }
        Text("Share asked of $others", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
            FlowButtons { listOf("50", "100", "0").forEach { s -> Choice(share == s, "$s%", { share = s }) } }
            Field(share, { share = it }, "%", modifier = Modifier.weight(1f), keyboard = KeyboardType.Number)
        }
        if (minor != null && pct != null && pct in 0..100) Text("They'd owe you ${formatMoney(minor * pct / 100, currency)}.", color = pp.accent)
        Field(children, { children = it }, "Which children (optional, separate with commas)")
        Field(note, { note = it }, "Note (optional)", singleLine = false, minLines = 2)
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            SecondaryButton("Add receipt…", onClick = { scope.launch { files = files + platform.pickFiles() } })
            Text(when {
                files.isNotEmpty() -> files.joinToString { it.name ?: it.path.substringAfterLast('/').substringAfterLast('\\') }
                edit != null && edit.receipts.isNotEmpty() -> "Keeps ${edit.receipts.size} receipt(s)"
                else -> "Photos or PDFs - encrypted like everything else"
            }, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        }
        if (files.isNotEmpty()) TextButton(onClick = { files = emptyList() }) { Text("Remove the new receipts", color = pp.danger) }
        if (edit != null) Field(reason, { reason = it }, "Why are you changing it?", supporting = "Required - ${edit.waiting.joinToString().ifBlank { "the other parent" }} will be asked again")
    }
}
