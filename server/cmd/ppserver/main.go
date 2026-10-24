// ppserver is the ParallelPath server for Windows and Linux.
//
//	ppserver install                    interactive setup: service, firewall, backups, encryption checks
//	ppserver install --plan FILE|-      unattended setup from a plan (the Windows setup wizard uses this)
//	ppserver uninstall [--purge]        remove the service (and with --purge, all data)
//	ppserver status                     is the installed server running? fingerprints, addresses
//	ppserver invite                     one-time code that lets someone create an account
//	ppserver init / run                 manual setup and serving, without installing a service
//	ppserver version
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pp/core/pqc"
	"pp/core/wire"
	"pp/server/internal/admin"
	"pp/server/internal/atrest"
	"pp/server/internal/config"
	"pp/server/internal/install"
	"pp/server/internal/seal"
	"pp/server/internal/store"
	"pp/server/internal/tlsx"
)

var version = "dev"

func main() {
	install.Version = version
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "install":
		err = install.Command(os.Args[2:])
	case "uninstall":
		err = install.UninstallCommand(os.Args[2:])
	case "status":
		err = install.StatusCommand(os.Args[2:])
	case "service":
		err = install.ServiceCommand(os.Args[2:])
	case "admin":
		err = cmdAdmin(os.Args[2:])
	case "init":
		err = cmdInit(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "invite":
		err = cmdInvite(os.Args[2:])
	case "verify-report":
		err = cmdVerifyReport(os.Args[2:])
	case "version", "--version":
		fmt.Println("ppserver", version)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ppserver:", err)
		install.PauseIfAsked()
		os.Exit(1)
	}
	install.PauseIfAsked()
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  ppserver install                   interactive setup (service, firewall, backups, encryption)
  ppserver install --plan FILE|-     unattended setup from a JSON plan
  ppserver uninstall [--purge]       remove the installed server (--purge also deletes its data)
  ppserver status                    show the installed server's state and fingerprints
  ppserver service start|stop|restart   control the installed service
  ppserver invite                    one-time code to create an account
  ppserver admin setup-link|login-link   one-time links into the admin portal
  ppserver init --data DIR --name NAME --host HOST[,HOST]   manual setup, no service
  ppserver run  --data DIR                                   serve in the foreground
  ppserver verify-report FILE.json                           check a court-ready report's signature
  ppserver version`)
	os.Exit(2)
}

// dataFlag is --data, defaulting to $PP_DATA, then the installed server's data folder, then ./data.
func dataFlag(fs *flag.FlagSet) *string {
	def := "data"
	if v := os.Getenv("PP_DATA"); v != "" {
		def = v
	} else if d := install.InstalledDataDir(); d != "" {
		def = d
	}
	return fs.String("data", def, "data directory")
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	data := dataFlag(fs)
	name := fs.String("name", "ParallelPath", "server name shown in the apps")
	hosts := fs.String("host", "localhost", "host name(s)/IP(s) for the certificate, comma-separated")
	listen := fs.String("listen", ":8443", "HTTPS listen address")
	dev := fs.Bool("dev", false, "testing: allow a data folder that isn't on an encrypted volume")
	fs.Parse(args)
	if _, err := os.Stat(config.Path(*data)); err == nil {
		return fmt.Errorf("%s already has a server", *data)
	}
	if err := os.MkdirAll(*data, 0o700); err != nil {
		return err
	}
	c := config.Default(*name, strings.Split(*hosts, ","))
	c.Listen, c.AllowUnencryptedVolume = *listen, *dev
	if !*dev {
		if err := atrest.Check(*data); err != nil {
			return fmt.Errorf("%w\nEncryption at rest is required. Put the data folder on a BitLocker (Windows) or LUKS (Linux) "+
				"volume, or add --dev for a test server", err)
		}
	}
	master, err := seal.Load(*data, true)
	if err != nil {
		return err
	}
	defer pqc.Wipe(master)
	k, err := seal.Derive(master)
	if err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(*data, "parallelpath.db"), k.Data)
	if err != nil {
		return err
	}
	db.Close()
	if err := c.Save(*data); err != nil {
		return err
	}
	if err := admin.EnsureToken(*data); err != nil {
		return err
	}
	certs, err := tlsx.Open(tlsx.Options{Mode: c.CertMode(*data), DataDir: *data, Name: c.Name, Hosts: c.Hosts, CertFile: c.CertFile,
		KeyFile: c.KeyFile, ACMEDomains: c.ACMEDomains, ACMEEmail: c.ACMEEmail, KEK: k.TLS})
	if err != nil {
		return err
	}
	fmt.Printf("Server created in %s\n", *data)
	fmt.Printf("  server key fingerprint: %s\n", wire.Fingerprint(k.Signing.Public().ID()))
	fmt.Printf("  TLS certificate (%s) SHA-256: %x\n", certs.Mode(), certs.Fingerprint())
	fmt.Println("Start it with: ppserver run --data", *data)
	return nil
}

func cmdInvite(args []string) error {
	fs := flag.NewFlagSet("invite", flag.ExitOnError)
	data := dataFlag(fs)
	install.PauseFlag(fs)
	fs.Parse(args)
	if *data == install.InstalledDataDir() {
		install.ElevateIfNeeded() // the installed data folder is for administrators only
	}
	code, exp, err := admin.Invite(*data)
	if err != nil {
		return err
	}
	fmt.Println("One-time invite code (give it to the person who will create the family space):")
	fmt.Println()
	fmt.Println("   ", code)
	if !exp.IsZero() {
		fmt.Println()
		fmt.Println("Valid until", exp.Local().Format(time.DateTime), "- it works once.")
	}
	return nil
}
