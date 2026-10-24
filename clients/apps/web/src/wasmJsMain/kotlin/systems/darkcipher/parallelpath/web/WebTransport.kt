package systems.darkcipher.parallelpath.web

import kotlinx.coroutines.await
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.serialization.json.Json
import systems.darkcipher.parallelpath.engine.EngineEvent
import systems.darkcipher.parallelpath.engine.EngineException
import systems.darkcipher.parallelpath.engine.EngineTransport

/** The engine in the same page (WebAssembly): calls are promises, events arrive through ppOnEvent. */
class WebTransport : EngineTransport {
    private val flow = MutableSharedFlow<EngineEvent>(extraBufferCapacity = 64)

    init {
        ppSetEventSink { name, json ->
            flow.tryEmit(EngineEvent(name, runCatching { Json.parseToJsonElement(json) }.getOrElse { Json.parseToJsonElement("{}") }))
        }
    }

    override suspend fun call(method: String, argsJson: String): String {
        val r = ppCall(method, argsJson).await<JsString>().toString()
        if (r.startsWith("1")) return r.substring(1)
        throw EngineException(r.substring(1).removePrefix("Error: ").ifBlank { "something went wrong" })
    }

    override val events: Flow<EngineEvent> get() = flow
}
