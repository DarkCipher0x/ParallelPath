package systems.darkcipher.parallelpath.ui

/** "14:05" today, "Mon 14:05" this week, "3 Oct 14:05" otherwise (local time). Empty for a missing time. */
expect fun shortTime(iso: String): String

/** Full timestamp, e.g. "Monday 3 October 2026, 14:05:09" (for the audit log and history). */
expect fun fullTime(iso: String): String

/** ISO-8601 for a date some days from now (end dates for professionals' access). */
expect fun isoDaysFromNow(days: Int): String
