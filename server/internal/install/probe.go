package install

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"pp/server/internal/atrest"
	"pp/server/internal/config"
)

// Check is one line of the "is this machine ready?" list.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  string `json:"state"` // ok, warn, fail or info
	Detail string `json:"detail"`
}

// Existing describes a server that is already installed.
type Existing struct {
	Version    string   `json:"version"`
	InstallDir string   `json:"install_dir,omitempty"`
	DataDir    string   `json:"data_dir"`
	Name       string   `json:"name"`
	Hosts      []string `json:"hosts"`
	Port       string   `json:"port"`
	State      string   `json:"state"` // running, stopped, ...
}

// Probe is what the installer finds before it changes anything.
type Probe struct {
	OS        string    `json:"os"`
	Version   string    `json:"version"` // of this installer
	Admin     bool      `json:"admin"`
	Hostname  string    `json:"hostname"`
	Addresses []string  `json:"addresses"`
	Existing  *Existing `json:"existing,omitempty"`
	Checks    []Check   `json:"checks"`
	Defaults  *Plan     `json:"defaults"`
}

// Ready reports whether no check failed.
func (p *Probe) Ready() bool {
	for _, c := range p.Checks {
		if c.State == "fail" {
			return false
		}
	}
	return true
}

// RunProbe looks at this machine. dataDir and port are what the person has chosen so far ("" and 0 for defaults).
func RunProbe(dataDir string, port int) *Probe {
	p := &Probe{OS: runtime.GOOS, Version: Version, Admin: isAdmin()}
	p.Hostname, _ = os.Hostname()
	p.Addresses = localAddresses()
	p.Existing = existingInstall()
	unfinished := false
	if e := p.Existing; e != nil {
		if _, err := os.Stat(config.Path(e.DataDir)); err != nil {
			// Registered but never configured: an earlier install stopped part-way. Install again over it.
			p.Existing, unfinished = nil, true
		}
	}
	d := defaults()
	d.Mode, d.Name = "install", "Family server"
	if h := strings.ToLower(p.Hostname); validHost(h) {
		d.Hosts = append(d.Hosts, h)
	}
	for _, a := range p.Addresses {
		if len(d.Hosts) < 6 {
			d.Hosts = append(d.Hosts, a)
		}
	}
	if e := p.Existing; e != nil {
		d.Mode, d.DataDir, d.Name, d.Hosts = "upgrade", e.DataDir, e.Name, e.Hosts
		if e.InstallDir != "" {
			d.InstallDir = e.InstallDir
		}
	}
	if dataDir == "" {
		dataDir = d.DataDir
	}
	if port == 0 {
		port = d.Port
	}
	if p.Existing == nil {
		// Suggest the encrypted container on Linux when the disk isn't encrypted.
		d.Container = d.Container || (runtime.GOOS == "linux" && atRestOf(dataDir) != nil)
	}
	p.Defaults = d
	p.Checks = platformChecks(d, dataDir, port, p.Existing != nil)
	if unfinished {
		p.Checks = append(p.Checks, Check{ID: "previous", Title: "Earlier installation", State: "info",
			Detail: "an earlier install didn't finish; setup will complete it"})
	}
	return p
}

// readExisting fills in an Existing from the data folder's config.
func readExisting(e *Existing) *Existing {
	if c, err := config.Load(e.DataDir); err == nil {
		e.Name, e.Hosts = c.Name, c.Hosts
		_, e.Port, _ = net.SplitHostPort(c.Listen)
	}
	return e
}

// atRestOf checks the nearest folder that exists at or above dir.
func atRestOf(dir string) error {
	d := filepath.Clean(dir)
	for {
		if _, err := os.Stat(d); err == nil || filepath.Dir(d) == d {
			return atrest.Check(d)
		}
		d = filepath.Dir(d)
	}
}

func encryptionCheck(dataDir string, unencryptedOK bool, fix string) Check {
	c := Check{ID: "encryption", Title: "Data folder on an encrypted volume"}
	if err := atRestOf(dataDir); err != nil {
		c.State, c.Detail = "warn", firstLine(err.Error())+". "+fix
		if !unencryptedOK {
			c.State = "fail"
		}
		return c
	}
	c.State, c.Detail = "ok", "encrypted"
	return c
}

func portCheck(port int, upgrading bool) Check {
	c := Check{ID: "port", Title: "Port free", State: "ok", Detail: "port " + strconv.Itoa(port) + " is free"}
	if upgrading {
		c.State, c.Detail = "info", "the installed server keeps its port"
	} else if err := portFree(port); err != nil {
		c.State, c.Detail = "fail", err.Error()+"; choose another port or stop the program using it"
	}
	return c
}

// localAddresses lists this machine's own IP addresses (not loopback or link-local), IPv4 first.
func localAddresses() []string {
	var v4, v6 []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || virtualAdapter(i.Name) {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.IsLinkLocalUnicast() || n.IP.IsLoopback() {
				continue
			}
			if n.IP.To4() != nil {
				v4 = append(v4, n.IP.String())
			} else {
				v6 = append(v6, n.IP.String())
			}
		}
	}
	return append(v4, v6...)
}

// virtualAdapter spots the host-only switches that hypervisors and container engines add (Hyper-V and WSL,
// VirtualBox, VMware, Docker, libvirt, LXD): other devices can't reach the server through those.
func virtualAdapter(name string) bool {
	n := strings.ToLower(name)
	for _, p := range []string{"vethernet", "virtualbox", "vmware", "docker", "veth", "br-", "virbr", "lxdbr", "cni", "flannel"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return strings.TrimPrefix(s, "atrest: ")
}
