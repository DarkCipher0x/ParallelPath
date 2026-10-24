//go:build windows

// "Uninstall ParallelPath.exe" removes the ParallelPath app for the current Windows user. It is installed next to
// the app by ParallelPath Setup and run from Settings › Apps (it's small, so the 170 MB setup isn't kept around).
//
// It asks before doing anything, makes sure the app is closed, and only deletes the encrypted vault and keys on this
// PC when the person says so (their family's messages stay on the server either way).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	title        = "ParallelPath"
	uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\ParallelPath`
	prefsKey     = `Software\JavaSoft\Prefs\systems\darkcipher\parallelpath`

	idYes, idRetry = 6, 4 // MessageBox answers
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		fail(err.Error())
	}
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, "ParallelPath.exe")); err != nil {
		fail("This doesn't look like a ParallelPath folder, so nothing was removed.")
	}
	quiet := len(os.Args) > 1 && os.Args[1] == "--quiet"
	if !quiet && ask("Remove ParallelPath from this PC?", windows.MB_YESNO|windows.MB_ICONQUESTION) != idYes {
		return
	}
	for running(dir) {
		if quiet || ask("ParallelPath is still open. Close it (your work is saved), then press Retry.",
			windows.MB_RETRYCANCEL|windows.MB_ICONWARNING) != idRetry {
			return
		}
	}
	purge := !quiet && ask("Also delete your ParallelPath data on this PC?\n\n"+
		"This removes your encrypted vault and this device's keys. Your family's messages and files stay on your server; "+
		"to use this PC again you would join with a new invite.\n\nChoose No to keep them (recommended if you'll reinstall).",
		windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_DEFBUTTON2) == idYes

	for _, f := range []windows.KNOWNFOLDERID{*windows.FOLDERID_Desktop, *windows.FOLDERID_Programs} {
		if p, err := windows.KnownFolderPath(&f, 0); err == nil {
			os.Remove(filepath.Join(p, "ParallelPath.lnk"))
		}
	}
	registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
	if purge {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			os.RemoveAll(filepath.Join(base, "ParallelPath"))
		}
		deleteTree(registry.CURRENT_USER, prefsKey)
	}
	removeLater(dir)
	if !quiet {
		msg := "ParallelPath has been removed."
		if !purge {
			msg += "\n\nYour encrypted data is still on this PC, ready if you reinstall."
		}
		ask(msg, windows.MB_OK|windows.MB_ICONINFORMATION)
	}
}

// running reports whether a ParallelPath program from this folder is still running.
func running(dir string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		if name != "parallelpath.exe" && name != "ppengine.exe" && name != "java.exe" {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &n)
		windows.CloseHandle(h)
		if err == nil && strings.HasPrefix(strings.ToLower(windows.UTF16ToString(buf[:n])), strings.ToLower(dir)+`\`) {
			return true
		}
	}
	return false
}

// removeLater deletes the app folder once this program has exited (Windows can't delete a running program). It
// retries for a while: antivirus scanners often hold just-closed files open for a moment.

func deleteTree(root registry.Key, path string) {
	if k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS); err == nil {
		subs, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, s := range subs {
			deleteTree(root, path+`\`+s)
		}
	}
	registry.DeleteKey(root, path)
}

func ask(text string, flags uint32) int32 {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(title)
	r, _ := windows.MessageBox(0, t, c, flags|windows.MB_SETFOREGROUND)
	return r
}

func fail(text string) {
	ask(text, windows.MB_OK|windows.MB_ICONERROR)
	os.Exit(1)
}

// removeLater deletes the app folder once this program has exited (Windows can't delete a running program). A hidden
// helper retries for a while, because antivirus scanners often hold just-closed files open for a moment. It leaves
// any job object this program runs in when allowed, or it would be killed as soon as this program exits.
func removeLater(dir string) {
	sys := filepath.Join(os.Getenv("SystemRoot"), "System32")
	q := `"` + strings.ReplaceAll(dir, `"`, ``) + `"`
	start := func(flags uint32) error {
		cmd := exec.Command(filepath.Join(sys, "cmd.exe"))
		cmd.Dir = sys
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags,
			CmdLine: `cmd.exe /d /c for /l %i in (1,1,20) do @(if exist ` + q + ` (ping -n 2 127.0.0.1 >nul & rd /s /q ` + q + ` 2>nul))`}
		return cmd.Start()
	}
	flags := uint32(windows.CREATE_NO_WINDOW | windows.DETACHED_PROCESS)
	if start(flags|windows.CREATE_BREAKAWAY_FROM_JOB) != nil {
		start(flags)
	}
}
