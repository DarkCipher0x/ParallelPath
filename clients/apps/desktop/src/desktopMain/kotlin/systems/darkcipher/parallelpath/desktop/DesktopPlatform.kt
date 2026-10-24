package systems.darkcipher.parallelpath.desktop

import androidx.compose.foundation.draganddrop.dragAndDropTarget
import androidx.compose.ui.draganddrop.DragData
import androidx.compose.ui.Modifier
import androidx.compose.ui.draganddrop.DragAndDropEvent
import androidx.compose.ui.draganddrop.DragAndDropTarget
import androidx.compose.ui.draganddrop.dragData
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.toComposeImageBitmap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.jetbrains.skia.Image
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.ui.PlatformServices
import systems.darkcipher.parallelpath.ui.SaveFile
import java.awt.Desktop
import java.awt.FileDialog
import java.awt.Frame
import java.awt.Toolkit
import java.awt.datatransfer.StringSelection
import java.io.File
import java.util.Base64

/** Windows implementations of what the screens need from the OS. */
class DesktopPlatform(private val frame: () -> Frame?) : PlatformServices {
    override val name = "Windows"

    override suspend fun pickFiles(): List<FileInput> = withContext(Dispatchers.Main) {
        val d = FileDialog(frame(), "Choose photos, videos or files to send", FileDialog.LOAD)
        d.isMultipleMode = true
        d.isVisible = true
        d.files.orEmpty().filter { it.isFile }.map { FileInput(path = it.absolutePath, name = it.name) }
    }

    override val canSaveCopies = true

    override suspend fun saveCopy(path: String, name: String, mime: String): Boolean = withContext(Dispatchers.Main) {
        val d = FileDialog(frame(), "Save a copy of $name", FileDialog.SAVE)
        d.file = name
        d.isVisible = true
        val dir = d.directory ?: return@withContext false
        val target = File(dir, d.file ?: return@withContext false)
        withContext(Dispatchers.IO) { runCatching { File(path).copyTo(target, overwrite = true) }.isSuccess }
    }

    /** Several files: the person picks a folder; a name that's taken there gets " (2)", " (3)"... */
    override suspend fun saveCopies(files: List<SaveFile>): Int {
        val dir = withContext(Dispatchers.Main) {
            val c = javax.swing.JFileChooser().apply {
                dialogTitle = "Save copies of ${files.size} files to…"
                fileSelectionMode = javax.swing.JFileChooser.DIRECTORIES_ONLY
                approveButtonText = "Save here"
            }
            if (c.showSaveDialog(frame()) == javax.swing.JFileChooser.APPROVE_OPTION) c.selectedFile else null
        } ?: return 0
        return withContext(Dispatchers.IO) {
            files.count { f ->
                var target = File(dir, f.name)
                var i = 2
                while (target.exists()) {
                    target = File(dir, f.name.substringBeforeLast('.') + " ($i)" + if ('.' in f.name) "." + f.name.substringAfterLast('.') else "")
                    i++
                }
                runCatching { File(f.path).copyTo(target) }.isSuccess
            }
        }
    }

    /** Files dragged in from File Explorer. */
    @OptIn(androidx.compose.ui.ExperimentalComposeUiApi::class, androidx.compose.foundation.ExperimentalFoundationApi::class)
    override fun Modifier.fileDrop(onFiles: (List<FileInput>) -> Unit, onHover: (Boolean) -> Unit): Modifier =
        this.dragAndDropTarget(shouldStartDragAndDrop = { it.dragData() is DragData.FilesList }, target = object : DragAndDropTarget {
            override fun onEntered(event: DragAndDropEvent) = onHover(true)
            override fun onExited(event: DragAndDropEvent) = onHover(false)
            override fun onEnded(event: DragAndDropEvent) = onHover(false)
            override fun onDrop(event: DragAndDropEvent): Boolean {
                onHover(false)
                val list = (event.dragData() as? DragData.FilesList)?.readFiles().orEmpty()
                    .mapNotNull { runCatching { File(java.net.URI(it)) }.getOrNull() }.filter { it.isFile }
                if (list.isEmpty()) return false
                onFiles(list.map { FileInput(path = it.absolutePath, name = it.name) })
                return true
            }
        })

    override fun openFile(path: String, mime: String) {
        runCatching { Desktop.getDesktop().open(File(path)) }
    }

    override fun decodeImage(bytes: ByteArray): ImageBitmap? =
        runCatching { Image.makeFromEncoded(bytes).toComposeImageBitmap() }.getOrNull()

    override fun readFile(path: String): ByteArray? = runCatching { File(path).readBytes() }.getOrNull()

    override fun copyText(text: String) {
        Toolkit.getDefaultToolkit().systemClipboard.setContents(StringSelection(text), null)
    }

    override fun base64(s: String): ByteArray = Base64.getDecoder().decode(s)

    override val timeZone: String get() = java.time.ZoneId.systemDefault().id

    private var tray: java.awt.TrayIcon? = null

    /**
     * A Windows notification (shown in the notification centre), unless ParallelPath is the window in front. It needs
     * an icon in the notification area, added the first time; clicking the notification brings the app forward.
     */
    override fun notify(title: String, text: String, tag: String) {
        val f = frame()
        if (f != null && f.isFocused) return
        if (!java.awt.SystemTray.isSupported()) return
        java.awt.EventQueue.invokeLater {
            runCatching {
                val icon = tray ?: java.awt.TrayIcon(trayImage(), "ParallelPath").also { t ->
                    t.isImageAutoSize = true
                    t.addActionListener { frame()?.let { it.isVisible = true; it.state = Frame.NORMAL; it.toFront(); it.requestFocus() } }
                    java.awt.SystemTray.getSystemTray().add(t)
                    tray = t
                }
                icon.displayMessage(title, text, java.awt.TrayIcon.MessageType.NONE)
            }
        }
    }

    private fun trayImage(): java.awt.Image =
        frame()?.iconImages?.firstOrNull() ?: java.awt.image.BufferedImage(16, 16, java.awt.image.BufferedImage.TYPE_INT_ARGB)

    /**
     * Calls open in a Microsoft Edge app window (every Windows 10/11 PC has Edge; Chrome is used if Edge is missing),
     * with a profile of its own that only ever opens this device's call page. Edge asks for the camera and microphone
     * the first time; the call page keeps the same address (see the engine's call bridge), so it remembers the answer.
     */
    override fun openCall(url: String) {
        val browser = callBrowser() ?: run { runCatching { Desktop.getDesktop().browse(java.net.URI(url)) }; return }
        val profile = File(System.getenv("LOCALAPPDATA") ?: System.getProperty("user.home"), "ParallelPath/CallWindow").apply { mkdirs() }
        runCatching {
            ProcessBuilder(browser.path, "--app=$url", "--user-data-dir=${profile.path}", "--no-first-run", "--no-default-browser-check",
                "--window-size=1280,800",
                "--disable-features=Translate,msEdgeSidebarV2", "--disable-sync").start()
        }
    }

    private fun callBrowser(): File? {
        val pf = listOfNotNull(System.getenv("ProgramFiles(x86)"), System.getenv("ProgramFiles"), System.getenv("LOCALAPPDATA"))
        return pf.flatMap { listOf(File(it, "Microsoft/Edge/Application/msedge.exe"), File(it, "Google/Chrome/Application/chrome.exe")) }
            .sortedBy { if (it.name == "msedge.exe") 0 else 1 }.firstOrNull { it.isFile }
    }
}
