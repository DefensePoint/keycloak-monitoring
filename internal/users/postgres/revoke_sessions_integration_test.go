// A logout moves the user's session epoch on, so every server instance and a
// restarted one reject the sessions it ended.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with an upd- subject are touched.
package postgres

import (
	"context"
	"sync"
	"testing"
)

func currentEpoch(t *testing.T, r *Repository, subject string) int64 {
	t.Helper()
	e, err := r.CurrentSessionEpoch(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRevokeSessions_MovesTheEpochOn(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	ctx := context.Background()

	if e := currentEpoch(t, r, u.Subject); e != 0 {
		t.Fatalf("new user epoch = %d, want 0", e)
	}
	moved, err := r.RevokeSessions(ctx, u.Subject, 0)
	if err != nil || !moved {
		t.Fatalf("first logout: moved=%v err=%v", moved, err)
	}
	if e := currentEpoch(t, r, u.Subject); e != 1 {
		t.Fatalf("epoch = %d after a logout, want 1", e)
	}
	got, err := r.GetBySubject(ctx, u.Subject)
	if err != nil || got.SessionEpoch != 1 {
		t.Fatalf("GetBySubject epoch = %v (err %v), want 1", got, err)
	}
}

// An old cookie, already rejected, must not be able to end the user's newer
// sessions by being replayed at logout.
func TestRevokeSessions_AnAlreadyRevokedSessionCannotMoveIt(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	ctx := context.Background()

	if _, err := r.RevokeSessions(ctx, u.Subject, 0); err != nil {
		t.Fatal(err)
	}
	moved, err := r.RevokeSessions(ctx, u.Subject, 0)
	if err != nil {
		t.Fatal(err)
	}
	if moved {
		t.Fatal("a stale session moved the epoch")
	}
	if e := currentEpoch(t, r, u.Subject); e != 1 {
		t.Fatalf("epoch = %d, want it unchanged at 1", e)
	}
}

// Two devices logging out at once with the same epoch move it once.
func TestRevokeSessions_ConcurrentLogoutsMoveItOnce(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	moves := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			moved, err := r.RevokeSessions(ctx, u.Subject, 0)
			if err != nil {
				t.Error(err)
				return
			}
			if moved {
				mu.Lock()
				moves++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if moves != 1 {
		t.Fatalf("%d logouts moved the epoch, want 1", moves)
	}
	if e := currentEpoch(t, r, u.Subject); e != 1 {
		t.Fatalf("epoch = %d, want 1", e)
	}
}

// Editing the user must not reset the epoch and so revive ended sessions.
func TestRevokeSessions_SurvivesAnUnrelatedEdit(t *testing.T) {
	r, db := newUpdateFlagsRepo(t)
	u := seedUser(t, db, true, false)
	ctx := context.Background()

	if _, err := r.RevokeSessions(ctx, u.Subject, 0); err != nil {
		t.Fatal(err)
	}
	u.Name = "Edited after logout"
	u.SessionEpoch = 0 // a copy loaded before the logout
	if err := r.Update(ctx, u); err != nil {
		t.Fatal(err)
	}
	if e := currentEpoch(t, r, u.Subject); e != 1 {
		t.Fatalf("epoch = %d after an edit, want 1", e)
	}
}
