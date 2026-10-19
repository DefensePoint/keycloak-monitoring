// Package postgres provides PostgreSQL implementation of users repository interface.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

const (
	sqlWhereSubject  = "subject = ?"
	errUserNotFound  = "user not found: %s"
	errFailedGetUser = "failed to get user: %w"
)

// Repository implements users.Repository using GORM.
type Repository struct {
	db *gorm.DB

	// linkByVerifiedEmail lets a login with an unknown subject adopt the
	// account holding its verified address. Off unless configured: see
	// adoptByVerifiedEmail.
	linkByVerifiedEmail bool
}

// Option configures a Repository.
type Option func(*Repository)

// WithLinkByVerifiedEmail turns linking a login to an existing account by
// verified email on or off. Meant for a planned realm rebuild or migration and
// switched off again afterwards; see adoptByVerifiedEmail.
func WithLinkByVerifiedEmail(on bool) Option {
	return func(r *Repository) { r.linkByVerifiedEmail = on }
}

// NewRepository creates a new users Repository.
func NewRepository(db *gorm.DB, opts ...Option) users.Repository {
	r := &Repository{db: db}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// FindOrCreateBySubject finds a user by subject (OAuth2 sub claim) or creates a new one.
// refuseIfDeleted reports users.ErrUserDeleted when this login belongs to an
// account that was deleted.
//
// Matched on subject and on email, because the two identify a returning person
// in different situations: the subject is stable while the identity provider is,
// and the email survives a realm being rebuilt, which is the case that hands a
// familiar person a brand new subject.
//
// Blank values match nobody. Every one of these columns is legitimately empty
// for some kind of account — Keycloak users often have no email — so matching on
// one would refuse every future login of that shape on behalf of a single
// deleted row.
//
// Unscoped, since the whole point is to see what ordinary queries hide.
func (r *Repository) refuseIfDeleted(ctx context.Context, candidate *database.User) error {
	q := r.db.WithContext(ctx).Unscoped().Model(&database.User{}).
		Where("deleted_at IS NOT NULL")

	switch {
	case candidate.Subject != "" && candidate.Email != "":
		q = q.Where("subject = ? OR email = ?", candidate.Subject, candidate.Email)
	case candidate.Subject != "":
		q = q.Where("subject = ?", candidate.Subject)
	case candidate.Email != "":
		q = q.Where("email = ?", candidate.Email)
	default:
		// Nothing identifying to match on; the create path takes over.
		return nil
	}

	var deleted database.User
	err := q.Select("id").First(&deleted).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check for a deleted account: %w", err)
	}
	return &users.DeletedAccountError{UserID: deleted.ID}
}

func (r *Repository) FindOrCreateBySubject(ctx context.Context, user *domain.User) (*domain.User, error) {
	dbUser := toDBUser(user)
	var existingUser database.User

	// Try to find existing user by subject
	result := r.db.WithContext(ctx).Where(sqlWhereSubject, dbUser.Subject).First(&existingUser)

	switch {
	case result.Error == nil:
		// Matched on subject: the ordinary case.

	case errors.Is(result.Error, gorm.ErrRecordNotFound):
		// Nobody live matches, but a deleted account might. Refuse rather than
		// create a second one for the same person: see users.ErrUserDeleted.
		if err := r.refuseIfDeleted(ctx, dbUser); err != nil {
			return nil, err
		}

		// An unfamiliar subject can still be a familiar person.
		adopted, err := r.adoptByVerifiedEmail(ctx, dbUser, &existingUser)
		if err != nil {
			return nil, err
		}
		if !adopted {
			// Not adopted, so an account already holding the address belongs
			// to somebody this login has not been shown to be. Refuse by name
			// rather than leave it to the unique index on email.
			if err := r.refuseIfEmailInUse(ctx, dbUser); err != nil {
				return nil, err
			}
			err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(dbUser).Error; err != nil {
					return fmt.Errorf("failed to create user: %w", err)
				}
				// GORM inserts the default:true tag in place of a false
				// is_active, and writes it back into dbUser too.
				res := tx.Model(&database.User{}).Where("id = ?", dbUser.ID).
					UpdateColumn("is_active", user.IsActive)
				if res.Error != nil {
					return fmt.Errorf("failed to set is_active on new user: %w", res.Error)
				}
				if res.RowsAffected != 1 {
					return fmt.Errorf("failed to set is_active on new user: %d rows affected, want 1", res.RowsAffected)
				}
				dbUser.IsActive = user.IsActive
				return nil
			})
			if err != nil {
				return nil, err
			}
			return toDomainUser(dbUser), nil
		}
		// existingUser now holds the account this login belongs to, so the
		// update below runs for it exactly as for a subject match: a relinked
		// login refreshes the profile like any other.

	default:
		return nil, fmt.Errorf("failed to query user: %w", result.Error)
	}

	// User exists, update their information
	updates := map[string]interface{}{
		// Carried because adoption above may have changed it. For a subject
		// match this writes back the value it just matched on.
		"subject":            dbUser.Subject,
		"email":              dbUser.Email,
		"email_verified":     dbUser.EmailVerified,
		"name":               dbUser.Name,
		"given_name":         dbUser.GivenName,
		"family_name":        dbUser.FamilyName,
		"preferred_username": dbUser.PreferredUsername,
		"locale":             dbUser.Locale,
		// The caller refuses an inactive or blocked account only after this
		// returns, so a sign-in it will refuse still arrives here. It must not
		// count as a sign-in: the MCP token check reads last_login_at as the
		// last sign-in that let the user in.
		"last_login_at":    onlyIfUsable("last_login_at", dbUser.LastLoginAt),
		"last_login_ip":    onlyIfUsable("last_login_ip", dbUser.LastLoginIP),
		"login_count":      gorm.Expr("login_count + 1"),
		"last_accessed_at": dbUser.LastAccessedAt,
	}
	// Written only when the caller set it. auth_method is what keeps an SSO
	// account's identity fields and password out of local editing, so a caller
	// that leaves it out must not blank it and switch that guard off.
	if dbUser.AuthMethod != "" {
		updates["auth_method"] = dbUser.AuthMethod
	}

	if err := r.db.WithContext(ctx).Model(&existingUser).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	// Reload user to get updated values
	if err := r.db.WithContext(ctx).Where(sqlWhereSubject, dbUser.Subject).First(&existingUser).Error; err != nil {
		return nil, fmt.Errorf("failed to reload user: %w", err)
	}

	return toDomainUser(&existingUser), nil
}

// onlyIfUsable sets column to value when the row is active and not blocked,
// and keeps it otherwise. The check runs in the UPDATE itself, against the
// row as it is when written.
func onlyIfUsable(column string, value any) clause.Expr {
	return gorm.Expr("CASE WHEN is_active AND NOT is_blocked THEN ? ELSE "+column+" END", value)
}

// adoptByVerifiedEmail finds the account this login belongs to when the subject
// is new but the person is not, and loads it into existing.
//
// An identity provider's subject is stable only while the provider is. Rebuild
// a realm, migrate to a new one, or re-provision a user, and the same person
// arrives with a new sub claim. Without this they become a second account:
// their roles, tenant policies and history stay with a row nobody can reach any
// more, and an administrator has to notice and merge them by hand.
//
// The address is what identifies them across that change, so this matches on
// email — but only on an address the identity provider says it has verified.
// That condition is the whole safety of it. An unverified address is a claim
// the person made about themselves, and honouring it would let anyone who can
// register an address in the realm inherit whatever account already holds it.
// Until recently every account read as verified whether or not anyone had
// checked, which is why this could not be built before that was fixed.
//
// Restricted further to accounts that hold no local password. Adopting a
// username-and-password account would let an SSO login take over local
// credentials, which is a different decision from the one this implements and a
// wider trust boundary than a realm rebuild needs. Those credentials also carry
// the way back in when the identity provider is unreachable, which is exactly
// when nobody can afford to have lost them.
//
// Tested on the password rather than on auth_method. auth_method is a label
// that can lag behind the account: rows written before the column existed, or
// with it left unset, read "" or the "simple" default until the boot-time
// migration repairs them. The password is the thing that would actually be
// taken over, so it is the thing to ask about.
//
// At most one account can match: the unique index on email covers live rows
// carrying an address, so there is no ambiguity to resolve.
//
// Off unless configured. A verified address shows the provider checked it, not
// that it is the only identity holding it: a realm that allows duplicate
// emails, an identity broker trusted to vouch for addresses, or anyone able to
// create realm users and mark them verified can produce a second identity with
// the same address, and adoption would hand it the account and every role it
// carries. The login cannot tell that from a rebuild, so it is an
// administrator's call, made for a planned migration and undone after it.
func (r *Repository) adoptByVerifiedEmail(ctx context.Context, candidate *database.User, existing *database.User) (bool, error) {
	if !r.linkByVerifiedEmail {
		return false, nil
	}
	if !candidate.EmailVerified || candidate.Email == "" || candidate.Subject == "" {
		return false, nil
	}

	err := r.db.WithContext(ctx).
		Where("email = ? AND (password_hash IS NULL OR password_hash = '')", candidate.Email).
		First(existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to look for an account with this email: %w", err)
	}

	return true, nil
}

// refuseIfEmailInUse reports users.ErrEmailInUse when a live account already
// holds the address of a login that was not adopted. Without it the insert
// fails on the unique index anyway, but as a bare constraint violation that
// tells an administrator nothing about which account or why.
//
// A blank address matches nobody, as everywhere else here. A concurrent login
// can still create the row between this check and the insert; the unique index
// then refuses that insert, so the outcome is the same, only less well named.
func (r *Repository) refuseIfEmailInUse(ctx context.Context, candidate *database.User) error {
	if candidate.Email == "" {
		return nil
	}

	var holder database.User
	err := r.db.WithContext(ctx).Select("id").Where("email = ?", candidate.Email).First(&holder).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to look for an account with this email: %w", err)
	}
	return &users.EmailInUseError{UserID: holder.ID}
}

// GetBySubject retrieves a user by their subject (OAuth2 sub claim).
func (r *Repository) GetBySubject(ctx context.Context, subject string) (*domain.User, error) {
	var user database.User
	result := r.db.WithContext(ctx).Where(sqlWhereSubject, subject).First(&user)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf(errUserNotFound, subject)
	}
	if result.Error != nil {
		return nil, fmt.Errorf(errFailedGetUser, result.Error)
	}

	return toDomainUser(&user), nil
}

// GetByEmail retrieves a user by their email address.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user database.User
	result := r.db.WithContext(ctx).Where("email = ?", email).First(&user)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf(errUserNotFound, email)
	}
	if result.Error != nil {
		return nil, fmt.Errorf(errFailedGetUser, result.Error)
	}

	return toDomainUser(&user), nil
}

// GetByUsername retrieves a user by their username (for simple auth).
func (r *Repository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user database.User
	result := r.db.WithContext(ctx).Where("username = ?", username).First(&user)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return toDomainUser(&user), nil
}

// GetByID retrieves a user by their ID.
func (r *Repository) GetByID(ctx context.Context, userID uint) (*domain.User, error) {
	var dbUser database.User
	result := r.db.WithContext(ctx).First(&dbUser, userID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", result.Error)
	}

	return toDomainUser(&dbUser), nil
}

// ListUsers retrieves users with pagination.
func (r *Repository) ListUsers(ctx context.Context, opts *httputil.ListOptions) ([]*domain.User, error) {
	if opts == nil {
		opts = &httputil.ListOptions{Limit: 100, Offset: 0}
	}

	var users []*database.User
	result := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset).
		Find(&users)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to query users: %w", result.Error)
	}

	// Convert database users to domain users
	domainUsers := make([]*domain.User, len(users))
	for i, user := range users {
		domainUsers[i] = toDomainUser(user)
	}

	return domainUsers, nil
}

// CountUsers returns the total number of users.
func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).Model(&database.User{}).Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count users: %w", result.Error)
	}

	return count, nil
}

// CreateSimpleAuthUser creates a new user with username/password authentication.
func (r *Repository) CreateSimpleAuthUser(ctx context.Context, user *domain.User) error {
	dbUser := toDBUser(user)
	result := r.db.WithContext(ctx).Create(dbUser)
	if result.Error != nil {
		return fmt.Errorf("failed to create user: %w", result.Error)
	}
	// Update the domain user with the created ID
	user.ID = dbUser.ID
	return nil
}

// Update writes the fields an admin can edit on an existing user.
//
// is_active and is_blocked are always written. Updates with a struct skips
// zero values, so deactivating or unblocking a user used to be silently
// ignored while the caller reported success. The identity fields keep their
// existing rule, written only when non-empty, so an empty value still leaves
// them unchanged. Nothing else is written back: the caller read this user a
// moment ago, and rewriting columns such as login_count or last_login_at
// would undo a login that landed in between.
//
// Any change to is_active or is_blocked, cutting the user off or restoring
// them, revokes their live API tokens in the same transaction; see
// revokeAPITokens.
func (r *Repository) Update(ctx context.Context, user *domain.User) error {
	updates := map[string]any{
		"is_active":  user.IsActive,
		"is_blocked": user.IsBlocked,
	}
	for column, value := range map[string]string{
		"email":              user.Email,
		"name":               user.Name,
		"username":           user.Username,
		"preferred_username": user.PreferredUsername,
		"subject":            user.Subject,
		"password_hash":      user.PasswordHash,
	} {
		if value != "" {
			updates[column] = value
		}
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before, err := lockStatus(tx, "id = ?", user.ID)
		if err != nil {
			return err
		}

		result := tx.Model(&database.User{}).Where("id = ?", user.ID).Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("failed to update user: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("user not found")
		}

		if before.IsActive != user.IsActive || before.IsBlocked != user.IsBlocked {
			return revokeAPITokens(tx, user.ID)
		}
		return nil
	})
}

// errNoSuchUser is what lockStatus returns when no user matches.
var errNoSuchUser = errors.New("user not found")

// userStatus is the part of a user row that decides whether a change to it
// revokes the user's API tokens.
type userStatus struct {
	ID        uint
	IsActive  bool
	IsBlocked bool
}

// lockStatus reads the matching user's status and locks the row until the
// transaction ends, so a concurrent change cannot slip between the read and
// the write that depends on it.
func lockStatus(tx *gorm.DB, query string, arg any) (userStatus, error) {
	var row userStatus
	err := tx.Model(&database.User{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "is_active", "is_blocked").
		Where(query, arg).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return userStatus{}, errNoSuchUser
	}
	if err != nil {
		return userStatus{}, fmt.Errorf("failed to read user: %w", err)
	}
	return row, nil
}

// revokeAPITokens revokes every live API token of a user who is being
// deactivated, blocked, reactivated or unblocked.
//
// Validation refuses a token while its user is inactive or blocked, but that
// alone meant reactivating or unblocking the user brought every unexpired
// token back. Deactivating and blocking are how an account is cut off, for
// offboarding, misuse or a suspected compromise, so the tokens end with it:
// reactivating or unblocking restores the account, not its tokens, and the
// user issues new ones. Expired tokens are left as they are, so they still
// read as expired rather than revoked.
//
// Both ends revoke, because either alone leaves a way back. Revoking at the
// cut-off ends the tokens the user held then. Revoking at the restore ends
// any that are still live: those of a user cut off by a release that did not
// revoke, during a rollback or by an older replica in a rolling deploy, and
// any issued to the user while cut off. A restored user always starts with no
// live tokens.
//
// It runs in the transaction that changes the status: a change that committed
// while its revocation failed would leave the tokens to come back later.
func revokeAPITokens(tx *gorm.DB, userID uint) error {
	now := time.Now().UTC()
	err := tx.Model(&database.APIToken{}).
		Where("user_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", userID, now).
		Update("revoked_at", now).Error
	if err != nil {
		return fmt.Errorf("failed to revoke the user's api tokens: %w", err)
	}
	return nil
}

// Delete deletes a user (soft delete by marking as inactive).
// Delete deactivates the account rather than deleting anything.
//
// The name is the interface's, kept so this stays a drop-in implementation,
// but it writes is_active and nothing else: no row is removed, deleted_at is
// never set, and the roles, policies and history all stay. Nothing anywhere in
// the platform sets deleted_at on a user, so an account is only ever
// deactivated, and reactivating it restores everything but its API tokens.
//
// Deactivating an active account revokes its API tokens in the same
// transaction; see revokeAPITokens.
//
// It does not end a session that already exists. The middleware re-reads the
// account on every request but only to recover the ID that RBAC needs, and the
// struct it fills in carries no status field, so is_active is never consulted
// once somebody holds a session: access continues until that session expires,
// up to session max_age, 24 hours by default. API tokens are revoked, and stop
// working immediately.
func (r *Repository) Delete(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before, err := lockStatus(tx, "id = ?", userID)
		if err != nil {
			return err
		}

		result := tx.Model(&database.User{}).Where("id = ?", userID).Update("is_active", false)
		if result.Error != nil {
			return fmt.Errorf("failed to delete user: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("user not found")
		}

		if before.IsActive {
			return revokeAPITokens(tx, userID)
		}
		return nil
	})
}

// UpdateLastAccessed updates the last accessed timestamp for a user.
func (r *Repository) UpdateLastAccessed(ctx context.Context, subject string) error {
	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where(sqlWhereSubject, subject).
		Update("last_accessed_at", gorm.Expr("NOW()"))

	if result.Error != nil {
		return fmt.Errorf("failed to update last accessed: %w", result.Error)
	}

	return nil
}

// UpdatePassword updates a user's password hash.
func (r *Repository) UpdatePassword(ctx context.Context, userID uint, passwordHash string) error {
	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where("id = ?", userID).
		Update("password_hash", passwordHash)

	if result.Error != nil {
		return fmt.Errorf("failed to update password: %w", result.Error)
	}

	return nil
}

// UpdatePasswordWithFlags updates a user's password hash, timestamp, and must change password flag.
func (r *Repository) UpdatePasswordWithFlags(ctx context.Context, userID uint, passwordHash string, passwordChangedAt *time.Time, mustChangePassword bool) error {
	updates := map[string]interface{}{
		"password_hash":        passwordHash,
		"password_changed_at":  passwordChangedAt,
		"must_change_password": mustChangePassword,
	}

	result := r.db.WithContext(ctx).
		Model(&database.User{}).
		Where("id = ?", userID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update password: %w", result.Error)
	}

	return nil
}

// BlockUser blocks a user account.
//
// Blocking an unblocked account revokes its API tokens in the same
// transaction, as blocking through Update does; see revokeAPITokens. Anyone
// who can make this run can therefore end a user's tokens, so it must stay
// behind an administrator's decision and never become something an outside
// party can trigger, such as a lockout after failed sign-ins. Blocking a
// subject that matches no user does nothing.
func (r *Repository) BlockUser(ctx context.Context, subject string, reason string) error {
	updates := map[string]interface{}{
		"is_blocked":     true,
		"blocked_at":     gorm.Expr("NOW()"),
		"blocked_reason": reason,
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before, err := lockStatus(tx, sqlWhereSubject, subject)
		if errors.Is(err, errNoSuchUser) {
			return nil
		}
		if err != nil {
			return err
		}

		if err := tx.Model(&database.User{}).Where("id = ?", before.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to block user: %w", err)
		}

		if !before.IsBlocked {
			return revokeAPITokens(tx, before.ID)
		}
		return nil
	})
}

// UnblockUser unblocks a user account.
//
// Unblocking a blocked account revokes any API tokens it still holds, in the
// same transaction, as unblocking through Update does; see revokeAPITokens.
// Unblocking a subject that matches no user does nothing.
func (r *Repository) UnblockUser(ctx context.Context, subject string) error {
	updates := map[string]interface{}{
		"is_blocked":     false,
		"blocked_at":     nil,
		"blocked_reason": "",
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before, err := lockStatus(tx, sqlWhereSubject, subject)
		if errors.Is(err, errNoSuchUser) {
			return nil
		}
		if err != nil {
			return err
		}

		if err := tx.Model(&database.User{}).Where("id = ?", before.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to unblock user: %w", err)
		}

		if before.IsBlocked {
			return revokeAPITokens(tx, before.ID)
		}
		return nil
	})
}

// toDomainUser converts a database User to a domain User.
func toDomainUser(user *database.User) *domain.User {
	if user == nil {
		return nil
	}

	// Convert LastAccessedAt from time.Time to *time.Time
	var lastAccessedAt *time.Time
	if !user.LastAccessedAt.IsZero() {
		lastAccessedAt = &user.LastAccessedAt
	}

	return &domain.User{
		ID:                 user.ID,
		Subject:            user.Subject,
		Email:              user.Email,
		EmailVerified:      user.EmailVerified,
		Name:               user.Name,
		GivenName:          user.GivenName,
		FamilyName:         user.FamilyName,
		PreferredUsername:  user.PreferredUsername,
		Locale:             user.Locale,
		Username:           user.Username,
		PasswordHash:       user.PasswordHash,
		AuthMethod:         domain.AuthMethod(user.AuthMethod),
		MustChangePassword: user.MustChangePassword,
		PasswordChangedAt:  user.PasswordChangedAt,
		IsActive:           user.IsActive,
		IsBlocked:          user.IsBlocked,
		BlockedReason:      user.BlockedReason,
		BlockedAt:          user.BlockedAt,
		SessionEpoch:       user.SessionEpoch,
		LastLoginAt:        user.LastLoginAt,
		LastLoginIP:        user.LastLoginIP,
		LastAccessedAt:     lastAccessedAt,
		LoginCount:         user.LoginCount,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}
}

// toDBUser converts a domain User to a database User.
func toDBUser(user *domain.User) *database.User {
	if user == nil {
		return nil
	}

	// Convert LastAccessedAt from *time.Time to time.Time
	var lastAccessedAt time.Time
	if user.LastAccessedAt != nil {
		lastAccessedAt = *user.LastAccessedAt
	}

	return &database.User{
		ID:                user.ID,
		Subject:           user.Subject,
		Email:             user.Email,
		EmailVerified:     user.EmailVerified,
		Name:              user.Name,
		GivenName:         user.GivenName,
		FamilyName:        user.FamilyName,
		PreferredUsername: user.PreferredUsername,
		Locale:            user.Locale,
		Username:          user.Username,
		PasswordHash:      user.PasswordHash,
		AuthMethod:        string(user.AuthMethod),
		IsActive:          user.IsActive,
		IsBlocked:         user.IsBlocked,
		BlockedReason:     user.BlockedReason,
		BlockedAt:         user.BlockedAt,
		SessionEpoch:      user.SessionEpoch,
		LastLoginAt:       user.LastLoginAt,
		LastLoginIP:       user.LastLoginIP,
		LastAccessedAt:    lastAccessedAt,
		LoginCount:        user.LoginCount,
		CreatedAt:         user.CreatedAt,
		UpdatedAt:         user.UpdatedAt,
	}
}

// CurrentSessionEpoch returns the user's session epoch, which a login stores
// in the session cookie.
func (r *Repository) CurrentSessionEpoch(ctx context.Context, subject string) (int64, error) {
	var epoch int64
	err := r.db.WithContext(ctx).Model(&database.User{}).
		Where("subject = ?", subject).
		Select("session_epoch").Scan(&epoch).Error
	if err != nil {
		return 0, fmt.Errorf("failed to read session epoch: %w", err)
	}
	return epoch, nil
}

// RevokeSessions records a logout for the user with subject: it increments
// their session epoch, so every session carrying the old value is rejected.
// It applies only when the session logging out carries the current value, so
// an old cookie that is already rejected cannot be replayed at logout to end
// the user's newer sessions. It reports whether the epoch moved.
func (r *Repository) RevokeSessions(ctx context.Context, subject string, sessionEpoch int64) (bool, error) {
	result := r.db.WithContext(ctx).Model(&database.User{}).
		Where("subject = ? AND session_epoch = ?", subject, sessionEpoch).
		Update("session_epoch", gorm.Expr("session_epoch + 1"))
	if result.Error != nil {
		return false, fmt.Errorf("failed to revoke sessions: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}
