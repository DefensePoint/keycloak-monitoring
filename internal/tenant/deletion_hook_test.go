package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// Deleting a tenant purges its rows, so the deletion hook must read what it
// needs before the purge and act only once the deletion has committed.

type recordingHook struct {
	steps *[]string
}

func (h recordingHook) PrepareDeletion(_ context.Context, t *domain.KeycloakTenant) func(context.Context) {
	*h.steps = append(*h.steps, "prepare "+t.TenantID)
	return func(context.Context) { *h.steps = append(*h.steps, "after "+t.TenantID) }
}

func hookedService(steps *[]string, deleteErr error, configDefined bool) Service {
	repo := &mockRepository{
		getByTenantIDFn: func(_ context.Context, id string) (*domain.KeycloakTenant, error) {
			return &domain.KeycloakTenant{TenantID: id, Name: "Tenant", IsConfigDefined: configDefined}, nil
		},
		deleteFn: func(_ context.Context, id string) error {
			*steps = append(*steps, "delete "+id)
			return deleteErr
		},
	}
	return NewService(repo, WithDeletionHook(recordingHook{steps: steps}))
}

func TestDeleteTenant_HookPreparesBeforeThePurgeAndActsAfterIt(t *testing.T) {
	var steps []string
	if err := hookedService(&steps, nil, false).DeleteTenant(context.Background(), "t1"); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}
	want := []string{"prepare t1", "delete t1", "after t1"}
	if len(steps) != len(want) {
		t.Fatalf("steps = %v, want %v", steps, want)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("steps = %v, want %v", steps, want)
		}
	}
}

func TestDeleteTenant_AFailedDeletionSkipsTheHooksFollowUp(t *testing.T) {
	var steps []string
	if err := hookedService(&steps, errors.New("db down"), false).DeleteTenant(context.Background(), "t1"); err == nil {
		t.Fatal("DeleteTenant reported success although the delete failed")
	}
	for _, s := range steps {
		if s == "after t1" {
			t.Fatalf("steps = %v: the follow-up ran for a deletion that failed", steps)
		}
	}
}

func TestDeleteTenant_AConfigDefinedTenantNeverReachesTheHook(t *testing.T) {
	var steps []string
	if err := hookedService(&steps, nil, true).DeleteTenant(context.Background(), "t1"); !errors.Is(err, ErrConfigDefinedTenantReadOnly) {
		t.Fatalf("DeleteTenant = %v, want ErrConfigDefinedTenantReadOnly", err)
	}
	if len(steps) != 0 {
		t.Errorf("steps = %v, want none for a tenant that cannot be deleted", steps)
	}
}
