package apitoken

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// noopLogger implements Logger and discards output for tests
type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Debug(string, ...any) {}

// mockRepository implements Repository interface for testing
type mockRepository struct {
	createFn      func(ctx context.Context, token *Token, digest string) error
	listFn        func(ctx context.Context) ([]*Token, error)
	getByDigestFn func(ctx context.Context, digest string) (*Token, error)
	revokeFn      func(ctx context.Context, id uint, revokedAt time.Time) error
}

func (m *mockRepository) Create(ctx context.Context, token *Token, digest string) error {
	if m.createFn != nil {
		return m.createFn(ctx, token, digest)
	}
	return nil
}

func (m *mockRepository) List(ctx context.Context) ([]*Token, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockRepository) GetByDigest(ctx context.Context, digest string) (*Token, error) {
	if m.getByDigestFn != nil {
		return m.getByDigestFn(ctx, digest)
	}
	return nil, nil
}

func (m *mockRepository) Revoke(ctx context.Context, id uint, revokedAt time.Time) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, id, revokedAt)
	}
	return nil
}

// mockUserStore implements UserStore interface for testing
type mockUserStore struct {
	getByIDFn func(ctx context.Context, userID uint) (*domain.User, error)
}

func (m *mockUserStore) GetByID(ctx context.Context, userID uint) (*domain.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, userID)
	}
	return nil, nil
}

func activeUser(userID uint) *domain.User {
	return &domain.User{ID: userID, Email: "user@example.com", IsActive: true}
}

func newTestService(repo Repository, users UserStore) Service {
	return NewService(repo, users, noopLogger{})
}

func TestCreateStoresDigestNotPlaintext(t *testing.T) {
	var storedToken *Token
	var storedDigest string
	repo := &mockRepository{
		createFn: func(ctx context.Context, token *Token, digest string) error {
			storedToken = token
			storedDigest = digest
			token.ID = 7
			token.CreatedAt = time.Now()
			return nil
		},
	}
	users := &mockUserStore{
		getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
			return activeUser(userID), nil
		},
	}
	svc := newTestService(repo, users)

	created, err := svc.Create(context.Background(), 1, "ci-token", []string{"tenant-1"}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !strings.HasPrefix(created.Plaintext, "pat_") {
		t.Errorf("Create() plaintext = %q, want pat_ prefix", created.Plaintext)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(created.Plaintext, "pat_"))
	if err != nil {
		t.Fatalf("Create() plaintext is not base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("Create() random payload = %d bytes, want 32", len(raw))
	}

	sum := sha256.Sum256([]byte(created.Plaintext))
	if wantDigest := hex.EncodeToString(sum[:]); storedDigest != wantDigest {
		t.Errorf("Create() stored digest = %q, want SHA-256 hex of plaintext %q", storedDigest, wantDigest)
	}
	if storedDigest == created.Plaintext {
		t.Error("Create() stored the plaintext as digest")
	}
	if storedToken == nil {
		t.Fatal("Create() did not store a token")
	}
	if created.ID != 7 {
		t.Errorf("Create() ID = %d, want 7", created.ID)
	}
	if created.Name != "ci-token" {
		t.Errorf("Create() Name = %q, want %q", created.Name, "ci-token")
	}
	if len(created.TenantIDs) != 1 || created.TenantIDs[0] != "tenant-1" {
		t.Errorf("Create() TenantIDs = %v, want [tenant-1]", created.TenantIDs)
	}
}

func TestCreateRequiresATenantAllowlist(t *testing.T) {
	users := &mockUserStore{
		getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
			return activeUser(userID), nil
		},
	}
	svc := newTestService(&mockRepository{}, users)

	for name, tenantIDs := range map[string][]string{"nil": nil, "empty": {}} {
		if _, err := svc.Create(context.Background(), 1, "token", tenantIDs, nil); !errors.Is(err, ErrNoTenantIDs) {
			t.Errorf("Create() with a %s allowlist = %v, want %v", name, err, ErrNoTenantIDs)
		}
	}
}

func TestCreateExpiry(t *testing.T) {
	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	past := time.Now().UTC().Add(-time.Hour)
	beyondMax := time.Now().UTC().Add(400 * 24 * time.Hour)

	tests := []struct {
		name       string
		expiresAt  *time.Time
		wantErr    error
		wantExpiry time.Duration
	}{
		{
			name:       "nil expiry defaults to 90 days",
			expiresAt:  nil,
			wantExpiry: DefaultExpiry,
		},
		{
			name:       "explicit expiry within range is preserved",
			expiresAt:  &future,
			wantExpiry: 30 * 24 * time.Hour,
		},
		{
			name:       "expiry beyond max is capped at 365 days",
			expiresAt:  &beyondMax,
			wantExpiry: MaxExpiry,
		},
		{
			name:      "expiry in the past is rejected",
			expiresAt: &past,
			wantErr:   ErrExpiryInPast,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{}
			users := &mockUserStore{
				getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
					return activeUser(userID), nil
				},
			}
			svc := newTestService(repo, users)

			created, err := svc.Create(context.Background(), 1, "token", []string{"tenant-1"}, tt.expiresAt)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Create() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if created.ExpiresAt == nil {
				t.Fatal("Create() ExpiresAt is nil")
			}
			got := time.Until(*created.ExpiresAt)
			if diff := got - tt.wantExpiry; diff < -time.Minute || diff > time.Minute {
				t.Errorf("Create() expiry in %v, want ~%v", got, tt.wantExpiry)
			}
		})
	}
}

func TestCreateErrors(t *testing.T) {
	tests := []struct {
		name      string
		getByIDFn func(ctx context.Context, userID uint) (*domain.User, error)
		createFn  func(ctx context.Context, token *Token, digest string) error
		wantErr   error
	}{
		{
			name: "unknown user",
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return nil, nil
			},
			wantErr: ErrUserNotFound,
		},
		{
			name: "user lookup error",
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return nil, errors.New("database error")
			},
		},
		{
			name: "repository error",
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return activeUser(userID), nil
			},
			createFn: func(ctx context.Context, token *Token, digest string) error {
				return errors.New("insert failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{createFn: tt.createFn}
			users := &mockUserStore{getByIDFn: tt.getByIDFn}
			svc := newTestService(repo, users)

			_, err := svc.Create(context.Background(), 1, "token", []string{"tenant-1"}, nil)
			if err == nil {
				t.Fatal("Create() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Create() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	plaintext := "pat_dGVzdHRva2VuLXRlc3R0b2tlbi10ZXN0dG9rZW4"
	sum := sha256.Sum256([]byte(plaintext))
	digest := hex.EncodeToString(sum[:])
	validExpiry := time.Now().UTC().Add(time.Hour)
	expired := time.Now().UTC().Add(-time.Hour)
	revoked := time.Now().UTC().Add(-time.Minute)

	tests := []struct {
		name          string
		getByDigestFn func(ctx context.Context, digest string) (*Token, error)
		getByIDFn     func(ctx context.Context, userID uint) (*domain.User, error)
		wantErr       error
		wantTenantIDs []string
	}{
		{
			name: "valid token resolves user and tenant allowlist",
			getByDigestFn: func(ctx context.Context, got string) (*Token, error) {
				if got != digest {
					t.Errorf("Validate() looked up digest %q, want %q", got, digest)
				}
				return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1", "tenant-2"}, ExpiresAt: &validExpiry}, nil
			},
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return activeUser(userID), nil
			},
			wantTenantIDs: []string{"tenant-1", "tenant-2"},
		},
		{
			name: "token without a tenant allowlist",
			getByDigestFn: func(ctx context.Context, digest string) (*Token, error) {
				return &Token{ID: 1, UserID: 42, ExpiresAt: &validExpiry}, nil
			},
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return activeUser(userID), nil
			},
			wantErr: ErrTokenUnscoped,
		},
		{
			name: "unknown token",
			getByDigestFn: func(ctx context.Context, digest string) (*Token, error) {
				return nil, nil
			},
			wantErr: ErrTokenNotFound,
		},
		{
			name: "revoked token",
			getByDigestFn: func(ctx context.Context, digest string) (*Token, error) {
				return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1"}, ExpiresAt: &validExpiry, RevokedAt: &revoked}, nil
			},
			wantErr: ErrTokenRevoked,
		},
		{
			name: "expired token",
			getByDigestFn: func(ctx context.Context, digest string) (*Token, error) {
				return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1"}, ExpiresAt: &expired}, nil
			},
			wantErr: ErrTokenExpired,
		},
		{
			name: "blocked user",
			getByDigestFn: func(ctx context.Context, digest string) (*Token, error) {
				return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1"}, ExpiresAt: &validExpiry}, nil
			},
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return &domain.User{ID: userID, IsActive: true, IsBlocked: true}, nil
			},
			wantErr: ErrUserBlocked,
		},
		{
			name: "inactive user",
			getByDigestFn: func(ctx context.Context, digest string) (*Token, error) {
				return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1"}, ExpiresAt: &validExpiry}, nil
			},
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return &domain.User{ID: userID, IsActive: false}, nil
			},
			wantErr: ErrUserInactive,
		},
		{
			name: "user no longer exists",
			getByDigestFn: func(ctx context.Context, digest string) (*Token, error) {
				return &Token{ID: 1, UserID: 42, TenantIDs: []string{"tenant-1"}, ExpiresAt: &validExpiry}, nil
			},
			getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
				return nil, nil
			},
			wantErr: ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{getByDigestFn: tt.getByDigestFn}
			users := &mockUserStore{getByIDFn: tt.getByIDFn}
			svc := newTestService(repo, users)

			identity, err := svc.Validate(context.Background(), plaintext)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if identity.TokenID != 1 {
				t.Errorf("Validate() TokenID = %d, want 1", identity.TokenID)
			}
			if identity.User == nil || identity.User.ID != 42 {
				t.Errorf("Validate() user = %+v, want ID 42", identity.User)
			}
			if !slices.Equal(identity.TenantIDs, tt.wantTenantIDs) {
				t.Errorf("Validate() TenantIDs = %v, want %v", identity.TenantIDs, tt.wantTenantIDs)
			}
		})
	}
}

func TestValidateRoundTrip(t *testing.T) {
	var storedDigest string
	repo := &mockRepository{
		createFn: func(ctx context.Context, token *Token, digest string) error {
			storedDigest = digest
			return nil
		},
	}
	users := &mockUserStore{
		getByIDFn: func(ctx context.Context, userID uint) (*domain.User, error) {
			return activeUser(userID), nil
		},
	}
	svc := newTestService(repo, users)

	created, err := svc.Create(context.Background(), 1, "token", []string{"tenant-1"}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	repo.getByDigestFn = func(ctx context.Context, digest string) (*Token, error) {
		if digest == storedDigest {
			return &created.Token, nil
		}
		return nil, nil
	}

	identity, err := svc.Validate(context.Background(), created.Plaintext)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if identity.User.ID != 1 {
		t.Errorf("Validate() user ID = %d, want 1", identity.User.ID)
	}
	if identity.TokenID != created.ID {
		t.Errorf("Validate() TokenID = %d, want %d", identity.TokenID, created.ID)
	}
}

func TestRevoke(t *testing.T) {
	tests := []struct {
		name     string
		id       uint
		revokeFn func(ctx context.Context, id uint, revokedAt time.Time) error
		wantErr  error
	}{
		{
			name: "successful revoke",
			id:   3,
			revokeFn: func(ctx context.Context, id uint, revokedAt time.Time) error {
				if id != 3 {
					t.Errorf("Revoke() id = %d, want 3", id)
				}
				if revokedAt.IsZero() {
					t.Error("Revoke() revokedAt is zero")
				}
				return nil
			},
		},
		{
			name: "token not found",
			id:   99,
			revokeFn: func(ctx context.Context, id uint, revokedAt time.Time) error {
				return ErrTokenNotFound
			},
			wantErr: ErrTokenNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{revokeFn: tt.revokeFn}
			svc := newTestService(repo, &mockUserStore{})

			err := svc.Revoke(context.Background(), tt.id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Revoke() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("Revoke() error = %v", err)
			}
		})
	}
}

func TestList(t *testing.T) {
	repo := &mockRepository{
		listFn: func(ctx context.Context) ([]*Token, error) {
			return []*Token{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}, nil
		},
	}
	svc := newTestService(repo, &mockUserStore{})

	tokens, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("List() count = %d, want 2", len(tokens))
	}
}
