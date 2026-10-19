-- The tables an early single-tenant server creates, as pg_dump --schema-only printed them from one.
-- They have no tenant_id. The hypertables and their compression are set up by the test.
CREATE TABLE public.configuration_alerts (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    alert_id character varying(255) NOT NULL,
    type text NOT NULL,
    severity text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    title character varying(255) NOT NULL,
    description text,
    recommendation text,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    resource_name text NOT NULL,
    realm_name text NOT NULL,
    check_type text NOT NULL,
    metadata jsonb,
    first_detected timestamp with time zone NOT NULL,
    last_seen timestamp with time zone NOT NULL,
    resolved_at timestamp with time zone,
    acknowledged_at timestamp with time zone,
    acknowledged_by character varying(255)
);
CREATE SEQUENCE public.configuration_alerts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.configuration_alerts_id_seq OWNED BY public.configuration_alerts.id;
CREATE TABLE public.events (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    event_id text NOT NULL,
    "timestamp" timestamp with time zone NOT NULL,
    type text,
    category text,
    severity text,
    description text,
    source text,
    source_ip text,
    source_system text,
    user_id text,
    username text,
    email text,
    client_id text,
    location text,
    raw_data jsonb,
    status text DEFAULT 'active'::text NOT NULL
);
CREATE SEQUENCE public.events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.events_id_seq OWNED BY public.events.id;
CREATE TABLE public.keycloak_events (
    "time" timestamp with time zone NOT NULL,
    event_id character varying(255) NOT NULL,
    realm_id text NOT NULL,
    realm_name text,
    client_id text,
    session_id text,
    ip_address text,
    event_type text NOT NULL,
    event_error text,
    user_id text,
    username text,
    email text,
    details jsonb,
    success boolean DEFAULT true NOT NULL
);
CREATE TABLE public.keycloak_health (
    "time" timestamp with time zone NOT NULL,
    status text NOT NULL,
    response_time bigint DEFAULT 0 NOT NULL,
    server_version text,
    memory_used bigint DEFAULT 0 NOT NULL,
    memory_max bigint DEFAULT 0 NOT NULL,
    memory_free bigint DEFAULT 0 NOT NULL,
    uptime_millis bigint DEFAULT 0 NOT NULL,
    raw_data jsonb,
    error_message text
);
CREATE TABLE public.keycloak_metrics (
    "time" timestamp with time zone NOT NULL,
    realm_name character varying(255) NOT NULL,
    total_users bigint DEFAULT 0 NOT NULL,
    enabled_users bigint DEFAULT 0 NOT NULL,
    disabled_users bigint DEFAULT 0 NOT NULL,
    active_sessions bigint DEFAULT 0 NOT NULL,
    offline_sessions bigint DEFAULT 0 NOT NULL,
    total_clients bigint DEFAULT 0 NOT NULL,
    login_events bigint DEFAULT 0 NOT NULL,
    logout_events bigint DEFAULT 0 NOT NULL,
    failed_login_events bigint DEFAULT 0 NOT NULL,
    register_events bigint DEFAULT 0 NOT NULL,
    raw_metrics jsonb
);
CREATE TABLE public.keycloak_realms (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    realm_id text NOT NULL,
    realm_name text NOT NULL,
    display_name text,
    enabled boolean DEFAULT true,
    ssl_required text,
    registration_allowed boolean DEFAULT false,
    remember_me boolean DEFAULT false,
    verify_email boolean DEFAULT false,
    login_with_email_allowed boolean DEFAULT true,
    duplicate_emails_allowed boolean DEFAULT false,
    reset_password_allowed boolean DEFAULT false,
    edit_username_allowed boolean DEFAULT false,
    brute_force_protected boolean DEFAULT false,
    events_enabled boolean DEFAULT false,
    events_listeners text,
    enabled_event_types text,
    admin_events_enabled boolean DEFAULT false,
    admin_events_details_enabled boolean DEFAULT false,
    raw_data jsonb,
    last_checked timestamp with time zone,
    is_healthy boolean DEFAULT true,
    health_message text
);
CREATE SEQUENCE public.keycloak_realms_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.keycloak_realms_id_seq OWNED BY public.keycloak_realms.id;
ALTER TABLE ONLY public.configuration_alerts ALTER COLUMN id SET DEFAULT nextval('public.configuration_alerts_id_seq'::regclass);
ALTER TABLE ONLY public.events ALTER COLUMN id SET DEFAULT nextval('public.events_id_seq'::regclass);
ALTER TABLE ONLY public.keycloak_realms ALTER COLUMN id SET DEFAULT nextval('public.keycloak_realms_id_seq'::regclass);
ALTER TABLE ONLY public.configuration_alerts
    ADD CONSTRAINT configuration_alerts_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.keycloak_events
    ADD CONSTRAINT keycloak_events_pkey PRIMARY KEY ("time", event_id);
ALTER TABLE ONLY public.keycloak_health
    ADD CONSTRAINT keycloak_health_pkey PRIMARY KEY ("time");
ALTER TABLE ONLY public.keycloak_metrics
    ADD CONSTRAINT keycloak_metrics_pkey PRIMARY KEY ("time", realm_name);
ALTER TABLE ONLY public.keycloak_realms
    ADD CONSTRAINT keycloak_realms_pkey PRIMARY KEY (id);
CREATE INDEX idx_alerts_acknowledged ON public.configuration_alerts USING btree (acknowledged_at);
CREATE INDEX idx_alerts_check_type ON public.configuration_alerts USING btree (check_type);
CREATE INDEX idx_alerts_first_detected ON public.configuration_alerts USING btree (first_detected);
CREATE INDEX idx_alerts_last_seen ON public.configuration_alerts USING btree (last_seen);
CREATE INDEX idx_alerts_realm ON public.configuration_alerts USING btree (realm_name);
CREATE INDEX idx_alerts_resolved ON public.configuration_alerts USING btree (resolved_at);
CREATE INDEX idx_alerts_resource_id ON public.configuration_alerts USING btree (resource_id);
CREATE INDEX idx_alerts_resource_name ON public.configuration_alerts USING btree (resource_name);
CREATE INDEX idx_alerts_resource_type ON public.configuration_alerts USING btree (resource_type);
CREATE INDEX idx_alerts_severity ON public.configuration_alerts USING btree (severity);
CREATE INDEX idx_alerts_status ON public.configuration_alerts USING btree (status);
CREATE INDEX idx_alerts_type ON public.configuration_alerts USING btree (type);
CREATE UNIQUE INDEX idx_configuration_alerts_alert_id ON public.configuration_alerts USING btree (alert_id);
CREATE INDEX idx_configuration_alerts_created_at ON public.configuration_alerts USING btree (created_at);
CREATE INDEX idx_configuration_alerts_deleted_at ON public.configuration_alerts USING btree (deleted_at);
CREATE INDEX idx_events_category ON public.events USING btree (category);
CREATE INDEX idx_events_client_id ON public.events USING btree (client_id);
CREATE INDEX idx_events_created_at ON public.events USING btree (created_at);
CREATE INDEX idx_events_deleted_at ON public.events USING btree (deleted_at);
CREATE UNIQUE INDEX idx_events_event_id ON public.events USING btree (event_id);
CREATE INDEX idx_events_severity ON public.events USING btree (severity);
CREATE INDEX idx_events_source ON public.events USING btree (source);
CREATE INDEX idx_events_status ON public.events USING btree (status);
CREATE INDEX idx_events_timestamp ON public.events USING btree ("timestamp");
CREATE INDEX idx_events_type ON public.events USING btree (type);
CREATE INDEX idx_events_user_id ON public.events USING btree (user_id);
CREATE INDEX idx_keycloak_events_client ON public.keycloak_events USING btree (client_id);
CREATE INDEX idx_keycloak_events_error ON public.keycloak_events USING btree (event_error);
CREATE INDEX idx_keycloak_events_ip ON public.keycloak_events USING btree (ip_address);
CREATE INDEX idx_keycloak_events_realm ON public.keycloak_events USING btree (realm_id);
CREATE INDEX idx_keycloak_events_realm_name ON public.keycloak_events USING btree (realm_name);
CREATE INDEX idx_keycloak_events_session ON public.keycloak_events USING btree (session_id);
CREATE INDEX idx_keycloak_events_success ON public.keycloak_events USING btree (success);
CREATE INDEX idx_keycloak_events_time ON public.keycloak_events USING btree ("time");
CREATE INDEX idx_keycloak_events_type ON public.keycloak_events USING btree (event_type);
CREATE INDEX idx_keycloak_events_user ON public.keycloak_events USING btree (user_id);
CREATE INDEX idx_keycloak_events_username ON public.keycloak_events USING btree (username);
CREATE INDEX idx_keycloak_health_status ON public.keycloak_health USING btree (status);
CREATE INDEX idx_keycloak_health_time ON public.keycloak_health USING btree ("time");
CREATE INDEX idx_keycloak_metrics_realm ON public.keycloak_metrics USING btree (realm_name);
CREATE INDEX idx_keycloak_metrics_time ON public.keycloak_metrics USING btree ("time");
CREATE INDEX idx_keycloak_realms_created_at ON public.keycloak_realms USING btree (created_at);
CREATE INDEX idx_keycloak_realms_deleted_at ON public.keycloak_realms USING btree (deleted_at);
CREATE INDEX idx_keycloak_realms_is_healthy ON public.keycloak_realms USING btree (is_healthy);
CREATE INDEX idx_keycloak_realms_last_checked ON public.keycloak_realms USING btree (last_checked);
CREATE UNIQUE INDEX idx_keycloak_realms_realm_id ON public.keycloak_realms USING btree (realm_id);
CREATE UNIQUE INDEX idx_keycloak_realms_realm_name ON public.keycloak_realms USING btree (realm_name);
