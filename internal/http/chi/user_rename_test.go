package chi

import (
	"errors"
	"net/http"
	"testing"
)

// Renaming a simple-auth user: a name no account has is available, which the
// repository reports as users.ErrUserNotFound. It used to be taken for a
// failure and every rename answered 500.
func TestUpdateUser_RenameToAFreeUsernameSucceeds(t *testing.T) {
	srv, svc := usersTestServer(t, adminUserID, []uint{adminUserID})

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/3/", `{"username":"renamed"}`); got != http.StatusOK {
		t.Fatalf("rename to a free username = %d, want 200", got)
	}
	u := svc.lastUpdate
	if u == nil || u.ID != regularUserID {
		t.Fatalf("the rename was not saved: %+v", u)
	}
	if u.Username != "renamed" || u.PreferredUsername != "renamed" || u.Subject != "renamed" {
		t.Errorf("saved username=%q preferred=%q subject=%q, want all renamed", u.Username, u.PreferredUsername, u.Subject)
	}
}

func TestUpdateUser_RenameToATakenUsernameIsAConflict(t *testing.T) {
	srv, svc := usersTestServer(t, adminUserID, []uint{adminUserID})

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/3/", `{"username":"manager"}`); got != http.StatusConflict {
		t.Fatalf("rename to a taken username = %d, want 409", got)
	}
	if len(svc.updated) != 0 {
		t.Fatalf("a refused rename reached the service: %v", svc.updated)
	}
}

// A failure to look the name up is still a server error, not "available".
func TestUpdateUser_RenameWhenTheLookupFailsIsAServerError(t *testing.T) {
	srv, svc := usersTestServer(t, adminUserID, []uint{adminUserID})
	svc.usernameErr = errors.New("connection refused")

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/3/", `{"username":"renamed"}`); got != http.StatusInternalServerError {
		t.Fatalf("rename with a failing lookup = %d, want 500", got)
	}
	if len(svc.updated) != 0 {
		t.Fatalf("the rename was saved although the name could not be checked: %v", svc.updated)
	}
}
