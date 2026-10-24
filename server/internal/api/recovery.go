package api

import (
	"net/http"

	"pp/core/keys"
	"pp/server/internal/svc"
)

func (a *Server) recoverySave(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		ID     string `json:"id"`
		Sealed []byte `json:"sealed"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.RecoverySave(*c, req.ID, req.Sealed); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) recoveryFetch(w http.ResponseWriter, r *http.Request) {
	b, err := a.svc.RecoveryFetch(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"sealed": b})
}

func (a *Server) recoveryRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cert keys.DeviceCert `json:"cert"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.RecoveryRegister(r.PathValue("id"), req.Cert); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}
