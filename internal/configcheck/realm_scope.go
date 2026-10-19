package configcheck

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// realmScope is what one run of a check knows about the tenant's realms: which
// ones it checks, which ones Keycloak lists, and which ones it could not read.
// It judges the open alerts of realms the run did not check.
type realmScope struct {
	ctx context.Context
	kc  KeycloakClient
	// configured is set when the check reads a configured list of realms
	// rather than every realm Keycloak lists.
	configured bool
	// checked holds the realms the run checks.
	checked map[string]bool
	// visible holds the realms Keycloak lists with their settings, enabled or
	// not.
	visible map[string]bool
	// gone caches, per realm the run did not check, whether it was deleted.
	gone map[string]bool
	// unreadable holds the realms Keycloak answered for without their
	// settings: listed as a stub, read as a stub, or unlisted but not deleted.
	unreadable map[string]bool
}

// scopeRealms returns the realms a check reads, the configured ones or else
// every enabled realm Keycloak lists, and the scope of the run.
func scopeRealms(ctx context.Context, kc KeycloakClient, configured []string) ([]string, *realmScope, error) {
	s := &realmScope{
		ctx:        ctx,
		kc:         kc,
		configured: len(configured) > 0,
		checked:    map[string]bool{},
		visible:    map[string]bool{},
		gone:       map[string]bool{},
		unreadable: map[string]bool{},
	}
	realms := configured
	if !s.configured {
		all, err := kc.GetAllRealms(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch all realms: %w", err)
		}
		realms = make([]string, 0, len(all))
		for _, realm := range all {
			// Keycloak lists a realm the client may not view as a stub with
			// no id and no enabled flag; that is not a disabled realm.
			if realm.ID == "" {
				s.unreadable[realm.Realm] = true
				continue
			}
			s.visible[realm.Realm] = true
			if realm.Enabled {
				realms = append(realms, realm.Realm)
			}
		}
	}
	for _, realm := range realms {
		s.checked[realm] = true
	}
	return realms, s, nil
}

// markUnreadable records that the run could not read realm's settings.
func (s *realmScope) markUnreadable(realm string) { s.unreadable[realm] = true }

// unreadableRealms lists the realms the run could not read the settings of,
// including the ones found while judging alerts; call it after goneWith.
func (s *realmScope) unreadableRealms() []string {
	realms := make([]string, 0, len(s.unreadable))
	for realm := range s.unreadable {
		realms = append(realms, realm)
	}
	return realms
}

// goneWith reports whether alert belongs to a realm the run did not check
// because the realm is out of the run or gone: left out on purpose, as one not
// configured or disabled is, or deleted. Its problem went with the realm.
// Keycloak lists only the realms the monitoring client can see, and still
// answers 200 for a realm the client lost its roles in, so only a 404 says a
// realm is gone; a realm that answers is unreadable and keeps its alerts.
func (s *realmScope) goneWith(alert *domain.Alert) bool {
	realm := alert.RealmName
	if realm == "" || s.checked[realm] || s.unreadable[realm] {
		return false
	}
	if s.configured || s.visible[realm] {
		return true
	}
	if gone, ok := s.gone[realm]; ok {
		return gone
	}
	_, err := s.kc.GetRealmInfo(s.ctx, realm)
	gone := errors.Is(err, keycloakadmin.ErrNotFound)
	s.gone[realm] = gone
	if err == nil {
		s.unreadable[realm] = true
	}
	return gone
}
