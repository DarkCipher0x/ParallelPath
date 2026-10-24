package systems.darkcipher.parallelpath.calendar

import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import systems.darkcipher.parallelpath.model.CalendarView
import systems.darkcipher.parallelpath.model.Proposal
import systems.darkcipher.parallelpath.model.ScheduleTemplate
import systems.darkcipher.parallelpath.ui.Avatar
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.shortTime

/** Schedule proposals and change requests: the ones waiting on you first, then everything else, newest first. */
@Composable
internal fun RequestsPanel(v: CalendarView, templates: List<ScheduleTemplate>, canEdit: Boolean, onDialog: (Dialog) -> Unit) {
    val all = (v.schedules + v.changes).sortedWith(compareByDescending<Proposal> { it.needsMe }
        .thenByDescending { it.state == "pending" }.thenByDescending { it.updated.ifBlank { it.created } })
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 760.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (all.isEmpty()) {
                EmptyState(Icons.Outlined.Inbox, "No requests yet",
                    "Ask for a change from any day on the calendar, or propose a parenting schedule from Schedule. The other " +
                        "parent agrees or declines, and both of you keep the whole record.")
            }
            all.forEach { ProposalCard(it, v, templates, canEdit, onDialog) }
        }
    }
}

/** The agreed parenting-time schedule, and any proposal for a new one. */
@Composable
internal fun SchedulePanel(v: CalendarView, templates: List<ScheduleTemplate>, canEdit: Boolean, onDialog: (Dialog) -> Unit) {
    val agreed = v.schedules.filter { it.state == "agreed" }.maxByOrNull { it.agreed }
    val open = v.schedules.filter { it.state == "pending" }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 760.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Panel(Modifier.fillMaxWidth()) {
                SectionTitle("Parenting schedule")
                if (agreed == null) {
                    Text("There's no agreed schedule yet. Propose one - it only goes on the calendar once the other parent agrees.",
                        color = pp.fgDim)
                } else {
                    Text(scheduleTitle(agreed, v, templates), color = pp.fg, style = MaterialTheme.typography.titleMedium)
                    Text("Agreed ${shortTime(agreed.agreed)}. Changes to single days go through change requests; a new schedule " +
                        "replaces this one from its start date once it's agreed.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                }
                if (v.parents.size < 2) Banner("A schedule shares time between two parents. Invite the other parent from Family first.", Tone.Info)
                if (canEdit && v.parents.size >= 2) {
                    PrimaryButton(if (agreed == null) "Propose a schedule" else "Propose a new schedule", onClick = { onDialog(Dialog.Schedule(null)) })
                }
            }
            open.forEach { ProposalCard(it, v, templates, canEdit, onDialog) }
            if (templates.isNotEmpty()) {
                Panel(Modifier.fillMaxWidth()) {
                    SectionTitle("Common schedules")
                    templates.forEach { t ->
                        Column {
                            Text(t.name, color = pp.fg, fontWeight = FontWeight.SemiBold)
                            Text(t.description, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ProposalCard(p: Proposal, v: CalendarView, templates: List<ScheduleTemplate>, canEdit: Boolean, onDialog: (Dialog) -> Unit) {
    val (stateText, stateColor) = when (p.state) {
        "agreed" -> "Agreed" to pp.ok
        "declined" -> "Declined" to pp.danger
        "withdrawn" -> "Withdrawn" to pp.fgMute
        else -> (if (p.needsMe) "Waiting for you" else "Waiting for ${p.waiting.joinToString().ifBlank { "an answer" }}") to pp.warm
    }
    Panel(Modifier.fillMaxWidth().let { if (p.needsMe) it.border(1.dp, pp.warm, androidx.compose.foundation.shape.RoundedCornerShape(16.dp)) else it }) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Avatar(p.authorName, p.author, 32)
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                Text(if (p.kind == "schedule") "Schedule proposal" else "Change request", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                Text("${if (p.mine) "You" else p.authorName} · ${shortTime(p.updated.ifBlank { p.created })}" +
                    if (p.rev > 0) " · changed ${p.rev}×" else "", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            }
            Text(stateText, Modifier.clip(CircleShape).background(stateColor.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 4.dp),
                color = stateColor, style = MaterialTheme.typography.labelMedium)
        }
        if (p.kind == "schedule") {
            Text(scheduleTitle(p, v, templates), color = pp.fg, style = MaterialTheme.typography.titleMedium)
        } else {
            p.changes.forEach { c ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    androidx.compose.foundation.layout.Box(Modifier.size(10.dp).clip(CircleShape).background(parentColor(v.parents, c.parent)))
                    Text("  With ${nameOf(v.parents, c.parent)} from ${whenText(c.from)} to ${whenText(c.to)}", color = pp.fg,
                        style = MaterialTheme.typography.titleSmall)
                }
            }
        }
        if (p.note.isNotBlank()) Text("“${p.note}”", color = pp.fg)
        if (p.reason.isNotBlank() && p.rev > 0) Text("Changed because: ${p.reason}", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        if (p.problem.isNotBlank()) Banner(capital(p.problem), Tone.Warn)
        p.answers.forEach { a ->
            val agreed = a.answer == "accept"
            Text("${if (v.parents.firstOrNull { it.id == a.person }?.me == true) "You" else a.name} ${if (agreed) "agreed" else "declined"}" +
                (if (a.time.isNotBlank()) " ${shortTime(a.time)}" else "") + if (a.comment.isNotBlank()) ": “${a.comment}”" else "",
                color = if (agreed) pp.ok else pp.danger, style = MaterialTheme.typography.bodySmall)
        }
        FlowRowButtons {
            if (p.needsMe && canEdit) {
                PrimaryButton("Agree", onClick = { onDialog(Dialog.Answer(p, true)) })
                SecondaryButton("Decline…", danger = true, onClick = { onDialog(Dialog.Answer(p, false)) })
            }
            if (p.mine && canEdit && p.state == "pending") {
                if (p.kind == "schedule") SecondaryButton("Change…", onClick = { onDialog(Dialog.Schedule(p)) })
                TextButton(onClick = { onDialog(Dialog.Withdraw(p)) }) { Text("Withdraw", color = pp.danger) }
            }
            if (p.history.size > 1) TextButton(onClick = { onDialog(Dialog.History(p.history, "History of this ${if (p.kind == "schedule") "proposal" else "request"}")) }) {
                Text("History (${p.history.size})", color = pp.accent)
            }
        }
    }
}

@Composable
internal fun scheduleTitle(p: Proposal, v: CalendarView, templates: List<ScheduleTemplate>): String {
    val s = p.schedule ?: return "A schedule"
    val name = templates.firstOrNull { it.id == s.template }?.name ?: s.template.ifBlank { "Custom schedule" }
    val first = s.pattern.cycle.firstOrNull()?.parent
    val from = parseDate(s.from.take(10).ifBlank { s.pattern.start.take(10) })
    return buildString {
        append(name)
        if (from != null) append(" from ${from.short()} ${from.year}")
        if (s.pattern.exchange.isNotBlank()) append(", handovers at ${s.pattern.exchange}")
        if (first != null) append(", starting with ${nameOf(v.parents, first)}")
    }
}
