//go:build windows

package install

import (
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"pp/server/internal/config"
	"pp/server/internal/serverinfo"
)

//go:embed assets/parallelpath.ico
var icon []byte

const adminHint = "the installer needs administrator rights: run it from an administrator terminal"

const encryptionAdvice = "Turn on BitLocker for that drive, or choose a data folder on a BitLocker drive."

const (
	serviceAccount = `NT SERVICE\` + ServiceName
	productKey     = `SOFTWARE\DarkCipher Systems\ParallelPath Server`
	uninstallKey   = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\` + ServiceName
	firewallRule   = "ParallelPath Server"
)

func isAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }

func enableVT() bool {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	if windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
		return false
	}
	_ = windows.SetConsoleOutputCP(65001)
	return true
}

func programData() string {
	if d := os.Getenv("ProgramData"); d != "" {
		return d
	}
	return `C:\ProgramData`
}

func programFiles() string {
	if d := os.Getenv("ProgramFiles"); d != "" {
		return d
	}
	return `C:\Program Files`
}

// Everything DarkCipher Systems installs lives in a "DarkCipher Systems" folder, in Program Files and ProgramData.
const vendor = "DarkCipher Systems"

func rootDir() string { return filepath.Join(programData(), vendor, "ParallelPath Server") }

// legacyInstallDir is where early test builds installed the program; upgrades move it.
func legacyInstallDir() string { return filepath.Join(programFiles(), "ParallelPath Server") }

func defaults() *Plan {
	return &Plan{InstallDir: filepath.Join(programFiles(), vendor, "ParallelPath Server"), DataDir: filepath.Join(rootDir(), "data"),
		Port: 8443, AutoStart: true, Firewall: true, LANOnly: true, Backups: true, BackupDir: filepath.Join(rootDir(), "backups"),
		BackupKeep: 14, Shortcuts: true, FirstInvite: true, Tray: true, TrustCA: true, Calls: true}
}

func features(p *Plan, encrypted bool) []feature {
	fs := []feature{
		{"Start automatically with Windows", "a Windows service running as its own low-privilege virtual account", &p.AutoStart},
		{"Allow through Windows Firewall", "an inbound rule for the server's port, for this program only", &p.Firewall},
		{"  … on Private and Domain networks only", "untick to also allow Public networks (a VPS, or port forwarding)", &p.LANOnly},
		{"Voice and video calls", "the call relay (TURN) and firewall rules for it: UDP/TCP 3478 and UDP 49160-49200", &p.Calls},
		{"Trust the certificate on this PC", "so browsers on this PC open the web app with no warning (other devices: /trust)", &p.TrustCA},
		{"Daily backups", "sealed copies of the database (kept for a number of days) and of the encrypted files", &p.Backups},
		{"Start-menu shortcuts", "Create an invite code · Server status · Uninstall", &p.Shortcuts},
	}
	if !encrypted {
		fs = append(fs, feature{"Allow a drive without BitLocker", "TESTING ONLY - never for real families", &p.Unencrypted})
	}
	return append(fs, feature{"Create the first invite code", "for the person who will create the family space", &p.FirstInvite})
}

func existingInstall() *Existing {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, productKey, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	e := &Existing{}
	e.Version, _, _ = k.GetStringValue("Version")
	e.InstallDir, _, _ = k.GetStringValue("InstallDir")
	e.DataDir, _, _ = k.GetStringValue("DataDir")
	if e.DataDir == "" {
		return nil
	}
	e.State = serviceState()
	return readExisting(e)
}

func serviceState() string {
	m, err := mgr.Connect()
	if err != nil {
		return "unknown (" + err.Error() + ")"
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return "not installed"
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return "unknown"
	}
	return map[svc.State]string{svc.Stopped: "stopped", svc.StartPending: "starting", svc.StopPending: "stopping",
		svc.Running: "running", svc.Paused: "paused"}[st.State]
}

func platformLogs(data string) string { return filepath.Join(data, "logs", "server.log") }

func platformChecks(d *Plan, dataDir string, port int, upgrading bool) []Check {
	v := windows.RtlGetVersion()
	osc := Check{ID: "os", Title: "Windows", State: "ok", Detail: fmt.Sprintf("Windows %d.%d build %d", v.MajorVersion, v.MinorVersion, v.BuildNumber)}
	if v.MajorVersion < 10 || v.BuildNumber < 17763 {
		osc.State, osc.Detail = "fail", osc.Detail+" - Windows 10 1809 / Server 2019 or newer is needed"
	}
	cs := []Check{osc}
	if isAdmin() {
		cs = append(cs, Check{ID: "admin", Title: "Administrator", State: "ok", Detail: "yes"})
	} else {
		cs = append(cs, Check{ID: "admin", Title: "Administrator", State: "fail", Detail: "setup needs administrator rights"})
	}
	cs = append(cs, Check{ID: "keys", Title: "Key protection", State: "ok",
		Detail: "Windows DPAPI, bound to the service's own account on this PC"})
	if upgrading {
		return cs
	}
	return append(cs, encryptionCheck(dataDir, true, encryptionAdvice), portCheck(port, false),
		Check{ID: "packages", Title: "Dependencies", State: "ok", Detail: "none - everything used is built into Windows"})
}

// command runs a program with an exact command line (netsh and icacls parse their own quoting).
func command(exe, cmdline string) (string, error) {
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(exe) + " " + cmdline, HideWindow: true}
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("%s: %v\n%s", filepath.Base(exe), err, s)
	}
	return s, nil
}

func system32(name string) string {
	dir, _ := windows.GetSystemDirectory()
	return filepath.Join(dir, name)
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, ``) + `"` }

func (j *job) exePath() string { return filepath.Join(j.plan.InstallDir, "ppserver.exe") }

func (j *job) steps() []step {
	p := j.plan
	if p.Mode == "upgrade" {
		return []step{
			{id: "checks", title: "Checking the installed server", run: j.upgradeChecks},
			{id: "encryption", title: "Verifying the data drive's encryption", run: j.refreshAttestation},
			{id: "stop", title: "Stopping the running server", run: stopService},
			{id: "files", title: "Installing the new program", run: j.copyFiles},
			{id: "manager", title: "ParallelPath Server Manager", run: j.installManager},
			{id: "service", title: "Refreshing the Windows service", run: j.configureService},
			{id: "firewall", title: "Windows Firewall rule", run: j.firewall},
			{id: "shortcuts", title: "Start-menu shortcuts", run: j.shortcuts},
			{id: "register", title: "Registering with Windows", run: j.register},
			{id: "start", title: "Starting the server", run: j.start},
			{id: "invite", title: "Creating the first invite code", skip: skipUnless(j.neverStarted(), "the server has run before"), run: j.firstInvite},
			{id: "tidy", title: "Tidying up", run: j.removeOldFolder},
		}
	}
	return []step{
		{id: "checks", title: "Checking this PC", run: j.checks},
		{id: "stop", title: "Stopping the running server", skip: skipUnless(serviceState() != "not installed", "not installed yet"),
			run: stopService},
		{id: "files", title: "Installing the program", run: j.copyFiles},
		{id: "manager", title: "ParallelPath Server Manager", skip: skipUnless(p.ManagerExe != "", "not part of this install"),
			run: j.installManager},
		{id: "service", title: "Creating the Windows service", run: j.configureService},
		{id: "register", title: "Registering with Windows", run: j.register},
		{id: "folders", title: "Locking down the data folder", run: j.prepareFolders},
		{id: "config", title: "Writing the settings", run: j.writeConfig},
		{id: "firewall", title: "Windows Firewall rule", run: j.firewall},
		{id: "shortcuts", title: "Start-menu shortcuts", run: j.shortcuts},
		{id: "start", title: "Starting the server", run: j.start},
		{id: "trust", title: "Trusting the server's certificate on this PC",
			skip: skipUnless(p.TrustCA && (p.TLSMode == "" || p.TLSMode == "local-ca"), "not selected"), run: j.trustCA},
		{id: "invite", title: "Creating the first invite code", skip: skipUnless(p.FirstInvite, "not selected"), run: j.firstInvite},
	}
}

func (j *job) checks() (string, error) {
	p := j.plan
	if !isAdmin() {
		return "", errors.New(adminHint)
	}
	if !filepath.IsAbs(p.InstallDir) || isRoot(p.InstallDir) {
		return "", errors.New("the program folder must be a full path, and not a drive root")
	}
	if err := portFree(p.Port); err != nil {
		return "", err
	}
	if _, err := checkDataDir(p.DataDir); err != nil {
		return "", err
	}
	if err := atRestOf(p.DataDir); err != nil {
		if !p.Unencrypted {
			return "", fmt.Errorf("%s. %s", firstLine(err.Error()), encryptionAdvice)
		}
		j.note("The data drive is NOT protected by BitLocker (allowed for testing). Don't use this server for real families.")
	}
	return "ready", nil
}

func (j *job) upgradeChecks() (string, error) {
	if !isAdmin() {
		return "", errors.New(adminHint)
	}
	e := existingInstall()
	if e == nil {
		return "", errors.New("no installed server found")
	}
	c, err := config.Load(e.DataDir)
	if err != nil {
		return "", err
	}
	p := j.plan
	p.DataDir, p.InstallDir = e.DataDir, e.InstallDir
	if p.InstallDir == "" {
		p.InstallDir = defaults().InstallDir
	}
	p.Name, p.Hosts = c.Name, c.Hosts
	if _, port, err := net.SplitHostPort(c.Listen); err == nil {
		p.Port, _ = strconv.Atoi(port)
	}
	j.loadChoices()
	moved := ""
	if strings.EqualFold(filepath.Clean(p.InstallDir), legacyInstallDir()) {
		j.oldDir, p.InstallDir = p.InstallDir, defaults().InstallDir
		moved = "; moving the program to " + p.InstallDir
	}
	p.AutoStart = true
	if m, err := mgr.Connect(); err == nil {
		if s, err := m.OpenService(ServiceName); err == nil {
			if cfg, err := s.Config(); err == nil {
				p.AutoStart = cfg.StartType == mgr.StartAutomatic
			}
			s.Close()
		}
		m.Disconnect()
	}
	return "version " + e.Version + " → " + Version + moved, nil
}

func stopService() (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return "", err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return "not installed", nil
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return "", err
	}
	if st.State == svc.Stopped {
		return "was not running", nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		return "", err
	}
	for range 60 {
		time.Sleep(500 * time.Millisecond)
		if st, err = s.Query(); err == nil && st.State == svc.Stopped {
			time.Sleep(500 * time.Millisecond) // let the process exit and release its files
			return "stopped", nil
		}
	}
	return "", errors.New("the service didn't stop within 30 seconds")
}

func (j *job) copyFiles() (string, error) {
	dst := j.exePath()
	if err := os.MkdirAll(j.plan.InstallDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(j.plan.InstallDir, "parallelpath.ico"), icon, 0o644); err != nil {
		return "", err
	}
	if strings.EqualFold(filepath.Clean(j.self), filepath.Clean(dst)) {
		return "already in place", nil
	}
	b, err := os.ReadFile(j.self)
	if err != nil {
		return "", err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return dst, nil
}

// requiredPrivileges limits the service's token to the one privilege every process needs (traverse checking);
// everything else a virtual account would normally hold is removed.
func requiredPrivileges(h windows.Handle) error {
	list, _ := windows.UTF16FromString("SeChangeNotifyPrivilege")
	list = append(list, 0) // a multi-string ends with an extra NUL
	info := struct{ RequiredPrivileges *uint16 }{&list[0]}
	return windows.ChangeServiceConfig2(h, windows.SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO, (*byte)(unsafe.Pointer(&info)))
}

func (j *job) configureService() (string, error) {
	p := j.plan
	m, err := mgr.Connect()
	if err != nil {
		return "", err
	}
	defer m.Disconnect()
	start := uint32(mgr.StartManual)
	if p.AutoStart {
		start = mgr.StartAutomatic
	}
	cfg := mgr.Config{
		ServiceType: windows.SERVICE_WIN32_OWN_PROCESS, StartType: start, ErrorControl: mgr.ErrorNormal,
		DisplayName: DisplayName, ServiceStartName: serviceAccount, DelayedAutoStart: p.AutoStart,
		SidType:     windows.SERVICE_SID_TYPE_UNRESTRICTED,
		Description: "ParallelPath post-quantum secure co-parenting server. Built by DarkCipher0x at DarkCipher Systems.",
	}
	verb := "updated"
	s, err := m.OpenService(ServiceName)
	if err == nil {
		cfg.BinaryPathName = syscall.EscapeArg(j.exePath()) + " run --data " + syscall.EscapeArg(p.DataDir)
		if err := s.UpdateConfig(cfg); err != nil {
			s.Close()
			return "", err
		}
	} else {
		verb = "created"
		if s, err = m.CreateService(ServiceName, j.exePath(), cfg, "run", "--data", p.DataDir); err != nil {
			return "", err
		}
	}
	defer s.Close()
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 2 * time.Minute},
	}, 86400); err != nil {
		return "", err
	}
	if err := requiredPrivileges(s.Handle); err != nil {
		return "", err
	}
	how := "starts with Windows"
	if !p.AutoStart {
		how = "started by hand"
	}
	return fmt.Sprintf("%s %s (%s), runs as %s", verb, ServiceName, how, serviceAccount), nil
}

// lockDir makes a folder (and everything in it) accessible only to SYSTEM, Administrators and the service.
func lockDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	icacls := system32("icacls.exe")
	if _, err := command(icacls, quote(dir)+" /setowner *S-1-5-32-544 /T /C /Q"); err != nil {
		return err
	}
	_, err := command(icacls, quote(dir)+` /inheritance:r /grant:r *S-1-5-18:(OI)(CI)F *S-1-5-32-544:(OI)(CI)F "`+serviceAccount+`:(OI)(CI)M" /T /C /Q`)
	return err
}

// secureVendorFolder makes sure ProgramData\DarkCipher Systems belongs to Administrators with ProgramData's normal
// permissions. Anyone can create folders in ProgramData; if someone else had made this one first, they would own it
// and could rename or replace the server's folder inside it.
func secureVendorFolder() error {
	dir := filepath.Dir(rootDir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	icacls := system32("icacls.exe")
	if _, err := command(icacls, quote(dir)+" /setowner *S-1-5-32-544 /C /Q"); err != nil {
		return err
	}
	_, err := command(icacls, quote(dir)+" /reset /C /Q")
	return err
}

func under(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	return err == nil && !strings.HasPrefix(rel, "..") && rel != "."
}

func (j *job) prepareFolders() (string, error) {
	p := j.plan
	locked := []string{p.DataDir}
	if under(p.DataDir, rootDir()) {
		locked[0] = rootDir() // the default layout: lock the whole ParallelPath Server folder
		if err := secureVendorFolder(); err != nil {
			return "", err
		}
	}
	if p.Backups && !under(p.BackupDir, locked[0]) {
		if _, err := checkBackupDir(p.BackupDir); err != nil {
			return "", err
		}
		locked = append(locked, p.BackupDir)
	}
	for _, d := range locked {
		if err := lockDir(d); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return "", err
	}
	if p.Backups {
		if err := os.MkdirAll(p.BackupDir, 0o700); err != nil {
			return "", err
		}
	}
	return strings.Join(locked, ", ") + ": SYSTEM, Administrators and the service only", nil
}

// checkBackupDir accepts a new or empty folder, or one holding only ParallelPath backups.
func checkBackupDir(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "parallelpath-") {
			return false, fmt.Errorf("%s already holds other files; choose a new or empty backup folder", dir)
		}
	}
	return len(entries) > 0, nil
}

func (j *job) firewall() (string, error) {
	netsh := system32("netsh.exe")
	command(netsh, `advfirewall firewall delete rule name="`+firewallRule+`"`) // fails harmlessly when there is none
	if !j.plan.Firewall {
		j.note("No firewall rule was added: other devices can't reach the server until you allow TCP port " +
			strconv.Itoa(j.plan.Port) + ".")
		return "no rule (not selected)", nil
	}
	profile, scope := "private,domain", "Private and Domain networks"
	if !j.plan.LANOnly {
		profile, scope = "any", "all networks"
	}
	add := func(proto, ports string) error {
		_, err := command(netsh, fmt.Sprintf(`advfirewall firewall add rule name="%s" dir=in action=allow protocol=%s localport=%s `+
			`program="%s" profile=%s enable=yes description="ParallelPath Server (DarkCipher Systems)"`,
			firewallRule, proto, ports, strings.ReplaceAll(j.exePath(), `"`, ``), profile))
		return err
	}
	tcp := strconv.Itoa(j.plan.Port)
	if j.plan.TLSMode == "acme" {
		tcp += ",80" // Let's Encrypt checks the name over HTTP, and plain HTTP is redirected to HTTPS
	}
	if j.plan.Calls {
		tcp += "," + strconv.Itoa(turnPort)
	}
	if err := add("TCP", tcp); err != nil {
		return "", err
	}
	if j.plan.Calls {
		if err := add("UDP", strconv.Itoa(turnPort)+","+turnRelayPorts); err != nil {
			return "", err
		}
		return fmt.Sprintf("TCP %s and UDP %d,%s (calls) allowed on %s", tcp, turnPort, turnRelayPorts, scope), nil
	}
	return fmt.Sprintf("TCP %s allowed on %s", tcp, scope), nil
}

// trustCA adds the server's local certificate authority to this PC's trusted roots, so browsers here open the web
// app with no warning. (Its name constraints mean it can only ever vouch for this server's own addresses.)
func (j *job) trustCA() (string, error) {
	f := caFile(j.plan.DataDir)
	if _, err := os.Stat(f); err != nil {
		return "", fmt.Errorf("the server hasn't created its certificate authority (%s)", f)
	}
	if out, err := command(system32("certutil.exe"), `-f -addstore Root "`+f+`"`); err != nil {
		return "", fmt.Errorf("certutil: %s", firstLine(out))
	}
	return "trusted by this PC (Local Machine root store)", nil
}

// untrustCA removes this server's CA from the PC's trusted roots (uninstall).
func untrustCA(dataDir string) {
	b, err := os.ReadFile(caFile(dataDir))
	if err != nil {
		return
	}
	if blk, _ := pem.Decode(b); blk != nil {
		if c, err := x509.ParseCertificate(blk.Bytes); err == nil {
			command(system32("certutil.exe"), `-delstore Root `+c.SerialNumber.Text(16))
		}
	}
}

func startMenuDir() string {
	return filepath.Join(programData(), `Microsoft\Windows\Start Menu\Programs`, DisplayName)
}

// shortcutScript creates the Start-menu shortcuts. Paths come in through environment variables, never inside the
// script text. The ppserver tools get the "Run as administrator" flag (bit 0x20 of byte 0x15 in the .lnk); the
// Manager runs as the person.
const shortcutScript = `$ErrorActionPreference = 'Stop'
$sh = New-Object -ComObject WScript.Shell
New-Item -ItemType Directory -Force -Path $env:PP_MENU | Out-Null
function L($name, $target, $arguments, $desc, $admin) {
  $path = Join-Path $env:PP_MENU ($name + '.lnk')
  $l = $sh.CreateShortcut($path)
  $l.TargetPath = $target; $l.Arguments = $arguments; $l.Description = $desc
  $l.IconLocation = $env:PP_ICON + ',0'; $l.WorkingDirectory = $env:PP_DIR
  $l.Save()
  if ($admin) { $b = [IO.File]::ReadAllBytes($path); $b[0x15] = $b[0x15] -bor 0x20; [IO.File]::WriteAllBytes($path, $b) }
}
if ($env:PP_MANAGER) { L 'ParallelPath Server' $env:PP_MANAGER '--tray' 'Open the ParallelPath server and see whether it is running' $false }
L 'Create an invite code' $env:PP_EXE 'invite --pause' 'A one-time code that lets someone create a ParallelPath account' $true
L 'Server status' $env:PP_EXE 'status --pause' 'Is the ParallelPath server running? Fingerprints, addresses and recent log lines' $true
L 'Uninstall ParallelPath Server' $env:PP_EXE 'uninstall --pause' 'Remove the ParallelPath server' $true
`

func (j *job) shortcuts() (string, error) {
	os.RemoveAll(startMenuDir())
	if !j.plan.Shortcuts {
		return "none (not selected)", nil
	}
	cmd := exec.Command(system32(`WindowsPowerShell\v1.0\powershell.exe`), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", shortcutScript)
	manager := filepath.Join(j.plan.InstallDir, managerName)
	if _, err := os.Stat(manager); err != nil {
		manager = ""
	}
	cmd.Env = append(os.Environ(), "PP_MENU="+startMenuDir(), "PP_EXE="+j.exePath(), "PP_DIR="+j.plan.InstallDir,
		"PP_ICON="+filepath.Join(j.plan.InstallDir, "parallelpath.ico"), "PP_MANAGER="+manager)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v\n%s", err, strings.TrimSpace(string(out)))
	}
	return "Start menu › " + DisplayName, nil
}

func (j *job) register() (string, error) {
	p := j.plan
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, productKey, registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	for name, v := range map[string]string{"Version": Version, "InstallDir": p.InstallDir, "DataDir": p.DataDir} {
		if err := k.SetStringValue(name, v); err != nil {
			k.Close()
			return "", err
		}
	}
	err = j.saveChoices(k)
	k.Close()
	if err != nil {
		return "", err
	}
	u, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	defer u.Close()
	exe := syscall.EscapeArg(j.exePath())
	size := uint32(0)
	if st, err := os.Stat(j.exePath()); err == nil {
		size = uint32(st.Size() >> 10)
	}
	for name, v := range map[string]string{
		"DisplayName": DisplayName, "DisplayVersion": Version, "Publisher": Publisher,
		"DisplayIcon": filepath.Join(p.InstallDir, "parallelpath.ico"), "InstallLocation": p.InstallDir,
		"UninstallString": exe + " uninstall --pause", "QuietUninstallString": exe + " uninstall --yes",
		"Comments": "Post-quantum secure co-parenting server",
	} {
		if err := u.SetStringValue(name, v); err != nil {
			return "", err
		}
	}
	for name, v := range map[string]uint32{"NoModify": 1, "NoRepair": 1, "EstimatedSize": size} {
		if err := u.SetDWordValue(name, v); err != nil {
			return "", err
		}
	}
	return "Apps & features › " + DisplayName + " " + Version, nil
}

func (j *job) start() (string, error) {
	data := j.plan.DataDir
	j.res.Logs = platformLogs(data)
	serverinfo.Remove(data)
	m, err := mgr.Connect()
	if err != nil {
		return "", err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return "", err
	}
	defer s.Close()
	if err := s.Start(); err != nil {
		return "", fmt.Errorf("%w\n%s", err, logTail(data, 15))
	}
	detail, err := j.waitForServer(func() (int, error) {
		st, err := s.Query()
		if err != nil {
			return 0, err
		}
		if st.State == svc.Stopped {
			return 0, fmt.Errorf("the service stopped while starting (exit code %d)", st.ServiceSpecificExitCode)
		}
		return int(st.ProcessId), nil
	}, func() string { return logTail(data, 15) })
	if err != nil {
		return "", err
	}
	return detail, j.publishInfo()
}

// logTail returns the last n lines of the service log.
func logTail(data string, n int) string {
	b, err := os.ReadFile(platformLogs(data))
	if err != nil {
		return "(no log yet: " + platformLogs(data) + ")"
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return "Last log lines (" + platformLogs(data) + "):\n" + strings.Join(lines, "\n")
}

func uninstall(e *Existing, purge bool, r Reporter) error {
	steps := []step{
		{id: "manager", title: "Closing ParallelPath Server Manager", run: func() (string, error) {
			stopManagers()
			removeManagerAutostart()
			return "closed", nil
		}},
		{id: "stop", title: "Stopping the server", run: stopService},
		{id: "service", title: "Removing the Windows service", run: func() (string, error) {
			m, err := mgr.Connect()
			if err != nil {
				return "", err
			}
			defer m.Disconnect()
			s, err := m.OpenService(ServiceName)
			if err != nil {
				return "not installed", nil
			}
			defer s.Close()
			return "removed", s.Delete()
		}},
		{id: "firewall", title: "Removing the firewall rules", run: func() (string, error) {
			command(system32("netsh.exe"), `advfirewall firewall delete rule name="`+firewallRule+`"`)
			return "removed", nil
		}},
		{id: "trust", title: "Removing the server's certificate from this PC", run: func() (string, error) {
			untrustCA(e.DataDir)
			return "removed (if it was trusted)", nil
		}},
		{id: "shortcuts", title: "Removing the Start-menu shortcuts", run: func() (string, error) {
			return "removed", os.RemoveAll(startMenuDir())
		}},
		{id: "register", title: "Unregistering from Windows", run: func() (string, error) {
			registry.DeleteKey(registry.LOCAL_MACHINE, uninstallKey)
			registry.DeleteKey(registry.LOCAL_MACHINE, productKey)
			registry.DeleteKey(registry.LOCAL_MACHINE, `SOFTWARE\DarkCipher Systems`) // only succeeds when empty
			return "removed", nil
		}},
	}
	if purge {
		steps = append(steps, step{id: "purge", title: "Deleting all data, backups and keys", run: func() (string, error) {
			c, _ := config.Load(e.DataDir)
			if _, err := os.Stat(config.Path(e.DataDir)); err == nil {
				if err := os.RemoveAll(e.DataDir); err != nil {
					return "", err
				}
			}
			if c != nil && c.BackupDir != "" {
				removeBackups(c.BackupDir)
			}
			// The folders above the data, when they're left empty (os.Remove never deletes a folder with anything in it).
			for d := filepath.Dir(e.DataDir); filepath.Dir(d) != d && !strings.EqualFold(d, programData()); d = filepath.Dir(d) {
				if os.Remove(d) != nil {
					break
				}
			}
			return "deleted", nil
		}})
	}
	steps = append(steps, step{id: "files", title: "Removing the program", run: func() (string, error) {
		dir := e.InstallDir
		if dir == "" {
			return "nothing recorded", nil
		}
		self, _ := os.Executable()
		if under(self, dir) {
			if err := removeAfterExit(dir); err != nil {
				return "", err
			}
			return dir + " (removed when this window closes)", nil
		}
		err := os.RemoveAll(dir)
		os.Remove(filepath.Dir(dir)) // Program Files\DarkCipher Systems, when nothing else is in it
		return dir, err
	}})
	return runSteps(r, steps)
}

// relaunchElevated starts this program again with the same arguments through UAC. It reports whether it did.
func relaunchElevated() bool {
	if isAdmin() {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	args := make([]string, 0, len(os.Args))
	for _, a := range os.Args[1:] {
		args = append(args, syscall.EscapeArg(a))
	}
	if !strings.Contains(strings.Join(args, " "), "--pause") {
		args = append(args, "--pause")
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(strings.Join(args, " "))
	return windows.ShellExecute(0, verb, file, params, nil, windows.SW_NORMAL) == nil
}

func recentLog(data string) string { return logTail(data, 15) }

// removeAfterExit deletes a folder once this program has exited (Windows can't delete a running program). A hidden
// helper retries for a while, because antivirus scanners often hold just-closed files open for a moment. It leaves
// any job object this program runs in when allowed, or it would be killed as soon as this program exits.
func removeAfterExit(dir string) error {
	start := func(flags uint32) error {
		cmd := exec.Command(system32("cmd.exe"))
		cmd.Dir = filepath.Dir(system32("cmd.exe"))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags,
			// Each round also tries the folder above (Program Files\DarkCipher Systems), which plain rd only removes when empty.
			CmdLine: `cmd.exe /d /c for /l %i in (1,1,20) do @(if exist ` + quote(dir) + ` (ping -n 2 127.0.0.1 >nul & rd /s /q ` +
				quote(dir) + ` 2>nul)) & rd ` + quote(filepath.Dir(dir)) + ` 2>nul`}
		return cmd.Start()
	}
	flags := uint32(windows.CREATE_NO_WINDOW | windows.DETACHED_PROCESS)
	if start(flags|windows.CREATE_BREAKAWAY_FROM_JOB) == nil {
		return nil
	}
	return start(flags)
}
