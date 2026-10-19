package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// /ready follows the readiness check; /health is liveness and stays 200.
func TestReadyFollowsTheCheckAndHealthDoesNot(t *testing.T) {
	server := newServerWithClock(defaultTestCfg(), testLogger(), acceptingValidator("pat_good"),
		&mockPermissionService{}, &mockTenantReader{}, &mockRealmReader{},
		&mockAlertReader{}, &mockAmfaStatsReader{}, &mockEventReader{}, nil, time.Now)
	var dbErr error
	server.SetReadinessCheck(func(context.Context) error { return dbErr })
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	status := func(path string) int {
		t.Helper()
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if got := status("/ready"); got != http.StatusOK {
		t.Errorf("/ready with the database up = %d, want 200", got)
	}
	dbErr = errors.New("connection refused")
	if got := status("/ready"); got != http.StatusServiceUnavailable {
		t.Errorf("/ready with the database down = %d, want 503", got)
	}
	if got := status("/health"); got != http.StatusOK {
		t.Errorf("/health with the database down = %d, want 200", got)
	}
}
