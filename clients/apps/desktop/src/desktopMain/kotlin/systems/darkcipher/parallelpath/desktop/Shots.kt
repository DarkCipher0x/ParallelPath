package systems.darkcipher.parallelpath.desktop

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Modifier
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.pointer.PointerEventType
import androidx.compose.ui.ImageComposeScene
import androidx.compose.ui.unit.Density
import kotlinx.coroutines.runBlocking
import org.jetbrains.skia.EncodedImageFormat
import systems.darkcipher.parallelpath.App
import systems.darkcipher.parallelpath.UiPrefs
import systems.darkcipher.parallelpath.calendar.CalendarScreen
import systems.darkcipher.parallelpath.ui.ToneDialog
import systems.darkcipher.parallelpath.expenses.ExpensesScreen
import systems.darkcipher.parallelpath.files.DocumentsScreen
import systems.darkcipher.parallelpath.records.InfoBankScreen
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Change
import systems.darkcipher.parallelpath.model.EventData
import systems.darkcipher.parallelpath.model.Repeat
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.ParallelPathTheme
import systems.darkcipher.parallelpath.ui.ThemeMode
import systems.darkcipher.parallelpath.ui.isoDaysFromNow
import systems.darkcipher.parallelpath.ui.pp
import java.io.File

/**
 * Development tool: renders the real app (real engine, real vault) offscreen to PNGs, for docs and for checking
 * the UI without driving a desktop. Only ever used with the fictional ppdemo family.
 *
 *   Shots <ppengine.exe> <engine data dir> <passphrase> <out dir> [fresh engine data dir]
 */
fun main(args: Array<String>) {
    val (exe, data, pass, out) = args
    File(out).mkdirs()
    val transport = ProcessTransport(File(exe), File(data))
    val engine = Engine(transport)
    runBlocking {
        engine.unlock(pass)
        repeat(100) {
            val s = engine.spaces()
            if (s.isNotEmpty() && engine.messages(s.first().id).size >= 6 &&
                runCatching { engine.calendar(s.first().id, "2000-01-01", "2000-01-01", "UTC").schedules.isNotEmpty() }.getOrDefault(false)
            ) return@runBlocking
            Thread.sleep(100)
        }
    }
    val platform = DesktopPlatform { null }
    // clicks: points to click once the screen has settled (to open a dialog, say).
    // rightClick: a right-click after the clicks (to show a context menu).
    fun shot(name: String, w: Int = 1280, h: Int = 820, clicks: List<Offset> = emptyList(), rightClick: Offset? = null, content: @Composable () -> Unit) {
        val scene = ImageComposeScene(w, h, Density(1f), content = content)
        var t = 0L
        fun settle() = repeat(80) {
            scene.render(t)
            t += 50_000_000
            Thread.sleep(40)
        }
        settle()
        clicks.forEach { p ->
            scene.sendPointerEvent(PointerEventType.Press, p)
            scene.sendPointerEvent(PointerEventType.Release, p)
            settle()
        }
        rightClick?.let { p ->
            scene.sendPointerEvent(PointerEventType.Press, p, buttons = androidx.compose.ui.input.pointer.PointerButtons(isSecondaryPressed = true),
                button = androidx.compose.ui.input.pointer.PointerButton.Secondary)
            scene.sendPointerEvent(PointerEventType.Release, p, button = androidx.compose.ui.input.pointer.PointerButton.Secondary)
            settle()
        }
        val img = scene.render(t)
        File(out, "$name.png").writeBytes(img.encodeToData(EncodedImageFormat.PNG)!!.bytes)
        scene.close()
        println("wrote $name.png")
    }
    shot("01-chat-dark") { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Dark), {}) }
    shot("02-chat-light") { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Light), {}) }
    shot("03-family") { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Dark), {}, startTab = 6) }
    shot("04-audit") { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Dark), {}, startTab = 7) }
    shot("05-phone", 412, 900) { App(engine, platform, "android", "Phone", UiPrefs(ThemeMode.Dark), {}) }
    shot("06-calendar") { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Dark), {}, startTab = 1) }
    shot("07-calendar-light") { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Light), {}, startTab = 1) }
    val space = runBlocking { engine.spaces().first() }
    fun calendar(mode: Int, light: Boolean = false): @Composable () -> Unit = {
        ParallelPathTheme(if (light) ThemeMode.Light else ThemeMode.Dark) {
            CompositionLocalProvider(LocalPlatform provides platform) {
                Surface(Modifier.fillMaxSize(), color = pp.bg) { CalendarScreen(engine, space, 0, {}, startMode = mode) }
            }
        }
    }
    shot("08-requests", content = calendar(1))
    // The month on a day without a check-in (a check-in shows a location: never on the website).
    shot("43-calendar-month") {
        ParallelPathTheme(ThemeMode.Dark) {
            CompositionLocalProvider(LocalPlatform provides platform) {
                Surface(Modifier.fillMaxSize(), color = pp.bg) {
                    CalendarScreen(engine, space, 0, {}, startDate = java.time.LocalDate.now().withDayOfMonth(1).plusMonths(1).toString())
                }
            }
        }
    }
    shot("09-schedule", content = calendar(2))
    shot("10-calendar-phone", 412, 900, content = calendar(0))
    shot("12-add-event", clicks = listOf(Offset(961f, 282f))) { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Dark), {}, startTab = 1) }
    shot("13-ask-change", clicks = listOf(Offset(1094f, 282f))) { App(engine, platform, "windows", "PC", UiPrefs(ThemeMode.Dark), {}, startTab = 1) }
    shot("14-decline", clicks = listOf(Offset(169f, 225f)), content = calendar(1))
    shot("15-new-schedule", clicks = listOf(Offset(132f, 219f)), content = calendar(2))
    fun themed(light: Boolean = false, content: @Composable () -> Unit): @Composable () -> Unit = {
        ParallelPathTheme(if (light) ThemeMode.Light else ThemeMode.Dark) {
            CompositionLocalProvider(LocalPlatform provides platform) { Surface(Modifier.fillMaxSize(), color = pp.bg) { content() } }
        }
    }
    fun tab(n: Int, light: Boolean = false): @Composable () -> Unit =
        { App(engine, platform, "windows", "PC", UiPrefs(if (light) ThemeMode.Light else ThemeMode.Dark), {}, startTab = n) }
    val report = runBlocking { engine.documents(space.id).first { it.name.startsWith("Autumn") }.id }
    shot("16-expenses", content = tab(2))
    shot("17-documents", content = themed { DocumentsScreen(engine, space, 0, {}, startFolder = "School", startSelected = report) })
    shot("37-explorer-root", 1440, 860, content = themed { DocumentsScreen(engine, space, 0, {}) })
    shot("38-explorer-menu", 1440, 860, rightClick = Offset(420f, 128f), content = themed { DocumentsScreen(engine, space, 0, {}, startFolder = "School") })
    shot("39-explorer-background-menu", 1440, 860, rightClick = Offset(700f, 600f), content = themed { DocumentsScreen(engine, space, 0, {}, startFolder = "School") })
    shot("42-explorer-click", 1440, 860, clicks = listOf(Offset(420f, 140f)), content = themed { DocumentsScreen(engine, space, 0, {}, startFolder = "School") })
    shot("40-explorer-phone", 412, 900, content = themed { DocumentsScreen(engine, space, 0, {}, startFolder = "School") })
    shot("41-explorer-light", 1440, 860, content = themed(light = true) { DocumentsScreen(engine, space, 0, {}, startFolder = "School", startSelected = report) })
    shot("18-info-bank", content = tab(4))
    shot("19-journal", content = tab(5))
    shot("20-expenses-light", content = tab(2, light = true))
    shot("21-expenses-phone", 412, 900, content = themed { ExpensesScreen(engine, space, 0, {}) })
    shot("22-info-phone", 412, 900, content = themed { InfoBankScreen(engine, space, 0, {}) })
    shot("23-add-expense", clicks = listOf(Offset(1085f, 161f)), content = tab(2))
    shot("24-add-detail", clicks = listOf(Offset(425f, 281f)), content = tab(4))
    shot("25-new-journal", clicks = listOf(Offset(420f, 240f)), content = tab(5))
    shot("26-agree-expense-phone", 412, 900, clicks = listOf(Offset(75f, 501f)), content = themed { ExpensesScreen(engine, space, 0, {}) })
    shot("27-professional-invite", clicks = listOf(Offset(644f, 428f)), content = tab(6))
    shot("28-settings", clicks = listOf(Offset(243f, 38f)), content = tab(0))
    shot("29-custom-schedule", clicks = listOf(Offset(132f, 219f), Offset(739f, 305f)), content = calendar(2))
    val tone = runBlocking { engine.checkTone("Pick-up is at 5 on Friday. You ALWAYS do this, it's your fault, you useless idiot!!! Pay it or else.") }
    shot("31-help", clicks = listOf(Offset(194f, 38f)), content = tab(0))
    shot("32-call-picker", clicks = listOf(Offset(1009f, 30f)), content = tab(0))
    shot("33-messages-private", content = tab(0))
    shot("34-invite-child", clicks = listOf(Offset(739f, 428f), Offset(1048f, 489f)), content = tab(6))
    shot("30-tone-check", content = themed { ToneDialog(tone, canReplace = true, onEdit = {}, onCalmer = {}, onSend = {}) })
    shot("35-tone-bar", content = themed {
        androidx.compose.foundation.layout.Column(Modifier.padding(androidx.compose.ui.unit.Dp(24f)),
            verticalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(androidx.compose.ui.unit.Dp(18f))) {
            listOf("Could we swap to Saturday this week? Thanks.", "You never tell me anything. Why is it always late???",
                "Pick-up is at 5 on Friday. You ALWAYS do this, you useless idiot!!! Pay it or else.").forEach { t ->
                val r = runBlocking { engine.checkTone(t) }
                androidx.compose.material3.Text(t, color = systems.darkcipher.parallelpath.ui.pp.fgDim)
                systems.darkcipher.parallelpath.ui.ToneBar(r) {}
            }
        }
    })
    shot("36-advice", content = themed {
        androidx.compose.runtime.CompositionLocalProvider(systems.darkcipher.parallelpath.ui.LocalHelpAsk provides { q: String -> engine.helpAsk(q) }) {
            systems.darkcipher.parallelpath.ui.AdviceDialog(tone, onUseCalmer = {}) {}
        }
    })
    if (args.size > 4) {
        val fresh = Engine(ProcessTransport(File(exe), File(args[4])))
        shot("11-welcome") { App(fresh, platform, "windows", "PC", UiPrefs(ThemeMode.Dark), {}) }
    }
    calendarSmoke(engine, space.id)
    recordsSmoke(engine, space.id)
    newFeaturesSmoke(engine, space.id)
    runBlocking { engine.lock() }
    transport.close()
    kotlin.system.exitProcess(0)
}

/**
 * After the screenshots: drives every calendar call the screens make, with the same arguments they build, through
 * the real engine, and checks the results. (The engine's own logic is covered by the Go end-to-end test; this
 * checks the app's side of the conversation.)
 */
private fun calendarSmoke(engine: Engine, space: String) = runBlocking {
    val zone = java.time.ZoneId.systemDefault().id
    val day = java.time.LocalDate.now().plusDays(3)
    fun at(h: Int) = day.atTime(h, 0).atZone(java.time.ZoneId.systemDefault()).toInstant().toString()
    fun view() = runBlocking { engine.sync(); engine.calendar(space, day.minusDays(1).toString(), day.plusDays(1).toString(), zone) }
    fun check(what: String, ok: Boolean) {
        check(ok) { "calendar smoke: $what" }
        println("ok   $what")
    }

    val id = engine.saveEvent(space, EventData(title = "Swimming", start = at(16), end = at(17), zone = zone, category = "activity",
        children = listOf("Leo"), repeat = Repeat("weekly", day.plusWeeks(4).toString())), "Towel and goggles")
    check("an event is added", view().events.any { it.id == id && it.data.title == "Swimming" && it.notes == "Towel and goggles" })
    engine.saveEvent(space, EventData(title = "Swimming", start = at(17), end = at(18), zone = zone, category = "activity"), "", id, "Pool times changed")
    check("an edit keeps its reason", view().events.any { it.id == id && it.edited && it.history.lastOrNull()?.reason == "Pool times changed" })
    engine.cancelEvent(space, id, "Pool closed")
    check("a cancelled event stays, crossed out", view().events.any { it.id == id && it.cancelled && it.reason == "Pool closed" })

    val other = engine.spaces().first { it.id == space }.members.first { !it.me && it.role == "parent" }.person
    val req = engine.requestChange(space, listOf(Change(at(9), at(19), other)), "Could you have them for my work trip?")
    var p = view().changes.first { it.id == req }
    check("a change request waits for the other parent", p.state == "pending" && p.mine && p.waiting == listOf("Alex"))
    engine.withdraw(space, req, "Trip cancelled")
    p = view().changes.first { it.id == req }
    check("a withdrawn request says so", p.state == "withdrawn")

    val waiting = view().changes.first { it.needsMe }
    val declineErr = runCatching { engine.answer(space, waiting.id, false, "") }.exceptionOrNull()
    check("declining needs a reason", declineErr?.message?.contains("why") == true)
    engine.answer(space, waiting.id, false, "We're at the cinema already that morning")
    check("a decline is recorded", view().changes.first { it.id == waiting.id }.let { it.state == "declined" && !it.needsMe })

    val me = engine.spaces().first { it.id == space }.members.first { it.me }.person
    val sched = engine.proposeSchedule(space, "2-2-3", me, other, day.toString(), "17:30", zone, "Trying 2-2-3 for the new term")
    check("a schedule proposal is pending", view().schedules.any { it.id == sched && it.state == "pending" && it.schedule?.template == "2-2-3" })

    engine.checkIn(space, "Library", "Collected Leo", null, null, 0.0)
    val today = java.time.LocalDate.now().toString()
    check("a check-in without a location", engine.calendar(space, today, today, zone).checkins.any { it.mine && it.data.place == "Library" && it.data.lat == null })
    check("templates are listed", engine.scheduleTemplates().size >= 6)
}

/** Drives the records calls (expenses, documents, info bank, journal) the way the screens do, and checks them. */
private fun recordsSmoke(engine: Engine, space: String) = runBlocking {
    fun check(what: String, ok: Boolean) {
        check(ok) { "records smoke: $what" }
        println("ok   $what")
    }
    val tmp = kotlin.io.path.createTempDirectory("pp-smoke").toFile()
    fun file(name: String, text: String) = File(tmp, name).apply { writeText(text) }.path

    val today = java.time.LocalDate.now().toString()
    val x = engine.saveExpense(space, systems.darkcipher.parallelpath.model.ExpenseData("Swim kit", 2599, "GBP", today, "clothing", listOf("Mia"), 50),
        "Goggles too", listOf(systems.darkcipher.parallelpath.model.FileInput(file("kit.txt", "receipt"))))
    var ex = engine.expenses(space).expenses.first { it.id == x }
    check("an expense with a receipt waits for the other parent", ex.state == "pending" && ex.owed == 1299L && ex.receipts.size == 1 && ex.mine)
    engine.saveExpense(space, ex.data.copy(amount = 2999), "Goggles too", emptyList(), x, "Forgot the swim cap")
    ex = engine.expenses(space).expenses.first { it.id == x }
    check("a changed expense keeps its receipt and reason", ex.rev == 1L && ex.receipts.size == 1 && ex.reason == "Forgot the swim cap")
    val waiting = engine.expenses(space).expenses.first { it.needsMe }
    engine.answerExpense(space, waiting.id, true, "")
    check("agreeing to the other parent's expense lets you pay", engine.expenses(space).expenses.first { it.id == waiting.id }.canPay)
    engine.payExpense(space, waiting.id, "Cash", "", "Handed over at pick-up")
    check("a payment is recorded", engine.expenses(space).expenses.first { it.id == waiting.id }.state == "paid")
    engine.withdrawExpense(space, x, "Bought it together in the end")
    check("a withdrawn expense says so", engine.expenses(space).expenses.first { it.id == x }.state == "withdrawn")

    engine.saveDocument(space, "folder", "", "Holidays", "", null)
    val d = engine.saveDocument(space, "file", "Holidays", "", "Booking", systems.darkcipher.parallelpath.model.FileInput(file("booking.txt", "Cornwall")))
    var doc = engine.documents(space).first { it.id == d }
    check("a file goes in its folder", doc.folder == "Holidays" && doc.name == "booking.txt" && doc.file != null)
    val path = engine.openAttachment(space, d, doc.file!!.blobId)
    check("a file opens with its content", File(path).readText() == "Cornwall")
    engine.saveDocument(space, "file", "Holidays", "booking.txt", "Booking", systems.darkcipher.parallelpath.model.FileInput(file("booking.txt", "Cornwall, 2 weeks")),
        d, "Extended")
    engine.saveDocument(space, "file", "", "Cornwall booking.txt", "Booking", null, d, "Tidying up")
    doc = engine.documents(space).first { it.id == d }
    check("a new version and a move", doc.versions.size == 2 && doc.folder == "" && doc.name == "Cornwall booking.txt")
    engine.removeDocument(space, d, "Trip cancelled")
    check("a removed file is kept", engine.documents(space).first { it.id == d }.removed)

    val info = engine.infoBank(space)
    val mia = info.children.first { it.label == "Mia" }.chain
    val id = engine.saveInfo(space, "info", mia, "clothing", "Coat size", "8-9", "")
    val coat = engine.infoBank(space).entries.first { it.id == id }
    engine.saveInfo(space, "info", mia, "clothing", "Coat size", "9-10", "", replaces = coat.id, why = "Grew")
    val coat2 = engine.infoBank(space).entries.first { it.chain == coat.chain }
    check("an info update keeps history", coat2.value == "9-10" && coat2.history.size == 2 && coat2.history.last().why == "Grew")
    engine.saveInfo(space, "info", mia, "clothing", "Coat size", "9-10", "", replaces = coat2.id, why = "Not needed", remove = true)
    check("a removed entry moves to removed", engine.infoBank(space).removed.any { it.chain == coat.chain })

    val j = engine.saveJournal(space, "Quiet weekend", today, "good", listOf("weekend"), emptyList())
    check("a journal entry", engine.journal(space).any { it.id == j && it.data.mood == "good" && it.data.tags == listOf("weekend") })
    engine.saveJournal(space, "Quiet weekend, park on Sunday", today, "good", listOf("weekend"), emptyList(), j, "Added Sunday")
    check("an edited journal entry", engine.journal(space).first { it.id == j }.edited)
    engine.shareJournal(space, j)
    check("a shared entry appears in the messages", engine.messages(space).any { it.mine && it.text.contains("park on Sunday") })
    check("the journal isn't listed as a family", engine.spaces().size == 1)
    tmp.deleteRecursively()
}

/** Professional windows, rotation, reports and the rest, through the real engine with the arguments the screens build. */
private fun newFeaturesSmoke(engine: Engine, space: String) = runBlocking {
    fun check(what: String, ok: Boolean) {
        println((if (ok) "ok   " else "FAIL ") + what)
        if (!ok) kotlin.system.exitProcess(3)
    }
    check("a calm message passes the tone check", engine.checkTone("Can we swap Saturday for Sunday?").issues.isEmpty())
    val hot = engine.checkTone("You're useless. See you in court!!!")
    check("a hostile message is flagged", hot.level == "heated" && hot.issues.size >= 2)
    engine.setTimeZone(java.time.ZoneId.systemDefault().id)
    check("the time zone is accepted", true)
    val inv = engine.invite(space, "professional", isoDaysFromNow(60), isoDaysFromNow(-90))
    check("a professional invite with a history window", inv.code.startsWith("J."))
    val today = java.time.LocalDate.now().toString()
    val r = engine.report(space, today, today, java.time.ZoneId.systemDefault().id, listOf("messages", "expenses"))
    check("a report comes with a spreadsheet copy", r.csv.endsWith(".csv") && File(r.csv).readText().contains("Section,Date,Time"))
    val s = engine.spaces().first { it.id == space }
    check("a parent can chat", s.canChat && !s.childrenChat)
    val contacts = engine.contacts(space)
    check("the other parent is a contact for messages and calls", contacts.size == 1 && contacts[0].message && contacts[0].call)
    val convs = engine.conversations(space)
    check("the demo's private conversation is listed", convs.size == 1 && convs[0].canWrite && convs[0].canCall)
    check("its messages read", engine.messages(convs[0].id).any { it.text.startsWith("Can we agree the summer plan") })
    check("starting it again finds the same one", engine.startConversation(space, listOf(contacts[0].person)) == convs[0].id)
    val answers = engine.helpAsk("how do I message just the other parent privately")
    check("help answers an everyday question", answers.firstOrNull()?.title?.contains("privately") == true)
    val tone = engine.checkTone("Pick-up is at 5. You useless idiot!!!")
    check("the tone check offers a calmer version", tone.calmer == "Pick-up is at 5.")
    val zone = java.time.ZoneId.systemDefault().id
    val cal = engine.calendar(space, today, today, zone)
    val (a, b) = cal.parents.map { it.id }
    val start = java.time.LocalDate.now().plusDays(10).toString()
    engine.proposeSchedule(space, "custom", a, b, start, "17:00", zone, "Weekdays and weekends",
        cycle = listOf(systems.darkcipher.parallelpath.model.Segment(a, 5), systems.darkcipher.parallelpath.model.Segment(b, 2)))
    val pending = engine.calendar(space, today, today, zone).schedules.firstOrNull { it.note == "Weekdays and weekends" }
    check("a custom 5-2 schedule is proposed", pending?.schedule?.pattern?.cycle?.map { it.days } == listOf(5, 2))
    val day = java.time.LocalDate.now().plusDays(20)
    engine.requestChange(space, listOf(
        systems.darkcipher.parallelpath.model.Change("${day}T09:00:00Z", "${day}T18:00:00Z", b),
        systems.darkcipher.parallelpath.model.Change("${day.plusDays(7)}T09:00:00Z", "${day.plusDays(7)}T18:00:00Z", b),
    ), "Two Saturdays")
    val two = engine.calendar(space, today, today, zone).changes.firstOrNull { it.note == "Two Saturdays" }
    check("one request with two periods", two?.changes?.size == 2)
}
