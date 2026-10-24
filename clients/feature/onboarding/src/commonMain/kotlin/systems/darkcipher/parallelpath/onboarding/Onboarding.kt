package systems.darkcipher.parallelpath.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.ServerPreview
import systems.darkcipher.parallelpath.model.SetupResult
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.Mono
import systems.darkcipher.parallelpath.ui.PQBadge
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.QuietButton
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.pp

/** The ParallelPath wordmark: "Parallel" + "Path" in the accent colour. */
@Composable
fun Wordmark(large: Boolean = true) {
    Text(
        buildAnnotatedString {
            append("Parallel")
            withStyle(SpanStyle(color = pp.accent)) { append("Path") }
        },
        style = if (large) MaterialTheme.typography.displaySmall else MaterialTheme.typography.titleLarge,
        color = pp.fg,
    )
}

/** Centered, scrollable column used by the welcome, setup and unlock screens. */
@Composable
fun Centered(content: @Composable () -> Unit) {
    Box(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp), contentAlignment = Alignment.TopCenter) {
        Column(Modifier.widthIn(max = 520.dp).fillMaxWidth().padding(top = 32.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            content()
        }
    }
}

private enum class Step { Welcome, Server, You, Working }

/**
 * First-time setup on this device. Nothing is stored until the server accepts the invite; keys are generated on
 * this device and never leave it unencrypted. In the web app the server is the site itself ([fixedServer]).
 */
@Composable
fun SetupScreen(
    engine: Engine, deviceKind: String, defaultDeviceName: String, fixedServer: String? = null, onDone: (SetupResult) -> Unit,
) {
    var step by remember { mutableStateOf(Step.Welcome) }
    var server by rememberSaveable { mutableStateOf(fixedServer ?: "") }
    var invite by rememberSaveable { mutableStateOf("") }
    var preview by remember { mutableStateOf<ServerPreview?>(null) }
    var yourName by rememberSaveable { mutableStateOf("") }
    var family by rememberSaveable { mutableStateOf("") }
    var deviceName by rememberSaveable { mutableStateOf(defaultDeviceName) }
    var pass by remember { mutableStateOf("") }
    var pass2 by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    // invite: a new account; link: another device of an existing account (a code from that device); recover: a
    // recovery code, when every device is lost.
    var mode by rememberSaveable { mutableStateOf("invite") }
    val startsFamily = mode == "invite" && invite.trim().startsWith("S.")

    Centered {
        Wordmark()
        PQBadge()
        when (step) {
            Step.Welcome -> {
                Text("Calm, organised co-parenting - with a record nobody can quietly change.", style = MaterialTheme.typography.titleMedium, color = pp.fg)
                Text(
                    "Everything you share is end-to-end encrypted with post-quantum cryptography (ML-KEM-1024 and ML-DSA-87). " +
                        "Messages can't be deleted or edited without a reason, and every action is kept in a tamper-evident audit log.",
                    color = pp.fgDim,
                )
                Panel {
                    Text("You need an invitation", style = MaterialTheme.typography.titleMedium)
                    Text(
                        (if (fixedServer == null) "Ask the person who runs your ParallelPath server for the server address and an invite code - or, "
                        else "Ask the person who runs this ParallelPath server for an invite code - or, ") +
                            "to join an existing family, ask the other parent for their invite code.",
                        color = pp.fgDim,
                    )
                    if (fixedServer == null) Field(server, { server = it }, "Server address", supporting = "For example family.example.org:8443")
                    else Text("Server: ${fixedServer.removePrefix("https://")}", color = pp.fgDim, fontFamily = Mono)
                    androidx.compose.foundation.layout.FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        listOf("invite" to "I have an invite", "link" to "I already use ParallelPath", "recover" to "Recover my account").forEach { (id, label) ->
                            androidx.compose.material3.FilterChip(selected = mode == id, onClick = { mode = id; invite = "" }, label = { Text(label) },
                                colors = androidx.compose.material3.FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft,
                                    selectedLabelColor = pp.accent))
                        }
                    }
                    when (mode) {
                        "link" -> Field(invite, { invite = it }, "Link code from your other device",
                            supporting = "On that device: Settings › Link another device. Starts with L-")
                        "recover" -> Field(invite, { invite = it }, "Your recovery code",
                            supporting = "Starts with R-. Your old devices will be signed out; each family approves this one.")
                        else -> Field(invite, { invite = it }, "Invite code", supporting = "Starts with S. (new family) or J. (join a family)")
                    }
                    error?.let { Banner(it, Tone.Error) }
                    PrimaryButton("Continue", busy = busy, enabled = server.isNotBlank() && invite.isNotBlank(), onClick = {
                        error = null
                        busy = true
                        scope.launch {
                            try {
                                preview = engine.preview(server.trim())
                                step = Step.Server
                            } catch (e: kotlinx.coroutines.CancellationException) {
                                throw e // a screen closing or reloading isn't an error
                            } catch (e: Exception) {
                                error = e.message
                            } finally {
                                busy = false
                            }
                        }
                    })
                }
            }

            Step.Server -> preview?.let { p ->
                Panel {
                    Text(p.name.ifBlank { "ParallelPath server" }, style = MaterialTheme.typography.titleLarge)
                    Text(p.url, color = pp.fgDim, fontFamily = Mono)
                    if (p.keyExchange.startsWith("none")) {
                        Banner("Development connection on this machine only - not encrypted in transit. Never use this for a real family.", Tone.Warn)
                    } else {
                        Banner("Connected with post-quantum encryption (${p.keyExchange}). No classical-only connection is ever allowed.", Tone.Ok)
                    }
                    if (!p.trustedByCa) {
                        Banner(
                            "This server uses its own certificate. Check the fingerprint below matches the one the server's " +
                                "owner gave you, then continue. ParallelPath will refuse any other certificate from now on.",
                            Tone.Warn,
                        )
                        Text("Certificate fingerprint", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                        Text(hexGroups(p.certSha256), fontFamily = Mono, color = pp.fg)
                    }
                    Text("Server signing key", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                    Text(p.serverKeyFingerprint, fontFamily = Mono, color = pp.fg)
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        QuietButton("Back", { step = Step.Welcome })
                        PrimaryButton("It matches - continue", { step = Step.You })
                    }
                }
            }

            Step.You -> Panel {
                Text("About you", style = MaterialTheme.typography.titleLarge)
                if (mode == "invite") Field(yourName, { yourName = it }, "Your name", supporting = "How you appear to your family (encrypted - the server never sees it)")
                if (startsFamily) {
                    Field(family, { family = it }, "Name for your family space", supporting = "For example \"Emma and Jack\"")
                }
                Field(deviceName, { deviceName = it }, "Name for this device")
                Text("Passphrase for this device", style = MaterialTheme.typography.titleMedium)
                Text(
                    "Your keys are stored on this device, encrypted with this passphrase (Argon2id). Nobody - not even the " +
                        "server's owner - can recover it, so keep it somewhere safe.",
                    color = pp.fgDim, style = MaterialTheme.typography.bodySmall,
                )
                Field(pass, { pass = it }, "Passphrase", secret = true, supporting = strength(pass))
                Field(pass2, { pass2 = it }, "Repeat passphrase", secret = true, onDone = {}) // Done closes the keyboard, showing the button
                error?.let { Banner(it, Tone.Error) }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    QuietButton("Back", { step = Step.Server })
                    PrimaryButton(
                        when (mode) { "link" -> "Link this device"; "recover" -> "Recover my account"; else -> "Create my keys" },
                        enabled = (mode != "invite" || yourName.isNotBlank()) && deviceName.isNotBlank() && pass.length >= 8 && pass == pass2,
                        onClick = {
                            val p = preview ?: return@PrimaryButton
                            error = null
                            step = Step.Working
                            scope.launch {
                                try {
                                    val res = when (mode) {
                                        "link" -> engine.setupLinked(p.url, p.certSha256, invite, pass, deviceName.trim(), deviceKind)
                                        "recover" -> engine.setupRecover(p.url, p.certSha256, invite, pass, deviceName.trim(), deviceKind)
                                        else -> engine.setup(p.url, p.certSha256, invite, pass, yourName.trim(), deviceName.trim(), deviceKind, family.trim())
                                    }
                                    onDone(res)
                                } catch (e: kotlinx.coroutines.CancellationException) {
                                    throw e // a screen closing or reloading isn't an error
                                } catch (e: Exception) {
                                    error = e.message
                                    step = Step.You
                                }
                            }
                        },
                    )
                }
                if (pass2.isNotEmpty() && pass != pass2) Text("The passphrases don't match", color = pp.danger)
            }

            Step.Working -> Panel {
                Text(when (mode) { "link" -> "Waiting for your other device…"; "recover" -> "Recovering your account…"; else -> "Creating your keys…" },
                    style = MaterialTheme.typography.titleLarge)
                Text(
                    when (mode) {
                        "link" -> "On your other device, ParallelPath will ask whether \"${deviceName.trim()}\" is yours. Confirm it there, " +
                            "and this device receives your identity and your families' keys, sealed so only it can open them."
                        "recover" -> "Opening your recovery kit with your code (Argon2id), creating this device's keys (ML-KEM-1024 + P-384) " +
                            "and signing your old devices out. Then each family's parents approve this device."
                        else -> "Generating your identity (ML-DSA-87 + Ed25519) and this device's keys (ML-KEM-1024 + P-384), " +
                            "then registering with the server. This takes a few seconds."
                    },
                    color = pp.fgDim,
                )
                Spacer(Modifier.height(4.dp))
            }
        }
        Text("Built by DarkCipher0x at DarkCipher Systems · free and open source (AGPL-3.0)", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
    }
}

/** Unlock screen shown every time the app starts or locks. */
@Composable
fun UnlockScreen(engine: Engine, serverName: String, onUnlocked: () -> Unit) {
    var pass by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    Centered {
        Wordmark()
        PQBadge()
        Panel {
            Text("Unlock ParallelPath", style = MaterialTheme.typography.titleLarge)
            if (serverName.isNotBlank()) Text(serverName, color = pp.fgDim)
            val unlock: () -> Unit = unlock@{
                if (pass.isEmpty() || busy) return@unlock
                busy = true
                error = null
                scope.launch {
                    try {
                        engine.unlock(pass)
                        pass = ""
                        onUnlocked()
                    } catch (e: kotlinx.coroutines.CancellationException) {
                        throw e // a screen closing or reloading isn't an error
                    } catch (e: Exception) {
                        error = e.message
                    } finally {
                        busy = false
                    }
                }
            }
            Field(pass, { pass = it }, "Passphrase", secret = true, onDone = unlock)
            error?.let { Banner(it, Tone.Error) }
            PrimaryButton("Unlock", busy = busy, enabled = pass.isNotEmpty(), onClick = unlock)
        }
    }
}

private fun strength(p: String): String = when {
    p.isEmpty() -> "At least 8 characters - a few random words is best"
    p.length < 8 -> "${p.length}/8 characters"
    p.length >= 16 || (p.any { it.isUpperCase() } && p.any { it.isDigit() } && p.any { !it.isLetterOrDigit() }) -> "Strong"
    else -> "OK - longer is stronger"
}

/** Base64 SHA-256 -> "AB12 CD34 ..." hex groups for reading aloud. */
private fun hexGroups(b64: String): String {
    val bytes = decodeBase64(b64)
    val hex = bytes.joinToString("") { (it.toInt() and 0xff).toString(16).padStart(2, '0') }.uppercase()
    return hex.chunked(4).chunked(8).joinToString("\n") { it.joinToString(" ") }
}

private fun decodeBase64(s: String): ByteArray {
    val alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
    val clean = s.filter { it != '=' && !it.isWhitespace() }
    val out = ArrayList<Byte>(clean.length * 3 / 4)
    var buf = 0
    var bits = 0
    for (c in clean) {
        val v = alphabet.indexOf(c)
        if (v < 0) continue
        buf = (buf shl 6) or v
        bits += 6
        if (bits >= 8) {
            bits -= 8
            out.add(((buf shr bits) and 0xff).toByte())
        }
    }
    return out.toByteArray()
}
