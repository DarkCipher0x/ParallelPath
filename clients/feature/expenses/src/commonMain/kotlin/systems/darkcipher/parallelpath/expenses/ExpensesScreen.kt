package systems.darkcipher.parallelpath.expenses

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ReceiptLong
import androidx.compose.material.icons.outlined.AttachFile
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Payments
import androidx.compose.material.icons.outlined.RemoveCircleOutline
import systems.darkcipher.parallelpath.ui.ContextMenuArea
import systems.darkcipher.parallelpath.ui.MenuDivider
import systems.darkcipher.parallelpath.ui.MenuItem
import systems.darkcipher.parallelpath.ui.menu
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Attachment
import systems.darkcipher.parallelpath.model.Expense
import systems.darkcipher.parallelpath.model.ExpensesView
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Avatar
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Choice
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.formatMoney
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.prettyDate
import systems.darkcipher.parallelpath.ui.shortTime

private enum class Filter(val label: String) { Open("Open"), Waiting("Waiting for you"), Settled("Settled"), All("All") }

/**
 * Shared costs: what each parent paid and the share asked of the other, the other parent's answer, payments and
 * who owes whom. Everything is end-to-end encrypted, receipts included, and every change is kept with its reason.
 */
@Composable
fun ExpensesScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit) {
    var view by remember { mutableStateOf<ExpensesView?>(null) }
    var filter by remember { mutableStateOf(Filter.Open) }
    var dialog by remember { mutableStateOf<ExpenseDialog?>(null) }
    var busy by remember { mutableStateOf(false) }
    var reload by remember { mutableIntStateOf(0) }
    val scope = rememberCoroutineScope()
    val platform = LocalPlatform.current
    val canEdit = space.isParent && !space.waiting && !space.readOnly

    LaunchedEffect(space.id, tick, reload) {
        view = runCatching { engine.expenses(space.id) }.onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't read the expenses") }.getOrNull() ?: view
    }
    val act: (suspend () -> Unit) -> Unit = { block ->
        scope.launch {
            busy = true
            try {
                block()
                dialog = null
                reload++
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e // a screen closing or reloading isn't an error
            } catch (e: Exception) {
                onError(e.message ?: "something went wrong")
            } finally {
                busy = false
            }
        }
    }
    val open: (Expense, Attachment) -> Unit = { x, a ->
        scope.launch {
            runCatching { platform.openFile(engine.openAttachment(space.id, x.id, a.blobId), a.mime) }
                .onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't open the receipt") }
        }
    }

    val v = view ?: return
    val shown = v.expenses.filter {
        when (filter) {
            Filter.Open -> it.state in setOf("pending", "agreed", "paid")
            Filter.Waiting -> it.needsMe || it.canPay
            Filter.Settled -> it.state == "settled"
            Filter.All -> true
        }
    }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 820.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (space.waiting) Banner("Expenses open once a parent approves you.", Tone.Warn)
            BalancePanel(v, canEdit) { dialog = ExpenseDialog.Edit(null) }
            FlowButtons {
                Filter.entries.forEach { f ->
                    val n = if (f == Filter.Waiting) v.expenses.count { it.needsMe || it.canPay } else 0
                    Choice(filter == f, if (n > 0) "${f.label} ($n)" else f.label, { filter = f }, if (f == Filter.Waiting && n > 0) pp.warm else pp.accent)
                }
            }
            if (shown.isEmpty()) {
                EmptyState(Icons.AutoMirrored.Outlined.ReceiptLong, if (v.expenses.isEmpty()) "No expenses yet" else "Nothing here",
                    if (v.expenses.isEmpty()) "Add something you paid for the children and the share you're asking for. The other parent " +
                        "agrees or declines, records their payment, and you confirm it arrived." else "Try another filter.")
            }
            shown.forEach { x -> ExpenseCard(x, v, canEdit, { dialog = it }) { a -> open(x, a) } }
        }
    }
    dialog?.let { d -> ExpenseDialogs(d, v, busy, engine, space.id, act) { dialog = null } }
}

@Composable
private fun BalancePanel(v: ExpensesView, canEdit: Boolean, onAdd: () -> Unit) {
    Panel(Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.Payments, null, tint = pp.accent, modifier = Modifier.size(22.dp))
            Spacer(Modifier.width(10.dp))
            SectionTitle("Balance", Modifier.weight(1f))
            if (canEdit) PrimaryButton("Add expense", onClick = onAdd)
        }
        if (v.balances.isEmpty()) {
            Text("All square - nothing agreed is waiting to be paid.", color = pp.fg, style = MaterialTheme.typography.titleMedium)
        }
        v.balances.forEach { b ->
            val me = v.parents.firstOrNull { it.me }?.id
            val text = when (me) {
                b.from -> "You owe ${b.toName} ${formatMoney(b.amount, b.currency)}"
                b.to -> "${b.fromName} owes you ${formatMoney(b.amount, b.currency)}"
                else -> "${b.fromName} owes ${b.toName} ${formatMoney(b.amount, b.currency)}"
            }
            Text(text, color = if (me == b.from) pp.warm else pp.fg, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
        }
        Text("Only agreed shares that haven't been paid count towards the balance.", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
    }
}

internal val expenseCategories = listOf("medical" to "Medical", "school" to "School", "clothing" to "Clothing", "activities" to "Activities",
    "childcare" to "Childcare", "travel" to "Travel", "food" to "Food", "other" to "Other")

internal fun categoryLabel(c: String) = expenseCategories.firstOrNull { it.first == c }?.second ?: "Other"

@Composable
private fun ExpenseCard(x: Expense, v: ExpensesView, canEdit: Boolean, onDialog: (ExpenseDialog) -> Unit, onOpen: (Attachment) -> Unit) {
    val (stateText, stateColor) = when (x.state) {
        "agreed" -> (if (x.canPay) "Agreed - your share to pay" else "Agreed - waiting for payment") to pp.accent
        "paid" -> (if (x.mine) "Paid - confirm it arrived" else "Paid - waiting for confirmation") to pp.ok
        "settled" -> "Settled" to pp.ok
        "declined" -> "Declined" to pp.danger
        "withdrawn" -> "Withdrawn" to pp.fgMute
        else -> (if (x.needsMe) "Waiting for you" else "Waiting for ${x.waiting.joinToString().ifBlank { "an answer" }}") to pp.warm
    }
    val shape = RoundedCornerShape(16.dp)
    val platform = LocalPlatform.current
    val paidAny = x.answers.any { it.current && it.answer == "paid" }
    // Right-click (long-press on a phone): everything the buttons do, plus the receipts and copying the details.
    ContextMenuArea({
        menu(
            if (canEdit && x.needsMe && x.state == "pending") MenuItem("Agree", Icons.Outlined.Check) { onDialog(ExpenseDialog.Answer(x, true)) } else null,
            if (canEdit && x.needsMe && x.state == "pending") MenuItem("Decline…", Icons.Outlined.Close, danger = true) { onDialog(ExpenseDialog.Answer(x, false)) } else null,
            if (canEdit && x.canPay) MenuItem("Record payment…", Icons.Outlined.Payments) { onDialog(ExpenseDialog.Pay(x)) } else null,
            *(if (canEdit && x.mine) x.toConfirm.map { p ->
                val name = v.parents.firstOrNull { it.id == p }?.name ?: "their"
                MenuItem("Confirm $name's payment arrived", Icons.Outlined.Check) { onDialog(ExpenseDialog.Confirm(x, p, name)) }
            }.toTypedArray() else emptyArray()),
            MenuDivider,
            *x.receipts.map { a -> MenuItem("Open ${a.name}", Icons.Outlined.AttachFile) { onOpen(a) } }.toTypedArray(),
            MenuDivider,
            MenuItem("Copy details", Icons.Outlined.ContentCopy) {
                platform.copyText("${x.data.title} - ${formatMoney(x.data.amount, x.data.currency)} (${categoryLabel(x.data.category)}, " +
                    "${prettyDate(x.data.date)}), share ${formatMoney(x.owed, x.data.currency)} (${x.data.share}%) - ${x.state}")
            },
            if (x.history.size > 1) MenuItem("History (${x.history.size})", Icons.Outlined.History) { onDialog(ExpenseDialog.History(x)) } else null,
            MenuDivider,
            if (canEdit && x.mine) MenuItem("Change…", Icons.Outlined.Edit, enabled = x.state in setOf("pending", "agreed", "declined") && !paidAny,
                hint = "Paid or settled expenses can't be changed") { onDialog(ExpenseDialog.Edit(x)) } else null,
            if (canEdit && x.mine && x.state != "withdrawn" && x.state != "settled") MenuItem("Withdraw…", Icons.Outlined.RemoveCircleOutline, danger = true) {
                onDialog(ExpenseDialog.Withdraw(x))
            } else null,
        )
    }, Modifier.fillMaxWidth()) {
    Panel(Modifier.fillMaxWidth().let { if (x.needsMe || x.canPay) it.border(1.dp, pp.warm, shape) else it }) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Avatar(x.authorName, x.author, 34)
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                Text(x.data.title, color = if (x.state == "withdrawn") pp.fgMute else pp.fg, style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold)
                Text("${categoryLabel(x.data.category)} · paid by ${if (x.mine) "you" else x.authorName} · ${prettyDate(x.data.date)}" +
                    if (x.data.children.isNotEmpty()) " · ${x.data.children.joinToString()}" else "", color = pp.fgDim,
                    style = MaterialTheme.typography.bodySmall)
            }
            Column(horizontalAlignment = Alignment.End) {
                Text(formatMoney(x.data.amount, x.data.currency), color = pp.fg, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                Text("share ${formatMoney(x.owed, x.data.currency)} (${x.data.share}%)", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            }
        }
        Text(stateText, Modifier.clip(CircleShape).background(stateColor.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 4.dp),
            color = stateColor, style = MaterialTheme.typography.labelMedium)
        if (x.note.isNotBlank()) Text(x.note, color = pp.fg)
        if (x.rev > 0 && x.reason.isNotBlank()) Text("${if (x.state == "withdrawn") "Withdrawn" else "Changed"} because: ${x.reason}",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        if (x.problem.isNotBlank()) Banner(x.problem.replaceFirstChar { it.uppercase() }, Tone.Warn)
        if (x.receipts.isNotEmpty()) {
            FlowButtons {
                x.receipts.forEach { a ->
                    Row(Modifier.clip(RoundedCornerShape(Radius.small)).background(pp.bg2).clickable { onOpen(a) }.padding(horizontal = 10.dp, vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Outlined.AttachFile, null, tint = pp.accent, modifier = Modifier.size(16.dp))
                        Text(" ${a.name}", color = pp.fg, style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
        x.answers.forEach { a ->
            val text = when (a.answer) {
                "accept" -> "${a.name} agreed"
                "decline" -> "${a.name} declined"
                "paid" -> "${a.name} paid by ${a.method}" + if (a.reference.isNotBlank()) " (ref ${a.reference})" else ""
                else -> "${a.name} confirmed ${a.forName}'s payment arrived"
            } + " ${shortTime(a.time)}" + (if (a.comment.isNotBlank()) ": “${a.comment}”" else "") +
                if (!a.current) " - about an earlier version" else ""
            val color = when {
                !a.current -> pp.fgMute
                a.answer == "decline" -> pp.danger
                else -> pp.ok
            }
            Text(text, color = color, style = MaterialTheme.typography.bodySmall)
        }
        FlowButtons {
            if (canEdit && x.needsMe && x.state == "pending") {
                PrimaryButton("Agree", onClick = { onDialog(ExpenseDialog.Answer(x, true)) })
                SecondaryButton("Decline…", danger = true, onClick = { onDialog(ExpenseDialog.Answer(x, false)) })
            }
            if (canEdit && x.canPay) PrimaryButton("Record payment", onClick = { onDialog(ExpenseDialog.Pay(x)) })
            if (canEdit && x.mine) x.toConfirm.forEach { p ->
                val name = v.parents.firstOrNull { it.id == p }?.name ?: "their"
                PrimaryButton("Confirm $name's payment arrived", onClick = { onDialog(ExpenseDialog.Confirm(x, p, name)) })
            }
            if (canEdit && x.mine && x.state in setOf("pending", "agreed", "declined") && !paidAny) {
                SecondaryButton("Change…", onClick = { onDialog(ExpenseDialog.Edit(x)) })
            }
            if (canEdit && x.mine && x.state != "withdrawn" && x.state != "settled") {
                TextButton(onClick = { onDialog(ExpenseDialog.Withdraw(x)) }) { Text("Withdraw", color = pp.danger) }
            }
            if (x.history.size > 1) TextButton(onClick = { onDialog(ExpenseDialog.History(x)) }) { Text("History (${x.history.size})", color = pp.accent) }
        }
    }
    }
}
