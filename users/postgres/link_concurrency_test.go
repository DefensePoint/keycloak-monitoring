package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Adoption reads an account and then writes a new subject onto it, with nothing
// holding the row in between. Two logins arriving together therefore interleave,
// and these tests state what must survive that regardless of the order the
// database happens to choose.
//
// The invariants matter more than the individual outcomes: whatever happens,
// one person must not end up as two accounts, and nobody may be handed an
// account that belongs to a different subject.
//
// What these found, across 25 repetitions under -race: the invariants hold and
// no login was ever refused, so the window between the read and the write was
// never actually observed to open. That is evidence, not proof — two logins
// with different subjects and the same verified address need two realms or a
// provider that does not keep addresses unique, and the window is narrow
// because each write is its own transaction and the second one waits on the
// first.
//
// Should it ever open, the design already fails the safe way. The reload after
// the update looks the account up by the subject this login presented, so a
// login whose subject was overwritten in between finds nothing and errors
// instead of succeeding as somebody else. Widening the window deliberately to
// prove that would mean putting a delay into the code these tests exist to
// protect, which costs more than it settles.

// concurrentSignIns runs the given logins at once and returns each result.
func concurrentSignIns(r *Repository, logins []struct{ subject, email string }) ([]*domain.User, []error) {
	users := make([]*domain.User, len(logins))
	errs := make([]error, len(logins))

	var ready sync.WaitGroup
	var done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(len(logins))
	done.Add(len(logins))

	for i, l := range logins {
		go func(i int, subject, email string) {
			defer done.Done()
			ready.Done()
			<-start // line them up so the reads genuinely overlap
			users[i], errs[i] = r.FindOrCreateBySubject(context.Background(), &domain.User{
				Subject: subject, Email: email, EmailVerified: true,
				AuthMethod: domain.AuthMethodOAuth, IsActive: true,
			})
		}(i, l.subject, l.email)
	}

	ready.Wait()
	close(start)
	done.Wait()
	return users, errs
}

// The realistic case: one person, one new subject, two tabs. Both logins carry
// the same claims, so no interleaving can make them disagree about who is
// signing in.
func TestLinkConcurrency_TheSamePersonTwiceIsStillOneAccount(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	first, err := signInAs(r, "del-conc-old", "del-conc@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}

	logins := make([]struct{ subject, email string }, 4)
	for i := range logins {
		logins[i] = struct{ subject, email string }{"del-conc-new", "del-conc@example.invalid"}
	}
	users, errs := concurrentSignIns(r, logins)

	for i, err := range errs {
		if err != nil {
			t.Errorf("login %d failed: %v", i, err)
		} else if users[i].ID != first.ID {
			t.Errorf("login %d was given account %d instead of %d", i, users[i].ID, first.ID)
		}
	}

	var n int64
	db.Model(&database.User{}).Where("email = ?", "del-conc@example.invalid").Count(&n)
	if n != 1 {
		t.Errorf("one person became %d accounts", n)
	}
}

// Two different subjects sharing one verified address, arriving together. Rarer
// — Keycloak keeps addresses unique within a realm, so this needs two realms or
// a provider that does not — but it is the interleaving with something at stake,
// because the two logins disagree about who they are.
func TestLinkConcurrency_TwoSubjectsOneAddressStaysConsistent(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	first, err := signInAs(r, "del-race-old", "del-race@example.invalid", true)
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}

	users, errs := concurrentSignIns(r, []struct{ subject, email string }{
		{"del-race-a", "del-race@example.invalid"},
		{"del-race-b", "del-race@example.invalid"},
	})

	// Nobody may be handed an account whose subject is not the one they
	// presented. A login that fails is disappointing; a login that succeeds as
	// somebody else is not acceptable.
	for i, u := range users {
		if errs[i] != nil {
			t.Logf("login %d was refused: %v", i, errs[i])
			continue
		}
		want := []string{"del-race-a", "del-race-b"}[i]
		if u.Subject != want {
			t.Errorf("login %d presented subject %q and was given an account holding %q",
				i, want, u.Subject)
		}
		if u.ID != first.ID {
			t.Errorf("login %d adopted account %d rather than the existing %d", i, u.ID, first.ID)
		}
	}

	// However they interleaved, the address must still belong to one account.
	var n int64
	db.Model(&database.User{}).Where("email = ?", "del-race@example.invalid").Count(&n)
	if n != 1 {
		t.Errorf("the address ended up on %d accounts", n)
	}
}

// Several first-time logins at once, no existing account to adopt. Whoever wins
// the create, the rest must not produce duplicates.
func TestLinkConcurrency_SimultaneousFirstLoginsMakeOneAccount(t *testing.T) {
	r, db := newDeletedAccountRepo(t)

	logins := make([]struct{ subject, email string }, 4)
	for i := range logins {
		logins[i] = struct{ subject, email string }{"del-first-sub", "del-first@example.invalid"}
	}
	users, errs := concurrentSignIns(r, logins)

	succeeded := 0
	for i, err := range errs {
		if err != nil {
			t.Logf("login %d was refused: %v", i, err)
			continue
		}
		succeeded++
		if users[i].Subject != "del-first-sub" {
			t.Errorf("login %d was given an account holding %q", i, users[i].Subject)
		}
	}
	if succeeded == 0 {
		t.Error("every simultaneous first login failed; one of them has to win")
	}

	var n int64
	db.Model(&database.User{}).Where("email = ?", "del-first@example.invalid").Count(&n)
	if n != 1 {
		t.Errorf("simultaneous first logins produced %d accounts", n)
	}

}
