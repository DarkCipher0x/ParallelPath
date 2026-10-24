package systems.darkcipher.parallelpath.files

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.FolderOpen
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Doc
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.model.SharedFile
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Choice
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.FormDialog
import systems.darkcipher.parallelpath.ui.HistoryList
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.ReasonDialog
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.pp

internal sealed interface DocDialog {
    data class NewFolder(val parent: String) : DocDialog
    data class NewVersion(val d: Doc) : DocDialog
    data class Rename(val d: Doc) : DocDialog
    data class Note(val d: Doc) : DocDialog
    /** Move or copy [docs]; [to] preselects the target folder (a paste or a drop), [copy] starts as a copy. */
    data class MoveCopy(val docs: List<Doc>, val copy: Boolean, val to: String? = null) : DocDialog
    data class Remove(val docs: List<Doc>) : DocDialog
    data class Restore(val d: Doc) : DocDialog
    data class History(val d: Doc) : DocDialog
    data class Properties(val e: Entry) : DocDialog
    data class FileShared(val f: SharedFile) : DocDialog
}

@Composable
internal fun DocDialogs(
    d: DocDialog, all: List<Doc>, families: List<Space>, busy: Boolean, engine: Engine, space: String,
    act: (suspend () -> Unit) -> Unit, onOpen: (Doc, systems.darkcipher.parallelpath.model.Attachment) -> Unit, onClose: () -> Unit,
) {
    when (d) {
        is DocDialog.NewFolder -> {
            var name by remember { mutableStateOf("") }
            FormDialog("New folder", "Create", name.isNotBlank() && '/' !in name, busy, onClose,
                { act { engine.saveDocument(space, "folder", d.parent, name.trim(), "", null) } }) {
                Text(if (d.parent.isEmpty()) "In Documents" else "In Documents/${d.parent}", color = pp.fgDim)
                Field(name, { name = it }, "Folder name", supporting = "e.g. School, Medical, Court")
            }
        }
        is DocDialog.NewVersion -> {
            val platform = LocalPlatform.current
            val scope = rememberCoroutineScope()
            var file by remember { mutableStateOf<FileInput?>(null) }
            var reason by remember { mutableStateOf("") }
            FormDialog("New version of ${d.d.name}", "Upload", file != null && reason.isNotBlank(), busy, onClose,
                { act { engine.saveDocument(space, "file", d.d.folder, d.d.name, d.d.note, file, d.d.id, reason.trim()) } }, toneText = reason) {
                Text("The current version stays in the history and can still be opened.", color = pp.fgDim)
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    SecondaryButton("Choose file…", onClick = { scope.launch { platform.pickDocuments().firstOrNull()?.let { file = it } } })
                    Text(file?.let { it.name ?: it.path.substringAfterLast('/').substringAfterLast('\\') } ?: "No file chosen", color = pp.fgDim)
                }
                Field(reason, { reason = it }, "What changed?", supporting = "Required - kept with the version")
            }
        }
        is DocDialog.Rename -> {
            var name by remember { mutableStateOf(d.d.name) }
            var reason by remember { mutableStateOf("") }
            val ok = name.isNotBlank() && name.trim() != d.d.name && '/' !in name && reason.isNotBlank()
            FormDialog("Rename ${if (d.d.isFolder) "folder" else "file"}", "Rename", ok, busy, onClose, {
                act { engine.moveDocument(space, d.d.id, d.d.path, d.d.folder, name.trim(), reason.trim()) }
            }, toneText = reason) {
                Field(name, { name = it }, "New name")
                if (d.d.isFolder) Text("Everything in the folder moves with it.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                Field(reason, { reason = it }, "Why?", supporting = "Required - kept in the history")
            }
        }
        is DocDialog.Note -> {
            var note by remember { mutableStateOf(d.d.note) }
            var reason by remember { mutableStateOf("") }
            FormDialog("Note on ${d.d.name}", "Save", note.trim() != d.d.note && reason.isNotBlank(), busy, onClose, {
                act { engine.saveDocument(space, d.d.type, d.d.folder, d.d.name, note.trim(), null, d.d.id, reason.trim()) }
            }, toneText = note + "\n" + reason) {
                Field(note, { note = it }, "Note", singleLine = false, minLines = 3)
                Field(reason, { reason = it }, "Why?", supporting = "Required - kept in the history")
            }
        }
        is DocDialog.MoveCopy -> {
            var copy by remember { mutableStateOf(d.copy) }
            var target by remember { mutableStateOf(space) }
            var targetDocs by remember { mutableStateOf(all) }
            var folder by remember { mutableStateOf(d.to ?: d.docs.firstOrNull()?.folder ?: "") }
            var reason by remember { mutableStateOf(if (d.to != null) "Moved to ${d.to.ifEmpty { "Documents" }}" else "") }
            val scope = rememberCoroutineScope()
            val crossFamily = target != space
            val moving = !copy && !crossFamily
            val canMove = d.docs.all { it.mine || it.implied }
            val inside = d.docs.any { it.isFolder && (folder == it.path || folder.startsWith(it.path + "/")) } && !crossFamily
            val unchanged = moving && d.docs.all { it.folder == folder }
            val what = if (d.docs.size == 1) d.docs[0].name else "${d.docs.size} items"
            FormDialog(if (moving) "Move $what" else "Copy $what", if (moving) "Move here" else "Copy here",
                !inside && !unchanged && (!moving || (canMove && reason.isNotBlank())), busy, onClose, {
                    act {
                        for (x in d.docs) {
                            if (moving) engine.moveDocument(space, x.id, x.path, folder, x.name, reason.trim())
                            else engine.copyDocument(space, x.id, x.path, target, folder)
                        }
                    }
                }, toneText = if (moving) reason else null) {
                FlowButtons {
                    Choice(!copy, "Move", { copy = false; target = space; targetDocs = all })
                    Choice(copy, "Copy", { copy = true })
                }
                if (!moving && !canMove && !copy) Text("Only what you added can be moved - you can copy it.", color = pp.warm)
                if (moving && !canMove) Text("Some of these were added by someone else: only they can move them. Copy them instead.", color = pp.warm,
                    style = MaterialTheme.typography.bodySmall)
                if (copy && families.isNotEmpty()) {
                    Text("To", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                    FlowButtons {
                        Choice(target == space, "This family", { target = space; targetDocs = all; folder = "" })
                        families.forEach { f ->
                            Choice(target == f.id, f.name, {
                                target = f.id
                                folder = ""
                                scope.launch { targetDocs = runCatching { engine.documents(f.id) }.getOrDefault(emptyList()) }
                            })
                        }
                    }
                    if (crossFamily) Text("Each file is decrypted on this device and encrypted again with that family's keys.",
                        color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                }
                Text("Folder", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                FolderPicker(targetDocs, folder, disabled = if (crossFamily) emptyList() else d.docs.filter { it.isFolder }.map { it.path }) { folder = it }
                if (inside) Text("A folder can't go inside itself.", color = pp.danger, style = MaterialTheme.typography.bodySmall)
                if (moving) Field(reason, { reason = it }, "Why?", supporting = "Required - kept in the history")
                else Text("Copies are yours; the originals stay where they are.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            }
        }
        is DocDialog.Remove -> {
            val what = if (d.docs.size == 1) d.docs[0].name else "${d.docs.size} items"
            val folders = d.docs.count { it.isFolder }
            ReasonDialog("Remove $what?",
                "Removed things are hidden but kept for the record: everyone can still see them under Removed and open every version." +
                    if (folders > 0) " A folder goes with everything in it." else "",
                "Remove", busy, onClose) { r ->
                act { engine.removeDocuments(space, d.docs.filter { !it.isFolder }.map { it.id }, d.docs.filter { it.isFolder }.map { it.path }, r) }
            }
        }
        is DocDialog.Restore -> {
            var reason by remember { mutableStateOf("") }
            FormDialog("Restore ${d.d.name}", "Restore", reason.isNotBlank(), busy, onClose,
                { act { engine.restoreDocument(space, d.d.id, reason.trim()) } }, toneText = reason) {
                Text("It goes back to Documents/${d.d.folder.ifEmpty { "" }} as it was. The removal stays in its history.", color = pp.fgDim)
                Field(reason, { reason = it }, "Why?", supporting = "Required - kept in the history")
            }
        }
        is DocDialog.FileShared -> {
            var folder by remember { mutableStateOf("") }
            FormDialog("Save ${d.f.file.name} to Documents", "Save here", true, busy, onClose,
                { act { engine.saveSharedFile(space, d.f.space, d.f.message, d.f.file.blobId, folder) } }) {
                Text("A copy goes into the family's documents, where versions and who opened it are kept.", color = pp.fgDim)
                FolderPicker(all, folder, emptyList()) { folder = it }
            }
        }
        is DocDialog.History -> AlertDialog(
            onDismissRequest = onClose, containerColor = pp.panel, title = { Text("History of ${d.d.name}") },
            text = {
                Column(Modifier.widthIn(min = 300.dp, max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    HistoryList(d.d.history)
                }
            },
            confirmButton = { PrimaryButton("Close", onClick = onClose) },
        )
        is DocDialog.Properties -> AlertDialog(
            onDismissRequest = onClose, containerColor = pp.panel, title = { Text("${d.e.name} - properties") },
            text = {
                Column(Modifier.widthIn(min = 320.dp, max = 620.dp).heightIn(max = 640.dp).verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    EntryDetails(d.e, all, onOpen = { a -> d.e.doc?.let { onOpen(it, a) } })
                }
            },
            confirmButton = { PrimaryButton("Close", onClick = onClose) },
        )
    }
}

/** The documents' folders as an indented tree to choose from. */
@Composable
internal fun FolderPicker(docs: List<Doc>, selected: String, disabled: List<String>, onPick: (String) -> Unit) {
    val paths = folderPaths(docs)
    Column(Modifier.fillMaxWidth().heightIn(max = 260.dp).clip(RoundedCornerShape(Radius.small)).background(pp.bg2)
        .verticalScroll(rememberScrollState()).padding(4.dp)) {
        @Composable
        fun row(path: String, depth: Int) {
            val off = disabled.any { path == it || path.startsWith("$it/") }
            Row(Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small))
                .background(if (path == selected) pp.accentSoft else androidx.compose.ui.graphics.Color.Transparent)
                .clickable(enabled = !off) { onPick(path) }.padding(start = (8 + depth * 16).dp, top = 6.dp, bottom = 6.dp, end = 8.dp),
                verticalAlignment = Alignment.CenterVertically) {
                Icon(if (path == selected) Icons.Outlined.FolderOpen else Icons.Outlined.Folder, null,
                    tint = if (off) pp.fgMute else pp.warm, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(if (path.isEmpty()) "Documents" else path.substringAfterLast('/'), color = if (off) pp.fgMute else pp.fg,
                    fontWeight = if (path == selected) FontWeight.SemiBold else null)
            }
            childFolders(paths, path).forEach { row(it, depth + 1) }
        }
        row("", 0)
    }
}

/** Everything about one entry: what it is, where, who added it, versions, who opened it. */
@Composable
internal fun EntryDetails(e: Entry, all: List<Doc>, onOpen: (systems.darkcipher.parallelpath.model.Attachment) -> Unit, onHistory: (() -> Unit)? = null) {
    @Composable
    fun line(label: String, value: String) {
        if (value.isBlank()) return
        Row {
            Text(label, color = pp.fgMute, style = MaterialTheme.typography.bodySmall, modifier = Modifier.width(110.dp))
            Text(value, color = pp.fg, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
        }
    }
    val d = e.doc
    val s = e.shared
    line("Type", e.type)
    line("Location", if (d != null) e.where + if (d.removed) " (removed)" else "" else e.where)
    if (d != null && d.isFolder) {
        val inside = all.filter { !it.removed && it.path != d.path && (it.folder == d.path || it.folder.startsWith(d.path + "/")) }
        val files = inside.filter { !it.isFolder }
        line("Contains", "${files.size} file${if (files.size == 1) "" else "s"}, ${inside.size - files.size} folder${if (inside.size - files.size == 1) "" else "s"}")
        line("Size", sizeText(files.sumOf { it.file?.size ?: 0 }))
    } else {
        e.file?.let { f ->
            line("Size", sizeText(f.size))
            if (f.width > 0) line("Dimensions", "${f.width} × ${f.height}")
            if (f.millis > 0) line("Length", "${f.millis / 60000}:${((f.millis / 1000) % 60).toString().padStart(2, '0')}")
        }
    }
    if (d != null && !d.implied) {
        line("Added by", if (d.mine) "You" else d.authorName)
        line("Added", fullTime(d.created))
        if (d.updated != d.created) line("Changed", fullTime(d.updated))
        if (d.removed) line("Removed", d.reason.ifBlank { "-" })
        if (d.note.isNotBlank()) line("Note", d.note)
    }
    if (s != null) {
        line("Shared by", if (s.mine) "You" else s.authorName)
        line("Shared", fullTime(s.time))
        if (s.text.isNotBlank()) line("Message", s.text)
    }
    line("Encryption", "End-to-end (each file has its own key; the server only stores ciphertext)")
    if (d != null && !d.isFolder) {
        Text("Versions (${d.versions.size})", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        d.versions.asReversed().forEachIndexed { i, v ->
            Row(Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small)).clickable { onOpen(v.file) }.padding(vertical = 4.dp),
                verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("${if (i == 0) "Current" else "Version ${d.versions.size - i}"} · ${v.file.name} · ${sizeText(v.file.size)}", color = pp.fg,
                        style = MaterialTheme.typography.bodySmall)
                    Text(fullTime(v.time) + if (v.reason.isNotBlank()) " · ${v.reason}" else "", color = pp.fgDim, style = MaterialTheme.typography.labelSmall)
                }
                Text("Open", color = pp.accent, style = MaterialTheme.typography.labelMedium)
            }
        }
        Text("Opened by", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        if (d.opened.isEmpty()) Text("Nobody has opened it yet.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        d.opened.take(30).forEach { o ->
            Text("${o.name} · ${fullTime(o.time)}" + if (d.versions.size > 1) " · version ${d.versions.indexOfFirst { it.rev == o.rev } + 1}" else "",
                color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        }
        Text("Opening a file is recorded in the family's audit log.", color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
    }
    if (onHistory != null && d != null && d.history.size > 1) TextButton(onClick = onHistory) { Text("Change history (${d.history.size})", color = pp.accent) }
}
