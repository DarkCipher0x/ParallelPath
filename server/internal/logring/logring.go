// Package logring keeps the server's most recent log lines in memory, so the admin portal can show them on any
// system (on Linux the log otherwise lives in the journal, which the server's own account can't read).
package logring

import (
	"bytes"
	"sync"
)

// Ring is an io.Writer that remembers the last N lines written to it.
type Ring struct {
	mu    sync.Mutex
	lines []string
	next  int
	full  bool
	part  []byte
}

// New keeps the last n lines.
func New(n int) *Ring { return &Ring{lines: make([]string, n)} }

// Write stores complete lines (a log record is one line).
func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.part = append(r.part, p...)
	for {
		i := bytes.IndexByte(r.part, '\n')
		if i < 0 {
			break
		}
		r.lines[r.next] = string(r.part[:i])
		r.next = (r.next + 1) % len(r.lines)
		r.full = r.full || r.next == 0
		r.part = r.part[i+1:]
	}
	if len(r.part) > 64<<10 { // a runaway line without an end
		r.part = r.part[:0]
	}
	return len(p), nil
}

// Lines returns the remembered lines, oldest first.
func (r *Ring) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]string(nil), r.lines[:r.next]...)
	}
	return append(append([]string(nil), r.lines[r.next:]...), r.lines[:r.next]...)
}
