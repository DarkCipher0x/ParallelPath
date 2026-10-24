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
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.Status
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.Mono
import systems.darkcipher.parallelpath.ui.Panel
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.SectionTitle
import systems.darkcipher.parallelpath.ui.ThemeMode
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.pp

@Composable
internal fun SettingsScreen(
    engine: Engine, status: Status, prefs: UiPrefs, onPrefs: (UiPrefs) -> Unit, onLock: () -> Unit,
    onError: (String) -> Unit, onChanged: () -> Unit, onClose: () -> Unit,
) {
    var newFamily by remember { mutableStateOf("") }
    var joinCode by remember { mutableStateOf("") }
    var note by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    fun act(block: suspend () -> Unit) = scope.launch {
        try {
            block()
            onChanged()
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e // a screen closing or reloading isn't an error
        } catch (e: Exception) {
            onError(e.message ?: "something went wrong")
        }
    }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onClose) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, "Back", tint = pp.fg) }
            Text("Settings", style = MaterialTheme.typography.headlineSmall, color = pp.fg)
        }
        Column(Modifier.widthIn(max = 720.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Panel {
                SectionTitle("Appearance")
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ThemeMode.entries.forEach { m ->
                        FilterChip(selected = prefs.theme == m, onClick = { onPrefs(prefs.copy(theme = m)) }, label = { Text(m.name) },
                            colors = FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft, selectedLabelColor = pp.accent))
                    }
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("High contrast", color = pp.fg, modifier = Modifier.weight(1f))
                    Switch(prefs.highContrast, { onPrefs(prefs.copy(highContrast = it)) },
                        colors = SwitchDefaults.colors(checkedTrackColor = pp.accent, checkedThumbColor = pp.accentInk))
                }
            }
            Panel {
                SectionTitle("Notifications and messages")
                @Composable
                fun toggle(title: String, detail: String, on: Boolean, enabled: Boolean = true, set: (Boolean) -> Unit) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(title, color = if (enabled) pp.fg else pp.fgMute)
                            Text(detail, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                        }
                        Switch(on, set, enabled = enabled, colors = SwitchDefaults.colors(checkedTrackColor = pp.accent, checkedThumbColor = pp.accentInk))
                    }
                }
                toggle("Notifications", "New messages, requests that need your answer, and event reminders.", prefs.notifications) {
                    onPrefs(prefs.copy(notifications = it))
                }
                toggle("Show message text in notifications", "Off: they only say who wrote and what kind of thing it is - " +
                    "notifications can show on a locked screen.", prefs.previews, enabled = prefs.notifications) { onPrefs(prefs.copy(previews = it)) }
                toggle("Check my tone before sending", "Points out wording that may come across as hostile, with a calmer way to " +
                    "put it. It runs on this device; you always decide.", prefs.toneCheck) { onPrefs(prefs.copy(toneCheck = it)) }
            }
            Panel {
                SectionTitle("Families")
                // Starting a family on the server is its administrator's call: the box only shows when they've allowed it.
                var account by remember { mutableStateOf<systems.darkcipher.parallelpath.model.Account?>(null) }
                LaunchedEffect(note) { account = runCatching { engine.account() }.getOrNull() }
                if (account?.canStartFamily == true) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Field(newFamily, { newFamily = it }, "New family space name", modifier = Modifier.weight(1f))
                        PrimaryButton("Create", enabled = newFamily.isNotBlank(), onClick = {
                            val n = newFamily
                            newFamily = ""
                            act { engine.createSpace(n); note = "Created \"$n\"." }
                        })
                    }
                } else if (account != null) {
                    Text("Bringing a new family onto this server is up to its administrator. To add people to your own " +
                        "family, invite them from the Family tab.", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                }
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Field(joinCode, { joinCode = it }, "Invite code to join another family", modifier = Modifier.weight(1f))
                    SecondaryButton("Join", enabled = joinCode.isNotBlank(), onClick = {
                        val c = joinCode
                        joinCode = ""
                        act { engine.join(c); note = "Joined - a parent there needs to approve you." }
                    })
                }
                note?.let { Banner(it, Tone.Ok) }
            }
            DevicesPanel(engine, onError)
            Panel {
                SectionTitle("Security")
                Text(status.crypto, color = pp.fg)
                Text("Server", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                Text("${status.serverName}  ·  ${status.serverUrl}", color = pp.fgDim)
                Text("Your identity (person ID)", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                SelectionContainer { Text(status.person, fontFamily = Mono, color = pp.fgDim) }
                Text("This device", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                SelectionContainer { Text(status.device, fontFamily = Mono, color = pp.fgDim) }
                Text(
                    "Keys never leave this device unencrypted. The server stores only ciphertext and signed records; " +
                        "its operator can't read your family's content.",
                    color = pp.fgDim, style = MaterialTheme.typography.bodySmall,
                )
                SecondaryButton("Lock now", onClick = onLock)
            }
            Text("ParallelPath · Built by DarkCipher0x at DarkCipher Systems · free and open source (AGPL-3.0) · beta software",
                color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
        }
    }
}
