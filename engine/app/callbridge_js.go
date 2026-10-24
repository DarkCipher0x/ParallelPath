//go:build js

package app

// callBridge isn't needed in the web app: the call page is served by the server next to the app and talks to the
// engine in the same browser tab.
type callBridge struct{}

func (b *callBridge) close() {}

// CallBridge returns the call page's address in the web app.
func (e *Engine) CallBridge() (string, error) { return "call/", nil }
