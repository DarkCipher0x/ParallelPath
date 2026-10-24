package systems.darkcipher.parallelpath

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.Mono
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.pp

private val kinds = mapOf("windows" to "Windows", "android" to "Android", "web" to "web browser", "linux" to "Linux", "mac" to "Mac", "ios" to "iPhone")

/** Your other devices: link one with a one-time code, and keep a recovery code for if they're all lost. */
@Composable
internal fun DevicesPanel(engine: Engine, onError: (String) -> Unit) {
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    var linkCode by remember { mutableStateOf<String?>(null) }
    var recovery by remember { mutableStateOf<String?>(null) }
    var made by remember { mutableStateOf<String?>(null) }
    var confirmRecovery by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) { made = runCatching { engine.recoveryMade() }.getOrNull() }
    fun act(block: suspend () -> Unit) = scope.launch {
        try {
            block()
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (e: Exception) {
            onError(e.message ?: "something went wrong")
        }
    }

    Panel {
        SectionTitle("Your devices")
        Text("Use ParallelPath on your phone, computer and in a browser as the same person. Linking gives the other device " +
            "your identity and every family's keys - your private journal too - sealed so only that device can open them.",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        FlowButtons { PrimaryButton("Link another device…", onClick = { act { linkCode = engine.linkStart() } }) }
    }
    Panel {
        SectionTitle("Recovery code")
        Text(if (made == null) "If you lose every device, a recovery code is the only way back into your account. Without one, " +
            "you'd start again with a new invite (your families' history stays, but your private journal would be lost)."
            else "You made a recovery code ${fullTime(made!!)}. Making a new one replaces it - the old code stops working.",
            color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
        if (made == null) Banner("You don't have a recovery code yet.", Tone.Warn)
        FlowButtons {
            SecondaryButton(if (made == null) "Make a recovery code" else "Make a new recovery code", onClick = { confirmRecovery = true })
        }
    }

    linkCode?.let { code -> LinkDialog(engine, code, onError) { linkCode = null } }

    if (confirmRecovery) {
        AlertDialog(
            onDismissRequest = { confirmRecovery = false }, containerColor = pp.panel,
            title = { Text("Make a recovery code?") },
            text = {
                Text("You'll see it once. Write it on paper and keep it somewhere safe - with it, anyone could take over your " +
                    "account on a new device (and your current devices would be signed out). The server keeps only a sealed " +
                    "copy it can't open.", color = pp.fgDim)
            },
            confirmButton = { PrimaryButton("Make it", onClick = {
                confirmRecovery = false
                act {
                    recovery = engine.recoveryCreate()
                    made = engine.recoveryMade()
                }
            }) },
            dismissButton = { TextButton(onClick = { confirmRecovery = false }) { Text("Cancel") } },
        )
    }
    recovery?.let { code ->
        AlertDialog(
            onDismissRequest = {}, containerColor = pp.panel,
            title = { Text("Your recovery code") },
            text = {
                Column(Modifier.widthIn(max = 520.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text("Write this down exactly. To get back in on a new device, choose \"Recover my account\" when you set " +
                        "ParallelPath up, and enter it.", color = pp.fgDim)
                    SelectionContainer { Text(code, fontFamily = Mono, fontSize = 20.sp, color = pp.accent, lineHeight = 30.sp) }
                    Banner("Keep it secret and offline. It isn't shown again.", Tone.Warn)
                }
            },
            confirmButton = { PrimaryButton("I've written it down", onClick = { recovery = null }) },
            dismissButton = { TextButton(onClick = { platform.copyText(code) }) { Text("Copy") } },
        )
    }
}

@Composable
private fun LinkDialog(engine: Engine, code: String, onError: (String) -> Unit, onClose: () -> Unit) {
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    var asking by remember { mutableStateOf<Pair<String, String>?>(null) }
    var done by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    LaunchedEffect(code) {
        while (asking == null && done == null) {
            asking = runCatching { engine.linkCheck(code) }.getOrNull()
            if (asking == null) delay(3000)
        }
    }
    AlertDialog(
        onDismissRequest = {}, containerColor = pp.panel,
        title = { Text(if (done != null) "Linked" else "Link another device") },
        text = {
            Column(Modifier.widthIn(max = 520.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                when {
                    done != null -> Banner("$done is now linked. It's signing in and fetching your families.", Tone.Ok)
                    asking == null -> {
                        Text("On the other device, set up ParallelPath, choose \"I already use ParallelPath\", and enter this " +
                            "code (it works once, for 10 minutes):", color = pp.fgDim)
                        SelectionContainer { Text(code, fontFamily = Mono, fontSize = 20.sp, color = pp.accent, lineHeight = 30.sp) }
                        Text("Waiting for the other device…", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
                    }
                    else -> {
                        val (name, kind) = asking!!
                        Text("\"$name\" (${kinds[kind] ?: kind}) is asking to be linked as you.", color = pp.fg,
                            style = MaterialTheme.typography.titleMedium)
                        Banner("Only continue if that's your device and you're setting it up right now. It will be able to read " +
                            "and write everything you can.", Tone.Warn)
                    }
                }
            }
        },
        confirmButton = {
            when {
                done != null -> PrimaryButton("Done", onClick = onClose)
                asking != null -> PrimaryButton("Yes, it's mine - link it", busy = busy, onClick = {
                    scope.launch {
                        busy = true
                        try {
                            engine.linkConfirm(code)
                            done = asking?.first
                        } catch (e: kotlinx.coroutines.CancellationException) {
                            throw e
                        } catch (e: Exception) {
                            onError(e.message ?: "couldn't link the device")
                        } finally {
                            busy = false
                        }
                    }
                })
                else -> TextButton(onClick = { platform.copyText(code) }) { Text("Copy code") }
            }
        },
        dismissButton = { if (done == null) TextButton(onClick = onClose) { Text("Cancel") } },
    )
}
