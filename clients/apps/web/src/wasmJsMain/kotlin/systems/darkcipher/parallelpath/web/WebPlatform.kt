package systems.darkcipher.parallelpath.web

import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.toComposeImageBitmap
import kotlinx.coroutines.await
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.ui.GeoFix
import systems.darkcipher.parallelpath.ui.PlatformServices
import kotlin.io.encoding.Base64
import kotlin.io.encoding.ExperimentalEncodingApi

@Serializable
private data class Picked(val path: String, val name: String, val mime: String = "")

@Serializable
private data class Fix(val lat: Double, val lon: Double, val accuracy: Double)

/** The browser's side of the app: file picking, opening decrypted files, the clipboard and location. */
@OptIn(ExperimentalEncodingApi::class)
class WebPlatform : PlatformServices {
    private val json = Json { ignoreUnknownKeys = true }

    override suspend fun pickFiles(): List<FileInput> =
        json.decodeFromString<List<Picked>>(ppPickFiles().await<JsString>().toString())
            .map { FileInput(path = it.path, name = it.name, mime = it.mime.ifBlank { null }) }

    override fun openFile(path: String, mime: String) = ppOpenFile(path, mime)

    override val canSaveCopies = true

    /** A browser download (where it goes is the browser's setting). */
    override suspend fun saveCopy(path: String, name: String, mime: String): Boolean = ppSaveFile(path, name, mime)

    override fun decodeImage(bytes: ByteArray): ImageBitmap? =
        runCatching { org.jetbrains.skia.Image.makeFromEncoded(bytes).toComposeImageBitmap() }.getOrNull()

    override fun readFile(path: String): ByteArray? = ppReadFile(path)?.toString()?.let { Base64.decode(it) }

    override fun copyText(text: String) = ppCopy(text)

    override fun openCall(url: String) = ppOpenCall(url)

    override fun base64(s: String): ByteArray = Base64.decode(s)

    override val name: String = "Web"

    override fun notify(title: String, text: String, tag: String) = ppNotify(title, text, tag)

    override fun prepareNotifications() = ppPrepareNotify()

    override val timeZone: String get() = ppTimeZone()

    override val canLocate: Boolean get() = ppCanLocate()

    override suspend fun currentLocation(): GeoFix? {
        val s = runCatching { ppLocate().await<JsString>().toString() }.getOrNull()
        if (s.isNullOrBlank()) return null
        return runCatching { json.decodeFromString<Fix>(s).let { GeoFix(it.lat, it.lon, it.accuracy) } }.getOrNull()
    }
}
