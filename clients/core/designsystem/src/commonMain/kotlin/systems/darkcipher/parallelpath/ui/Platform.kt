package systems.darkcipher.parallelpath.ui

import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import systems.darkcipher.parallelpath.model.FileInput

/**
 * What the screens need from the operating system. Each app (Android, Windows) supplies its own implementation.
 */
interface PlatformServices {
    /** Lets the person pick photos, videos or files to send. Returns nothing if they cancel. */
    suspend fun pickFiles(): List<FileInput>

    /** Opens a decrypted file with the system viewer (videos, PDFs...). */
    fun openFile(path: String, mime: String)

    /** Decodes an image file or bytes for showing in the app. */
    fun decodeImage(bytes: ByteArray): ImageBitmap?

    /** Reads a (decrypted, private) file. */
    fun readFile(path: String): ByteArray?

    /** Copies text (invite codes) to the clipboard. */
    fun copyText(text: String)

    /** Opens the call page (the engine's call bridge address plus #space=...) in a call window. */
    fun openCall(url: String)

    /** Base64 decoding (thumbnails). */
    fun base64(s: String): ByteArray

    /** "Windows" or "Android". */
    val name: String

    /** Whether this device can tell where it is (phones; check-ins then include the location). */
    val canLocate: Boolean get() = false

    /** The device's current location, asking permission the first time. Null if there's none or the person says no. */
    suspend fun currentLocation(): GeoFix? = null

    /**
     * Shows a system notification (new message, request, reminder). [tag] groups them per family, so a newer one
     * replaces the older. Implementations skip it while the app is in front and focused.
     */
    fun notify(title: String, text: String, tag: String) {}

    /** Asks for permission to notify, where the OS needs that (Android 13+, browsers). Called after unlocking. */
    fun prepareNotifications() {}

    /** The device's time zone (IANA name, e.g. Europe/London), for reminders. */
    val timeZone: String get() = "UTC"

    /** Lets the person pick any files (documents, PDFs... not only photos and videos). */
    suspend fun pickDocuments(): List<FileInput> = pickFiles()

    /**
     * Saves a copy of a decrypted file where the person chooses (a save dialog). Returns false if they cancel.
     * The copy is no longer encrypted - the dialogs say so.
     */
    suspend fun saveCopy(path: String, name: String, mime: String): Boolean = false

    /** Saves copies of several decrypted files into a folder the person chooses. Returns how many were saved. */
    suspend fun saveCopies(files: List<SaveFile>): Int {
        var n = 0
        for (f in files) if (saveCopy(f.path, f.name, f.mime)) n++ else break
        return n
    }

    /** Whether the person can save copies of files from this device. */
    val canSaveCopies: Boolean get() = false

    /** Lets files be dropped onto [this] from the system's file manager (Windows); elsewhere it does nothing. */
    fun Modifier.fileDrop(onFiles: (List<FileInput>) -> Unit, onHover: (Boolean) -> Unit = {}): Modifier = this
}

/** A decrypted file to save a copy of. */
data class SaveFile(val path: String, val name: String, val mime: String)

/** A location fix: degrees and its accuracy in metres. */
data class GeoFix(val lat: Double, val lon: Double, val accuracy: Double)

val LocalPlatform = staticCompositionLocalOf<PlatformServices> { error("no platform services") }
