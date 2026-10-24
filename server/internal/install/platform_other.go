//go:build !windows && !linux

package install

import "errors"

// The server installs on Windows and Linux; other systems can still build and run ppserver by hand.

const adminHint = "installing a service is supported on Windows and Linux only"

const encryptionAdvice = ""

var errUnsupported = errors.New(adminHint)

func isAdmin() bool                  { return false }
func enableVT() bool                 { return true }
func defaults() *Plan                { return &Plan{Port: 8443} }
func features(*Plan, bool) []feature { return nil }
func existingInstall() *Existing     { return nil }
func platformLogs(string) string     { return "" }
func relaunchElevated() bool         { return false }
func recentLog(string) string        { return "" }
func (j *job) steps() []step {
	return []step{{id: "checks", title: "Checking", run: func() (string, error) { return "", errUnsupported }}}
}
func uninstall(*Existing, bool, Reporter) error { return errUnsupported }

func platformChecks(*Plan, string, int, bool) []Check {
	return []Check{{ID: "os", Title: "Operating system", State: "fail", Detail: adminHint}}
}

func serviceControl(string) error { return errUnsupported }
