package systems.darkcipher.parallelpath.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.model.ToneResult

/**
 * The tone check (the engine's built-in model, on this device), or null when the person turned it off in Settings.
 * Every screen with free text that other people read goes through [rememberToneGate].
 */
val LocalToneChecker = staticCompositionLocalOf<(suspend (String) -> ToneResult)?> { null }

/** Holds a draft back while the person looks at what the tone check found. */
class ToneGate internal constructor(private val scope: CoroutineScope, private val check: (suspend (String) -> ToneResult)?) {
    internal class Pending(val result: ToneResult, val text: String, val replace: ((String) -> Unit)?, val send: (String) -> Unit)

    internal var pending by mutableStateOf<Pending?>(null)
    var checking by mutableStateOf(false)
        private set

    /**
     * Sends [text] through [send] - straight away if it reads calmly, otherwise after the person decides. [replace],
     * when given, lets them take the calmer version into the editor instead.
     */
    fun guard(text: String, replace: ((String) -> Unit)? = null, send: (String) -> Unit) {
        val c = check
        if (c == null || text.isBlank()) return send(text)
        if (checking) return
        checking = true
        scope.launch {
            val r = try {
                c(text)
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (_: Exception) {
                null // the check never stops anything being sent
            } finally {
                checking = false
            }
            if (r != null && r.issues.isNotEmpty()) pending = Pending(r, text, replace, send) else send(text)
        }
    }
}

/** A tone gate for this screen; it shows its own dialog when a draft needs a second look. */
@Composable
fun rememberToneGate(): ToneGate {
    val scope = rememberCoroutineScope()
    val check = LocalToneChecker.current
    val gate = remember(check) { ToneGate(scope, check) }
    gate.pending?.let { p ->
        ToneDialog(p.result, canReplace = p.replace != null,
            onEdit = { gate.pending = null },
            onCalmer = { gate.pending = null; p.replace?.invoke(p.result.calmer) },
            onSend = { gate.pending = null; p.send(p.text) })
    }
    return gate
}

/**
 * Shown before something that may come across as hostile goes out. It's still the person's choice: change it, take
 * the calmer version, or send it as it is. (Courts and mediators may read every message.)
 */
@Composable
fun ToneDialog(r: ToneResult, canReplace: Boolean, onEdit: () -> Unit, onCalmer: () -> Unit, onSend: () -> Unit) {
    AlertDialog(
        onDismissRequest = onEdit, containerColor = pp.panel,
        title = { Text(if (r.level == "heated") "This may read as hostile" else "A second look?") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("This becomes part of the family's permanent record. Some wording may make things harder:", color = pp.fgDim)
                r.issues.take(5).forEach { i ->
                    Column {
                        Text("“${i.match}”", color = pp.warm, style = MaterialTheme.typography.titleSmall)
                        Text(i.suggestion, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                    }
                }
                if (canReplace && r.calmer.isNotBlank()) {
                    Text("A calmer version", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                    Text(r.calmer, color = pp.fg, style = MaterialTheme.typography.bodyMedium)
                }
                Text("Checked on this device - nothing is sent anywhere. You can turn this check off in Settings.", color = pp.fgMute,
                    style = MaterialTheme.typography.bodySmall)
            }
        },
        confirmButton = {
            if (canReplace && r.calmer.isNotBlank()) PrimaryButton("Use the calmer version", onClick = onCalmer)
            else PrimaryButton("Change it", onClick = onEdit)
        },
        dismissButton = {
            androidx.compose.foundation.layout.Row {
                if (canReplace && r.calmer.isNotBlank()) TextButton(onClick = onEdit) { Text("Change it myself", color = pp.fgDim) }
                TextButton(onClick = onSend) { Text("Send as it is", color = pp.fgDim) }
            }
        },
    )
}
