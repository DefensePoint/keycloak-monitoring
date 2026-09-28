package alerts

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/rs/zerolog"
)

// newTestLogger creates a logger that discards output for tests
func newTestLogger() *logger.Logger {
	return logger.New(zerolog.New(io.Discard))
}

// mockRepository implements Repository interface for testing
type mockRepository struct {
	getByAlertIDFn       func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
	getByAlertIDGlobalFn func(ctx context.Context, alertID string) (*domain.Alert, error)
	listFn               func(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error)
	listByRealmFn        func(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error)
	countActiveFn        func(ctx context.Context, tenantID string) (int64, error)
	countFn              func(ctx context.Context, tenantID string, opts *ListOptions) (int64, error)
	getStatisticsFn      func(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error)
	saveFn               func(ctx context.Context, alert *domain.Alert) error
	updateStatusFn       func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error
	resolveFn            func(ctx context.Context, tenantID, alertID string) error
	deleteFn             func(ctx context.Context, tenantID, alertID string) error
}

func (m *mockRepository) GetByID(ctx context.Context, id uint) (*domain.Alert, error) {
	return nil, nil
}

func (m *mockRepository) GetByAlertID(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	if m.getByAlertIDFn != nil {
		return m.getByAlertIDFn(ctx, tenantID, alertID)
	}
	return nil, nil
}

func (m *mockRepository) GetByAlertIDGlobal(ctx context.Context, alertID string) (*domain.Alert, error) {
	if m.getByAlertIDGlobalFn != nil {
		return m.getByAlertIDGlobalFn(ctx, alertID)
	}
	return nil, nil
}

func (m *mockRepository) List(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error) {
	if m.listFn != nil {
		return m.listFn(ctx, tenantID, opts)
	}
	return nil, nil
}

func (m *mockRepository) ListByRealm(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error) {
	if m.listByRealmFn != nil {
		return m.listByRealmFn(ctx, tenantID, realmName, opts)
	}
	return nil, nil
}

func (m *mockRepository) CountActive(ctx context.Context, tenantID string) (int64, error) {
	if m.countActiveFn != nil {
		return m.countActiveFn(ctx, tenantID)
	}
	return 0, nil
}

func (m *mockRepository) Count(ctx context.Context, tenantID string, opts *ListOptions) (int64, error) {
	if m.countFn != nil {
		return m.countFn(ctx, tenantID, opts)
	}
	return 0, nil
}

func (m *mockRepository) GetStatistics(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error) {
	if m.getStatisticsFn != nil {
		return m.getStatisticsFn(ctx, tenantID, realmName...)
	}
	return nil, nil
}

func (m *mockRepository) Save(ctx context.Context, alert *domain.Alert) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, alert)
	}
	return nil
}

func (m *mockRepository) UpdateStatus(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, tenantID, alertID, status, acknowledgedBy)
	}
	return nil
}

func (m *mockRepository) Resolve(ctx context.Context, tenantID, alertID string) error {
	if m.resolveFn != nil {
		return m.resolveFn(ctx, tenantID, alertID)
	}
	return nil
}

func (m *mockRepository) Delete(ctx context.Context, tenantID, alertID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, tenantID, alertID)
	}
	return nil
}

func TestGetAlert(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		alertID   string
		mockFn    func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
		wantErr   bool
		wantAlert *domain.Alert
	}{
		{
			name:     "successful get",
			tenantID: "tenant-1",
			alertID:  "alert-123",
			mockFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{
					AlertID:  alertID,
					TenantID: tenantID,
					Status:   domain.AlertStatusActive,
				}, nil
			},
			wantErr: false,
			wantAlert: &domain.Alert{
				AlertID:  "alert-123",
				TenantID: "tenant-1",
				Status:   domain.AlertStatusActive,
			},
		},
		{
			name:     "alert not found",
			tenantID: "tenant-1",
			alertID:  "non-existent",
			mockFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return nil, errors.New("alert not found")
			},
			wantErr:   true,
			wantAlert: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getByAlertIDFn: tt.mockFn,
			}
			svc := NewService(repo, newTestLogger())

			got, err := svc.GetAlert(context.Background(), tt.tenantID, tt.alertID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAlert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantAlert != nil && got != nil {
				if got.AlertID != tt.wantAlert.AlertID {
					t.Errorf("GetAlert() AlertID = %v, want %v", got.AlertID, tt.wantAlert.AlertID)
				}
				if got.TenantID != tt.wantAlert.TenantID {
					t.Errorf("GetAlert() TenantID = %v, want %v", got.TenantID, tt.wantAlert.TenantID)
				}
			}
		})
	}
}

func TestGetAlertGlobal(t *testing.T) {
	tests := []struct {
		name      string
		alertID   string
		mockFn    func(ctx context.Context, alertID string) (*domain.Alert, error)
		wantErr   bool
		wantAlert *domain.Alert
	}{
		{
			name:    "successful global get",
			alertID: "alert-123",
			mockFn: func(ctx context.Context, alertID string) (*domain.Alert, error) {
				return &domain.Alert{
					AlertID:  alertID,
					TenantID: "tenant-1",
					Status:   domain.AlertStatusActive,
				}, nil
			},
			wantErr: false,
			wantAlert: &domain.Alert{
				AlertID:  "alert-123",
				TenantID: "tenant-1",
			},
		},
		{
			name:    "alert not found globally",
			alertID: "non-existent",
			mockFn: func(ctx context.Context, alertID string) (*domain.Alert, error) {
				return nil, errors.New("alert not found")
			},
			wantErr:   true,
			wantAlert: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getByAlertIDGlobalFn: tt.mockFn,
			}
			svc := NewService(repo, newTestLogger())

			got, err := svc.GetAlertGlobal(context.Background(), tt.alertID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAlertGlobal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantAlert != nil && got != nil {
				if got.AlertID != tt.wantAlert.AlertID {
					t.Errorf("GetAlertGlobal() AlertID = %v, want %v", got.AlertID, tt.wantAlert.AlertID)
				}
			}
		})
	}
}

func TestListAlerts(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		opts      *ListOptions
		listFn    func(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error)
		countFn   func(ctx context.Context, tenantID string, opts *ListOptions) (int64, error)
		wantErr   bool
		wantCount int
		wantTotal int64
	}{
		{
			name:     "successful list with default options",
			tenantID: "tenant-1",
			opts:     nil,
			listFn: func(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error) {
				return []*domain.Alert{
					{AlertID: "alert-1", TenantID: tenantID},
					{AlertID: "alert-2", TenantID: tenantID},
				}, nil
			},
			countFn: func(ctx context.Context, tenantID string, opts *ListOptions) (int64, error) {
				return 2, nil
			},
			wantErr:   false,
			wantCount: 2,
			wantTotal: 2,
		},
		{
			name:     "list with custom options",
			tenantID: "tenant-1",
			opts:     &ListOptions{Limit: 10, Offset: 0},
			listFn: func(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error) {
				return []*domain.Alert{
					{AlertID: "alert-1", TenantID: tenantID},
				}, nil
			},
			countFn: func(ctx context.Context, tenantID string, opts *ListOptions) (int64, error) {
				return 5, nil
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 5,
		},
		{
			name:     "list error",
			tenantID: "tenant-1",
			opts:     nil,
			listFn: func(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error) {
				return nil, errors.New("database error")
			},
			countFn:   nil,
			wantErr:   true,
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:     "count error returns zero but no error",
			tenantID: "tenant-1",
			opts:     nil,
			listFn: func(ctx context.Context, tenantID string, opts *ListOptions) ([]*domain.Alert, error) {
				return []*domain.Alert{{AlertID: "alert-1"}}, nil
			},
			countFn: func(ctx context.Context, tenantID string, opts *ListOptions) (int64, error) {
				return 0, errors.New("count error")
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 0, // Count fails but service continues
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				listFn:  tt.listFn,
				countFn: tt.countFn,
			}
			svc := NewService(repo, newTestLogger())

			alerts, total, err := svc.ListAlerts(context.Background(), tt.tenantID, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListAlerts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(alerts) != tt.wantCount {
				t.Errorf("ListAlerts() count = %v, want %v", len(alerts), tt.wantCount)
			}
			if total != tt.wantTotal {
				t.Errorf("ListAlerts() total = %v, want %v", total, tt.wantTotal)
			}
		})
	}
}

func TestListAlertsByRealm(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		realmName string
		opts      *ListOptions
		mockFn    func(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error)
		wantErr   bool
		wantCount int
	}{
		{
			name:      "successful list by realm",
			tenantID:  "tenant-1",
			realmName: "master",
			opts:      nil,
			mockFn: func(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error) {
				return []*domain.Alert{
					{AlertID: "alert-1", TenantID: tenantID, RealmName: realmName},
				}, nil
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name:      "empty result",
			tenantID:  "tenant-1",
			realmName: "empty-realm",
			opts:      nil,
			mockFn: func(ctx context.Context, tenantID, realmName string, opts *ListOptions) ([]*domain.Alert, error) {
				return []*domain.Alert{}, nil
			},
			wantErr:   false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				listByRealmFn: tt.mockFn,
			}
			svc := NewService(repo, newTestLogger())

			alerts, err := svc.ListAlertsByRealm(context.Background(), tt.tenantID, tt.realmName, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListAlertsByRealm() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(alerts) != tt.wantCount {
				t.Errorf("ListAlertsByRealm() count = %v, want %v", len(alerts), tt.wantCount)
			}
		})
	}
}

func TestGetStatistics(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		realmName []string
		mockFn    func(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error)
		wantErr   bool
		wantStats *Statistics
	}{
		{
			name:      "successful statistics",
			tenantID:  "tenant-1",
			realmName: []string{"master"},
			mockFn: func(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error) {
				return &Statistics{
					TotalActive: 10,
					BySeverity:  map[string]int{"high": 3, "medium": 7},
				}, nil
			},
			wantErr: false,
			wantStats: &Statistics{
				TotalActive: 10,
			},
		},
		{
			name:      "statistics error",
			tenantID:  "tenant-1",
			realmName: nil,
			mockFn: func(ctx context.Context, tenantID string, realmName ...string) (*Statistics, error) {
				return nil, errors.New("database error")
			},
			wantErr:   true,
			wantStats: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getStatisticsFn: tt.mockFn,
			}
			svc := NewService(repo, newTestLogger())

			stats, err := svc.GetStatistics(context.Background(), tt.tenantID, tt.realmName...)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetStatistics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantStats != nil && stats != nil {
				if stats.TotalActive != tt.wantStats.TotalActive {
					t.Errorf("GetStatistics() TotalActive = %v, want %v", stats.TotalActive, tt.wantStats.TotalActive)
				}
			}
		})
	}
}

func TestUpdateStatus(t *testing.T) {
	tests := []struct {
		name           string
		tenantID       string
		alertID        string
		status         domain.AlertStatus
		acknowledgedBy string
		updateFn       func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error
		getFn          func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
		wantErr        bool
	}{
		{
			name:           "successful status update",
			tenantID:       "tenant-1",
			alertID:        "alert-123",
			status:         domain.AlertStatusAcknowled,
			acknowledgedBy: "user@example.com",
			updateFn: func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error {
				return nil
			},
			getFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{
					AlertID:  alertID,
					TenantID: tenantID,
					Status:   domain.AlertStatusAcknowled,
				}, nil
			},
			wantErr: false,
		},
		{
			name:           "invalid status",
			tenantID:       "tenant-1",
			alertID:        "alert-123",
			status:         domain.AlertStatus("invalid"),
			acknowledgedBy: "user@example.com",
			updateFn:       nil,
			getFn:          nil,
			wantErr:        true,
		},
		{
			name:           "update error",
			tenantID:       "tenant-1",
			alertID:        "alert-123",
			status:         domain.AlertStatusResolved,
			acknowledgedBy: "user@example.com",
			updateFn: func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error {
				return errors.New("update failed")
			},
			getFn:   nil,
			wantErr: true,
		},
		{
			name:           "get after update fails but returns minimal alert",
			tenantID:       "tenant-1",
			alertID:        "alert-123",
			status:         domain.AlertStatusResolved,
			acknowledgedBy: "user@example.com",
			updateFn: func(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, acknowledgedBy string) error {
				return nil
			},
			getFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return nil, errors.New("get failed")
			},
			wantErr: false, // Service returns minimal alert on get failure
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				updateStatusFn: tt.updateFn,
				getByAlertIDFn: tt.getFn,
			}
			svc := NewService(repo, newTestLogger())

			alert, err := svc.UpdateStatus(context.Background(), tt.tenantID, tt.alertID, tt.status, tt.acknowledgedBy)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && alert == nil {
				t.Error("UpdateStatus() returned nil alert on success")
			}
		})
	}
}

func TestResolveAlert(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		alertID   string
		resolveFn func(ctx context.Context, tenantID, alertID string) error
		getFn     func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error)
		wantErr   bool
	}{
		{
			name:     "successful resolve",
			tenantID: "tenant-1",
			alertID:  "alert-123",
			resolveFn: func(ctx context.Context, tenantID, alertID string) error {
				return nil
			},
			getFn: func(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
				return &domain.Alert{
					AlertID:  alertID,
					TenantID: tenantID,
					Status:   domain.AlertStatusResolved,
				}, nil
			},
			wantErr: false,
		},
		{
			name:     "resolve error",
			tenantID: "tenant-1",
			alertID:  "alert-123",
			resolveFn: func(ctx context.Context, tenantID, alertID string) error {
				return errors.New("resolve failed")
			},
			getFn:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				resolveFn:      tt.resolveFn,
				getByAlertIDFn: tt.getFn,
			}
			svc := NewService(repo, newTestLogger())

			alert, err := svc.ResolveAlert(context.Background(), tt.tenantID, tt.alertID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveAlert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && alert == nil {
				t.Error("ResolveAlert() returned nil alert on success")
			}
		})
	}
}

func TestDeleteAlert(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		alertID  string
		mockFn   func(ctx context.Context, tenantID, alertID string) error
		wantErr  bool
	}{
		{
			name:     "successful delete",
			tenantID: "tenant-1",
			alertID:  "alert-123",
			mockFn: func(ctx context.Context, tenantID, alertID string) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:     "delete error",
			tenantID: "tenant-1",
			alertID:  "alert-123",
			mockFn: func(ctx context.Context, tenantID, alertID string) error {
				return errors.New("delete failed")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				deleteFn: tt.mockFn,
			}
			svc := NewService(repo, newTestLogger())

			err := svc.DeleteAlert(context.Background(), tt.tenantID, tt.alertID)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteAlert() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveAlert(t *testing.T) {
	tests := []struct {
		name    string
		alert   *domain.Alert
		mockFn  func(ctx context.Context, alert *domain.Alert) error
		wantErr bool
	}{
		{
			name: "successful save",
			alert: &domain.Alert{
				AlertID:  "alert-123",
				TenantID: "tenant-1",
				Status:   domain.AlertStatusActive,
			},
			mockFn: func(ctx context.Context, alert *domain.Alert) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "save error",
			alert: &domain.Alert{
				AlertID:  "alert-123",
				TenantID: "tenant-1",
			},
			mockFn: func(ctx context.Context, alert *domain.Alert) error {
				return errors.New("save failed")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				saveFn: tt.mockFn,
			}
			svc := NewService(repo, newTestLogger())

			err := svc.SaveAlert(context.Background(), tt.alert)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveAlert() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidStatus(t *testing.T) {
	tests := []struct {
		name   string
		status domain.AlertStatus
		want   bool
	}{
		{"active is valid", domain.AlertStatusActive, true},
		{"resolved is valid", domain.AlertStatusResolved, true},
		{"acknowledged is valid", domain.AlertStatusAcknowled, true},
		{"ignored is valid", domain.AlertStatusIgnored, true},
		{"invalid status", domain.AlertStatus("invalid"), false},
		{"empty status", domain.AlertStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidStatus(tt.status); got != tt.want {
				t.Errorf("isValidStatus(%v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
