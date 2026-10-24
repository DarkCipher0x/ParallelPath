package systems.darkcipher.parallelpath.family

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import systems.darkcipher.parallelpath.model.Member
import systems.darkcipher.parallelpath.model.Perms
import systems.darkcipher.parallelpath.ui.pp

/**
 * What someone may do in the family: the sections they see and can add to, who they may message privately and who
 * they may call. [people] are the family's other members (for "chosen people"). Parents set this for the people
 * they invite; the server enforces it.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun PermsEditor(role: String, perms: Perms, people: List<Member>, enabled: Boolean, onChange: (Perms) -> Unit) {
    val chip = FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft, selectedLabelColor = pp.accent)
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("They see", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Perms.SECTIONS.forEach { s ->
                val on = s in perms.see
                FilterChip(on, enabled = enabled, onClick = {
                    onChange(if (on) perms.copy(see = perms.see - s, add = perms.add - s) else perms.copy(see = perms.see + s))
                }, label = { Text(Perms.sectionLabel(s)) }, colors = chip)
            }
        }
        if (role != "professional") {
            Text("They can add to", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Perms.SECTIONS.filter { it != "expenses" }.forEach { s ->
                    val on = s in perms.add
                    FilterChip(on, enabled = enabled, onClick = {
                        onChange(if (on) perms.copy(add = perms.add - s) else perms.copy(add = perms.add + s, see = (perms.see + s).distinct()))
                    }, label = { Text(Perms.sectionLabel(s)) }, colors = chip)
                }
            }
            Text("Expenses, schedules and change requests are between parents.", color = pp.fgMute, style = MaterialTheme.typography.bodySmall)
        }
        ReachPicker("Private messages with", perms.message, perms.messageWith, people, enabled) { r, w -> onChange(perms.copy(message = r, messageWith = w)) }
        ReachPicker("Calls with", perms.call, perms.callWith, people, enabled) { r, w -> onChange(perms.copy(call = r, callWith = w)) }
        if (perms.call != "nobody") {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                Text("Video calls (otherwise voice only)", color = pp.fg, modifier = Modifier.weight(1f))
                Switch(perms.video, { onChange(perms.copy(video = it)) }, enabled = enabled,
                    colors = SwitchDefaults.colors(checkedTrackColor = pp.accent, checkedThumbColor = pp.accentInk))
            }
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                Text("Can ring the whole family at once", color = pp.fg, modifier = Modifier.weight(1f))
                Switch(perms.familyCall, { onChange(perms.copy(familyCall = it)) }, enabled = enabled,
                    colors = SwitchDefaults.colors(checkedTrackColor = pp.accent, checkedThumbColor = pp.accentInk))
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ReachPicker(title: String, rule: String, with: List<String>, people: List<Member>, enabled: Boolean, onChange: (String, List<String>) -> Unit) {
    val chip = FilterChipDefaults.filterChipColors(selectedContainerColor = pp.accentSoft, selectedLabelColor = pp.accent)
    Text(title, color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
    FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        listOf("anyone" to "Anyone in the family", "parents" to "The parents", "chosen" to "Chosen people", "nobody" to "Nobody").forEach { (r, label) ->
            FilterChip(rule == r, enabled = enabled, onClick = { onChange(r, with) }, label = { Text(label) }, colors = chip)
        }
    }
    if (rule == "chosen") {
        people.filter { it.status == "active" }.forEach { p ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Checkbox(p.person in with, { on -> onChange(rule, if (on) with + p.person else with - p.person) }, enabled = enabled,
                    colors = CheckboxDefaults.colors(checkedColor = pp.accent))
                Text("${p.name} (${systems.darkcipher.parallelpath.model.Roles.label(p.role).substringBefore(" (")})", color = pp.fg)
            }
        }
    }
}

/** Plain-language summary of someone's permissions, for the member list. */
fun permsSummary(p: Perms?): String {
    if (p == null) return ""
    val reach = { r: String -> when (r) { "anyone" -> "anyone"; "parents" -> "the parents"; "chosen" -> "chosen people"; else -> "nobody" } }
    val sees = if (p.see.size == Perms.SECTIONS.size) "everything" else p.see.joinToString { Perms.sectionLabel(it).lowercase() }.ifBlank { "nothing" }
    return "sees $sees · messages ${reach(p.message)} · calls ${reach(p.call)}" + if (p.call != "nobody" && !p.video) " (voice only)" else ""
}
