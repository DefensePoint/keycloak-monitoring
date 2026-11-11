package chi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// versionInfoErrService answers GetVersionInfo with err.
type versionInfoErrService struct {
	stubKeycloakService
	err error
}

func (s *versionInfoErrService) GetVersionInfo(context.Context, string) (*keycloak.VersionCheckResult, error) {
	return nil, s.err
}

// A failed GitHub fetch is logged by the version checker, once, so the
// handler answers 500 without logging it again; any other failure is logged.
func TestHandleKeycloakVersionCheck_LogsOnlyFailuresTheCheckerDidNotLog(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantLog bool
	}{
		{"failed GitHub fetch", fmt.Errorf("%w: dial tcp: i/o timeout", keycloak.ErrVersionLookupFailed), false},
		{"other failure", errors.New("health query failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := NewKeycloakHandlers(&versionInfoErrService{err: tt.err}, nil, nil,
				logger.New(zerolog.New(&buf)), testAuthMiddleware, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/t1/keycloak/version-check", nil)
			rc := chi.NewRouteContext()
			rc.URLParams.Add("tenantID", "t1")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
			w := httptest.NewRecorder()
			h.handleKeycloakVersionCheck(w, req)

			if w.Code != http.StatusInternalServerError {
				t.Errorf("status %d, want 500", w.Code)
			}
			if logged := strings.Contains(buf.String(), "Failed to get version info"); logged != tt.wantLog {
				t.Errorf("logged = %v, want %v: %s", logged, tt.wantLog, buf.String())
			}
		})
	}
}
