package svc

import (
	"fmt"

	"pp/server/internal/store"
)

// Bringing a new family onto the server is the operator's decision, never a member's: a server invite (admin only)
// lets its person start one family, and the admin portal can let a person start more. Parents invite people into
// their own family; they can't start other families.

// familiesLeft is how many families p may still start. Accounts from earlier test builds have no allowance
// recorded: they keep the one family a server invite gave them only if they're in no family yet.
func (s *Service) familiesLeft(tx *store.Tx, p *Person) int {
	if p.FamiliesLeft != nil {
		return *p.FamiliesLeft
	}
	inFamily := false
	tx.Keys(store.PersonSpaces, store.Key(p.ID, ""), func(k []byte) bool {
		var sp Space
		if tx.Get(store.Spaces, k[len(p.ID)+1:], &sp) == nil && sp.Parent == "" && !sp.Journal && !s.soleMember(tx, &sp, p.ID) {
			inFamily = true
			return false
		}
		return true
	})
	if inFamily {
		return 0
	}
	return 1
}

// soleMember reports whether sp was started by person and nobody else has ever been in it (an old, unmarked journal
// looks like this - so does a family nobody has joined yet, which then can't be told apart and isn't counted).
func (s *Service) soleMember(tx *store.Tx, sp *Space, person string) bool {
	if sp.CreatedBy != person {
		return false
	}
	others := false
	tx.Keys(store.Members, store.Key(sp.ID, ""), func(k []byte) bool {
		if string(k[len(sp.ID)+1:]) != person {
			others = true
			return false
		}
		return true
	})
	return !others
}

// hasJournal reports whether person already has a (marked) journal.
func (s *Service) hasJournal(tx *store.Tx, person string) bool {
	found := false
	tx.Keys(store.PersonSpaces, store.Key(person, ""), func(k []byte) bool {
		var sp Space
		if tx.Get(store.Spaces, k[len(person)+1:], &sp) == nil && sp.Journal && sp.CreatedBy == person {
			found = true
			return false
		}
		return true
	})
	return found
}

// Account is what a signed-in person may know about their own account.
type Account struct {
	Person         string `json:"person"`
	FamiliesLeft   int    `json:"families_left"`
	CanStartFamily bool   `json:"can_start_family"`
}

// Account returns the caller's own account details.
func (s *Service) Account(c Caller) (*Account, error) {
	var a *Account
	err := s.db.View(func(tx *store.Tx) error {
		var p Person
		if err := tx.Get(store.Persons, []byte(c.Person), &p); err != nil {
			return ErrUnauthorized
		}
		left := s.familiesLeft(tx, &p)
		a = &Account{Person: p.ID, FamiliesLeft: left, CanStartFamily: left > 0}
		return nil
	})
	return a, err
}

// AdminSetFamiliesLeft sets how many more families a person may start (0 to 20), recorded in the server's log.
func (s *Service) AdminSetFamiliesLeft(person string, n int, by string) error {
	if n < 0 || n > 20 {
		return fmt.Errorf("%w: 0 to 20 families", ErrInvalid)
	}
	err := s.db.Update(func(tx *store.Tx) error {
		var p Person
		if err := tx.Get(store.Persons, []byte(person), &p); err != nil {
			return ErrNotFound
		}
		p.FamiliesLeft = &n
		return tx.Put(store.Persons, []byte(person), p)
	})
	if err != nil {
		return err
	}
	return s.AdminAudit("person.families", fmt.Sprintf("%s may start %d more famil%s (by %s)", person, n, map[bool]string{true: "y", false: "ies"}[n == 1], by))
}
