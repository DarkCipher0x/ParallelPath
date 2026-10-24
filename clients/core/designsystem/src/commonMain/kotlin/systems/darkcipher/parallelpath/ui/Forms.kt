package systems.darkcipher.parallelpath.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDefaults
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import kotlinx.datetime.LocalDate
import kotlinx.datetime.TimeZone
import kotlinx.datetime.isoDayNumber
import kotlinx.datetime.number
import kotlinx.datetime.todayIn
import kotlin.time.Clock
import kotlin.time.ExperimentalTime

// Form building blocks shared by the feature screens: dates, choices, dialogs, money.

@OptIn(ExperimentalTime::class)
fun todayLocal(): LocalDate = Clock.System.todayIn(TimeZone.currentSystemDefault())

private val monthNames = listOf("Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec")
private val dayNames = listOf("Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun")

/** "Sat 7 Nov 2026". */
fun LocalDate.pretty(): String = "${dayNames[dayOfWeek.isoDayNumber - 1]} $day ${monthNames[month.number - 1]} $year"

/** A yyyy-MM-dd date as "Sat 7 Nov 2026" (or the text as it is, if it isn't a date). */
fun prettyDate(iso: String): String = runCatching { LocalDate.parse(iso.take(10)).pretty() }.getOrDefault(iso)

/** A button showing a date; tapping it opens the date picker. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DateField(label: String, date: LocalDate, onDate: (LocalDate) -> Unit, modifier: Modifier = Modifier) {
    var open by remember { mutableStateOf(false) }
    Column(modifier) {
        Text(label, color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        OutlinedButton(
            onClick = { open = true }, shape = RoundedCornerShape(Radius.small), border = BorderStroke(1.dp, pp.line2),
            colors = ButtonDefaults.outlinedButtonColors(contentColor = pp.fg, containerColor = pp.bg2),
            contentPadding = PaddingValues(horizontal = 14.dp, vertical = 10.dp),
        ) {
            Icon(Icons.Outlined.CalendarMonth, null, tint = pp.accent)
            Text("  ${date.pretty()}")
        }
    }
    if (open) {
        // The Material picker works in UTC milliseconds at midnight.
        val state = rememberDatePickerState(initialSelectedDateMillis = date.toEpochDays() * 86_400_000L)
        DatePickerDialog(
            onDismissRequest = { open = false },
            confirmButton = {
                TextButton(onClick = {
                    state.selectedDateMillis?.let { onDate(LocalDate.fromEpochDays((it / 86_400_000L).toInt())) }
                    open = false
                }) { Text("OK") }
            },
            dismissButton = { TextButton(onClick = { open = false }) { Text("Cancel") } },
            colors = DatePickerDefaults.colors(containerColor = pp.panel),
        ) { DatePicker(state, showModeToggle = false) }
    }
}

/** A selectable chip. */
@Composable
fun Choice(selected: Boolean, label: String, onClick: () -> Unit, color: Color = pp.accent) {
    FilterChip(selected = selected, onClick = onClick, label = { Text(label) },
        colors = FilterChipDefaults.filterChipColors(selectedContainerColor = color.copy(alpha = 0.16f), selectedLabelColor = color))
}

/** Buttons or chips that wrap onto more lines on narrow screens. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun FlowButtons(content: @Composable () -> Unit) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { content() }
}

/** A dialog holding a form: a scrolling body, a confirm button (red for destructive actions) and Cancel. */
@Composable
fun FormDialog(
    title: String, confirm: String, enabled: Boolean, busy: Boolean, onClose: () -> Unit, onConfirm: () -> Unit,
    danger: Boolean = false,
    // Text other people will read (a reason, a comment, a note): it goes through the tone check first.
    toneText: String? = null,
    content: @Composable () -> Unit,
) {
    val gate = rememberToneGate()
    val confirmChecked: () -> Unit = { if (toneText == null) onConfirm() else gate.guard(toneText) { onConfirm() } }
    AlertDialog(
        onDismissRequest = onClose, containerColor = pp.panel,
        title = { Text(title) },
        text = {
            Column(Modifier.widthIn(min = 300.dp, max = 560.dp).heightIn(max = 620.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)) { content() }
        },
        confirmButton = {
            if (danger) SecondaryButton(confirm, danger = true, enabled = enabled && !busy && !gate.checking, onClick = confirmChecked)
            else PrimaryButton(confirm, enabled = enabled && !gate.checking, busy = busy, onClick = confirmChecked)
        },
        dismissButton = { TextButton(onClick = onClose) { Text("Cancel") } },
    )
}

/** Asks for the reason that every change, withdrawal or removal must carry. */
@Composable
fun ReasonDialog(title: String, body: String, confirm: String, busy: Boolean, onClose: () -> Unit, onConfirm: (String) -> Unit) {
    var reason by remember { mutableStateOf("") }
    FormDialog(title, confirm, reason.isNotBlank(), busy, onClose, { onConfirm(reason.trim()) }, danger = true, toneText = reason) {
        Text(body, color = pp.fgDim)
        Field(reason, { reason = it }, "Reason", supporting = "Required - kept in the history")
    }
}

/** A read-only list of earlier versions: when, and why it changed. */
@Composable
fun HistoryList(history: List<systems.darkcipher.parallelpath.model.Revision>) {
    history.forEach { r ->
        Column {
            Text("${if (r.rev == 0L) "Added" else if (r.retracted) "Removed" else "Changed"} ${fullTime(r.time)}", color = pp.fgDim,
                style = MaterialTheme.typography.bodySmall)
            if (r.reason.isNotBlank()) Text("Reason: ${r.reason}", color = pp.fg, style = MaterialTheme.typography.bodySmall)
        }
    }
}

private val symbols = mapOf("GBP" to "£", "EUR" to "€", "USD" to "$", "AUD" to "A$", "CAD" to "C$", "NZD" to "NZ$", "IEP" to "£")

/** 2400, "GBP" -> "£24.00". */
fun formatMoney(minor: Long, currency: String): String {
    val sign = if (minor < 0) "-" else ""
    val abs = kotlin.math.abs(minor)
    val units = (abs / 100).toString().reversed().chunked(3).joinToString(",").reversed()
    val s = "$units.${(abs % 100).toString().padStart(2, '0')}"
    return sign + (symbols[currency]?.let { it + s } ?: "$s $currency")
}

/** "24", "24.5", "£1,024.50" -> minor units; null if it isn't an amount. */
fun parseMoney(text: String): Long? {
    val t = text.trim().trimStart('£', '€', '$').replace(",", "")
    val m = Regex("""^(\d{1,9})(?:\.(\d{1,2}))?$""").matchEntire(t) ?: return null
    val (units, frac) = m.destructured
    return units.toLong() * 100 + frac.padEnd(2, '0').ifBlank { "00" }.toLong()
}
