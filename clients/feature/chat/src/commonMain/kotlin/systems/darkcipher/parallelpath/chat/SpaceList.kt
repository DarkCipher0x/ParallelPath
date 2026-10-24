package systems.darkcipher.parallelpath.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Avatar
import systems.darkcipher.parallelpath.ui.Radius
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.shortTime

/** The person's family spaces, most recent activity first. */
@Composable
fun SpaceList(spaces: List<Space>, selected: String?, onSelect: (Space) -> Unit, modifier: Modifier = Modifier) {
    LazyColumn(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
        items(spaces, key = { it.id }) { s ->
            val active = s.id == selected
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(Radius.small))
                    .background(if (active) pp.accentSoft else androidx.compose.ui.graphics.Color.Transparent)
                    .clickable { onSelect(s) }.padding(10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Avatar(s.name, s.id, 42)
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(s.name, color = pp.fg, style = MaterialTheme.typography.titleMedium, maxLines = 1, modifier = Modifier.weight(1f))
                        Text(shortTime(s.lastTime), color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
                    }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        val sub = when {
                            s.waiting -> "Waiting for approval"
                            s.pending.isNotEmpty() && s.isParent -> "${s.pending.size} waiting for your approval"
                            else -> s.lastText.ifBlank { "${s.members.count { it.status == "active" }} members" }
                        }
                        Text(sub, color = if (s.waiting || (s.pending.isNotEmpty() && s.isParent)) pp.warm else pp.fgDim,
                            style = MaterialTheme.typography.bodySmall, maxLines = 1, modifier = Modifier.weight(1f))
                        if (s.unread > 0) {
                            Box(Modifier.clip(CircleShape).background(pp.accent).padding(horizontal = 7.dp, vertical = 1.dp)) {
                                Text("${s.unread}", color = pp.accentInk, style = MaterialTheme.typography.labelSmall)
                            }
                        }
                    }
                }
            }
        }
    }
}
