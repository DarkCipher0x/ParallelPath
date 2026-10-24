package systems.darkcipher.parallelpath.ui

import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit
import java.util.Locale

/** java.time formatting shared by Android and Windows (both run on the JVM). */
internal object JvmTime {
    private fun parse(iso: String): ZonedDateTime? = try {
        if (iso.isBlank() || iso.startsWith("0001-")) null
        else OffsetDateTime.parse(iso).atZoneSameInstant(ZoneId.systemDefault())
    } catch (_: Exception) {
        null
    }

    private val time = DateTimeFormatter.ofPattern("HH:mm", Locale.getDefault())
    private val weekday = DateTimeFormatter.ofPattern("EEE HH:mm", Locale.getDefault())
    private val date = DateTimeFormatter.ofPattern("d MMM HH:mm", Locale.getDefault())
    private val dateYear = DateTimeFormatter.ofPattern("d MMM yyyy", Locale.getDefault())
    private val full = DateTimeFormatter.ofPattern("EEEE d MMMM yyyy, HH:mm:ss", Locale.getDefault())

    fun short(iso: String): String {
        val t = parse(iso) ?: return ""
        val today = LocalDate.now()
        val d = t.toLocalDate()
        return when {
            d == today -> t.format(time)
            ChronoUnit.DAYS.between(d, today) < 7 -> t.format(weekday)
            d.year == today.year -> t.format(date)
            else -> t.format(dateYear)
        }
    }

    fun full(iso: String): String = parse(iso)?.format(full) ?: ""

    fun daysFromNow(days: Int): String = OffsetDateTime.now().plusDays(days.toLong()).withNano(0).toString()
}
