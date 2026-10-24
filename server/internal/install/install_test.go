package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validPlan(t *testing.T) *Plan {
	dir := t.TempDir()
	return &Plan{Mode: "install", DataDir: filepath.Join(dir, "data"), Name: "Family server", Hosts: []string{"pp.example.org", "192.0.2.10"},
		Port: 8443, Backups: true, BackupDir: filepath.Join(dir, "backups"), BackupKeep: 14}
}

func TestPlanValidate(t *testing.T) {
	if err := validPlan(t).Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Plan){
		"mode":         func(p *Plan) { p.Mode = "wipe" },
		"relative":     func(p *Plan) { p.DataDir = "data" },
		"root":         func(p *Plan) { p.DataDir = filepath.VolumeName(os.TempDir()) + string(filepath.Separator) },
		"name":         func(p *Plan) { p.Name = " " },
		"control":      func(p *Plan) { p.Name = "a\nb" },
		"no hosts":     func(p *Plan) { p.Hosts = nil },
		"bad host":     func(p *Plan) { p.Hosts = []string{"pp example"} },
		"dash host":    func(p *Plan) { p.Hosts = []string{"-pp.example.org"} },
		"port":         func(p *Plan) { p.Port = 70000 },
		"admin port":   func(p *Plan) { p.Port = 8444 },
		"backup keep":  func(p *Plan) { p.BackupKeep = 0 },
		"backup dir":   func(p *Plan) { p.BackupDir = "backups" },
		"container gb": func(p *Plan) { p.Container, p.ContainerGB = true, 0 },
	}
	for name, mutate := range cases {
		p := validPlan(t)
		mutate(p)
		if p.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	up := &Plan{Mode: "upgrade", DataDir: validPlan(t).DataDir}
	if err := up.Validate(); err != nil {
		t.Errorf("upgrade needs only the data folder: %v", err)
	}
}

func TestCheckDataDir(t *testing.T) {
	dir := t.TempDir()
	if ex, err := checkDataDir(filepath.Join(dir, "new")); ex || err != nil {
		t.Fatalf("new folder: %v %v", ex, err)
	}
	os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("x"), 0o600)
	if _, err := checkDataDir(dir); err == nil {
		t.Fatal("a folder with other files was accepted")
	}
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o600)
	if ex, err := checkDataDir(dir); !ex || err != nil {
		t.Fatalf("existing ParallelPath data: %v %v", ex, err)
	}
}

func TestUnits(t *testing.T) {
	d := unitData{Bin: linuxBin, DataDir: "/var/lib/parallelpath", BackupDir: "/var/backups/parallelpath"}
	m := render(mainUnit, d)
	for _, want := range []string{"User=parallelpath", "ExecStart=/usr/local/bin/ppserver run --data /var/lib/parallelpath",
		"LoadCredentialEncrypted=master_key:/etc/credstore.encrypted/parallelpath-master_key",
		"ReadWritePaths=/var/lib/parallelpath /var/backups/parallelpath", "CapabilityBoundingSet=\n", "NoNewPrivileges=yes"} {
		if !strings.Contains(m, want) {
			t.Errorf("main unit lacks %q:\n%s", want, m)
		}
	}
	if strings.Contains(m, "Requires=") || strings.Contains(m, "CAP_NET_BIND_SERVICE") {
		t.Errorf("unexpected container/low-port lines:\n%s", m)
	}
	d.Container, d.LowPort = true, true
	d.ContainerFile, d.Cryptsetup, d.Mount, d.Umount = "/var/lib/parallelpath.luks", "/usr/sbin/cryptsetup", "/usr/bin/mount", "/usr/bin/umount"
	m = render(mainUnit, d)
	if !strings.Contains(m, "Requires=parallelpath-volume.service") || !strings.Contains(m, "CapabilityBoundingSet=CAP_NET_BIND_SERVICE") {
		t.Errorf("container/low-port lines missing:\n%s", m)
	}
	v := render(volumeUnit, d)
	if !strings.Contains(v, "--key-file ${CREDENTIALS_DIRECTORY}/volume_key /var/lib/parallelpath.luks parallelpath-data") {
		t.Errorf("volume unit:\n%s", v)
	}
}

func TestGroupHex(t *testing.T) {
	if g := groupHex("9c98c70cebdb"); g != "9C98 C70C EBDB" {
		t.Fatal(g)
	}
}
