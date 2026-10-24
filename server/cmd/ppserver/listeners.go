package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"pp/server/internal/config"
	"pp/server/internal/svc"
	"pp/server/internal/tlsx"
	"pp/server/internal/turn"
)

// extraListeners starts the optional listeners next to the HTTPS one:
//
//   - http_listen (e.g. ":80"): redirects to HTTPS and answers Let's Encrypt HTTP-01 challenges;
//   - PP_DEV_HTTP (development only, loopback only): the whole server over plain HTTP on this machine, so the web app
//     can be tried in a browser without installing a certificate (browsers treat http://127.0.0.1 as secure).
func extraListeners(c *config.Config, certs *tlsx.Source, root http.Handler, log *slog.Logger, errc chan<- error) []*http.Server {
	var out []*http.Server
	start := func(addr string, h http.Handler, what string) {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Warn("couldn't listen for "+what, "addr", addr, "err", err)
			return
		}
		srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
		out = append(out, srv)
		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				errc <- err
			}
		}()
		log.Info("listening for "+what, "addr", addr)
	}
	if c.HTTPListen != "" {
		_, port, _ := net.SplitHostPort(c.Listen)
		start(c.HTTPListen, certs.HTTPHandler(port), "HTTP redirects and certificate challenges")
	}
	if dev := os.Getenv("PP_DEV_HTTP"); dev != "" {
		host, _, err := net.SplitHostPort(dev)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			log.Warn("PP_DEV_HTTP must be a loopback address (127.0.0.1:port) - ignored", "value", dev)
		} else {
			log.Warn("development plain-HTTP listener on this machine only - never use this for real families", "addr", dev)
			start(dev, root, "development (plain HTTP, this machine only)")
		}
	}
	return out
}

// renewDaily keeps the local CA's server certificate current.
func renewDaily(ctx context.Context, certs *tlsx.Source, log *slog.Logger) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := certs.Renew(); err != nil {
				log.Warn("certificate renewal failed", "err", err)
			}
		}
	}
}

// startRelay starts the call relay (TURN) unless it's switched off; it returns the port devices should use.
func startRelay(c *config.Config, secret []byte, valid func(string) bool, log *slog.Logger) (*turn.Server, int) {
	listen := c.TURNListen
	if listen == "off" {
		return nil, 0
	}
	if listen == "" {
		listen = ":3478"
	}
	relayIP := c.TURNRelayIP
	if relayIP == "" {
		relayIP = turn.GuessRelayIP(c.Hosts)
	}
	ports := c.TURNPorts
	if ports == "" {
		ports = "49160-49200"
	}
	srv, err := turn.Start(turn.Config{Listen: listen, RelayIP: relayIP, PortRange: ports, Realm: "parallelpath",
		Valid:    valid,
		Password: func(u string) string { return svc.TurnPassword(secret, u) },
		Log:      log})
	if err != nil {
		log.Warn("the call relay (TURN) couldn't start - calls will only connect directly", "listen", listen, "err", err)
		return nil, 0
	}
	_, port, _ := net.SplitHostPort(listen)
	p, _ := strconv.Atoi(port)
	return srv, p
}
