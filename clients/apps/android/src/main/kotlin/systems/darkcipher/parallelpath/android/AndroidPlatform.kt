package systems.darkcipher.parallelpath.android

import android.Manifest
import android.annotation.SuppressLint
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.media.MediaMetadataRetriever
import android.location.LocationManager
import android.net.Uri
import android.os.CancellationSignal
import android.provider.OpenableColumns
import android.util.Base64
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import android.content.pm.PackageManager
import androidx.core.location.LocationManagerCompat
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume
import systems.darkcipher.parallelpath.R
import systems.darkcipher.parallelpath.model.FileInput
import systems.darkcipher.parallelpath.ui.GeoFix
import systems.darkcipher.parallelpath.ui.PlatformServices
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.UUID

/** Android implementations of what the screens need from the OS. */
class AndroidPlatform(private val activity: MainActivity) : PlatformServices {
    override val name = "Android"
    private var pending: CompletableDeferred<List<Uri>>? = null

    private val picker = activity.registerForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(20)) { uris ->
        pending?.complete(uris)
    }

    /** The system photo/video picker: no storage permission needed, the app sees only what the person picks. */
    override suspend fun pickFiles(): List<FileInput> {
        val d = CompletableDeferred<List<Uri>>()
        pending = d
        activity.inPicker = true
        picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageAndVideo))
        val uris = d.await()
        activity.inPicker = false
        return withContext(Dispatchers.IO) { uris.mapNotNull { copyIn(it) } }
    }

    /** Copies a picked item into the engine's private temp folder (the engine encrypts it, then deletes the copy). */
    private fun copyIn(uri: Uri): FileInput? = runCatching {
        val cr = activity.contentResolver
        val name = cr.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
            if (c.moveToFirst()) c.getString(0) else null
        } ?: "file"
        val mime = cr.getType(uri)
        val dir = File(activity.filesDir, "engine/tmp/picked").apply { mkdirs() }
        val f = File(dir, UUID.randomUUID().toString() + "-" + name.replace(Regex("[^A-Za-z0-9._ -]"), "_"))
        cr.openInputStream(uri)!!.use { input -> f.outputStream().use { input.copyTo(it) } }
        var thumb: String? = null
        var w = 0
        var h = 0
        var ms = 0L
        if (mime?.startsWith("video/") == true) {
            val r = MediaMetadataRetriever()
            try {
                r.setDataSource(f.absolutePath)
                ms = r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull() ?: 0
                w = r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_VIDEO_WIDTH)?.toIntOrNull() ?: 0
                h = r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_VIDEO_HEIGHT)?.toIntOrNull() ?: 0
                r.getScaledFrameAtTime(1_000_000, MediaMetadataRetriever.OPTION_CLOSEST_SYNC, 320, 320)?.let { bmp ->
                    val out = ByteArrayOutputStream()
                    bmp.compress(Bitmap.CompressFormat.JPEG, 75, out)
                    thumb = Base64.encodeToString(out.toByteArray(), Base64.NO_WRAP)
                }
            } finally {
                r.release()
            }
        }
        FileInput(path = f.absolutePath, name = name, mime = mime, thumb = thumb, width = w, height = h, millis = ms)
    }.getOrNull()

    private var pendingDocs: CompletableDeferred<List<Uri>>? = null
    private val docPicker = activity.registerForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
        pendingDocs?.complete(uris)
    }

    /** Any kind of file, through the system's file picker (no storage permission: the app sees only what's picked). */
    override suspend fun pickDocuments(): List<FileInput> {
        val d = CompletableDeferred<List<Uri>>()
        pendingDocs = d
        activity.inPicker = true
        docPicker.launch(arrayOf("*/*"))
        val uris = d.await()
        activity.inPicker = false
        return withContext(Dispatchers.IO) { uris.mapNotNull { copyIn(it) } }
    }

    private var pendingSave: CompletableDeferred<Uri?>? = null
    private var saveMime = "application/octet-stream"
    private val saver = activity.registerForActivityResult(object : ActivityResultContracts.CreateDocument("*/*") {
        override fun createIntent(context: Context, input: String) = super.createIntent(context, input).setType(saveMime)
    }) { uri -> pendingSave?.complete(uri) }

    override val canSaveCopies = true

    /** The system's "save to" screen (Downloads, Drive...), then the decrypted copy is written there. */
    override suspend fun saveCopy(path: String, name: String, mime: String): Boolean {
        val d = CompletableDeferred<Uri?>()
        pendingSave = d
        saveMime = mime.ifBlank { "application/octet-stream" }
        activity.inPicker = true
        saver.launch(name)
        val uri = d.await()
        activity.inPicker = false
        uri ?: return false
        return withContext(Dispatchers.IO) {
            runCatching {
                activity.contentResolver.openOutputStream(uri, "wt")!!.use { out -> File(path).inputStream().use { it.copyTo(out) } }
            }.isSuccess
        }
    }

    private var pendingTree: CompletableDeferred<Uri?>? = null
    private val treePicker = activity.registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri -> pendingTree?.complete(uri) }

    /** Several files: the person picks a folder once, and each copy is created in it. */
    override suspend fun saveCopies(files: List<systems.darkcipher.parallelpath.ui.SaveFile>): Int {
        val d = CompletableDeferred<Uri?>()
        pendingTree = d
        activity.inPicker = true
        treePicker.launch(null)
        val tree = d.await()
        activity.inPicker = false
        tree ?: return 0
        return withContext(Dispatchers.IO) {
            val cr = activity.contentResolver
            val parent = android.provider.DocumentsContract.buildDocumentUriUsingTree(tree, android.provider.DocumentsContract.getTreeDocumentId(tree))
            files.count { f ->
                runCatching {
                    val uri = android.provider.DocumentsContract.createDocument(cr, parent, f.mime.ifBlank { "application/octet-stream" }, f.name)!!
                    cr.openOutputStream(uri, "wt")!!.use { out -> File(f.path).inputStream().use { it.copyTo(out) } }
                }.isSuccess
            }
        }
    }

    override fun openFile(path: String, mime: String) {
        val uri = FileProvider.getUriForFile(activity, activity.packageName + ".files", File(path))
        val i = Intent(Intent.ACTION_VIEW).setDataAndType(uri, mime).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        activity.inPicker = true
        runCatching { activity.startActivity(Intent.createChooser(i, "Open with")) }
    }

    override fun decodeImage(bytes: ByteArray): ImageBitmap? =
        runCatching { BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap() }.getOrNull()

    override fun readFile(path: String): ByteArray? = runCatching { File(path).readBytes() }.getOrNull()

    override fun openCall(url: String) {
        activity.inPicker = true // a call isn't "leaving the app": don't lock behind it
        activity.startActivity(Intent(activity, CallActivity::class.java).putExtra("url", url))
    }

    override fun copyText(text: String) {
        val cm = activity.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        cm.setPrimaryClip(ClipData.newPlainText("ParallelPath", text))
    }

    override fun base64(s: String): ByteArray = Base64.decode(s, Base64.DEFAULT)

    override val timeZone: String get() = java.util.TimeZone.getDefault().id

    private val notifyPermission = activity.registerForActivityResult(ActivityResultContracts.RequestPermission()) { }

    override fun prepareNotifications() {
        val nm = activity.getSystemService(Context.NOTIFICATION_SERVICE) as android.app.NotificationManager
        if (nm.getNotificationChannel(CHANNEL) == null) {
            nm.createNotificationChannel(android.app.NotificationChannel(CHANNEL, "Messages and requests",
                android.app.NotificationManager.IMPORTANCE_DEFAULT).apply {
                description = "New messages, requests that need your answer, and event reminders"
                lockscreenVisibility = android.app.Notification.VISIBILITY_PRIVATE
            })
        }
        if (android.os.Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(activity, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            val sp = activity.getSharedPreferences("ui", Context.MODE_PRIVATE)
            if (!sp.getBoolean("askedNotify", false)) { // ask once; after that it's the person's choice in Android settings
                sp.edit().putBoolean("askedNotify", true).apply()
                notifyPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }

    /** A notification, unless the app is on screen. Its lock-screen version only says "ParallelPath". */
    @SuppressLint("MissingPermission")
    override fun notify(title: String, text: String, tag: String) {
        if (activity.lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.RESUMED)) return
        val nm = androidx.core.app.NotificationManagerCompat.from(activity)
        if (!nm.areNotificationsEnabled()) return
        prepareNotifications()
        val open = android.app.PendingIntent.getActivity(activity, 0,
            Intent(activity, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP),
            android.app.PendingIntent.FLAG_IMMUTABLE or android.app.PendingIntent.FLAG_UPDATE_CURRENT)
        val public = androidx.core.app.NotificationCompat.Builder(activity, CHANNEL).setSmallIcon(R.drawable.ic_stat_pp)
            .setContentTitle("ParallelPath").setContentText("Something new").build()
        val n = androidx.core.app.NotificationCompat.Builder(activity, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_pp).setContentTitle(title).setContentText(text)
            .setStyle(androidx.core.app.NotificationCompat.BigTextStyle().bigText(text))
            .setContentIntent(open).setAutoCancel(true).setOnlyAlertOnce(false)
            .setVisibility(androidx.core.app.NotificationCompat.VISIBILITY_PRIVATE).setPublicVersion(public)
            .build()
        runCatching { nm.notify(tag, 1, n) }
    }

    private companion object {
        const val CHANNEL = "updates"
    }

    private var permissionAnswer: CompletableDeferred<Boolean>? = null
    private val locationPermission = activity.registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { r ->
        permissionAnswer?.complete(r.values.any { it })
    }

    override val canLocate = true

    /** One location fix for a check-in. Permission is asked the first time; nothing runs in the background. */
    @SuppressLint("MissingPermission")
    override suspend fun currentLocation(): GeoFix? {
        fun granted(p: String) = ContextCompat.checkSelfPermission(activity, p) == PackageManager.PERMISSION_GRANTED
        if (!granted(Manifest.permission.ACCESS_FINE_LOCATION) && !granted(Manifest.permission.ACCESS_COARSE_LOCATION)) {
            val d = CompletableDeferred<Boolean>()
            permissionAnswer = d
            activity.inPicker = true
            locationPermission.launch(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION))
            val ok = d.await()
            activity.inPicker = false
            if (!ok) return null
        }
        val lm = activity.getSystemService(LocationManager::class.java) ?: return null
        val provider = when {
            granted(Manifest.permission.ACCESS_FINE_LOCATION) && lm.isProviderEnabled(LocationManager.GPS_PROVIDER) -> LocationManager.GPS_PROVIDER
            lm.isProviderEnabled(LocationManager.NETWORK_PROVIDER) -> LocationManager.NETWORK_PROVIDER
            else -> return null
        }
        return withTimeoutOrNull(20_000) {
            suspendCancellableCoroutine { cont ->
                val signal = CancellationSignal()
                cont.invokeOnCancellation { signal.cancel() }
                LocationManagerCompat.getCurrentLocation(lm, provider, signal, ContextCompat.getMainExecutor(activity)) { loc ->
                    cont.resume(loc?.let { GeoFix(it.latitude, it.longitude, it.accuracy.toDouble()) })
                }
            }
        }
    }
}
