package systems.darkcipher.parallelpath.android

import android.content.Context
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import systems.darkcipher.parallelpath.App
import systems.darkcipher.parallelpath.UiPrefs
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.ui.ThemeMode
import java.io.File

/**
 * Android shell: hosts the shared App(), the in-process Go engine and the photo picker.
 * Screenshots and screen recording are blocked (FLAG_SECURE); the app locks after 5 minutes in the background.
 */
class MainActivity : ComponentActivity() {
    private lateinit var transport: GoTransport
    private var lockSignal by mutableIntStateOf(0)
    private var prefs by mutableStateOf(UiPrefs())
    private var backgroundedAt = 0L

    /** Set while the system picker/viewer is open, so leaving for it doesn't count as backgrounding. */
    var inPicker = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Release builds block screenshots and screen recording; debug builds allow them so the UI can be checked.
        if (applicationInfo.flags and android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE == 0) {
            window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)
        }
        enableEdgeToEdge()
        transport = (application as? PPApp)?.transport ?: GoTransport(File(filesDir, "engine")).also { (application as? PPApp)?.transport = it }
        val engine = Engine(transport)
        val sp = getSharedPreferences("ui", Context.MODE_PRIVATE)
        prefs = UiPrefs(
            theme = runCatching { ThemeMode.valueOf(sp.getString("theme", "System")!!) }.getOrDefault(ThemeMode.System),
            highContrast = sp.getBoolean("highContrast", false),
            notifications = sp.getBoolean("notifications", true),
            previews = sp.getBoolean("previews", false),
            toneCheck = sp.getBoolean("toneCheck", true),
        )
        val platform = AndroidPlatform(this)
        val name = "${Build.MANUFACTURER} ${Build.MODEL}".trim().take(60)
        setContent {
            // Status/navigation bar icons follow the app's theme, not just the system's.
            val dark = when (prefs.theme) {
                ThemeMode.Dark -> true
                ThemeMode.Light -> false
                ThemeMode.System -> isSystemInDarkTheme()
            }
            LaunchedEffect(dark) {
                val bars = if (dark) SystemBarStyle.dark(Color.TRANSPARENT) else SystemBarStyle.light(Color.TRANSPARENT, Color.TRANSPARENT)
                enableEdgeToEdge(statusBarStyle = bars, navigationBarStyle = bars)
            }
            App(engine, platform, "android", name, prefs, onPrefs = { p ->
                prefs = p
                sp.edit().putString("theme", p.theme.name).putBoolean("highContrast", p.highContrast)
                    .putBoolean("notifications", p.notifications).putBoolean("previews", p.previews).putBoolean("toneCheck", p.toneCheck).apply()
            }, lockSignal = lockSignal)
        }
    }

    override fun onStop() {
        super.onStop()
        if (!inPicker) backgroundedAt = SystemClock.elapsedRealtime()
    }

    override fun onStart() {
        super.onStart()
        if (backgroundedAt > 0 && SystemClock.elapsedRealtime() - backgroundedAt > 5 * 60_000L) lockSignal++
        backgroundedAt = 0
        inPicker = false
    }
}
