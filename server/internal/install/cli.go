package install

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"pp/server/internal/serverinfo"
)

// Command is `ppserver install`.
func Command(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	planFile := fs.String("plan", "", "unattended: JSON plan file, or - to read it from stdin")
	asJSON := fs.Bool("json", false, "machine-readable progress, one JSON object per line")
	probe := fs.Bool("probe", false, "only report what this machine has, as JSON")
	data := fs.String("data", "", "with --probe: the data folder to check")
	port := fs.Int("port", 0, "with --probe: the port to check")
	PauseFlag(fs)
	fs.Parse(args)
	switch {
	case *probe:
		return json.NewEncoder(os.Stdout).Encode(RunProbe(*data, *port))
	case *planFile != "":
		p, err := readPlan(*planFile)
		if err != nil {
			return err
		}
		if *asJSON {
			r := &jsonReporter{w: os.Stdout}
			res, err := Install(p, r)
			if err != nil {
				r.emit(map[string]any{"type": "error", "error": err.Error()})
				return err
			}
			r.emit(map[string]any{"type": "result", "result": res})
			return nil
		}
		res, err := Install(p, newTextReporter(os.Stdout))
		if err != nil {
			return err
		}
		printResult(os.Stdout, res)
		return nil
	default:
		ElevateIfNeeded()
		return interactive()
	}
}

func readPlan(name string) (*Plan, error) {
	var r io.Reader = os.Stdin
	if name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, 64<<10))
	if err != nil {
		return nil, err
	}
	// Windows tools (PowerShell pipes, Notepad) often start text with a UTF-8 byte-order mark.
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
	dec.DisallowUnknownFields()
	var p Plan
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	return &p, nil
}

// UninstallCommand is `ppserver uninstall`.
func UninstallCommand(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	purge := fs.Bool("purge", false, "also delete the data folder, backups and keys (cannot be undone)")
	yes := fs.Bool("yes", false, "don't ask for confirmation")
	PauseFlag(fs)
	fs.Parse(args)
	ElevateIfNeeded()
	e := existingInstall()
	if e == nil {
		return errors.New("ParallelPath Server is not installed on this machine")
	}
	ui := newTUI()
	ui.banner("Uninstall")
	fmt.Printf("  Installed: version %s, data in %s\n\n", e.Version, e.DataDir)
	if !*yes {
		if !ui.isTerminal() {
			return errors.New("run in a terminal, or add --yes")
		}
		if !*purge {
			*purge = ui.confirm("Also delete ALL data, backups and keys? This cannot be undone", false)
			if *purge && !ui.confirm("Really delete everything - every family's messages, files and audit logs", false) {
				*purge = false
			}
		}
		what := "Remove ParallelPath Server (data is kept)"
		if *purge {
			what = "Remove ParallelPath Server and DELETE all its data"
		}
		if !ui.confirm(what+"?", false) {
			return errors.New("cancelled")
		}
	}
	return uninstall(e, *purge, newTextReporter(os.Stdout))
}

// StatusCommand is `ppserver status`.
func StatusCommand(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	PauseFlag(fs)
	fs.Parse(args)
	ElevateIfNeeded()
	e := existingInstall()
	if e == nil {
		return errors.New("ParallelPath Server is not installed on this machine")
	}
	t := newTUI()
	t.banner("Status")
	t.kv("Version", e.Version)
	t.kv("Service", e.State)
	t.kv("Data folder", e.DataDir)
	t.kv("Logs", logsHint(e.DataDir))
	if info, err := serverinfo.Read(e.DataDir); err == nil && e.State == "running" {
		t.kv("Name", info.Name)
		t.kv("Encryption", info.AtRest)
		t.kv("Certificate", groupHex(info.CertSHA256))
		t.kv("Server key", groupHex(info.ServerKey))
		_, port, _ := net.SplitHostPort(info.Listen)
		for _, h := range info.Hosts {
			t.kv("Address", net.JoinHostPort(h, port))
		}
	}
	t.section("Recent log")
	for _, l := range strings.Split(recentLog(e.DataDir), "\n") {
		t.p("  %s%s%s\n", cDim, l, cReset)
	}
	return nil
}

// InstalledDataDir is the installed server's data folder, or "" when none is installed.
func InstalledDataDir() string {
	if e := existingInstall(); e != nil {
		return e.DataDir
	}
	return ""
}

var pause bool

// PauseFlag adds --pause (wait for Enter before exiting), for Start-menu shortcuts that open a console window.
func PauseFlag(fs *flag.FlagSet) {
	fs.BoolVar(&pause, "pause", false, "wait for Enter before closing")
}

// PauseIfAsked waits for Enter when --pause was given.
func PauseIfAsked() {
	if pause {
		fmt.Print("\nPress Enter to close.")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
}

// jsonReporter writes one JSON object per line, for the setup wizard.
type jsonReporter struct {
	mu sync.Mutex
	w  io.Writer
}

func (r *jsonReporter) emit(v map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, _ := json.Marshal(v)
	r.w.Write(append(b, '\n'))
}
func (r *jsonReporter) Step(id, title string) {
	r.emit(map[string]any{"type": "step", "id": id, "title": title})
}
func (r *jsonReporter) Done(id, detail string) {
	r.emit(map[string]any{"type": "done", "id": id, "detail": detail})
}
func (r *jsonReporter) Skip(id, title, reason string) {
	r.emit(map[string]any{"type": "skip", "id": id, "title": title, "reason": reason})
}
func (r *jsonReporter) Fail(id string, err error) {
	r.emit(map[string]any{"type": "fail", "id": id, "error": err.Error()})
}
func (r *jsonReporter) Note(text string) { r.emit(map[string]any{"type": "note", "text": text}) }

// groupHex splits a long hex string into groups of 4 for reading aloud.
func groupHex(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}

// ElevateIfNeeded restarts the command through UAC on Windows when it isn't running as administrator (the
// elevated copy opens its own window), then exits this one.
func ElevateIfNeeded() {
	if relaunchElevated() {
		os.Exit(0)
	}
}

// ServiceCommand is `ppserver service start|stop|restart` (the Windows Manager uses it).
func ServiceCommand(args []string) error {
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	PauseFlag(fs)
	fs.Parse(args)
	if fs.NArg() != 1 {
		return errors.New("usage: ppserver service start|stop|restart")
	}
	ElevateIfNeeded()
	if existingInstall() == nil {
		return errors.New("ParallelPath Server is not installed on this machine")
	}
	return serviceControl(fs.Arg(0))
}
