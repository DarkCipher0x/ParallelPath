package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"pp/server/internal/admin"
	"pp/server/internal/install"
)

// cmdAdmin is `ppserver admin setup-link | login-link`: one-time links into the admin portal, made on the server by
// someone with administrator rights there (root, or an elevated Windows administrator).
//
//	setup-link   create an admin account (works once, from anywhere the portal is reachable, for 24 hours)
//	login-link   sign in from this machine without a passphrase (works once, for 2 minutes)
//
// --out FILE writes the link to a new file instead of printing it (ParallelPath Server Manager uses this, because it
// can't read the output of an elevated program).
func cmdAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ppserver admin setup-link | login-link [--out FILE]")
	}
	which := args[0]
	fs := flag.NewFlagSet("admin "+which, flag.ExitOnError)
	data := dataFlag(fs)
	out := fs.String("out", "", "write the link to this new file instead of printing it")
	install.PauseFlag(fs)
	fs.Parse(args[1:])
	kind := map[string]string{"setup-link": "setup", "login-link": "local"}[which]
	if kind == "" {
		return errors.New("usage: ppserver admin setup-link | login-link [--out FILE]")
	}
	if *data == install.InstalledDataDir() && *out == "" {
		install.ElevateIfNeeded()
	}
	link, err := admin.Link(*data, kind)
	if err != nil {
		return err
	}
	if *out != "" {
		// A new file only: never follow or overwrite something already there.
		f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(link)
		return errors.Join(err, f.Close())
	}
	if kind == "setup" {
		fmt.Println("Open this link to create an admin account for the ParallelPath admin portal.")
		fmt.Println("It works once, within 24 hours - keep it private:")
	} else {
		fmt.Println("Open this link on this machine to sign in to the admin portal (works once, within 2 minutes):")
	}
	fmt.Println()
	fmt.Println("   ", link)
	return nil
}
