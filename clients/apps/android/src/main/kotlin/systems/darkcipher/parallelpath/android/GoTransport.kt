package systems.darkcipher.parallelpath.android

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import systems.darkcipher.parallelpath.engine.EngineEvent
import systems.darkcipher.parallelpath.engine.EngineException
import systems.darkcipher.parallelpath.engine.EngineTransport
import systems.darkcipher.parallelpath.gocore.mobile.Listener
import systems.darkcipher.parallelpath.gocore.mobile.Mobile
import java.io.File

/** The Go engine in-process (gomobile). All cryptography happens inside it. */
class GoTransport(dir: File) : EngineTransport {
    private val flow = MutableSharedFlow<EngineEvent>(extraBufferCapacity = 64)
    override val events: SharedFlow<EngineEvent> = flow
    private val json = Json

    private val engine = Mobile.newEngine(dir.absolutePath, object : Listener {
        override fun onEvent(event: String, json: String) {
            val data = runCatching { this@GoTransport.json.parseToJsonElement(json) }.getOrDefault(JsonNull)
            flow.tryEmit(EngineEvent(event, data))
        }
    })

    override suspend fun call(method: String, argsJson: String): String = withContext(Dispatchers.IO) {
        try {
            engine.call(method, argsJson)
        } catch (e: Exception) {
            throw EngineException(e.message ?: "engine error")
        }
    }

    fun lock() = engine.lock()
}
