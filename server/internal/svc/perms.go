package svc

import (
	"fmt"
	"slices"

	"pp/core/envelope"
)

// Sections of a family's record, for what someone may see and add to.
const (
	SecMessages  = "messages"  // the family chat (messages, reactions, call records)
	SecCalendar  = "calendar"  // events, schedules, change requests, check-ins
	SecExpenses  = "expenses"  //
	SecDocuments = "documents" //
	SecInfo      = "info"      // the info bank
)

// AllSections is every section, in the order the apps show them.
var AllSections = []string{SecMessages, SecCalendar, SecExpenses, SecDocuments, SecInfo}

// Who a member may contact privately (messages) or call.
const (
	ReachAnyone  = "anyone"  // anyone in the family
	ReachParents = "parents" // the parents only
	ReachChosen  = "chosen"  // the people listed
	ReachNobody  = "nobody"
)

// Perms are what a member may do in a family, set by the parent who invited them (and changeable by that parent
// later; every change is in the audit log). Parents always have everything. The server enforces all of it except
// Video, which it can't see (a call's kind is end-to-end encrypted) - the apps enforce that.
type Perms struct {
	See         []string `json:"see"`                    // sections whose items they receive
	Add         []string `json:"add"`                    // sections they may add to (and answer in)
	Message     string   `json:"message"`                // private conversations: anyone | parents | chosen | nobody
	MessageWith []string `json:"message_with,omitempty"` // person IDs, for "chosen"
	Call        string   `json:"call"`                   // calls: anyone | parents | chosen | nobody
	CallWith    []string `json:"call_with,omitempty"`    // person IDs, for "chosen"
	Video       bool     `json:"video"`                  // video as well as voice
	FamilyCall  bool     `json:"family_call"`            // may ring the whole family at once
}

// DefaultPerms are a role's permissions when the inviting parent doesn't change anything.
func DefaultPerms(role string) Perms {
	switch role {
	case RoleParent:
		return Perms{See: AllSections, Add: AllSections, Message: ReachAnyone, Call: ReachAnyone, Video: true, FamilyCall: true}
	case RoleFamily:
		return Perms{See: []string{SecMessages, SecCalendar, SecDocuments, SecInfo}, Add: []string{SecMessages, SecCalendar, SecDocuments, SecInfo},
			Message: ReachAnyone, Call: ReachAnyone, Video: true}
	case RoleChild:
		return Perms{See: []string{SecMessages, SecCalendar}, Message: ReachParents, Call: ReachParents, Video: true}
	case RoleProfessional:
		return Perms{See: AllSections, Message: ReachNobody, Call: ReachNobody}
	}
	return Perms{Message: ReachNobody, Call: ReachNobody}
}

// check normalises and validates permissions for a role (parents can't be limited).
func (p *Perms) check(role string) error {
	if role == RoleParent {
		*p = DefaultPerms(RoleParent)
		return nil
	}
	valid := func(list []string) bool {
		for _, s := range list {
			if !slices.Contains(AllSections, s) {
				return false
			}
		}
		return true
	}
	reach := func(r string) bool {
		return r == ReachAnyone || r == ReachParents || r == ReachChosen || r == ReachNobody
	}
	if !valid(p.See) || !valid(p.Add) || !reach(p.Message) || !reach(p.Call) || len(p.MessageWith) > 50 || len(p.CallWith) > 50 {
		return fmt.Errorf("%w: unknown permission", ErrInvalid)
	}
	for _, s := range p.Add { // adding to a section you can't see makes no sense
		if !slices.Contains(p.See, s) {
			p.See = append(p.See, s)
		}
	}
	if role == RoleProfessional && len(p.Add) > 0 {
		return fmt.Errorf("%w: professionals have read-only access", ErrInvalid)
	}
	if role != RoleParent {
		for _, s := range p.Add {
			if s == SecExpenses {
				return fmt.Errorf("%w: only parents add expenses", ErrInvalid)
			}
		}
	}
	return nil
}

// permsOf is a member's effective permissions (stored ones, or the role's defaults for memberships from earlier
// test builds). Children who may chat (the family's ChildrenChat switch) can also add to the family chat.
func permsOf(m *Member, sp *Space) Perms {
	p := DefaultPerms(m.Role)
	if m.Perms != nil && m.Role != RoleParent {
		p = *m.Perms
	}
	if m.Role == RoleChild && sp != nil && sp.ChildrenChat && !slices.Contains(p.Add, SecMessages) {
		p.Add = append(p.Add, SecMessages)
	}
	return p
}

// sectionOf is the section an item kind belongs to ("" = always visible: names, conversation records).
func sectionOf(kind string) string {
	switch kind {
	case envelope.KindMessage, envelope.KindReaction, envelope.KindCall:
		return SecMessages
	case envelope.KindEvent, envelope.KindSchedule, envelope.KindSwap, envelope.KindCheckIn:
		return SecCalendar
	case envelope.KindExpense:
		return SecExpenses
	case envelope.KindFile:
		return SecDocuments
	case envelope.KindInfo:
		return SecInfo
	}
	return ""
}

// parentsOnly kinds: proposals and money are between parents.
func parentsOnly(kind string) bool {
	return kind == envelope.KindExpense || kind == envelope.KindSchedule || kind == envelope.KindSwap
}

// reaches reports whether permissions allow contacting a person (whose family role is given).
func reaches(rule string, with []string, person, role string) bool {
	switch rule {
	case ReachAnyone:
		return true
	case ReachParents:
		return role == RoleParent
	case ReachChosen:
		return slices.Contains(with, person)
	}
	return false
}
