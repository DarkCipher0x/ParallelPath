package systems.darkcipher.parallelpath.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// Milestone 3, as the engine returns it: expenses (engine/app/expenses.go), documents (documents.go), the info bank
// (infobank.go) and the private journal (journal.go). Amounts are minor units (pence); dates are yyyy-MM-dd.

// ---- expenses

@Serializable
data class ExpenseData(
    val title: String = "",
    val amount: Long = 0,
    val currency: String = "GBP",
    val date: String = "",
    val category: String = "",
    val children: List<String> = emptyList(),
    val share: Int = 50,
)

@Serializable
data class ExpenseAnswer(
    val person: String,
    val name: String = "",
    val answer: String,
    val comment: String = "",
    val method: String = "",
    val reference: String = "",
    @SerialName("for") val forName: String = "",
    val time: String = "",
    val current: Boolean = false,
)

@Serializable
data class Expense(
    val id: String,
    val rev: Long = 0,
    val author: String = "",
    @SerialName("author_name") val authorName: String = "",
    val mine: Boolean = false,
    val created: String = "",
    val updated: String = "",
    val data: ExpenseData = ExpenseData(),
    val note: String = "",
    val owed: Long = 0,
    val state: String = "pending",
    val reason: String = "",
    val waiting: List<String> = emptyList(), // names
    @SerialName("needs_me") val needsMe: Boolean = false,
    @SerialName("can_pay") val canPay: Boolean = false,
    @SerialName("to_confirm") val toConfirm: List<String> = emptyList(), // person IDs whose payment awaits confirmation
    val answers: List<ExpenseAnswer> = emptyList(),
    val history: List<Revision> = emptyList(),
    val receipts: List<Attachment> = emptyList(),
    val problem: String = "",
)

@Serializable
data class Owing(
    val from: String,
    @SerialName("from_name") val fromName: String = "",
    val to: String,
    @SerialName("to_name") val toName: String = "",
    val amount: Long,
    val currency: String,
)

@Serializable
data class ExpensesView(
    val parents: List<CalPerson> = emptyList(),
    val expenses: List<Expense> = emptyList(),
    val balances: List<Owing> = emptyList(),
    @SerialName("needs_me") val needsMe: Int = 0,
)

// ---- documents

@Serializable
data class DocVersion(val rev: Long = 0, val time: String = "", val reason: String = "", val file: Attachment)

@Serializable
data class DocAccess(val person: String, val name: String = "", val time: String = "", val rev: Long = 0)

@Serializable
data class Doc(
    val id: String = "",
    val rev: Long = 0,
    val type: String,
    val folder: String = "",
    val name: String,
    val path: String,
    val note: String = "",
    val author: String = "",
    @SerialName("author_name") val authorName: String = "",
    val mine: Boolean = false,
    val created: String = "",
    val updated: String = "",
    val removed: Boolean = false,
    val reason: String = "",
    val file: Attachment? = null,
    val versions: List<DocVersion> = emptyList(),
    val opened: List<DocAccess> = emptyList(),
    val history: List<Revision> = emptyList(),
    val implied: Boolean = false,
) {
    val isFolder get() = type == "folder"
}

/** A file attached to a message in the family or one of your private conversations there. */
@Serializable
data class SharedFile(
    val space: String,
    val conversation: String = "",
    val message: String,
    val author: String = "",
    @SerialName("author_name") val authorName: String = "",
    val mine: Boolean = false,
    val time: String = "",
    val text: String = "",
    val file: Attachment,
)

// ---- info bank

@Serializable
data class InfoVersion(
    val id: String,
    val person: String = "",
    val name: String = "",
    val time: String = "",
    val label: String = "",
    val value: String = "",
    val note: String = "",
    val why: String = "",
    val removed: Boolean = false,
)

@Serializable
data class InfoEntry(
    val id: String,
    val chain: String,
    val type: String,
    val child: String = "",
    val section: String = "",
    val label: String = "",
    val value: String = "",
    val note: String = "",
    val attachments: List<Attachment> = emptyList(),
    @SerialName("by_name") val byName: String = "",
    val mine: Boolean = false,
    val updated: String = "",
    val conflict: Boolean = false,
    val history: List<InfoVersion> = emptyList(),
)

@Serializable
data class InfoView(
    val children: List<InfoEntry> = emptyList(),
    val entries: List<InfoEntry> = emptyList(),
    val removed: List<InfoEntry> = emptyList(),
)

// ---- journal (private)

@Serializable
data class JournalData(
    val family: String = "",
    val date: String = "",
    val mood: String = "",
    val tags: List<String> = emptyList(),
)

@Serializable
data class JournalEntry(
    val id: String,
    val rev: Long = 0,
    val data: JournalData = JournalData(),
    val text: String = "",
    val written: String = "",
    val updated: String = "",
    val edited: Boolean = false,
    val removed: Boolean = false,
    val reason: String = "",
    val attachments: List<Attachment> = emptyList(),
    val history: List<Revision> = emptyList(),
)

// ---- calls

/** A call-setup message from the engine (the app uses "ring", "accept" and "hangup"; the call page the rest). */
@Serializable
data class CallEvent(
    val space: String,
    val call: String,
    val type: String,
    @SerialName("from_person") val fromPerson: String = "",
    @SerialName("from_name") val fromName: String = "",
    @SerialName("from_device") val fromDevice: String = "",
    val video: Boolean = false,
    @SerialName("family_name") val familyName: String = "",
)

/** A finished call shown in the conversation. */
@Serializable
data class CallInfo(val video: Boolean = false, val seconds: Int = 0, val missed: Boolean = false)

// ---- court-ready reports

@Serializable
data class ReportFiles(val id: String, val pdf: String, val json: String, val csv: String = "", val sha3: String = "", val signer: String = "")

/** Something to show as a system notification (see the engine's notify.go). */
@Serializable
data class Notice(
    val space: String = "",
    val family: String = "",
    val kind: String = "",
    val title: String = "",
    val body: String = "",
    val preview: String = "",
    val item: String = "",
    val time: String = "",
)

@Serializable
data class ToneIssue(val kind: String, val match: String, val suggestion: String)

@Serializable
data class ToneResult(val issues: List<ToneIssue> = emptyList(), val level: String = "calm", val calmer: String = "", val meter: Float = 0f)

/** What a member may do in a family (set by the parent who invited them). */
@Serializable
data class Perms(
    val see: List<String> = emptyList(),
    val add: List<String> = emptyList(),
    val message: String = "nobody",
    @SerialName("message_with") val messageWith: List<String> = emptyList(),
    val call: String = "nobody",
    @SerialName("call_with") val callWith: List<String> = emptyList(),
    val video: Boolean = false,
    @SerialName("family_call") val familyCall: Boolean = false,
) {
    companion object {
        val SECTIONS = listOf("messages", "calendar", "expenses", "documents", "info")
        fun sectionLabel(s: String) = when (s) {
            "messages" -> "Family chat"; "calendar" -> "Calendar"; "expenses" -> "Expenses"; "documents" -> "Documents"; "info" -> "Info bank"; else -> s
        }
        /** The defaults for a role (the same as the server's). */
        fun defaults(role: String): Perms = when (role) {
            "parent" -> Perms(SECTIONS, SECTIONS, "anyone", call = "anyone", video = true, familyCall = true)
            "family" -> Perms(listOf("messages", "calendar", "documents", "info"), listOf("messages", "calendar", "documents", "info"), "anyone", call = "anyone", video = true)
            "child" -> Perms(listOf("messages", "calendar"), emptyList(), "parents", call = "parents", video = true)
            "professional" -> Perms(SECTIONS, emptyList(), "nobody", call = "nobody")
            else -> Perms()
        }
    }
}

/** A private conversation inside a family. */
@Serializable
data class Conversation(
    val id: String,
    val family: String = "",
    val title: String = "",
    val with: List<Member> = emptyList(),
    @SerialName("last_text") val lastText: String = "",
    @SerialName("last_time") val lastTime: String = "",
    val unread: Int = 0,
    @SerialName("can_write") val canWrite: Boolean = false,
    @SerialName("can_call") val canCall: Boolean = false,
    val video: Boolean = false,
    val waiting: Boolean = false,
)

/** Someone this person may message privately and/or call. */
@Serializable
data class Contact(val person: String, val name: String, val role: String = "", val message: Boolean = false, val call: Boolean = false)

/** This person's account: whether the server's administrator has let them start another family. */
@Serializable
data class Account(
    @SerialName("families_left") val familiesLeft: Int = 0,
    @SerialName("can_start_family") val canStartFamily: Boolean = false,
)

/** An answer from the built-in help. */
@Serializable
data class HelpAnswer(val title: String, val text: String, val source: String = "", val section: String = "", val score: Double = 0.0)

@Serializable
data class ReportCheck(
    val valid: Boolean = false,
    val problem: String = "",
    val id: String = "",
    val family: String = "",
    val from: String = "",
    val to: String = "",
    @SerialName("by_name") val byName: String = "",
    val signer: String = "",
    val generated: String = "",
    @SerialName("audit_seq") val auditSeq: Long = 0,
    @SerialName("audit_hash") val auditHash: String = "",
    val sha3: String = "",
    @SerialName("chain_match") val chainMatch: String = "",
)
