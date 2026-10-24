package systems.darkcipher.parallelpath.audit

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ReceiptLong
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.AuditLog
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Avatar
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.EmptyState
import systems.darkcipher.parallelpath.ui.Field
import systems.darkcipher.parallelpath.ui.Mono
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.pp

/**
 * The family's audit log, as verified on this device: every action is signed by the device that did it and
 * countersigned by the server in a hash chain. If anything has been altered, removed or reordered, this screen says
 * so and shows where.
 */
@Composable
fun AuditScreen(engine: Engine, space: Space, tick: Int, onError: (String) -> Unit) {
    var log by remember { mutableStateOf<AuditLog?>(null) }
    var filter by remember { mutableStateOf("") }
    LaunchedEffect(space.id, tick) {
        try {
            log = engine.audit(space.id)
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e // a screen closing or reloading isn't an error
        } catch (e: Exception) {
            onError(e.message ?: "couldn't load the audit log")
        }
    }
    val l = log ?: return
    val rows = if (filter.isBlank()) l.rows else l.rows.filter {
        it.name.contains(filter, true) || it.what.contains(filter, true) || it.action.contains(filter, true)
    }
    Column(Modifier.fillMaxSize().padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Column(Modifier.widthIn(max = 860.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (l.verified && l.problem.isBlank()) {
                Banner(
                    "Verified on this device: ${l.headSeq} entries, each signed by the acting device (ML-DSA-87 + Ed25519) and " +
                        "countersigned by the server in an unbroken SHA3-512 chain. Chain head ${l.headHash.take(16)}…",
                    Tone.Ok,
                )
            } else {
                Banner("The audit log failed verification: ${l.problem.ifBlank { "not verified yet" }}. " +
                    "This can mean the server's records were altered. Keep this device's copy.", Tone.Error)
            }
            ReportButtons(engine, space, onError)
            Field(filter, { filter = it }, "Search (person, action...)")
        }
        if (rows.isEmpty()) {
            EmptyState(Icons.AutoMirrored.Outlined.ReceiptLong, "Nothing recorded yet", "Every action in the family will appear here.")
        }
        LazyColumn(verticalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.widthIn(max = 860.dp)) {
            items(rows, key = { it.seq }) { r ->
                Row(
                    Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small)).background(pp.panel).padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("#${r.seq}", fontFamily = Mono, color = pp.fgMute, style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.width(52.dp))
                    Avatar(r.name, r.person, 30)
                    Spacer(Modifier.width(10.dp))
                    Column(Modifier.weight(1f)) {
                        Text("${r.name} ${r.what}", color = pp.fg, style = MaterialTheme.typography.bodyMedium)
                        Text(fullTime(r.time) + (if (r.clientTime.isNotBlank()) "  ·  device clock ${fullTime(r.clientTime).substringAfterLast(", ")}" else ""),
                            color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
                    }
                    Text(r.action, fontFamily = Mono, color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
                }
            }
        }
    }
}
