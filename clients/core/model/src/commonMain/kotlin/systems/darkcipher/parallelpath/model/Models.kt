package systems.darkcipher.parallelpath.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// Plain data, exactly as the engine returns it (engine/app/*.go). Byte arrays arrive as base64 strings;
// times as RFC 3339 strings (formatted for display by the design system).

@Serializable
data class Status(
    @SerialName("set_up") val setUp: Boolean = false,
    val unlocked: Boolean = false,
    val person: String = "",
    val device: String = "",
    @SerialName("server_name") val serverName: String = "",
    @SerialName("server_url") val serverUrl: String = "",
    val online: Boolean = false,
    val crypto: String = "",
)

@Serializable
data class ServerPreview(
    val url: String,
    val name: String = "",
    val version: String = "",
    @SerialName("cert_sha256") val certSha256: String = "",
    @SerialName("trusted_by_ca") val trustedByCa: Boolean = false,
    @SerialName("key_exchange") val keyExchange: String = "",
    @SerialName("server_key_fingerprint") val serverKeyFingerprint: String = "",
)

@Serializable
data class SetupResult(
    val person: String = "",
    @SerialName("space_id") val spaceId: String = "",
    val role: String = "",
    @SerialName("waiting_for_approval") val waiting: Boolean = false,
)

@Serializable
data class Member(
    val person: String,
    val name: String,
    val role: String,
    val status: String,
    val me: Boolean = false,
    @SerialName("read_only") val readOnly: Boolean = false,
    @SerialName("not_after") val notAfter: String? = null,
    @SerialName("needs_approval") val needsApproval: Boolean = false,
    val joined: String = "",
    @SerialName("history_from") val historyFrom: String? = null,
    val perms: Perms? = null,
    @SerialName("invited_by") val invitedBy: String = "",
)

@Serializable
data class Space(
    val id: String,
    val name: String,
    @SerialName("my_role") val myRole: String = "",
    @SerialName("read_only") val readOnly: Boolean = false,
    @SerialName("waiting_for_approval") val waiting: Boolean = false,
    val members: List<Member> = emptyList(),
    val unread: Int = 0,
    @SerialName("last_text") val lastText: String = "",
    @SerialName("last_time") val lastTime: String = "",
    @SerialName("audit_problem") val auditProblem: String = "",
    @SerialName("children_chat") val childrenChat: Boolean = false,
    @SerialName("can_chat") val canChat: Boolean = true,
) {
    val isParent get() = myRole == "parent"
    val pending get() = members.filter { it.needsApproval && !it.me }
}

@Serializable
data class Revision(
    val rev: Long,
    val time: String,
    val text: String = "",
    val reason: String = "",
    val retracted: Boolean = false,
)

@Serializable
data class Reaction(val emoji: String, val count: Int, val mine: Boolean = false, val people: List<String> = emptyList())

@Serializable
data class Receipt(val person: String, val name: String, val time: String)

@Serializable
data class Attachment(
    @SerialName("blob_id") val blobId: String,
    val name: String,
    val mime: String,
    val size: Long,
    val width: Int = 0,
    val height: Int = 0,
    val millis: Long = 0,
    val thumb: String? = null, // base64 JPEG
) {
    val isImage get() = mime.startsWith("image/")
    val isVideo get() = mime.startsWith("video/")
}

@Serializable
data class Message(
    val id: String,
    val rev: Long = 0,
    val author: String,
    @SerialName("author_name") val authorName: String,
    val mine: Boolean = false,
    val sent: String,
    val text: String = "",
    @SerialName("reply_to") val replyTo: String? = null,
    val edited: Boolean = false,
    val retracted: Boolean = false,
    val reason: String = "",
    val history: List<Revision> = emptyList(),
    val attachments: List<Attachment> = emptyList(),
    val reactions: List<Reaction> = emptyList(),
    @SerialName("read_by") val readBy: List<Receipt> = emptyList(),
    val call: CallInfo? = null,
    val locked: Boolean = false,
    val problem: String = "",
)

@Serializable
data class AuditRow(
    val seq: Long,
    val time: String,
    @SerialName("client_time") val clientTime: String = "",
    val person: String,
    val name: String,
    val device: String,
    val action: String,
    val what: String,
    @SerialName("item_id") val itemId: String? = null,
    @SerialName("item_rev") val itemRev: Long = 0,
)

@Serializable
data class AuditLog(
    val rows: List<AuditRow> = emptyList(),
    val verified: Boolean = false,
    @SerialName("head_seq") val headSeq: Long = 0,
    @SerialName("head_hash") val headHash: String = "",
    val problem: String = "",
)

@Serializable
data class InviteCode(val code: String, val expires: String)

/** A file the person picked to send, with anything the platform already knows about it. */
@Serializable
data class FileInput(
    val path: String,
    val name: String? = null,
    val mime: String? = null,
    val thumb: String? = null,
    val width: Int = 0,
    val height: Int = 0,
    val millis: Long = 0,
)

object Roles {
    const val PARENT = "parent"
    const val FAMILY = "family"
    const val PROFESSIONAL = "professional"
    const val CHILD = "child"

    fun label(role: String) = when (role) {
        PARENT -> "Parent"
        FAMILY -> "Family member"
        PROFESSIONAL -> "Professional (read-only)"
        CHILD -> "Child"
        else -> role
    }
}
