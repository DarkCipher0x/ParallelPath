// Package mobile is the gomobile facade of the client engine for Android (and iOS later). gomobile only passes
// simple types, so everything goes through the same JSON Call interface as the Windows engine process.
package mobile

import (
	"encoding/json"

	"pp/engine/app"
)

// Listener receives engine events on the app side.
type Listener interface {
	OnEvent(event string, json string)
}

// Engine wraps the client engine.
type Engine struct{ e *app.Engine }

// NewEngine creates the engine with its data in dir (the app's private files directory).
func NewEngine(dir string, l Listener) *Engine {
	return &Engine{e: app.New(dir, func(event string, payload any) {
		if l == nil {
			return
		}
		b, _ := json.Marshal(payload)
		l.OnEvent(event, string(b))
	})}
}

// Call runs an engine method with JSON arguments and returns the JSON result.
func (m *Engine) Call(method, args string) (string, error) {
	res, err := m.e.Call(method, json.RawMessage(args))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(res)
	return string(b), err
}

// Lock wipes keys and decrypted data (call when the app goes to the background for too long).
func (m *Engine) Lock() { m.e.Lock() }
