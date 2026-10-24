package systems.darkcipher.parallelpath.web

import kotlin.js.Promise

// The page's host script (resources/host.js): the Go engine in WebAssembly, an in-memory file system for picked and
// decrypted files, IndexedDB for the (sealed) vault, and small browser helpers. Bytes cross as base64.

external fun ppCall(method: String, args: String): Promise<JsString> // "1" + result JSON, or "0" + error message
external fun ppSetEventSink(sink: (String, String) -> Unit)
external fun ppPickFiles(): Promise<JsString> // JSON: [{path, name, mime}]
external fun ppOpenFile(path: String, mime: String)
external fun ppSaveFile(path: String, name: String, mime: String): Boolean // a download
external fun ppReadFile(path: String): JsString? // base64
external fun ppCopy(text: String)
external fun ppCanLocate(): Boolean
external fun ppLocate(): Promise<JsString> // JSON {lat, lon, accuracy} or ""
external fun ppGetPref(key: String): JsString?
external fun ppSetPref(key: String, value: String)
external fun ppDeviceName(): String
external fun ppOrigin(): String
external fun ppOpenCall(url: String)
external fun ppNotify(title: String, text: String, tag: String) // skipped while the tab is visible
external fun ppPrepareNotify()
external fun ppTimeZone(): String
