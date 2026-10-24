package systems.darkcipher.parallelpath.chat

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Reply
import androidx.compose.material.icons.automirrored.outlined.Send
import androidx.compose.material.icons.outlined.AttachFile
import androidx.compose.material.icons.outlined.Call
import androidx.compose.material.icons.outlined.Videocam
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.DoneAll
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.EmojiEmotions
import androidx.compose.material.icons.outlined.FolderOpen
import androidx.compose.material.icons.outlined.Forum
import androidx.compose.material.icons.outlined.Save
import androidx.compose.material.icons.automirrored.outlined.DriveFileMove
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.automirrored.outlined.InsertDriveFile
import androidx.compose.material.icons.outlined.Lightbulb
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.outlined.PlayCircle
import androidx.compose.material.icons.outlined.RemoveCircleOutline
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Attachment
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.model.Message
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.onRightClick
import systems.darkcipher.parallelpath.ui.rememberToneGate
import systems.darkcipher.parallelpath.ui.rememberLiveTone
import systems.darkcipher.parallelpath.ui.ToneBar
import systems.darkcipher.parallelpath.ui.AdviceDialog
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.shortTime

/**
 * The family conversation. [tick] changes whenever the engine reports new data, which reloads the messages.
 */
@Composable
fun ChatScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit, family: String = space.id) {
    var messages by remember { mutableStateOf<List<Message>>(emptyList()) }
    var replyTo by remember { mutableStateOf<Message?>(null) }
    var editing by remember { mutableStateOf<Message?>(null) }
    var withdrawing by remember { mutableStateOf<Message?>(null) }
    var history by remember { mutableStateOf<Message?>(null) }
    var viewer by remember { mutableStateOf<Pair<String, ImageBitmap>?>(null) }
    val scope = rememberCoroutineScope()
    val list = rememberLazyListState()
    val platform = LocalPlatform.current

    LaunchedEffect(space.id, tick) {
        try {
            val ms = engine.messages(space.id)
            val grew = ms.size > messages.size
            messages = ms
            if (grew && ms.isNotEmpty()) list.animateScrollToItem(ms.size - 1)
            // Opening the conversation records read receipts for messages from others (once each, audited).
            ms.takeLast(50).filter { !it.mine && !it.locked }.forEach { m ->
                runCatching { engine.markRead(space.id, m.id) }
            }
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e // a screen closing or reloading isn't an error
        } catch (e: Exception) {
            onError(e.message ?: "couldn't load messages")
        }
    }

    fun act(block: suspend () -> Unit) = scope.launch {
        try {
            block()
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e // a screen closing or reloading isn't an error
        } catch (e: Exception) {
            onError(e.message ?: "something went wrong")
        }
    }

    fun open(m: Message, a: Attachment) = act {
        val path = engine.openAttachment(space.id, m.id, a.blobId)
        val bytes = if (a.isImage) platform.readFile(path) else null
        val img = bytes?.let { platform.decodeImage(it) }
        if (img != null) viewer = a.name to img else platform.openFile(path, a.mime)
    }

    // A decrypted copy saved where the person chooses (no longer encrypted - the save dialog is theirs).
    fun saveCopy(m: Message, a: Attachment) = act {
        val path = engine.openAttachment(space.id, m.id, a.blobId)
        platform.saveCopy(path, a.name, a.mime)
    }

    // Filed into the family's documents (top level), where versions and openings are kept.
    fun saveToDocuments(m: Message, a: Attachment) = act {
        engine.saveSharedFile(family, space.id, m.id, a.blobId, "")
        onError("Saved ${a.name} to Documents.")
    }

    Column(Modifier.fillMaxSize()) {
        if (space.waiting) {
            Banner(
                "A parent needs to approve this device before you can read or send messages. Compare your safety number " +
                    "with them (Family tab) on a call or in person.",
                Tone.Warn, Modifier.fillMaxWidth().padding(12.dp),
            )
        } else if (!space.canChat) {
            Banner("You have read-only access to this family.", Tone.Info, Modifier.fillMaxWidth().padding(12.dp))
        }
        if (space.auditProblem.isNotBlank()) {
            Banner("The audit log failed verification: ${space.auditProblem}", Tone.Error, Modifier.fillMaxWidth().padding(12.dp))
        }
        Box(Modifier.weight(1f).fillMaxWidth()) {
            if (messages.isEmpty() && !space.waiting) {
                EmptyState(Icons.Outlined.Forum, "No messages yet", "Everything here is end-to-end encrypted and kept in the family record.",
                    Modifier.align(Alignment.Center))
            }
            LazyColumn(state = list, modifier = Modifier.fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)) {
                items(messages, key = { it.id }) { m ->
                    m.call?.let { c -> CallRow(m, c); return@items }
                    MessageBubble(
                        m, messages.firstOrNull { it.id == m.replyTo },
                        onReact = { emoji, remove -> act { engine.react(space.id, m.id, emoji, remove) } },
                        onReply = { replyTo = m },
                        onEdit = { editing = m },
                        onWithdraw = { withdrawing = m },
                        onHistory = { history = m },
                        onOpen = { a -> open(m, a) },
                        onSaveCopy = if (platform.canSaveCopies) { a -> saveCopy(m, a) } else null,
                        onSaveToDocuments = if (!space.readOnly && !space.waiting) { a -> saveToDocuments(m, a) } else null,
                        canWrite = space.canChat && !space.waiting,
                    )
                }
            }
        }
        if (space.canChat && !space.waiting) {
            Composer(replyTo, onCancelReply = { replyTo = null }) { text, files ->
                val reply = replyTo?.id
                replyTo = null
                act { engine.send(space.id, text, reply, files) }
            }
        }
    }

    editing?.let { m ->
        ReasonDialog(
            title = "Edit message", initialText = m.text, needsText = true,
            explain = "The original stays visible in the message's history, together with your reason.",
            confirm = "Save edit",
            onDismiss = { editing = null },
        ) { text, reason ->
            editing = null
            act { engine.edit(space.id, m.id, text, reason) }
        }
    }
    withdrawing?.let { m ->
        ReasonDialog(
            title = "Withdraw message", initialText = null, needsText = false,
            explain = "The message is marked as withdrawn for everyone. It is NOT deleted: it stays in the history and the audit log, with your reason.",
            confirm = "Withdraw",
            onDismiss = { withdrawing = null },
        ) { _, reason ->
            withdrawing = null
            act { engine.retract(space.id, m.id, reason) }
        }
    }
    history?.let { m -> HistoryDialog(m) { history = null } }
    viewer?.let { (name, img) -> ImageDialog(name, img) { viewer = null } }
}

@OptIn(ExperimentalFoundationApi::class, ExperimentalLayoutApi::class)
@Composable
private fun MessageBubble(
    m: Message, parent: Message?, canWrite: Boolean,
    onReact: (String, Boolean) -> Unit, onReply: () -> Unit, onEdit: () -> Unit, onWithdraw: () -> Unit,
    onHistory: () -> Unit, onOpen: (Attachment) -> Unit,
    onSaveCopy: ((Attachment) -> Unit)? = null, onSaveToDocuments: ((Attachment) -> Unit)? = null,
) {
    var menu by remember { mutableStateOf(false) }
    val platform = LocalPlatform.current
    val shape = RoundedCornerShape(
        topStart = Radius.large, topEnd = Radius.large,
        bottomStart = if (m.mine) Radius.large else 4.dp, bottomEnd = if (m.mine) 4.dp else Radius.large,
    )
    Row(Modifier.fillMaxWidth(), horizontalArrangement = if (m.mine) Arrangement.End else Arrangement.Start) {
        Column(horizontalAlignment = if (m.mine) Alignment.End else Alignment.Start, modifier = Modifier.widthIn(max = 560.dp)) {
            Box {
                Column(
                    Modifier.clip(shape)
                        .background(if (m.mine) pp.accentSoft else pp.panel2)
                        .border(1.dp, if (m.mine) pp.accent.copy(alpha = 0.45f) else pp.line, shape)
                        .combinedClickable(onClick = {}, onLongClick = { menu = true })
                        .onRightClick { menu = true }
                        .padding(start = 14.dp, end = 4.dp, top = 8.dp, bottom = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(if (m.mine) "You" else m.authorName, color = if (m.mine) pp.accent else pp.warm,
                            style = MaterialTheme.typography.labelLarge, modifier = Modifier.weight(1f, fill = false))
                        Spacer(Modifier.width(8.dp))
                        Text(shortTime(m.sent), color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
                        IconButton(onClick = { menu = true }, modifier = Modifier.size(28.dp)) {
                            Icon(Icons.Outlined.MoreVert, "Message actions", tint = pp.fgMute, modifier = Modifier.size(18.dp))
                        }
                    }
                    Column(Modifier.padding(end = 10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        parent?.let { p ->
                            Row(Modifier.clip(RoundedCornerShape(6.dp)).background(pp.bg2).padding(8.dp)) {
                                Box(Modifier.width(3.dp).heightIn(min = 16.dp).background(pp.accent))
                                Spacer(Modifier.width(8.dp))
                                Text("${p.authorName}: ${if (p.retracted) "withdrawn message" else p.text.take(80)}",
                                    color = pp.fgDim, style = MaterialTheme.typography.bodySmall, maxLines = 2)
                            }
                        }
                        when {
                            m.locked -> Row(verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Outlined.Lock, null, tint = pp.fgMute, modifier = Modifier.size(16.dp))
                                Spacer(Modifier.width(6.dp))
                                Text("Encrypted for a key this device doesn't have yet", color = pp.fgMute, fontStyle = FontStyle.Italic)
                            }
                            m.problem.isNotBlank() -> Text("This message failed verification and isn't shown (${m.problem}).", color = pp.danger)
                            m.retracted -> Column {
                                Text("Message withdrawn", color = pp.fgDim, fontStyle = FontStyle.Italic)
                                Text("Reason: ${m.reason}", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
                            }
                            else -> {
                                if (m.text.isNotBlank()) SelectionContainer { Text(m.text, color = pp.fg, style = emojiAware(m.text)) }
                                if (m.attachments.isNotEmpty()) {
                                    FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                        m.attachments.forEach { a -> AttachmentTile(a, platform.thumb(a)) { onOpen(a) } }
                                    }
                                }
                            }
                        }
                        if (m.edited || m.retracted) {
                            Text(if (m.retracted) "View history" else "Edited · ${m.reason} · View history",
                                color = pp.accent, style = MaterialTheme.typography.labelSmall,
                                modifier = Modifier.clickable(onClick = onHistory))
                        }
                    }
                }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    if (canWrite && !m.locked && !m.retracted) {
                        Row(Modifier.padding(horizontal = 8.dp)) {
                            Emoji.quick.forEach { e ->
                                val mine = m.reactions.any { it.emoji == e && it.mine }
                                Text(e, fontSize = 22.sp, modifier = Modifier.clip(CircleShape)
                                    .background(if (mine) pp.accentSoft else androidx.compose.ui.graphics.Color.Transparent)
                                    .clickable { menu = false; onReact(e, mine) }.padding(6.dp))
                            }
                        }
                        HorizontalDivider(color = pp.line)
                        DropdownMenuItem(text = { Text("Reply") }, leadingIcon = { Icon(Icons.AutoMirrored.Outlined.Reply, null) },
                            onClick = { menu = false; onReply() })
                    }
                    if (m.text.isNotBlank() && !m.retracted) {
                        DropdownMenuItem(text = { Text("Copy text") }, leadingIcon = { Icon(Icons.Outlined.ContentCopy, null) },
                            onClick = { menu = false; platform.copyText(m.text) })
                    }
                    if (!m.retracted && !m.locked) m.attachments.forEach { a ->
                        HorizontalDivider(color = pp.line)
                        DropdownMenuItem(text = { Text("Open ${a.name}", maxLines = 1) }, leadingIcon = { Icon(Icons.Outlined.FolderOpen, null) },
                            onClick = { menu = false; onOpen(a) })
                        if (onSaveCopy != null) DropdownMenuItem(text = { Text("Save a copy…") }, leadingIcon = { Icon(Icons.Outlined.Save, null) },
                            onClick = { menu = false; onSaveCopy(a) })
                        if (onSaveToDocuments != null) DropdownMenuItem(text = { Text("Save to Documents") },
                            leadingIcon = { Icon(Icons.AutoMirrored.Outlined.DriveFileMove, null) }, onClick = { menu = false; onSaveToDocuments(a) })
                    }
                    if (!m.retracted && !m.locked && m.attachments.isNotEmpty()) HorizontalDivider(color = pp.line)
                    if (m.mine && canWrite && !m.retracted && !m.locked) {
                        DropdownMenuItem(text = { Text("Edit (with a reason)") }, leadingIcon = { Icon(Icons.Outlined.Edit, null) },
                            onClick = { menu = false; onEdit() })
                        DropdownMenuItem(text = { Text("Withdraw (with a reason)", color = pp.danger) },
                            leadingIcon = { Icon(Icons.Outlined.RemoveCircleOutline, null, tint = pp.danger) },
                            onClick = { menu = false; onWithdraw() })
                    }
                    DropdownMenuItem(text = { Text("History") }, leadingIcon = { Icon(Icons.Outlined.History, null) },
                        onClick = { menu = false; onHistory() })
                }
            }
            if (m.reactions.isNotEmpty()) {
                FlowRow(Modifier.padding(top = 4.dp), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    m.reactions.forEach { r ->
                        Row(
                            Modifier.clip(CircleShape).background(if (r.mine) pp.accentSoft else pp.panel)
                                .border(1.dp, if (r.mine) pp.accent else pp.line, CircleShape)
                                .clickable(enabled = canWrite) { onReact(r.emoji, r.mine) }
                                .padding(horizontal = 8.dp, vertical = 2.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(r.emoji, fontSize = 15.sp)
                            Spacer(Modifier.width(4.dp))
                            Text("${r.count}", color = pp.fgDim, style = MaterialTheme.typography.labelSmall)
                        }
                    }
                }
            }
            if (m.mine && m.readBy.isNotEmpty()) {
                Row(Modifier.padding(top = 3.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.DoneAll, null, tint = pp.accent, modifier = Modifier.size(14.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("Seen by " + m.readBy.joinToString { "${it.name} ${shortTime(it.time)}" }, color = pp.fgMute,
                        style = MaterialTheme.typography.labelSmall)
                }
            }
        }
    }
}

/** Larger text for messages that are only emoji. */
@Composable
private fun emojiAware(text: String) =
    if (text.length <= 8 && text.none { it.isLetterOrDigit() }) MaterialTheme.typography.bodyLarge.copy(fontSize = 30.sp)
    else MaterialTheme.typography.bodyLarge

private fun systems.darkcipher.parallelpath.ui.PlatformServices.thumb(a: Attachment): ImageBitmap? =
    a.thumb?.let { runCatching { decodeImage(base64(it)) }.getOrNull() }

@Composable
private fun AttachmentTile(a: Attachment, thumb: ImageBitmap?, onOpen: () -> Unit) {
    val shape = RoundedCornerShape(Radius.small)
    if ((a.isImage || a.isVideo) && thumb != null) {
        Box(Modifier.clip(shape).clickable(onClick = onOpen)) {
            Image(thumb, a.name, contentScale = ContentScale.Crop, modifier = Modifier.sizeIn(maxWidth = 260.dp, maxHeight = 260.dp)
                .size(width = if (thumb.width >= thumb.height) 240.dp else 180.dp, height = if (thumb.width >= thumb.height) 170.dp else 240.dp))
            if (a.isVideo) Icon(Icons.Outlined.PlayCircle, "Play", tint = androidx.compose.ui.graphics.Color.White,
                modifier = Modifier.align(Alignment.Center).size(48.dp))
        }
    } else {
        Row(
            Modifier.clip(shape).background(pp.bg2).border(1.dp, pp.line, shape).clickable(onClick = onOpen).padding(10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(if (a.isVideo) Icons.Outlined.PlayCircle else Icons.AutoMirrored.Outlined.InsertDriveFile, null, tint = pp.accent)
            Spacer(Modifier.width(8.dp))
            Column {
                Text(a.name, color = pp.fg, style = MaterialTheme.typography.bodyMedium, maxLines = 1)
                Text(size(a.size), color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
            }
        }
    }
}

private fun size(n: Long): String = when {
    n < 1024 -> "$n B"
    n < 1024 * 1024 -> "${n / 1024} KB"
    n < 1024L * 1024 * 1024 -> "${(n * 10 / (1024 * 1024)) / 10.0} MB"
    else -> "${(n * 10 / (1024L * 1024 * 1024)) / 10.0} GB"
}

@Composable
private fun Composer(
    replyTo: Message?, onCancelReply: () -> Unit, onSend: (String, List<FileInput>) -> Unit,
) {
    var text by remember { mutableStateOf("") }
    var files by remember { mutableStateOf<List<FileInput>>(emptyList()) }
    var emojis by remember { mutableStateOf(false) }
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    val tone = rememberToneGate()
    val live = rememberLiveTone(text)
    var advice by remember { mutableStateOf(false) }
    fun send() {
        if (text.isBlank() && files.isEmpty()) return
        // The tone check first (on this device); the calmer version, if taken, goes back into the box to review.
        tone.guard(text, replace = { text = it }) { checked ->
            onSend(checked, files)
            text = ""
            files = emptyList()
            emojis = false
        }
    }
    Column(Modifier.fillMaxWidth().background(pp.bg2).padding(10.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        replyTo?.let { r ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.AutoMirrored.Outlined.Reply, null, tint = pp.accent, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text("Replying to ${r.authorName}: ${r.text.take(60)}", color = pp.fgDim, modifier = Modifier.weight(1f), maxLines = 1)
                IconButton(onClick = onCancelReply) { Icon(Icons.Outlined.Close, "Cancel reply", tint = pp.fgMute) }
            }
        }
        if (files.isNotEmpty()) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                files.forEach { f ->
                    Row(Modifier.clip(CircleShape).background(pp.panel2).padding(start = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text(f.name ?: f.path.substringAfterLast('/').substringAfterLast('\\'), color = pp.fg,
                            style = MaterialTheme.typography.labelMedium, maxLines = 1, modifier = Modifier.widthIn(max = 160.dp))
                        IconButton(onClick = { files = files - f }, modifier = Modifier.size(30.dp)) {
                            Icon(Icons.Outlined.Close, "Remove", modifier = Modifier.size(16.dp), tint = pp.fgMute)
                        }
                    }
                }
            }
        }
        // The live tone bar (worked out on this device as the person types) and advice on the draft.
        live?.let { ToneBar(it) { advice = true } }
        if (advice) AdviceDialog(live, onUseCalmer = { text = it; advice = false }) { advice = false }
        Row(verticalAlignment = Alignment.Bottom) {
            IconButton(onClick = { scope.launch { files = files + platform.pickFiles() } }) {
                Icon(Icons.Outlined.AttachFile, "Attach photos, videos or files", tint = pp.fgDim)
            }
            IconButton(onClick = { emojis = !emojis }) {
                Icon(Icons.Outlined.EmojiEmotions, "Emoji", tint = if (emojis) pp.accent else pp.fgDim)
            }
            IconButton(onClick = { advice = true }) {
                Icon(Icons.Outlined.Lightbulb, "Advice on what to write", tint = pp.fgDim)
            }
            Field(
                text, { text = it }, "Message (end-to-end encrypted)", singleLine = false,
                modifier = Modifier.weight(1f).heightIn(max = 180.dp).onPreviewKeyEvent {
                    if (it.type == KeyEventType.KeyDown && it.key == Key.Enter && !it.isShiftPressed) { send(); true } else false
                },
            )
            Spacer(Modifier.width(6.dp))
            IconButton(onClick = { send() }, enabled = text.isNotBlank() || files.isNotEmpty(),
                modifier = Modifier.clip(CircleShape).background(if (text.isNotBlank() || files.isNotEmpty()) pp.accent else pp.panel2)) {
                Icon(Icons.AutoMirrored.Outlined.Send, "Send", tint = if (text.isNotBlank() || files.isNotEmpty()) pp.accentInk else pp.fgMute)
            }
        }
        if (emojis) EmojiPicker { text += it }
    }
}

@Composable
private fun EmojiPicker(onPick: (String) -> Unit) {
    var cat by remember { mutableStateOf(0) }
    Column(Modifier.fillMaxWidth().heightIn(max = 260.dp).clip(RoundedCornerShape(Radius.small)).background(pp.panel)) {
        Row(Modifier.fillMaxWidth().padding(4.dp), horizontalArrangement = Arrangement.spacedBy(2.dp)) {
            Emoji.categories.forEachIndexed { i, (name, list) ->
                Text(list.first(), fontSize = 18.sp, modifier = Modifier.clip(CircleShape)
                    .background(if (i == cat) pp.accentSoft else androidx.compose.ui.graphics.Color.Transparent)
                    .clickable { cat = i }.padding(6.dp))
                if (i == cat) Text(name, color = pp.fgMute, style = MaterialTheme.typography.labelSmall, modifier = Modifier.align(Alignment.CenterVertically))
            }
        }
        LazyVerticalGrid(GridCells.Adaptive(40.dp), Modifier.fillMaxWidth().padding(4.dp)) {
            items(Emoji.categories[cat].second) { e ->
                Text(e, fontSize = 22.sp, modifier = Modifier.clip(CircleShape).clickable { onPick(e) }.padding(6.dp))
            }
        }
    }
}

/** Edit or withdraw: a reason is always required, and the dialog says it will be kept. */
@Composable
private fun ReasonDialog(
    title: String, initialText: String?, needsText: Boolean, explain: String, confirm: String,
    onDismiss: () -> Unit, onConfirm: (String, String) -> Unit,
) {
    var text by remember { mutableStateOf(initialText ?: "") }
    var reason by remember { mutableStateOf("") }
    val tone = rememberToneGate()
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = pp.panel,
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(explain, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                if (needsText) Field(text, { text = it }, "Message", singleLine = false, minLines = 2)
                Field(reason, { reason = it }, "Reason (required, kept with the history)", singleLine = false)
            }
        },
        confirmButton = {
            PrimaryButton(confirm, enabled = reason.isNotBlank() && (!needsText || text.isNotBlank()), onClick = {
                tone.guard((if (needsText) text + "\n" else "") + reason) {
                    onConfirm(text.trim(), reason.trim())
                }
            })
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun HistoryDialog(m: Message, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = pp.panel,
        title = { Text("Message history") },
        text = {
            LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp), modifier = Modifier.heightIn(max = 440.dp)) {
                item {
                    Text("Every version is kept, signed by its author and recorded in the audit log. Nothing can be removed.",
                        color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                }
                items(m.history) { r ->
                    Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small)).background(pp.bg2).padding(10.dp)) {
                        Text(if (r.rev == 0L) "Original" else if (r.retracted) "Withdrawn" else "Edit ${r.rev}",
                            color = pp.accent, style = MaterialTheme.typography.labelLarge)
                        Text(fullTime(r.time), color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
                        if (!r.retracted) Text(r.text, color = pp.fg, modifier = Modifier.padding(top = 4.dp))
                        if (r.reason.isNotBlank()) Text("Reason: ${r.reason}", color = pp.warm, style = MaterialTheme.typography.bodySmall,
                            fontWeight = FontWeight.Medium, modifier = Modifier.padding(top = 4.dp))
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Close") } },
    )
}

@Composable
private fun ImageDialog(name: String, img: ImageBitmap, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = pp.panel,
        title = { Text(name, maxLines = 1) },
        text = { Image(img, name, contentScale = ContentScale.Fit, modifier = Modifier.fillMaxWidth().heightIn(max = 600.dp)) },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Close") } },
    )
}

/** A finished call in the conversation: who called, voice or video, how long (or missed). */
@Composable
private fun CallRow(m: Message, c: systems.darkcipher.parallelpath.model.CallInfo) {
    val mins = c.seconds / 60
    val length = if (c.seconds >= 3600) "${c.seconds / 3600}:${(mins % 60).toString().padStart(2, '0')}:${(c.seconds % 60).toString().padStart(2, '0')}"
        else "$mins:${(c.seconds % 60).toString().padStart(2, '0')}"
    val what = (if (c.video) "Video call" else "Voice call") + " from ${if (m.mine) "you" else m.authorName}"
    androidx.compose.foundation.layout.Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        Row(Modifier.clip(androidx.compose.foundation.shape.RoundedCornerShape(999.dp)).background(pp.panel)
            .padding(horizontal = 14.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(if (c.video) Icons.Outlined.Videocam else Icons.Outlined.Call, null,
                tint = if (c.missed) pp.danger else pp.accent, modifier = Modifier.size(16.dp))
            Text("  $what · " + (if (c.missed) "missed" else length) + " · ${shortTime(m.sent)}", color = pp.fgDim,
                style = MaterialTheme.typography.labelMedium)
        }
    }
}
