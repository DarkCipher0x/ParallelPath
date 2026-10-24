//go:build windows

package install

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// ParallelPath Server Manager is the Windows setup program installed a second time as the server's companion: the
// notification-area icon that shows whether the server is running and opens it. It runs in each person's own
// session, without administrator rights (the service itself can't show anything on a desktop).
const (
	managerName = "ParallelPath Server Manager.exe"
	runKey      = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	runValue    = "ParallelPath Server"
)

// installManager copies the Manager into the program folder and starts it at everyone's sign-in (when chosen).
func (j *job) installManager() (string, error) {
	p := j.plan
	if p.ManagerExe == "" {
		return "not part of this install", nil
	}
	stopManagers()
	dst := filepath.Join(p.InstallDir, managerName)
	if !strings.EqualFold(filepath.Clean(p.ManagerExe), filepath.Clean(dst)) {
		b, err := os.ReadFile(p.ManagerExe)
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
	}
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, runKey, registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	if !p.Tray {
		k.DeleteValue(runValue)
		return dst + " (icon not started at sign-in)", nil
	}
	if err := k.SetStringValue(runValue, quote(dst)+" --tray"); err != nil {
		return "", err
	}
	return dst + ", icon starts at sign-in", nil
}

// stopManagers closes every running Manager (in any session), so its program file can be replaced or removed.
func stopManagers() {
	command(system32("taskkill.exe"), `/F /IM "`+managerName+`"`) // fails harmlessly when none is running
	time.Sleep(500 * time.Millisecond)
}

func removeManagerAutostart() {
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, runKey, registry.SET_VALUE); err == nil {
		k.DeleteValue(runValue)
		k.Close()
	}
}

// saveChoices remembers what was ticked, so an upgrade sets things up the same way.
func (j *job) saveChoices(k registry.Key) error {
	p := j.plan
	fw := "none"
	if p.Firewall {
		fw = map[bool]string{true: "lan", false: "any"}[p.LANOnly]
	}
	if err := k.SetStringValue("Firewall", fw); err != nil {
		return err
	}
	for name, on := range map[string]bool{"Shortcuts": p.Shortcuts, "Tray": p.Tray} {
		v := uint32(0)
		if on {
			v = 1
		}
		if err := k.SetDWordValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

// loadChoices restores the choices an earlier install saved. Installs from before they were saved get what can be
// seen on the system (a firewall rule, the Start-menu folder) and the icon.
func (j *job) loadChoices() {
	p := j.plan
	p.Tray = true
	p.Shortcuts = dirExists(startMenuDir())
	out, err := command(system32("netsh.exe"), `advfirewall firewall show rule name="`+firewallRule+`"`)
	p.Firewall = err == nil
	p.LANOnly = !strings.Contains(out, "Public")
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, productKey, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if fw, _, err := k.GetStringValue("Firewall"); err == nil {
		p.Firewall, p.LANOnly = fw != "none", fw != "any"
	}
	if v, _, err := k.GetIntegerValue("Shortcuts"); err == nil {
		p.Shortcuts = v == 1
	}
	if v, _, err := k.GetIntegerValue("Tray"); err == nil {
		p.Tray = v == 1
	}
}

func dirExists(d string) bool {
	st, err := os.Stat(d)
	return err == nil && st.IsDir()
}

// publishInfo records the running server's public details where the Manager (running without administrator
// rights, so it can't open the data folder) can read them: the port, the name and the certificate fingerprint it
// pins when it shows the server's page. None of it is secret.
func (j *job) publishInfo() error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, productKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	r := j.res
	port := uint64(j.plan.Port)
	if len(r.Addresses) > 0 {
		if _, ps, err := net.SplitHostPort(r.Addresses[0]); err == nil {
			port, _ = strconv.ParseUint(ps, 10, 16)
		}
	}
	for name, v := range map[string]string{"Name": r.Name, "CertSHA256": r.CertSHA256, "ServerKey": r.ServerKey} {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	if err := k.SetDWordValue("Port", uint32(port)); err != nil {
		return err
	}
	return k.SetStringsValue("Addresses", r.Addresses)
}

// removeOldFolder: in early test builds the program lived in Program Files\ParallelPath Server; it now lives in
// Program Files\DarkCipher Systems\ParallelPath Server. The service, firewall rule and shortcuts are pointed at the
// new folder by the upgrade; this removes the old one afterwards.
func (j *job) removeOldFolder() (string, error) {
	if j.oldDir == "" {
		return "nothing to tidy", nil
	}
	self, _ := os.Executable()
	if under(self, j.oldDir) {
		return j.oldDir + " (removed when this window closes)", removeAfterExit(j.oldDir)
	}
	return "removed " + j.oldDir, os.RemoveAll(j.oldDir)
}

// serviceControl starts, stops or restarts the installed service (ppserver service ..., used by the Manager).
func serviceControl(action string) error {
	switch action {
	case "stop":
		_, err := stopService()
		return err
	case "start", "restart":
		if action == "restart" {
			if _, err := stopService(); err != nil {
				return err
			}
		}
		m, err := mgr.Connect()
		if err != nil {
			return err
		}
		defer m.Disconnect()
		s, err := m.OpenService(ServiceName)
		if err != nil {
			return errors.New("ParallelPath Server is not installed")
		}
		defer s.Close()
		if st, err := s.Query(); err == nil && st.State == svc.Running {
			return nil
		}
		return s.Start()
	}
	return errors.New("use start, stop or restart")
}
