package systems.darkcipher.parallelpath.android

import android.app.Application

/** Keeps one engine for the whole process (survives activity re-creation, e.g. rotation). */
class PPApp : Application() {
    var transport: GoTransport? = null
}
