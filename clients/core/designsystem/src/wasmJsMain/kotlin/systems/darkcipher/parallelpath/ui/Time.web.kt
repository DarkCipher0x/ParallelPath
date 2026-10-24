package systems.darkcipher.parallelpath.ui

import kotlinx.datetime.DateTimeUnit
import kotlinx.datetime.TimeZone
import kotlinx.datetime.daysUntil
import kotlinx.datetime.isoDayNumber
import kotlinx.datetime.number
import kotlinx.datetime.plus
import kotlinx.datetime.toLocalDateTime
import kotlin.time.Clock
import kotlin.time.ExperimentalTime
import kotlin.time.Instant

// The web app's time formatting (the JVM apps use java.time; the browser has kotlinx-datetime).

private val monthsShort = listOf("Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec")
private val monthsLong = listOf("January", "February", "March", "April", "May", "June", "July", "August", "September", "October",
    "November", "December")
private val daysShort = listOf("Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun")
private val daysLong = listOf("Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday")

private fun two(n: Int) = n.toString().padStart(2, '0')

@OptIn(ExperimentalTime::class)
private fun local(iso: String) =
    if (iso.isBlank() || iso.startsWith("0001-")) null
    else runCatching { Instant.parse(iso).toLocalDateTime(TimeZone.currentSystemDefault()) }.getOrNull()

@OptIn(ExperimentalTime::class)
actual fun shortTime(iso: String): String {
    val t = local(iso) ?: return ""
    val today = Clock.System.now().toLocalDateTime(TimeZone.currentSystemDefault()).date
    val hm = "${two(t.hour)}:${two(t.minute)}"
    val ago = t.date.daysUntil(today)
    return when {
        ago == 0 -> hm
        ago in 1..6 -> "${daysShort[t.dayOfWeek.isoDayNumber - 1]} $hm"
        t.year == today.year -> "${t.day} ${monthsShort[t.month.number - 1]} $hm"
        else -> "${t.day} ${monthsShort[t.month.number - 1]} ${t.year}"
    }
}

actual fun fullTime(iso: String): String {
    val t = local(iso) ?: return ""
    return "${daysLong[t.dayOfWeek.isoDayNumber - 1]} ${t.day} ${monthsLong[t.month.number - 1]} ${t.year}, " +
        "${two(t.hour)}:${two(t.minute)}:${two(t.second)}"
}

@OptIn(ExperimentalTime::class)
actual fun isoDaysFromNow(days: Int): String =
    Clock.System.now().plus(days, DateTimeUnit.DAY, TimeZone.currentSystemDefault()).toString()
