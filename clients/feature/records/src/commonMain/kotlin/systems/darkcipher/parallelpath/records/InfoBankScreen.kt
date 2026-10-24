package systems.darkcipher.parallelpath.records

import androidx.compose.foundation.background
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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AttachFile
import androidx.compose.material.icons.outlined.ChildCare
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.History
import systems.darkcipher.parallelpath.ui.ContextMenuArea
import systems.darkcipher.parallelpath.ui.MenuDivider
import systems.darkcipher.parallelpath.ui.MenuItem
import systems.darkcipher.parallelpath.ui.menu
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
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Attachment
import systems.darkcipher.parallelpath.model.InfoEntry
import systems.darkcipher.parallelpath.model.InfoView
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Choice
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.prettyDate
import systems.darkcipher.parallelpath.ui.shortTime

internal val sections = listOf(
    "medical" to "Medical", "school" to "School", "contacts" to "Contacts", "clothing" to "Clothing and sizes",
    "routines" to "Routines", "documents" to "Documents and IDs", "other" to "Other",
)

internal fun sectionLabel(s: String) = sections.firstOrNull { it.first == s }?.second ?: "Other"

/** Label ideas per section (tap to fill in). */
internal val suggestions = mapOf(
    "medical" to listOf("GP", "Dentist", "Allergies", "Medication", "Blood type", "NHS number", "Vaccinations"),
    "school" to listOf("School", "Class teacher", "Year", "School phone", "Pick-up time", "After-school club"),
    "contacts" to listOf("Grandparent", "Childminder", "Emergency contact", "Best friend's parent"),
    "clothing" to listOf("Shoe size", "Clothes size", "Coat size", "School uniform"),
    "routines" to listOf("Bedtime", "Screen time", "Favourite foods", "Comforter"),
    "documents" to listOf("Passport", "Passport expiry", "Birth certificate", "Insurance"),
    "other" to listOf("Hobbies", "Pets"),
)

/**
 * The info bank: the children, and facts about each that both parents need (doctor, school, allergies, sizes...).
 * Either parent can update an entry; the reason and every earlier version are kept.
 */
@Composable
fun InfoBankScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit) {
    var view by remember { mutableStateOf<InfoView?>(null) }
    var child by remember { mutableStateOf<String?>(null) } // a child's chain, "" = the whole family, null = everyone
    var showRemoved by remember { mutableStateOf(false) }
    var dialog by remember { mutableStateOf<InfoDialog?>(null) }
    var busy by remember { mutableStateOf(false) }
    var reload by remember { mutableIntStateOf(0) }
    val scope = rememberCoroutineScope()
    val platform = LocalPlatform.current
    val canEdit = space.isParent && !space.waiting && !space.readOnly

    LaunchedEffect(space.id, tick, reload) {
        view = runCatching { engine.infoBank(space.id) }
            .onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't read the info bank") }.getOrNull() ?: view
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
    val open: (InfoEntry, Attachment) -> Unit = { en, a ->
        scope.launch {
            runCatching { platform.openFile(engine.openAttachment(space.id, en.id, a.blobId), a.mime) }
                .onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't open the file") }
        }
    }

    val v = view ?: return
    val childName = { id: String -> v.children.firstOrNull { it.chain == id }?.label ?: if (id.isEmpty()) "Whole family" else "A removed child" }
    val entries = v.entries.filter { child == null || it.child == child }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 820.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (space.waiting) Banner("The info bank opens once a parent approves you.", Tone.Warn)
            Panel(Modifier.fillMaxWidth()) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    SectionTitle("Children", Modifier.weight(1f))
                    if (canEdit) TextButton(onClick = { dialog = InfoDialog.Child(null) }) { Text("Add a child", color = pp.accent) }
                }
                if (v.children.isEmpty()) Text("Add each child, then the details both of you need about them.", color = pp.fgMute)
                FlowButtons {
                    Choice(child == null, "Everything", { child = null })
                    v.children.forEach { c -> Choice(child == c.chain, c.label, { child = c.chain }) }
                    Choice(child == "", "Whole family", { child = "" })
                }
                v.children.firstOrNull { it.chain == child }?.let { c ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Outlined.ChildCare, null, tint = pp.accent, modifier = Modifier.size(20.dp))
                        Text("  ${c.label}" + if (c.value.isNotBlank()) " · born ${prettyDate(c.value)}" else "", color = pp.fg, modifier = Modifier.weight(1f))
                        if (canEdit) {
                            TextButton(onClick = { dialog = InfoDialog.Child(c) }) { Text("Update", color = pp.accent) }
                            TextButton(onClick = { dialog = InfoDialog.Remove(c) }) { Text("Remove", color = pp.danger) }
                        }
                    }
                }
                if (canEdit) PrimaryButton("Add a detail", onClick = { dialog = InfoDialog.Entry(null, child ?: v.children.firstOrNull()?.chain ?: "", null) })
            }
            if (entries.isEmpty() && v.children.isNotEmpty()) {
                EmptyState(Icons.Outlined.ChildCare, "Nothing here yet", "Add the GP, school, allergies, sizes - anything the other parent " +
                    "should be able to look up. Either of you can update it, and every change is kept.")
            }
            sections.forEach { (id, label) ->
                val list = entries.filter { it.section == id }
                if (list.isNotEmpty()) Panel(Modifier.fillMaxWidth()) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        SectionTitle(label, Modifier.weight(1f))
                        if (canEdit) TextButton(onClick = { dialog = InfoDialog.Entry(null, child ?: list.first().child, id) }) { Text("Add", color = pp.accent) }
                    }
                    list.forEach { en -> EntryRow(en, if (child == null) childName(en.child) else null, canEdit, { dialog = it }) { a -> open(en, a) } }
                }
            }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Checkbox(showRemoved, { showRemoved = it }, colors = CheckboxDefaults.colors(checkedColor = pp.accent, checkmarkColor = pp.accentInk))
                Text("Show removed (${v.removed.size}, kept for the record)", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            }
            if (showRemoved && v.removed.isNotEmpty()) Panel(Modifier.fillMaxWidth()) {
                SectionTitle("Removed")
                v.removed.forEach { en ->
                    Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small)).clickable { dialog = InfoDialog.History(en) }.padding(vertical = 4.dp)) {
                        Text("${en.label}: ${en.value}", color = pp.fgMute)
                        Text("Removed by ${en.byName} ${shortTime(en.updated)}: ${en.history.lastOrNull()?.why ?: ""}", color = pp.fgMute,
                            style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
    dialog?.let { d -> InfoDialogs(d, v, busy, engine, space.id, act) { dialog = null } }
}

@Composable
private fun EntryRow(en: InfoEntry, childName: String?, canEdit: Boolean, onDialog: (InfoDialog) -> Unit, onOpen: (Attachment) -> Unit) {
    val platform = LocalPlatform.current
    ContextMenuArea({
        menu(
            MenuItem("Copy ${en.label.lowercase()}", Icons.Outlined.ContentCopy, enabled = en.value.isNotBlank()) { platform.copyText(en.value) },
            MenuItem("Copy with label", Icons.Outlined.ContentCopy) { platform.copyText("${en.label}${childName?.let { " ($it)" } ?: ""}: ${en.value}") },
            MenuDivider,
            *en.attachments.map { a -> MenuItem("Open ${a.name}", Icons.Outlined.AttachFile) { onOpen(a) } }.toTypedArray(),
            MenuDivider,
            if (canEdit) MenuItem("Update…", Icons.Outlined.Edit) { onDialog(InfoDialog.Entry(en, en.child, en.section)) } else null,
            if (en.history.size > 1) MenuItem("History (${en.history.size})", Icons.Outlined.History) { onDialog(InfoDialog.History(en)) } else null,
            MenuDivider,
            if (canEdit) MenuItem("Remove…", Icons.Outlined.Delete, danger = true) { onDialog(InfoDialog.Remove(en)) } else null,
        )
    }, Modifier.fillMaxWidth()) {
    Column(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Row(verticalAlignment = Alignment.Top) {
            Column(Modifier.weight(1f)) {
                Text(en.label + if (childName != null) " · $childName" else "", color = pp.fgDim, style = MaterialTheme.typography.labelLarge)
                SelectionContainer { Text(en.value.ifBlank { "-" }, color = pp.fg, style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.Medium) }
                if (en.note.isNotBlank()) Text(en.note, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            }
            if (canEdit) {
                TextButton(onClick = { onDialog(InfoDialog.Entry(en, en.child, en.section)) }) { Text("Update", color = pp.accent) }
            }
        }
        if (en.conflict) Banner("Both parents updated this at the same time, so both versions show. Update it to settle on one.", Tone.Warn)
        if (en.attachments.isNotEmpty()) FlowButtons {
            en.attachments.forEach { a ->
                Row(Modifier.clip(RoundedCornerShape(Radius.small)).background(pp.bg2).clickable { onOpen(a) }.padding(horizontal = 10.dp, vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.AttachFile, null, tint = pp.accent, modifier = Modifier.size(16.dp))
                    Text(" ${a.name}", color = pp.fg, style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("${if (en.history.size > 1) "Updated" else "Added"} by ${if (en.mine) "you" else en.byName} ${shortTime(en.updated)}",
                color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
            Spacer(Modifier.width(8.dp))
            if (en.history.size > 1) Text("History (${en.history.size})", Modifier.clip(RoundedCornerShape(Radius.small))
                .clickable { onDialog(InfoDialog.History(en)) }.padding(4.dp), color = pp.accent, style = MaterialTheme.typography.bodySmall)
            if (canEdit) Text("Remove", Modifier.clip(RoundedCornerShape(Radius.small)).clickable { onDialog(InfoDialog.Remove(en)) }.padding(4.dp),
                color = pp.danger, style = MaterialTheme.typography.bodySmall)
        }
    }
    }
}
