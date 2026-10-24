package systems.darkcipher.parallelpath.android

import android.Manifest
import android.annotation.SuppressLint
import android.content.pm.PackageManager
import android.graphics.Color
import android.media.AudioManager
import android.net.Uri
import android.os.Bundle
import android.view.WindowManager
import android.webkit.PermissionRequest
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat

/**
 * A call: the shared call page (WebRTC with end-to-end encrypted frames) in a WebView, talking to the in-process
 * engine over this device's loopback address. It can only ever show that page; camera and microphone are granted
 * to it when the person allows them, and the screen stays on and can't be captured.
 */
class CallActivity : ComponentActivity() {
    private lateinit var web: WebView
    private var pendingRequest: PermissionRequest? = null

    private val askMedia = registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { _ ->
        pendingRequest?.let { grant(it) }
        pendingRequest = null
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val url = intent.getStringExtra("url") ?: return finish()
        val origin = Uri.parse(url).let { "${it.scheme}://${it.authority}" }
        if (!origin.startsWith("http://127.0.0.1:")) return finish()
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        if (applicationInfo.flags and android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE == 0) {
            window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)
        } else {
            WebView.setWebContentsDebuggingEnabled(true) // debug builds only: inspect the call page over adb
        }
        volumeControlStream = AudioManager.STREAM_VOICE_CALL
        web = WebView(this).apply {
            setBackgroundColor(Color.parseColor("#0b1517"))
            settings.javaScriptEnabled = true
            settings.mediaPlaybackRequiresUserGesture = false
            settings.allowFileAccess = false
            settings.allowContentAccess = false
            webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean =
                    !request.url.toString().startsWith(origin) // nothing but the call page
            }
            webChromeClient = object : WebChromeClient() {
                override fun onPermissionRequest(request: PermissionRequest) {
                    if (!request.origin.toString().startsWith(origin)) return request.deny()
                    val needed = arrayOf(Manifest.permission.CAMERA, Manifest.permission.RECORD_AUDIO)
                    if (needed.all { ContextCompat.checkSelfPermission(this@CallActivity, it) == PackageManager.PERMISSION_GRANTED }) {
                        grant(request)
                    } else {
                        pendingRequest = request
                        askMedia.launch(needed)
                    }
                }

                override fun onCloseWindow(window: WebView?) = finish()
            }
        }
        setContentView(web)
        (getSystemService(AUDIO_SERVICE) as AudioManager).mode = AudioManager.MODE_IN_COMMUNICATION
        web.loadUrl(url)
    }

    /** Grants the page whichever of camera and microphone the person allowed. */
    private fun grant(request: PermissionRequest) {
        val ok = request.resources.filter {
            when (it) {
                PermissionRequest.RESOURCE_VIDEO_CAPTURE -> has(Manifest.permission.CAMERA)
                PermissionRequest.RESOURCE_AUDIO_CAPTURE -> has(Manifest.permission.RECORD_AUDIO)
                else -> false
            }
        }
        if (ok.isEmpty()) request.deny() else request.grant(ok.toTypedArray())
    }

    private fun has(p: String) = ContextCompat.checkSelfPermission(this, p) == PackageManager.PERMISSION_GRANTED

    override fun onDestroy() {
        (getSystemService(AUDIO_SERVICE) as AudioManager).mode = AudioManager.MODE_NORMAL
        if (::web.isInitialized) {
            web.loadUrl("about:blank") // the page's pagehide hangs up if the call is still going
            web.destroy()
        }
        super.onDestroy()
    }
}
