package systems.darkcipher.parallelpath

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.HelpAnswer
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.PQBadge
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.pp

/**
 * Help & FAQ: ask in your own words; the answer comes from ParallelPath's own guide and FAQ, found on this device
 * (no internet, no AI service - and nothing made up: the answers are the guide's own words).
 */
@Composable
internal fun HelpScreen(engine: Engine, onClose: () -> Unit) {
    var question by remember { mutableStateOf("") }
    var answers by remember { mutableStateOf<List<HelpAnswer>>(emptyList()) }
    LaunchedEffect(question) {
        delay(150) // answer as they type, without asking for every keystroke
        answers = if (question.isBlank()) emptyList() else try {
            engine.helpAsk(question)
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            emptyList()
        }
    }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onClose) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, "Back", tint = pp.fg) }
            Text("Help & FAQ", style = MaterialTheme.typography.headlineSmall, color = pp.fg)
        }
        Column(Modifier.widthIn(max = 760.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            Field(question, { question = it }, "Ask a question", supporting = "For example: how do I message just the other parent?")
            PQBadge(text = "Answered on this device · nothing is sent anywhere")
            if (question.isBlank()) {
                Text("Common questions", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                listOf(
                    "How do I message one person privately?", "Who can my child call?", "How do I make a court report?",
                    "I forgot my passphrase", "How do I add my phone?", "Why does the tone check warn me?",
                    "How do I invite a mediator?", "Can the server read my messages?",
                ).forEach { q -> SuggestionChip(onClick = { question = q }, label = { Text(q) }) }
            } else if (answers.isEmpty()) {
                Text("Nothing found - try other words, or see the user guide.", color = pp.fgDim)
            }
            answers.forEach { a ->
                Panel {
                    SectionTitle(a.title)
                    SelectionContainer { Text(a.text, color = pp.fg, style = MaterialTheme.typography.bodyMedium) }
                    Text(if (a.source == "FAQ") "From the FAQ" else "From the user guide · ${a.section}", color = pp.fgMute,
                        style = MaterialTheme.typography.labelSmall)
                }
            }
        }
    }
}
