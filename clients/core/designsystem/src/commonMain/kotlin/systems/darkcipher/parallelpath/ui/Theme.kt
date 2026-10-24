package systems.darkcipher.parallelpath.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * The ParallelPath theme: the same tokens as ParallelPath's web app (static/css/app.css), so every surface (web,
 * Windows, Android) looks like one product. Calm teal on deep blue-green, a warm accent for "needs attention",
 * and dark, light and high-contrast variants.
 */
@Immutable
data class PPColors(
    val bg: Color, val bg2: Color, val panel: Color, val panel2: Color, val line: Color, val line2: Color,
    val fg: Color, val fgDim: Color, val fgMute: Color,
    val accent: Color, val accent2: Color, val accentInk: Color, val accentSoft: Color,
    val warm: Color, val violet: Color, val ok: Color, val warn: Color, val danger: Color,
    val isDark: Boolean,
)

val DarkColors = PPColors(
    bg = Color(0xFF0B1517), bg2 = Color(0xFF0F1D20), panel = Color(0xFF132427), panel2 = Color(0xFF182D31),
    line = Color(0xFF22393D), line2 = Color(0xFF2E4B50),
    fg = Color(0xFFE8F3F2), fgDim = Color(0xFFA8C1BF), fgMute = Color(0xFF7A9693),
    accent = Color(0xFF3DD6D0), accent2 = Color(0xFF66ECE6), accentInk = Color(0xFF052826), accentSoft = Color(0x1F3DD6D0),
    warm = Color(0xFFF2A65A), violet = Color(0xFFA970FF), ok = Color(0xFF5FD38D), warn = Color(0xFFF2C14E), danger = Color(0xFFFF7A7A),
    isDark = true,
)

val LightColors = PPColors(
    bg = Color(0xFFF4F8F8), bg2 = Color(0xFFEAF1F1), panel = Color(0xFFFFFFFF), panel2 = Color(0xFFF2F7F7),
    line = Color(0xFFD6E3E3), line2 = Color(0xFFC3D6D6),
    fg = Color(0xFF11292B), fgDim = Color(0xFF3F5D5F), fgMute = Color(0xFF6B8587),
    accent = Color(0xFF0F9E98), accent2 = Color(0xFF0C8782), accentInk = Color(0xFFFFFFFF), accentSoft = Color(0x1A0F9E98),
    warm = Color(0xFFC46A12), violet = Color(0xFF7A3FD6), ok = Color(0xFF1E8E4E), warn = Color(0xFF9A6B00), danger = Color(0xFFC0392B),
    isDark = false,
)

/** High contrast (accessibility setting): pure black/white with a vivid accent, as on the web app. */
val HighContrastDark = DarkColors.copy(
    bg = Color.Black, bg2 = Color.Black, panel = Color.Black, panel2 = Color(0xFF111111), line = Color(0xFFDCDCDC), line2 = Color.White,
    fg = Color.White, fgDim = Color(0xFFF0F0F0), fgMute = Color(0xFFDCDCDC), accent = Color(0xFF5FFFF8), accent2 = Color(0xFFA6FFFB),
    accentInk = Color.Black,
)
val HighContrastLight = LightColors.copy(
    bg = Color.White, bg2 = Color.White, panel = Color.White, panel2 = Color(0xFFF2F2F2), line = Color(0xFF555555), line2 = Color(0xFF222222),
    fg = Color.Black, fgDim = Color(0xFF111111), fgMute = Color(0xFF222222), accent = Color(0xFF00524F), accent2 = Color(0xFF003B39),
    accentInk = Color.White,
)

enum class ThemeMode { System, Dark, Light }

val LocalPP = staticCompositionLocalOf { DarkColors }

/** Shorthand: `pp.accent`, `pp.panel`... */
val pp: PPColors @Composable get() = LocalPP.current

object Radius {
    val large = 16.dp
    val small = 10.dp
}

private fun scheme(c: PPColors): ColorScheme = if (c.isDark) darkColorScheme(
    primary = c.accent, onPrimary = c.accentInk, primaryContainer = c.accentSoft, onPrimaryContainer = c.accent2,
    secondary = c.warm, tertiary = c.violet, background = c.bg, onBackground = c.fg, surface = c.panel, onSurface = c.fg,
    surfaceVariant = c.panel2, onSurfaceVariant = c.fgDim, outline = c.line2, outlineVariant = c.line, error = c.danger,
    surfaceContainer = c.panel, surfaceContainerHigh = c.panel2, surfaceContainerLow = c.bg2, surfaceContainerHighest = c.panel2,
    surfaceContainerLowest = c.bg,
) else lightColorScheme(
    primary = c.accent, onPrimary = c.accentInk, primaryContainer = c.accentSoft, onPrimaryContainer = c.accent2,
    secondary = c.warm, tertiary = c.violet, background = c.bg, onBackground = c.fg, surface = c.panel, onSurface = c.fg,
    surfaceVariant = c.panel2, onSurfaceVariant = c.fgDim, outline = c.line2, outlineVariant = c.line, error = c.danger,
    surfaceContainer = c.panel, surfaceContainerHigh = c.panel2, surfaceContainerLow = c.bg2, surfaceContainerHighest = c.panel2,
    surfaceContainerLowest = c.bg,
)

private val type = Typography().let { t ->
    val sans = FontFamily.SansSerif
    Typography(
        displaySmall = TextStyle(fontFamily = sans, fontWeight = FontWeight.SemiBold, fontSize = 30.sp, lineHeight = 36.sp),
        headlineSmall = TextStyle(fontFamily = sans, fontWeight = FontWeight.SemiBold, fontSize = 22.sp, lineHeight = 28.sp),
        titleLarge = TextStyle(fontFamily = sans, fontWeight = FontWeight.SemiBold, fontSize = 19.sp, lineHeight = 26.sp),
        titleMedium = TextStyle(fontFamily = sans, fontWeight = FontWeight.SemiBold, fontSize = 16.sp, lineHeight = 22.sp),
        bodyLarge = TextStyle(fontFamily = sans, fontSize = 16.sp, lineHeight = 24.sp),
        bodyMedium = TextStyle(fontFamily = sans, fontSize = 14.sp, lineHeight = 20.sp),
        bodySmall = TextStyle(fontFamily = sans, fontSize = 12.5.sp, lineHeight = 17.sp),
        labelLarge = TextStyle(fontFamily = sans, fontWeight = FontWeight.SemiBold, fontSize = 14.sp),
        labelMedium = TextStyle(fontFamily = sans, fontWeight = FontWeight.Medium, fontSize = 12.sp),
        labelSmall = TextStyle(fontFamily = sans, fontWeight = FontWeight.Medium, fontSize = 11.sp),
        titleSmall = t.titleSmall, headlineMedium = t.headlineMedium, headlineLarge = t.headlineLarge,
        displayMedium = t.displayMedium, displayLarge = t.displayLarge,
    )
}

val Mono = FontFamily.Monospace

@Composable
fun ParallelPathTheme(mode: ThemeMode = ThemeMode.System, highContrast: Boolean = false, content: @Composable () -> Unit) {
    val dark = when (mode) {
        ThemeMode.System -> isSystemInDarkTheme()
        ThemeMode.Dark -> true
        ThemeMode.Light -> false
    }
    val c = when {
        highContrast && dark -> HighContrastDark
        highContrast -> HighContrastLight
        dark -> DarkColors
        else -> LightColors
    }
    CompositionLocalProvider(LocalPP provides c) {
        MaterialTheme(
            colorScheme = scheme(c), typography = type,
            shapes = Shapes(small = RoundedCornerShape(Radius.small), medium = RoundedCornerShape(Radius.small), large = RoundedCornerShape(Radius.large)),
            content = content,
        )
    }
}
