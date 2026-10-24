package systems.darkcipher.parallelpath.chat

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
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
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Contact
import systems.darkcipher.parallelpath.model.Conversation
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.pp

/**
 * Messages: the family chat and this person's private conversations in the family (each with keys only its people
 * hold). [onCall] starts a call in a conversation (video or voice).
 */
@Composable
fun MessagesScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit, onCall: ((String, Boolean) -> Unit)? = null) {
    var convs by remember { mutableStateOf<List<Conversation>>(emptyList()) }
    var contacts by remember { mutableStateOf<List<Contact>>(emptyList()) }
    val seesFamilyChat = space.isParent || space.members.firstOrNull { it.me }?.perms?.see?.contains("messages") != false
    var open by remember(space.id) { mutableStateOf<String?>(null) } // null = the family chat
    var picking by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(space.id, tick) {
        convs = try {
            engine.conversations(space.id)
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            convs
        }
        contacts = try {
            engine.contacts(space.id)
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            contacts
        }
        if (open == null && !seesFamilyChat) open = convs.firstOrNull()?.id
    }
    val canStart = contacts.any { it.message }
    Column(Modifier.fillMaxSize()) {
        if (convs.isNotEmpty() || canStart) {
            Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp, vertical = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                val chip = FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft, selectedLabelColor = pp.accent)
                if (seesFamilyChat) FilterChip(open == null, { open = null }, label = { Text("Family chat") }, colors = chip)
                convs.forEach { c ->
                    FilterChip(open == c.id, { open = c.id }, colors = chip,
                        leadingIcon = { Icon(Icons.Outlined.Lock, null, tint = pp.fgMute) },
                        label = { Text(c.title + if (c.unread > 0) " (${c.unread})" else "") })
                }
                if (canStart) TextButton(onClick = { picking = true }) {
                    Icon(Icons.Outlined.Add, null, tint = pp.accent)
                    Text("New private conversation", color = pp.accent)
                }
            }
        }
        val c = convs.firstOrNull { it.id == open }
        when {
            c != null -> {
                Row(Modifier.fillMaxWidth().padding(horizontal = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("Private: only ${c.title} and you can read this - not the rest of the family, and not the server.",
                        color = pp.fgDim, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                    if (onCall != null && c.canCall && !c.waiting) {
                        TextButton(onClick = { onCall(c.id, false) }) { Text("Voice call") }
                        if (c.video) TextButton(onClick = { onCall(c.id, true) }) { Text("Video call") }
                    }
                }
                ChatScreen(engine, conversationSpace(space, c), tick, onError, family = space.id)
            }
            seesFamilyChat -> ChatScreen(engine, space, tick, onError)
            else -> EmptyState(Icons.Outlined.Lock, "No conversations yet",
                if (canStart) "Start a private conversation with someone you're allowed to message." else "Nobody you may message yet.",
                Modifier.fillMaxSize())
        }
    }
    if (picking) PeoplePicker("New private conversation", "Only the people you choose (and you) can read it.",
        contacts.filter { it.message }, "Start", onDismiss = { picking = false }) { people ->
        picking = false
        scope.launch {
            try {
                open = engine.startConversation(space.id, people)
                convs = engine.conversations(space.id)
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                onError(e.message ?: "couldn't start the conversation")
            }
        }
    }
}

/** A conversation shown with the chat screen (which works on any space). */
private fun conversationSpace(family: Space, c: Conversation) = Space(
    id = c.id, name = c.title, myRole = "participant", readOnly = !c.canWrite, waiting = c.waiting,
    members = family.members.filter { m -> m.me || c.with.any { it.person == m.person } }, canChat = c.canWrite,
)

/** Chooses one or more people (for a conversation or a call). */
@Composable
fun PeoplePicker(title: String, explain: String, people: List<Contact>, confirm: String, onDismiss: () -> Unit, onPick: (List<String>) -> Unit) {
    var chosen by remember { mutableStateOf(setOf<String>()) }
    AlertDialog(
        onDismissRequest = onDismiss, containerColor = pp.panel,
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(explain, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                if (people.isEmpty()) Text("Nobody - the family's permissions don't allow it.", color = pp.fgMute)
                people.forEach { p ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(p.person in chosen, { on -> chosen = if (on) chosen + p.person else chosen - p.person },
                            colors = CheckboxDefaults.colors(checkedColor = pp.accent))
                        Text(p.name, color = pp.fg)
                    }
                }
            }
        },
        confirmButton = { PrimaryButton(confirm, enabled = chosen.isNotEmpty(), onClick = { onPick(chosen.toList()) }) },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
