package systems.darkcipher.parallelpath

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Call
import androidx.compose.material.icons.outlined.CallEnd
import androidx.compose.material.icons.outlined.Videocam
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.engine.EngineEvent
import systems.darkcipher.parallelpath.model.CallEvent
import systems.darkcipher.parallelpath.ui.Avatar
import systems.darkcipher.parallelpath.ui.PQBadge
import systems.darkcipher.parallelpath.ui.PlatformServices
import systems.darkcipher.parallelpath.ui.pp

/**
 * Calls from the app's side: starting one (the call page then does the media), and ringing for incoming ones. The
 * call page itself is the same everywhere: a browser tab or iframe on the web, an Edge app window on Windows and a
 * WebView on Android, all served by this device's own engine.
 */
class Calls(private val engine: Engine, private val platform: PlatformServices, private val scope: CoroutineScope) {
    private val json = Json { ignoreUnknownKeys = true }

    /** The call ringing on this device, if any. */
    var ringing by mutableStateOf<CallEvent?>(null)
        private set

    /** The call this device is in (started or answered here), so a second one gets "busy". */
    private var active: String? = null

    fun onEvent(ev: EngineEvent) {
        val c = runCatching { json.decodeFromJsonElement(CallEvent.serializer(), ev.data) }.getOrNull() ?: return
        when (c.type) {
            "ring" -> if (active != null) scope.launch { runCatching { engine.callBusy(c.space, c.call) } } else ringing = c
            // Answered on another of my devices, cancelled by the caller, or over.
            "accept", "hangup", "decline" -> if (ringing?.call == c.call) ringing = null
            "ended" -> if (active == c.call) active = null // this device hung up
        }
    }

    /** Starts a call to the rest of the family. */
    suspend fun start(space: String, video: Boolean) {
        val id = engine.callStart(space, video)
        open(space, id, "caller")
    }

    suspend fun answer() {
        val c = ringing ?: return
        ringing = null
        open(c.space, c.call, "callee")
    }

    suspend fun decline() {
        val c = ringing ?: return
        ringing = null
        runCatching { engine.callDecline(c.space, c.call) }
    }

    fun dismiss() { ringing = null }

    private suspend fun open(space: String, call: String, role: String) {
        active = call
        platform.openCall(engine.callBridge() + "#space=$space&call=$call&role=$role")
    }
}

val LocalCalls = staticCompositionLocalOf<Calls?> { null }

/** The ringing screen for an incoming call. */
@Composable
fun IncomingCall(calls: Calls, onError: (String) -> Unit) {
    val c = calls.ringing ?: return
    val scope = androidx.compose.runtime.rememberCoroutineScope()
    LaunchedEffect(c.call) {
        delay(45_000) // the caller gives up after 45 seconds too
        if (calls.ringing?.call == c.call) calls.dismiss()
    }
    AlertDialog(
        onDismissRequest = {}, containerColor = pp.panel,
        icon = { Avatar(c.fromName, c.fromPerson, 64) },
        title = { Text("${c.fromName} is calling", fontWeight = FontWeight.SemiBold) },
        text = {
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text((if (c.video) "Video call" else "Voice call") + if (c.familyName.isNotBlank()) " · ${c.familyName}" else "", color = pp.fgDim)
                PQBadge(text = "End-to-end encrypted · post-quantum key")
                androidx.compose.foundation.layout.Spacer(Modifier.size(8.dp))
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(40.dp, Alignment.CenterHorizontally),
                    verticalAlignment = Alignment.CenterVertically) {
                    RoundButton(Icons.Outlined.CallEnd, "Decline", Color(0xFFFF4D5E)) {
                        scope.launch { calls.decline() }
                    }
                    RoundButton(if (c.video) Icons.Outlined.Videocam else Icons.Outlined.Call, "Answer", pp.ok) {
                        scope.launch {
                            try {
                                calls.answer()
                            } catch (e: kotlinx.coroutines.CancellationException) {
                                throw e
                            } catch (e: Exception) {
                                onError(e.message ?: "couldn't answer the call")
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {},
    )
}

@Composable
private fun RoundButton(icon: androidx.compose.ui.graphics.vector.ImageVector, label: String, color: Color, onClick: () -> Unit) {
    Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Button(onClick = onClick, shape = CircleShape, modifier = Modifier.size(64.dp).clip(CircleShape),
            colors = ButtonDefaults.buttonColors(containerColor = color, contentColor = Color.White),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(0.dp)) {
            Icon(icon, label, modifier = Modifier.size(28.dp))
        }
        Text(label, color = pp.fgDim, style = MaterialTheme.typography.labelMedium)
    }
}

/**
 * Who to call: the whole family (when this person may ring everyone), or chosen people - the call then happens in the
 * private conversation with exactly them, so only their devices ring and only they get the call's key.
 * [onTarget] gets the space to call in.
 */
@Composable
fun CallPicker(
    engine: Engine, space: systems.darkcipher.parallelpath.model.Space, video: Boolean, onDismiss: () -> Unit,
    onError: (String) -> Unit, onTarget: (String) -> Unit,
) {
    var contacts by androidx.compose.runtime.remember { mutableStateOf<List<systems.darkcipher.parallelpath.model.Contact>?>(null) }
    val scope = androidx.compose.runtime.rememberCoroutineScope()
    LaunchedEffect(space.id) {
        contacts = try {
            engine.contacts(space.id).filter { it.call }
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            emptyList()
        }
    }
    val me = space.members.firstOrNull { it.me }?.perms
    val whole = space.isParent || me?.familyCall == true || me?.call == "anyone"
    val list = contacts ?: return
    var chosen by androidx.compose.runtime.remember { mutableStateOf(setOf<String>()) }
    AlertDialog(
        onDismissRequest = onDismiss, containerColor = pp.panel,
        title = { Text(if (video) "Video call" else "Voice call") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                if (whole) {
                    androidx.compose.material3.TextButton(onClick = { onTarget(space.id) }) { Text("Everyone in ${space.name}") }
                    Text("…or choose people (only their devices ring):", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                } else {
                    Text("Choose who to call. Only their devices ring.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                }
                if (list.isEmpty()) Text("Nobody - the family's permissions don't allow calls.", color = pp.fgMute)
                list.forEach { p ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        androidx.compose.material3.Checkbox(p.person in chosen, { on -> chosen = if (on) chosen + p.person else chosen - p.person })
                        Text(p.name, color = pp.fg)
                    }
                }
            }
        },
        confirmButton = {
            androidx.compose.material3.Button(enabled = chosen.isNotEmpty(), onClick = {
                scope.launch {
                    try {
                        onTarget(engine.startConversation(space.id, chosen.toList()))
                    } catch (e: kotlinx.coroutines.CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        onError(e.message ?: "couldn't start the call")
                    }
                }
            }) { Text("Call") }
        },
        dismissButton = { androidx.compose.material3.TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
