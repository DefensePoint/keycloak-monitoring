package configcheck

import (
	"context"
	"errors"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// testKeycloakClient implements KeycloakClient for testing
type testKeycloakClient struct {
	realms            []*keycloakadmin.RealmRepresentation
	clients           map[string][]*keycloakadmin.ClientRepresentation
	identityProviders map[string][]*keycloakadmin.IdentityProviderRepresentation
	err               error
}

func (m *testKeycloakClient) GetAllRealms(ctx context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.realms, nil
}

func (m *testKeycloakClient) GetRealmInfo(ctx context.Context, realmName string) (*keycloakadmin.RealmRepresentation, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, realm := range m.realms {
		if realm.Realm == realmName {
			return realm, nil
		}
	}
	return nil, errors.New("realm not found")
}

func (m *testKeycloakClient) GetClients(ctx context.Context, realmName string) ([]*keycloakadmin.ClientRepresentation, error) {
	if m.err != nil {
		return nil, m.err
	}
	if clients, ok := m.clients[realmName]; ok {
		return clients, nil
	}
	return []*keycloakadmin.ClientRepresentation{}, nil
}

func (m *testKeycloakClient) GetIdentityProviders(ctx context.Context, realmName string) ([]*keycloakadmin.IdentityProviderRepresentation, error) {
	if m.err != nil {
		return nil, m.err
	}
	if idps, ok := m.identityProviders[realmName]; ok {
		return idps, nil
	}
	return []*keycloakadmin.IdentityProviderRepresentation{}, nil
}
