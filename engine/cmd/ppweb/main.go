//go:build js && wasm

// ppweb is the client engine compiled to WebAssembly for the web app: the same engine and the same JSON Call
// interface as the Windows engine process and the Android library, so the browser does all the end-to-end
// cryptography itself and the server still only ever sees ciphertext.
//
// The page (web/host.js) provides:
//   - globalThis.fs: an in-memory file system for picked files, the decrypted-file cache and temp files;
//   - ppPersist(dir, bucket, key, bytes|null): mirrors vault changes into IndexedDB (values are already sealed);
//   - ppOnEvent(name, json): engine events ("changed", "connection", "error").
//
// It exposes:
//   - ppPreload(dir, bucket, key, bytes): load one stored vault value (before ppStart);
//   - ppStart(dir): create the engine;
//   - ppGoCall(method, argsJSON) -> Promise<resultJSON> (rejected with the error message).
package main

import (
	"encoding/json"
	"syscall/js"

	"pp/engine/app"
	"pp/engine/vault"
)

func bytesOf(v js.Value) []byte {
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

func main() {
	g := js.Global()
	var eng *app.Engine

	vault.Persist = func(dir, bucket, key string, val []byte) {
		var arg any // null deletes
		if val != nil {
			a := g.Get("Uint8Array").New(len(val))
			js.CopyBytesToJS(a, val)
			arg = a
		}
		g.Call("ppPersist", dir, bucket, key, arg)
	}

	g.Set("ppPreload", js.FuncOf(func(_ js.Value, a []js.Value) any {
		vault.Preload(a[0].String(), a[1].String(), a[2].String(), bytesOf(a[3]))
		return nil
	}))

	g.Set("ppStart", js.FuncOf(func(_ js.Value, a []js.Value) any {
		eng = app.New(a[0].String(), func(event string, payload any) {
			b, _ := json.Marshal(payload)
			g.Call("ppOnEvent", event, string(b))
		})
		return nil
	}))

	g.Set("ppGoCall", js.FuncOf(func(_ js.Value, a []js.Value) any {
		method, args := a[0].String(), a[1].String()
		return g.Get("Promise").New(js.FuncOf(func(_ js.Value, p []js.Value) any {
			resolve, reject := p[0], p[1]
			go func() { // engine calls block on the network: never on the browser's event loop
				if eng == nil {
					reject.Invoke("the engine hasn't started")
					return
				}
				res, err := eng.Call(method, json.RawMessage(args))
				if err != nil {
					reject.Invoke(err.Error())
					return
				}
				b, err := json.Marshal(res)
				if err != nil {
					reject.Invoke(err.Error())
					return
				}
				resolve.Invoke(string(b))
			}()
			return nil
		}))
	}))

	g.Call("ppReady")
	select {} // keep the engine alive for the life of the page
}
