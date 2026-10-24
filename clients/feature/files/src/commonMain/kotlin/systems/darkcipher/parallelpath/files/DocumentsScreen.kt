package systems.darkcipher.parallelpath.files

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.ArrowForward
import androidx.compose.material.icons.automirrored.outlined.InsertDriveFile
import androidx.compose.material.icons.automirrored.outlined.Send
import androidx.compose.material.icons.automirrored.outlined.ViewList
import androidx.compose.material.icons.outlined.ArrowDownward
import androidx.compose.material.icons.outlined.ArrowUpward
import androidx.compose.material.icons.outlined.CheckBox
import androidx.compose.material.icons.outlined.CheckBoxOutlineBlank
import androidx.compose.material.icons.outlined.ChevronRight
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.ContentCut
import androidx.compose.material.icons.outlined.ContentPaste
import androidx.compose.material.icons.outlined.CreateNewFolder
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Description
import androidx.compose.material.icons.automirrored.outlined.DriveFileMove
import androidx.compose.material.icons.outlined.DriveFileRenameOutline
import androidx.compose.material.icons.outlined.EditNote
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.FileUpload
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.FolderOpen
import androidx.compose.material.icons.outlined.GridView
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Image
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.outlined.PictureAsPdf
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.RestoreFromTrash
import androidx.compose.material.icons.outlined.Save
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.SelectAll
import androidx.compose.material.icons.automirrored.outlined.Sort
import androidx.compose.material.icons.outlined.Upload
import androidx.compose.material.icons.outlined.Videocam
import androidx.compose.material.icons.automirrored.outlined.ViewSidebar
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material.icons.outlined.Forum
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isAltPressed
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.isMetaPressed
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Attachment
import systems.darkcipher.parallelpath.model.Conversation
import systems.darkcipher.parallelpath.model.Doc
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.model.SharedFile
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.ContextMenu
import systems.darkcipher.parallelpath.ui.ContextMenuArea
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.MenuDivider
import systems.darkcipher.parallelpath.ui.MenuEntry
import systems.darkcipher.parallelpath.ui.MenuGroup
import systems.darkcipher.parallelpath.ui.MenuItem
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.SaveFile
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.menu
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.shortTime

/**
 * The family's documents as a full file explorer: a folder tree with Recent, Shared in messages and Removed; details
 * or icon views with sortable columns; search across every folder; multi-select (Ctrl/Shift-click, or Select on a
 * phone); right-click (long-press) menus on everything; cut/copy/paste and drag-to-move; dropping files from the
 * system to upload; keyboard shortcuts; and a details pane with versions and who opened what. Files are encrypted on
 * this device before they're uploaded; every version is kept and every opening is recorded.
 */
@Composable
fun DocumentsScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit, startFolder: String = "", startSelected: String? = null) {
    var docs by remember { mutableStateOf<List<Doc>?>(null) }
    var shared by remember { mutableStateOf<List<SharedFile>>(emptyList()) }
    var families by remember { mutableStateOf<List<Space>>(emptyList()) }
    var conversations by remember { mutableStateOf<List<Conversation>>(emptyList()) }
    var loc by remember { mutableStateOf<Loc>(Loc.Folder(startFolder)) }
    val back = remember { mutableStateOf(listOf<Loc>()) }
    val forward = remember { mutableStateOf(listOf<Loc>()) }
    var selected by remember { mutableStateOf(setOfNotNull(startSelected)) }
    var anchor by remember { mutableStateOf<String?>(startSelected) }
    var selectMode by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    var showRemoved by remember { mutableStateOf(false) }
    var dialog by remember { mutableStateOf<DocDialog?>(null) }
    var preview by remember { mutableStateOf<Preview?>(null) }
    var busy by remember { mutableStateOf(false) }
    var status by remember { mutableStateOf<String?>(null) }
    var reload by remember { mutableIntStateOf(0) }
    var dropHover by remember { mutableStateOf(false) }
    var menuAt by remember { mutableStateOf<List<MenuEntry>?>(null) } // the ⋮ menu
    val scope = rememberCoroutineScope()
    val platform = LocalPlatform.current
    val phone = platform.name == "Android"
    val canEdit = !space.waiting && !space.readOnly
    val focus = remember { FocusRequester() }
    val searchFocus = remember { FocusRequester() }
    var searching by remember { mutableStateOf(false) }
    val listState = rememberLazyListState()

    // Drag-to-move: where folders are on screen, what's being dragged and where the pointer is.
    val targets = remember { mutableStateMapOf<String, Rect>() }
    var dragging by remember { mutableStateOf<List<Doc>?>(null) }
    var dragAt by remember { mutableStateOf(Offset.Zero) }
    var origin by remember { mutableStateOf(Offset.Zero) }
    val dropTarget = dragging?.let { targets.entries.firstOrNull { (_, r) -> r.contains(dragAt) }?.key }

    LaunchedEffect(space.id, tick, reload) {
        docs = runCatching { engine.documents(space.id) }.onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't read the documents") }.getOrNull() ?: docs
        shared = runCatching { engine.sharedFiles(space.id) }.getOrDefault(shared)
    }
    LaunchedEffect(space.id) {
        families = runCatching { engine.spaces() }.getOrDefault(emptyList()).filter { it.id != space.id && !it.waiting && !it.readOnly }
        conversations = runCatching { engine.conversations(space.id) }.getOrDefault(emptyList()).filter { it.canWrite }
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
                reload++
            } finally {
                busy = false
                status = null
            }
        }
    }
    val all = docs ?: return
    val folder = (loc as? Loc.Folder)?.path

    fun go(to: Loc) {
        if (to == loc) return
        back.value = back.value + loc
        forward.value = emptyList()
        loc = to
        selected = emptySet()
        anchor = null
        selectMode = false
        query = ""
        searching = false
    }
    fun goBack() { back.value.lastOrNull()?.let { forward.value = forward.value + loc; back.value = back.value.dropLast(1); loc = it; selected = emptySet() } }
    fun goForward() { forward.value.lastOrNull()?.let { back.value = back.value + loc; forward.value = forward.value.dropLast(1); loc = it; selected = emptySet() } }
    fun goUp() { if (folder != null && folder.isNotEmpty()) go(Loc.Folder(folder.substringBeforeLast('/', ""))) }

    // What's listed here.
    val q = query.trim()
    val entries: List<Entry> = run {
        val raw = when {
            q.isNotEmpty() && loc != Loc.Shared -> all.filter { (showRemoved || !it.removed || loc == Loc.Removed) && it.name.contains(q, ignoreCase = true) }
                .filter { loc != Loc.Removed || it.removed }.map { Entry(it) }
            loc is Loc.Folder -> all.filter { it.folder == folder && (showRemoved || !it.removed) }.map { Entry(it) }
            loc == Loc.Recent -> all.filter { !it.isFolder && !it.removed }.sortedByDescending { it.updated }.take(50).map { Entry(it) }
            loc == Loc.Removed -> all.filter { it.removed }.map { Entry(it) }
            else -> shared.filter { q.isEmpty() || it.file.name.contains(q, ignoreCase = true) }.map { Entry(shared = it) }
        }
        if (loc == Loc.Recent && q.isEmpty()) raw else sortEntries(raw, ExplorerPrefs.sortBy, ExplorerPrefs.descending)
    }
    val chosen = entries.filter { it.key in selected }
    val chosenDocs = chosen.mapNotNull { it.doc }
    val single = chosen.singleOrNull()

    // ---- actions
    val open: (Doc, Attachment) -> Unit = { d, a ->
        scope.launch {
            try {
                status = "Opening ${a.name}…"
                val path = engine.openAttachment(space.id, d.id, a.blobId)
                reload++
                preview = previewOf(platform, path, a) ?: run { platform.openFile(path, a.mime); null }
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                onError(e.message ?: "couldn't open the file")
            } finally {
                status = null
            }
        }
    }
    fun decrypt(e: Entry, then: (String, Attachment) -> Unit) {
        val a = e.file ?: return
        scope.launch {
            try {
                status = "Decrypting ${a.name}…"
                val path = if (e.doc != null) engine.openAttachment(space.id, e.doc.id, a.blobId)
                else engine.openAttachment(e.shared!!.space, e.shared.message, a.blobId)
                then(path, a)
            } catch (x: kotlinx.coroutines.CancellationException) {
                throw x
            } catch (x: Exception) {
                onError(x.message ?: "couldn't open the file")
            } finally {
                status = null
                reload++
            }
        }
    }
    fun openEntry(e: Entry) {
        val d = e.doc
        when {
            d != null && d.isFolder -> if (!d.removed) go(Loc.Folder(d.path))
            else -> decrypt(e) { path, a -> preview = previewOf(platform, path, a) ?: run { platform.openFile(path, a.mime); null } }
        }
    }
    fun openOutside(e: Entry) = decrypt(e) { path, a -> platform.openFile(path, a.mime) }
    fun saveCopies(list: List<Entry>) {
        val files = list.filter { it.file != null && !it.isFolder }
        if (files.isEmpty()) return
        scope.launch {
            try {
                status = "Decrypting ${files.size} file${if (files.size == 1) "" else "s"}…"
                val ready = files.map { e ->
                    val a = e.file!!
                    val path = if (e.doc != null) engine.openAttachment(space.id, e.doc.id, a.blobId) else engine.openAttachment(e.shared!!.space, e.shared.message, a.blobId)
                    SaveFile(path, a.name, a.mime)
                }
                status = null
                val n = if (ready.size == 1) (if (platform.saveCopy(ready[0].path, ready[0].name, ready[0].mime)) 1 else 0) else platform.saveCopies(ready)
                if (n > 0) onError(if (n == 1) "Saved a copy of ${ready[0].name}." else "Saved $n copies.")
            } catch (x: kotlinx.coroutines.CancellationException) {
                throw x
            } catch (x: Exception) {
                onError(x.message ?: "couldn't save the copies")
            } finally {
                status = null
                reload++
            }
        }
    }
    fun upload(files: List<FileInput>, into: String? = folder) {
        if (files.isEmpty() || into == null || !canEdit) return
        act {
            files.forEachIndexed { i, f ->
                status = "Encrypting and uploading ${i + 1} of ${files.size}: ${f.name ?: f.path.substringAfterLast('/').substringAfterLast('\\')}"
                engine.saveDocument(space.id, "file", into, "", "", f)
            }
        }
    }
    fun pickAndUpload() { scope.launch { upload(platform.pickDocuments()) } }
    fun sendInMessage(e: Entry, to: String) = decrypt(e) { path, a ->
        act { engine.send(to, "", null, listOf(FileInput(path = path, name = a.name, mime = a.mime, thumb = a.thumb, width = a.width, height = a.height))) }
    }
    fun copyToClip(cut: Boolean) {
        val d = chosenDocs.filter { !it.removed }
        if (d.isNotEmpty()) ExplorerClipboard.clip = Clip(cut && d.all { it.mine || it.implied }, space.id, d)
    }
    fun paste(into: String? = folder) {
        val c = ExplorerClipboard.clip ?: return
        if (into == null || !canEdit) return
        if (c.space == space.id && c.cut) dialog = DocDialog.MoveCopy(c.docs, copy = false, to = into)
        else act {
            c.docs.forEachIndexed { i, d ->
                status = "Copying ${i + 1} of ${c.docs.size}: ${d.name}"
                engine.copyDocument(c.space, d.id, d.path, space.id, into)
            }
            if (c.cut) ExplorerClipboard.clip = null
        }
    }
    fun select(e: Entry, keys: systems.darkcipher.parallelpath.ui.ClickKeys) {
        when {
            keys.shift && anchor != null -> {
                val i = entries.indexOfFirst { it.key == anchor }
                val j = entries.indexOfFirst { it.key == e.key }
                if (i >= 0 && j >= 0) selected = (if (keys.ctrl) selected else emptySet()) + entries.subList(minOf(i, j), maxOf(i, j) + 1).map { it.key }
            }
            keys.ctrl || selectMode -> { selected = if (e.key in selected) selected - e.key else selected + e.key; anchor = e.key }
            else -> { selected = setOf(e.key); anchor = e.key }
        }
    }
    fun mayChange(d: Doc) = canEdit && (d.mine || d.implied)
    fun whyNot(d: Doc) = when {
        !canEdit -> "You can look, not change"
        else -> "Added by ${d.authorName} - only they can change it"
    }

    // ---- menus
    fun itemMenu(e: Entry): List<MenuEntry> {
        val many = chosen.size > 1 && e.key in selected
        val d = e.doc
        val s = e.shared
        if (many) {
            val ds = chosenDocs
            val removable = ds.filter { !it.removed }
            return menu(
                MenuItem("Save copies…", Icons.Outlined.Save, "Ctrl+S", enabled = platform.canSaveCopies && chosen.any { it.file != null && !it.isFolder }) { saveCopies(chosen) },
                MenuDivider,
                if (ds.isNotEmpty()) MenuItem("Cut", Icons.Outlined.ContentCut, "Ctrl+X", enabled = canEdit && removable.all { mayChange(it) },
                    hint = "Some were added by someone else") { copyToClip(true) } else null,
                if (ds.isNotEmpty()) MenuItem("Copy", Icons.Outlined.ContentCopy, "Ctrl+C", enabled = canEdit) { copyToClip(false) } else null,
                if (ds.isNotEmpty()) MenuItem("Move to…", Icons.AutoMirrored.Outlined.DriveFileMove, enabled = canEdit && removable.all { mayChange(it) },
                    hint = "Some were added by someone else") { dialog = DocDialog.MoveCopy(removable, copy = false) } else null,
                if (ds.isNotEmpty()) MenuItem("Copy to…", Icons.Outlined.ContentCopy, enabled = canEdit) { dialog = DocDialog.MoveCopy(removable, copy = true) } else null,
                MenuDivider,
                if (ds.isNotEmpty()) MenuItem("Remove ${removable.size} items", Icons.Outlined.Delete, "Del", enabled = canEdit && removable.isNotEmpty() && removable.all { mayChange(it) },
                    danger = true, hint = "Some were added by someone else") { dialog = DocDialog.Remove(removable) } else null,
                MenuDivider,
                MenuItem("Clear selection", Icons.Outlined.Close, "Esc") { selected = emptySet(); selectMode = false },
            )
        }
        if (s != null) return menu(
            MenuItem("Open", Icons.Outlined.FolderOpen, "Enter") { openEntry(e) },
            MenuItem("Open in another app", Icons.AutoMirrored.Outlined.OpenInNew) { openOutside(e) },
            MenuItem("Save a copy…", Icons.Outlined.Save, "Ctrl+S", enabled = platform.canSaveCopies) { saveCopies(listOf(e)) },
            MenuItem("Save to Documents…", Icons.AutoMirrored.Outlined.DriveFileMove, enabled = canEdit) { dialog = DocDialog.FileShared(s) },
            MenuDivider,
            MenuItem("Copy name", Icons.Outlined.ContentCopy) { platform.copyText(s.file.name) },
            MenuItem("Properties", Icons.Outlined.Info, "Alt+Enter") { dialog = DocDialog.Properties(e) },
        )
        d ?: return emptyList()
        val sendTo = if (!d.isFolder && d.file != null && canEdit) MenuGroup("Send in a message", Icons.AutoMirrored.Outlined.Send,
            listOf<MenuEntry>(MenuItem("Family messages") { sendInMessage(e, space.id) }) +
                conversations.map { c -> MenuItem("Private: ${c.title}") { sendInMessage(e, c.id) } }) else null
        return menu(
            MenuItem(if (d.isFolder) "Open" else "Open", Icons.Outlined.FolderOpen, "Enter", enabled = !(d.isFolder && d.removed)) { openEntry(e) },
            if (!d.isFolder) MenuItem("Open in another app", Icons.AutoMirrored.Outlined.OpenInNew) { openOutside(e) } else null,
            if (!d.isFolder) MenuItem("Save a copy…", Icons.Outlined.Save, "Ctrl+S", enabled = platform.canSaveCopies) { saveCopies(listOf(e)) } else null,
            sendTo,
            MenuDivider,
            if (d.isFolder && !d.removed && canEdit) MenuItem("New folder inside", Icons.Outlined.CreateNewFolder) { dialog = DocDialog.NewFolder(d.path) } else null,
            if (d.isFolder && !d.removed && canEdit) MenuItem("Upload files here…", Icons.Outlined.FileUpload) { scope.launch { upload(platform.pickDocuments(), d.path) } } else null,
            if (d.isFolder && !d.removed && ExplorerClipboard.clip != null && canEdit) MenuItem("Paste into folder", Icons.Outlined.ContentPaste) { paste(d.path) } else null,
            MenuDivider,
            if (!d.removed) MenuItem("Cut", Icons.Outlined.ContentCut, "Ctrl+X", enabled = mayChange(d), hint = whyNot(d)) { selected = setOf(e.key); ExplorerClipboard.clip = Clip(true, space.id, listOf(d)) } else null,
            if (!d.removed) MenuItem("Copy", Icons.Outlined.ContentCopy, "Ctrl+C", enabled = canEdit) { ExplorerClipboard.clip = Clip(false, space.id, listOf(d)) } else null,
            if (!d.removed) MenuItem("Move to…", Icons.AutoMirrored.Outlined.DriveFileMove, enabled = mayChange(d), hint = whyNot(d)) { dialog = DocDialog.MoveCopy(listOf(d), copy = false) } else null,
            MenuItem("Copy to…", Icons.Outlined.ContentCopy, enabled = canEdit) { dialog = DocDialog.MoveCopy(listOf(d), copy = true) },
            if (!d.removed) MenuItem("Rename…", Icons.Outlined.DriveFileRenameOutline, "F2", enabled = mayChange(d), hint = whyNot(d)) { dialog = DocDialog.Rename(d) } else null,
            if (!d.isFolder && !d.removed) MenuItem("Upload a new version…", Icons.Outlined.Upload, enabled = canEdit && d.mine, hint = whyNot(d)) { dialog = DocDialog.NewVersion(d) } else null,
            if (!d.implied && !d.removed) MenuItem("Edit note…", Icons.Outlined.EditNote, enabled = canEdit && d.mine, hint = whyNot(d)) { dialog = DocDialog.Note(d) } else null,
            MenuDivider,
            MenuItem("Copy name", Icons.Outlined.ContentCopy) { platform.copyText(d.name) },
            MenuItem("Copy path", Icons.Outlined.ContentCopy, "Ctrl+Shift+C") { platform.copyText("Documents/" + d.path) },
            if (q.isNotEmpty() || loc !is Loc.Folder) MenuItem("Show in folder", Icons.Outlined.Folder) { go(Loc.Folder(d.folder)); selected = setOf(e.key) } else null,
            if (!d.implied && d.history.size > 1) MenuItem("Change history", Icons.Outlined.History) { dialog = DocDialog.History(d) } else null,
            MenuItem("Properties", Icons.Outlined.Info, "Alt+Enter") { dialog = DocDialog.Properties(e) },
            MenuDivider,
            if (phone) MenuItem("Select", Icons.Outlined.CheckBox) { selectMode = true; selected = selected + e.key } else null,
            if (d.removed) MenuItem("Restore", Icons.Outlined.RestoreFromTrash, enabled = canEdit && d.mine, hint = whyNot(d)) { dialog = DocDialog.Restore(d) }
            else if (!d.implied || d.isFolder) MenuItem("Remove…", Icons.Outlined.Delete, "Del", enabled = mayChange(d), danger = true, hint = whyNot(d)) { dialog = DocDialog.Remove(listOf(d)) }
            else null,
        )
    }
    fun viewMenu(): List<MenuEntry> = menu(
        MenuGroup("View", Icons.AutoMirrored.Outlined.ViewList, listOf(
            MenuItem((if (ExplorerPrefs.view == ViewMode.Details) "✓ " else "") + "Details", Icons.AutoMirrored.Outlined.ViewList) { ExplorerPrefs.view = ViewMode.Details },
            MenuItem((if (ExplorerPrefs.view == ViewMode.Icons) "✓ " else "") + "Large icons", Icons.Outlined.GridView) { ExplorerPrefs.view = ViewMode.Icons },
            MenuDivider,
            MenuItem((if (ExplorerPrefs.detailsPane) "✓ " else "") + "Details pane", Icons.AutoMirrored.Outlined.ViewSidebar, "Alt+P") { ExplorerPrefs.detailsPane = !ExplorerPrefs.detailsPane },
            MenuItem((if (ExplorerPrefs.tree) "✓ " else "") + "Folder tree", Icons.Outlined.Folder) { ExplorerPrefs.tree = !ExplorerPrefs.tree },
        )),
        MenuGroup("Sort by", Icons.AutoMirrored.Outlined.Sort, SortBy.entries.map { s ->
            MenuItem((if (ExplorerPrefs.sortBy == s) "✓ " else "") + s.label) { ExplorerPrefs.sortBy = s }
        } + listOf(MenuDivider,
            MenuItem((if (!ExplorerPrefs.descending) "✓ " else "") + "Ascending") { ExplorerPrefs.descending = false },
            MenuItem((if (ExplorerPrefs.descending) "✓ " else "") + "Descending") { ExplorerPrefs.descending = true })),
        MenuItem(if (showRemoved) "Hide removed" else "Show removed", if (showRemoved) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility) { showRemoved = !showRemoved },
    )
    fun backgroundMenu(): List<MenuEntry> = menu(
        if (folder != null && canEdit) MenuItem("New folder", Icons.Outlined.CreateNewFolder, "Ctrl+Shift+N") { dialog = DocDialog.NewFolder(folder) } else null,
        if (folder != null && canEdit) MenuItem("Upload files…", Icons.Outlined.FileUpload, "Ctrl+U") { pickAndUpload() } else null,
        if (folder != null && canEdit && ExplorerClipboard.clip != null) MenuItem("Paste", Icons.Outlined.ContentPaste, "Ctrl+V") { paste() } else null,
        MenuDivider,
        *viewMenu().toTypedArray(),
        MenuDivider,
        MenuItem("Select all", Icons.Outlined.SelectAll, "Ctrl+A") { selected = entries.map { it.key }.toSet(); if (phone) selectMode = true },
        MenuItem("Refresh", Icons.Outlined.Refresh, "F5") { reload++ },
        if (folder != null) MenuItem("Copy path", Icons.Outlined.ContentCopy) { platform.copyText("Documents/$folder") } else null,
    )

    preview?.let { p -> PreviewDialog(p, onOpen = { platform.openFile(p.path, p.mime) }) { preview = null } }

    // ---- keyboard (Windows and the web; a phone keyboard rarely has these keys)
    val keys = Modifier.onPreviewKeyEvent { ev ->
        if (ev.type != KeyEventType.KeyDown || searching || dialog != null) return@onPreviewKeyEvent false
        val ctrl = ev.isCtrlPressed || ev.isMetaPressed
        val i = entries.indexOfFirst { it.key == anchor }
        fun moveTo(j: Int) {
            if (entries.isEmpty()) return
            val k = j.coerceIn(0, entries.lastIndex)
            val e = entries[k]
            if (ev.isShiftPressed && anchor != null) {
                val a = entries.indexOfFirst { it.key == (selected.firstOrNull() ?: anchor) }.coerceAtLeast(0)
                selected = entries.subList(minOf(a, k), maxOf(a, k) + 1).map { it.key }.toSet()
            } else selected = setOf(e.key)
            anchor = e.key
            scope.launch { listState.animateScrollToItem(k) }
        }
        when {
            ev.key == Key.DirectionDown -> moveTo(if (i < 0) 0 else i + 1)
            ev.key == Key.DirectionUp && !ev.isAltPressed -> moveTo(if (i < 0) 0 else i - 1)
            ev.key == Key.MoveHome -> moveTo(0)
            ev.key == Key.MoveEnd -> moveTo(entries.lastIndex)
            ev.key == Key.Enter && ev.isAltPressed -> single?.let { dialog = DocDialog.Properties(it) }
            ev.key == Key.Enter -> single?.let { openEntry(it) }
            ev.key == Key.Delete -> chosenDocs.filter { !it.removed }.takeIf { it.isNotEmpty() && it.all(::mayChange) }?.let { dialog = DocDialog.Remove(it) }
            ev.key == Key.F2 -> single?.doc?.takeIf { mayChange(it) && !it.removed }?.let { dialog = DocDialog.Rename(it) }
            ev.key == Key.F5 -> reload++
            ev.key == Key.Escape -> { selected = emptySet(); selectMode = false; ExplorerClipboard.clip = ExplorerClipboard.clip }
            ev.key == Key.Backspace || (ev.isAltPressed && ev.key == Key.DirectionUp) -> goUp()
            ev.isAltPressed && ev.key == Key.DirectionLeft -> goBack()
            ev.isAltPressed && ev.key == Key.DirectionRight -> goForward()
            ev.isAltPressed && ev.key == Key.P -> ExplorerPrefs.detailsPane = !ExplorerPrefs.detailsPane
            ctrl && ev.key == Key.A -> selected = entries.map { it.key }.toSet()
            ctrl && ev.isShiftPressed && ev.key == Key.C -> single?.doc?.let { platform.copyText("Documents/" + it.path) }
            ctrl && ev.key == Key.C -> copyToClip(false)
            ctrl && ev.key == Key.X -> copyToClip(true)
            ctrl && ev.key == Key.V -> paste()
            ctrl && ev.key == Key.S -> if (platform.canSaveCopies) saveCopies(chosen)
            ctrl && ev.key == Key.F -> { searching = true; scope.launch { runCatching { searchFocus.requestFocus() } } }
            ctrl && ev.isShiftPressed && ev.key == Key.N -> if (folder != null && canEdit) dialog = DocDialog.NewFolder(folder)
            ctrl && ev.key == Key.U -> if (folder != null && canEdit) pickAndUpload()
            else -> return@onPreviewKeyEvent false
        }
        true
    }

    var rootPos by remember { mutableStateOf(Offset.Zero) }
    val fileDrop = with(platform) { Modifier.fileDrop(onFiles = { upload(it) }, onHover = { dropHover = it && folder != null && canEdit }) }
    BoxWithConstraints(Modifier.fillMaxSize().then(fileDrop).onGloballyPositioned { rootPos = it.positionInRoot() }
        .then(keys).focusRequester(focus).focusable()) {
        val wide = maxWidth >= 900.dp
        val medium = maxWidth >= 640.dp
        Column(Modifier.fillMaxSize()) {
            // Toolbar: back/forward/up, the address (breadcrumbs, each a drop target), search, view.
            Row(Modifier.fillMaxWidth().background(pp.panel).padding(horizontal = 6.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                ToolIcon(Icons.AutoMirrored.Outlined.ArrowBack, "Back (Alt+Left)", back.value.isNotEmpty()) { goBack() }
                if (medium) ToolIcon(Icons.AutoMirrored.Outlined.ArrowForward, "Forward (Alt+Right)", forward.value.isNotEmpty()) { goForward() }
                ToolIcon(Icons.Outlined.ArrowUpward, "Up a level (Backspace)", !folder.isNullOrEmpty()) { goUp() }
                if (!searching || medium) Address(loc, Modifier.weight(1f), dropTarget, targets) { go(it) }
                if (searching || medium) SearchBox(query, { query = it }, if (q.isNotEmpty()) "Search" else "Search ${loc.title()}",
                    Modifier.then(if (searching && !medium) Modifier.weight(1f) else Modifier.width(220.dp)).focusRequester(searchFocus)
                        .onFocusChanged { searching = it.isFocused || query.isNotEmpty() }) { query = ""; searching = false; runCatching { focus.requestFocus() } }
                else ToolIcon(Icons.Outlined.Search, "Search (Ctrl+F)", true) { searching = true; scope.launch { runCatching { searchFocus.requestFocus() } } }
                ToolIcon(if (ExplorerPrefs.view == ViewMode.Details) Icons.Outlined.GridView else Icons.AutoMirrored.Outlined.ViewList,
                    if (ExplorerPrefs.view == ViewMode.Details) "Large icons" else "Details", true) {
                    ExplorerPrefs.view = if (ExplorerPrefs.view == ViewMode.Details) ViewMode.Icons else ViewMode.Details
                }
                Box {
                    ToolIcon(Icons.Outlined.MoreVert, "More", true) { menuAt = backgroundMenu() }
                    menuAt?.let { ContextMenu(it, androidx.compose.ui.unit.DpOffset.Zero) { menuAt = null } }
                }
            }
            // Command bar: the everyday actions, enabled for what's selected.
            if (medium || selected.isNotEmpty()) Row(Modifier.fillMaxWidth().background(pp.bg2).horizontalScroll(rememberScrollState()).padding(horizontal = 6.dp),
                verticalAlignment = Alignment.CenterVertically) {
                if (folder != null && canEdit && selected.isEmpty()) {
                    Command(Icons.Outlined.FileUpload, "Upload", busy = busy) { pickAndUpload() }
                    Command(Icons.Outlined.CreateNewFolder, "New folder") { dialog = DocDialog.NewFolder(folder) }
                    if (ExplorerClipboard.clip != null) Command(Icons.Outlined.ContentPaste, "Paste") { paste() }
                }
                if (selected.isNotEmpty()) {
                    if (single != null) Command(Icons.Outlined.FolderOpen, "Open") { openEntry(single) }
                    if (platform.canSaveCopies && chosen.any { it.file != null && !it.isFolder }) Command(Icons.Outlined.Save, "Save copy") { saveCopies(chosen) }
                    if (chosenDocs.isNotEmpty() && canEdit) {
                        Command(Icons.Outlined.ContentCut, "Cut", enabled = chosenDocs.all { mayChange(it) && !it.removed }) { copyToClip(true) }
                        Command(Icons.Outlined.ContentCopy, "Copy") { copyToClip(false) }
                        Command(Icons.AutoMirrored.Outlined.DriveFileMove, "Move to", enabled = chosenDocs.all { mayChange(it) && !it.removed }) {
                            dialog = DocDialog.MoveCopy(chosenDocs, copy = false)
                        }
                        single?.doc?.let { d ->
                            if (!d.removed) Command(Icons.Outlined.DriveFileRenameOutline, "Rename", enabled = mayChange(d)) { dialog = DocDialog.Rename(d) }
                            if (d.removed) Command(Icons.Outlined.RestoreFromTrash, "Restore", enabled = d.mine) { dialog = DocDialog.Restore(d) }
                        }
                        val live = chosenDocs.filter { !it.removed }
                        if (live.isNotEmpty()) Command(Icons.Outlined.Delete, "Remove", enabled = live.all { mayChange(it) }, danger = true) { dialog = DocDialog.Remove(live) }
                    }
                    if (single != null) Command(Icons.Outlined.Info, "Properties") { dialog = DocDialog.Properties(single) }
                    Command(Icons.Outlined.Close, "${selected.size} selected") { selected = emptySet(); selectMode = false }
                }
                Spacer(Modifier.width(12.dp))
                if (medium) Command(Icons.AutoMirrored.Outlined.ViewSidebar, if (ExplorerPrefs.detailsPane) "Hide details" else "Details") { ExplorerPrefs.detailsPane = !ExplorerPrefs.detailsPane }
            }
            if (busy || status != null) LinearProgressIndicator(Modifier.fillMaxWidth().height(2.dp), color = pp.accent, trackColor = pp.bg2)
            if (space.waiting) Banner("Documents open once a parent approves you.", Tone.Warn, Modifier.fillMaxWidth().padding(8.dp))
            Row(Modifier.weight(1f)) {
                if (wide && ExplorerPrefs.tree) {
                    Tree(all, loc, shared.size, dropTarget, targets, Modifier.width(230.dp).fillMaxHeight()) { go(it) }
                    VerticalDivider(color = pp.line)
                }
                // The listing; right-click on empty space for the folder's menu.
                ContextMenuArea({ backgroundMenu() }, Modifier.weight(1f).fillMaxHeight(), yieldToChildren = true,
                    onClick = { if (!it.ctrl && !it.shift) { selected = emptySet(); anchor = null }; runCatching { focus.requestFocus() } }) {
                    Column(Modifier.fillMaxSize()) {
                        if (q.isNotEmpty()) Text("Results for \"$q\" in ${if (loc is Loc.Folder) "all folders" else loc.title()} (${entries.size})",
                            color = pp.fgDim, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(start = 12.dp, top = 8.dp))
                        if (entries.isEmpty()) {
                            Box(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp)) {
                                EmptyState(Icons.Outlined.FolderOpen, emptyTitle(loc, q, all.isEmpty()), emptyBody(loc, q, canEdit))
                            }
                        } else if (ExplorerPrefs.view == ViewMode.Details) {
                            if (medium) Header(loc, q.isNotEmpty())
                            LazyColumn(Modifier.fillMaxSize(), state = listState, contentPadding = PaddingValues(bottom = 24.dp)) {
                                items(entries, key = { it.key }) { e ->
                                    var rowPos by remember { mutableStateOf(Offset.Zero) }
                                    val folderTarget = e.doc?.takeIf { it.isFolder && !it.removed }?.path
                                    ContextMenuArea({
                                        if (e.key !in selected) { selected = setOf(e.key); anchor = e.key }
                                        itemMenu(e)
                                    }, Modifier.fillMaxWidth().onGloballyPositioned { c ->
                                        rowPos = c.positionInRoot()
                                        if (folderTarget != null) targets["row:$folderTarget"] = c.boundsInRoot() else targets.remove("row:${e.key}")
                                    },
                                        onClick = { k -> runCatching { focus.requestFocus() }; if (phone && !selectMode) openEntry(e) else select(e, k) },
                                        onDoubleClick = { openEntry(e) },
                                        onContext = { if (e.key !in selected) { selected = setOf(e.key); anchor = e.key } },
                                        onDragStart = if (e.doc != null && !e.removed && canEdit) { p ->
                                            if (e.key !in selected) { selected = setOf(e.key); anchor = e.key }
                                            dragging = (if (e.key in selected) chosenDocs else listOf(e.doc)).filter { !it.removed }
                                            origin = rowPos
                                            dragAt = rowPos + p
                                        } else null,
                                        onDrag = { p -> dragAt = rowPos + p },
                                        onDragEnd = {
                                            val t = dropTarget
                                            val list = dragging
                                            dragging = null
                                            if (t != null && list != null) {
                                                val to = t.removePrefix("row:").removePrefix("tree:").removePrefix("crumb:")
                                                if (list.none { it.path == to || it.folder == to }) dialog = DocDialog.MoveCopy(list, copy = !list.all { mayChange(it) }, to = to)
                                            }
                                        },
                                    ) {
                                        DetailRow(e, e.key in selected, selectMode || (phone && selected.isNotEmpty()), medium, loc, q.isNotEmpty(),
                                            cut = ExplorerClipboard.clip?.let { c -> c.cut && c.docs.any { it.id.isNotEmpty() && it.id == e.doc?.id } } == true,
                                            dropping = dropTarget == "row:$folderTarget" && folderTarget != null)
                                    }
                                }
                            }
                        } else {
                            LazyVerticalGrid(GridCells.Adaptive(118.dp), Modifier.fillMaxSize(), contentPadding = PaddingValues(8.dp)) {
                                items(entries, key = { it.key }) { e ->
                                    ContextMenuArea({
                                        if (e.key !in selected) { selected = setOf(e.key); anchor = e.key }
                                        itemMenu(e)
                                    }, Modifier.padding(4.dp),
                                        onClick = { k -> runCatching { focus.requestFocus() }; if (phone && !selectMode) openEntry(e) else select(e, k) },
                                        onDoubleClick = { openEntry(e) },
                                        onContext = { if (e.key !in selected) { selected = setOf(e.key); anchor = e.key } }) {
                                        IconTile(e, e.key in selected)
                                    }
                                }
                            }
                        }
                    }
                }
                if (wide && ExplorerPrefs.detailsPane) {
                    VerticalDivider(color = pp.line)
                    Column(Modifier.width(320.dp).fillMaxHeight().verticalScroll(rememberScrollState()).padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        DetailsPane(single, chosen, entries, all, platform, onOpen = { d, a -> open(d, a) }, onHistory = { d -> dialog = DocDialog.History(d) },
                            onShared = { e -> openEntry(e) })
                    }
                }
            }
            // Status bar.
            Row(Modifier.fillMaxWidth().background(pp.panel).padding(horizontal = 12.dp, vertical = 5.dp), verticalAlignment = Alignment.CenterVertically) {
                val files = entries.filter { !it.isFolder }
                Text(status ?: buildString {
                    append("${entries.size} item${if (entries.size == 1) "" else "s"}")
                    if (selected.isNotEmpty()) {
                        append(" · ${chosen.size} selected")
                        val bytes = chosen.sumOf { it.size }
                        if (bytes > 0) append(" · ${sizeText(bytes)}")
                    } else if (files.isNotEmpty()) append(" · ${sizeText(files.sumOf { it.size })}")
                    ExplorerClipboard.clip?.let { append(" · ${it.docs.size} ${if (it.cut) "cut" else "copied"}") }
                }, color = pp.fgDim, style = MaterialTheme.typography.labelSmall, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                if (medium) Text("End-to-end encrypted · every version kept · openings recorded", color = pp.fgMute, style = MaterialTheme.typography.labelSmall, maxLines = 1)
            }
        }
        // Drop overlays: files from the system, and the drag-to-move ghost.
        if (dropHover) Box(Modifier.fillMaxSize().background(pp.accent.copy(alpha = 0.12f)).border(2.dp, pp.accent, RoundedCornerShape(Radius.small)),
            contentAlignment = Alignment.Center) {
            Text("Drop to encrypt and upload to ${folder?.let { if (it.isEmpty()) "Documents" else it } ?: "Documents"}", color = pp.accent,
                style = MaterialTheme.typography.titleMedium, modifier = Modifier.background(pp.panel, RoundedCornerShape(Radius.small)).padding(16.dp))
        }
        dragging?.let { list ->
            val d = LocalDensity.current
            val p = dragAt - rootPos
            Row(Modifier.offset { IntOffset(p.x.toInt() + 14, p.y.toInt() + 10) }.background(pp.panel, RoundedCornerShape(Radius.small))
                .border(1.dp, pp.accent, RoundedCornerShape(Radius.small)).padding(horizontal = 10.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.AutoMirrored.Outlined.DriveFileMove, null, tint = pp.accent, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text(buildString {
                    append(if (list.size == 1) list[0].name else "${list.size} items")
                    dropTarget?.let { append(" → " + it.substringAfter(':').ifEmpty { "Documents" }.substringAfterLast('/')) }
                }, color = pp.fg, style = MaterialTheme.typography.labelMedium)
            }
            @Suppress("UNUSED_EXPRESSION") d
        }
    }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    dialog?.let { d -> DocDialogs(d, all, families, busy, engine, space.id, act, onOpen = { doc, a -> open(doc, a) }) { dialog = null } }
}

@Composable
private fun ToolIcon(icon: ImageVector, label: String, enabled: Boolean, onClick: () -> Unit) {
    IconButton(onClick = onClick, enabled = enabled) { Icon(icon, label, tint = if (enabled) pp.fg else pp.fgMute.copy(alpha = 0.5f)) }
}

@Composable
private fun Command(icon: ImageVector, label: String, enabled: Boolean = true, danger: Boolean = false, busy: Boolean = false, onClick: () -> Unit) {
    Row(Modifier.clip(RoundedCornerShape(Radius.small)).clickable(enabled = enabled && !busy, onClick = onClick).padding(horizontal = 10.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically) {
        val c = when {
            !enabled -> pp.fgMute
            danger -> pp.danger
            else -> pp.fg
        }
        Icon(icon, null, tint = if (enabled && !danger) pp.accent else c, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(6.dp))
        Text(label, color = c, style = MaterialTheme.typography.labelLarge, maxLines = 1)
    }
}

/** The address bar: Documents › School › Reports, each part a link and a drop target. */
@Composable
private fun Address(loc: Loc, modifier: Modifier, dropTarget: String?, targets: MutableMap<String, Rect>, onGo: (Loc) -> Unit) {
    Row(modifier.padding(horizontal = 4.dp).clip(RoundedCornerShape(Radius.small)).background(pp.bg2).horizontalScroll(rememberScrollState())
        .padding(horizontal = 6.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
        @Composable
        fun crumb(label: String, target: Loc, path: String?, last: Boolean) {
            val key = path?.let { "crumb:$it" }
            Text(label, Modifier.clip(RoundedCornerShape(Radius.small))
                .then(if (key != null) Modifier.onGloballyPositioned { targets[key] = it.boundsInRoot() } else Modifier)
                .background(if (key != null && key == dropTarget) pp.accentSoft else Color.Transparent)
                .clickable { onGo(target) }.padding(horizontal = 6.dp, vertical = 6.dp),
                color = if (last) pp.fg else pp.accent, fontWeight = if (last) FontWeight.SemiBold else null, maxLines = 1,
                style = MaterialTheme.typography.bodyMedium)
        }
        when (loc) {
            is Loc.Folder -> {
                crumb("Documents", Loc.Folder(""), "", loc.path.isEmpty())
                val parts = if (loc.path.isEmpty()) emptyList() else loc.path.split('/')
                parts.forEachIndexed { i, p ->
                    Icon(Icons.Outlined.ChevronRight, null, tint = pp.fgMute, modifier = Modifier.size(16.dp))
                    val path = parts.take(i + 1).joinToString("/")
                    crumb(p, Loc.Folder(path), path, i == parts.lastIndex)
                }
            }
            else -> crumb(loc.title(), loc, null, true)
        }
    }
}

@Composable
private fun SearchBox(value: String, onChange: (String) -> Unit, hint: String, modifier: Modifier, onClear: () -> Unit) {
    Row(modifier.padding(horizontal = 4.dp).clip(RoundedCornerShape(Radius.small)).background(pp.bg2).padding(horizontal = 8.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically) {
        Icon(Icons.Outlined.Search, null, tint = pp.fgMute, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(6.dp))
        Box(Modifier.weight(1f)) {
            if (value.isEmpty()) Text(hint, color = pp.fgMute, style = MaterialTheme.typography.bodyMedium, maxLines = 1)
            BasicTextField(value, onChange, singleLine = true, textStyle = TextStyle(color = pp.fg, fontSize = MaterialTheme.typography.bodyMedium.fontSize),
                cursorBrush = SolidColor(pp.accent), modifier = Modifier.fillMaxWidth().onPreviewKeyEvent {
                    if (it.type == KeyEventType.KeyDown && it.key == Key.Escape) { onClear(); true } else false
                })
        }
        if (value.isNotEmpty()) Icon(Icons.Outlined.Close, "Clear search", tint = pp.fgMute, modifier = Modifier.size(18.dp).clickable { onClear() })
    }
}

/** The navigation pane: places, and the folder tree (folders are drop targets). */
@Composable
private fun Tree(all: List<Doc>, loc: Loc, sharedCount: Int, dropTarget: String?, targets: MutableMap<String, Rect>, modifier: Modifier, onGo: (Loc) -> Unit) {
    val paths = folderPaths(all)
    val open = remember { mutableStateMapOf<String, Boolean>() }
    val current = (loc as? Loc.Folder)?.path
    // The way to the current folder is always open.
    current?.split('/')?.runningReduce { a, b -> "$a/$b" }?.forEach { if (open[it] == null) open[it] = true }
    Column(modifier.background(pp.bg2).verticalScroll(rememberScrollState()).padding(vertical = 6.dp)) {
        @Composable
        fun node(label: String, icon: ImageVector, target: Loc, depth: Int, path: String?, hasKids: Boolean, count: Int? = null) {
            val sel = target == loc
            val key = path?.let { "tree:$it" }
            val expanded = path != null && (path.isEmpty() || open[path] == true)
            Row(Modifier.fillMaxWidth().padding(horizontal = 6.dp).clip(RoundedCornerShape(Radius.small))
                .then(if (key != null) Modifier.onGloballyPositioned { targets[key] = it.boundsInRoot() } else Modifier)
                .background(when {
                    key != null && key == dropTarget -> pp.accent.copy(alpha = 0.25f)
                    sel -> pp.accentSoft
                    else -> Color.Transparent
                }).clickable { onGo(target) }.padding(start = (4 + depth * 14).dp, top = 5.dp, bottom = 5.dp, end = 6.dp),
                verticalAlignment = Alignment.CenterVertically) {
                if (hasKids && path != null && path.isNotEmpty()) Icon(if (expanded) Icons.Outlined.ExpandMore else Icons.Outlined.ChevronRight,
                    if (expanded) "Collapse" else "Expand", tint = pp.fgMute, modifier = Modifier.size(16.dp).clickable { open[path] = !expanded })
                else Spacer(Modifier.width(16.dp))
                Spacer(Modifier.width(4.dp))
                Icon(icon, null, tint = if (path != null) pp.warm else pp.accent, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(label, color = if (sel) pp.fg else pp.fgDim, fontWeight = if (sel) FontWeight.SemiBold else null, maxLines = 1,
                    overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                if (count != null && count > 0) Text("$count", color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
            }
        }
        @Composable
        fun folders(parent: String, depth: Int) {
            childFolders(paths, parent).forEach { p ->
                val kids = childFolders(paths, p).isNotEmpty()
                node(p.substringAfterLast('/'), if (p == current) Icons.Outlined.FolderOpen else Icons.Outlined.Folder, Loc.Folder(p), depth, p, kids)
                if (kids && open[p] == true) folders(p, depth + 1)
            }
        }
        node("Recent", Icons.Outlined.Schedule, Loc.Recent, 0, null, false)
        node("Shared in messages", Icons.Outlined.Forum, Loc.Shared, 0, null, false, sharedCount)
        Spacer(Modifier.height(6.dp))
        node("Documents", if (current == "") Icons.Outlined.FolderOpen else Icons.Outlined.Folder, Loc.Folder(""), 0, "", paths.isNotEmpty())
        folders("", 1)
        Spacer(Modifier.height(6.dp))
        node("Removed", Icons.Outlined.Delete, Loc.Removed, 0, null, false, all.count { it.removed })
    }
}

private val colModified = 150.dp
private val colType = 130.dp
private val colSize = 80.dp
private val colAuthor = 110.dp

@Composable
private fun Header(loc: Loc, searching: Boolean) {
    Row(Modifier.fillMaxWidth().background(pp.bg2).padding(start = 46.dp, end = 12.dp), verticalAlignment = Alignment.CenterVertically) {
        @Composable
        fun col(by: SortBy, label: String, m: Modifier, end: Boolean = false) {
            val on = ExplorerPrefs.sortBy == by
            Row(m.clip(RoundedCornerShape(Radius.small)).clickable {
                if (on) ExplorerPrefs.descending = !ExplorerPrefs.descending else { ExplorerPrefs.sortBy = by; ExplorerPrefs.descending = by == SortBy.Modified }
            }.padding(vertical = 7.dp, horizontal = 4.dp), verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = if (end) Arrangement.End else Arrangement.Start) {
                Text(label, color = if (on) pp.fg else pp.fgDim, style = MaterialTheme.typography.labelMedium, maxLines = 1)
                if (on) Icon(if (ExplorerPrefs.descending) Icons.Outlined.ArrowDownward else Icons.Outlined.ArrowUpward, null, tint = pp.fgDim, modifier = Modifier.size(14.dp))
            }
        }
        col(SortBy.Name, "Name", Modifier.weight(1f))
        if (searching || loc == Loc.Recent || loc == Loc.Removed || loc == Loc.Shared) Text("Location", color = pp.fgDim, style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.width(colType).padding(horizontal = 4.dp))
        col(SortBy.Modified, if (loc == Loc.Shared) "Shared" else "Date modified", Modifier.width(colModified))
        col(SortBy.Type, "Type", Modifier.width(colType))
        col(SortBy.Size, "Size", Modifier.width(colSize), end = true)
        col(SortBy.Author, if (loc == Loc.Shared) "From" else "Added by", Modifier.width(colAuthor))
    }
    HorizontalDivider(color = pp.line)
}

@Composable
private fun DetailRow(e: Entry, selected: Boolean, checks: Boolean, columns: Boolean, loc: Loc, searching: Boolean, cut: Boolean, dropping: Boolean) {
    val d = e.doc
    Row(Modifier.fillMaxWidth().background(when {
        dropping -> pp.accent.copy(alpha = 0.25f)
        selected -> pp.accentSoft
        else -> Color.Transparent
    }).padding(horizontal = 8.dp, vertical = if (columns) 5.dp else 9.dp), verticalAlignment = Alignment.CenterVertically) {
        if (checks) Icon(if (selected) Icons.Outlined.CheckBox else Icons.Outlined.CheckBoxOutlineBlank, null, tint = if (selected) pp.accent else pp.fgMute,
            modifier = Modifier.size(20.dp).padding(end = 2.dp))
        Thumb(e, if (columns) 26 else 36, faded = cut || e.removed)
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Text(e.name, color = if (e.removed || cut) pp.fgMute else pp.fg, maxLines = 1, overflow = TextOverflow.Ellipsis,
                textDecoration = if (e.removed) TextDecoration.LineThrough else null, fontWeight = if (e.isFolder) FontWeight.SemiBold else null,
                style = MaterialTheme.typography.bodyMedium)
            if (!columns) {
                val detail = listOfNotNull(
                    if (searching || loc != Loc.Shared && loc !is Loc.Folder) e.where else null,
                    if (e.isFolder) null else sizeText(e.size),
                    if (e.author.isNotEmpty()) e.author else null,
                    shortTime(e.modified).takeIf { e.modified.isNotEmpty() },
                    d?.versions?.size?.takeIf { it > 1 }?.let { "$it versions" },
                    if (e.removed) "removed" else null,
                ).joinToString(" · ")
                Text(detail, color = pp.fgDim, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
        if (columns) {
            val cell = MaterialTheme.typography.bodySmall
            if (searching || loc == Loc.Recent || loc == Loc.Removed || loc == Loc.Shared) Text(e.where, color = pp.fgDim, style = cell, maxLines = 1,
                overflow = TextOverflow.Ellipsis, modifier = Modifier.width(colType).padding(horizontal = 4.dp))
            Text(if (e.modified.isNotEmpty() && d?.implied != true) shortTime(e.modified) else "", color = pp.fgDim, style = cell, maxLines = 1,
                modifier = Modifier.width(colModified).padding(horizontal = 4.dp))
            Text(e.type, color = pp.fgDim, style = cell, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.width(colType).padding(horizontal = 4.dp))
            Text(if (e.isFolder) "" else sizeText(e.size), color = pp.fgDim, style = cell, maxLines = 1, textAlign = TextAlign.End,
                modifier = Modifier.width(colSize).padding(horizontal = 4.dp))
            Text(e.author, color = pp.fgDim, style = cell, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.width(colAuthor).padding(horizontal = 4.dp))
        }
    }
}

@Composable
private fun IconTile(e: Entry, selected: Boolean) {
    Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small)).background(if (selected) pp.accentSoft else Color.Transparent).padding(8.dp),
        horizontalAlignment = Alignment.CenterHorizontally) {
        Thumb(e, 64, faded = e.removed)
        Spacer(Modifier.height(6.dp))
        Text(e.name, color = if (e.removed) pp.fgMute else pp.fg, style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.Center, textDecoration = if (e.removed) TextDecoration.LineThrough else null)
    }
}

/** A picture's thumbnail (made on the sender's device, inside the encrypted record), else the type's icon. */
@Composable
private fun Thumb(e: Entry, size: Int, faded: Boolean = false) {
    val platform = LocalPlatform.current
    val thumb = e.file?.thumb
    val img = remember(thumb) { thumb?.let { runCatching { platform.decodeImage(platform.base64(it)) }.getOrNull() } }
    if (img != null) {
        Image(img, null, Modifier.size(size.dp).clip(RoundedCornerShape(4.dp)), contentScale = ContentScale.Crop, alpha = if (faded) 0.5f else 1f)
    } else {
        Icon(iconFor(e), null, tint = (if (e.isFolder) pp.warm else pp.accent).copy(alpha = if (faded) 0.5f else 1f), modifier = Modifier.size(size.dp))
    }
}

internal fun iconFor(e: Entry): ImageVector {
    val mime = e.file?.mime.orEmpty()
    return when {
        e.isFolder -> Icons.Outlined.Folder
        mime.startsWith("image/") -> Icons.Outlined.Image
        mime.startsWith("video/") -> Icons.Outlined.Videocam
        mime == "application/pdf" -> Icons.Outlined.PictureAsPdf
        mime.startsWith("text/") -> Icons.Outlined.Description
        else -> Icons.AutoMirrored.Outlined.InsertDriveFile
    }
}

internal fun iconFor(d: Doc): ImageVector = iconFor(Entry(d))

@Composable
private fun DetailsPane(
    single: Entry?, chosen: List<Entry>, entries: List<Entry>, all: List<Doc>, platform: systems.darkcipher.parallelpath.ui.PlatformServices,
    onOpen: (Doc, Attachment) -> Unit, onHistory: (Doc) -> Unit, onShared: (Entry) -> Unit,
) {
    when {
        single != null -> {
            val thumb = single.file?.thumb
            val img = remember(thumb) { thumb?.let { runCatching { platform.decodeImage(platform.base64(it)) }.getOrNull() } }
            if (img != null) Image(img, single.name, Modifier.fillMaxWidth().height(180.dp).clip(RoundedCornerShape(Radius.small)), contentScale = ContentScale.Fit)
            else Icon(iconFor(single), null, tint = if (single.isFolder) pp.warm else pp.accent, modifier = Modifier.size(56.dp))
            Text(single.name, color = pp.fg, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            if (single.removed) Banner("Removed${single.doc?.reason?.takeIf { it.isNotBlank() }?.let { ": $it" } ?: ""}. Kept for the record and can still be opened.", Tone.Warn)
            EntryDetails(single, all, onOpen = { a ->
                if (single.doc != null) onOpen(single.doc, a) else onShared(single)
            }, onHistory = single.doc?.let { d -> { onHistory(d) } })
        }
        chosen.size > 1 -> {
            Icon(Icons.Outlined.SelectAll, null, tint = pp.accent, modifier = Modifier.size(48.dp))
            Text("${chosen.size} items selected", color = pp.fg, style = MaterialTheme.typography.titleMedium)
            val files = chosen.filter { !it.isFolder }
            Text("${files.size} file${if (files.size == 1) "" else "s"} · ${sizeText(files.sumOf { it.size })}" +
                (chosen.size - files.size).let { if (it > 0) " · $it folder${if (it == 1) "" else "s"}" else "" }, color = pp.fgDim)
            Text("Right-click for what you can do with them.", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
        }
        else -> {
            Icon(Icons.Outlined.Info, null, tint = pp.fgMute, modifier = Modifier.size(40.dp))
            Text("Select a file or folder to see its details, versions and who has opened it.", color = pp.fgDim)
            val files = entries.filter { !it.isFolder }
            if (entries.isNotEmpty()) Text("Here: ${entries.size - files.size} folders, ${files.size} files, ${sizeText(files.sumOf { it.size })}",
                color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
            Text("Tips: double-click to open · right-click for everything else · drag files onto a folder to move them · " +
                "drop files from your computer here to upload them · Ctrl+F searches every folder.", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
        }
    }
}

private fun emptyTitle(loc: Loc, q: String, nothing: Boolean) = when {
    q.isNotEmpty() -> "Nothing matches \"$q\""
    loc == Loc.Recent -> "No files yet"
    loc == Loc.Shared -> "No files shared in messages yet"
    loc == Loc.Removed -> "Nothing has been removed"
    nothing -> "No documents yet"
    else -> "This folder is empty"
}

private fun emptyBody(loc: Loc, q: String, canEdit: Boolean) = when {
    q.isNotEmpty() -> "Search looks at names in every folder. Removed things are included when Show removed is on."
    loc == Loc.Shared -> "Photos and files sent in the family's messages and your private conversations appear here. Right-click one to save it to Documents."
    loc == Loc.Removed -> "Removed files and folders stay here for the record; whoever added one can restore it."
    !canEdit -> "Files the family adds appear here."
    else -> "School reports, medical letters, court orders, passports - upload them here, or drag them in from your computer. " +
        "Everyone in the family can open them, every version is kept, and each time someone opens one it's recorded."
}

internal fun sizeText(n: Long): String = when {
    n >= 1L shl 30 -> "${(n * 10 / (1L shl 30)) / 10.0} GB"
    n >= 1L shl 20 -> "${(n * 10 / (1L shl 20)) / 10.0} MB"
    n >= 1024 -> "${n / 1024} KB"
    else -> "$n bytes"
}
