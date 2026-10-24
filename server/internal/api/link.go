package api

import (
	"net/http"

	"pp/server/internal/svc"
)

type linkBody struct {
	Payload []byte `json:"payload"`
}

func (a *Server) linkRequest(w http.ResponseWriter, r *http.Request) {
	var req linkBody
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.LinkRequest(r.PathValue("id"), req.Payload); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) linkPending(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	p, err := a.svc.LinkPending(*c, r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, linkBody{Payload: p})
}

func (a *Server) linkAnswer(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req linkBody
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.LinkAnswer(*c, r.PathValue("id"), req.Payload); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) linkResult(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.LinkResult(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, linkBody{Payload: p})
}

func (a *Server) linkOpen(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	if err := a.svc.LinkOpen(*c, r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}
