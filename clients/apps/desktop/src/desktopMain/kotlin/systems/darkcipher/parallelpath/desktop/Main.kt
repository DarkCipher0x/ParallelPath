package systems.darkcipher.parallelpath.desktop

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.toComposeImageBitmap
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Window
import androidx.compose.ui.window.application
import androidx.compose.ui.window.rememberWindowState
import kotlinx.coroutines.delay
import systems.darkcipher.parallelpath.App
import systems.darkcipher.parallelpath.UiPrefs
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.ui.ThemeMode
import java.awt.AWTEvent
import java.awt.Toolkit
import java.io.File
import java.net.InetAddress
import java.util.prefs.Preferences
import javax.swing.JOptionPane
import kotlin.system.exitProcess

private const val IDLE_LOCK_MS = 15 * 60 * 1000L

private val appIcon by lazy {
    runCatching {
        val bytes = Thread.currentThread().contextClassLoader.getResourceAsStream("icon.png")!!.use { it.readBytes() }
        androidx.compose.ui.graphics.painter.BitmapPainter(org.jetbrains.skia.Image.makeFromEncoded(bytes).toComposeImageBitmap())
    }.getOrNull()
}

/** Where the engine binary is: inside the installed app, or PP_ENGINE for development. */
private fun engineExe(): File? {
    System.getenv("PP_ENGINE")?.let { return File(it).takeIf { f -> f.isFile } }
    val res = System.getProperty("compose.application.resources.dir") ?: return null
    return File(res, "ppengine.exe").takeIf { it.isFile } ?: File(res, "windows/ppengine.exe").takeIf { it.isFile }
}

/** Per-user private data folder (the vault inside is encrypted; the folder is the user's own). */
private fun dataDir(): File {
    val base = System.getenv("LOCALAPPDATA") ?: (System.getProperty("user.home") + "/.local/share")
    return File(base, "ParallelPath").apply { mkdirs() }
}

fun main() {
    val exe = engineExe()
    if (exe == null) {
        JOptionPane.showMessageDialog(null, "ParallelPath's engine (ppengine.exe) is missing. Please reinstall.", "ParallelPath",
            JOptionPane.ERROR_MESSAGE)
        exitProcess(1)
    }
    val transport = ProcessTransport(exe, dataDir())
    Runtime.getRuntime().addShutdownHook(Thread { transport.close() })
    val engine = Engine(transport)
    val store = Preferences.userRoot().node("systems/darkcipher/parallelpath")
    val deviceName = runCatching { InetAddress.getLocalHost().hostName }.getOrDefault("Windows PC")

    // Any keyboard or mouse input counts as activity for the idle lock.
    var lastInput = System.currentTimeMillis()
    Toolkit.getDefaultToolkit().addAWTEventListener({ lastInput = System.currentTimeMillis() },
        AWTEvent.KEY_EVENT_MASK or AWTEvent.MOUSE_EVENT_MASK or AWTEvent.MOUSE_WHEEL_EVENT_MASK)

    application {
        var prefs by remember {
            mutableStateOf(UiPrefs(
                theme = runCatching { ThemeMode.valueOf(store.get("theme", "System")) }.getOrDefault(ThemeMode.System),
                highContrast = store.getBoolean("highContrast", false),
                notifications = store.getBoolean("notifications", true),
                previews = store.getBoolean("previews", false),
                toneCheck = store.getBoolean("toneCheck", true),
            ))
        }
        var lockSignal by remember { mutableIntStateOf(0) }
        androidx.compose.runtime.LaunchedEffect(Unit) {
            while (true) {
                delay(30_000)
                if (System.currentTimeMillis() - lastInput > IDLE_LOCK_MS) {
                    lockSignal++
                    lastInput = System.currentTimeMillis()
                }
            }
        }
        val state = rememberWindowState(width = 1280.dp, height = 820.dp)
        var window: java.awt.Frame? = null
        Window(
            onCloseRequest = { transport.close(); exitApplication() },
            state = state,
            title = "ParallelPath",
            icon = appIcon,
        ) {
            window = this.window
            App(
                engine, DesktopPlatform { window }, "windows", deviceName, prefs,
                onPrefs = { p ->
                    prefs = p
                    store.put("theme", p.theme.name)
                    store.putBoolean("highContrast", p.highContrast)
                    store.putBoolean("notifications", p.notifications)
                    store.putBoolean("previews", p.previews)
                    store.putBoolean("toneCheck", p.toneCheck)
                },
                lockSignal = lockSignal,
            )
        }
    }
}
