package fx

import (
	"context"
	"net/http"

	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
	"github.com/DefensePoint/keycloak-monitoring/internal/operator"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/reports"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

// HandlersModule provides HTTP handlers
var HandlersModule = fx.Module("handlers",
	fx.Provide(
		provideKeycloakHandlers,
		provideAlertsHandlers,
		provideOperatorHandlers,
		provideRBACHandlers,
		provideAPITokenHandlers,
		provideUserHandlers,
		provideTenantHandlers,
		provideReportHandlers,
		provideAuthHandlers,
		provideSystemHandlers,
		provideRBACMiddleware,
		provideRouter,
	),
)

// KeycloakHandlersParams contains dependencies for keycloak handlers.
type KeycloakHandlersParams struct {
	fx.In

	Service        keycloak.Service
	RBACService    rbac.Service
	TenantService  tenant.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
}

func provideKeycloakHandlers(p KeycloakHandlersParams) *chihttp.KeycloakHandlers {
	return chihttp.NewKeycloakHandlers(
		p.Service,
		&rbacCheckerAdapter{p.RBACService},
		&keycloakTenantAdapter{p.TenantService},
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

// AlertsHandlersParams contains dependencies for alerts handlers.
type AlertsHandlersParams struct {
	fx.In

	Service             alerts.Service
	RBACService         rbac.Service
	OperatorService     operator.Service      `optional:"true"`
	NotificationService notifications.Service `optional:"true"`
	Logger              *logger.Logger
	AuthMiddleware      *chihttp.AuthMiddleware
	RBACMiddleware      *chihttp.RBACMiddleware
}

func provideAlertsHandlers(p AlertsHandlersParams) *chihttp.AlertsHandlers {
	var opSvc chihttp.AlertsOperatorService
	if p.OperatorService != nil {
		opSvc = &alertsOperatorAdapter{p.OperatorService}
	}

	var notifSvc chihttp.AlertsNotificationService
	if p.NotificationService != nil {
		notifSvc = &alertsNotificationAdapter{p.NotificationService}
	}

	return chihttp.NewAlertsHandlers(
		p.Service,
		&rbacCheckerAdapter{p.RBACService},
		opSvc,
		notifSvc,
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

// OperatorHandlersParams contains dependencies for operator handlers.
type OperatorHandlersParams struct {
	fx.In

	Service        operator.Service
	RBACService    rbac.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
}

func provideOperatorHandlers(p OperatorHandlersParams) *chihttp.OperatorHandlers {
	return chihttp.NewOperatorHandlers(
		p.Service,
		&rbacCheckerAdapter{p.RBACService},
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

// RBACHandlersParams contains dependencies for RBAC handlers.
type RBACHandlersParams struct {
	fx.In

	Service        rbac.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
}

func provideRBACHandlers(p RBACHandlersParams) *chihttp.RBACHandlers {
	return chihttp.NewRBACHandlers(
		p.Service,
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

// APITokenHandlersParams contains dependencies for API token handlers.
type APITokenHandlersParams struct {
	fx.In

	Service        apitoken.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
}

func provideAPITokenHandlers(p APITokenHandlersParams) *chihttp.APITokenHandlers {
	return chihttp.NewAPITokenHandlers(
		p.Service,
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

// UserHandlersParams contains dependencies for user handlers.
type UserHandlersParams struct {
	fx.In

	UserRepo       users.Repository
	RBACService    rbac.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
}

func provideUserHandlers(p UserHandlersParams) *chihttp.UserHandlers {
	return chihttp.NewUserHandlers(
		&userServiceAdapter{p.UserRepo},
		&rbacCheckerAdapter{p.RBACService},
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

// TenantHandlersParams contains dependencies for tenant handlers.
type TenantHandlersParams struct {
	fx.In

	Service          tenant.Service
	RBACService      rbac.Service
	ConnectionTester chihttp.KeycloakConnectionTester
	Logger           *logger.Logger
	AuthMiddleware   *chihttp.AuthMiddleware
	RBACMiddleware   *chihttp.RBACMiddleware

	// Sub-routers that register routes under /api/tenants/{tenantID}
	KeycloakHandlers *chihttp.KeycloakHandlers
	AlertsHandlers   *chihttp.AlertsHandlers
	AmfaHandlers     *chihttp.AmfaHandlers
	OperatorHandlers *chihttp.OperatorHandlers
	ReportHandlers   *chihttp.ReportHandlers
	SystemHandlers   *chihttp.SystemHandlers
}

func provideTenantHandlers(p TenantHandlersParams) *chihttp.TenantHandlers {
	return chihttp.NewTenantHandlers(
		p.Service,
		&rbacCheckerAdapter{p.RBACService},
		p.ConnectionTester,
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
		// Pass all tenant sub-routers
		p.KeycloakHandlers,
		p.AlertsHandlers,
		p.AmfaHandlers,
		p.OperatorHandlers,
		p.ReportHandlers,
		p.SystemHandlers,
	)
}

// ReportHandlersParams contains dependencies for report handlers.
type ReportHandlersParams struct {
	fx.In

	Service        reports.Service
	TenantService  tenant.Service
	RBACService    rbac.Service
	Logger         *logger.Logger
	AuthMiddleware *chihttp.AuthMiddleware
	RBACMiddleware *chihttp.RBACMiddleware
}

func provideReportHandlers(p ReportHandlersParams) *chihttp.ReportHandlers {
	return chihttp.NewReportHandlers(
		p.Service,
		&reportTenantAdapter{p.TenantService},
		&rbacCheckerAdapter{p.RBACService},
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

// SystemHandlersParams contains dependencies for system handlers.
type SystemHandlersParams struct {
	fx.In

	EventRepo       events.Repository
	TenantService   tenant.Service
	KeycloakService keycloak.Service
	RBACService     rbac.Service
	Logger          *logger.Logger
	AuthMiddleware  *chihttp.AuthMiddleware
	RBACMiddleware  *chihttp.RBACMiddleware
}

func provideSystemHandlers(p SystemHandlersParams) *chihttp.SystemHandlers {
	return chihttp.NewSystemHandlers(
		&eventRepoAdapter{p.EventRepo},
		p.TenantService,
		p.KeycloakService,
		&rbacCheckerAdapter{p.RBACService},
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
	)
}

func provideRBACMiddleware(service rbac.Service, log *logger.Logger) *chihttp.RBACMiddleware {
	return chihttp.NewRBACMiddleware(&rbacCheckerAdapter{service}, log)
}

// AuthHandlersParams contains dependencies for auth handlers.
type AuthHandlersParams struct {
	fx.In

	Service          auth.Service             `optional:"true"`
	SimpleAuth       auth.SimpleAuthService   `optional:"true"`
	OAuth2AdminSvc   *auth.OAuth2AdminService `optional:"true"`
	Provider         auth.Provider            `optional:"true"`
	SessionManager   *auth.CookieSessionManager
	StateStore       *auth.StateStore
	SessionBlacklist *auth.SessionBlacklist
	Config           *config.AppConfig
	Logger           *logger.Logger
	AuthMiddleware   *chihttp.AuthMiddleware
}

func provideAuthHandlers(p AuthHandlersParams) *chihttp.AuthHandlers {
	// Check config for enabled auth methods
	simpleEnabled := p.Config.Auth.Simple.Enabled
	oauth2Enabled := p.Config.Auth.OAuth2.Enabled

	// SimpleAuth already implements chihttp.SimpleAuthService via auth.SimpleAuthService
	simpleAuth := ProvideSimpleAuthAdapter(p.SimpleAuth)

	// OAuth2AdminService can be nil if not configured
	var oauth2AdminSvc chihttp.OAuth2AdminService
	if p.OAuth2AdminSvc != nil {
		oauth2AdminSvc = p.OAuth2AdminSvc
	}

	// Create session store adapter from session manager
	var sessionStore chihttp.SessionStore
	sessionName := p.Config.Auth.Session.Name
	if p.SessionManager != nil {
		sessionStore = chihttp.NewSessionStoreAdapter(p.SessionManager)
	}

	handlers := chihttp.NewAuthHandlers(
		p.Service,
		simpleAuth,
		oauth2AdminSvc,
		p.Provider,
		sessionStore,
		sessionName,
		p.Logger,
		simpleEnabled,
		oauth2Enabled,
		p.StateStore,
		p.AuthMiddleware.Authenticate,
	)

	// Set session blacklist for logout revocation (security fix)
	if p.SessionBlacklist != nil {
		handlers.SetSessionBlacklist(p.SessionBlacklist)
	}

	return handlers
}

// RouterParams contains all dependencies for the router.
type RouterParams struct {
	fx.In

	Logger           *logger.Logger
	AuthMiddleware   *chihttp.AuthMiddleware
	RBACMiddleware   *chihttp.RBACMiddleware
	RBACHandlers     *chihttp.RBACHandlers
	APITokenHandlers *chihttp.APITokenHandlers
	UserHandlers     *chihttp.UserHandlers
	AuthHandlers     *chihttp.AuthHandlers
	SystemHandlers   *chihttp.SystemHandlers
	TenantHandlers   *chihttp.TenantHandlers
	KeycloakHandlers *chihttp.KeycloakHandlers
	AlertsHandlers   *chihttp.AlertsHandlers
	AmfaHandlers     *chihttp.AmfaHandlers
	OperatorHandlers *chihttp.OperatorHandlers
	ReportHandlers   *chihttp.ReportHandlers
}

func provideRouter(p RouterParams) *chihttp.Router {
	return chihttp.NewRouter(
		p.Logger,
		p.AuthMiddleware.Authenticate,
		p.RBACMiddleware,
		p.RBACHandlers,
		p.APITokenHandlers,
		p.UserHandlers,
		p.AuthHandlers,
		p.SystemHandlers,
		p.TenantHandlers,
		p.KeycloakHandlers,
		p.AlertsHandlers,
		p.AmfaHandlers,
		p.OperatorHandlers,
		p.ReportHandlers,
	)
}

// Adapters to satisfy handler interfaces

// rbacCheckerAdapter is a single adapter that implements chihttp.RBACChecker.
// It adapts rbac.Service to the unified RBACChecker interface used by all handlers.
type rbacCheckerAdapter struct {
	service rbac.Service
}

func (a *rbacCheckerAdapter) HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
	return a.service.HasPermission(ctx, userID, permission, tenantID)
}

func (a *rbacCheckerAdapter) GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
	return a.service.GetUserPermissions(ctx, userID, tenantID)
}

func (a *rbacCheckerAdapter) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	return a.service.IsAdmin(ctx, userID)
}

func (a *rbacCheckerAdapter) GetUserRoles(ctx context.Context, userID uint) ([]*chihttp.UserRoleInfo, error) {
	roles, err := a.service.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]*chihttp.UserRoleInfo, len(roles))
	for i, r := range roles {
		var roleName string
		if r.Role != nil {
			roleName = r.Role.Name
		}
		result[i] = &chihttp.UserRoleInfo{
			RoleID:   r.RoleID,
			RoleName: roleName,
			TenantID: r.TenantID,
		}
	}
	return result, nil
}

func (a *rbacCheckerAdapter) HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error) {
	return a.service.HasAccessToTenant(ctx, userID, tenantID)
}

func (a *rbacCheckerAdapter) HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error) {
	return a.service.HasAccessToRealm(ctx, userID, tenantID, realmName)
}

func (a *rbacCheckerAdapter) GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error) {
	return a.service.GetUserPolicies(ctx, userID)
}

// reportTenantAdapter adapts tenant.Service to chihttp.ReportTenantService.
type reportTenantAdapter struct {
	service tenant.Service
}

func (a *reportTenantAdapter) GetTenantName(ctx context.Context, tenantID string) (string, error) {
	t, err := a.service.GetTenant(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return t.Name, nil
}

// userServiceAdapter adapts users.Repository to chihttp.UserService.
type userServiceAdapter struct {
	repo users.Repository
}

func (a *userServiceAdapter) ListUsers(ctx context.Context, opts *httputil.ListOptions) ([]*domain.User, error) {
	return a.repo.ListUsers(ctx, opts)
}

func (a *userServiceAdapter) GetByID(ctx context.Context, userID uint) (*domain.User, error) {
	return a.repo.GetByID(ctx, userID)
}

func (a *userServiceAdapter) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	return a.repo.GetByUsername(ctx, username)
}

func (a *userServiceAdapter) CreateUser(ctx context.Context, user *domain.User) error {
	return a.repo.CreateSimpleAuthUser(ctx, user)
}

func (a *userServiceAdapter) UpdateUser(ctx context.Context, user *domain.User) error {
	return a.repo.Update(ctx, user)
}

func (a *userServiceAdapter) DeleteUser(ctx context.Context, userID uint) error {
	return a.repo.Delete(ctx, userID)
}

func (a *userServiceAdapter) CountUsers(ctx context.Context) (int64, error) {
	return a.repo.CountUsers(ctx)
}

// eventRepoAdapter adapts events.Repository to chihttp.EventRepository.
type eventRepoAdapter struct {
	repo events.Repository
}

func (a *eventRepoAdapter) List(ctx context.Context, opts *events.ListOptions) ([]*domain.Event, error) {
	return a.repo.List(ctx, opts)
}

func (a *eventRepoAdapter) CountWithFilter(ctx context.Context, opts *events.ListOptions) (int64, error) {
	return a.repo.CountWithFilter(ctx, opts)
}

// alertsOperatorAdapter adapts operator.Service to chihttp.AlertsOperatorService.
type alertsOperatorAdapter struct {
	service operator.Service
}

func (a *alertsOperatorAdapter) RecordAction(ctx context.Context, action *domain.OperatorAction) error {
	return a.service.RecordAction(ctx, action)
}

func (a *alertsOperatorAdapter) GetLastActionForAlert(ctx context.Context, tenantID string, alertID uint) (*domain.OperatorAction, error) {
	return a.service.GetLastActionForAlert(ctx, tenantID, alertID)
}

// alertsNotificationAdapter adapts notifications.Service to chihttp.AlertsNotificationService.
type alertsNotificationAdapter struct {
	service notifications.Service
}

func (a *alertsNotificationAdapter) NotifyAlertResolution(ctx context.Context, alert *domain.Alert) error {
	return a.service.NotifyAlertResolution(ctx, alert)
}

// keycloakTenantAdapter adapts tenant.Service to chihttp.KeycloakTenantService.
type keycloakTenantAdapter struct {
	service tenant.Service
}

func (a *keycloakTenantAdapter) GetTenant(ctx context.Context, tenantID string) (*chihttp.KeycloakTenant, error) {
	t, err := a.service.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return &chihttp.KeycloakTenant{
		TenantID: t.TenantID,
		Name:     t.Name,
		Enabled:  t.Enabled,
	}, nil
}

// Ensure adapters implement the required interfaces
var (
	_ chihttp.RBACChecker               = (*rbacCheckerAdapter)(nil)
	_ chihttp.AlertsOperatorService     = (*alertsOperatorAdapter)(nil)
	_ chihttp.AlertsNotificationService = (*alertsNotificationAdapter)(nil)
	_ chihttp.KeycloakTenantService     = (*keycloakTenantAdapter)(nil)
	_ chihttp.ReportTenantService       = (*reportTenantAdapter)(nil)
	_ chihttp.UserService               = (*userServiceAdapter)(nil)
	_ chihttp.EventRepository           = (*eventRepoAdapter)(nil)
)

// Ensure http.Handler type is used
var _ http.Handler = (http.Handler)(nil)
