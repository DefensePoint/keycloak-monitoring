package requests

// CreateTenant represents the request to create a new tenant.
//
//	@Description	Request body for tenant creation
type CreateTenant struct {
	TenantID      string      `json:"tenant_id" validate:"required,min=1,max=100" example:"my-keycloak"`
	Name          string      `json:"name" validate:"required,min=1,max=255" example:"My Keycloak Instance"`
	Description   string      `json:"description,omitempty" validate:"omitempty,max=1000" example:"Production Keycloak server"`
	ServerURL     string      `json:"server_url" validate:"required,url" example:"https://keycloak.example.com"`
	AdminRealm    string      `json:"admin_realm" validate:"omitempty,max=100,keycloak_realm" example:"master"`
	ClientID      string      `json:"client_id" validate:"required,max=255" example:"monitoring-service"`
	ClientSecret  string      `json:"client_secret" validate:"required,max=255" example:"client-secret"`
	Configuration string      `json:"configuration,omitempty" example:"{}"`
	DefaultRealm  string      `json:"default_realm,omitempty" validate:"omitempty,max=100" example:"master"`
	Enabled       bool        `json:"enabled" example:"true"`
	IsDefault     bool        `json:"is_default,omitempty" example:"false"`
	Tags          []string    `json:"tags,omitempty" example:"production,main"`
	Owner         string      `json:"owner,omitempty" validate:"omitempty,max=255" example:"admin@example.com"`
	Amfa          *TenantAmfa `json:"amfa,omitempty"`
}

// TenantAmfa is a tenant's AMFA integration settings.
//
// The API endpoint only. Reading AMFA over HTTP needs no database credentials,
// which is what makes these settings safe to accept here.
type TenantAmfa struct {
	Enabled bool `json:"enabled" example:"true"`
	// required_if runs before omitempty: it is what makes an empty value an
	// error at all when Enabled is true, rather than silently registering a
	// tenant whose AMFA sync can never actually apply.
	APIBaseURL         string `json:"api_base_url" validate:"required_if=Enabled true,omitempty,url" example:"https://amfa.internal:8000"`
	EventsLookbackDays int    `json:"events_lookback_days,omitempty" validate:"omitempty,min=1,max=90" example:"30"`
	APITimeoutSeconds  int    `json:"api_timeout_seconds,omitempty" validate:"omitempty,min=1,max=300" example:"30"`
}

// UpdateTenant represents the request to update a tenant.
//
//	@Description	Request body for tenant update (all fields optional)
type UpdateTenant struct {
	Name          *string     `json:"name,omitempty" validate:"omitempty,min=1,max=255" example:"Updated Name"`
	Description   *string     `json:"description,omitempty" validate:"omitempty,max=1000" example:"Updated description"`
	ServerURL     *string     `json:"server_url,omitempty" validate:"omitempty,url" example:"https://new-keycloak.example.com"`
	AdminRealm    *string     `json:"admin_realm,omitempty" validate:"omitempty,max=100,keycloak_realm" example:"master"`
	ClientID      *string     `json:"client_id,omitempty" validate:"omitempty,max=255" example:"monitoring-service"`
	ClientSecret  *string     `json:"client_secret,omitempty" validate:"omitempty,max=255" example:"new-secret"`
	Configuration *string     `json:"configuration,omitempty" example:"{}"`
	DefaultRealm  *string     `json:"default_realm,omitempty" validate:"omitempty,max=100" example:"master"`
	Enabled       *bool       `json:"enabled,omitempty" example:"true"`
	IsDefault     *bool       `json:"is_default,omitempty" example:"false"`
	Tags          []string    `json:"tags,omitempty" example:"production,updated"`
	Owner         *string     `json:"owner,omitempty" validate:"omitempty,max=255" example:"newowner@example.com"`
	Amfa          *TenantAmfa `json:"amfa,omitempty"`
}

// TestTenantConnection represents the request to test a Keycloak connection.
//
//	@Description	Request body for testing Keycloak connection
type TestTenantConnection struct {
	ServerURL    string `json:"server_url" validate:"required,url" example:"https://keycloak.example.com"`
	AdminRealm   string `json:"admin_realm" validate:"omitempty,max=100,keycloak_realm" example:"master"`
	ClientID     string `json:"client_id" validate:"required,max=255" example:"monitoring-service"`
	ClientSecret string `json:"client_secret" validate:"required,max=255" example:"client-secret"`
}
