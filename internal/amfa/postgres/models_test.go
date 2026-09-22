package postgres

import "testing"

func TestAuthEventTableName(t *testing.T) {
	got := AuthEvent{}.TableName()
	if got != "auth_event" {
		t.Errorf("AuthEvent.TableName() = %q, want %q", got, "auth_event")
	}
}

func TestAuthProcessTableName(t *testing.T) {
	got := AuthProcess{}.TableName()
	if got != "auth_process" {
		t.Errorf("AuthProcess.TableName() = %q, want %q", got, "auth_process")
	}
}

func TestAuthContextTableName(t *testing.T) {
	got := AuthContext{}.TableName()
	if got != "auth_context" {
		t.Errorf("AuthContext.TableName() = %q, want %q", got, "auth_context")
	}
}

// TestModelsHaveTableNames is a sanity check that all three models expose a
// TableName() method returning a non-empty string. Read-only enforcement
// itself relies on the gorm:"->" tag (compile-time convention) and the DB
// role's least-privilege grants (runtime enforcement).
func TestModelsHaveTableNames(t *testing.T) {
	cases := []struct {
		name string
		got  string
	}{
		{"AuthEvent", AuthEvent{}.TableName()},
		{"AuthProcess", AuthProcess{}.TableName()},
		{"AuthContext", AuthContext{}.TableName()},
	}
	for _, c := range cases {
		if c.got == "" {
			t.Errorf("%s.TableName() is empty", c.name)
		}
	}
}
