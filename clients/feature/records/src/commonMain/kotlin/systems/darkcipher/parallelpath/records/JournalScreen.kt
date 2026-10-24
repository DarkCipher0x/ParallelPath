package systems.darkcipher.parallelpath.records

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AttachFile
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.automirrored.outlined.Send
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.History
import systems.darkcipher.parallelpath.ui.ContextMenuArea
import systems.darkcipher.parallelpath.ui.MenuDivider
import systems.darkcipher.parallelpath.ui.MenuItem
import systems.darkcipher.parallelpath.ui.menu
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
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
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.datetime.LocalDate
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.model.JournalEntry
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Choice
import systems.darkcipher.parallelpath.ui.DateField
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.FormDialog
import systems.darkcipher.parallelpath.ui.HistoryList
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.ReasonDialog
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.prettyDate
import systems.darkcipher.parallelpath.ui.shortTime
import systems.darkcipher.parallelpath.ui.todayLocal

private sealed interface JDialog {
    data class Edit(val en: JournalEntry?) : JDialog
    data class Share(val en: JournalEntry) : JDialog
    data class Remove(val en: JournalEntry) : JDialog
    data class History(val en: JournalEntry) : JDialog
}

/**
 * Your private journal about this family: notes for yourself (and, if it comes to it, your solicitor). It's kept in
 * a space only your own devices hold keys for, so the other parent can't read it or even see that it exists.
 */
@Composable
fun JournalScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit) {
    var entries by remember { mutableStateOf<List<JournalEntry>?>(null) }
    var showRemoved by remember { mutableStateOf(false) }
    var dialog by remember { mutableStateOf<JDialog?>(null) }
    var busy by remember { mutableStateOf(false) }
    var reload by remember { mutableIntStateOf(0) }
    val scope = rememberCoroutineScope()
    val platform = LocalPlatform.current

    LaunchedEffect(space.id, tick, reload) {
        entries = runCatching { engine.journal(space.id) }
            .onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't read your journal") }.getOrNull() ?: entries
    }
    val act: (suspend () -> Unit) -> Unit = { block ->
        scope.launch {
            busy = true
            try {
                block()
                dialog = null
                reload++
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                onError(e.message ?: "something went wrong")
            } finally {
                busy = false
            }
        }
    }

    val all = entries ?: return
    val shown = all.filter { showRemoved || !it.removed }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 820.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Panel(Modifier.fillMaxWidth()) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.Lock, null, tint = pp.accent, modifier = Modifier.size(20.dp))
                    Text("  Only you can read your journal", color = pp.fg, style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                }
                Text("It's encrypted for your devices alone and kept apart from the family space, so the other parent can't read it " +
                    "or even see that it exists. Sharing an entry posts a copy into the family's messages.", color = pp.fgDim,
                    style = MaterialTheme.typography.bodySmall)
                PrimaryButton("New entry", onClick = { dialog = JDialog.Edit(null) })
            }
            if (shown.isEmpty()) EmptyState(Icons.Outlined.Lock, "Nothing written yet",
                "Keep notes as things happen: late handovers, how the children were, what was agreed. Dated, signed and kept " +
                    "with every edit, so it's a record you can rely on.")
            shown.forEach { en -> EntryCard(en, { dialog = it }) { a ->
                scope.launch {
                    runCatching { platform.openFile(engine.openJournalFile(en.id, a.blobId), a.mime) }
                        .onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't open the file") }
                }
            } }
            if (all.any { it.removed }) Row(verticalAlignment = Alignment.CenterVertically) {
                Checkbox(showRemoved, { showRemoved = it }, colors = CheckboxDefaults.colors(checkedColor = pp.accent, checkmarkColor = pp.accentInk))
                Text("Show removed entries", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            }
        }
    }

    when (val d = dialog) {
        null -> Unit
        is JDialog.Edit -> JournalEditor(d.en, busy, { dialog = null }) { text, date, mood, tags, files, reason ->
            act { engine.saveJournal(space.id, text, date, mood, tags, files, d.en?.id, reason) }
        }
        is JDialog.Share -> FormDialog("Share this entry with ${space.name}?", "Share", true, busy, { dialog = null },
            { act { engine.shareJournal(space.id, d.en.id) } }) {
            Text("A copy is posted to the family's messages as a message from you, with any photos or files. Everyone in the family " +
                "will be able to read that copy, and it can't be unsent (only withdrawn, with a reason). Your journal stays private.", color = pp.fgDim)
        }
        is JDialog.Remove -> ReasonDialog("Remove this entry?", "It's hidden from your journal but every version is kept (only you can see them).",
            "Remove", busy, { dialog = null }) { r -> act { engine.removeJournal(d.en.id, r) } }
        is JDialog.History -> AlertDialog(
            onDismissRequest = { dialog = null }, containerColor = pp.panel, title = { Text("History") },
            text = { Column(verticalArrangement = Arrangement.spacedBy(10.dp)) { HistoryList(d.en.history) } },
            confirmButton = { PrimaryButton("Close", onClick = { dialog = null }) },
        )
    }
}

private val moods = listOf("good" to "Good day", "okay" to "Okay", "hard" to "Hard day")

@Composable
private fun EntryCard(en: JournalEntry, onDialog: (JDialog) -> Unit, onOpen: (systems.darkcipher.parallelpath.model.Attachment) -> Unit) {
    val platform = systems.darkcipher.parallelpath.ui.LocalPlatform.current
    ContextMenuArea({
        menu(
            if (!en.removed) MenuItem("Edit…", Icons.Outlined.Edit) { onDialog(JDialog.Edit(en)) } else null,
            if (!en.removed) MenuItem("Share with the family…", Icons.AutoMirrored.Outlined.Send) { onDialog(JDialog.Share(en)) } else null,
            MenuItem("Copy text", Icons.Outlined.ContentCopy) { platform.copyText("${prettyDate(en.data.date)}\n\n${en.text}") },
            MenuDivider,
            *en.attachments.map { a -> MenuItem("Open ${a.name}", Icons.Outlined.AttachFile) { onOpen(a) } }.toTypedArray(),
            MenuDivider,
            if (en.history.size > 1) MenuItem("History (${en.history.size})", Icons.Outlined.History) { onDialog(JDialog.History(en)) } else null,
            if (!en.removed) MenuItem("Remove…", Icons.Outlined.Delete, danger = true) { onDialog(JDialog.Remove(en)) } else null,
        )
    }, Modifier.fillMaxWidth()) {
    Panel(Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(prettyDate(en.data.date), color = pp.fg, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f))
            moods.firstOrNull { it.first == en.data.mood }?.let { (id, label) ->
                val c = when (id) { "good" -> pp.ok; "hard" -> pp.warm; else -> pp.accent }
                Text(label, Modifier.clip(CircleShape).background(c.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 4.dp), color = c,
                    style = MaterialTheme.typography.labelMedium)
            }
        }
        if (en.removed) Text("Removed: ${en.reason}", color = pp.danger, style = MaterialTheme.typography.bodySmall)
        SelectionContainer {
            Text(en.text, color = if (en.removed) pp.fgMute else pp.fg, textDecoration = if (en.removed) TextDecoration.LineThrough else null)
        }
        if (en.data.tags.isNotEmpty()) Text(en.data.tags.joinToString("  ") { "#$it" }, color = pp.accent, style = MaterialTheme.typography.bodySmall)
        if (en.attachments.isNotEmpty()) FlowButtons {
            en.attachments.forEach { a ->
                Row(Modifier.clip(RoundedCornerShape(Radius.small)).background(pp.bg2).clickable { onOpen(a) }.padding(horizontal = 10.dp, vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.AttachFile, null, tint = pp.accent, modifier = Modifier.size(16.dp))
                    Text(" ${a.name}", color = pp.fg, style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        Text("Written ${shortTime(en.written)}" + if (en.edited) " · edited ${shortTime(en.updated)}" else "", color = pp.fgMute,
            style = MaterialTheme.typography.bodySmall)
        if (!en.removed) FlowButtons {
            SecondaryButton("Edit", onClick = { onDialog(JDialog.Edit(en)) })
            SecondaryButton("Share with the family…", onClick = { onDialog(JDialog.Share(en)) })
            TextButton(onClick = { onDialog(JDialog.Remove(en)) }) { Text("Remove", color = pp.danger) }
            if (en.history.size > 1) TextButton(onClick = { onDialog(JDialog.History(en)) }) { Text("History (${en.history.size})", color = pp.accent) }
        }
    }
    }
}

@Composable
private fun JournalEditor(
    edit: JournalEntry?, busy: Boolean, onClose: () -> Unit,
    onSave: (String, String, String, List<String>, List<FileInput>, String?) -> Unit,
) {
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    var text by remember { mutableStateOf(edit?.text ?: "") }
    var date by remember { mutableStateOf(edit?.data?.date?.let { runCatching { LocalDate.parse(it) }.getOrNull() } ?: todayLocal()) }
    var mood by remember { mutableStateOf(edit?.data?.mood ?: "") }
    var tags by remember { mutableStateOf(edit?.data?.tags?.joinToString(", ") ?: "") }
    var files by remember { mutableStateOf<List<FileInput>>(emptyList()) }
    var reason by remember { mutableStateOf("") }
    val ok = (text.isNotBlank() || files.isNotEmpty()) && (edit == null || reason.isNotBlank())
    FormDialog(if (edit == null) "New journal entry" else "Edit entry", "Save", ok, busy, onClose, {
        onSave(text.trim(), date.toString(), mood, tags.split(',').map { it.trim().removePrefix("#") }.filter { it.isNotEmpty() }, files,
            if (edit == null) null else reason.trim())
    }) {
        DateField("About the day", date, { date = it })
        FlowButtons {
            Choice(mood == "", "No mood", { mood = "" })
            moods.forEach { (id, label) -> Choice(mood == id, label, { mood = id }) }
        }
        Field(text, { text = it }, "What happened?", singleLine = false, minLines = 6)
        Field(tags, { tags = it }, "Tags (optional, separate with commas)", supporting = "e.g. handover, school, health")
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            SecondaryButton("Attach…", onClick = { scope.launch { files = files + platform.pickFiles() } })
            Text(if (files.isEmpty()) (if (edit != null && edit.attachments.isNotEmpty()) "Keeps ${edit.attachments.size} file(s)" else "Photos, screenshots, letters")
                else files.joinToString { it.name ?: it.path.substringAfterLast('/').substringAfterLast('\\') }, color = pp.fgDim,
                style = MaterialTheme.typography.bodySmall)
        }
        if (edit != null) Field(reason, { reason = it }, "Why are you changing it?", supporting = "Required - the earlier version is kept")
    }
}
