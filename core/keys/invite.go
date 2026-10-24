package keys

import (
	"errors"
	"strings"

	"pp/core/wire"
)

// Invite codes. The secret part is 32 random bytes (base64url); the server only stores a keyed hash of it.
//
//	S.<secret>                       join the server and start your own family space (from the operator)
//	J.<space id>.<role>.<secret>     join a family space with a role (from a parent)
//
// A space code carries its space and role in the clear because the person joining must sign "I joined space X as
// role Y" before the server accepts them; the server checks those match what the code was issued for.
type InviteCode struct {
	Kind   byte // 'S' or 'J'
	Space  string
	Role   string
	Secret string
}

// ErrBadInvite means a code is malformed.
var ErrBadInvite = errors.New("that invite code isn't valid - check it was copied completely")

// String renders the code.
func (c InviteCode) String() string {
	if c.Kind == 'J' {
		return "J." + c.Space + "." + c.Role + "." + c.Secret
	}
	return "S." + c.Secret
}

// ParseInvite reads a code typed, pasted or scanned by a person (surrounding spaces are ignored).
func ParseInvite(s string) (InviteCode, error) {
	p := strings.Split(strings.TrimSpace(s), ".")
	switch {
	case len(p) == 2 && p[0] == "S" && goodSecret(p[1]):
		return InviteCode{Kind: 'S', Secret: p[1]}, nil
	case len(p) == 4 && p[0] == "J" && wire.CheckID(p[1]) == nil && goodRole(p[2]) && goodSecret(p[3]):
		return InviteCode{Kind: 'J', Space: p[1], Role: p[2], Secret: p[3]}, nil
	}
	return InviteCode{}, ErrBadInvite
}

func goodSecret(s string) bool {
	if len(s) != 43 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func goodRole(r string) bool {
	switch r {
	case "parent", "family", "professional", "child":
		return true
	}
	return false
}
