package systems.darkcipher.parallelpath.web

import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.platform.LocalFontFamilyResolver
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.platform.Font
import androidx.compose.ui.window.ComposeViewport
import systems.darkcipher.parallelpath.web.res.Res
import systems.darkcipher.parallelpath.App
import systems.darkcipher.parallelpath.UiPrefs
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.ui.ThemeMode

/** The web app: started by host.js once the engine (WebAssembly) is running. */
@OptIn(ExperimentalComposeUiApi::class)
fun main() {
    val engine = Engine(WebTransport())
    val platform = WebPlatform()
    ComposeViewport("pp-root") {
        // Emoji: the browser canvas has no font for them. Load Noto Color Emoji in the background (the app doesn't
        // wait for it; emoji appear once it's here, and the browser keeps it cached).
        val fonts = LocalFontFamilyResolver.current
        LaunchedEffect(Unit) {
            runCatching { fonts.preload(FontFamily(Font("NotoColorEmoji", Res.readBytes("files/NotoColorEmoji.ttf")))) }
        }
        var prefs by remember {
            mutableStateOf(UiPrefs(
                theme = runCatching { ThemeMode.valueOf(ppGetPref("theme")?.toString() ?: "System") }.getOrDefault(ThemeMode.System),
                highContrast = ppGetPref("highContrast")?.toString() == "true",
                notifications = ppGetPref("notifications")?.toString() != "false",
                previews = ppGetPref("previews")?.toString() == "true",
                toneCheck = ppGetPref("toneCheck")?.toString() != "false",
            ))
        }
        App(engine, platform, "web", ppDeviceName(), prefs, { p ->
            prefs = p
            ppSetPref("theme", p.theme.name)
            ppSetPref("highContrast", p.highContrast.toString())
            ppSetPref("notifications", p.notifications.toString())
            ppSetPref("previews", p.previews.toString())
            ppSetPref("toneCheck", p.toneCheck.toString())
        }, fixedServer = ppOrigin())
    }
}
