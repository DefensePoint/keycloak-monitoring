// A role's permission list replaces what the role holds, so a permission left
// out is revoked, and a list naming an unknown permission writes nothing.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset. Only rows with a roleperm- prefix are touched.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

type rolePermFixture struct {
	db     *gorm.DB
	svc    rbac.Service
	prefix string
	perms  []string // a, b, c
}

func newRolePermFixture(t *testing.T) *rolePermFixture {
	t.Helper()

	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	f := &rolePermFixture{
		db: client.DB(),
		svc: rbac.NewService(NewRoleRepository(client), NewPermissionRepository(client),
			NewUserRoleRepository(client), NewTenantPolicyRepository(client), logger.NewNoop()),
		prefix: fmt.Sprintf("roleperm-%d", time.Now().UnixNano()),
	}
	for _, action := range []string{"a", "b", "c"} {
		name := f.prefix + ":" + action
		if err := f.db.Exec(`INSERT INTO permissions (name, display_name, resource, action, created_at, updated_at)
			VALUES (?, 'test', ?, ?, NOW(), NOW())`, name, f.prefix, action).Error; err != nil {
			t.Fatalf("seed permission: %v", err)
		}
		f.perms = append(f.perms, name)
	}
	t.Cleanup(func() {
		f.db.Exec(`DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE name LIKE ?)`, f.prefix+"%")
		f.db.Exec(`DELETE FROM roles WHERE name LIKE ?`, f.prefix+"%")
		f.db.Exec(`DELETE FROM permissions WHERE name LIKE ?`, f.prefix+"%")
	})
	return f
}

func (f *rolePermFixture) createRole(t *testing.T, suffix string, perms ...string) uint {
	t.Helper()
	role, err := f.svc.CreateRoleWithPermissions(context.Background(), f.prefix+"-"+suffix, "Test", "", perms, true)
	if err != nil {
		t.Fatalf("CreateRoleWithPermissions: %v", err)
	}
	return role.ID
}

// held reads the role's permissions straight from role_permissions.
func (f *rolePermFixture) held(t *testing.T, roleID uint) []string {
	t.Helper()
	var names []string
	if err := f.db.Raw(`SELECT p.name FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = ? ORDER BY p.name`, roleID).Scan(&names).Error; err != nil {
		t.Fatalf("read role permissions: %v", err)
	}
	return names
}

func sameNames(got, want []string) bool {
	w := append([]string(nil), want...)
	sort.Strings(w)
	if len(got) != len(w) {
		return false
	}
	for i := range got {
		if got[i] != w[i] {
			return false
		}
	}
	return true
}

func TestCreateRoleWithPermissions_SavesThem(t *testing.T) {
	f := newRolePermFixture(t)
	a, b := f.perms[0], f.perms[1]

	role, err := f.svc.CreateRoleWithPermissions(context.Background(), f.prefix+"-new", "New", "d", []string{a, b, a}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.held(t, role.ID); !sameNames(got, []string{a, b}) {
		t.Fatalf("stored %v, want %v", got, []string{a, b})
	}
	if len(role.Permissions) != 2 {
		t.Fatalf("returned role lists %d permissions, want 2", len(role.Permissions))
	}
}

// A misspelt name must not leave an empty role behind.
func TestCreateRoleWithPermissions_UnknownNameCreatesNothing(t *testing.T) {
	f := newRolePermFixture(t)

	_, err := f.svc.CreateRoleWithPermissions(context.Background(), f.prefix+"-bad", "Bad", "", []string{f.perms[0], f.prefix + ":nope"}, true)
	if !errors.Is(err, rbac.ErrUnknownPermission) {
		t.Fatalf("err = %v, want ErrUnknownPermission", err)
	}
	var n int64
	f.db.Raw(`SELECT COUNT(*) FROM roles WHERE name = ?`, f.prefix+"-bad").Scan(&n)
	if n != 0 {
		t.Fatalf("%d roles created for a rejected request", n)
	}
}

func TestSetRolePermissions_ReplacesTheList(t *testing.T) {
	f := newRolePermFixture(t)
	a, b, c := f.perms[0], f.perms[1], f.perms[2]
	ctx := context.Background()
	id := f.createRole(t, "set", a, b)

	steps := []struct {
		name string
		set  []string
		want []string
	}{
		{"revoke one", []string{a}, []string{a}},
		{"swap", []string{b, c}, []string{b, c}},
		{"same list again", []string{b, c}, []string{b, c}},
		{"grant one more", []string{a, b, c}, []string{a, b, c}},
		{"empty removes all", []string{}, nil},
		{"grant from empty", []string{c}, []string{c}},
	}
	for _, s := range steps {
		if err := f.svc.SetRolePermissions(ctx, id, s.set); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := f.held(t, id); !sameNames(got, s.want) {
			t.Fatalf("%s: stored %v, want %v", s.name, got, s.want)
		}
	}
}

func TestSetRolePermissions_UnknownNameChangesNothing(t *testing.T) {
	f := newRolePermFixture(t)
	a, b := f.perms[0], f.perms[1]
	id := f.createRole(t, "unknown", a, b)

	err := f.svc.SetRolePermissions(context.Background(), id, []string{a, f.prefix + ":nope"})
	if !errors.Is(err, rbac.ErrUnknownPermission) {
		t.Fatalf("err = %v, want ErrUnknownPermission", err)
	}
	if got := f.held(t, id); !sameNames(got, []string{a, b}) {
		t.Fatalf("stored %v after a rejected update, want it unchanged", got)
	}
}

// Other roles holding the same permissions are untouched.
func TestSetRolePermissions_OnlyTouchesThatRole(t *testing.T) {
	f := newRolePermFixture(t)
	a, b := f.perms[0], f.perms[1]
	changed := f.createRole(t, "changed", a, b)
	other := f.createRole(t, "other", a, b)

	if err := f.svc.SetRolePermissions(context.Background(), changed, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.held(t, other); !sameNames(got, []string{a, b}) {
		t.Fatalf("other role holds %v, want it unchanged", got)
	}
}

// Concurrent replacements end with one of the lists, never a mix of both.
// Without the role row lock, six writers leave the union within a round.
func TestSetRolePermissions_ConcurrentReplacementsDoNotMix(t *testing.T) {
	f := newRolePermFixture(t)
	a, b, c := f.perms[0], f.perms[1], f.perms[2]
	ctx := context.Background()
	id := f.createRole(t, "race", a)

	lists := [][]string{{a, b}, {c}, {a, b}, {c}, {a, b}, {c}}
	for round := 0; round < 50; round++ {
		var wg sync.WaitGroup
		for _, l := range lists {
			wg.Add(1)
			go func(l []string) {
				defer wg.Done()
				if err := f.svc.SetRolePermissions(ctx, id, l); err != nil {
					t.Error(err)
				}
			}(l)
		}
		wg.Wait()
		got := f.held(t, id)
		if !sameNames(got, []string{a, b}) && !sameNames(got, []string{c}) {
			t.Fatalf("round %d: stored %v, want exactly %v or %v", round, got, lists[0], lists[1])
		}
	}
}

func TestSetRolePermissions_MissingRole(t *testing.T) {
	f := newRolePermFixture(t)
	if err := f.svc.SetRolePermissions(context.Background(), 1<<31, []string{f.perms[0]}); err == nil {
		t.Fatal("setting permissions on a missing role succeeded")
	}
}
