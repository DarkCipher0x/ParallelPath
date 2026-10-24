package systems.darkcipher.parallelpath.files

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.dp
import systems.darkcipher.parallelpath.model.Attachment
import systems.darkcipher.parallelpath.ui.Mono
import systems.darkcipher.parallelpath.ui.PlatformServices
import systems.darkcipher.parallelpath.ui.SecondaryButton
import systems.darkcipher.parallelpath.ui.pp

/** A decrypted file shown inside the app (so it never has to go to another app at all). */
internal class Preview(val name: String, val path: String, val mime: String, val image: ImageBitmap? = null, val text: String? = null)

private val textNames = Regex("""(?i)\.(txt|md|csv|json|log|xml|ics|vcf)$""")

/** An in-app preview for pictures and text files (null: hand the file to the system viewer instead). */
internal fun previewOf(platform: PlatformServices, path: String, a: Attachment): Preview? {
    if (a.isImage) {
        val img = platform.readFile(path)?.let { platform.decodeImage(it) } ?: return null
        return Preview(a.name, path, a.mime, image = img)
    }
    if (a.mime.startsWith("text/") || a.mime == "application/json" || textNames.containsMatchIn(a.name)) {
        if (a.size > 1_000_000) return null
        val bytes = platform.readFile(path) ?: return null
        return Preview(a.name, path, a.mime, text = bytes.decodeToString())
    }
    return null
}

@Composable
internal fun PreviewDialog(p: Preview, onOpen: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss, containerColor = pp.panel,
        title = { Text(p.name, maxLines = 2) },
        text = {
            val img = p.image
            if (img != null) {
                Image(img, p.name, Modifier.fillMaxWidth().heightIn(max = 560.dp), contentScale = ContentScale.Fit)
            } else {
                SelectionContainer(Modifier.heightIn(max = 520.dp).verticalScroll(rememberScrollState())) {
                    Text(p.text.orEmpty(), fontFamily = Mono, color = pp.fg, style = MaterialTheme.typography.bodySmall)
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Close") } },
        dismissButton = { SecondaryButton("Open in another app", onClick = onOpen) },
    )
}
