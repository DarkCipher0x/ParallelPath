// Command pppeer is a scripted family member for testing a real app against: it starts a family on a test server,
// invites a second parent, approves them, then - when told to, by files appearing in its folder - sends a message
// and starts a video call. The call window is headless Edge with a fake camera and microphone, served by this
// engine's own call bridge exactly as the Windows app does it. Development only; fictional data; loopback servers.
//
//	pppeer --server https://127.0.0.1:18443 --invite S.... --dir C:\temp\peer
//
// Then: create <dir>\go-message to send a message, <dir>\go-call to start a video call, <dir>\stop to finish.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"pp/engine/app"
)

func main() {
	server := flag.String("server", "", "server URL")
	invite := flag.String("invite", "", "server invite code (S.)")
	dir := flag.String("dir", "", "working folder (engine data, invite code, triggers)")
	edge := flag.String("edge", `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, "Edge, for the call window")
	flag.Parse()
	if *server == "" || *invite == "" || *dir == "" {
		flag.Usage()
		os.Exit(2)
	}
	logf := func(f string, a ...any) { fmt.Printf(time.Now().Format("15:04:05 ")+f+"\n", a...) }
	e := app.New(filepath.Join(*dir, "engine"), func(ev string, p any) {
		if ev == "call" || ev == "notify" || ev == "error" {
			b, _ := json.Marshal(p)
			logf("event %s %s", ev, b)
		}
	})
	defer e.Lock()
	const pass = "peer test passphrase"
	var space string
	approved := false
	if raw, err := os.ReadFile(filepath.Join(*dir, "space.txt")); err == nil { // a second run: same family
		must(e.Unlock([]byte(pass), nil))
		space, approved = string(raw), true
		logf("unlocked; family %s", space)
	} else {
		p, err := e.Preview(*server)
		must(err)
		res, err := e.Setup(app.SetupRequest{Server: *server, CertSHA256: p.CertSHA256, Invite: *invite, Passphrase: pass,
			YourName: "Sam", DeviceName: "Test PC", DeviceKind: "windows", FamilyName: "Mia & Leo"})
		must(err)
		space = res.SpaceID
		must(os.WriteFile(filepath.Join(*dir, "space.txt"), []byte(space), 0o600))
		code, _, err := e.Invite(space, "parent", time.Time{}, time.Time{}, nil)
		must(err)
		must(os.WriteFile(filepath.Join(*dir, "invite.txt"), []byte(code), 0o600))
		logf("family ready; parent invite written to invite.txt")
	}

	exists := func(n string) bool { _, err := os.Stat(filepath.Join(*dir, n)); return err == nil }
	messaged := false
	var window *exec.Cmd // the call window of the latest call
	for !exists("stop") {
		time.Sleep(time.Second)
		e.SyncAll()
		if !approved {
			s, _ := e.Spaces()
			for _, sp := range s {
				for _, m := range sp.Members {
					if sp.ID == space && m.NeedsKeys && !m.Me {
						n, _ := e.SafetyNumber(m.Person)
						logf("approving %s (safety number %s)", m.Name, n)
						must(e.Approve(space, m.Person))
						approved = true
					}
				}
			}
		}
		if approved && !messaged && exists("go-message") {
			must(e.Send(app.SendRequest{Space: space, Text: "Pick-up from school is at 3:30 tomorrow."}))
			logf("message sent")
			messaged = true
		}
		if approved && exists("go-call") {
			_ = os.Remove(filepath.Join(*dir, "go-call")) // one call per trigger; a new trigger ends the last window
			closeWindow(window)
			c, err := e.CallStart(space, true)
			must(err)
			bridge, err := e.CallBridge()
			must(err)
			// test=1: the call page's synthetic camera (headless Edge's fake camera sends next to nothing).
			url := fmt.Sprintf("%s#space=%s&call=%s&role=caller&test=1", bridge, space, c["call"])
			profile := filepath.Join(*dir, "edge")
			cmd := exec.Command(*edge, "--headless=new", "--user-data-dir="+profile, "--no-first-run", "--use-fake-ui-for-media-stream",
				"--use-fake-device-for-media-stream", "--autoplay-policy=no-user-gesture-required", "--remote-debugging-address=127.0.0.1",
				"--remote-debugging-port=19333", "--window-size=1280,800", url)
			must(cmd.Start())
			window = cmd
			logf("call %s started; call window running headless (DevTools on 127.0.0.1:19333)", c["call"])
		}
	}
	closeWindow(window)
	logf("stopping")
}

// closeWindow ends a headless Edge call window with all its child processes.
func closeWindow(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
		_ = cmd.Wait()
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "pppeer:", err)
		os.Exit(1)
	}
}
