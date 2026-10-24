package systems.darkcipher.parallelpath

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.HelpOutline
import androidx.compose.material.icons.outlined.Call
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Videocam
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.ScrollableTabRow
import androidx.compose.material3.TabRowDefaults
import androidx.compose.material3.TabRowDefaults.tabIndicatorOffset
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.audit.AuditScreen
import systems.darkcipher.parallelpath.calendar.CalendarScreen
import systems.darkcipher.parallelpath.calendar.calendarWaiting
import systems.darkcipher.parallelpath.chat.MessagesScreen
import systems.darkcipher.parallelpath.chat.SpaceList
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.expenses.ExpensesScreen
import systems.darkcipher.parallelpath.files.DocumentsScreen
import systems.darkcipher.parallelpath.records.InfoBankScreen
import systems.darkcipher.parallelpath.records.JournalScreen
import systems.darkcipher.parallelpath.engine.message
import systems.darkcipher.parallelpath.engine.online
import systems.darkcipher.parallelpath.engine.notice
import systems.darkcipher.parallelpath.family.FamilyScreen
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.model.Status
import systems.darkcipher.parallelpath.onboarding.SetupScreen
import systems.darkcipher.parallelpath.onboarding.UnlockScreen
import systems.darkcipher.parallelpath.onboarding.Wordmark
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.LocalToneChecker
import systems.darkcipher.parallelpath.ui.PQBadge
import systems.darkcipher.parallelpath.ui.ParallelPathTheme
import systems.darkcipher.parallelpath.ui.PlatformServices
import systems.darkcipher.parallelpath.ui.ThemeMode
import systems.darkcipher.parallelpath.ui.pp

/** Settings the app shell can persist. */
data class UiPrefs(
    val theme: ThemeMode = ThemeMode.System,
    val highContrast: Boolean = false,
    val notifications: Boolean = true, // system notifications for new messages, requests and reminders
    val previews: Boolean = false,     // show message text in them (off: they can appear on a locked screen)
    val toneCheck: Boolean = true,     // check a message's tone before sending
)

/**
 * The whole ParallelPath app, identical on Windows and Android. Each shell supplies the engine transport,
 * the platform services and where to keep UI preferences.
 */
@Composable
fun App(
    engine: Engine, platform: PlatformServices, deviceKind: String, defaultDeviceName: String,
    prefs: UiPrefs, onPrefs: (UiPrefs) -> Unit, lockSignal: Int = 0, startTab: Int = 0, fixedServer: String? = null,
) {
    ParallelPathTheme(prefs.theme, prefs.highContrast) {
        androidx.compose.runtime.CompositionLocalProvider(LocalPlatform provides platform,
            LocalToneChecker provides (if (prefs.toneCheck) { t: String -> engine.checkTone(t) } else null),
            systems.darkcipher.parallelpath.ui.LocalHelpAsk provides { q: String -> engine.helpAsk(q) }) {
            // The background runs edge to edge (behind phone status/navigation bars); content keeps clear of them.
            Surface(Modifier.fillMaxSize(), color = pp.bg) {
                Box(Modifier.fillMaxSize().safeDrawingPadding()) {
                    Root(engine, deviceKind, defaultDeviceName, prefs, onPrefs, lockSignal, startTab, fixedServer)
                }
            }
        }
    }
}

@Composable
private fun Root(
    engine: Engine, deviceKind: String, defaultDeviceName: String, prefs: UiPrefs, onPrefs: (UiPrefs) -> Unit, lockSignal: Int,
    startTab: Int, fixedServer: String?,
) {
    var status by remember { mutableStateOf<Status?>(null) }
    var tick by remember { mutableIntStateOf(0) }
    var online by remember { mutableStateOf(false) }
    val snack = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val platform = LocalPlatform.current
    val calls = remember { Calls(engine, platform, scope) }
    val currentPrefs by rememberUpdatedState(prefs)

    suspend fun refresh() {
        status = runCatching { engine.status() }.getOrNull() ?: Status()
        online = status?.online == true
    }
    LaunchedEffect(Unit) {
        // The engine reports connection changes as events; this also re-reads the state now and then, so the
        // indicator is right even if a change happened before the screen started listening.
        while (true) {
            refresh()
            kotlinx.coroutines.delay(15_000)
        }
    }
    LaunchedEffect(Unit) {
        engine.events.collect { ev ->
            when (ev.name) {
                "changed" -> tick++
                "connection" -> online = ev.online()
                "error" -> ev.message().takeIf { it.isNotBlank() }?.let { snack.showSnackbar(it) }
                "call" -> calls.onEvent(ev)
                "notify" -> ev.notice(engine.json)?.let { n ->
                    val p = currentPrefs
                    if (p.notifications) {
                        val text = if (p.previews && n.preview.isNotBlank()) n.preview else n.body
                        platform.notify(if (n.family.isBlank()) n.title else "${n.title} · ${n.family}", text, n.space)
                    }
                }
            }
        }
    }
    LaunchedEffect(status?.unlocked) {
        if (status?.unlocked == true) {
            if (currentPrefs.notifications) platform.prepareNotifications()
            try {
                engine.setTimeZone(platform.timeZone) // for reminders
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (_: Exception) {
            }
        }
    }
    LaunchedEffect(lockSignal) {
        if (lockSignal > 0 && status?.unlocked == true) {
            engine.lock()
            refresh()
        }
    }
    val onError: (String) -> Unit = { m -> scope.launch { snack.showSnackbar(m) } }

    CompositionLocalProvider(LocalCalls provides calls) {
    Box(Modifier.fillMaxSize()) {
        val s = status
        when {
            s == null -> Unit
            !s.setUp -> SetupScreen(engine, deviceKind, defaultDeviceName, fixedServer) { scope.launch { refresh(); tick++ } }
            !s.unlocked -> UnlockScreen(engine, s.serverName) { scope.launch { refresh(); tick++ } }
            else -> Home(engine, s, tick, online, prefs, onPrefs, onError, startTab,
                onLock = { scope.launch { engine.lock(); refresh() } },
                onChanged = { scope.launch { engine.sync(); tick++ } })
        }
        SnackbarHost(snack, Modifier.align(Alignment.BottomCenter).padding(16.dp))
        IncomingCall(calls, onError)
    }
    }
}

private enum class Tab { Chat, Calendar, Expenses, Documents, Info, Journal, Family, Audit }

@Composable
private fun Home(
    engine: Engine, status: Status, tick: Int, online: Boolean, prefs: UiPrefs, onPrefs: (UiPrefs) -> Unit,
    onError: (String) -> Unit, startTab: Int, onLock: () -> Unit, onChanged: () -> Unit,
) {
    var spaces by remember { mutableStateOf<List<Space>>(emptyList()) }
    var selected by remember { mutableStateOf<String?>(null) }
    var tab by remember { mutableStateOf(Tab.entries.getOrElse(startTab) { Tab.Chat }) }
    var settings by remember { mutableStateOf(false) }
    var help by remember { mutableStateOf(false) }
    LaunchedEffect(tick) {
        spaces = runCatching { engine.spaces() }.getOrDefault(spaces)
    }
    val current = spaces.firstOrNull { it.id == selected }

    BoxWithConstraints(Modifier.fillMaxSize()) {
        val wide = maxWidth >= 900.dp
        if (wide) {
            if (selected == null && spaces.isNotEmpty()) selected = spaces.first().id
            Row(Modifier.fillMaxSize()) {
                Column(Modifier.width(330.dp).fillMaxHeight().background(pp.bg2).padding(14.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Header(status, online, onLock, onHelp = { help = true; settings = false }) { settings = true; help = false }
                    SpaceList(spaces, if (settings || help) null else selected, { selected = it.id; settings = false; help = false }, Modifier.weight(1f))
                }
                VerticalDivider(color = pp.line)
                Box(Modifier.weight(1f).fillMaxHeight()) {
                    when {
                        help -> HelpScreen(engine) { help = false }
                        settings -> SettingsScreen(engine, status, prefs, onPrefs, onLock, onError, onChanged) { settings = false }
                        current != null -> SpaceDetail(engine, current, tab, { tab = it }, tick, onError, onChanged, null)
                        else -> NoSpaces()
                    }
                }
            }
        } else {
            when {
                help -> HelpScreen(engine) { help = false }
                settings -> SettingsScreen(engine, status, prefs, onPrefs, onLock, onError, onChanged) { settings = false }
                current == null -> Column(Modifier.fillMaxSize().padding(14.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Header(status, online, onLock, onHelp = { help = true }) { settings = true }
                    if (spaces.isEmpty()) NoSpaces() else SpaceList(spaces, null, { selected = it.id })
                }
                else -> SpaceDetail(engine, current, tab, { tab = it }, tick, onError, onChanged) { selected = null }
            }
        }
    }
}

@Composable
private fun Header(status: Status, online: Boolean, onLock: () -> Unit, onHelp: () -> Unit, onSettings: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.weight(1f)) {
            Wordmark(large = false)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(8.dp).clip(CircleShape).background(if (online) pp.ok else pp.fgMute))
                Spacer(Modifier.width(6.dp))
                Text(if (online) status.serverName.ifBlank { "Connected" } else "Connecting…", color = pp.fgDim,
                    style = MaterialTheme.typography.labelMedium, maxLines = 1)
            }
        }
        IconButton(onClick = onHelp) { Icon(Icons.AutoMirrored.Outlined.HelpOutline, "Help & FAQ", tint = pp.fgDim) }
        IconButton(onClick = onSettings) { Icon(Icons.Outlined.Settings, "Settings", tint = pp.fgDim) }
        IconButton(onClick = onLock) { Icon(Icons.Outlined.Lock, "Lock", tint = pp.fgDim) }
    }
}

@Composable
private fun NoSpaces() {
    Column(Modifier.fillMaxSize().padding(32.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Text("No family spaces yet", style = MaterialTheme.typography.titleLarge, color = pp.fg)
        Text("Create one, or join with an invite code, from Settings.", color = pp.fgDim)
    }
}

@Composable
private fun SpaceDetail(
    engine: Engine, space: Space, tab: Tab, onTab: (Tab) -> Unit, tick: Int, onError: (String) -> Unit,
    onChanged: () -> Unit, onBack: (() -> Unit)?,
) {
    // How many calendar requests and expenses wait on this person (shown on the tabs).
    var waiting by remember { mutableIntStateOf(0) }
    var expensesWaiting by remember { mutableIntStateOf(0) }
    var callPicker by remember { mutableStateOf<Boolean?>(null) } // open, for video (true) or voice
    val callScope = rememberCoroutineScope()
    val activeCalls = LocalCalls.current
    val startCall: (String, Boolean) -> Unit = { target, video ->
        callScope.launch {
            try {
                activeCalls?.start(target, video)
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (e: Exception) {
                onError(e.message ?: "couldn't start the call")
            }
        }
    }
    LaunchedEffect(space.id, tick) {
        waiting = runCatching { calendarWaiting(engine, space.id) }.getOrDefault(waiting)
        expensesWaiting = runCatching { engine.expenses(space.id).expenses.count { it.needsMe || it.canPay } }.getOrDefault(expensesWaiting)
    }
    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().background(pp.panel).padding(horizontal = 8.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            if (onBack != null) IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, "Back", tint = pp.fg) }
            Column(Modifier.weight(1f).padding(start = 8.dp)) {
                Text(space.name, style = MaterialTheme.typography.titleLarge, color = pp.fg, maxLines = 1)
                Text(space.members.filter { it.status == "active" }.joinToString { if (it.me) "You" else it.name }, color = pp.fgDim,
                    style = MaterialTheme.typography.labelMedium, maxLines = 1)
            }
            val calls = LocalCalls.current
            val me = space.members.firstOrNull { it.me }
            val myPerms = me?.perms
            val mayCall = space.isParent || (myPerms != null && myPerms.call != "nobody")
            if (calls != null && !space.waiting && mayCall) {
                for (video in if (space.isParent || myPerms?.video == true) listOf(false, true) else listOf(false)) {
                    IconButton(onClick = { callPicker = video }) {
                        Icon(if (video) Icons.Outlined.Videocam else Icons.Outlined.Call, if (video) "Video call…" else "Voice call…", tint = pp.fg)
                    }
                }
            }
            PQBadge(text = "End-to-end · post-quantum")
        }
        ScrollableTabRow(selectedTabIndex = tab.ordinal, modifier = Modifier.fillMaxWidth().background(pp.panel), containerColor = pp.panel, contentColor = pp.accent, edgePadding = 8.dp, indicator = { pos ->
            TabRowDefaults.SecondaryIndicator(Modifier.tabIndicatorOffset(pos[tab.ordinal]), color = pp.accent)
        }) {
            Tab.entries.forEach { t ->
                val label = when (t) {
                    Tab.Chat -> "Messages"
                    Tab.Calendar -> if (waiting > 0) "Calendar ($waiting)" else "Calendar"
                    Tab.Expenses -> if (expensesWaiting > 0) "Expenses ($expensesWaiting)" else "Expenses"
                    Tab.Documents -> "Documents"
                    Tab.Info -> "Info bank"
                    Tab.Journal -> "My journal"
                    Tab.Family -> if (space.isParent && space.pending.isNotEmpty()) "Family (${space.pending.size})" else "Family"
                    Tab.Audit -> "Audit log"
                }
                Tab(selected = tab == t, onClick = { onTab(t) }, text = { Text(label, color = if (tab == t) pp.accent else pp.fgDim) })
            }
        }
        callPicker?.let { video -> CallPicker(engine, space, video, onDismiss = { callPicker = null }, onError = onError) { target ->
            callPicker = null
            startCall(target, video)
        } }
        Box(Modifier.weight(1f)) {
            when (tab) {
                Tab.Chat -> MessagesScreen(engine, space, tick, onError, onCall = startCall)
                Tab.Calendar -> CalendarScreen(engine, space, tick, onError)
                Tab.Expenses -> ExpensesScreen(engine, space, tick, onError)
                Tab.Documents -> DocumentsScreen(engine, space, tick, onError)
                Tab.Info -> InfoBankScreen(engine, space, tick, onError)
                Tab.Journal -> JournalScreen(engine, space, tick, onError)
                Tab.Family -> FamilyScreen(engine, space, onChanged, onError)
                Tab.Audit -> AuditScreen(engine, space, tick, onError)
            }
        }
    }
}
