//go:build !windows

package main

// On Linux the server runs under systemd as an ordinary process (it logs to the journal through stderr).
func isWindowsService() bool { return false }

func runWindowsService(string) error { return nil }
