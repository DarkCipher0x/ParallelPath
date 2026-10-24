package systems.darkcipher.parallelpath.engine

import kotlinx.coroutines.flow.Flow
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.encodeToString
import systems.darkcipher.parallelpath.model.*

/**
 * How an app reaches the Go engine. Android: the gomobile library in-process. Windows: the ppengine process over
 * private pipes. Either way the engine does all cryptography; this side only sees plain results.
 */
interface EngineTransport {
    /** Runs an engine method. Returns the JSON result, or throws [EngineException] with a message safe to show. */
    suspend fun call(method: String, argsJson: String): String

    /** Engine events: "changed", "connection", "error". */
    val events: Flow<EngineEvent>
}

data class EngineEvent(val name: String, val data: JsonElement)

class EngineException(message: String) : Exception(message)

/** Typed access to the engine. One instance per app. */
class Engine(private val t: EngineTransport) {
    @OptIn(ExperimentalSerializationApi::class)
    val json = Json {
        ignoreUnknownKeys = true
        coerceInputValues = true // Go encodes empty lists as null
        explicitNulls = false
        encodeDefaults = true // the engine must see every field, e.g. a 50% share or GBP (the model defaults)
    }

    val events: Flow<EngineEvent> get() = t.events

    private suspend fun raw(method: String, args: JsonObject = JsonObject(emptyMap())): JsonElement {
        val res = t.call(method, args.toString())
        return if (res.isBlank()) JsonNull else json.parseToJsonElement(res)
    }

    private suspend inline fun <reified T> get(method: String, args: JsonObject = JsonObject(emptyMap())): T =
        json.decodeFromJsonElement(raw(method, args))

    suspend fun status(): Status = get("status")
    suspend fun preview(server: String): ServerPreview = get("preview", buildJsonObject { put("server", server) })

    suspend fun setup(
        server: String, certSha256: String, invite: String, passphrase: String,
        yourName: String, deviceName: String, deviceKind: String, familyName: String?, deviceSecret: String? = null,
    ): SetupResult = get("setup", buildJsonObject {
        put("server", server); put("cert_sha256", certSha256); put("invite", invite.trim()); put("passphrase", passphrase)
        put("your_name", yourName); put("device_name", deviceName); put("device_kind", deviceKind)
        if (!familyName.isNullOrBlank()) put("family_name", familyName)
        if (deviceSecret != null) put("device_secret", deviceSecret)
    })

    suspend fun unlock(passphrase: String, deviceSecret: String? = null) {
        raw("unlock", buildJsonObject { put("passphrase", passphrase); if (deviceSecret != null) put("device_secret", deviceSecret) })
    }

    suspend fun lock() { raw("lock") }
    suspend fun sync() { raw("sync") }

    suspend fun spaces(): List<Space> = get("spaces")
    suspend fun account(): Account = get("account")
    suspend fun createSpace(name: String): String =
        raw("create_space", buildJsonObject { put("name", name) }).jsonObject["space"]!!.jsonPrimitive.content
    suspend fun renameSpace(space: String, name: String) { raw("rename_space", buildJsonObject { put("space", space); put("name", name) }) }
    suspend fun setMyName(space: String, name: String) { raw("set_my_name", buildJsonObject { put("space", space); put("name", name) }) }

    /**
     * [historyFromIso]: professionals only - they see the history from then on (null = all of it). [perms]: what they
     * may do (null = the role's defaults); only this parent can change them later.
     */
    suspend fun invite(space: String, role: String, notAfterIso: String?, historyFromIso: String? = null, perms: Perms? = null): InviteCode =
        get("invite", buildJsonObject {
            put("space", space); put("role", role)
            put("not_after", notAfterIso ?: "0001-01-01T00:00:00Z")
            put("history_from", historyFromIso ?: "0001-01-01T00:00:00Z")
            if (perms != null) put("perms", json.parseToJsonElement(json.encodeToString(perms)))
        })

    /** Changes what someone this parent invited may do (recorded in the audit log). */
    suspend fun setPermissions(space: String, person: String, perms: Perms) {
        raw("set_permissions", buildJsonObject {
            put("space", space); put("person", person); put("perms", json.parseToJsonElement(json.encodeToString(perms)))
        })
    }

    /** The people in a family this person may message privately and/or call. */
    suspend fun contacts(space: String): List<Contact> = get("contacts", buildJsonObject { put("space", space) })

    /** This person's private conversations in a family. */
    suspend fun conversations(space: String): List<Conversation> = get("conversations", buildJsonObject { put("space", space) })

    /** Opens (or finds) the private conversation with exactly these people; returns its ID. */
    suspend fun startConversation(space: String, people: List<String>): String =
        raw("start_conversation", buildJsonObject {
            put("space", space); putJsonArray("people") { people.forEach { add(JsonPrimitive(it)) } }
        }).jsonObject["space"]!!.jsonPrimitive.content

    /** Answers from the built-in help (ParallelPath's own guide and FAQ; on this device). */
    suspend fun helpAsk(question: String): List<HelpAnswer> = get("help_ask", buildJsonObject { put("question", question) })
    suspend fun setChildrenChat(space: String, on: Boolean) { raw("set_children_chat", buildJsonObject { put("space", space); put("on", on) }) }
    suspend fun setTimeZone(zone: String) { raw("set_time_zone", buildJsonObject { put("zone", zone) }) }

    /** The tone check of a draft (on this device; nothing is sent). */
    suspend fun checkTone(text: String): ToneResult = get("check_tone", buildJsonObject { put("text", text) })
    suspend fun join(invite: String): SetupResult = get("join", buildJsonObject { put("invite", invite.trim()) })
    suspend fun safetyNumber(person: String): String =
        raw("safety_number", buildJsonObject { put("person", person) }).jsonObject["number"]!!.jsonPrimitive.content
    suspend fun approve(space: String, person: String) { raw("approve", buildJsonObject { put("space", space); put("person", person) }) }
    suspend fun remove(space: String, person: String) { raw("remove", buildJsonObject { put("space", space); put("person", person) }) }

    suspend fun messages(space: String): List<Message> = get("messages", buildJsonObject { put("space", space) })
    suspend fun send(space: String, text: String, replyTo: String?, files: List<FileInput>) {
        raw("send", buildJsonObject {
            put("space", space); put("text", text)
            if (replyTo != null) put("reply_to", replyTo)
            putJsonArray("files") { files.forEach { add(json.parseToJsonElement(json.encodeToString(it))) } }
        })
    }
    suspend fun react(space: String, item: String, emoji: String, remove: Boolean) {
        raw("react", buildJsonObject { put("space", space); put("item", item); put("emoji", emoji); put("remove", remove) })
    }
    suspend fun edit(space: String, item: String, text: String, reason: String) {
        raw("edit", buildJsonObject { put("space", space); put("item", item); put("text", text); put("reason", reason) })
    }
    suspend fun retract(space: String, item: String, reason: String) {
        raw("retract", buildJsonObject { put("space", space); put("item", item); put("reason", reason) })
    }
    suspend fun markRead(space: String, item: String) { raw("mark_read", buildJsonObject { put("space", space); put("item", item) }) }
    suspend fun openAttachment(space: String, item: String, blob: String): String =
        raw("open_attachment", buildJsonObject { put("space", space); put("item", item); put("blob", blob) })
            .jsonObject["path"]!!.jsonPrimitive.content

    suspend fun audit(space: String): AuditLog = get("audit", buildJsonObject { put("space", space) })

    // ---- calendar: events, parenting-time schedules, change requests, answers, check-ins

    suspend fun calendar(space: String, from: String, to: String, zone: String): CalendarView =
        get("calendar", buildJsonObject { put("space", space); put("from", from); put("to", to); put("zone", zone) })

    suspend fun scheduleTemplates(): List<ScheduleTemplate> = get("schedule_templates")

    /** Adds an event, or (with [item] and [reason]) edits one of yours. */
    suspend fun saveEvent(space: String, data: EventData, notes: String, item: String? = null, reason: String? = null): String =
        raw("save_event", buildJsonObject {
            put("space", space); put("notes", notes)
            put("data", json.parseToJsonElement(json.encodeToString(data)))
            if (item != null) put("item", item)
            if (reason != null) put("reason", reason)
        }).jsonObject["item"]!!.jsonPrimitive.content

    suspend fun cancelEvent(space: String, item: String, reason: String) {
        raw("cancel_event", buildJsonObject { put("space", space); put("item", item); put("reason", reason) })
    }

    suspend fun proposeSchedule(
        space: String, template: String, parentA: String, parentB: String, start: String, exchange: String, zone: String,
        note: String, item: String? = null, reason: String? = null, cycle: List<Segment> = emptyList(),
    ): String = raw("propose_schedule", buildJsonObject {
        put("space", space); put("template", template); put("parent_a", parentA); put("parent_b", parentB)
        put("start", start); put("exchange", exchange); put("zone", zone); put("note", note)
        if (cycle.isNotEmpty()) put("cycle", json.parseToJsonElement(json.encodeToString(cycle)))
        if (item != null) put("item", item)
        if (reason != null) put("reason", reason)
    }).jsonObject["item"]!!.jsonPrimitive.content

    suspend fun requestChange(space: String, changes: List<Change>, note: String, item: String? = null, reason: String? = null): String =
        raw("request_change", buildJsonObject {
            put("space", space); put("note", note)
            putJsonArray("changes") { changes.forEach { add(json.parseToJsonElement(json.encodeToString(it))) } }
            if (item != null) put("item", item)
            if (reason != null) put("reason", reason)
        }).jsonObject["item"]!!.jsonPrimitive.content

    suspend fun withdraw(space: String, item: String, reason: String) {
        raw("withdraw", buildJsonObject { put("space", space); put("item", item); put("reason", reason) })
    }

    suspend fun answer(space: String, item: String, accept: Boolean, comment: String) {
        raw("answer", buildJsonObject { put("space", space); put("item", item); put("accept", accept); put("comment", comment) })
    }

    suspend fun checkIn(space: String, place: String, note: String, lat: Double?, lon: Double?, accuracy: Double) {
        raw("check_in", buildJsonObject {
            put("space", space); put("place", place); put("note", note)
            if (lat != null && lon != null) { put("lat", lat); put("lon", lon); put("accuracy", accuracy) }
        })
    }

    private fun fileJson(fs: List<FileInput>) = json.parseToJsonElement(json.encodeToString(fs))
    private fun itemOf(e: JsonElement) = e.jsonObject["item"]!!.jsonPrimitive.content

    // ---- expenses

    suspend fun expenses(space: String): ExpensesView = get("expenses", buildJsonObject { put("space", space) })

    /** Records an expense you paid, or (with [item] and [reason]) changes one nobody has paid yet. */
    suspend fun saveExpense(space: String, data: ExpenseData, note: String, receipts: List<FileInput>, item: String? = null, reason: String? = null): String =
        itemOf(raw("save_expense", buildJsonObject {
            put("space", space); put("note", note)
            put("data", json.parseToJsonElement(json.encodeToString(data)))
            if (receipts.isNotEmpty()) put("receipts", fileJson(receipts))
            if (item != null) put("item", item)
            if (reason != null) put("reason", reason)
        }))

    suspend fun withdrawExpense(space: String, item: String, reason: String) {
        raw("withdraw_expense", buildJsonObject { put("space", space); put("item", item); put("reason", reason) })
    }

    suspend fun answerExpense(space: String, item: String, accept: Boolean, comment: String) {
        raw("answer_expense", buildJsonObject { put("space", space); put("item", item); put("accept", accept); put("comment", comment) })
    }

    suspend fun payExpense(space: String, item: String, method: String, reference: String, note: String) {
        raw("pay_expense", buildJsonObject {
            put("space", space); put("item", item); put("method", method); put("reference", reference); put("note", note)
        })
    }

    suspend fun confirmPayment(space: String, item: String, person: String) {
        raw("confirm_payment", buildJsonObject { put("space", space); put("item", item); put("person", person) })
    }

    // ---- documents

    suspend fun documents(space: String): List<Doc> = get("documents", buildJsonObject { put("space", space) })

    /** Adds a folder or file, or (with [item] and [reason]) renames, moves or replaces one of yours. */
    suspend fun saveDocument(
        space: String, type: String, folder: String, name: String, note: String, file: FileInput?, item: String? = null, reason: String? = null,
    ): String = itemOf(raw("save_document", buildJsonObject {
        put("space", space); put("type", type); put("folder", folder); put("name", name); put("note", note)
        if (file != null) put("file", json.parseToJsonElement(json.encodeToString(file)))
        if (item != null) put("item", item)
        if (reason != null) put("reason", reason)
    }))

    suspend fun removeDocument(space: String, item: String, reason: String) {
        raw("remove_document", buildJsonObject { put("space", space); put("item", item); put("reason", reason) })
    }

    /** Renames and/or moves a file, or a folder with everything in it ([path] for a folder without an item of its own). */
    suspend fun moveDocument(space: String, item: String, path: String, toFolder: String, name: String, reason: String) {
        raw("move_document", buildJsonObject {
            put("space", space); put("item", item); put("path", path); put("to_folder", toFolder); put("name", name); put("reason", reason)
        })
    }

    /** Removes files and folders (with what's in them) - yours only, with one reason. */
    suspend fun removeDocuments(space: String, items: List<String>, folders: List<String>, reason: String) {
        raw("remove_documents", buildJsonObject {
            put("space", space); put("reason", reason)
            putJsonArray("items") { items.forEach { add(JsonPrimitive(it)) } }
            putJsonArray("folders") { folders.forEach { add(JsonPrimitive(it)) } }
        })
    }

    suspend fun restoreDocument(space: String, item: String, reason: String) {
        raw("restore_document", buildJsonObject { put("space", space); put("item", item); put("reason", reason) })
    }

    /** Copies a file or folder into a folder here, or into another family ([toSpace]). */
    suspend fun copyDocument(space: String, item: String, path: String, toSpace: String, toFolder: String) {
        raw("copy_document", buildJsonObject {
            put("space", space); put("item", item); put("path", path); put("to_space", toSpace); put("to_folder", toFolder)
        })
    }

    suspend fun sharedFiles(space: String): List<SharedFile> = get("shared_files", buildJsonObject { put("space", space) })

    /** Files a copy of a file shared in a message ([from]: the family or a conversation in it) into the documents. */
    suspend fun saveSharedFile(space: String, from: String, message: String, blob: String, toFolder: String): String =
        itemOf(raw("save_shared_file", buildJsonObject {
            put("space", space); put("from", from); put("message", message); put("blob_id", blob); put("to_folder", toFolder)
        }))

    // ---- info bank

    suspend fun infoBank(space: String): InfoView = get("info_bank", buildJsonObject { put("space", space) })

    /** Adds a child or entry, or (with [replaces] and [why]) updates or removes the current version of one. */
    suspend fun saveInfo(
        space: String, type: String, child: String, section: String, label: String, value: String, note: String,
        files: List<FileInput> = emptyList(), replaces: String? = null, why: String? = null, remove: Boolean = false,
    ): String = itemOf(raw("save_info", buildJsonObject {
        put("space", space); put("type", type); put("child", child); put("section", section); put("label", label)
        put("value", value); put("note", note)
        if (files.isNotEmpty()) put("files", fileJson(files))
        if (replaces != null) put("replaces", replaces)
        if (why != null) put("why", why)
        if (remove) put("remove", true)
    }))

    // ---- journal (private to this person)

    suspend fun journal(family: String): List<JournalEntry> = get("journal", buildJsonObject { put("family", family) })

    suspend fun saveJournal(
        family: String, text: String, date: String, mood: String, tags: List<String>, files: List<FileInput>,
        item: String? = null, reason: String? = null,
    ): String = itemOf(raw("save_journal", buildJsonObject {
        put("family", family); put("text", text); put("date", date); put("mood", mood)
        putJsonArray("tags") { tags.forEach { add(JsonPrimitive(it)) } }
        if (files.isNotEmpty()) put("files", fileJson(files))
        if (item != null) put("item", item)
        if (reason != null) put("reason", reason)
    }))

    suspend fun removeJournal(item: String, reason: String) {
        raw("remove_journal", buildJsonObject { put("item", item); put("reason", reason) })
    }

    suspend fun openJournalFile(item: String, blob: String): String =
        raw("open_journal_file", buildJsonObject { put("item", item); put("blob", blob) }).jsonObject["path"]!!.jsonPrimitive.content

    suspend fun shareJournal(family: String, item: String) {
        raw("share_journal", buildJsonObject { put("family", family); put("item", item) })
    }

    // ---- your other devices, and getting back in

    /** A one-time code (L-...) for linking another device of yours. */
    suspend fun linkStart(): String = raw("link_start").jsonObject["code"]!!.jsonPrimitive.content

    /** Null while no device has asked yet; otherwise its name and kind. */
    suspend fun linkCheck(code: String): Pair<String, String>? {
        val o = raw("link_check", buildJsonObject { put("code", code) }).jsonObject
        if (o["waiting"]?.jsonPrimitive?.content == "true") return null
        return (o["name"]?.jsonPrimitive?.content ?: "") to (o["kind"]?.jsonPrimitive?.content ?: "")
    }

    suspend fun linkConfirm(code: String) { raw("link_confirm", buildJsonObject { put("code", code) }) }

    /** Sets this device up as another device of an existing person (waits for them to confirm on the other one). */
    suspend fun setupLinked(server: String, certSha256: String, code: String, passphrase: String, deviceName: String, deviceKind: String,
        deviceSecret: String? = null): SetupResult = get("setup_linked", buildJsonObject {
        put("server", server); put("cert_sha256", certSha256); put("code", code.trim()); put("passphrase", passphrase)
        put("device_name", deviceName); put("device_kind", deviceKind)
        if (deviceSecret != null) put("device_secret", deviceSecret)
    })

    /** Makes (or replaces) the recovery kit; returns the code (R-...) to write down. */
    suspend fun recoveryCreate(): String = raw("recovery_create").jsonObject["code"]!!.jsonPrimitive.content

    /** When the recovery kit was made, or null if there isn't one. */
    suspend fun recoveryMade(): String? = raw("recovery_status").jsonObject.let { o ->
        if (o["has_kit"]?.jsonPrimitive?.content == "true") o["made"]?.jsonPrimitive?.content ?: "" else null
    }

    suspend fun setupRecover(server: String, certSha256: String, code: String, passphrase: String, deviceName: String, deviceKind: String,
        deviceSecret: String? = null): SetupResult = get("setup_recover", buildJsonObject {
        put("server", server); put("cert_sha256", certSha256); put("code", code.trim()); put("passphrase", passphrase)
        put("device_name", deviceName); put("device_kind", deviceKind)
        if (deviceSecret != null) put("device_secret", deviceSecret)
    })

    // ---- court-ready reports (made and signed on this device)

    suspend fun report(space: String, from: String, to: String, zone: String, sections: List<String>): ReportFiles =
        get("report", buildJsonObject {
            put("space", space); put("from", from); put("to", to); put("zone", zone)
            putJsonArray("sections") { sections.forEach { add(JsonPrimitive(it)) } }
        })

    suspend fun verifyReport(path: String): ReportCheck = get("verify_report", buildJsonObject { put("path", path) })

    // ---- calls (the call page does the rest through the engine's call bridge)

    /** Starts a call to the rest of the family and returns its ID. */
    suspend fun callStart(space: String, video: Boolean): String =
        raw("call_start", buildJsonObject { put("space", space); put("video", video) }).jsonObject["call"]!!.jsonPrimitive.content

    /** The call page's address on this device. */
    suspend fun callBridge(): String = raw("call_bridge").jsonObject["url"]!!.jsonPrimitive.content

    suspend fun callDecline(space: String, call: String) {
        raw("call_decline", buildJsonObject { put("space", space); put("call", call) })
    }

    /** Tells the caller this device is busy on another call. */
    suspend fun callBusy(space: String, call: String) {
        raw("call_signal", buildJsonObject { put("space", space); put("call", call); put("type", "busy") })
    }
}

/** Reads the "message" of an engine error event. */
fun EngineEvent.message(): String =
    (data as? JsonObject)?.get("message")?.let { (it as? JsonPrimitive)?.content } ?: ""

/** The notice in a "notify" event (null for other events). */
fun EngineEvent.notice(json: Json): Notice? =
    if (name == "notify") runCatching { json.decodeFromJsonElement<Notice>(data) }.getOrNull() else null

/** True if a "connection" event says the engine is online. */
fun EngineEvent.online(): Boolean =
    (data as? JsonObject)?.get("online")?.let { (it as? JsonPrimitive)?.content == "true" } ?: false
