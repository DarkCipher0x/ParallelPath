package install

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/term"
)

// The terminal installer, in ParallelPath's colours (24-bit; plain text when NO_COLOR is set or output isn't a
// terminal).
var (
	cAccent = "\x1b[38;2;61;214;208m"
	cWarm   = "\x1b[38;2;242;166;90m"
	cOK     = "\x1b[38;2;88;214;141m"
	cBad    = "\x1b[38;2;242;102;122m"
	cDim    = "\x1b[38;2;138;164;167m"
	cBold   = "\x1b[1m"
	cReset  = "\x1b[0m"
)

type tui struct {
	in    *bufio.Reader
	out   io.Writer
	color bool
}

func newTUI() *tui {
	return &tui{in: bufio.NewReader(os.Stdin), out: os.Stdout, color: initColors()}
}

var (
	colorOnce sync.Once
	colorOn   bool
)

// initColors decides once whether output is a terminal that understands colours and cursor control.
func initColors() bool {
	colorOnce.Do(func() {
		colorOn = term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("NO_COLOR") == "" && enableVT()
		if !colorOn {
			cAccent, cWarm, cOK, cBad, cDim, cBold, cReset = "", "", "", "", "", "", ""
		}
	})
	return colorOn
}

func (t *tui) isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func (t *tui) p(format string, a ...any) { fmt.Fprintf(t.out, format, a...) }

func (t *tui) banner(sub string) {
	const inner = 58
	pad := func(s string, w int) string {
		if n := len([]rune(s)); n < w {
			return s + strings.Repeat(" ", w-n)
		}
		return s
	}
	line := strings.Repeat("─", inner)
	t.p("\n  %s╭%s╮%s\n", cAccent, line, cReset)
	t.p("  %s│%s  %sParallel%s%s%sPath%s  %s%s%s%s│%s\n", cAccent, cReset, cBold, cReset, cBold, cAccent, cReset,
		cDim, pad(sub+"  "+Version, inner-16), cReset, cAccent, cReset)
	t.p("  %s│%s  %s%s│%s\n", cAccent, cReset, pad("Post-quantum secure co-parenting server", inner-2), cAccent, cReset)
	t.p("  %s│%s  %s%s%s%s│%s\n", cAccent, cReset, cDim, pad("ML-KEM-1024 · ML-DSA-87 · AES-256-GCM · SHA3-512", inner-2),
		cReset, cAccent, cReset)
	t.p("  %s╰%s╯%s\n", cAccent, line, cReset)
	t.p("  %sBuilt by DarkCipher0x at DarkCipher Systems · AGPL-3.0%s\n\n", cDim, cReset)
}

func (t *tui) section(title string) {
	t.p("\n  %s%s%s%s\n  %s%s%s\n", cBold, cAccent, title, cReset, cDim, strings.Repeat("─", len([]rune(title))), cReset)
}

func (t *tui) kv(k, v string) { t.p("  %s%-13s%s %s\n", cDim, k, cReset, v) }

func (t *tui) checks(cs []Check) {
	for _, c := range cs {
		mark := map[string]string{"ok": cOK + "✓", "warn": cWarm + "!", "fail": cBad + "✗", "info": cAccent + "i"}[c.State]
		t.p("  %s%s %-36s %s%s%s\n", mark, cReset, c.Title, cDim, c.Detail, cReset)
	}
}

// ask reads a line, offering def (Enter keeps it), until check accepts it.
func (t *tui) ask(label, def string, check func(string) error) string {
	for {
		if def != "" {
			t.p("  %s›%s %s %s[%s]%s: ", cAccent, cReset, label, cDim, def, cReset)
		} else {
			t.p("  %s›%s %s: ", cAccent, cReset, label)
		}
		s, err := t.in.ReadString('\n')
		if err != nil {
			t.p("\n")
			os.Exit(1)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			s = def
		}
		if check == nil {
			return s
		}
		if err := check(s); err != nil {
			t.p("    %s%v%s\n", cBad, err, cReset)
			continue
		}
		return s
	}
}

func (t *tui) askInt(label string, def, lo, hi int) int {
	s := t.ask(label, strconv.Itoa(def), func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n < lo || n > hi {
			return fmt.Errorf("enter a number from %d to %d", lo, hi)
		}
		return nil
	})
	n, _ := strconv.Atoi(s)
	return n
}

func (t *tui) confirm(label string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		t.p("  %s?%s %s %s[%s]%s ", cWarm, cReset, label, cDim, hint, cReset)
		s, err := t.in.ReadString('\n')
		if err != nil {
			t.p("\n")
			os.Exit(1)
		}
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
	}
}

// feature is one tick box.
type feature struct {
	label, help string
	on          *bool
}

// checklist lets the person tick features with the arrow keys and space; Enter accepts.
func (t *tui) checklist(items []feature) error {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return t.checklistByNumber(items)
	}
	defer term.Restore(fd, old)
	cur := 0
	draw := func(first bool) {
		if !first {
			t.p("\x1b[%dA", len(items)*2+1)
		}
		for i, it := range items {
			box := cDim + "[ ]" + cReset
			if *it.on {
				box = cAccent + "[✓]" + cReset
			}
			ptr := "  "
			label := it.label
			if i == cur {
				ptr, label = cAccent+"›"+cReset+" ", cBold+label+cReset
			}
			t.p("\x1b[2K  %s%s %s\r\n\x1b[2K        %s%s%s\r\n", ptr, box, label, cDim, it.help, cReset)
		}
		t.p("\x1b[2K  %s↑/↓ move · space tick/untick · Enter continue%s\r\n", cDim, cReset)
	}
	draw(true)
	buf := make([]byte, 8)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}
		k := string(buf[:n])
		switch {
		case k == "\x1b[A" || k == "k":
			cur = (cur + len(items) - 1) % len(items)
		case k == "\x1b[B" || k == "j" || k == "\t":
			cur = (cur + 1) % len(items)
		case k == " " || k == "x":
			*items[cur].on = !*items[cur].on
		case k == "\r" || k == "\n":
			return nil
		case k == "\x03" || k == "q":
			term.Restore(fd, old)
			t.p("\n")
			os.Exit(1)
		}
		draw(false)
	}
}

// checklistByNumber is the fallback for terminals without raw mode: type a number to toggle it.
func (t *tui) checklistByNumber(items []feature) error {
	for {
		for i, it := range items {
			box := "[ ]"
			if *it.on {
				box = "[x]"
			}
			t.p("  %d. %s %s\n", i+1, box, it.label)
		}
		s := t.ask("Number to tick/untick, or Enter to continue", "", nil)
		if s == "" {
			return nil
		}
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(items) {
			*items[n-1].on = !*items[n-1].on
		}
	}
}

// textReporter prints the steps as they run.
// On a terminal the running step shows as "…" and is replaced by its result; otherwise only results are printed.
type textReporter struct {
	out   io.Writer
	live  bool
	title string
}

func newTextReporter(w io.Writer) *textReporter { return &textReporter{out: w, live: initColors()} }

func (r *textReporter) Step(_, title string) {
	r.title = title
	if r.live {
		fmt.Fprintf(r.out, "  %s…%s %s", cAccent, cReset, title)
	}
}
func (r *textReporter) Done(_, detail string) {
	fmt.Fprintf(r.out, "%s  %s✓%s %s  %s%s%s\n", r.clear(), cOK, cReset, r.title, cDim, detail, cReset)
	r.title = ""
}
func (r *textReporter) Skip(_, title, reason string) {
	fmt.Fprintf(r.out, "  %s–%s %s%s (%s)%s\n", cDim, cReset, cDim, title, reason, cReset)
}
func (r *textReporter) Fail(_ string, err error) {
	fmt.Fprintf(r.out, "%s  %s✗ %s%s\n", r.clear(), cBad, r.title, cReset)
	r.title = ""
	for _, l := range strings.Split(err.Error(), "\n") {
		fmt.Fprintf(r.out, "      %s\n", l)
	}
}

// Note prints above the running step: the "…" line is wiped, the note printed, and the step line drawn again.
func (r *textReporter) Note(text string) {
	fmt.Fprintf(r.out, "%s  %si%s %s\n", r.clear(), cAccent, cReset, text)
	if r.live && r.title != "" {
		fmt.Fprintf(r.out, "  %s…%s %s", cAccent, cReset, r.title)
	}
}

func printResult(w io.Writer, res *Result) {
	fmt.Fprintf(w, "\n  %s%sParallelPath Server is running%s\n\n", cBold, cOK, cReset)
	row := func(k, v string) { fmt.Fprintf(w, "  %s%-14s%s %s\n", cDim, k, cReset, v) }
	row("Name", res.Name)
	row("Version", res.Version)
	row("Encryption", res.AtRest)
	for _, a := range res.Addresses {
		row("Address", a)
	}
	fmt.Fprintf(w, "\n  %sThe apps show these when someone connects - they must match:%s\n", cDim, cReset)
	row("Certificate", groupHex(res.CertSHA256))
	row("Server key", groupHex(res.ServerKey))
	if res.Invite != "" {
		fmt.Fprintf(w, "\n  %sFirst invite code%s (the person who creates the family space; works once, 7 days):\n\n", cBold, cReset)
		fmt.Fprintf(w, "      %s%s%s\n", cAccent, res.Invite, cReset)
	}
	fmt.Fprintf(w, "\n")
	row("Data", res.DataDir)
	row("Logs", res.Logs)
	if res.AdminLink != "" {
		fmt.Fprintf(w, "  %sAdmin portal%s - open this link to create your admin account (works once, within 24 hours; keep it private):\n\n",
			cBold, cReset)
		fmt.Fprintf(w, "      %s%s%s\n\n", cAccent, res.AdminLink, cReset)
	}
	row("More codes", "ppserver invite")
	row("Admin portal", "https://"+firstOr(res.Addresses, "this-server:8443")+"/admin/  (new link: sudo ppserver admin setup-link)")
	row("Uninstall", "ppserver uninstall")
	fmt.Fprintln(w)
}

// interactive is `ppserver install` in a terminal.
func interactive() error {
	t := newTUI()
	if !t.isTerminal() {
		return errors.New("run `ppserver install` in a terminal, or give a plan with --plan")
	}
	t.banner("Server Setup")
	if !isAdmin() {
		return errors.New(adminHint)
	}
	pr := RunProbe("", 0)
	p := pr.Defaults
	if e := pr.Existing; e != nil {
		t.section("Upgrade")
		t.kv("Installed", e.Version+" ("+e.State+")")
		t.kv("Data folder", e.DataDir)
		if !t.confirm(fmt.Sprintf("Upgrade to %s, keeping all data and settings?", Version), true) {
			return errors.New("cancelled")
		}
	} else if err := t.askPlan(p); err != nil {
		return err
	}
	t.section("Installing")
	res, err := Install(p, newTextReporter(t.out))
	if err != nil {
		return err
	}
	if p.Mode == "upgrade" {
		t.p("\n  %s%sUpgraded to %s%s\n\n", cBold, cOK, Version, cReset)
		return nil
	}
	printResult(t.out, res)
	return nil
}

func (t *tui) askPlan(p *Plan) error {
	t.section("This machine")
	t.checks(RunProbe(p.DataDir, p.Port).Checks)

	t.section("Server")
	p.Name = t.ask("Server name (shown in the apps)", p.Name, func(s string) error {
		q := *p
		q.Name = s
		return fieldErr(q.Validate(), "server name")
	})
	hosts := t.ask("Names / IP addresses people use to reach it (comma-separated)", strings.Join(p.Hosts, ","), func(s string) error {
		q := *p
		q.Hosts = splitList(s)
		return fieldErr(q.Validate(), "host", "IP address")
	})
	p.Hosts = splitList(hosts)
	for {
		p.Port = t.askInt("Port", p.Port, 1, 65535)
		if err := portFree(p.Port); err == nil {
			break
		} else {
			t.p("    %s%v%s\n", cBad, err, cReset)
		}
	}
	p.DataDir = t.ask("Data folder", p.DataDir, func(s string) error {
		q := *p
		q.DataDir = s
		if err := fieldErr(q.Validate(), "data folder"); err != nil {
			return err
		}
		_, err := checkDataDir(s)
		return err
	})
	if p.InstallDir != "" {
		p.InstallDir = t.ask("Program folder", p.InstallDir, nil)
	}

	t.section("Certificate (so browsers trust the web app)")
	t.p("  1  This server's own certificate authority - for home and office networks. Each device trusts it once\n")
	t.p("     (the server's /trust page explains how); the apps need nothing.\n")
	t.p("  2  Let's Encrypt - for a server with a public domain name pointing at it (ports 80 and %d reachable).\n", p.Port)
	t.p("  3  My own certificate files.\n")
	switch t.ask("Choose 1, 2 or 3", "1", func(s string) error {
		if s != "1" && s != "2" && s != "3" {
			return errors.New("choose 1, 2 or 3")
		}
		return nil
	}) {
	case "2":
		p.TLSMode = "acme"
		p.ACMEDomain = t.ask("Public domain name (e.g. family.example.org)", p.ACMEDomain, func(s string) error {
			q := *p
			q.ACMEDomain = s
			return fieldErr(q.Validate(), "domain")
		})
		p.ACMEEmail = t.ask("Email for expiry notices (optional)", p.ACMEEmail, nil)
	case "3":
		p.TLSMode = "files"
		p.CertFile = t.ask("Certificate file (PEM, full chain)", p.CertFile, nil)
		p.KeyFile = t.ask("Private key file (PEM)", p.KeyFile, nil)
	default:
		p.TLSMode = "local-ca"
	}

	encrypted := atRestOf(p.DataDir) == nil
	for {
		t.section("Features")
		if !encrypted {
			t.p("  %s! The data folder's volume isn't encrypted. %s%s\n\n", cWarm, encryptionAdvice, cReset)
		}
		if err := t.checklist(features(p, encrypted)); err != nil {
			return err
		}
		if encrypted || p.Container || p.Unencrypted {
			break
		}
		t.p("\n  %s✗ ParallelPath only runs on an encrypted volume. Tick the encrypted container, choose another data "+
			"folder, or (testing only) allow an unencrypted volume.%s\n", cBad, cReset)
	}
	if p.Backups {
		p.BackupDir = t.ask("Backup folder", p.BackupDir, nil)
		p.BackupKeep = t.askInt("Daily backups to keep", p.BackupKeep, 1, 365)
	}
	if p.Container {
		p.ContainerGB = t.askInt("Encrypted container size in GB", p.ContainerGB, 1, 16384)
	}
	if err := p.Validate(); err != nil {
		return err
	}

	t.section("Ready to install")
	t.kv("Name", p.Name)
	t.kv("Reachable at", strings.Join(p.Hosts, ", ")+" (port "+strconv.Itoa(p.Port)+")")
	t.kv("Data folder", p.DataDir)
	for _, f := range features(p, encrypted) {
		if *f.on {
			t.kv("✓", f.label)
		}
	}
	if !t.confirm("Install now?", true) {
		return errors.New("cancelled")
	}
	return nil
}

// fieldErr keeps only the validation errors about one field (the rest are asked separately).
func fieldErr(err error, words ...string) error {
	if err == nil {
		return nil
	}
	for _, l := range strings.Split(err.Error(), "\n") {
		for _, w := range words {
			if strings.Contains(l, w) {
				return errors.New(l)
			}
		}
	}
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, h := range strings.Split(s, ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}

func logsHint(data string) string {
	if logs := platformLogs(data); logs != "" {
		return logs
	}
	return data
}

// clear returns the sequence that wipes the "…" line on a terminal.
func (r *textReporter) clear() string {
	if r.live {
		return "\r\x1b[2K"
	}
	return ""
}

func firstOr(list []string, fallback string) string {
	if len(list) > 0 {
		return list[0]
	}
	return fallback
}
