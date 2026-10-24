package systems.darkcipher.parallelpath.ui

actual fun shortTime(iso: String): String = JvmTime.short(iso)
actual fun fullTime(iso: String): String = JvmTime.full(iso)
actual fun isoDaysFromNow(days: Int): String = JvmTime.daysFromNow(days)
