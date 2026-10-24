package systems.darkcipher.parallelpath.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// The calendar as the engine returns it (engine/app/calendar.go). Times are ISO-8601 instants; dates are
// yyyy-MM-dd; people are person IDs (names come alongside).

@Serializable
data class CalPerson(val id: String, val name: String = "", val me: Boolean = false)

@Serializable
data class Handover(val time: String, val from: String = "", val to: String = "", val source: String = "")

/** One day: who has the children as it starts and ends, and any handovers in between. */
@Serializable
data class CalDay(
    val date: String,
    val start: String = "",
    val end: String = "",
    val handovers: List<Handover> = emptyList(),
    val changed: Boolean = false,
)

@Serializable
data class Repeat(val every: String = "", val until: String = "")

@Serializable
data class EventData(
    val title: String = "",
    val start: String = "",
    val end: String = "",
    @SerialName("all_day") val allDay: Boolean = false,
    val zone: String = "",
    val location: String = "",
    val category: String = "",
    val children: List<String> = emptyList(),
    val repeat: Repeat = Repeat(),
    val remind: Int = 0,
)

/** One occurrence of an event. */
@Serializable
data class CalEvent(
    val id: String,
    val rev: Long = 0,
    val start: String,
    val end: String,
    val data: EventData = EventData(),
    val notes: String = "",
    val author: String = "",
    @SerialName("author_name") val authorName: String = "",
    val mine: Boolean = false,
    val edited: Boolean = false,
    val cancelled: Boolean = false,
    val reason: String = "",
    val history: List<Revision> = emptyList(),
)

@Serializable
data class CheckInData(
    val at: String = "",
    val place: String = "",
    val lat: Double? = null,
    val lon: Double? = null,
    val accuracy: Double = 0.0,
)

@Serializable
data class CheckIn(
    val id: String,
    val person: String = "",
    val name: String = "",
    val mine: Boolean = false,
    val data: CheckInData = CheckInData(),
    val note: String = "",
    val received: String = "",
)

@Serializable
data class Segment(val parent: String, val days: Int)

@Serializable
data class Pattern(val start: String = "", val exchange: String = "", val zone: String = "", val cycle: List<Segment> = emptyList())

@Serializable
data class ScheduleData(val template: String = "", val from: String = "", val pattern: Pattern = Pattern())

@Serializable
data class Change(val from: String, val to: String, val parent: String)

@Serializable
data class Answer(
    val person: String,
    val name: String = "",
    val answer: String,
    val comment: String = "",
    val time: String = "",
)

/** A schedule proposal or change request, and where it stands. */
@Serializable
data class Proposal(
    val id: String,
    val kind: String,
    val rev: Long = 0,
    val author: String = "",
    @SerialName("author_name") val authorName: String = "",
    val mine: Boolean = false,
    val created: String = "",
    val updated: String = "",
    val note: String = "",
    val reason: String = "",
    val state: String = "pending",
    val agreed: String = "",
    val waiting: List<String> = emptyList(),
    @SerialName("needs_me") val needsMe: Boolean = false,
    val answers: List<Answer> = emptyList(),
    val history: List<Revision> = emptyList(),
    val schedule: ScheduleData? = null,
    val changes: List<Change> = emptyList(),
    val problem: String = "",
)

@Serializable
data class CalendarView(
    val from: String = "",
    val to: String = "",
    val parents: List<CalPerson> = emptyList(),
    val days: List<CalDay> = emptyList(),
    val events: List<CalEvent> = emptyList(),
    val checkins: List<CheckIn> = emptyList(),
    val schedules: List<Proposal> = emptyList(),
    val changes: List<Proposal> = emptyList(),
    @SerialName("needs_me") val needsMe: Int = 0,
)

@Serializable
data class ScheduleTemplate(val id: String, val name: String, val description: String = "", val monday: Boolean = false)
