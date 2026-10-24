package systems.darkcipher.parallelpath.ui

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import systems.darkcipher.parallelpath.model.HelpAnswer
import systems.darkcipher.parallelpath.model.ToneResult

/** The built-in help (the guide and FAQ, searched on this device), for the advice dialog's question box. */
val LocalHelpAsk = staticCompositionLocalOf<(suspend (String) -> List<HelpAnswer>)?> { null }

/**
 * Checks [text] as the person types (a third of a second after they pause; each check takes microseconds on the
 * device) and returns the latest result, or null when the check is off or there's nothing to check.
 */
@Composable
fun rememberLiveTone(text: String): ToneResult? {
    val check = LocalToneChecker.current ?: return null
    var result by remember { mutableStateOf<ToneResult?>(null) }
    LaunchedEffect(text) {
        if (text.isBlank()) {
            result = null
            return@LaunchedEffect
        }
        delay(300)
        result = try {
            check(text)
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
    }
    return result
}

private fun toneColor(level: String): Color = when (level) {
    "heated" -> Color(0xFFFF6B6B)
    "careful" -> Color(0xFFF2A65A)
    else -> Color(0xFF4CC38A)
}

/** The live tone bar: how a draft may come across, with a way to ask for advice. */
@Composable
fun ToneBar(r: ToneResult, onAdvice: () -> Unit) {
    val fill by animateFloatAsState(maxOf(0.06f, r.meter))
    val label = when (r.level) {
        "heated" -> "May read as hostile"
        "careful" -> "Could come across badly"
        else -> "Calm"
    }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.fillMaxWidth()) {
        Text("Tone", color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
        Box(Modifier.weight(1f).height(6.dp).clip(RoundedCornerShape(3.dp)).background(pp.line)) {
            Box(Modifier.fillMaxWidth(fill).fillMaxHeight().clip(RoundedCornerShape(3.dp)).background(toneColor(r.level)))
        }
        Text(label, color = toneColor(r.level), style = MaterialTheme.typography.labelSmall)
        TextButton(onClick = onAdvice) { Text("Advice", style = MaterialTheme.typography.labelMedium) }
    }
}

private val tips = listOf(
    "Brief: one topic, a few sentences.",
    "Informative: the facts - dates, times, places, costs.",
    "Friendly: a neutral greeting and thanks cost nothing.",
    "Firm: say what you need and by when; no threats, no history.",
    "Read it as a judge would - one might.",
)

/**
 * Advice on a draft: what may come across badly and why, a calmer version to use, tips for writing to a co-parent,
 * and a question box answered from ParallelPath's own guide and FAQ. All on this device.
 */
@Composable
fun AdviceDialog(r: ToneResult?, onUseCalmer: ((String) -> Unit)?, onDismiss: () -> Unit) {
    val ask = LocalHelpAsk.current
    var question by remember { mutableStateOf("") }
    var answers by remember { mutableStateOf<List<HelpAnswer>>(emptyList()) }
    LaunchedEffect(question) {
        delay(200)
        answers = if (question.isBlank() || ask == null) emptyList() else try {
            ask(question).take(2)
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            emptyList()
        }
    }
    AlertDialog(
        onDismissRequest = onDismiss, containerColor = pp.panel,
        title = { Text("Advice") },
        text = {
            Column(Modifier.widthIn(max = 560.dp).heightIn(max = 620.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp)) {
                if (r != null && r.issues.isNotEmpty()) {
                    Text("What may come across badly", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                    r.issues.take(6).forEach { i ->
                        Column {
                            Text("“${i.match}”", color = toneColor(r.level), style = MaterialTheme.typography.titleSmall)
                            Text(i.suggestion, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                    if (r.calmer.isNotBlank()) {
                        Text("A calmer version", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                        SelectionContainer { Text(r.calmer, color = pp.fg) }
                        if (onUseCalmer != null) TextButton(onClick = { onUseCalmer(r.calmer) }) { Text("Use it") }
                    }
                } else if (r != null) {
                    Text("This reads calmly.", color = pp.fg)
                }
                if (ask != null) {
                    Spacer(Modifier.height(4.dp))
                    Field(question, { question = it }, "Ask for advice", supporting = "e.g. how do I reply to an angry message?")
                    answers.forEach { a ->
                        Column {
                            Text(a.title, color = pp.fg, style = MaterialTheme.typography.titleSmall)
                            Text(a.text, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
                Text("Writing to a co-parent", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                tips.forEach { Text("• $it", color = pp.fgDim, style = MaterialTheme.typography.bodySmall) }
                Text("Worked out on this device - nothing is sent anywhere.", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Close") } },
    )
}
