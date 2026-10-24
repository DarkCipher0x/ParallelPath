package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"pp/core/audit"
	"pp/core/envelope"
	"pp/core/keys"
	"pp/core/wire"
	"pp/server/internal/svc"
)

// --- accounts and sign-in ----------------------------------------------------------------------------------

func (a *Server) enrol(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Invite     string          `json:"invite"`
		Cert       keys.DeviceCert `json:"cert"`
		JoinIntent *audit.Intent   `json:"join_intent,omitempty"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := a.svc.Enrol(req.Invite, req.Cert, req.JoinIntent)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, res)
}

func (a *Server) challenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device string `json:"device"`
	}
	if !decode(w, r, &req) {
		return
	}
	nonce, err := a.svc.Challenge(req.Device)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"nonce": nonce})
}

func (a *Server) signIn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device    string `json:"device"`
		Signature []byte `json:"signature"`
	}
	if !decode(w, r, &req) {
		return
	}
	tok, c, err := a.svc.SignIn(req.Device, req.Signature)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"token": tok, "person": c.Person})
}

func (a *Server) signOut(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	tok := r.Header.Get("Authorization")[len("Bearer "):]
	if err := a.svc.SignOut(tok); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) addDevice(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Cert keys.DeviceCert `json:"cert"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.AddDevice(*c, req.Cert); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) device(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	cert, revoked, err := a.svc.DeviceCertByID(*c, r.PathValue("device"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"cert": cert, "revoked": revoked})
}

func (a *Server) personDevices(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	certs, err := a.svc.Devices(*c, r.PathValue("person"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"certs": certs})
}

// --- spaces ------------------------------------------------------------------------------------------------

func (a *Server) spaces(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	v, err := a.svc.Spaces(*c)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"spaces": v})
}

// account is the caller's own account: whether they may start a family (an allowance only the operator gives).
func (a *Server) account(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	v, err := a.svc.Account(*c)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, v)
}

func (a *Server) space(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	v, err := a.svc.SpaceDetail(*c, r.PathValue("space"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, v)
}

func (a *Server) createSpace(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		SpaceID string            `json:"space_id"`
		Intent  *audit.Intent     `json:"intent"`
		Grants  []keys.EpochGrant `json:"grants"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.CreateSpace(*c, req.SpaceID, req.Intent, req.Grants); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]string{"space_id": req.SpaceID})
}

// conversation makes a private conversation inside a family (see svc/conversations.go).
func (a *Server) conversation(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		SpaceID      string            `json:"space_id"`
		Participants []string          `json:"participants"`
		Intent       *audit.Intent     `json:"intent"`
		Grants       []keys.EpochGrant `json:"grants"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.CreateConversation(*c, r.PathValue("space"), req.SpaceID, req.Participants, req.Intent, req.Grants); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]string{"space_id": req.SpaceID})
}

func (a *Server) permissions(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Perms  svc.Perms     `json:"perms"`
		Intent *audit.Intent `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.SetPermissions(*c, r.PathValue("space"), r.PathValue("person"), req.Perms, req.Intent); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) invite(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Role        string        `json:"role"`
		NotAfter    time.Time     `json:"not_after"`
		HistoryFrom time.Time     `json:"history_from"`
		Perms       *svc.Perms    `json:"perms"`
		Intent      *audit.Intent `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	code, exp, err := a.svc.SpaceInvite(*c, r.PathValue("space"), req.Role, req.NotAfter, req.HistoryFrom, req.Perms, req.Intent)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"code": code, "expires": exp})
}

func (a *Server) settings(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		ChildrenChat bool          `json:"children_chat"`
		Intent       *audit.Intent `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.SetChildrenChat(*c, r.PathValue("space"), req.ChildrenChat, req.Intent); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) join(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Invite string        `json:"invite"`
		Intent *audit.Intent `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := a.svc.JoinExisting(*c, req.Invite, req.Intent)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, res)
}

func (a *Server) removeMember(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Intent *audit.Intent `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.RemoveMember(*c, r.PathValue("space"), r.PathValue("person"), req.Intent); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (a *Server) myGrants(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	g, err := a.svc.MyGrants(*c, r.PathValue("space"))
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"grants": g})
}

func (a *Server) shareKeys(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Epoch  uint64            `json:"epoch"`
		Grants []keys.EpochGrant `json:"grants"`
		Intent *audit.Intent     `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.ShareKeys(*c, r.PathValue("space"), req.Epoch, req.Grants, req.Intent); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

// --- items and audit ---------------------------------------------------------------------------------------

func (a *Server) items(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, err := a.svc.Items(*c, r.PathValue("space"), after, limit)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"items": v})
}

func (a *Server) postItem(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Item   *envelope.Item `json:"item"`
		Intent *audit.Intent  `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Item == nil || req.Item.SpaceID != r.PathValue("space") {
		fail(w, svc.ErrInvalid)
		return
	}
	rec, e, err := a.svc.PostItem(*c, req.Item, req.Intent)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"seq": rec.Seq, "audit": e})
}

func (a *Server) record(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	var req struct {
		Intent *audit.Intent `json:"intent"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Intent == nil || req.Intent.SpaceID != r.PathValue("space") {
		fail(w, svc.ErrInvalid)
		return
	}
	e, err := a.svc.Record(*c, req.Intent)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"audit": e})
}

func (a *Server) audit(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	from, _ := strconv.ParseUint(r.URL.Query().Get("from"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, err := a.svc.Audit(*c, r.PathValue("space"), from, limit)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"entries": v})
}

// --- encrypted files ---------------------------------------------------------------------------------------

// The audit intent for an upload or download travels in a header (base64 JSON), so the body can be streamed.
func intentHeader(r *http.Request) (*audit.Intent, error) {
	raw, err := base64.StdEncoding.DecodeString(r.Header.Get("X-PP-Intent"))
	if err != nil || len(raw) == 0 || len(raw) > 32<<10 {
		return nil, svc.ErrInvalid
	}
	var in audit.Intent
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, svc.ErrInvalid
	}
	return &in, nil
}

func (a *Server) putBlob(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	id, space := r.PathValue("blob"), r.PathValue("space")
	if wire.CheckID(id) != nil {
		fail(w, svc.ErrInvalid)
		return
	}
	in, err := intentHeader(r)
	if err != nil {
		fail(w, err)
		return
	}
	tmpDir := filepath.Join(a.dataDir, "tmp")
	_ = os.MkdirAll(tmpDir, 0o700)
	f, err := os.CreateTemp(tmpDir, "up-*")
	if err != nil {
		fail(w, err)
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op once it has been moved into the store
	const maxBlob = 2<<30 + 4<<20
	n, err := io.Copy(f, http.MaxBytesReader(w, r.Body, maxBlob))
	cerr := f.Close()
	if err != nil || cerr != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			fail(w, svc.ErrInvalid)
			return
		}
		fail(w, svc.ErrInvalid)
		return
	}
	if err := a.svc.StoreBlob(*c, a.dataDir, space, id, tmp, n, in); err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"size": n})
}

func (a *Server) getBlob(w http.ResponseWriter, r *http.Request, c *svc.Caller) {
	in, err := intentHeader(r)
	if err != nil {
		fail(w, err)
		return
	}
	f, size, err := a.svc.OpenBlob(*c, a.dataDir, r.PathValue("blob"), in)
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	_, _ = io.Copy(w, f)
}
