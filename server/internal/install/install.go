// Package install sets up ppserver as a service on Windows and Ubuntu: the files, a dedicated low-privilege
// account, a locked-down data folder, the master key sealed by the operating system (DPAPI, or a systemd encrypted
// credential with the TPM where there is one), firewall rule, backups and the first invite code.
//
// Everything is driven by a Plan. The interactive terminal installer (tui.go) and the Windows setup wizard
// (which runs `ppserver install --plan - --json` elevated) both just build a Plan, so the work itself is done in
// exactly one place for every front end.
package install

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"pp/server/internal/admin"
	"pp/server/internal/atrest"
	"pp/server/internal/config"
	"pp/server/internal/serverinfo"
)

// Version is this build's version (set from main).
var Version = "dev"

const (
	// ServiceName is the Windows service; its virtual account is NT SERVICE\ParallelPathServer.
	ServiceName = "ParallelPathServer"
	// DisplayName is what people see in Services, Apps & features and the Start menu.
	DisplayName = "ParallelPath Server"
	// Publisher is shown in Apps & features.
	Publisher = "DarkCipher Systems"
)

// Plan is everything the installer needs to know. Defaults() fills it in for this machine.
type Plan struct {
	Mode        string   `json:"mode"` // "install", or "upgrade" (replace the program, keep data and settings)
	InstallDir  string   `json:"install_dir,omitempty"`
	DataDir     string   `json:"data_dir"`
	Name        string   `json:"name"`
	Hosts       []string `json:"hosts"`
	Port        int      `json:"port"`
	AutoStart   bool     `json:"autostart"`
	Firewall    bool     `json:"firewall"`
	LANOnly     bool     `json:"firewall_lan_only"` // Windows: Private and Domain networks only
	Backups     bool     `json:"backups"`
	BackupDir   string   `json:"backup_dir,omitempty"`
	BackupKeep  int      `json:"backup_keep,omitempty"`
	Shortcuts   bool     `json:"shortcuts"`              // Windows: Start-menu tools
	Container   bool     `json:"encrypted_container"`    // Linux: LUKS2 file for the data, for unencrypted disks
	ContainerGB int      `json:"container_gb,omitempty"` // its size
	Packages    bool     `json:"install_packages"`       // Linux: may apt-get install what's missing
	Unencrypted bool     `json:"allow_unencrypted"`      // testing only: allow a data volume without encryption
	FirstInvite bool     `json:"first_invite"`           // create the first account invite code at the end
	Tray        bool     `json:"tray"`                   // Windows: the ParallelPath Server Manager icon at sign-in
	ManagerExe  string   `json:"manager_exe,omitempty"`  // Windows: the Manager program to install (the setup itself)
	// HTTPS certificate: local-ca (the server's own CA - the default), acme (Let's Encrypt for ACMEDomain) or files.
	TLSMode    string `json:"tls_mode,omitempty"`
	ACMEDomain string `json:"acme_domain,omitempty"`
	ACMEEmail  string `json:"acme_email,omitempty"`
	CertFile   string `json:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"`
	TrustCA    bool   `json:"trust_ca"` // local-ca: trust it on this machine (browsers here then show no warning)
	Calls      bool   `json:"calls"`    // the call relay (TURN), and firewall rules for it
}

// Validate checks a plan before anything is changed.
func (p *Plan) Validate() error {
	var errs []error
	bad := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }
	if p.Mode != "install" && p.Mode != "upgrade" {
		bad("mode must be install or upgrade")
	}
	if !filepath.IsAbs(p.DataDir) || isRoot(p.DataDir) {
		bad("the data folder must be a full path, and not a drive or filesystem root")
	}
	if p.ManagerExe != "" {
		if st, err := os.Stat(p.ManagerExe); err != nil || !st.Mode().IsRegular() || !filepath.IsAbs(p.ManagerExe) ||
			!strings.EqualFold(filepath.Ext(p.ManagerExe), ".exe") {
			bad("the Manager program %q isn't a program file", p.ManagerExe)
		}
	}
	if p.Mode == "upgrade" {
		return errors.Join(errs...)
	}
	if n := strings.TrimSpace(p.Name); n == "" || len(n) > 80 || strings.IndexFunc(n, unicode.IsControl) >= 0 {
		bad("the server name must be 1-80 characters")
	}
	if len(p.Hosts) == 0 || len(p.Hosts) > 20 {
		bad("give between 1 and 20 host names or IP addresses")
	}
	for _, h := range p.Hosts {
		if !validHost(h) {
			bad("%q is not a host name or IP address", h)
		}
	}
	if p.Port < 1 || p.Port > 65535 || p.Port == 8444 {
		bad("the port must be 1-65535 (8444 is the server's private admin port)")
	}
	if p.Backups {
		if !filepath.IsAbs(p.BackupDir) || isRoot(p.BackupDir) {
			bad("the backup folder must be a full path, and not a drive or filesystem root")
		}
		if p.BackupKeep < 1 || p.BackupKeep > 365 {
			bad("keep between 1 and 365 backups")
		}
	}
	switch p.TLSMode {
	case "", "local-ca":
	case "acme":
		if !validHost(p.ACMEDomain) || net.ParseIP(p.ACMEDomain) != nil || !strings.Contains(p.ACMEDomain, ".") {
			bad("Let's Encrypt needs the server's public domain name (e.g. family.example.org)")
		}
	case "files":
		if !filepath.IsAbs(p.CertFile) || !filepath.IsAbs(p.KeyFile) {
			bad("give the full paths of the certificate and key files")
		}
	default:
		bad("unknown certificate choice %q", p.TLSMode)
	}
	if p.Container && (p.ContainerGB < 1 || p.ContainerGB > 16384) {
		bad("the encrypted container must be 1-16384 GB")
	}
	return errors.Join(errs...)
}

func isRoot(p string) bool {
	c := filepath.Clean(p)
	return filepath.Dir(c) == c
}

func validHost(h string) bool {
	if net.ParseIP(h) != nil {
		return true
	}
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
				return false
			}
		}
	}
	return true
}

// Result is what the person needs once the server is running.
type Result struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	ServerKey     string   `json:"server_key"`
	CertSHA256    string   `json:"tls_cert_sha256"`
	Addresses     []string `json:"addresses"`
	AtRest        string   `json:"at_rest"`
	Invite        string   `json:"invite,omitempty"`
	InviteExpires string   `json:"invite_expires,omitempty"`
	AdminLink     string   `json:"admin_link,omitempty"` // one-time link to create the first admin portal account
	DataDir       string   `json:"data_dir"`
	Logs          string   `json:"logs"`
	Notes         []string `json:"notes,omitempty"`
}

// Reporter shows progress. Each step is Step then Done or Fail; skipped steps only get Skip.
type Reporter interface {
	Step(id, title string)
	Done(id, detail string)
	Skip(id, title, reason string)
	Fail(id string, err error)
	Note(text string)
}

type step struct {
	id, title string
	skip      string // when set, the step is skipped with this reason
	run       func() (detail string, err error)
}

func runSteps(r Reporter, steps []step) error {
	for _, s := range steps {
		if s.skip != "" {
			r.Skip(s.id, s.title, s.skip)
			continue
		}
		r.Step(s.id, s.title)
		detail, err := s.run()
		if err != nil {
			r.Fail(s.id, err)
			return fmt.Errorf("%s: %w", s.title, err)
		}
		r.Done(s.id, detail)
	}
	return nil
}

// Install carries out a plan.
func Install(p *Plan, r Reporter) (*Result, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return nil, err
	}
	res := &Result{Version: Version, DataDir: p.DataDir}
	j := &job{plan: p, r: r, res: res, self: exe, started: time.Now()}
	if err := runSteps(r, j.steps()); err != nil {
		return res, err
	}
	return res, nil
}

// job is one run of the installer.
type job struct {
	plan    *Plan
	r       Reporter
	res     *Result
	self    string // this executable (copied into place)
	started time.Time
	oldDir  string // an upgrade moving the program out of an older folder (removed at the end)
}

func (j *job) note(s string) {
	j.res.Notes = append(j.res.Notes, s)
	j.r.Note(s)
}

// checkDataDir refuses folders that aren't new, empty or already ParallelPath's own, because the installer locks
// its permissions down (it must never do that to a folder that holds anything else).
func checkDataDir(dir string) (existing bool, err error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return false, nil
	}
	if _, err := os.Stat(config.Path(dir)); err == nil {
		return true, nil
	}
	for _, e := range entries {
		if e.Name() != "lost+found" { // an empty, freshly made filesystem
			return false, fmt.Errorf("%s already holds other files; choose a new or empty folder", dir)
		}
	}
	return false, nil
}

// writeConfig creates or updates config.json from the plan (keys and data are never touched).
func (j *job) writeConfig() (string, error) {
	p := j.plan
	c, err := config.Load(p.DataDir)
	verb := "updated"
	if err != nil {
		c, verb = config.Default(p.Name, nil), "created"
	}
	c.Name, c.Hosts, c.Listen = strings.TrimSpace(p.Name), p.Hosts, ":"+strconv.Itoa(p.Port)
	c.AllowUnencryptedVolume = p.Unencrypted
	c.BackupDir, c.BackupKeep = "", 0
	if p.Backups {
		c.BackupDir, c.BackupKeep = p.BackupDir, p.BackupKeep
	}
	c.AtRestVerified, _ = atrest.Attest(p.DataDir, time.Now()) // nil when not verified (or not needed, on Linux)
	if verb == "created" {                                     // the certificate choice is made once: changing it later changes what the apps have pinned
		c.TLSMode, c.CertFile, c.KeyFile, c.ACMEDomains, c.ACMEEmail, c.HTTPListen = "local-ca", "", "", nil, "", ""
		switch p.TLSMode {
		case "acme":
			c.TLSMode, c.ACMEDomains, c.ACMEEmail, c.HTTPListen = "acme", []string{p.ACMEDomain}, p.ACMEEmail, ":80"
			c.Hosts = appendHost(c.Hosts, p.ACMEDomain)
		case "files":
			c.TLSMode, c.CertFile, c.KeyFile = "files", p.CertFile, p.KeyFile
		}
	}
	c.TURNListen = ""
	if !p.Calls {
		c.TURNListen = "off"
	}
	if err := c.Save(p.DataDir); err != nil {
		return "", err
	}
	return verb + " " + config.Path(p.DataDir), nil
}

// refreshAttestation re-verifies the data volume's encryption as administrator on an upgrade or repair, and records
// it for the service (which may not be able to check for itself).
func (j *job) refreshAttestation() (string, error) {
	c, err := config.Load(j.plan.DataDir)
	if err != nil {
		return "", err
	}
	att, err := atrest.Attest(j.plan.DataDir, time.Now())
	if err != nil && !c.AllowUnencryptedVolume {
		return "", fmt.Errorf("%s. %s", firstLine(err.Error()), encryptionAdvice)
	}
	c.AtRestVerified = att
	if err := c.Save(j.plan.DataDir); err != nil {
		return "", err
	}
	if att == nil {
		return "not encrypted - allowed for testing", nil
	}
	return "encrypted (" + att.Volume + ")", nil
}

// neverStarted reports whether the server has never run (no database yet): a repair then also makes the first invite.
func (j *job) neverStarted() bool {
	_, err := os.Stat(filepath.Join(j.plan.DataDir, "parallelpath.db"))
	return err != nil
}

// waitForServer waits until the freshly started server has published server.json, or the service has stopped.
// alive returns the service's process ID, or an error once it has stopped.
func (j *job) waitForServer(alive func() (int, error), logs func() string) (string, error) {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		pid, err := alive()
		if err != nil {
			return "", fmt.Errorf("%w\n%s", err, logs())
		}
		if info, err := serverinfo.Read(j.plan.DataDir); err == nil && pid != 0 && info.PID == pid {
			j.res.Name, j.res.ServerKey, j.res.CertSHA256, j.res.AtRest = info.Name, info.ServerKey, info.CertSHA256, info.AtRest
			_, port, _ := net.SplitHostPort(info.Listen)
			j.res.Addresses = nil
			for _, h := range info.Hosts {
				j.res.Addresses = append(j.res.Addresses, net.JoinHostPort(h, port))
			}
			return "running, process " + strconv.Itoa(pid), nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("the server didn't start within 90 seconds\n%s", logs())
}

func (j *job) firstInvite() (string, error) {
	code, exp, err := admin.Invite(j.plan.DataDir)
	if err != nil {
		return "", err
	}
	j.res.Invite = code
	if !exp.IsZero() {
		j.res.InviteExpires = exp.UTC().Format(time.RFC3339)
	}
	return "valid for 7 days, works once", nil
}

func skipUnless(ok bool, reason string) string {
	if ok {
		return ""
	}
	return reason
}

// portFree reports whether something already uses the port. It only ever touches loopback (listening on every
// interface, even briefly, would make Windows Firewall ask about this program).
func portFree(port int) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
		c.Close()
		return fmt.Errorf("port %d is already in use", port)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use", port)
	}
	return ln.Close()
}

func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// removeBackups deletes the server's own backup files, then the folder if that leaves it empty.
func removeBackups(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "parallelpath-") && strings.HasSuffix(e.Name(), ".db") {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	os.RemoveAll(filepath.Join(dir, "blobs")) // the copies of the families' (encrypted) files
	os.Remove(dir)
}

// adminLink makes a one-time link for creating the first admin portal account (it works for 24 hours; more can be
// made any time with `ppserver admin setup-link`).
func (j *job) adminLink() (string, error) {
	link, err := admin.Link(j.plan.DataDir, "setup")
	if err != nil {
		return "", err
	}
	j.res.AdminLink = link
	return "works once, within 24 hours", nil
}

func appendHost(hosts []string, h string) []string {
	for _, x := range hosts {
		if strings.EqualFold(x, h) {
			return hosts
		}
	}
	return append(hosts, h)
}

// turnPorts are the call relay's ports (its listener, and the range it relays media on).
const (
	turnPort       = 3478
	turnRelayPorts = "49160-49200"
)

// caFile is the local certificate authority's certificate in the data folder (the server makes it on first start).
func caFile(dataDir string) string { return filepath.Join(dataDir, "tls-ca.crt") }
