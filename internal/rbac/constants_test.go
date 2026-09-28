package rbac

import "testing"

// TestAmfaReadPermissionExists verifies that GetSystemPermissions includes the
// amfa:read permission with the expected metadata.
func TestAmfaReadPermissionExists(t *testing.T) {
	perms := GetSystemPermissions()

	var found *PermissionDefinition
	for i := range perms {
		if perms[i].Name == PermissionAmfaRead {
			found = &perms[i]
			break
		}
	}

	if found == nil {
		t.Fatalf("GetSystemPermissions() does not include %q permission", PermissionAmfaRead)
		return
	}

	if found.Name != "amfa:read" {
		t.Errorf("expected permission name %q, got %q", "amfa:read", found.Name)
	}
	if found.Resource != "amfa" {
		t.Errorf("expected resource %q, got %q", "amfa", found.Resource)
	}
	if found.Action != "read" {
		t.Errorf("expected action %q, got %q", "read", found.Action)
	}
	if found.Description == "" {
		t.Errorf("expected non-empty description for %q permission", PermissionAmfaRead)
	}
}

// TestDefaultRolesIncludeAmfaRead verifies that the admin, operator, and
// viewer system roles all include the amfa:read permission.
func TestDefaultRolesIncludeAmfaRead(t *testing.T) {
	expectedRoles := map[string]bool{
		RoleAdmin:    false,
		RoleOperator: false,
		RoleViewer:   false,
	}

	for _, role := range GetSystemRoles() {
		if _, ok := expectedRoles[role.Name]; !ok {
			continue
		}

		hasAmfaRead := false
		for _, perm := range role.Permissions {
			if perm == PermissionAmfaRead {
				hasAmfaRead = true
				break
			}
		}
		if !hasAmfaRead {
			t.Errorf("role %q does not include %q permission", role.Name, PermissionAmfaRead)
		}
		expectedRoles[role.Name] = true
	}

	for name, seen := range expectedRoles {
		if !seen {
			t.Errorf("GetSystemRoles() did not return expected role %q", name)
		}
	}
}
