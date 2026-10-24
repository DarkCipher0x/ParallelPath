package systems.darkcipher.parallelpath.calendar

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CalendarMonth
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
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import kotlinx.datetime.LocalDate
import kotlinx.datetime.LocalTime
import systems.darkcipher.parallelpath.model.CalPerson
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.pp

/** A button showing a date; tapping it opens the date picker. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun DateField(label: String, date: LocalDate, onDate: (LocalDate) -> Unit, modifier: Modifier = Modifier) {
    var open by remember { mutableStateOf(false) }
    Column(modifier) {
        Text(label, color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        OutlinedButton(
            onClick = { open = true }, shape = RoundedCornerShape(Radius.small), border = BorderStroke(1.dp, pp.line2),
            colors = ButtonDefaults.outlinedButtonColors(contentColor = pp.fg, containerColor = pp.bg2),
            contentPadding = PaddingValues(horizontal = 14.dp, vertical = 10.dp),
        ) {
            Icon(Icons.Outlined.CalendarMonth, null, tint = pp.accent)
            Text("  ${date.short()} ${date.year}")
        }
    }
    if (open) {
        val state = rememberDatePickerState(initialSelectedDateMillis = date.pickerMillis())
        DatePickerDialog(
            onDismissRequest = { open = false },
            confirmButton = {
                TextButton(onClick = { state.selectedDateMillis?.let { onDate(dateFromPicker(it)) }; open = false }) { Text("OK") }
            },
            dismissButton = { TextButton(onClick = { open = false }) { Text("Cancel") } },
            colors = DatePickerDefaults.colors(containerColor = pp.panel),
        ) { DatePicker(state, showModeToggle = false) }
    }
}

/** A 24-hour time typed as hh:mm; red until it parses. */
@Composable
internal fun TimeField(label: String, text: String, onText: (String) -> Unit, modifier: Modifier = Modifier) {
    Field(text, onText, label, modifier = modifier, keyboard = KeyboardType.Number,
        supporting = if (text.isNotBlank() && parseTime(text) == null) "Use 24-hour time, e.g. 18:00" else null)
}

internal fun timeOrNull(text: String): LocalTime? = parseTime(text)

@Composable
internal fun Choice(selected: Boolean, label: String, onClick: () -> Unit, color: Color = pp.accent) {
    FilterChip(selected = selected, onClick = onClick, label = { Text(label) },
        colors = FilterChipDefaults.filterChipColors(selectedContainerColor = color.copy(alpha = 0.16f), selectedLabelColor = color))
}

/** Each parent's colour, by their place in the family (stable for everyone in it). */
@Composable
internal fun parentColor(parents: List<CalPerson>, id: String): Color {
    val palette = listOf(pp.accent, pp.warm, pp.violet, pp.ok)
    val i = parents.indexOfFirst { it.id == id }
    return if (i < 0) pp.fgMute else palette[i % palette.size]
}

internal fun nameOf(parents: List<CalPerson>, id: String): String =
    parents.firstOrNull { it.id == id }?.let { if (it.me) "you" else it.name.ifBlank { "the other parent" } } ?: "nobody"

internal fun capital(s: String) = s.replaceFirstChar { it.uppercase() }
