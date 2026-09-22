package rbac

import (
	"slices"
	"strings"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// ResolveRealmScope reduces a user's tenant policy rows to the realms they may
// read within one tenant.
//
// This is the single definition of what a tenant_policies row means. It exists
// because there used to be two: the web resolver in internal/http/chi and the
// MCP authorizer in internal/mcp disagreed on three of the five shapes a row
// can take, and drifted apart twice, once when RealmsUnrestricted was added
// and again when blank realms were dropped on only one side. Both callers now
// share this, so a policy means one thing whatever transport reads it.
//
// The contract, in the order the checks apply:
//
//   - No policy row for the tenant means unrestricted. tenant_policies is an
//     opt-in restriction, not a grant, so the absence of a row is not a denial.
//   - A row whose allowed_realms column holds no list at all, a SQL NULL or the
//     legacy `null` literal, is unrestricted for the same reason. That is what
//     RealmsUnrestricted reports.
//   - Otherwise the caller is restricted to the union of the named realms
//     across every matching row, because (user_id, tenant_id) is not unique and
//     a caller holding two rows is entitled to both.
//   - Blank realms are dropped. A blank names no realm, so it can only narrow
//     to nothing, yet left in a scope it reads as "no filter" downstream and
//     widens the caller to the whole tenant.
//
// all = true means unrestricted, and realms is then nil rather than every
// realm. all = false with an empty realms is a real, fail-closed restriction
// meaning no realm: callers must never read it as "no restriction".
func ResolveRealmScope(policies []*domain.TenantPolicy, tenantID string) (realms []string, all bool) {
	matched := false

	for _, policy := range policies {
		if policy == nil || policy.TenantID != tenantID {
			continue
		}
		if policy.RealmsUnrestricted {
			return nil, true
		}

		// Set before the realm loop, and deliberately not conditional on the
		// loop keeping anything: a row of nothing but blanks is still a row,
		// so it must resolve to an empty scope and stay fail-closed rather
		// than fall through to the unrestricted default below.
		matched = true

		for _, realm := range policy.AllowedRealms {
			// Dropped, never trimmed. Readers match realms by exact equality,
			// so rewriting " realm " into "realm" would grant access the
			// policy does not literally name. Only blanks are removed.
			if strings.TrimSpace(realm) == "" {
				continue
			}
			if !slices.Contains(realms, realm) {
				realms = append(realms, realm)
			}
		}
	}

	if !matched {
		return nil, true
	}
	return realms, false
}
