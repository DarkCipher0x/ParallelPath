package api

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"pp/server/internal/svc"
)

// signal relays an end-to-end encrypted call-setup message to the family's devices.
func (a *Server) signal(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		ToDevice string `json:"to_device"`
		Payload  []byte `json:"payload"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.Relay(*c, r.PathValue("space"), req.ToDevice, req.Payload); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

// turn hands a signed-in device short-lived credentials for the server's call relay, with the addresses to use
// (the same host name the device used to reach the server).
func (a *Server) turn(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	cfg := a.svc.Config()
	if cfg.TurnPort == 0 {
		ok(w, map[string]any{"ice_servers": []any{}})
		return
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		host = "[" + host + "]"
	}
	const ttl = 12 * time.Hour
	user, pass := svc.TurnCredentials(cfg.TurnSecret, c.Person, ttl)
	base := fmt.Sprintf("%s:%d", host, cfg.TurnPort)
	ok(w, map[string]any{
		"ice_servers": []map[string]any{
			{"urls": []string{"stun:" + base}},
			{"urls": []string{"turn:" + base + "?transport=udp", "turn:" + base + "?transport=tcp"}, "username": user, "credential": pass},
		},
		"ttl": int(ttl.Seconds()),
	})
}
