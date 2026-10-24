// ppengine is the ParallelPath client engine as a separate process, used by the Windows app.
//
// The app starts it and talks to it over its own stdin/stdout pipes (private to the two processes): one JSON
// object per line.
//
//	app -> engine   {"id": 1, "method": "messages", "args": {"space": "..."}}
//	engine -> app   {"id": 1, "result": [...]}   or   {"id": 1, "error": "..."}
//	engine -> app   {"event": "changed", "data": {...}}
//
// Keeping the engine in its own process means keys and decrypted content never live in the UI process. When the
// app exits (stdin closes) the engine locks the vault, wipes its keys and exits too.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"

	"pp/engine/app"
)

var version = "dev"

type request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

type response struct {
	ID     uint64 `json:"id,omitempty"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	Event  string `json:"event,omitempty"`
	Data   any    `json:"data,omitempty"`
}

func main() {
	data := flag.String("data", "", "data directory (the app's private folder)")
	ver := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *ver {
		fmt.Println("ppengine", version)
		return
	}
	if *data == "" {
		fmt.Fprintln(os.Stderr, "ppengine: --data is required")
		os.Exit(2)
	}
	out := json.NewEncoder(os.Stdout)
	var outMu sync.Mutex
	send := func(r response) {
		outMu.Lock()
		defer outMu.Unlock()
		if err := out.Encode(r); err != nil {
			os.Exit(1) // the app is gone
		}
	}
	e := app.New(*data, func(event string, payload any) { send(response{Event: event, Data: payload}) })
	send(response{Event: "ready", Data: map[string]string{"version": version}})

	in := bufio.NewReaderSize(os.Stdin, 1<<20)
	var wg sync.WaitGroup
	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			var req request
			if json.Unmarshal(line, &req) != nil || req.ID == 0 {
				send(response{Event: "error", Data: map[string]string{"message": "malformed request"}})
			} else {
				wg.Add(1)
				go func(req request) {
					defer wg.Done()
					res, err := e.Call(req.Method, req.Args)
					if err != nil {
						send(response{ID: req.ID, Error: err.Error()})
					} else {
						send(response{ID: req.ID, Result: res})
					}
				}(req)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
	e.Lock()
	wg.Wait()
}
