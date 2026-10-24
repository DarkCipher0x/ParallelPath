package systems.darkcipher.parallelpath.calendar

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
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
import kotlinx.datetime.LocalTime
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.CalEvent
import systems.darkcipher.parallelpath.model.CalendarView
import systems.darkcipher.parallelpath.model.Change
import systems.darkcipher.parallelpath.model.EventData
import systems.darkcipher.parallelpath.model.Proposal
import systems.darkcipher.parallelpath.model.Repeat
import systems.darkcipher.parallelpath.model.Revision
import systems.darkcipher.parallelpath.model.ScheduleTemplate
import systems.darkcipher.parallelpath.model.Segment
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.FormDialog
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.shortTime

/** What the calendar is asking the person, if anything. */
internal sealed interface Dialog {
    data class Event(val date: LocalDate, val edit: CalEvent?) : Dialog
    data class Details(val event: CalEvent) : Dialog
    data class CancelEvent(val event: CalEvent) : Dialog
    data class Change(val date: LocalDate) : Dialog
    data class Schedule(val edit: Proposal?) : Dialog
    data class Answer(val p: Proposal, val accept: Boolean) : Dialog
    data class Withdraw(val p: Proposal) : Dialog
    data class History(val history: List<Revision>, val title: String) : Dialog
    data object CheckIn : Dialog
}

@Composable
internal fun Dialogs(
    d: Dialog, v: CalendarView, templates: List<ScheduleTemplate>, busy: Boolean, engine: Engine, space: String,
    act: (suspend () -> Unit) -> Unit, onDialog: (Dialog) -> Unit, onClose: () -> Unit,
) {
    when (d) {
        is Dialog.Event -> EventEditor(d.date, d.edit, busy, onClose) { data, notes, reason ->
            act { engine.saveEvent(space, data, notes, d.edit?.id, reason) }
        }
        is Dialog.Details -> EventDetails(d.event, onClose, onDialog)
        is Dialog.CancelEvent -> ReasonDialog("Cancel ${d.event.data.title}?",
            "It stays on the calendar, crossed out, with your reason. ${if (d.event.data.repeat.every.isNotBlank()) "This cancels every occurrence. " else ""}" +
                "The other parent sees the change and it's kept in the history.", "Cancel event", busy, onClose) { r ->
            act { engine.cancelEvent(space, d.event.id, r) }
        }
        is Dialog.Change -> ChangeDialog(d.date, v, busy, onClose) { changes, note -> act { engine.requestChange(space, changes, note) } }
        is Dialog.Schedule -> ScheduleEditor(d.edit, v, templates, busy, onClose) { t, a, b, start, exchange, note, reason, cycle ->
            act { engine.proposeSchedule(space, t, a, b, start.toString(), exchange, zone.id, note, d.edit?.id, reason, cycle) }
        }
        is Dialog.Answer -> AnswerDialog(d.p, d.accept, busy, onClose) { comment -> act { engine.answer(space, d.p.id, d.accept, comment) } }
        is Dialog.Withdraw -> ReasonDialog("Withdraw this ${if (d.p.kind == "schedule") "proposal" else "request"}?",
            "It can't be answered after this. It stays in the record with your reason.", "Withdraw", busy, onClose) { r ->
            act { engine.withdraw(space, d.p.id, r) }
        }
        is Dialog.History -> HistoryDialog(d.title, d.history, onClose)
        Dialog.CheckIn -> CheckInDialog(busy, onClose) { place, note, fix ->
            act { engine.checkIn(space, place, note, fix?.lat, fix?.lon, fix?.accuracy ?: 0.0) }
        }
    }
}

@Composable
private fun Form(
    title: String, confirm: String, enabled: Boolean, busy: Boolean, onClose: () -> Unit, onConfirm: () -> Unit,
    danger: Boolean = false, toneText: String? = null, content: @Composable () -> Unit,
) = FormDialog(title, confirm, enabled, busy, onClose, onConfirm, danger, toneText, content)

@Composable
private fun EventEditor(
    date: LocalDate, edit: CalEvent?, busy: Boolean, onClose: () -> Unit, onSave: (EventData, String, String?) -> Unit,
) {
    val base = edit?.data
    val startAt = base?.let { local(it.start) }
    val endAt = base?.let { local(it.end) }
    var title by remember { mutableStateOf(base?.title ?: "") }
    var category by remember { mutableStateOf(base?.category?.ifBlank { "other" } ?: "activity") }
    var day by remember { mutableStateOf(startAt?.date ?: date) }
    var allDay by remember { mutableStateOf(base?.allDay ?: false) }
    var from by remember { mutableStateOf(startAt?.time?.hhmm() ?: "09:00") }
    var to by remember { mutableStateOf(endAt?.time?.hhmm() ?: "10:00") }
    var endDay by remember { mutableStateOf(endAt?.date?.let { if (base.allDay) it.addDays(-1) else it } ?: date) }
    var location by remember { mutableStateOf(base?.location ?: "") }
    var children by remember { mutableStateOf(base?.children?.joinToString() ?: "") }
    var every by remember { mutableStateOf(base?.repeat?.every ?: "") }
    var until by remember { mutableStateOf(base?.repeat?.until?.let { parseDate(it.take(10)) } ?: date.addMonths(3)) }
    var notes by remember { mutableStateOf(edit?.notes ?: "") }
    var reason by remember { mutableStateOf("") }

    val t1 = timeOrNull(from)
    val t2 = timeOrNull(to)
    val start = if (allDay) isoAt(day, LocalTime(0, 0)) else t1?.let { isoAt(day, it) }
    val end = if (allDay) isoAt(maxOf(endDay, day).addDays(1), LocalTime(0, 0)) else t2?.let { isoAt(maxOf(endDay, day), it) }
    val ok = title.isNotBlank() && start != null && end != null && (edit == null || reason.isNotBlank())

    Form(if (edit == null) "Add event" else "Edit event", if (edit == null) "Add" else "Save", ok, busy, onClose, onConfirm = {
        onSave(EventData(
            title = title.trim(), start = start!!, end = end!!, allDay = allDay, zone = zone.id, location = location.trim(),
            category = category, children = children.split(',').map { it.trim() }.filter { it.isNotEmpty() },
            repeat = Repeat(every, if (every.isBlank()) "" else until.toString()),
        ), notes.trim(), if (edit == null) null else reason.trim())
    }) {
        Field(title, { title = it }, "What is it?")
        FlowRowButtons { categories.forEach { (id, label) -> Choice(category == id, label, { category = id }, categoryColor(id)) } }
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Switch(allDay, { allDay = it }, colors = SwitchDefaults.colors(checkedTrackColor = pp.accent, checkedThumbColor = pp.accentInk))
            Text("All day", color = pp.fg)
        }
        FlowRowButtons {
            DateField(if (allDay) "From" else "Date", day, { day = it; if (endDay < it) endDay = it })
            if (allDay) DateField("To", maxOf(endDay, day), { endDay = it })
        }
        if (!allDay) {
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                TimeField("Starts", from, { from = it }, Modifier.weight(1f))
                TimeField("Ends", to, { to = it }, Modifier.weight(1f))
            }
            if (start != null && end != null && end < start) Text("It ends before it starts.", color = pp.danger, style = MaterialTheme.typography.bodySmall)
        }
        Field(location, { location = it }, "Where (optional)")
        Field(children, { children = it }, "Which children (optional, separate with commas)")
        Text("Repeats", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        FlowRowButtons {
            listOf("" to "Never", "daily" to "Daily", "weekly" to "Weekly", "fortnightly" to "Fortnightly", "monthly" to "Monthly",
                "yearly" to "Yearly").forEach { (id, label) -> Choice(every == id, label, { every = id }) }
        }
        if (every.isNotBlank()) DateField("Until", until, { until = it })
        Field(notes, { notes = it }, "Notes (optional)", singleLine = false, minLines = 2)
        if (edit != null) Field(reason, { reason = it }, "Why are you changing it?", supporting = "Required - kept with the event's history")
    }
}

@Composable
private fun EventDetails(e: CalEvent, onClose: () -> Unit, onNext: (Dialog) -> Unit) {
    AlertDialog(
        onDismissRequest = onClose, containerColor = pp.panel,
        title = { Text(e.data.title) },
        text = {
            Column(Modifier.widthIn(min = 300.dp, max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (e.cancelled) Banner("Cancelled${if (e.reason.isNotBlank()) ": ${e.reason}" else ""}", Tone.Warn)
                val s = local(e.start)
                Text(if (e.data.allDay) "${s?.date?.long() ?: ""} · all day" else "${whenText(e.start)} – ${timeOf(e.end)}", color = pp.fg)
                if (e.data.repeat.every.isNotBlank()) Text("Repeats ${e.data.repeat.every}" +
                    if (e.data.repeat.until.isNotBlank()) " until ${parseDate(e.data.repeat.until.take(10))?.short() ?: e.data.repeat.until}" else "", color = pp.fgDim)
                if (e.data.location.isNotBlank()) Text("Where: ${e.data.location}", color = pp.fgDim)
                if (e.data.children.isNotEmpty()) Text("For: ${e.data.children.joinToString()}", color = pp.fgDim)
                if (e.notes.isNotBlank()) Text(e.notes, color = pp.fg)
                Text("Added by ${if (e.mine) "you" else e.authorName}" + if (e.edited) " · edited" else "", color = pp.fgMute,
                    style = MaterialTheme.typography.bodySmall)
                if (e.history.size > 1) {
                    Text("History", color = pp.fgMute, style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.SemiBold)
                    e.history.forEach { RevisionRow(it) }
                }
            }
        },
        confirmButton = {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (e.mine && !e.cancelled) {
                    TextButton(onClick = { onNext(Dialog.CancelEvent(e)) }) { Text("Cancel event", color = pp.danger) }
                    SecondaryButton("Edit", onClick = { onNext(Dialog.Event(local(e.start)?.date ?: today(), e)) })
                }
                PrimaryButton("Close", onClick = onClose)
            }
        },
    )
}

@Composable
private fun RevisionRow(r: Revision) {
    Column {
        Text("${if (r.rev == 0L) "Added" else "Changed"} ${fullTime(r.time)}", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        if (r.reason.isNotBlank()) Text("Reason: ${r.reason}", color = pp.fg, style = MaterialTheme.typography.bodySmall)
        if (r.text.isNotBlank()) Text(r.text, color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
    }
}

@Composable
private fun HistoryDialog(title: String, history: List<Revision>, onClose: () -> Unit) {
    AlertDialog(
        onDismissRequest = onClose, containerColor = pp.panel, title = { Text(title) },
        text = {
            Column(Modifier.widthIn(min = 300.dp, max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("Every version is kept, encrypted and signed by the person who made it.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                history.forEach { RevisionRow(it) }
            }
        },
        confirmButton = { PrimaryButton("Close", onClick = onClose) },
    )
}

@Composable
private fun ReasonDialog(title: String, body: String, confirm: String, busy: Boolean, onClose: () -> Unit, onConfirm: (String) -> Unit) {
    var reason by remember { mutableStateOf("") }
    Form(title, confirm, reason.isNotBlank(), busy, onClose, { onConfirm(reason.trim()) }, danger = true, toneText = reason) {
        Text(body, color = pp.fgDim)
        Field(reason, { reason = it }, "Reason", supporting = "Required")
    }
}

@Composable
private fun ChangeDialog(date: LocalDate, v: CalendarView, busy: Boolean, onClose: () -> Unit, onSend: (List<Change>, String) -> Unit) {
    var added by remember { mutableStateOf<List<Change>>(emptyList()) } // earlier periods in the same request
    val me = v.parents.firstOrNull { it.me }
    var parent by remember { mutableStateOf(me?.id ?: v.parents.firstOrNull()?.id ?: "") }
    var fromDay by remember { mutableStateOf(date) }
    var toDay by remember { mutableStateOf(date.addDays(1)) }
    var fromTime by remember { mutableStateOf("10:00") }
    var toTime by remember { mutableStateOf("17:00") }
    var note by remember { mutableStateOf("") }
    val f = timeOrNull(fromTime)?.let { isoAt(fromDay, it) }
    val t = timeOrNull(toTime)?.let { isoAt(toDay, it) }
    val current = parent.isNotBlank() && f != null && t != null && t > f
    val ok = current && note.isNotBlank()

    Form("Ask for a change", "Send request", ok, busy, onClose, { onSend(added + Change(f!!, t!!, parent), note.trim()) }, toneText = note) {
        Text("Nothing changes until the other parent agrees. They can decline with a reason; either way it's kept in the record.",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        Text("Who has the children", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        FlowRowButtons {
            v.parents.forEach { p -> Choice(parent == p.id, if (p.me) "Me" else p.name, { parent = p.id }, parentColor(v.parents, p.id)) }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.Bottom) {
            DateField("From", fromDay, { fromDay = it; if (toDay < it) toDay = it })
            TimeField("Time", fromTime, { fromTime = it }, Modifier.weight(1f))
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.Bottom) {
            DateField("Until", toDay, { toDay = it })
            TimeField("Time", toTime, { toTime = it }, Modifier.weight(1f))
        }
        if (f != null && t != null && t <= f) Text("The end must be after the start.", color = pp.danger, style = MaterialTheme.typography.bodySmall)
        added.forEachIndexed { i, c ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("${i + 1}. ${v.parents.firstOrNull { it.id == c.parent }?.let { if (it.me) "Me" else it.name } ?: ""}: " +
                    "${shortTime(c.from)} - ${shortTime(c.to)}", color = pp.fg, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                TextButton(onClick = { added = added.toMutableList().also { it.removeAt(i) } }) { Text("Remove", color = pp.danger) }
            }
        }
        if (added.size < 9) TextButton(enabled = current, onClick = {
            added = added + Change(f!!, t!!, parent)
            fromDay = toDay.addDays(1); toDay = fromDay.addDays(1)
        }) { Text(if (added.isEmpty()) "Ask for another period in the same request" else "Add another period") }
        Field(note, { note = it }, "Why are you asking?", singleLine = false, minLines = 2, supporting = "Required - it helps the other parent decide")
    }
}

@Composable
private fun ScheduleEditor(
    edit: Proposal?, v: CalendarView, templates: List<ScheduleTemplate>, busy: Boolean, onClose: () -> Unit,
    onSend: (String, String, String, LocalDate, String, String, String?, List<Segment>) -> Unit,
) {
    val old = edit?.schedule
    // A custom cycle: whose turn and for how many days, repeating (e.g. 5 days with one parent, 2 with the other).
    var cycle by remember {
        mutableStateOf(old?.takeIf { it.template == "custom" }?.pattern?.cycle?.takeIf { it.isNotEmpty() }
            ?: v.parents.take(2).map { Segment(it.id, 7) })
    }
    var template by remember { mutableStateOf(old?.template ?: templates.firstOrNull()?.id ?: "alternating-weeks") }
    val me = v.parents.firstOrNull { it.me }
    var first by remember { mutableStateOf(old?.pattern?.cycle?.firstOrNull()?.parent ?: me?.id ?: v.parents.firstOrNull()?.id ?: "") }
    var start by remember { mutableStateOf(old?.pattern?.start?.let { parseDate(it.take(10)) } ?: today().addDays(7)) }
    var exchange by remember { mutableStateOf(old?.pattern?.exchange ?: "18:00") }
    var note by remember { mutableStateOf(edit?.note ?: "") }
    var reason by remember { mutableStateOf("") }
    val other = v.parents.firstOrNull { it.id != first }?.id ?: ""
    val chosen = templates.firstOrNull { it.id == template }
    val custom = template == "custom"
    val cycleOk = !custom || (cycle.size >= 2 && cycle.all { it.days in 1..28 } && cycle.sumOf { it.days } <= 56 &&
        cycle.map { it.parent }.distinct().size >= 2)
    val ok = first.isNotBlank() && other.isNotBlank() && timeOrNull(exchange) != null && (edit == null || reason.isNotBlank()) && cycleOk

    Form(if (edit == null) "Propose a schedule" else "Change your proposal", if (edit == null) "Send proposal" else "Send changes", ok, busy, onClose, {
        onSend(template, first, other, start, timeOrNull(exchange)!!.hhmm(), note.trim(), if (edit == null) null else reason.trim(),
            if (custom) cycle else emptyList())
    }) {
        Text("The other parent is asked to agree. Once they do it fills the calendar from the start date; until then nothing changes.",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        FlowRowButtons {
            templates.forEach { t -> Choice(template == t.id, t.name, { template = t.id }) }
            Choice(custom, "Custom", { template = "custom" })
        }
        chosen?.let { Text(it.description, color = pp.fgDim, style = MaterialTheme.typography.bodySmall) }
        if (custom) {
            Text("Build the cycle: each step is a number of days with one parent. It repeats from the start date.",
                color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            cycle.forEachIndexed { i, s ->
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text("${i + 1}.", color = pp.fgMute, modifier = Modifier.width(22.dp))
                    v.parents.forEach { p ->
                        Choice(s.parent == p.id, if (p.me) "Me" else p.name, {
                            cycle = cycle.toMutableList().also { it[i] = s.copy(parent = p.id) }
                        }, parentColor(v.parents, p.id))
                    }
                    TextButton(onClick = { if (s.days > 1) cycle = cycle.toMutableList().also { it[i] = s.copy(days = s.days - 1) } }) { Text("−") }
                    Text("${s.days} day" + if (s.days == 1) "" else "s", color = pp.fg)
                    TextButton(onClick = { if (s.days < 28) cycle = cycle.toMutableList().also { it[i] = s.copy(days = s.days + 1) } }) { Text("+") }
                    if (cycle.size > 2) TextButton(onClick = { cycle = cycle.toMutableList().also { it.removeAt(i) } }) { Text("Remove", color = pp.danger) }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                if (cycle.size < 8) TextButton(onClick = {
                    val next = v.parents.firstOrNull { it.id != cycle.last().parent }?.id ?: cycle.last().parent
                    cycle = cycle + Segment(next, 2)
                }) { Text("Add a step") }
                Text("Repeats every ${cycle.sumOf { it.days }} days", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            }
            if (!cycleOk) Text("A cycle needs both parents, steps of 1-28 days, and at most 56 days in all.", color = pp.danger,
                style = MaterialTheme.typography.bodySmall)
        } else {
            Text("Starts with", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
            FlowRowButtons {
                v.parents.forEach { p -> Choice(first == p.id, if (p.me) "Me" else p.name, { first = p.id }, parentColor(v.parents, p.id)) }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.Bottom) {
            DateField("From", start, { start = it })
            TimeField("Handover time", exchange, { exchange = it }, Modifier.weight(1f))
        }
        if (chosen?.monday == true) Text("This pattern runs Monday to Sunday, so it starts on the Monday of that week.",
            color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
        Field(note, { note = it }, "Note for the other parent (optional)", singleLine = false, minLines = 2)
        if (edit != null) Field(reason, { reason = it }, "Why are you changing it?", supporting = "Required - they'll need to agree again")
    }
}

@Composable
private fun AnswerDialog(p: Proposal, accept: Boolean, busy: Boolean, onClose: () -> Unit, onAnswer: (String) -> Unit) {
    var comment by remember { mutableStateOf("") }
    val what = if (p.kind == "schedule") "schedule" else "change"
    Form(if (accept) "Agree to this $what?" else "Decline this $what?", if (accept) "Agree" else "Decline",
        accept || comment.isNotBlank(), busy, onClose, { onAnswer(comment.trim()) }, danger = !accept, toneText = comment) {
        Text(if (accept) "Your answer is signed and kept with this version of the $what. If ${p.authorName} changes it later, you'll be asked again."
            else "Say why - it's kept with the request so there's a clear record.", color = pp.fgDim)
        Field(comment, { comment = it }, if (accept) "Comment (optional)" else "Reason", singleLine = false, minLines = 2)
    }
}

@Composable
private fun CheckInDialog(busy: Boolean, onClose: () -> Unit, onSend: (String, String, systems.darkcipher.parallelpath.ui.GeoFix?) -> Unit) {
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    var place by remember { mutableStateOf("") }
    var note by remember { mutableStateOf("") }
    var withLocation by remember { mutableStateOf(platform.canLocate) }
    var locating by remember { mutableStateOf(false) }
    var problem by remember { mutableStateOf<String?>(null) }
    Form("Check in", "Check in", place.isNotBlank() || note.isNotBlank(), busy || locating, onClose, {
        scope.launch {
            var fix: systems.darkcipher.parallelpath.ui.GeoFix? = null
            if (withLocation) {
                locating = true
                fix = runCatching { platform.currentLocation() }.getOrNull()
                locating = false
                if (fix == null) { problem = "Couldn't get your location. Check in without it, or try again outside."; withLocation = false; return@launch }
            }
            onSend(place.trim(), note.trim(), fix)
        }
    }) {
        Text("A check-in records that you're here now - for example at a handover. Both parents see it, with the time it reached the server.",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        Field(place, { place = it }, "Where are you?", supporting = "e.g. School gate")
        Field(note, { note = it }, "Note (optional)")
        if (platform.canLocate) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Checkbox(withLocation, { withLocation = it }, colors = CheckboxDefaults.colors(checkedColor = pp.accent, checkmarkColor = pp.accentInk))
                Text("Include my location (only for this check-in)", color = pp.fg)
            }
        }
        problem?.let { Banner(it, Tone.Warn) }
    }
}
