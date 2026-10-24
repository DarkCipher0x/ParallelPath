package app

import (
	"encoding/json"
	"fmt"
	"time"

	"pp/core/audit"
	"pp/core/envelope"
	"pp/core/keys"
	"pp/core/pqc"
	"pp/engine/vault"
)

// Mirrors of the server's view types (only the fields the client uses).
type member struct {
	Person   string    `json:"person"`
	Role     string    `json:"role"`
	Status   string    `json:"status"`
	Joined   time.Time `json:"joined"`
	ReadOnly bool      `json:"read_only"`
	NotAfter time.Time `json:"not_after"`
	HasKeys  bool      `json:"has_keys"`
	// HistoryFrom: a professional sees items first created from then on (zero = everything).
	HistoryFrom time.Time `json:"history_from"`
	Perms       *Perms    `json:"perms"` // family spaces: their effective permissions
	InvitedBy   string    `json:"invited_by"`
}

type spaceView struct {
	Space struct {
		ID           string    `json:"id"`
		Created      time.Time `json:"created"`
		Epoch        uint64    `json:"epoch"`
		ChildrenChat bool      `json:"children_chat"`
		Parent       string    `json:"parent"` // a private conversation's family
	} `json:"space"`
	Members   []member `json:"members"`
	Me        member   `json:"me"`
	RotateDue bool     `json:"rotate_due"` // parents: the epoch key is due to be replaced (see Rotate)
	// KeyedDevices (parents) hold the current epoch: a rotation re-keys exactly these.
	KeyedDevices []string `json:"keyed_devices"`
	// FamilyKeyed (conversations): participants' devices approved in the family (see admitDevices).
	FamilyKeyed []string `json:"family_keyed"`
}

type itemRec struct {
	Seq  uint64        `json:"seq"`
	Item envelope.Item `json:"item"`
}

// SyncAll refreshes the list of spaces and syncs each one.
func (e *Engine) SyncAll() {
	var res struct {
		Spaces []spaceView `json:"spaces"`
	}
	if err := e.call("GET", "/api/v1/spaces", nil, &res); err != nil {
		e.listen("error", map[string]string{"message": err.Error()})
		return
	}
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return
	}
	_ = v.Put(vault.State, "spaces", res.Spaces)
	for _, s := range res.Spaces {
		e.syncSpace(s.Space.ID)
	}
	e.listen("changed", map[string]string{"what": "spaces"})
}

// syncSpace pulls new keys, items and audit entries for one space and verifies all of them.
func (e *Engine) syncSpace(space string) {
	e.mu.Lock()
	if e.v == nil || e.syncing[space] {
		e.mu.Unlock()
		return
	}
	e.syncing[space] = true
	v := e.v
	e.mu.Unlock()
	defer func() { e.mu.Lock(); delete(e.syncing, space); e.mu.Unlock() }()

	if err := e.syncSpaceView(v, space); err != nil {
		e.report(err)
		return
	}
	newKeys, err := e.syncGrants(v, space)
	if err != nil {
		e.report(err)
	}
	var cursor uint64
	firstSync := v.Get(vault.State, "cursor/"+space, &cursor) != nil || cursor == 0
	fresh, err := e.syncItems(v, space)
	if err != nil {
		e.report(err)
	}
	newItems := len(fresh) > 0
	newAudit, err := e.syncAudit(v, space)
	if err != nil {
		e.report(err)
	}
	e.postPendingProfile(v, space)
	e.carryNamesForWindows(v, space)
	e.rotateIfDue(v, space)
	e.admitDevices(v, space)
	if newKeys || newItems || newAudit {
		e.listen("changed", map[string]string{"what": "space", "space_id": space})
	}
	if !firstSync {
		e.notifyNew(space, fresh)
	}
	if newItems {
		go e.checkReminders(time.Now()) // a new or changed event may be due already
	}
}

// refreshKeys fetches the space's current epoch and any new keys right now (a full sync may already be running,
// and syncSpace would then return without waiting).
func (e *Engine) refreshKeys(space string) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return
	}
	if e.syncSpaceView(v, space) == nil {
		_, _ = e.syncGrants(v, space)
	}
	e.syncSpace(space)
}

// rotateIfDue replaces the space's epoch key when the server says it's due (a month old, or held by a device that
// shouldn't have it any more). Any parent's device may do it; if two race, the second is refused and that's fine.
func (e *Engine) rotateIfDue(v *vault.Vault, space string) {
	var sv spaceView
	if v.Get(vault.State, "space/"+space, &sv) != nil || !sv.RotateDue || (sv.Me.Role != "parent" && sv.Me.Role != "participant") || sv.Me.ReadOnly {
		return
	}
	k, ok := e.epochKey(v, space, sv.Space.Epoch)
	if !ok {
		return // this device can't share the current key yet
	}
	pqc.Wipe(k)
	if err := e.Rotate(space); err == nil {
		e.listen("changed", map[string]string{"what": "space", "space_id": space})
	}
}

func (e *Engine) report(err error) {
	if err != errLocked {
		e.listen("error", map[string]string{"message": err.Error()})
	}
}

func (e *Engine) syncSpaceView(v *vault.Vault, space string) error {
	var sv spaceView
	if err := e.call("GET", "/api/v1/spaces/"+space, nil, &sv); err != nil {
		return err
	}
	// A slower concurrent sync must not put back a view from before a rotation (posts would then use an old epoch).
	var have spaceView
	if v.Get(vault.State, "space/"+space, &have) == nil && have.Space.Epoch > sv.Space.Epoch {
		return nil
	}
	// A parent shared another section with this person: the server skipped those items so far, so fetch from the
	// start again (verified as always; items already here are simply stored again).
	if have.Me.Perms != nil && sv.Me.Perms != nil && !sameSet(have.Me.Perms.See, sv.Me.Perms.See) {
		_ = v.Delete(vault.State, "cursor/"+space)
	}
	return v.Put(vault.State, "space/"+space, sv)
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, x := range a {
		seen[x] = true
	}
	for _, x := range b {
		if !seen[x] {
			return false
		}
	}
	return true
}

func (e *Engine) spaceView(space string) (*spaceView, error) {
	e.mu.Lock()
	v := e.v
	e.mu.Unlock()
	if v == nil {
		return nil, errLocked
	}
	var sv spaceView
	if err := v.Get(vault.State, "space/"+space, &sv); err != nil {
		if err := e.syncSpaceView(v, space); err != nil {
			return nil, err
		}
		if err := v.Get(vault.State, "space/"+space, &sv); err != nil {
			return nil, err
		}
	}
	return &sv, nil
}

// syncGrants unwraps any epoch keys addressed to this device (from verified member devices only).
func (e *Engine) syncGrants(v *vault.Vault, space string) (bool, error) {
	var res struct {
		Grants []keys.EpochGrant `json:"grants"`
	}
	if err := e.call("GET", "/api/v1/spaces/"+space+"/keys", nil, &res); err != nil {
		return false, err
	}
	e.mu.Lock()
	dev := e.dev
	e.mu.Unlock()
	if dev == nil {
		return false, errLocked
	}
	added := false
	for i := range res.Grants {
		g := &res.Grants[i]
		k := epochKeyName(space, g.Epoch)
		var have []byte
		if v.Get(vault.Epochs, k, &have) == nil {
			pqc.Wipe(have)
			continue
		}
		from, err := e.cert(v, g.FromDevice)
		if err != nil {
			return added, err
		}
		key, err := g.Open(dev, from)
		if err != nil {
			return added, fmt.Errorf("a key shared with this device failed verification and was ignored")
		}
		if err := v.Put(vault.Epochs, k, key); err != nil {
			return added, err
		}
		pqc.Wipe(key)
		added = true
	}
	return added, nil
}

func epochKeyName(space string, epoch uint64) string { return fmt.Sprintf("%s/%020d", space, epoch) }

// epochKey returns a stored epoch key.
func (e *Engine) epochKey(v *vault.Vault, space string, epoch uint64) ([]byte, bool) {
	var k []byte
	if v.Get(vault.Epochs, epochKeyName(space, epoch), &k) != nil {
		return nil, false
	}
	return k, true
}

// syncItems fetches new item revisions, verifies each author signature and caches them (still encrypted). It
// returns the ones it added.
func (e *Engine) syncItems(v *vault.Vault, space string) ([]itemRec, error) {
	var cursor uint64
	_ = v.Get(vault.State, "cursor/"+space, &cursor)
	var added []itemRec
	for {
		var res struct {
			Items []itemRec `json:"items"`
		}
		if err := e.call("GET", fmt.Sprintf("/api/v1/spaces/%s/items?after=%d&limit=500", space, cursor), nil, &res); err != nil {
			return added, err
		}
		for i := range res.Items {
			r := &res.Items[i]
			author, err := e.cert(v, r.Item.Device)
			if err != nil || envelope.VerifySignature(&r.Item, author) != nil {
				e.listen("error", map[string]string{"message": "An item failed its signature check and was not shown. This can mean the server was tampered with."})
			} else if err := v.Put(vault.Items, itemKey(space, r.Seq), r); err != nil {
				return added, err
			} else {
				added = append(added, *r)
			}
			cursor = r.Seq
		}
		if err := v.Put(vault.State, "cursor/"+space, cursor); err != nil {
			return added, err
		}
		if len(res.Items) < 500 {
			return added, nil
		}
	}
}

func itemKey(space string, seq uint64) string { return fmt.Sprintf("%s/%020d", space, seq) }

// syncAudit fetches new audit entries and verifies them against the last verified head.
func (e *Engine) syncAudit(v *vault.Vault, space string) (bool, error) {
	var head *audit.Head
	var h audit.Head
	if v.Get(vault.State, "audit-head/"+space, &h) == nil {
		head = &h
	}
	from := uint64(1)
	if head != nil {
		from = head.Seq
	}
	added := false
	for {
		var res struct {
			Entries []audit.Entry `json:"entries"`
		}
		if err := e.call("GET", fmt.Sprintf("/api/v1/spaces/%s/audit?from=%d&limit=1000", space, from), nil, &res); err != nil {
			return added, err
		}
		if head != nil && len(res.Entries) == 1 && res.Entries[0].Seq == head.Seq {
			// Only the entry we already had: still check it's identical (proves no rewrite).
			if _, err := audit.VerifyFrom(space, head, res.Entries, e.serverKey(), e.deviceKeys(v)); err != nil {
				return added, e.auditProblem(v, space, err)
			}
			return added, nil
		}
		newHead, err := audit.VerifyFrom(space, head, res.Entries, e.serverKey(), e.deviceKeys(v))
		if err != nil {
			return added, e.auditProblem(v, space, err)
		}
		for i := range res.Entries {
			en := &res.Entries[i]
			if head != nil && en.Seq == head.Seq {
				continue
			}
			if err := v.Put(vault.State, fmt.Sprintf("audit/%s/%020d", space, en.Seq), en); err != nil {
				return added, err
			}
			added = true
		}
		head = newHead
		if err := v.Put(vault.State, "audit-head/"+space, head); err != nil {
			return added, err
		}
		_ = v.Delete(vault.State, "audit-problem/"+space)
		if len(res.Entries) < 1000 {
			return added, nil
		}
		from = head.Seq
	}
}

func (e *Engine) auditProblem(v *vault.Vault, space string, err error) error {
	_ = v.Put(vault.State, "audit-problem/"+space, err.Error())
	return fmt.Errorf("the family's audit log failed verification: %v", err)
}

// deviceKeys resolves device IDs to verified signing keys for audit verification.
func (e *Engine) deviceKeys(v *vault.Vault) audit.DeviceKeys {
	return func(id string) (pqc.VerifyKey, bool) {
		c, err := e.cert(v, id)
		if err != nil {
			return pqc.VerifyKey{}, false
		}
		return c.Sign, true
	}
}

// cert returns a verified device certificate, from the vault cache or the server. A certificate is cached only
// after its identity signature checks out, and a device ID can never be re-bound to different keys.
func (e *Engine) cert(v *vault.Vault, deviceID string) (*keys.VerifiedDevice, error) {
	var c keys.DeviceCert
	if v.Get(vault.Certs, deviceID, &c) == nil {
		return c.Verify()
	}
	var res struct {
		Cert *keys.DeviceCert `json:"cert"`
	}
	if err := e.call("GET", "/api/v1/devices/"+deviceID, nil, &res); err != nil {
		return nil, err
	}
	if res.Cert == nil {
		return nil, fmt.Errorf("unknown device")
	}
	vd, err := res.Cert.Verify()
	if err != nil || vd.DeviceID != deviceID {
		return nil, fmt.Errorf("a device certificate from the server failed verification")
	}
	// Pin the person's identity key: a person ID can only ever map to one identity key (it IS its fingerprint).
	_ = v.Put(vault.Certs, deviceID, res.Cert)
	return vd, nil
}

// postPendingProfile shares this person's name (and, for a new space, the family's name) once keys are available.
func (e *Engine) postPendingProfile(v *vault.Vault, space string) {
	var name, family string
	hasName := v.Get(vault.State, "pending-profile/"+space, &name) == nil
	hasFamily := v.Get(vault.State, "pending-spacename/"+space, &family) == nil
	if !hasName && !hasFamily {
		return
	}
	sv, err := e.spaceView(space)
	if err != nil {
		return
	}
	k, ok := e.epochKey(v, space, sv.Space.Epoch)
	if !ok {
		return
	}
	pqc.Wipe(k)
	if hasFamily && e.RenameSpace(space, family) == nil {
		_ = v.Delete(vault.State, "pending-spacename/"+space)
	}
	if hasName && name != "" && e.SetMyName(space, name) == nil {
		_ = v.Delete(vault.State, "pending-profile/"+space)
	}
}

func unmarshal[T any](raw []byte) (T, error) {
	var t T
	err := json.Unmarshal(raw, &t)
	return t, err
}
