package systems.darkcipher.parallelpath.calendar

import kotlinx.datetime.DateTimeUnit
import kotlinx.datetime.DayOfWeek
import kotlinx.datetime.LocalDate
import kotlinx.datetime.LocalDateTime
import kotlinx.datetime.LocalTime
import kotlinx.datetime.TimeZone
import kotlinx.datetime.isoDayNumber
import kotlinx.datetime.minus
import kotlinx.datetime.number
import kotlinx.datetime.plus
import kotlinx.datetime.toInstant
import kotlinx.datetime.toLocalDateTime
import kotlin.time.Clock
import kotlin.time.Instant

// Dates and times for the calendar screens. Everything is shown in this device's own time zone; the engine is told
// the zone so "who has the children today" is worked out for the same days the person sees.

internal val zone: TimeZone get() = TimeZone.currentSystemDefault()

internal fun today(): LocalDate = Clock.System.now().toLocalDateTime(zone).date

internal fun parseDate(s: String): LocalDate? = runCatching { LocalDate.parse(s) }.getOrNull()

internal fun instant(iso: String): Instant? =
    if (iso.isBlank() || iso.startsWith("0001")) null else runCatching { Instant.parse(iso) }.getOrNull()

internal fun local(iso: String): LocalDateTime? = instant(iso)?.toLocalDateTime(zone)

/** The ISO instant for a local date and time on this device. */
internal fun isoAt(date: LocalDate, time: LocalTime): String = LocalDateTime(date, time).toInstant(zone).toString()

internal fun parseTime(s: String): LocalTime? {
    val m = Regex("""^(\d{1,2})[:.](\d{2})$""").matchEntire(s.trim()) ?: return null
    val (h, mi) = m.destructured
    return if (h.toInt() in 0..23 && mi.toInt() in 0..59) LocalTime(h.toInt(), mi.toInt()) else null
}

internal fun LocalTime.hhmm(): String = "${hour.toString().padStart(2, '0')}:${minute.toString().padStart(2, '0')}"

internal fun timeOf(iso: String): String = local(iso)?.time?.hhmm() ?: ""

private val months = listOf("January", "February", "March", "April", "May", "June", "July", "August", "September", "October",
    "November", "December")
private val days = listOf("Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday")

internal val weekdayInitials = listOf("Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun")

internal fun monthTitle(d: LocalDate) = "${months[d.month.number - 1]} ${d.year}"

/** "Saturday 21 November". */
internal fun LocalDate.long(): String = "${days[dayOfWeek.isoDayNumber - 1]} $day ${months[month.number - 1]}"

/** "Sat 21 Nov". */
internal fun LocalDate.short(): String = "${days[dayOfWeek.isoDayNumber - 1].take(3)} $day ${months[month.number - 1].take(3)}"

/** "Sat 21 Nov, 10:00". */
internal fun whenText(iso: String): String = local(iso)?.let { "${it.date.short()}, ${it.time.hhmm()}" } ?: ""

internal fun firstOfMonth(d: LocalDate) = LocalDate(d.year, d.month, 1)

internal fun LocalDate.addMonths(n: Int) = plus(n, DateTimeUnit.MONTH)

internal fun LocalDate.addDays(n: Int) = plus(n, DateTimeUnit.DAY)

/** The Monday on or before the 1st: the month grid's first cell. */
internal fun gridStart(month: LocalDate): LocalDate {
    val first = firstOfMonth(month)
    return first.minus(first.dayOfWeek.isoDayNumber - DayOfWeek.MONDAY.isoDayNumber, DateTimeUnit.DAY)
}

/** Epoch milliseconds of a date at UTC midnight (what the Material date picker uses). */
internal fun LocalDate.pickerMillis(): Long = LocalDateTime(this, LocalTime(0, 0)).toInstant(TimeZone.UTC).toEpochMilliseconds()

internal fun dateFromPicker(ms: Long): LocalDate = Instant.fromEpochMilliseconds(ms).toLocalDateTime(TimeZone.UTC).date
