package systems.darkcipher.parallelpath.desktop

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import systems.darkcipher.parallelpath.engine.EngineEvent
import systems.darkcipher.parallelpath.engine.EngineException
import systems.darkcipher.parallelpath.engine.EngineTransport
import java.io.BufferedWriter
import java.io.File
import java.io.OutputStreamWriter
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong
import kotlin.concurrent.thread

/**
 * Runs the Go engine (ppengine.exe) as a child process and talks to it over its stdin/stdout - pipes only these
 * two processes can use. Keys and decrypted content stay in the engine process, never in this one's heap.
 * If this app exits, the engine sees stdin close, locks and wipes its keys, and exits.
 */
class ProcessTransport(engineExe: File, dataDir: File) : EngineTransport {
    private val json = Json { ignoreUnknownKeys = true }
    private val proc: Process = ProcessBuilder(engineExe.absolutePath, "--data", dataDir.absolutePath)
        .redirectError(ProcessBuilder.Redirect.DISCARD)
        .start()
    private val out = BufferedWriter(OutputStreamWriter(proc.outputStream, Charsets.UTF_8))
    private val pending = ConcurrentHashMap<Long, CompletableDeferred<String>>()
    private val ids = AtomicLong()
    private val flow = MutableSharedFlow<EngineEvent>(extraBufferCapacity = 64)
    override val events: SharedFlow<EngineEvent> = flow

    init {
        thread(isDaemon = true, name = "ppengine-reader") {
            proc.inputStream.bufferedReader(Charsets.UTF_8).forEachLine { line -> handle(line) }
            val gone = EngineException("The ParallelPath engine stopped. Restart the app.")
            pending.values.forEach { it.completeExceptionally(gone) }
            pending.clear()
        }
    }

    private fun handle(line: String) {
        val obj = runCatching { json.parseToJsonElement(line) as JsonObject }.getOrNull() ?: return
        val id = (obj["id"] as? JsonPrimitive)?.longOrNull
        if (id != null) {
            val d = pending.remove(id) ?: return
            val err = (obj["error"] as? JsonPrimitive)?.content
            if (err != null) d.completeExceptionally(EngineException(err))
            else d.complete(obj["result"]?.toString() ?: "null")
            return
        }
        val name = obj["event"]?.jsonPrimitive?.content ?: return
        flow.tryEmit(EngineEvent(name, obj["data"] ?: JsonNull))
    }

    override suspend fun call(method: String, argsJson: String): String {
        val id = ids.incrementAndGet()
        val d = CompletableDeferred<String>()
        pending[id] = d
        withContext(Dispatchers.IO) {
            synchronized(out) {
                out.write("""{"id":$id,"method":${JsonPrimitive(method)},"args":$argsJson}""")
                out.newLine()
                out.flush()
            }
        }
        return d.await()
    }

    /** Closes the engine: it locks, wipes its keys and exits. */
    fun close() {
        runCatching { out.close() }
        if (!proc.waitFor(5, java.util.concurrent.TimeUnit.SECONDS)) proc.destroy()
    }
}
