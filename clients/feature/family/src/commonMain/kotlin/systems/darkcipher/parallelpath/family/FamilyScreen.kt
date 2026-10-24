package systems.darkcipher.parallelpath.family

import systems.darkcipher.parallelpath.model.Perms
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.PersonRemove
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material.icons.outlined.VerifiedUser
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.InviteCode
import systems.darkcipher.parallelpath.model.Member
import systems.darkcipher.parallelpath.model.Roles
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Avatar
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.Mono
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.isoDaysFromNow
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.shortTime

/** Members, invitations and approvals for one family space. */
@Composable
fun FamilyScreen(engine: Engine, space: Space, onChanged: () -> Unit, onError: (String) -> Unit) {
    var approving by remember { mutableStateOf<Pair<Member, String>?>(null) }
    var removing by remember { mutableStateOf<Member?>(null) }
    var invite by remember { mutableStateOf<InviteCode?>(null) }
    var myNumberFor by remember { mutableStateOf<Pair<Member, String>?>(null) }
    var permsFor by remember { mutableStateOf<Member?>(null) }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    fun act(block: suspend () -> Unit) = scope.launch {
        busy = true
        try {
            block()
            onChanged()
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e // a screen closing or reloading isn't an error
        } catch (e: Exception) {
            onError(e.message ?: "something went wrong")
        } finally {
            busy = false
        }
    }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Column(Modifier.widthIn(max = 760.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            if (space.waiting) {
                Banner(
                    "Waiting for a parent to approve you. When they do, they'll see a 60-digit safety number - compare it with " +
                        "yours below on a call or in person. If it matches, nobody is impersonating either of you.",
                    Tone.Warn,
                )
            }
            Panel {
                SectionTitle("People in ${space.name}")
                space.members.forEach { m ->
                    val platform = systems.darkcipher.parallelpath.ui.LocalPlatform.current
                    val active = !m.me && m.status == "active"
                    systems.darkcipher.parallelpath.ui.ContextMenuArea({
                        systems.darkcipher.parallelpath.ui.menu(
                            if (active && m.needsApproval && space.isParent) systems.darkcipher.parallelpath.ui.MenuItem("Approve…",
                                Icons.Outlined.VerifiedUser) { act { approving = m to engine.safetyNumber(m.person) } } else null,
                            if (active && !(m.needsApproval && space.isParent)) systems.darkcipher.parallelpath.ui.MenuItem("Safety number",
                                Icons.Outlined.VerifiedUser) { act { myNumberFor = m to engine.safetyNumber(m.person) } } else null,
                            if (active && m.role != Roles.PARENT) systems.darkcipher.parallelpath.ui.MenuItem("Permissions…", Icons.Outlined.Tune) { permsFor = m } else null,
                            systems.darkcipher.parallelpath.ui.MenuItem("Copy name", Icons.Outlined.ContentCopy) { platform.copyText(m.name) },
                            systems.darkcipher.parallelpath.ui.MenuDivider,
                            if (active && (space.isParent && m.role != Roles.PARENT || space.isParent && m.needsApproval))
                                systems.darkcipher.parallelpath.ui.MenuItem("Remove from the family…", Icons.Outlined.PersonRemove, danger = true) { removing = m } else null,
                        )
                    }, Modifier.fillMaxWidth()) {
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                        Avatar(m.name, m.person)
                        Spacer(Modifier.width(12.dp))
                        Column(Modifier.weight(1f)) {
                            Text(m.name + if (m.me) " (you)" else "", color = pp.fg, style = MaterialTheme.typography.titleMedium)
                            val detail = buildString {
                                append(Roles.label(m.role))
                                if (m.status == "removed") append(" · removed")
                                if (m.needsApproval && m.status == "active") append(" · waiting for approval")
                                m.notAfter?.let { na -> if (!na.startsWith("0001")) append(" · access until ${shortTime(na)}") }
                                m.historyFrom?.let { hf -> if (!hf.startsWith("0001")) append(" · sees history from ${shortTime(hf)}") }
                                if (m.joined.isNotBlank()) append(" · joined ${shortTime(m.joined)}")
                            }
                            Text(detail, color = if (m.needsApproval && m.status == "active") pp.warm else pp.fgDim, style = MaterialTheme.typography.bodySmall)
                            if (m.role != Roles.PARENT && m.status == "active") Text(permsSummary(m.perms), color = pp.fgMute,
                                style = MaterialTheme.typography.bodySmall)
                        }
                        if (!m.me && m.status == "active") {
                            if (m.needsApproval && space.isParent) {
                                PrimaryButton("Approve…", busy = busy, onClick = {
                                    act { approving = m to engine.safetyNumber(m.person) }
                                })
                            } else {
                                TextButton(onClick = { act { myNumberFor = m to engine.safetyNumber(m.person) } }) { Text("Safety number") }
                            }
                            if (m.role != Roles.PARENT) TextButton(onClick = { permsFor = m }) { Text("Permissions…") }
                            if (space.isParent && m.role != Roles.PARENT || space.isParent && m.needsApproval) {
                                TextButton(onClick = { removing = m }) { Text("Remove", color = pp.danger) }
                            }
                        }
                    }
                    }
                }
            }

            if (space.isParent && !space.waiting) InvitePanel(busy, space.members.filter { !it.me }) { role, days, historyDays, perms ->
                act { invite = engine.invite(space.id, role, days?.let { isoDaysFromNow(it) }, historyDays?.let { isoDaysFromNow(-it) }, perms) }
            }

            if (space.isParent && !space.waiting && space.members.any { it.role == Roles.CHILD && it.status == "active" }) {
                Panel {
                    SectionTitle("Children")
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                        Column(Modifier.weight(1f)) {
                            Text("Children can send messages", color = pp.fg)
                            Text("Off: children can read but not write. On: they can send messages and reactions (nothing else). " +
                                "Either parent can change this; it's recorded in the audit log.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                        }
                        Switch(space.childrenChat, { on -> act { engine.setChildrenChat(space.id, on) } }, enabled = !busy)
                    }
                }
            }

            if (!space.waiting && !space.readOnly) NamesPanel(space, busy,
                onMyName = { n -> act { engine.setMyName(space.id, n) } },
                onFamilyName = { n -> act { engine.renameSpace(space.id, n) } })
        }
    }

    approving?.let { (m, number) ->
        AlertDialog(
            onDismissRequest = { approving = null }, containerColor = pp.panel,
            title = { Text("Approve ${m.name}?") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text("Call ${m.name} (or meet in person) and ask them to read the safety number shown on their Family screen. " +
                        "It must match exactly:", color = pp.fgDim)
                    SafetyNumber(number)
                    Text("If it matches, approving shares the family's keys with their devices so they can read the whole history " +
                        "and take part (${Roles.label(m.role)}). If it doesn't match, don't approve - someone may be impersonating them.",
                        color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                }
            },
            confirmButton = { PrimaryButton("It matches - approve", busy = busy, onClick = { approving = null; act { engine.approve(space.id, m.person) } }) },
            dismissButton = { TextButton(onClick = { approving = null }) { Text("Not now") } },
        )
    }
    myNumberFor?.let { (m, number) ->
        AlertDialog(
            onDismissRequest = { myNumberFor = null }, containerColor = pp.panel,
            title = { Text("Safety number with ${m.name}") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text("Read this to ${m.name}; it must match what their app shows for you.", color = pp.fgDim)
                    SafetyNumber(number)
                }
            },
            confirmButton = { TextButton(onClick = { myNumberFor = null }) { Text("Close") } },
        )
    }
    removing?.let { m ->
        AlertDialog(
            onDismissRequest = { removing = null }, containerColor = pp.panel,
            title = { Text("Remove ${m.name}?") },
            text = {
                Text("They lose access immediately and the family's encryption keys are rotated, so nothing new can be read " +
                    "with anything they still hold. Everything already shared stays in the record, and this removal is " +
                    "recorded in the audit log.", color = pp.fgDim)
            },
            confirmButton = { SecondaryButton("Remove", danger = true, onClick = { removing = null; act { engine.remove(space.id, m.person) } }) },
            dismissButton = { TextButton(onClick = { removing = null }) { Text("Cancel") } },
        )
    }
    invite?.let { code -> InviteDialog(code) { invite = null } }
    permsFor?.let { m ->
        val myId = space.members.firstOrNull { it.me }?.person
        val mine = space.isParent && (m.invitedBy.isBlank() || m.invitedBy == myId)
        var p by remember(m.person) { mutableStateOf(m.perms ?: Perms.defaults(m.role)) }
        AlertDialog(
            onDismissRequest = { permsFor = null }, containerColor = pp.panel,
            title = { Text("What ${m.name} can do") },
            text = {
                Column(Modifier.widthIn(max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text(if (mine) "Changes take effect at once and are recorded in the family's audit log."
                        else "Only the parent who invited ${m.name} can change these.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                    PermsEditor(m.role, p, space.members.filter { it.person != m.person }, enabled = mine) { p = it }
                }
            },
            confirmButton = {
                if (mine) PrimaryButton("Save", busy = busy, onClick = { permsFor = null; act { engine.setPermissions(space.id, m.person, p) } })
                else TextButton(onClick = { permsFor = null }) { Text("Close") }
            },
            dismissButton = { if (mine) TextButton(onClick = { permsFor = null }) { Text("Cancel") } },
        )
    }
}

@Composable
private fun SafetyNumber(number: String) {
    SelectionContainer {
        Text(number.chunked(18).joinToString("\n"), fontFamily = Mono, fontSize = 20.sp, color = pp.accent, lineHeight = 30.sp)
    }
}

@Composable
private fun InvitePanel(busy: Boolean, others: List<Member>, onInvite: (String, Int?, Int?, Perms?) -> Unit) {
    var role by remember { mutableStateOf(Roles.PARENT) }
    var perms by remember(role) { mutableStateOf(Perms.defaults(role)) }
    var showPerms by remember { mutableStateOf(false) }
    var days by remember { mutableStateOf(90) }
    var history by remember { mutableStateOf<Int?>(null) } // days back; null = the whole history
    Panel {
        SectionTitle("Invite someone")
        Text("Each invite is a one-time code that works for 7 days. The person sets up ParallelPath with it, then you approve them " +
            "after comparing safety numbers.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf(Roles.PARENT, Roles.FAMILY, Roles.PROFESSIONAL, Roles.CHILD).forEach { r ->
                FilterChip(selected = role == r, onClick = { role = r }, label = { Text(Roles.label(r).substringBefore(" (")) },
                    colors = FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft, selectedLabelColor = pp.accent))
            }
        }
        if (role == Roles.PROFESSIONAL) {
            Text("Professionals (mediators, solicitors, guardians) get read-only access that ends automatically.", color = pp.fgDim,
                style = MaterialTheme.typography.bodySmall)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("Access for", color = pp.fgDim)
                listOf(30, 90, 180, 365).forEach { d ->
                    FilterChip(selected = days == d, onClick = { days = d }, label = { Text("$d days") },
                        colors = FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft, selectedLabelColor = pp.accent))
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("They see", color = pp.fgDim)
                listOf<Pair<Int?, String>>(null to "All history", 365 to "Last year", 90 to "Last 3 months", 0 to "From today").forEach { (h, label) ->
                    FilterChip(selected = history == h, onClick = { history = h }, label = { Text(label) },
                        colors = FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft, selectedLabelColor = pp.accent))
                }
            }
            Text("Older messages and records stay hidden from them, and their devices never get the keys for them.", color = pp.fgDim,
                style = MaterialTheme.typography.bodySmall)
        }
        if (role != Roles.PARENT) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.weight(1f)) {
                    Text("What they can do", color = pp.fg)
                    Text(permsSummary(perms), color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                }
                TextButton(onClick = { showPerms = !showPerms }) { Text(if (showPerms) "Hide" else "Change…") }
            }
            if (showPerms) PermsEditor(role, perms, others, enabled = true) { perms = it }
        }
        PrimaryButton("Create invite code", busy = busy, onClick = {
            val pro = role == Roles.PROFESSIONAL
            onInvite(role, if (pro) days else null, if (pro) history else null, if (role == Roles.PARENT) null else perms)
        })
    }
}

@Composable
private fun InviteDialog(code: InviteCode, onDismiss: () -> Unit) {
    val platform = LocalPlatform.current
    AlertDialog(
        onDismissRequest = onDismiss, containerColor = pp.panel,
        title = { Text("Invite code") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("Send this to them privately (it works once, until ${fullTime(code.expires)}). They choose \"I have an invitation\" " +
                    "when they set up ParallelPath, with this server's address.", color = pp.fgDim)
                SelectionContainer { Text(code.code, fontFamily = Mono, color = pp.fg, style = MaterialTheme.typography.bodySmall) }
            }
        },
        confirmButton = { PrimaryButton("Copy code", onClick = { platform.copyText(code.code) }) },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Done") } },
    )
}

@Composable
private fun NamesPanel(space: Space, busy: Boolean, onMyName: (String) -> Unit, onFamilyName: (String) -> Unit) {
    val me = space.members.firstOrNull { it.me }
    var myName by remember(me?.name) { mutableStateOf(me?.name ?: "") }
    var family by remember(space.name) { mutableStateOf(space.name) }
    Panel {
        SectionTitle("Names")
        Text("Names are stored encrypted inside the family space - the server never sees them. Changes are kept in the history.",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Field(myName, { myName = it }, "Your name", modifier = Modifier.weight(1f))
            SecondaryButton("Save", enabled = !busy && myName.isNotBlank() && myName != me?.name, onClick = { onMyName(myName) })
        }
        if (space.isParent) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Field(family, { family = it }, "Family name", modifier = Modifier.weight(1f))
                SecondaryButton("Save", enabled = !busy && family.isNotBlank() && family != space.name, onClick = { onFamilyName(family) })
            }
        }
    }
}
