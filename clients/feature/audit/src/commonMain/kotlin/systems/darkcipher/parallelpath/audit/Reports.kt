package systems.darkcipher.parallelpath.audit

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.datetime.DateTimeUnit
import kotlinx.datetime.TimeZone
import kotlinx.datetime.minus
import systems.darkcipher.parallelpath.engine.Engine
import systems.darkcipher.parallelpath.model.ReportCheck
import systems.darkcipher.parallelpath.model.ReportFiles
import systems.darkcipher.parallelpath.model.Space
import systems.darkcipher.parallelpath.ui.Banner
import systems.darkcipher.parallelpath.ui.Choice
import systems.darkcipher.parallelpath.ui.DateField
import systems.darkcipher.parallelpath.ui.FlowButtons
import systems.darkcipher.parallelpath.ui.FormDialog
import systems.darkcipher.parallelpath.ui.LocalPlatform
import systems.darkcipher.parallelpath.ui.Mono
import systems.darkcipher.parallelpath.ui.PrimaryButton
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.Tone
import systems.darkcipher.parallelpath.ui.fullTime
import systems.darkcipher.parallelpath.ui.pp
import systems.darkcipher.parallelpath.ui.todayLocal

private val sections = listOf(
    "messages" to "Messages and calls", "conversations" to "My private conversations", "calendar" to "Parenting time and calendar", "expenses" to "Expenses",
    "documents" to "Documents", "info" to "Info bank", "audit" to "Audit log", "journal" to "My journal (private notes)",
)

/** Court-ready reports: make one (PDF to read, signed JSON as the evidence) or check one. */
@Composable
internal fun ReportButtons(engine: Engine, space: Space, onError: (String) -> Unit) {
    val platform = LocalPlatform.current
    val scope = rememberCoroutineScope()
    var making by remember { mutableStateOf(false) }
    var made by remember { mutableStateOf<ReportFiles?>(null) }
    var checked by remember { mutableStateOf<ReportCheck?>(null) }
    var busy by remember { mutableStateOf(false) }

    FlowButtons {
        if (!space.waiting) PrimaryButton("Court-ready report…", onClick = { making = true })
        SecondaryButton("Verify a report…", onClick = {
            scope.launch {
                val f = platform.pickFiles().firstOrNull() ?: return@launch
                runCatching { engine.verifyReport(f.path) }.onSuccess { checked = it }
                    .onFailure { if (it !is kotlinx.coroutines.CancellationException) onError(it.message ?: "couldn't check the report") }
            }
        })
    }

    if (making) {
        val today = todayLocal()
        var from by remember { mutableStateOf(today.minus(3, DateTimeUnit.MONTH)) }
        var to by remember { mutableStateOf(today) }
        var chosen by remember { mutableStateOf(sections.map { it.first }.filter { it != "journal" }.toSet()) }
        FormDialog("Court-ready report", "Make report", chosen.isNotEmpty() && from <= to, busy, { making = false }, {
            scope.launch {
                busy = true
                try {
                    made = engine.report(space.id, from.toString(), to.toString(), TimeZone.currentSystemDefault().id,
                        sections.map { it.first }.filter { it in chosen })
                    making = false
                } catch (e: kotlinx.coroutines.CancellationException) {
                    throw e
                } catch (e: Exception) {
                    onError(e.message ?: "couldn't make the report")
                } finally {
                    busy = false
                }
            }
        }) {
            Text("Made on this device from the verified, decrypted family record: a PDF to read, and a signed file that proves " +
                "it hasn't been changed. Every edit and withdrawal is shown with its reason. The family's audit log records " +
                "that you made a report (not what's in it).", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                DateField("From", from, { from = it })
                DateField("To", to, { to = it })
            }
            Text("Include", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
            FlowButtons {
                sections.forEach { (id, label) ->
                    Choice(id in chosen, label, { chosen = if (id in chosen) chosen - id else chosen + id })
                }
            }
            if ("journal" in chosen) Banner("Your journal is private. Including it puts those notes in this report - only share it " +
                "with people you'd show them to, such as your solicitor.", Tone.Warn)
        }
    }

    made?.let { r ->
        AlertDialog(
            onDismissRequest = { made = null }, containerColor = pp.panel,
            title = { Text("Report ready") },
            text = {
                Column(Modifier.widthIn(max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Text("Give both files to whoever needs them. The PDF is for reading; the .json file is the signed evidence - " +
                        "anyone can check it in ParallelPath (Verify a report) or with ppserver verify-report.", color = pp.fgDim)
                    Text("Signed by your identity key", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                    SelectionContainer { Text(r.signer, fontFamily = Mono, color = pp.fg) }
                    Text("Report fingerprint (SHA3-512)", color = pp.fgMute, style = MaterialTheme.typography.labelMedium)
                    SelectionContainer { Text(r.sha3.chunked(32).joinToString("\n"), fontFamily = Mono, color = pp.fg, style = MaterialTheme.typography.bodySmall) }
                    FlowButtons {
                        PrimaryButton("Open the PDF", onClick = { platform.openFile(r.pdf, "application/pdf") })
                        SecondaryButton("Open the signed file", onClick = { platform.openFile(r.json, "application/json") })
                        if (r.csv.isNotBlank()) SecondaryButton("Open as a spreadsheet", onClick = { platform.openFile(r.csv, "text/csv") })
                    }
                }
            },
            confirmButton = { TextButton(onClick = { made = null }) { Text("Done") } },
        )
    }

    checked?.let { c ->
        AlertDialog(
            onDismissRequest = { checked = null }, containerColor = pp.panel,
            title = { Text(if (c.valid) "This report is genuine" else "This report can't be trusted") },
            text = {
                Column(Modifier.widthIn(max = 560.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (c.valid) Banner("The signature matches: nothing has been changed since ${c.byName} signed it.", Tone.Ok)
                    else Banner(c.problem.replaceFirstChar { it.uppercase() }, Tone.Error)
                    if (c.id.isNotBlank()) {
                        Text("${c.family} · ${c.from} to ${c.to}", color = pp.fg)
                        Text("Made by ${c.byName} ${fullTime(c.generated)}", color = pp.fgDim)
                        Text("Signer's identity fingerprint: ${c.signer}", fontFamily = Mono, color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                        Text("Audit chain: entry #${c.auditSeq}" + if (c.chainMatch.isNotBlank()) " - ${c.chainMatch}" else
                            " (this device isn't in that family, so it can't compare the chain)", color = pp.fgDim, style = MaterialTheme.typography.bodySmall)
                    }
                }
            },
            confirmButton = { TextButton(onClick = { checked = null }) { Text("Close") } },
        )
    }
}
