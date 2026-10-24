package systems.darkcipher.parallelpath.calendar

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.outlined.EventNote
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.EventBusy
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.outlined.SwapHoriz
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.datetime.LocalDate
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.CalDay
import systems.darkcipher.parallelpath.model.CalEvent
import systems.darkcipher.parallelpath.model.CalendarView
import systems.darkcipher.parallelpath.model.CheckIn
import systems.darkcipher.parallelpath.model.ScheduleTemplate
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.shortTime

internal enum class Mode { Month, Requests, Schedule }

/**
 * The family calendar: who has the children each day (from the agreed schedule and agreed changes), events,
 * check-ins, and the requests waiting on someone. Everything here is end-to-end encrypted like the messages,
 * and every change is kept in the history and the audit log.
 */
@Composable
fun CalendarScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit, startMode: Int = 0, startDate: String? = null) {
    var month by remember { mutableStateOf(firstOfMonth(startDate?.let { parseDate(it) } ?: today())) }
    var selected by remember { mutableStateOf(startDate?.let { parseDate(it) } ?: today()) }
    var view by remember { mutableStateOf<CalendarView?>(null) }
    var templates by remember { mutableStateOf<List<ScheduleTemplate>>(emptyList()) }
    var mode by remember { mutableStateOf(Mode.entries.getOrElse(startMode) { Mode.Month }) }
    var reload by remember { mutableStateOf(0) }
    var dialog by remember { mutableStateOf<Dialog?>(null) }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val canEdit = space.isParent && !space.waiting && !space.readOnly

    LaunchedEffect(space.id, month, tick, reload) {
        val first = gridStart(month)
        view = runCatching { engine.calendar(space.id, first.toString(), first.addDays(41).toString(), zone.id) }
            .onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't read the calendar") }.getOrNull() ?: view
    }
    LaunchedEffect(Unit) { templates = runCatching { engine.scheduleTemplates() }.getOrDefault(emptyList()) }

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
            } finally {
                busy = false
            }
        }
    }

    val v = view
    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp), horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically) {
            Choice(mode == Mode.Month, "Calendar", { mode = Mode.Month })
            val waiting = v?.needsMe ?: 0
            Choice(mode == Mode.Requests, if (waiting > 0) "Requests ($waiting)" else "Requests", { mode = Mode.Requests },
                if (waiting > 0) pp.warm else pp.accent)
            Choice(mode == Mode.Schedule, "Schedule", { mode = Mode.Schedule })
        }
        if (v == null) return@Column
        if (space.waiting) Banner("The calendar opens once a parent approves you.", Tone.Warn, Modifier.padding(16.dp).fillMaxWidth())
        Box(Modifier.weight(1f)) {
            when (mode) {
                Mode.Month -> MonthMode(v, month, selected, canEdit, space.readOnly,
                    onMonth = { month = it }, onSelect = { selected = it; if (firstOfMonth(it) != month) month = firstOfMonth(it) },
                    onDialog = { dialog = it })
                Mode.Requests -> RequestsPanel(v, templates, canEdit, onDialog = { dialog = it })
                Mode.Schedule -> SchedulePanel(v, templates, canEdit, onDialog = { dialog = it })
            }
        }
    }

    val d = dialog
    if (d != null && v != null) Dialogs(d, v, templates, busy, engine, space.id, act, { dialog = it }) { dialog = null }
}

@Composable
private fun MonthMode(
    v: CalendarView, month: LocalDate, selected: LocalDate, canEdit: Boolean, readOnly: Boolean,
    onMonth: (LocalDate) -> Unit, onSelect: (LocalDate) -> Unit, onDialog: (Dialog) -> Unit,
) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val wide = maxWidth >= 860.dp
        val grid: @Composable (Modifier) -> Unit = { m -> MonthGrid(v, month, selected, onMonth, onSelect, m, if (wide) 1.35f else 1f) }
        val day: @Composable (Modifier) -> Unit = { m -> DayPanel(v, selected, canEdit, readOnly, onDialog, m) }
        if (wide) {
            Row(Modifier.fillMaxSize().padding(horizontal = 16.dp).padding(bottom = 16.dp), horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                Column(Modifier.weight(1.4f).fillMaxHeight().verticalScroll(rememberScrollState())) { grid(Modifier.fillMaxWidth()) }
                Column(Modifier.weight(1f).fillMaxHeight().verticalScroll(rememberScrollState())) { day(Modifier.fillMaxWidth()) }
            }
        } else {
            Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 12.dp).padding(bottom = 16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)) {
                grid(Modifier.fillMaxWidth())
                day(Modifier.fillMaxWidth())
            }
        }
    }
}

@Composable
private fun MonthGrid(
    v: CalendarView, month: LocalDate, selected: LocalDate, onMonth: (LocalDate) -> Unit, onSelect: (LocalDate) -> Unit,
    modifier: Modifier, cellRatio: Float,
) {
    val days = v.days.associateBy { it.date }
    val eventDays = v.events.filter { !it.cancelled }.flatMap { e -> datesOf(e) }.toSet()
    val now = today()
    Panel(modifier, padding = androidx.compose.foundation.layout.PaddingValues(12.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = { onMonth(month.addMonths(-1)) }) { Icon(Icons.AutoMirrored.Outlined.KeyboardArrowLeft, "Previous month", tint = pp.fg) }
            Text(monthTitle(month), Modifier.weight(1f), color = pp.fg, style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold)
            Text("Today", Modifier.clip(RoundedCornerShape(Radius.small)).clickable { onSelect(now) }.padding(8.dp), color = pp.accent)
            IconButton(onClick = { onMonth(month.addMonths(1)) }) { Icon(Icons.AutoMirrored.Outlined.KeyboardArrowRight, "Next month", tint = pp.fg) }
        }
        Row(Modifier.fillMaxWidth()) {
            weekdayInitials.forEach { Text(it, Modifier.weight(1f), color = pp.fgMute, style = MaterialTheme.typography.labelMedium,
                textAlign = androidx.compose.ui.text.style.TextAlign.Center) }
        }
        val start = gridStart(month)
        val weeks = (firstOfMonth(month).addMonths(1).addDays(-1).toEpochDays() - start.toEpochDays()).toInt() / 7 + 1
        for (week in 0 until weeks) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                for (i in 0 until 7) {
                    val date = start.addDays(week * 7 + i)
                    DayCell(date, days[date.toString()], v, date.month == month.month, date == selected, date == now,
                        date in eventDays, Modifier.weight(1f).aspectRatio(cellRatio)) { onSelect(date) }
                }
            }
            Spacer(Modifier.height(4.dp))
        }
        Legend(v)
    }
}

@Composable
private fun DayCell(
    date: LocalDate, day: CalDay?, v: CalendarView, inMonth: Boolean, selected: Boolean, isToday: Boolean, hasEvents: Boolean,
    modifier: Modifier, onClick: () -> Unit,
) {
    val startC = day?.start?.takeIf { it.isNotBlank() }?.let { parentColor(v.parents, it) }
    val endC = day?.end?.takeIf { it.isNotBlank() }?.let { parentColor(v.parents, it) }
    val shape = RoundedCornerShape(8.dp)
    Column(
        modifier.clip(shape).background(if (selected) pp.accentSoft else pp.bg2)
            .border(if (selected || isToday) 1.5.dp else 1.dp, if (selected) pp.accent else if (isToday) pp.accent2 else pp.line, shape)
            .clickable(onClick = onClick).padding(4.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(date.day.toString(), color = if (inMonth) pp.fg else pp.fgMute, fontWeight = if (isToday) FontWeight.Bold else null,
                style = MaterialTheme.typography.labelLarge)
            Spacer(Modifier.weight(1f))
            if (day?.changed == true) Icon(Icons.Outlined.SwapHoriz, "Changed", tint = pp.warm, modifier = Modifier.size(12.dp))
        }
        Spacer(Modifier.weight(1f))
        if (hasEvents) Box(Modifier.padding(bottom = 3.dp).size(6.dp).clip(CircleShape).background(pp.fg.copy(alpha = if (inMonth) 0.8f else 0.4f)))
        // Who has the children: the day's start and end (a handover splits the bar).
        if (startC != null || endC != null) {
            Row(Modifier.fillMaxWidth().height(5.dp).clip(CircleShape)) {
                Box(Modifier.weight(1f).fillMaxHeight().background((startC ?: pp.line).copy(alpha = if (inMonth) 1f else 0.4f)))
                Box(Modifier.weight(1f).fillMaxHeight().background((endC ?: pp.line).copy(alpha = if (inMonth) 1f else 0.4f)))
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun Legend(v: CalendarView) {
    if (v.parents.isEmpty()) return
    FlowRow(horizontalArrangement = Arrangement.spacedBy(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        v.parents.forEach { p ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(10.dp).clip(CircleShape).background(parentColor(v.parents, p.id)))
                Text("  " + if (p.me) "You" else p.name, color = pp.fgDim, style = MaterialTheme.typography.labelMedium)
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.SwapHoriz, null, tint = pp.warm, modifier = Modifier.size(12.dp))
            Text(" agreed change", color = pp.fgDim, style = MaterialTheme.typography.labelMedium)
        }
    }
}

@Composable
private fun DayPanel(v: CalendarView, date: LocalDate, canEdit: Boolean, readOnly: Boolean, onDialog: (Dialog) -> Unit, modifier: Modifier) {
    val day = v.days.firstOrNull { it.date == date.toString() }
    val events = v.events.filter { date in datesOf(it) }.sortedBy { it.start }
    val checkins = v.checkins.filter { local(it.data.at)?.date == date }.sortedBy { it.data.at }
    Column(modifier, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Panel(Modifier.fillMaxWidth()) {
            Text(date.long(), color = pp.fg, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            Text(custodyText(v, day), color = pp.fgDim)
            day?.handovers?.forEach { h ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.SwapHoriz, null, tint = parentColor(v.parents, h.to), modifier = Modifier.size(18.dp))
                    Text("  ${timeOf(h.time)}  handover to ${nameOf(v.parents, h.to)}" + if (h.source.isNotBlank() && h.source != "schedule") " (agreed change)" else "",
                        color = pp.fg, style = MaterialTheme.typography.bodyMedium)
                }
            }
            if (canEdit) {
                FlowRowButtons {
                    PrimaryButton("Add event", onClick = { onDialog(Dialog.Event(date, null)) })
                    SecondaryButton("Ask for a change", onClick = { onDialog(Dialog.Change(date)) })
                }
            }
            if (!readOnly && !canEdit) Text("Only parents can add events or ask for changes.", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
        }
        Panel(Modifier.fillMaxWidth()) {
            SectionTitle("Events")
            if (events.isEmpty()) Text("Nothing on this day.", color = pp.fgMute)
            events.forEach { e -> EventRow(e, canEdit, date, onDialog) { onDialog(Dialog.Details(e)) } }
        }
        Panel(Modifier.fillMaxWidth()) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                SectionTitle("Check-ins", Modifier.weight(1f))
                if (!readOnly && date == today()) androidx.compose.material3.TextButton(onClick = { onDialog(Dialog.CheckIn) }) { Text("Check in", color = pp.accent) }
            }
            if (checkins.isEmpty()) Text(if (date == today()) "No check-ins today. Check in at handovers so there's a record both of you can see."
                else "No check-ins on this day.", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
            checkins.forEach { CheckInRow(it) }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun FlowRowButtons(content: @Composable () -> Unit) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
}

@Composable
private fun EventRow(e: CalEvent, canEdit: Boolean, date: LocalDate, onDialog: (Dialog) -> Unit, onClick: () -> Unit) {
    val strike = if (e.cancelled) androidx.compose.ui.text.style.TextDecoration.LineThrough else null
    val platform = systems.darkcipher.parallelpath.ui.LocalPlatform.current
    val mayChange = canEdit && e.mine && !e.cancelled
    systems.darkcipher.parallelpath.ui.ContextMenuArea({
        systems.darkcipher.parallelpath.ui.menu(
            systems.darkcipher.parallelpath.ui.MenuItem("Details", Icons.AutoMirrored.Outlined.EventNote, "Enter") { onClick() },
            if (canEdit && e.mine) systems.darkcipher.parallelpath.ui.MenuItem("Change…", Icons.Outlined.Edit, enabled = mayChange,
                hint = "A cancelled event can't be changed") { onDialog(Dialog.Event(date, e)) } else null,
            systems.darkcipher.parallelpath.ui.MenuItem("Copy details", Icons.Outlined.ContentCopy) {
                platform.copyText(listOfNotNull(e.data.title, if (e.data.allDay) "All day" else "${timeOf(e.start)}–${timeOf(e.end)}",
                    e.data.location.ifBlank { null }, e.notes.ifBlank { null }).joinToString("\n"))
            },
            if (e.history.size > 1) systems.darkcipher.parallelpath.ui.MenuItem("History (${e.history.size})", Icons.Outlined.History) {
                onDialog(Dialog.History(e.history, e.data.title))
            } else null,
            systems.darkcipher.parallelpath.ui.MenuDivider,
            if (canEdit && e.mine) systems.darkcipher.parallelpath.ui.MenuItem("Cancel event…", Icons.Outlined.EventBusy, enabled = mayChange, danger = true,
                hint = "It's already cancelled") { onDialog(Dialog.CancelEvent(e)) } else null,
        )
    }, Modifier.fillMaxWidth(), onClick = { onClick() }) {
    Row(Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small)).padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.width(4.dp).height(36.dp).clip(CircleShape).background(categoryColor(e.data.category)))
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            Text(e.data.title, color = if (e.cancelled) pp.fgMute else pp.fg, textDecoration = strike, maxLines = 1, overflow = TextOverflow.Ellipsis,
                fontWeight = FontWeight.SemiBold)
            val detail = buildList {
                add(if (e.data.allDay) "All day" else "${timeOf(e.start)}–${timeOf(e.end)}")
                if (e.data.location.isNotBlank()) add(e.data.location)
                if (e.data.children.isNotEmpty()) add(e.data.children.joinToString())
                if (e.cancelled) add("cancelled") else if (e.edited) add("edited")
            }.joinToString(" · ")
            Text(detail, color = if (e.cancelled) pp.danger else pp.fgDim, style = MaterialTheme.typography.bodySmall, maxLines = 1,
                overflow = TextOverflow.Ellipsis)
        }
        Icon(Icons.AutoMirrored.Outlined.EventNote, null, tint = pp.fgMute, modifier = Modifier.size(18.dp))
    }
    }
}

@Composable
private fun CheckInRow(c: CheckIn) {
    Row(verticalAlignment = Alignment.Top) {
        Icon(Icons.Outlined.Place, null, tint = pp.ok, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(8.dp))
        Column {
            Text("${if (c.mine) "You" else c.name} · ${timeOf(c.data.at)}" + if (c.data.place.isNotBlank()) " · ${c.data.place}" else "",
                color = pp.fg, style = MaterialTheme.typography.bodyMedium)
            if (c.note.isNotBlank()) Text(c.note, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            val lat = c.data.lat
            val lon = c.data.lon
            if (lat != null && lon != null) Text("Location ${fmt(lat)}, ${fmt(lon)} (±${c.data.accuracy.toInt()} m) · sent from their device",
                color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
            if (c.received.isNotBlank()) Text("Received by the server ${shortTime(c.received)}", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
        }
    }
}

private fun fmt(d: Double): String {
    val r = kotlin.math.round(d * 100000) / 100000
    return r.toString()
}

@Composable
internal fun categoryColor(c: String) = when (c) {
    "school" -> pp.accent
    "medical" -> pp.danger
    "activity" -> pp.ok
    "holiday" -> pp.violet
    "handover" -> pp.warm
    else -> pp.fgDim
}

internal val categories = listOf("activity" to "Activity", "school" to "School", "medical" to "Medical", "holiday" to "Holiday",
    "handover" to "Handover", "other" to "Other")

private fun custodyText(v: CalendarView, day: CalDay?): String {
    if (day == null || (day.start.isBlank() && day.end.isBlank())) {
        return if (v.schedules.none { it.state == "agreed" }) "No agreed schedule yet - propose one from Schedule." else "No schedule on this day."
    }
    return when {
        day.handovers.isEmpty() && day.start == day.end -> "With ${nameOf(v.parents, day.start)} all day."
        day.start.isBlank() -> "With ${nameOf(v.parents, day.end)} from ${timeOf(day.handovers.firstOrNull()?.time ?: "")}."
        else -> "With ${nameOf(v.parents, day.start)} as the day starts, ${nameOf(v.parents, day.end)} by the end."
    } + if (day.changed) " Includes an agreed change." else ""
}

/** The local dates an event occurrence covers (an all-day event ending at midnight doesn't cover the next day). */
internal fun datesOf(e: CalEvent): List<LocalDate> {
    val s = local(e.start)?.date ?: return emptyList()
    val endLocal = local(e.end) ?: return listOf(s)
    var last = endLocal.date
    if (last > s && endLocal.time.hour == 0 && endLocal.time.minute == 0) last = last.addDays(-1)
    val out = mutableListOf<LocalDate>()
    var d = s
    while (d <= last && out.size < 63) { out += d; d = d.addDays(1) }
    return out
}

/** How many calendar requests are waiting on this person (for the tab's badge). */
suspend fun calendarWaiting(engine: Engine, space: String): Int {
    val d = today()
    return engine.calendar(space, d.toString(), d.toString(), zone.id).needsMe
}
