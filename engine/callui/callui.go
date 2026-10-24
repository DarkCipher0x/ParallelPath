// Package callui is the call page: WebRTC with an extra end-to-end encryption layer on every audio and video frame.
// The same files run in the web app (served by ppserver), in the Windows app (in an Edge app window) and in the
// Android app (in a WebView); the Windows and Android engines serve them on this device's loopback address.
package callui

import (
	"embed"
	"io/fs"
)

//go:embed web
var files embed.FS

// Files are the call page's files (index.html, call.js, frames.js, call.css).
func Files() fs.FS { sub, _ := fs.Sub(files, "web"); return sub }
