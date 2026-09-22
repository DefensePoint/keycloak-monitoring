package rbac

import (
	"strings"
	"testing"
)

// A blank realm names no realm, so it can only be a mistake, and stored it is
// a fail-open one: readers that drop it end up with no realm filter at all.
// Rejecting it at the write boundary keeps a policy meaning what it says on
// disk. The read path drops blanks defensively regardless.
func TestValidateAllowedRealms(t *testing.T) {
	tests := []struct {
		name    string
		realms  []string
		wantErr bool
	}{
		{
			// nil is not a list: it means unrestricted, and stays valid.
			name:   "nil list is unrestricted, not a blank realm",
			realms: nil,
		},
		{
			// An empty list is a real restriction that denies every realm.
			name:   "empty list is a fail-closed restriction",
			realms: []string{},
		},
		{
			name:   "a named realm is valid",
			realms: []string{"realmB"},
		},
		{
			name:   "several named realms are valid",
			realms: []string{"realmA", "realmB"},
		},
		{
			name:    "a single blank realm is rejected",
			realms:  []string{""},
			wantErr: true,
		},
		{
			name:    "a whitespace-only realm is rejected",
			realms:  []string{"   "},
			wantErr: true,
		},
		{
			name:    "a tab-only realm is rejected",
			realms:  []string{"\t"},
			wantErr: true,
		},
		{
			// The shape most likely to arrive from a form with a spare row.
			name:    "a blank beside a real realm is still rejected",
			realms:  []string{"realmB", ""},
			wantErr: true,
		},
		{
			// Padding is not blank. It is kept, and matched verbatim by
			// readers, so it grants nothing it does not literally name.
			name:   "a padded realm name is not blank",
			realms: []string{" realmB "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAllowedRealms(tt.realms)
			if tt.wantErr {
				if err == nil {
					t.Fatal("validateAllowedRealms() = nil, want an error")
				}
				// The message has to tell an admin what to do instead, since
				// the difference between [] and [""] is otherwise invisible.
				if !strings.Contains(err.Error(), "blank") {
					t.Errorf("error %q does not say the realm is blank", err)
				}
				return
			}
			if err != nil {
				t.Errorf("validateAllowedRealms() = %v, want nil", err)
			}
		})
	}
}

// The index has to point at the offending entry, or an admin with a long realm
// list cannot tell which one is wrong.
func TestValidateAllowedRealms_NamesTheOffendingIndex(t *testing.T) {
	err := validateAllowedRealms([]string{"realmA", "realmB", ""})
	if err == nil {
		t.Fatal("validateAllowedRealms() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "[2]") {
		t.Errorf("error %q does not name index 2", err)
	}
}
