// Package svc holds the server's rules: who may do what, in which order, and what gets recorded. It knows nothing
// about HTTP (internal/api translates requests into calls here) so the rules can be tested directly.
//
// The server never sees family content: items arrive end-to-end encrypted. What it does enforce:
//   - every item and every audit intent carries a valid post-quantum signature from a registered device of a member
//   - revisions follow the previous revision exactly, by the same author
//   - roles: only parents invite or remove people; professionals and children are read-only (children may chat if
//     the parents allow it)
//   - every accepted action is appended to the space's audit chain and countersigned
package svc

import (
	"time"

	"pp/core/envelope"
	"pp/core/keys"
)

// Roles in a family space.
const (
	RoleParent       = "parent"
	RoleFamily       = "family"       // grandparents, step-parents...
	RoleProfessional = "professional" // mediator, solicitor, guardian: read-only, time-limited
	RoleChild        = "child"
	// RoleParticipant is someone in a private conversation (a conversation space inside a family). Their family
	// permissions decide who they may talk to; inside the conversation every participant may write.
	RoleParticipant = "participant"
)

// ValidRole reports whether r is a known role.
func ValidRole(r string) bool {
	switch r {
	case RoleParent, RoleFamily, RoleProfessional, RoleChild:
		return true
	}
	return false
}

// CanWrite reports whether a role may add content.
func CanWrite(r string) bool { return r == RoleParent || r == RoleFamily || r == RoleParticipant }

// Person is an account. No name, email or other personal detail is stored by the server: names live in the
// family space, end-to-end encrypted.
type Person struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	Status  string    `json:"status"` // active | disabled
	// FamiliesLeft is how many family spaces this person may still start. Bringing a new family onto the server is
	// the operator's decision: a server invite gives one, the admin portal can give more. People who joined through
	// a family invite get none. Nil on accounts from earlier test builds (see familiesLeft).
	FamiliesLeft *int `json:"families_left,omitempty"`
}

// Space kinds, named in the space.create intent's detail (an empty detail is a family).
const (
	SpaceKindFamily  = ""
	SpaceKindJournal = "kind=journal"
)

// Device is a registered device of a person.
type Device struct {
	Cert    keys.DeviceCert `json:"cert"`
	Added   time.Time       `json:"added"`
	Revoked *time.Time      `json:"revoked,omitempty"`
}

// Session is a signed-in device. Only a hash of the bearer token is stored.
type Session struct {
	Person  string    `json:"person"`
	Device  string    `json:"device"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

// Challenge is a one-time sign-in nonce.
type Challenge struct {
	Device  string    `json:"device"`
	Nonce   []byte    `json:"nonce"`
	Expires time.Time `json:"expires"`
}

// Invite is a one-time code: either to join the server (and start a family space) or to join a given space.
type Invite struct {
	SpaceID   string    `json:"space_id,omitempty"` // empty = server invite (lets someone create their own space)
	Role      string    `json:"role,omitempty"`
	CreatedBy string    `json:"created_by"` // person, or "admin"
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
	ReadOnly  bool      `json:"read_only,omitempty"`
	NotAfter  time.Time `json:"not_after,omitempty"` // professionals: access ends then
	// HistoryFrom limits a professional to items created from then on (zero = the whole history).
	HistoryFrom time.Time `json:"history_from,omitempty"`
	// Perms are what the inviting parent allows (nil: the role's defaults).
	Perms *Perms `json:"perms,omitempty"`
}

// Space is a family space.
type Space struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	CreatedBy string    `json:"created_by"`
	Epoch     uint64    `json:"epoch"`
	// EpochSince is when the current epoch started (zero on spaces from earlier test builds: Created counts then).
	EpochSince time.Time `json:"epoch_since,omitempty"`
	// ChildrenChat lets members with the child role send messages and reactions (a parent's choice).
	ChildrenChat bool `json:"children_chat,omitempty"`
	// Parent is set on a private conversation: the family space it belongs to (whose members and permissions decide
	// who may be in it). Empty for a family space or a journal.
	Parent string `json:"parent,omitempty"`
	// Journal marks a person's private journal: only its creator is ever in it (unmarked on journals from earlier test builds).
	Journal bool `json:"journal,omitempty"`
}

// RotateAfter is the longest an epoch key stays current.
const RotateAfter = 30 * 24 * time.Hour

// epochStart is when the space's current epoch began.
func (sp *Space) epochStart() time.Time {
	if sp.EpochSince.IsZero() {
		return sp.Created
	}
	return sp.EpochSince
}

// Member is a person's membership of a space.
type Member struct {
	Person   string    `json:"person"`
	Role     string    `json:"role"`
	Status   string    `json:"status"` // active | removed
	Joined   time.Time `json:"joined"`
	ReadOnly bool      `json:"read_only,omitempty"`
	NotAfter time.Time `json:"not_after,omitempty"`
	Removed  time.Time `json:"removed,omitempty"`
	// HistoryFrom: professionals only see items created from then on (zero = everything).
	HistoryFrom time.Time `json:"history_from,omitempty"`
	// Perms: what this member may do (family spaces; nil = the role's defaults). InvitedBy is the parent who invited
	// them - the one who may change their permissions.
	Perms     *Perms `json:"perms,omitempty"`
	InvitedBy string `json:"invited_by,omitempty"`
	// HasKeys is computed for views (never stored meaningfully): whether any of the member's devices has been
	// given the current epoch key. False = waiting for a parent to approve them.
	HasKeys bool `json:"has_keys,omitempty"`
}

// Active reports whether the membership currently allows access.
func (m *Member) Active(now time.Time) bool {
	return m.Status == "active" && (m.NotAfter.IsZero() || now.Before(m.NotAfter))
}

// Sees reports whether the member may see something first created at t (a professional's history window).
func (m *Member) Sees(t time.Time) bool { return m.HistoryFrom.IsZero() || !t.Before(m.HistoryFrom) }

// ItemRec is a stored item revision with its per-space sequence number (the sync cursor).
type ItemRec struct {
	Seq  uint64        `json:"seq"`
	Item envelope.Item `json:"item"`
}

// Blob is the metadata of an encrypted file (the ciphertext lives on disk).
type Blob struct {
	ID       string    `json:"id"`
	SpaceID  string    `json:"space_id"`
	Uploader string    `json:"uploader"`
	Size     int64     `json:"size"`
	Created  time.Time `json:"created"`
}
