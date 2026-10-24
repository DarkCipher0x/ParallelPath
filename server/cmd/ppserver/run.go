package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"pp/core/pqc"
	"pp/core/wire"
	"pp/server/internal/admin"
	"pp/server/internal/api"
	"pp/server/internal/atrest"
	"pp/server/internal/backup"
	"pp/server/internal/config"
	"pp/server/internal/logring"
	"pp/server/internal/portal"
	"pp/server/internal/seal"
	"pp/server/internal/serverinfo"
	"pp/server/internal/store"
	"pp/server/internal/svc"
	"pp/server/internal/tlsx"
)

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	data := dataFlag(fs)
	fs.Parse(args)
	if isWindowsService() {
		return runWindowsService(*data)
	}
	log := newLogger(os.Stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := serve(ctx, *data, log)
	if errors.Is(err, errRestart) {
		log.Info("restart requested from the admin portal; exiting so the service manager starts the server again")
		os.Exit(restartExitCode)
	}
	return err
}

// serve runs the server until ctx ends. On the very first start of an installed server (a config but no database
// yet) it creates the master key and database itself, so the key is sealed to the account the service runs as.
func serve(ctx context.Context, data string, log *slog.Logger) error {
	c, err := config.Load(data)
	if err != nil {
		return fmt.Errorf("no server in %s (install it, or run `ppserver init`): %w", data, err)
	}
	checkErr := atrest.Check(data)
	volume, _ := atrest.VolumeID(data)
	atRest, err := atrest.Decide(checkErr, c.AtRestVerified, volume, c.AllowUnencryptedVolume)
	if err != nil {
		return fmt.Errorf("%w - refusing to start: encryption at rest is required", err)
	}
	if checkErr != nil {
		log.Warn("encryption at rest: "+atRest, "check", checkErr, "volume", volume)
	}
	dbPath := filepath.Join(data, "parallelpath.db")
	_, statErr := os.Stat(dbPath)
	first := errors.Is(statErr, os.ErrNotExist)
	master, err := seal.Load(data, first)
	if err != nil {
		return err
	}
	k, err := seal.Derive(master)
	pqc.Wipe(master)
	if err != nil {
		return err
	}
	db, err := store.Open(dbPath, k.Data)
	if err != nil {
		return err
	}
	defer db.Close()
	if first {
		log.Info("first start: created the master key and database")
	}
	if err := admin.EnsureToken(data); err != nil {
		return err
	}

	cfg := svc.DefaultConfig
	cfg.MaxBlobBytes, cfg.SpaceQuota = c.MaxBlobMB<<20, c.QuotaGB<<30
	hub := api.NewHub()
	cfg.TurnSecret = k.TURN
	s := svc.New(db, k.Signing, k.Session, hub, cfg)
	if relay, port := startRelay(c, k.TURN, s.TurnUserValid, log); relay != nil {
		defer relay.Close()
		s.SetTurnPort(port)
	}
	api.Version = version

	certs, err := tlsx.Open(tlsx.Options{Mode: c.CertMode(data), DataDir: data, Name: c.Name, Hosts: c.Hosts, CertFile: c.CertFile,
		KeyFile: c.KeyFile, ACMEDomains: c.ACMEDomains, ACMEEmail: c.ACMEEmail, KEK: k.TLS})
	if err != nil {
		return err
	}
	certPrint := fmt.Sprintf("%x", certs.Fingerprint())
	s.SetTLSPin(certs.Fingerprint)
	// An operator can restart the server from the admin portal: serving stops and the process exits with
	// restartExitCode, so the service manager (Windows SCM recovery, systemd Restart=on-failure) starts it again.
	ctx, stopServing := context.WithCancel(ctx)
	defer stopServing()
	var restart atomic.Bool
	adminPortal, err := portal.New(portal.Options{DB: db, Svc: s, DataDir: data, Logs: logs, Log: log,
		Status: portal.Status{Version: version, Started: time.Now().UTC(), AtRest: atRest, ServerKey: wire.Fingerprint(k.Signing.Public().ID()),
			CertSHA256: certPrint, OS: runtime.GOOS, TLSMode: certs.Mode(), Uncovered: certs.Uncovered},
		Restart: func() { restart.Store(true); stopServing() }})
	if err != nil {
		return err
	}
	root := http.NewServeMux()
	root.Handle("/admin/", adminPortal.Handler())
	root.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	root.Handle("/", api.WithTrust(api.New(s, hub, data, c.Name, log), api.Trust{Mode: certs.Mode(), CAPEM: certs.CACertPEM(),
		Fingerprint: certs.Fingerprint(), Name: c.Name}))
	pub := &http.Server{
		Addr:              c.Listen,
		Handler:           root,
		TLSConfig:         certs.Config(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	adm, err := adminServer(c.AdminListen, data, s, adminPortal)
	if err != nil {
		return err
	}
	// Listen before announcing, so server.json only appears once the server really accepts connections.
	pubLn, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	admLn, err := net.Listen("tcp", c.AdminListen)
	if err != nil {
		pubLn.Close()
		return err
	}

	errc := make(chan error, 4)
	go func() { errc <- pub.ServeTLS(pubLn, "", "") }()
	go func() { errc <- adm.Serve(admLn) }()
	extra := extraListeners(c, certs, root, log, errc)
	defer func() {
		for _, x := range extra {
			x.Close()
		}
	}()
	go renewDaily(ctx, certs, log)
	info := &serverinfo.Info{
		Name: c.Name, Version: version, ServerKey: wire.Fingerprint(k.Signing.Public().ID()),
		CertSHA256: certPrint, Listen: c.Listen, Hosts: c.Hosts, AtRest: atRest,
		PID: os.Getpid(), Started: time.Now().UTC(),
	}
	if err := serverinfo.Write(data, info); err != nil {
		log.Warn("couldn't write server.json", "err", err)
	}
	defer serverinfo.Remove(data)
	log.Info("ParallelPath server running", "name", c.Name, "listen", c.Listen, "version", version,
		"server_key", info.ServerKey, "tls_cert_sha256", info.CertSHA256, "at_rest", atRest)
	_ = s.AdminAudit("server.start", version)

	bctx, stopBackups := context.WithCancel(ctx)
	defer stopBackups()
	if c.BackupDir != "" {
		go backup.Run(bctx, db, data, c.BackupDir, c.BackupKeep, 10*time.Minute, 24*time.Hour, log)
	}

	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = adm.Shutdown(sctx)
	if err := pub.Shutdown(sctx); err != nil {
		return err
	}
	if restart.Load() {
		return errRestart
	}
	return nil
}

// errRestart: an operator asked for a restart; the process exits with restartExitCode and is started again.
var errRestart = errors.New("restart requested")

const restartExitCode = 3

// logs keeps the latest log lines for the admin portal.
var logs = logring.New(500)

// newLogger logs to w and to the portal's in-memory copy.
func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.MultiWriter(w, logs), &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// adminServer is the operator's loopback-only endpoint, protected by the admin token in the data folder.
func adminServer(addr, data string, s *svc.Service, p *portal.Portal) (*http.Server, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost") {
		return nil, fmt.Errorf("admin_listen must be a loopback address, got %q", addr)
	}
	tok, err := admin.ReadToken(data)
	if err != nil {
		return nil, err
	}
	m := http.NewServeMux()
	m.HandleFunc("POST /admin/v1/invite", func(w http.ResponseWriter, r *http.Request) {
		if !pqc.Equal([]byte(r.Header.Get("X-PP-Admin")), tok) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		code, exp, err := s.CreateServerInvite()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s\n%s\n", code, exp.Format(time.RFC3339))
	})
	// One-time admin portal links. Whoever can read the admin token is an administrator of this machine.
	m.HandleFunc("POST /admin/v1/link/{kind}", func(w http.ResponseWriter, r *http.Request) {
		if !pqc.Equal([]byte(r.Header.Get("X-PP-Admin")), tok) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		kind := r.PathValue("kind")
		if kind != "setup" && kind != "local" {
			http.NotFound(w, r)
			return
		}
		t, err := p.Accounts().NewToken(kind, "", "the server's administrator")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if kind == "setup" {
			_ = s.AdminAudit("admin.invite", "a link for a new admin account was made on the server")
			fmt.Fprintln(w, p.SetupURL(t))
			return
		}
		fmt.Fprintln(w, p.LocalURL(t))
	})
	return &http.Server{Addr: addr, Handler: m, ReadHeaderTimeout: 5 * time.Second}, nil
}
