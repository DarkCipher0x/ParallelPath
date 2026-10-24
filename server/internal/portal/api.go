package portal

import (
	"errors"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"pp/server/internal/backup"
)

func (p *Portal) routes(m *http.ServeMux) {
	m.HandleFunc("GET /admin/api/state", p.state)
	m.HandleFunc("POST /admin/api/setup/begin", p.setupBegin)
	m.HandleFunc("POST /admin/api/setup/finish", p.setupFinish)
	m.HandleFunc("POST /admin/api/login", p.login)
	m.HandleFunc("POST /admin/api/local-login", p.localLogin)
	m.HandleFunc("POST /admin/api/logout", p.authed(p.logout))

	m.HandleFunc("GET /admin/api/overview", p.authed(p.overview))
	m.HandleFunc("GET /admin/api/invites", p.authed(p.invites))
	m.HandleFunc("POST /admin/api/invites", p.authed(p.createInvite))
	m.HandleFunc("POST /admin/api/invites/{id}/revoke", p.authed(p.revokeInvite))
	m.HandleFunc("GET /admin/api/people", p.authed(p.people))
	m.HandleFunc("POST /admin/api/people/{id}/{action}", p.authed(p.personAction))
	m.HandleFunc("POST /admin/api/devices/{id}/{action}", p.authed(p.deviceAction))
	m.HandleFunc("GET /admin/api/spaces", p.authed(p.spaces))
	m.HandleFunc("GET /admin/api/backups", p.authed(p.backups))
	m.HandleFunc("POST /admin/api/backups", p.authed(p.backupNow))
	m.HandleFunc("GET /admin/api/settings", p.authed(p.settings))
	m.HandleFunc("PUT /admin/api/settings", p.authed(p.saveSettings))
	m.HandleFunc("GET /admin/api/logs", p.authed(p.logs))
	m.HandleFunc("GET /admin/api/audit", p.authed(p.audit))
	m.HandleFunc("GET /admin/api/admins", p.authed(p.admins))
	m.HandleFunc("POST /admin/api/admins", p.authed(p.newAdmin))
	m.HandleFunc("POST /admin/api/admins/{id}/{action}", p.authed(p.adminAction))
	m.HandleFunc("GET /admin/api/sessions", p.authed(p.sessions))
	m.HandleFunc("POST /admin/api/sessions/{id}/end", p.authed(p.endSession))
	m.HandleFunc("POST /admin/api/restart", p.authed(p.restart))
}

// ---- signing in

func (p *Portal) state(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"name": p.config().Name, "has_admins": p.st.HasAccounts(), "local": isLocal(r)}
	if c, err := r.Cookie(cookieName); err == nil {
		if sess, err := p.st.Session(c.Value); err == nil {
			out["me"] = map[string]any{"name": sess.Name, "csrf": sess.CSRF, "local": sess.Account == localAccount, "id": sess.ID}
		}
	}
	send(w, out)
}

func (p *Portal) setupBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := read(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if p.tooManyTries(clientAddr(r)) {
		fail(w, http.StatusTooManyRequests, errors.New("too many attempts - wait 15 minutes"))
		return
	}
	t, err := p.st.BeginSetup(req.Token)
	if err != nil {
		p.failedTry(clientAddr(r))
		fail(w, http.StatusBadRequest, err)
		return
	}
	resetName := ""
	if t.Account != "" {
		for _, a := range mustAccounts(p.st) {
			if a.ID == t.Account {
				resetName = a.Name
			}
		}
	}
	label := resetName
	if label == "" {
		label = "admin"
	}
	send(w, map[string]any{"setup_key": setupKey(t.TOTP), "uri": otpURI(t.TOTP, label), "reset": resetName})
}

func mustAccounts(s *Store) []Account {
	a, _ := s.Accounts()
	return a
}

func (p *Portal) setupFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := read(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if p.tooManyTries(clientAddr(r)) {
		fail(w, http.StatusTooManyRequests, errors.New("too many attempts - wait 15 minutes"))
		return
	}
	acc, err := p.st.FinishSetup(req.Token, req.Name, req.Password, req.Code)
	if err != nil {
		p.failedTry(clientAddr(r))
		fail(w, http.StatusBadRequest, err)
		return
	}
	if err := p.signIn(w, r, acc.ID, acc.Name); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, map[string]bool{"ok": true})
}

func (p *Portal) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := read(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	a := clientAddr(r)
	if p.tooManyTries(a) {
		fail(w, http.StatusTooManyRequests, errors.New("too many attempts from here - wait 15 minutes"))
		return
	}
	acc, err := p.st.Login(req.Name, req.Password, req.Code)
	if err != nil {
		p.failedTry(a)
		fail(w, http.StatusUnauthorized, err)
		return
	}
	if err := p.signIn(w, r, acc.ID, acc.Name); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, map[string]bool{"ok": true})
}

// localLogin: one-time links made on this machine work only from this machine.
func (p *Portal) localLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := read(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if !isLocal(r) {
		fail(w, http.StatusForbidden, errors.New("this kind of link only works on the server itself"))
		return
	}
	if err := p.st.LocalLogin(req.Token); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	name := "Administrator (this computer)"
	if runtime.GOOS == "linux" {
		name = "root (this server)"
	}
	if err := p.signIn(w, r, localAccount, name); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, map[string]bool{"ok": true})
}

func (p *Portal) logout(w http.ResponseWriter, r *http.Request, _ *Session) {
	if c, err := r.Cookie(cookieName); err == nil {
		_ = p.st.EndSession(c.Value)
	}
	p.setCookie(w, "", -1)
	send(w, map[string]bool{"ok": true})
}

// ---- the server

func (p *Portal) overview(w http.ResponseWriter, _ *http.Request, _ *Session) {
	o, err := p.o.Svc.AdminOverview()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	c := p.config()
	var dbSize int64
	if st, err := os.Stat(filepath.Join(p.o.DataDir, "parallelpath.db")); err == nil {
		dbSize = st.Size()
	}
	files := listBackups(c.BackupDir)
	var lastBackup *backupFile
	if len(files) > 0 {
		lastBackup = &files[0]
	}
	send(w, map[string]any{
		"status": p.o.Status, "uptime_seconds": int(p.o.Now().Sub(p.o.Status.Started).Seconds()),
		"name": c.Name, "listen": c.Listen, "hosts": c.Hosts, "database_bytes": dbSize, "counts": o,
		"backups":        map[string]any{"on": c.BackupDir != "", "keep": c.BackupKeep, "last": lastBackup},
		"portal_limited": len(c.AdminAllow) > 0, "unencrypted_allowed": c.AllowUnencryptedVolume,
		"calls": map[string]any{"relay_port": p.o.Svc.Config().TurnPort, "relay_ip": c.TURNRelayIP, "ports": c.TURNPorts},
	})
}

func (p *Portal) invites(w http.ResponseWriter, _ *http.Request, _ *Session) {
	inv, err := p.o.Svc.AdminInvites()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, inv)
}

func (p *Portal) createInvite(w http.ResponseWriter, _ *http.Request, s *Session) {
	code, exp, err := p.o.Svc.CreateServerInvite()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	_ = p.o.Svc.AdminAudit("invite.by", s.Name+" created a server invite")
	send(w, map[string]any{"code": code, "expires": exp})
}

func (p *Portal) revokeInvite(w http.ResponseWriter, r *http.Request, s *Session) {
	if err := p.o.Svc.AdminRevokeInvite(r.PathValue("id"), s.Name); err != nil {
		fail(w, errStatus(err), err)
		return
	}
	send(w, map[string]bool{"ok": true})
}

func (p *Portal) people(w http.ResponseWriter, _ *http.Request, _ *Session) {
	pp, err := p.o.Svc.AdminPeople()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, pp)
}

func (p *Portal) personAction(w http.ResponseWriter, r *http.Request, s *Session) {
	var err error
	switch r.PathValue("action") {
	case "block":
		err = p.o.Svc.AdminSetBlocked(r.PathValue("id"), true, s.Name)
	case "unblock":
		err = p.o.Svc.AdminSetBlocked(r.PathValue("id"), false, s.Name)
	case "families":
		// How many more families this person may start: bringing families onto the server is the operator's call.
		var req struct {
			Left int `json:"left"`
		}
		if err := read(r, &req); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		err = p.o.Svc.AdminSetFamiliesLeft(r.PathValue("id"), req.Left, s.Name)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, errStatus(err), err)
		return
	}
	send(w, map[string]bool{"ok": true})
}

func (p *Portal) deviceAction(w http.ResponseWriter, r *http.Request, s *Session) {
	var err error
	switch r.PathValue("action") {
	case "signout":
		err = p.o.Svc.AdminSignOutDevice(r.PathValue("id"), s.Name)
	case "revoke":
		err = p.o.Svc.AdminRevokeDevice(r.PathValue("id"), s.Name)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, errStatus(err), err)
		return
	}
	send(w, map[string]bool{"ok": true})
}

func (p *Portal) spaces(w http.ResponseWriter, _ *http.Request, _ *Session) {
	sp, err := p.o.Svc.AdminSpaces()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, sp)
}

// ---- backups

type backupFile struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	Time time.Time `json:"time"`
}

func listBackups(dir string) []backupFile {
	if dir == "" {
		return nil
	}
	entries, _ := os.ReadDir(dir)
	out := []backupFile{}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "parallelpath-") || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, backupFile{Name: e.Name(), Size: info.Size(), Time: info.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

func (p *Portal) backups(w http.ResponseWriter, _ *http.Request, _ *Session) {
	c := p.config()
	send(w, map[string]any{"on": c.BackupDir != "", "dir": c.BackupDir, "keep": c.BackupKeep, "files": listBackups(c.BackupDir)})
}

func (p *Portal) backupNow(w http.ResponseWriter, _ *http.Request, s *Session) {
	c := p.config()
	if c.BackupDir == "" {
		fail(w, http.StatusBadRequest, errors.New("backups are off - turn them on in setup (Windows) or `ppserver install` (Linux)"))
		return
	}
	path, err := backup.Once(p.o.DB, c.BackupDir, c.BackupKeep, p.o.Now())
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := backup.Blobs(p.o.DataDir, c.BackupDir); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	_ = p.o.Svc.AdminAudit("backup.now", s.Name+" made a backup: "+filepath.Base(path))
	send(w, map[string]string{"file": filepath.Base(path)})
}

// ---- settings

// editable is what can be changed here. The address, port, certificate, data and backup folders, the firewall and
// the service itself belong to setup (they need administrator rights on the machine).
type editable struct {
	Name       string   `json:"name"`
	MaxBlobMB  int64    `json:"max_blob_mb"`
	QuotaGB    int64    `json:"space_quota_gb"`
	BackupKeep int      `json:"backup_keep"`
	AdminAllow []string `json:"admin_allow"`
}

func (p *Portal) settings(w http.ResponseWriter, _ *http.Request, _ *Session) {
	c := p.config()
	send(w, map[string]any{
		"editable": editable{Name: c.Name, MaxBlobMB: c.MaxBlobMB, QuotaGB: c.QuotaGB, BackupKeep: c.BackupKeep, AdminAllow: c.AdminAllow},
		"fixed": map[string]any{"listen": c.Listen, "hosts": c.Hosts, "data_dir": p.o.DataDir, "backup_dir": c.BackupDir,
			"unencrypted_allowed": c.AllowUnencryptedVolume, "cert_file": c.CertFile != ""},
	})
}

func (p *Portal) saveSettings(w http.ResponseWriter, r *http.Request, s *Session) {
	var e editable
	if err := read(r, &e); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	e.Name = strings.TrimSpace(e.Name)
	switch {
	case e.Name == "" || len(e.Name) > 80:
		fail(w, http.StatusBadRequest, errors.New("the server name must be 1-80 characters"))
		return
	case e.MaxBlobMB < 1 || e.MaxBlobMB > 16384:
		fail(w, http.StatusBadRequest, errors.New("the largest file must be 1-16384 MB"))
		return
	case e.QuotaGB < 1 || e.QuotaGB > 100000:
		fail(w, http.StatusBadRequest, errors.New("the storage per family must be 1-100000 GB"))
		return
	}
	var allow []string
	for _, a := range e.AdminAllow {
		if a = strings.TrimSpace(a); a == "" {
			continue
		}
		pf, err := netip.ParsePrefix(a)
		if err != nil {
			addr, err2 := netip.ParseAddr(a)
			if err2 != nil {
				fail(w, http.StatusBadRequest, errors.New(a+" isn't a network like 192.168.1.0/24 or an address"))
				return
			}
			pf = netip.PrefixFrom(addr, addr.BitLen())
		}
		allow = append(allow, pf.Masked().String())
	}
	if me := clientAddr(r); len(allow) > 0 && !me.IsLoopback() && !covers(allow, me) {
		fail(w, http.StatusBadRequest, errors.New("those networks don't include the address you're using ("+me.String()+"), so you'd lock yourself out"))
		return
	}
	p.mu.Lock()
	c := *p.cfg
	c.Name, c.MaxBlobMB, c.QuotaGB, c.AdminAllow = e.Name, e.MaxBlobMB, e.QuotaGB, allow
	if c.BackupDir != "" {
		if e.BackupKeep < 1 || e.BackupKeep > 365 {
			p.mu.Unlock()
			fail(w, http.StatusBadRequest, errors.New("keep between 1 and 365 backups"))
			return
		}
		c.BackupKeep = e.BackupKeep
	}
	err := c.Save(p.o.DataDir)
	if err == nil {
		p.cfg = &c
	}
	p.mu.Unlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	_ = p.o.Svc.AdminAudit("settings", s.Name+" changed the settings")
	send(w, map[string]any{"ok": true, "restart_needed": true})
}

func covers(allow []string, a netip.Addr) bool {
	for _, s := range allow {
		if pf, err := netip.ParsePrefix(s); err == nil && pf.Contains(a) {
			return true
		}
	}
	return false
}

// ---- logs and audit

func (p *Portal) logs(w http.ResponseWriter, _ *http.Request, _ *Session) {
	lines := []string{}
	if p.o.Logs != nil {
		lines = p.o.Logs.Lines()
	}
	send(w, lines)
}

func (p *Portal) audit(w http.ResponseWriter, _ *http.Request, _ *Session) {
	entries, problem, err := p.o.Svc.ServerAudit(500)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, map[string]any{"verified": problem == "", "problem": problem, "entries": entries})
}

// ---- operators

func (p *Portal) admins(w http.ResponseWriter, _ *http.Request, _ *Session) {
	a, err := p.st.Accounts()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, a)
}

// newAdmin makes a one-time link for someone to create their own operator account (they choose their passphrase
// and set up their authenticator; nobody else ever knows either).
func (p *Portal) newAdmin(w http.ResponseWriter, _ *http.Request, s *Session) {
	tok, err := p.st.NewToken("setup", "", s.Name)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	_ = p.o.Svc.AdminAudit("admin.invite", s.Name+" made a link for a new admin account")
	send(w, map[string]any{"link": p.SetupURL(tok), "expires": p.o.Now().Add(SetupTTL)})
}

func (p *Portal) adminAction(w http.ResponseWriter, r *http.Request, s *Session) {
	id := r.PathValue("id")
	switch r.PathValue("action") {
	case "disable", "enable":
		if id == s.Account {
			fail(w, http.StatusBadRequest, errors.New("you can't disable your own account"))
			return
		}
		if err := p.st.SetDisabled(id, r.PathValue("action") == "disable", s.Name); err != nil {
			fail(w, errStatus(err), err)
			return
		}
		send(w, map[string]bool{"ok": true})
	case "reset":
		tok, err := p.st.NewToken("setup", id, s.Name)
		if err != nil {
			fail(w, http.StatusInternalServerError, err)
			return
		}
		_ = p.o.Svc.AdminAudit("admin.reset", s.Name+" made a reset link for admin account "+id[:min(12, len(id))])
		send(w, map[string]any{"link": p.SetupURL(tok), "expires": p.o.Now().Add(SetupTTL)})
	default:
		http.NotFound(w, r)
	}
}

func (p *Portal) sessions(w http.ResponseWriter, _ *http.Request, s *Session) {
	list, err := p.st.Sessions()
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	send(w, map[string]any{"me": s.ID, "sessions": list})
}

func (p *Portal) endSession(w http.ResponseWriter, r *http.Request, s *Session) {
	if err := p.st.EndSessionByID(r.PathValue("id")); err != nil {
		fail(w, errStatus(err), err)
		return
	}
	_ = p.o.Svc.AdminAudit("admin.signout", s.Name+" ended an admin session")
	send(w, map[string]bool{"ok": true})
}

func (p *Portal) restart(w http.ResponseWriter, _ *http.Request, s *Session) {
	_ = p.o.Svc.AdminAudit("server.restart", s.Name+" restarted the server")
	send(w, map[string]bool{"ok": true})
	if p.o.Restart != nil {
		go func() {
			time.Sleep(500 * time.Millisecond)
			p.o.Restart()
		}()
	}
}
