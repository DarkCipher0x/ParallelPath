//go:build linux

package install

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"pp/core/pqc"
	"pp/server/internal/atrest"
	"pp/server/internal/config"
	"pp/server/internal/serverinfo"
)

const adminHint = "the installer needs root: run it with sudo (sudo ./ppserver install)"

const encryptionAdvice = "Tick the encrypted container (recommended on a VPS), or put the data on a LUKS-encrypted disk."

func isAdmin() bool  { return os.Geteuid() == 0 }
func enableVT() bool { return true }

func defaults() *Plan {
	return &Plan{DataDir: "/var/lib/parallelpath", Port: 8443, AutoStart: true, Firewall: true, Backups: true,
		BackupDir: "/var/backups/parallelpath", BackupKeep: 14, ContainerGB: 20, Packages: true, FirstInvite: true, Calls: true}
}

func features(p *Plan, encrypted bool) []feature {
	fs := []feature{
		{"Start at boot", "systemd service, sandboxed, running as its own 'parallelpath' account", &p.AutoStart},
		{"Allow the port in ufw", "adds a ufw rule for the server's port (other firewalls: open it yourself)", &p.Firewall},
		{"Daily backups", "sealed copies of the database (kept for a number of days) and of the encrypted files", &p.Backups},
		{"Voice and video calls", "the call relay (TURN); ufw: UDP/TCP 3478 and UDP 49160-49200 (open them in a cloud firewall too)", &p.Calls},
	}
	if !encrypted {
		fs = append(fs,
			feature{"Encrypted data container", "a LUKS2 file for the data, unlocked at boot with a TPM/host-sealed key", &p.Container},
			feature{"Allow an unencrypted data volume", "TESTING ONLY - never for real families", &p.Unencrypted})
	}
	return append(fs,
		feature{"Install missing packages", "apt-get install what's needed (cryptsetup for the container)", &p.Packages},
		feature{"Create the first invite code", "for the person who will create the family space", &p.FirstInvite})
}

// marker records the installation in /etc/parallelpath/install.json.
type marker struct {
	Version       string `json:"version"`
	DataDir       string `json:"data_dir"`
	Container     bool   `json:"encrypted_container"`
	ContainerFile string `json:"container_file,omitempty"`
	BackupDir     string `json:"backup_dir,omitempty"` // for uninstall --purge (the data volume may be closed by then)
}

func readMarker() *marker {
	b, err := os.ReadFile(linuxMarker)
	if err != nil {
		return nil
	}
	var m marker
	if json.Unmarshal(b, &m) != nil || m.DataDir == "" {
		return nil
	}
	return &m
}

func existingInstall() *Existing {
	m := readMarker()
	if m == nil {
		return nil
	}
	return readExisting(&Existing{Version: m.Version, DataDir: m.DataDir, State: unitState()})
}

func unitState() string {
	out, _ := exec.Command("systemctl", "is-active", linuxUnit).Output()
	if s := strings.TrimSpace(string(out)); s == "active" {
		return "running"
	} else if s != "" {
		return s
	}
	return "unknown"
}

func platformLogs(string) string { return "journalctl -u parallelpath" }

func platformChecks(d *Plan, dataDir string, port int, upgrading bool) []Check {
	cs := []Check{{ID: "os", Title: "Operating system", State: "info", Detail: osRelease()}}
	if isAdmin() {
		cs = append(cs, Check{ID: "admin", Title: "Running as root", State: "ok", Detail: "yes"})
	} else {
		cs = append(cs, Check{ID: "admin", Title: "Running as root", State: "fail", Detail: "run with sudo"})
	}
	cs = append(cs, systemdCheck(), tpmCheck())
	if upgrading {
		return cs
	}
	cs = append(cs, encryptionCheck(dataDir, true, "The installer can create an encrypted container for it."), portCheck(port, false))
	c := Check{ID: "packages", Title: "Dependencies", State: "ok", Detail: "nothing to install"}
	if _, err := exec.LookPath("cryptsetup"); err != nil {
		c.State, c.Detail = "info", "cryptsetup is needed only for the encrypted container; the installer adds it"
	}
	return append(cs, c)
}

func osRelease() string {
	b, _ := os.ReadFile("/etc/os-release")
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return "Linux"
}

func systemdVersion() int {
	out, err := exec.Command("systemctl", "--version").Output()
	if err != nil {
		return 0
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}

func systemdCheck() Check {
	c := Check{ID: "systemd", Title: "systemd encrypted credentials"}
	v := systemdVersion()
	_, credErr := exec.LookPath("systemd-creds")
	switch {
	case v >= 250 && credErr == nil:
		c.State, c.Detail = "ok", "systemd "+strconv.Itoa(v)
	case v == 0:
		c.State, c.Detail = "fail", "systemd not found - ParallelPath needs a systemd-based system"
	default:
		c.State, c.Detail = "fail", "systemd "+strconv.Itoa(v)+" is too old: 250 or newer is needed (Ubuntu 24.04 LTS or newer)"
	}
	return c
}

func hasTPM() bool { return exec.Command("systemd-creds", "has-tpm2", "--quiet").Run() == nil }

func tpmCheck() Check {
	if hasTPM() {
		return Check{ID: "tpm", Title: "TPM 2.0", State: "ok", Detail: "keys are sealed to this machine's TPM and host key"}
	}
	return Check{ID: "tpm", Title: "TPM 2.0", State: "info",
		Detail: "none - keys are sealed with the host key (root-only, /var/lib/systemd/credential.secret)"}
}

// run runs a command, returning its trimmed output; on failure the error includes what it printed.
func run(stdin []byte, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s != "" {
			return s, fmt.Errorf("%s: %v\n%s", name, err, s)
		}
		return s, fmt.Errorf("%s: %v", name, err)
	}
	return s, nil
}

func lookPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

func (j *job) steps() []step {
	p := j.plan
	if p.Mode == "upgrade" {
		return j.upgradeSteps()
	}
	_, cryptErr := exec.LookPath("cryptsetup")
	return []step{
		{id: "checks", title: "Checking this machine", run: j.checks},
		{id: "packages", title: "Installing dependencies (cryptsetup)", skip: skipUnless(p.Container && cryptErr != nil, "nothing missing"),
			run: j.installPackages},
		{id: "stop", title: "Stopping the running server", skip: skipUnless(unitExists(), "not installed yet"), run: stopUnits},
		{id: "user", title: "Creating the parallelpath service account", run: ensureUser},
		{id: "files", title: "Installing the program", run: j.copyBinary},
		{id: "register", title: "Recording the installation", run: j.writeMarker},
		{id: "container", title: "Creating the encrypted data container", skip: skipUnless(p.Container, "not selected"), run: j.createContainer},
		{id: "folders", title: "Preparing the data folder", run: j.prepareFolders},
		{id: "encryption", title: "Checking encryption at rest", run: j.checkEncryption},
		{id: "key", title: "Sealing the master key", run: j.sealMasterKey},
		{id: "config", title: "Writing the settings", run: j.writeConfigLinux},
		{id: "service", title: "Setting up the systemd service", run: j.writeUnits},
		{id: "firewall", title: "Allowing the port in ufw", skip: skipUnless(p.Firewall, "not selected"), run: j.ufw},
		{id: "start", title: "Starting the server", run: j.start},
		{id: "invite", title: "Creating the first invite code", skip: skipUnless(p.FirstInvite, "not selected"), run: j.firstInvite},
		{id: "admin", title: "Admin portal link", run: j.adminLink},
	}
}

func (j *job) upgradeSteps() []step {
	return []step{
		{id: "checks", title: "Checking the installed server", run: j.upgradeChecks},
		{id: "stop", title: "Stopping the running server", run: stopUnits},
		{id: "files", title: "Installing the new program", run: j.copyBinary},
		{id: "service", title: "Refreshing the systemd service", run: j.writeUnits},
		{id: "start", title: "Starting the server", run: j.start},
		{id: "register", title: "Recording the installation", run: j.writeMarker},
	}
}

func (j *job) checks() (string, error) {
	p := j.plan
	if !isAdmin() {
		return "", errors.New(adminHint)
	}
	if c := systemdCheck(); c.State != "ok" {
		return "", errors.New(c.Detail)
	}
	if err := portFree(p.Port); err != nil {
		return "", err
	}
	existing, err := checkDataDir(p.DataDir)
	if err != nil {
		return "", err
	}
	if p.Container {
		if existing {
			return "", errors.New("the data folder already holds a server; the encrypted container is only for new installs")
		}
		file := p.DataDir + ".luks"
		if _, err := os.Stat(file); err == nil {
			return "", fmt.Errorf("%s already exists", file)
		}
		if _, err := os.Stat("/dev/mapper/" + linuxMapper); err == nil {
			return "", errors.New("an encrypted volume named " + linuxMapper + " is already open; close it (cryptsetup close " + linuxMapper + ") or reboot")
		}
		if _, err := exec.LookPath("cryptsetup"); err != nil && !p.Packages {
			return "", errors.New("cryptsetup isn't installed: tick 'Install missing packages' or run apt-get install cryptsetup-bin")
		}
		if _, err := exec.LookPath("mkfs.ext4"); err != nil {
			return "", errors.New("mkfs.ext4 isn't installed (apt-get install e2fsprogs)")
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(filepath.Dir(file), &st); err == nil {
			if free := int64(st.Bavail) * st.Bsize >> 30; free < int64(p.ContainerGB)+1 {
				return "", fmt.Errorf("not enough space for a %d GB container (%d GB free)", p.ContainerGB, free)
			}
		}
	} else if !p.Unencrypted {
		if err := atRestOf(p.DataDir); err != nil {
			return "", fmt.Errorf("%s. %s", firstLine(err.Error()), encryptionAdvice)
		}
	}
	return "ready", nil
}

func (j *job) upgradeChecks() (string, error) {
	if !isAdmin() {
		return "", errors.New(adminHint)
	}
	m := readMarker()
	if m == nil {
		return "", errors.New("no installed server found")
	}
	c, err := config.Load(m.DataDir)
	if err != nil {
		return "", err
	}
	// Refresh the unit with the installed server's own settings.
	p := j.plan
	p.DataDir, p.Container = m.DataDir, m.Container
	p.Backups, p.BackupDir = c.BackupDir != "", c.BackupDir
	_, port, _ := strings.Cut(c.Listen, ":")
	p.Port, _ = strconv.Atoi(port)
	p.AutoStart = exec.Command("systemctl", "is-enabled", "--quiet", linuxUnit).Run() == nil
	return "version " + m.Version + " → " + Version, nil
}

func (j *job) installPackages() (string, error) {
	if !j.plan.Packages {
		return "", errors.New("cryptsetup is missing and installing packages wasn't allowed")
	}
	if _, err := run(nil, "apt-get", "update", "-qq"); err != nil {
		return "", err
	}
	if _, err := run(nil, "apt-get", "install", "-y", "-qq", "--no-install-recommends", "cryptsetup-bin"); err != nil {
		return "", err
	}
	return "cryptsetup-bin installed", nil
}

func unitExists() bool {
	_, err := os.Stat(filepath.Join(linuxUnitDir, linuxUnit))
	return err == nil
}

func stopUnits() (string, error) {
	if _, err := run(nil, "systemctl", "stop", linuxUnit); err != nil {
		return "", err
	}
	return "stopped", nil
}

func ensureUser() (string, error) {
	if _, err := user.Lookup(linuxUser); err == nil {
		return "already exists", nil
	}
	nologin := "/usr/sbin/nologin"
	if _, err := os.Stat(nologin); err != nil {
		nologin = "/bin/false"
	}
	if _, err := run(nil, "useradd", "--system", "--user-group", "--home-dir", "/nonexistent", "--no-create-home",
		"--shell", nologin, "--comment", "ParallelPath server", linuxUser); err != nil {
		return "", err
	}
	return "created (no login, no home)", nil
}

func (j *job) copyBinary() (string, error) {
	if j.self == linuxBin {
		return "already in place", nil
	}
	b, err := os.ReadFile(j.self)
	if err != nil {
		return "", err
	}
	tmp := linuxBin + ".new"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, linuxBin); err != nil {
		return "", err
	}
	return linuxBin, nil
}

func (j *job) createContainer() (detail string, err error) {
	p := j.plan
	file := p.DataDir + ".luks"
	cryptsetup := lookPath("cryptsetup")
	key := make([]byte, 64)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	defer pqc.Wipe(key)
	opened := false
	defer func() {
		if err != nil { // leave nothing half-made behind
			if opened {
				run(nil, "umount", p.DataDir)
				run(nil, cryptsetup, "close", linuxMapper)
			}
			os.Remove(file)
			os.Remove(linuxVolumeCred)
		}
	}()
	if err := os.MkdirAll(linuxCredDir, 0o700); err != nil {
		return "", err
	}
	if err := sealCredential("volume_key", linuxVolumeCred, key); err != nil {
		return "", err
	}
	if _, err := run(nil, "fallocate", "-l", strconv.Itoa(p.ContainerGB)+"G", file); err != nil {
		return "", err
	}
	if err := os.Chmod(file, 0o600); err != nil {
		return "", err
	}
	if _, err := run(key, cryptsetup, "luksFormat", "--type", "luks2", "--batch-mode", "--pbkdf", "argon2id",
		"--pbkdf-memory", "65536", "--label", "parallelpath", "--key-file", "-", file); err != nil {
		return "", err
	}
	if _, err := run(key, cryptsetup, "open", "--type", "luks2", "--key-file", "-", file, linuxMapper); err != nil {
		return "", err
	}
	opened = true
	if _, err := run(nil, "mkfs.ext4", "-q", "-L", "parallelpath", "-m", "0", "/dev/mapper/"+linuxMapper); err != nil {
		return "", err
	}
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return "", err
	}
	if _, err := run(nil, "mount", "-o", "nodev,nosuid,noexec", "/dev/mapper/"+linuxMapper, p.DataDir); err != nil {
		return "", err
	}
	how := "host key"
	if hasTPM() {
		how = "TPM2 + host key"
	}
	return fmt.Sprintf("%d GB LUKS2 (AES-256-XTS) at %s, key sealed with the %s", p.ContainerGB, file, how), nil
}

func chownToService(path string) error {
	u, err := user.Lookup(linuxUser)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	if err := os.Chown(path, uid, gid); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func (j *job) prepareFolders() (string, error) {
	p := j.plan
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return "", err
	}
	if err := chownToService(p.DataDir); err != nil {
		return "", err
	}
	os.Remove(filepath.Join(p.DataDir, "lost+found")) // only succeeds while empty
	if p.Backups {
		if err := os.MkdirAll(p.BackupDir, 0o700); err != nil {
			return "", err
		}
		if err := chownToService(p.BackupDir); err != nil {
			return "", err
		}
	}
	return p.DataDir + " (owner parallelpath, mode 700)", nil
}

func (j *job) checkEncryption() (string, error) {
	if err := atrest.Check(j.plan.DataDir); err != nil {
		if j.plan.Unencrypted {
			j.note("The data folder is NOT on an encrypted volume (allowed for testing). Don't use this server for real families.")
			return "not encrypted - allowed for testing", nil
		}
		return "", err
	}
	return "LUKS/dm-crypt", nil
}

func (j *job) sealMasterKey() (string, error) {
	if _, err := os.Stat(linuxMasterCred); err == nil {
		return "kept the existing key", nil
	}
	if _, err := os.Stat(filepath.Join(j.plan.DataDir, "parallelpath.db")); err == nil {
		return "", fmt.Errorf("this data folder has a database but its master key (%s) is missing; restore it first", linuxMasterCred)
	}
	if err := os.MkdirAll(linuxCredDir, 0o700); err != nil {
		return "", err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	defer pqc.Wipe(key)
	if err := sealCredential("master_key", linuxMasterCred, key); err != nil {
		return "", err
	}
	if hasTPM() {
		return "systemd credential sealed with the TPM2 and host key", nil
	}
	return "systemd credential sealed with the host key (no TPM)", nil
}

func (j *job) writeConfigLinux() (string, error) {
	d, err := j.writeConfig()
	if err != nil {
		return "", err
	}
	path := config.Path(j.plan.DataDir)
	if err := chownToService(path); err != nil {
		return "", err
	}
	return d, os.Chmod(path, 0o600)
}

func (j *job) writeUnits() (string, error) {
	p := j.plan
	d := unitData{Bin: linuxBin, DataDir: p.DataDir, Container: p.Container, ContainerFile: p.DataDir + ".luks",
		Cryptsetup: lookPath("cryptsetup"), Mount: lookPath("mount"), Umount: lookPath("umount"), LowPort: p.Port < 1024}
	if p.Backups {
		d.BackupDir = p.BackupDir
	}
	if err := os.WriteFile(filepath.Join(linuxUnitDir, linuxUnit), []byte(render(mainUnit, d)), 0o644); err != nil {
		return "", err
	}
	units := []string{linuxUnit}
	if p.Container {
		if err := os.WriteFile(filepath.Join(linuxUnitDir, linuxVolumeUnit), []byte(render(volumeUnit, d)), 0o644); err != nil {
			return "", err
		}
		units = append(units, linuxVolumeUnit)
	}
	if _, err := run(nil, "systemctl", "daemon-reload"); err != nil {
		return "", err
	}
	verb := "enable"
	if !p.AutoStart {
		verb = "disable"
	}
	if _, err := run(nil, "systemctl", append([]string{verb, "--quiet"}, units...)...); err != nil {
		return "", err
	}
	if p.AutoStart {
		return linuxUnit + ", starts at boot", nil
	}
	return linuxUnit + ", started by hand (systemctl start parallelpath)", nil
}

func (j *job) ufw() (string, error) {
	port := strconv.Itoa(j.plan.Port) + "/tcp"
	if _, err := exec.LookPath("ufw"); err != nil {
		j.note("No ufw found. If you use another firewall (nftables, your provider's cloud firewall), allow TCP port " + strconv.Itoa(j.plan.Port) + ".")
		return "ufw not installed - nothing changed", nil
	}
	rules := []string{port}
	if j.plan.TLSMode == "acme" {
		rules = append(rules, "80/tcp") // Let's Encrypt's HTTP check, and redirects to HTTPS
	}
	if j.plan.Calls {
		t := strconv.Itoa(turnPort)
		rules = append(rules, t+"/tcp", t+"/udp", strings.Replace(turnRelayPorts, "-", ":", 1)+"/udp")
	}
	for _, r := range rules {
		if _, err := run(nil, "ufw", "allow", r, "comment", "ParallelPath server"); err != nil {
			return "", err
		}
	}
	if out, _ := run(nil, "ufw", "status"); !strings.Contains(out, "Status: active") {
		j.note("ufw is installed but not enabled, so the rules do nothing yet. If you enable it, allow SSH first.")
	}
	if j.plan.Calls {
		j.note("Calls: if your server is behind a cloud firewall or router, also open TCP/UDP " + strconv.Itoa(turnPort) +
			" and UDP " + turnRelayPorts + " there.")
	}
	return "allowed " + strings.Join(rules, ", "), nil
}

func (j *job) start() (string, error) {
	serverinfo.Remove(j.plan.DataDir)
	j.res.Logs = platformLogs("")
	if j.plan.Container && exec.Command("systemctl", "is-active", "--quiet", linuxVolumeUnit).Run() != nil {
		// The installer opened the new container itself to prepare it. Hand it over to the volume unit, which opens
		// it at every boot - so this start also proves the boot path works.
		if _, err := run(nil, "umount", j.plan.DataDir); err != nil {
			return "", err
		}
		if _, err := run(nil, lookPath("cryptsetup"), "close", linuxMapper); err != nil {
			return "", err
		}
	}
	if _, err := run(nil, "systemctl", "restart", linuxUnit); err != nil {
		return "", fmt.Errorf("%w\n%s", err, journal())
	}
	return j.waitForServer(func() (int, error) {
		out, _ := run(nil, "systemctl", "show", "--property=MainPID,ActiveState", linuxUnit)
		pid, state := 0, ""
		for _, l := range strings.Split(out, "\n") {
			if v, ok := strings.CutPrefix(l, "MainPID="); ok {
				pid, _ = strconv.Atoi(v)
			} else if v, ok := strings.CutPrefix(l, "ActiveState="); ok {
				state = v
			}
		}
		if state == "failed" || state == "inactive" {
			return 0, errors.New("the server stopped while starting")
		}
		return pid, nil
	}, journal)
}

func journal() string {
	out, _ := run(nil, "journalctl", "-u", linuxUnit, "-n", "15", "--no-pager", "-o", "cat")
	return "Last log lines (journalctl -u parallelpath):\n" + out
}

func (j *job) writeMarker() (string, error) {
	m := &marker{Version: Version, DataDir: j.plan.DataDir, Container: j.plan.Container}
	if j.plan.Backups {
		m.BackupDir = j.plan.BackupDir
	}
	if m.Container {
		m.ContainerFile = j.plan.DataDir + ".luks"
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.MkdirAll(filepath.Dir(linuxMarker), 0o755); err != nil {
		return "", err
	}
	return linuxMarker, os.WriteFile(linuxMarker, append(b, '\n'), 0o644)
}

func uninstall(e *Existing, purge bool, r Reporter) error {
	m := readMarker()
	// Where the backups are: read now, while the (possibly encrypted) data volume is still open.
	backupDir := ""
	if c, err := config.Load(e.DataDir); err == nil {
		backupDir = c.BackupDir
	}
	if backupDir == "" && m != nil {
		backupDir = m.BackupDir
	}
	steps := []step{
		{id: "stop", title: "Stopping and disabling the service", run: func() (string, error) {
			run(nil, "systemctl", "disable", "--now", "--quiet", linuxUnit, linuxVolumeUnit)
			return "stopped", nil
		}},
		{id: "firewall", title: "Removing the ufw rule", skip: skipUnless(e.Port != "", "no port recorded"), run: func() (string, error) {
			if _, err := exec.LookPath("ufw"); err != nil {
				return "ufw not installed", nil
			}
			t := strconv.Itoa(turnPort)
			for _, r := range []string{e.Port + "/tcp", "80/tcp", t + "/tcp", t + "/udp", strings.Replace(turnRelayPorts, "-", ":", 1) + "/udp"} {
				run(nil, "ufw", "delete", "allow", r)
			}
			return "removed", nil
		}},
		{id: "service", title: "Removing the systemd units", run: func() (string, error) {
			os.Remove(filepath.Join(linuxUnitDir, linuxUnit))
			os.Remove(filepath.Join(linuxUnitDir, linuxVolumeUnit))
			_, err := run(nil, "systemctl", "daemon-reload")
			return "removed", err
		}},
		{id: "files", title: "Removing the program", run: func() (string, error) {
			return linuxBin, ignoreMissing(os.Remove(linuxBin))
		}},
	}
	if purge {
		steps = append(steps, step{id: "purge", title: "Deleting all data, backups and keys", run: func() (string, error) {
			if m != nil && m.Container {
				run(nil, "umount", e.DataDir)
				run(nil, lookPath("cryptsetup"), "close", linuxMapper)
				os.Remove(m.ContainerFile)
			} else if _, err := os.Stat(config.Path(e.DataDir)); err == nil {
				if err := os.RemoveAll(e.DataDir); err != nil {
					return "", err
				}
			}
			os.Remove(e.DataDir)
			if backupDir != "" {
				removeBackups(backupDir)
			}
			os.Remove(linuxMasterCred)
			os.Remove(linuxVolumeCred)
			run(nil, "userdel", linuxUser)
			return "deleted", nil
		}})
	}
	steps = append(steps, step{id: "register", title: "Removing the installation record", run: func() (string, error) {
		os.Remove(linuxMarker)
		os.Remove(filepath.Dir(linuxMarker))
		return "done", nil
	}})
	return runSteps(r, steps)
}

// relaunchElevated is Windows-only: on Linux the person runs the installer with sudo.
func relaunchElevated() bool { return false }

func recentLog(string) string { return journal() }

// sealCredential encrypts a secret into a systemd credential file (TPM2 + host key when there is a TPM).
func sealCredential(name, path string, secret []byte) error {
	if _, err := run(secret, "systemd-creds", "encrypt", "--name="+name, "-", path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// serviceControl starts, stops or restarts the installed service (ppserver service ...).
func serviceControl(action string) error {
	if action != "start" && action != "stop" && action != "restart" {
		return errors.New("use start, stop or restart")
	}
	if !isAdmin() {
		return errors.New(adminHint)
	}
	_, err := run(nil, "systemctl", action, linuxUnit)
	return err
}
