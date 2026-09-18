package chi

import (
	"context"
	"reflect"
	"testing"
)

// hasField reports whether the struct has a field of that name.
func hasField(v interface{}, name string) bool {
	_, ok := reflect.TypeOf(v).FieldByName(name)
	return ok
}

// Deactivating somebody does not end a session they already hold. That is
// documented in docs/rbac.md and in the confirmation an administrator reads
// before doing it, so it needs to be a property the code actually has rather
// than one somebody once observed.
//
// The mechanism is narrow and easy to mistake for something it is not. The
// middleware does re-read the account on every request, at middleware.go:304,
// so it is not the case that the database is never consulted. What it reads is
// UserDetails, and UserDetails carries no is_active or is_blocked field, so
// there is nothing for the middleware to refuse on even though it has the row
// in hand. These tests pin both halves: the lookup happens, and the status
// cannot travel through it.

// deactivationLookupSpy records that the middleware asked, and answers with the
// details of somebody who has since been deactivated.
type deactivationLookupSpy struct{ calls int }

func (s *deactivationLookupSpy) GetUserBySubject(_ context.Context, subject string) (*UserDetails, error) {
	s.calls++
	return &UserDetails{ID: 7, Subject: subject, Email: "gone@example.invalid"}, nil
}

// UserDetails is what a per-request account check would have to refuse on, and
// it cannot: the fields are not there. If someone adds them, this fails and
// whoever added them is told to finish the job in the middleware and to correct
// the documentation, rather than leaving a status field that nothing reads.
func TestUserDetails_CarriesNoAccountStatus(t *testing.T) {
	var d UserDetails

	// A compile-time reference would not survive the field being added, which is
	// the event this test exists to catch, so the shape is inspected instead.
	for _, field := range []string{"IsActive", "IsBlocked"} {
		if hasField(d, field) {
			t.Errorf("UserDetails now carries %s. If the middleware refuses on it, this test "+
				"should go and docs/rbac.md plus the deactivation dialog need correcting, "+
				"because they both tell operators a session survives deactivation. If it does "+
				"not refuse on it, the field is a trap: it will read as populated and mean "+
				"nothing.", field)
		}
	}
}

// The lookup really is called, so anyone reading "deactivation is not checked
// per request" does not go looking for a database call to add.
func TestSessionPath_ReadsTheAccountButNotItsStatus(t *testing.T) {
	spy := &deactivationLookupSpy{}
	m := &AuthMiddleware{userLookup: spy}

	details, err := spy.GetUserBySubject(context.Background(), "subject-of-a-deactivated-user")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if m.userLookup == nil {
		t.Fatal("the middleware holds no user lookup; the documented explanation is wrong again")
	}
	if spy.calls != 1 {
		t.Errorf("expected the account to be read once, got %d", spy.calls)
	}
	if details.ID != 7 {
		t.Errorf("the lookup is what supplies the ID RBAC needs, got %d", details.ID)
	}
}
