package systems.darkcipher.parallelpath.records

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.datetime.LocalDate
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.model.InfoEntry
import systems.darkcipher.parallelpath.model.InfoView
import systems.darkcipher.parallelpath.ui.Choice
import systems.darkcipher.parallelpath.ui.DateField
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.FormDialog
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.todayLocal

internal sealed interface InfoDialog {
    data class Child(val edit: InfoEntry?) : InfoDialog
    data class Entry(val edit: InfoEntry?, val child: String, val section: String?) : InfoDialog
    data class Remove(val en: InfoEntry) : InfoDialog
    data class History(val en: InfoEntry) : InfoDialog
}

@Composable
internal fun InfoDialogs(d: InfoDialog, v: InfoView, busy: Boolean, engine: Engine, space: String, act: (suspend () -> Unit) -> Unit, onClose: () -> Unit) {
    when (d) {
        is InfoDialog.Child -> {
            var name by remember { mutableStateOf(d.edit?.label ?: "") }
            var hasBirthday by remember { mutableStateOf(d.edit?.value?.isNotBlank() == true) }
            var born by remember { mutableStateOf(d.edit?.value?.let { runCatching { LocalDate.parse(it) }.getOrNull() } ?: todayLocal()) }
            var why by remember { mutableStateOf("") }
            FormDialog(if (d.edit == null) "Add a child" else "Update ${d.edit.label}", "Save", name.isNotBlank() && (d.edit == null || why.isNotBlank()),
                busy, onClose, {
                    act {
                        engine.saveInfo(space, "child", "", "", name.trim(), if (hasBirthday) born.toString() else "", "",
                            replaces = d.edit?.id, why = if (d.edit == null) null else why.trim())
                    }
                }) {
                Field(name, { name = it }, "Name")
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(hasBirthday, { hasBirthday = it }, colors = CheckboxDefaults.colors(checkedColor = pp.accent, checkmarkColor = pp.accentInk))
                    Text("Add their date of birth", color = pp.fg)
                }
                if (hasBirthday) DateField("Date of birth", born, { born = it })
                if (d.edit != null) Field(why, { why = it }, "Why is it changing?", supporting = "Required - kept with the history")
            }
        }
        is InfoDialog.Entry -> EntryEditor(d, v, busy, onClose) { child, section, label, value, note, files, why ->
            act { engine.saveInfo(space, "info", child, section, label, value, note, files, d.edit?.id, why) }
        }
        is InfoDialog.Remove -> {
            var why by remember { mutableStateOf("") }
            FormDialog("Remove ${d.en.label}?", "Remove", why.isNotBlank(), busy, onClose,
                { act { engine.saveInfo(space, d.en.type, d.en.child, d.en.section, d.en.label, d.en.value, d.en.note, replaces = d.en.id, why = why.trim(), remove = true) } },
                danger = true) {
                Text("It moves to \"removed\" and stays in the record with every earlier version.", color = pp.fgDim)
                Field(why, { why = it }, "Why?", supporting = "Required")
            }
        }
        is InfoDialog.History -> AlertDialog(
            onDismissRequest = onClose, containerColor = pp.panel, title = { Text("History of ${d.en.label}") },
            text = {
                Column(Modifier.widthIn(min = 300.dp, max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    d.en.history.asReversed().forEach { h ->
                        Column {
                            Text("${if (h.removed) "Removed" else h.value.ifBlank { "-" }}", color = pp.fg, fontWeight = FontWeight.Medium)
                            Text("${h.name} · ${fullTime(h.time)}", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                            if (h.why.isNotBlank()) Text("Why: ${h.why}", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
            },
            confirmButton = { PrimaryButton("Close", onClick = onClose) },
        )
    }
}

@Composable
private fun EntryEditor(
    d: InfoDialog.Entry, v: InfoView, busy: Boolean, onClose: () -> Unit,
    onSave: (String, String, String, String, String, List<FileInput>, String?) -> Unit,
) {
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    val e = d.edit
    var child by remember { mutableStateOf(e?.child ?: d.child) }
    var section by remember { mutableStateOf(e?.section ?: d.section ?: "medical") }
    var label by remember { mutableStateOf(e?.label ?: "") }
    var value by remember { mutableStateOf(e?.value ?: "") }
    var note by remember { mutableStateOf(e?.note ?: "") }
    var files by remember { mutableStateOf<List<FileInput>>(emptyList()) }
    var why by remember { mutableStateOf("") }
    val ok = label.isNotBlank() && (e == null || why.isNotBlank())
    FormDialog(if (e == null) "Add a detail" else "Update ${e.label}", "Save", ok, busy, onClose,
        { onSave(child, section, label.trim(), value.trim(), note.trim(), files, if (e == null) null else why.trim()) }) {
        if (e != null) Text("Currently: ${e.value.ifBlank { "-" }} (by ${e.byName}). The current version is kept in the history.", color = pp.fgDim,
            style = MaterialTheme.typography.bodySmall)
        Text("About", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        FlowButtons {
            v.children.forEach { c -> Choice(child == c.chain, c.label, { child = c.chain }) }
            Choice(child == "", "Whole family", { child = "" })
        }
        Text("Section", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        FlowButtons { sections.forEach { (id, l) -> Choice(section == id, l, { section = id }) } }
        Field(label, { label = it }, "Label", supporting = "e.g. ${suggestions[section]?.take(3)?.joinToString() ?: "GP"}")
        if (e == null) FlowButtons {
            suggestions[section].orEmpty().forEach { s -> TextButton(onClick = { label = s }) { Text(s, color = pp.accent) } }
        }
        Field(value, { value = it }, "Detail", singleLine = false, minLines = 2)
        Field(note, { note = it }, "Note (optional)", singleLine = false)
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            SecondaryButton("Attach…", onClick = { scope.launch { files = files + platform.pickFiles() } })
            Text(if (files.isEmpty()) (if (e != null && e.attachments.isNotEmpty()) "Keeps ${e.attachments.size} file(s)" else "A photo of a card or letter, say")
                else files.joinToString { it.name ?: it.path.substringAfterLast('/').substringAfterLast('\\') }, color = pp.fgDim,
                style = MaterialTheme.typography.bodySmall)
        }
        if (e != null) Field(why, { why = it }, "Why is it changing?", supporting = "Required - kept with the history")
    }
}
