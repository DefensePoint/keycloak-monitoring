-- Seeds demo AMFA login data so KMT's AMFA Events page (table, KPIs, map)
-- has something to render against a local stack.
--
-- Do not run this by hand: it requires the :demo_users variable, which
-- seed.sh builds from the Keycloak user IDs it just created. Run seed.sh.
--
-- Contract with :demo_users -- a VALUES tuple list of (idx, user_id, username):
--   -v demo_users="(0,'uuid'::uuid,'demo-alice'),(1,'uuid'::uuid,'demo-bob')"
-- The UUIDs must be real Keycloak user IDs in the target realm, otherwise
-- KMT's enrichment 404s and the events table shows raw UUIDs.
--
-- WHAT THIS DELIBERATELY DOES NOT DO: populate auth_context. The AMFA
-- engine never writes that table (its AuthContextService only
-- computes a hash), which is why every KMT query COALESCEs through to
-- auth_process.auth_context_json -- see internal/amfa/postgres/context_expr.go.
-- Seeding auth_context would exercise a code path that never runs in
-- production and would hide regressions in the fallback that does.

\set ON_ERROR_STOP on

-- :realm and :demo_users are supplied by seed.sh. Do NOT \set realm here: a
-- literal assignment silently overrides whatever the driver passes, which would
-- write every row with the wrong realm_id and make the data invisible to KMT.

-- Identifier markers. Every row this script writes carries one, which is what
-- lets 03-purge-demo-data.sql remove exactly the demo data and nothing else.
\set process_prefix 'de300001-de30-4de3-8de3-'
\set event_prefix   'de300002-de30-4de3-8de3-'
\set params_id      'de300003-de30-4de3-8de3-000000000001'
\set device_prefix  'de30de30dev-'
\set netloc_prefix  'de30de30net-'
\set ctx_prefix     'de30de30ctx-'

\set event_count 720

BEGIN;

-- ---------------------------------------------------------------------------
-- 0. Clear any previous run. Re-running the seed must not double the data.
--    FK order: auth_event -> auth_process -> device / location_network /
--    decision_params_config.
-- ---------------------------------------------------------------------------
DELETE FROM auth_event   WHERE id::text LIKE :'event_prefix'   || '%';
DELETE FROM auth_process WHERE id::text LIKE :'process_prefix' || '%';
DELETE FROM device            WHERE hash LIKE :'device_prefix' || '%';
DELETE FROM location_network  WHERE hash LIKE :'netloc_prefix' || '%';
DELETE FROM decision_params_config WHERE id = :'params_id'::uuid;

-- Fixed seed so two runs on the same Postgres produce identical data. Without
-- it, "the map looked different after I re-seeded" becomes a debugging session.
SELECT setseed(0.42);

-- ---------------------------------------------------------------------------
-- 1. Reference data
-- ---------------------------------------------------------------------------

-- Addresses come only from the ranges RFC 5737 reserves for documentation
-- (192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24), so no real host is shown
-- as a login source or a VPN exit. Each city owns sixteen of them: five
-- cities share a /24, at offsets 1 + (idx % 5) * 16.
--
-- Real coordinates only. internal/amfa/postgres/context_expr.go's exprHasGeo drops any
-- 0,0 pair, so a placeholder coordinate would silently vanish from the map.
-- `weight` drives how much traffic a city gets, so the map has real hotspots
-- instead of 14 identical dots. Reykjavik carries weight 0: it exists solely
-- as the origin of the planted deny burst in section 4, and must never be
-- picked by the random draw.
CREATE TEMP TABLE cities ON COMMIT DROP AS
SELECT *,
       sum(weight) OVER (ORDER BY idx ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS cum
FROM (VALUES
    ( 0, 'Berlin',        'Germany',        52.5200,  13.4050,  '192.0.2',     18),
    ( 1, 'Vienna',        'Austria',        48.2082,  16.3738,  '192.0.2',     10),
    ( 2, 'Frankfurt',     'Germany',        50.1109,   8.6821,  '192.0.2',     14),
    ( 3, 'London',        'United Kingdom', 51.5074,  -0.1278,  '192.0.2',     12),
    ( 4, 'Amsterdam',     'Netherlands',    52.3676,   4.9041,  '192.0.2',      8),
    ( 5, 'Paris',         'France',         48.8566,   2.3522,  '198.51.100',   7),
    ( 6, 'Madrid',        'Spain',          40.4168,  -3.7038,  '198.51.100',   5),
    ( 7, 'Milan',         'Italy',          45.4642,   9.1900,  '198.51.100',   5),
    ( 8, 'New York',      'United States',  40.7128, -74.0060,  '198.51.100',  12),
    ( 9, 'Chicago',       'United States',  41.8781, -87.6298,  '198.51.100',   6),
    (10, 'San Francisco', 'United States',  37.7749,-122.4194,  '203.0.113',    6),
    (11, 'Toronto',       'Canada',         43.6532, -79.3832,  '203.0.113',    4),
    (12, 'Mumbai',        'India',          19.0760,  72.8777,  '203.0.113',    4),
    (13, 'Singapore',     'Singapore',       1.3521, 103.8198,  '203.0.113',    3),
    (14, 'Reykjavik',     'Iceland',        64.1466, -21.9426,  '203.0.113',    0)
) t(idx, city, country, lat, long, ip_prefix, weight);

CREATE TEMP TABLE devices ON COMMIT DROP AS
SELECT *, :'device_prefix' || lpad(idx::text, 2, '0') AS hash
FROM (VALUES
    (0, 'Desktop', 'Windows 10',  'Chrome',        '1920x1080', 'en-US', 'account-console'),
    (1, 'Desktop', 'Windows 11',  'Edge',          '2560x1440', 'en-GB', 'account-console'),
    (2, 'Desktop', 'Mac OS X',    'Safari',        '1710x1107', 'en-US', 'kmt-web'),
    (3, 'Desktop', 'Mac OS X',    'Chrome',        '1512x982',  'en-US', 'kmt-web'),
    (4, 'Desktop', 'Ubuntu',      'Firefox',       '1920x1080', 'en-US', 'security-admin-console'),
    (5, 'Mobile',  'Android',     'Chrome Mobile', '412x915',   'en-US', 'mobile-app'),
    (6, 'Mobile',  'iOS',         'Mobile Safari', '390x844',   'en-US', 'mobile-app'),
    (7, 'Desktop', 'Windows 10',  'Firefox',       '1366x768',  'de-DE', 'account-console')
) t(idx, device, operating_system, browser, screen_resolution, system_language, client);

-- Injected by seed.sh from the Keycloak users it created.
CREATE TEMP TABLE demo_users ON COMMIT DROP AS
SELECT * FROM (VALUES :demo_users) t(idx, user_id, username);

-- Give each user a home city, weighted the same way traffic is. Logins then
-- cluster per person instead of scattering uniformly, which is what makes the
-- planted impossible-travel pair read as anomalous rather than as more noise.
-- The random draw must be a column on the outer row, not a bare random() call
-- inside the LATERAL. An uncorrelated subquery is evaluated once, which hands
-- every user the same home city and pins ~85% of all traffic to one map marker.
CREATE TEMP TABLE user_homes ON COMMIT DROP AS
WITH u AS (
    SELECT idx, user_id, username, random() AS r_home FROM demo_users
)
SELECT u.idx, u.user_id, u.username, c.idx AS home_city
FROM u
CROSS JOIN LATERAL (
    SELECT ci.idx
    FROM cities ci
    WHERE ci.cum > u.r_home * (SELECT max(cum) FROM cities)
    ORDER BY ci.cum
    LIMIT 1
) c;

-- Exists only because auth_process references it. It must stay inactive: the
-- engine takes the realm's active default-group row as the realm's whole
-- configuration, so an active row here would replace the real one and fail
-- every real login in the realm. '[]' keeps it a valid, empty parameter list.
INSERT INTO decision_params_config (id, realm_id, group_id, is_active, parameters, parameters_hash)
VALUES (:'params_id'::uuid, :'realm', 'default', false, '[]'::json, 'de30demoparams');

INSERT INTO device (hash, device, operating_system, browser, screen_resolution, system_language, client)
SELECT hash, device, operating_system, browser, screen_resolution, system_language, client FROM devices;

-- One row per (city, vpn) pair. auth_process has NOT NULL FKs to both device
-- and location_network, so these have to exist before any process row.
INSERT INTO location_network (hash, ip_address, country, is_vpn_flag, geolocation_cluster_label)
SELECT :'netloc_prefix' || lpad(c.idx::text, 2, '0') || '-' || v.flag::text,
       CASE WHEN v.flag = 1 THEN '203.0.113.250' ELSE c.ip_prefix || '.' || (1 + (c.idx % 5) * 16)::text END,
       c.country,
       v.flag,
       c.idx
FROM cities c CROSS JOIN (VALUES (0), (1)) v(flag);

-- ---------------------------------------------------------------------------
-- 2. Plan the ordinary traffic
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE demo_plan ON COMMIT DROP AS
WITH draws AS (
    SELECT n,
           random() AS r_user,
           random() AS r_away,
           random() AS r_city,
           random() AS r_dev,
           random() AS r_risk,
           random() AS r_vpn,
           random() AS r_type,
           random() AS r_day,
           random() AS r_hour,
           random() AS r_min,
           random() AS r_dur
    FROM generate_series(1, :event_count) AS n
),
picked AS (
    SELECT d.*,
           u.user_id,
           u.username,
           u.home_city,
           dev.idx AS dev_idx,
           -- 85% of logins come from the user's home city; the rest are travel.
           CASE WHEN d.r_away < 0.85 THEN u.home_city ELSE away.idx END AS city_idx,
           -- Risk mix: mostly benign, a thin tail of denials.
           CASE WHEN d.r_risk < 0.55 THEN 1
                WHEN d.r_risk < 0.80 THEN 2
                WHEN d.r_risk < 0.95 THEN 3
                ELSE 4
           END AS risk
    FROM draws d
    JOIN user_homes u
      ON u.idx = floor(d.r_user * (SELECT count(*) FROM user_homes))::int
    JOIN devices dev
      ON dev.idx = floor(d.r_dev * (SELECT count(*) FROM devices))::int
    CROSS JOIN LATERAL (
        SELECT ci.idx
        FROM cities ci
        WHERE ci.cum > d.r_city * (SELECT max(cum) FROM cities)
        ORDER BY ci.cum
        LIMIT 1
    ) away
)
SELECT
    p.n,
    p.user_id,
    p.username,
    p.risk,
    c.idx  AS city_idx,
    c.city,
    c.country,
    c.lat,
    c.long,
    c.ip_prefix,
    p.dev_idx,
    dv.device, dv.operating_system, dv.browser, dv.screen_resolution,
    dv.system_language, dv.client,
    -- Risk 4 means the login was denied outright, so it can only be an error.
    CASE WHEN p.risk = 4 THEN 'LOGIN_ERROR'
         WHEN p.r_type < 0.90  THEN 'LOGIN'
         WHEN p.r_type < 0.95  THEN 'LOGIN_ERROR'
         WHEN p.r_type < 0.985 THEN 'CLIENT_LOGIN'
         ELSE 'CLIENT_LOGIN_ERROR'
    END AS event_type,
    -- Denials skew heavily toward VPN exits; ordinary traffic rarely uses one.
    CASE WHEN p.risk = 4 THEN p.r_vpn < 0.60 ELSE p.r_vpn < 0.10 END AS is_vpn,
    -- 1..29 days back, never today: the window must stay inside the 30-day
    -- lookback and never land in the future.
    (date_trunc('day', now())
        - ((1 + floor(p.r_day * 29))::int) * interval '1 day'
        + ((7 + floor(p.r_hour * 13))::int) * interval '1 hour'
        + (floor(p.r_min * 60)::int) * interval '1 minute') AS event_time,
    (2 + floor(p.r_dur * 38))::int AS duration_secs
FROM picked p
JOIN cities  c  ON c.idx  = p.city_idx
JOIN devices dv ON dv.idx = p.dev_idx;

-- ---------------------------------------------------------------------------
-- 3. Planted anomalies
--    Two recognisable stories, so the page shows something worth looking at
--    rather than uniform noise.
-- ---------------------------------------------------------------------------

-- Impossible travel: one user, Berlin then Singapore twelve minutes later.
INSERT INTO demo_plan
SELECT
    a.n, u.user_id, u.username, a.risk,
    c.idx, c.city, c.country, c.lat, c.long, c.ip_prefix,
    dv.idx, dv.device, dv.operating_system, dv.browser, dv.screen_resolution,
    dv.system_language, dv.client,
    a.event_type, a.is_vpn,
    date_trunc('day', now()) - interval '4 days' + a.offset_min * interval '1 minute',
    6
FROM (VALUES
    (901,  0, 3, 3, 'LOGIN',       false, 9 * 60 + 14),  -- Berlin
    (902, 13, 3, 4, 'LOGIN_ERROR', true,  9 * 60 + 26)   -- Singapore, 12 min later
) a(n, city_idx, dev_idx, risk, event_type, is_vpn, offset_min)
JOIN cities  c  ON c.idx  = a.city_idx
JOIN devices dv ON dv.idx = a.dev_idx
JOIN user_homes u ON u.idx = 0;

-- Deny burst: one user, six rejected attempts three minutes apart from an
-- Icelandic VPN exit, a country that appears nowhere else in the data.
INSERT INTO demo_plan
SELECT
    920 + g.i, u.user_id, u.username, 4,
    c.idx, c.city, c.country, c.lat, c.long, c.ip_prefix,
    dv.idx, dv.device, dv.operating_system, dv.browser, dv.screen_resolution,
    dv.system_language, dv.client,
    'LOGIN_ERROR', true,
    date_trunc('day', now()) - interval '2 days' + interval '23 hours'
        + (g.i * 3) * interval '1 minute',
    4
FROM generate_series(0, 5) AS g(i)
JOIN cities  c  ON c.idx = 14   -- Reykjavik
JOIN devices dv ON dv.idx = 4
JOIN user_homes u ON u.idx = 7;

-- ---------------------------------------------------------------------------
-- 4. Materialise the plan into AMFA's tables
-- ---------------------------------------------------------------------------

INSERT INTO auth_process (
    id, user_id, auth_context_hash, device_info_hash, network_location_hash,
    auth_context_json, pre_auth_risk_decision, parameters_config_id,
    final_status, started_at, finished_at, risk_eval_vars,
    device_credibility, net_loc_credibility
)
SELECT
    (:'process_prefix' || lpad(to_hex(p.n), 12, '0'))::uuid,
    p.user_id,
    :'ctx_prefix' || lpad(p.n::text, 6, '0'),
    :'device_prefix' || lpad(p.dev_idx::text, 2, '0'),
    :'netloc_prefix' || lpad(p.city_idx::text, 2, '0') || '-' || (p.is_vpn::int)::text,
    -- Every field KMT reads for the table and the map lives here, not in
    -- auth_context. lat/long must stay JSON numbers: context_expr.go casts
    -- them with ->>'lat'::float8.
    json_build_object(
        'realm_id',          :'realm',
        'group_id',          'default',
        'user_id',           p.user_id,
        'client',            p.client,
        'ip_address',        p.ip_prefix || '.' || (1 + (p.city_idx % 5) * 16 + (p.n % 16))::text,
        -- Deliberately empty on roughly one ordinary login in seven.
        --
        -- Real AMFA emits this key but leaves it blank wherever GeoIP resolves
        -- a country and no city, so a dataset with a city on every row would
        -- be rosier than production. KMT reads it through
        -- NULLIF(...,'') (context_expr.go exprCity), which turns blank into
        -- absent, hiding the City field and falling the map's caption back to
        -- the country alone. Keeping both cases in the data means the demo
        -- exercises the labelled path and the fallback path side by side.
        --
        -- The planted anomalies in section 3 (n >= 900) always keep their
        -- city: those two stories are the ones worth reading, and a blank
        -- Berlin or Singapore would blunt them.
        'city_name',         CASE WHEN p.n < 900 AND p.n % 7 = 0
                                  THEN '' ELSE p.city END,
        'country_name',      p.country,
        'lat',               p.lat,
        'long',              p.long,
        'is_vpn',            p.is_vpn,
        'device',            p.device,
        'operating_system',  p.operating_system,
        'browser',           p.browser,
        'system_language',   p.system_language,
        'screen_resolution', p.screen_resolution,
        'event_time',        (extract(epoch FROM p.event_time) * 1000)::bigint
    ),
    p.risk,
    :'params_id'::uuid,
    CASE WHEN p.event_type LIKE '%ERROR' THEN 'LOGIN_ERROR' ELSE 'LOGIN' END,
    p.event_time - p.duration_secs * interval '1 second',
    p.event_time,
    json_build_object(
        'anonymous_detection', CASE WHEN p.is_vpn THEN 'ANONYMOUS' ELSE 'NOT ANONYMOUS' END,
        'impossible_travel',   CASE WHEN p.n = 902 THEN 'IMPOSSIBLE' ELSE 'FEASIBLE' END,
        'current_event', json_build_object(
            'is_vpn',       p.is_vpn,
            'country_name', p.country
        )
    ),
    round((0.35 + (p.n % 60) / 100.0)::numeric, 2)::float8,
    round((0.40 + (p.n % 55) / 100.0)::numeric, 2)::float8
FROM demo_plan p;

INSERT INTO auth_event (
    id, auth_process, user_id, event_type, details, event_time,
    auth_context_hash, device_info_hash, network_location_hash
)
SELECT
    (:'event_prefix' || lpad(to_hex(p.n), 12, '0'))::uuid,
    (:'process_prefix' || lpad(to_hex(p.n), 12, '0'))::uuid,
    p.user_id,
    p.event_type,
    json_build_object(
        'username',  p.username,
        'client_id', p.client,
        'auth_method', 'openid-connect',
        'risk_level', p.risk::text,
        'error', CASE
            WHEN p.risk = 4 THEN 'access_denied'
            WHEN p.event_type LIKE '%ERROR' THEN 'invalid_user_credentials'
            ELSE NULL
        END
    )::text,
    p.event_time,
    :'ctx_prefix' || lpad(p.n::text, 6, '0'),
    :'device_prefix' || lpad(p.dev_idx::text, 2, '0'),
    :'netloc_prefix' || lpad(p.city_idx::text, 2, '0') || '-' || (p.is_vpn::int)::text
FROM demo_plan p;

COMMIT;

-- ---------------------------------------------------------------------------
-- 5. Report, using the same joins and fallback expressions KMT itself uses,
--    so a green summary here means the page will render.
-- ---------------------------------------------------------------------------
\echo ''
\echo '=== Seeded AMFA demo data ==='

SELECT count(*) AS events,
       count(*) FILTER (WHERE p.pre_auth_risk_decision >= 3) AS risky,
       count(DISTINCT e.user_id) AS unique_users,
       count(DISTINCT COALESCE(p.auth_context_json->>'ip_address', ''))
           FILTER (WHERE (p.auth_context_json->>'is_vpn')::boolean) AS flagged_ips,
       -- exprCity from context_expr.go, verbatim: blank city reads as absent,
       -- so this is the count of rows that will show a City field and a
       -- "City, Country" map caption. The remainder exercise the fallback.
       count(*) FILTER (
           WHERE NULLIF(p.auth_context_json->>'city_name', '') IS NOT NULL
       ) AS with_city,
       min(e.event_time)::date AS oldest,
       max(e.event_time)::date AS newest
FROM auth_event e
JOIN auth_process p ON p.id = e.auth_process
WHERE p.auth_context_json->>'realm_id' = :'realm';

SELECT p.auth_context_json->>'country_name' AS country,
       count(*) AS events,
       count(*) FILTER (WHERE p.pre_auth_risk_decision >= 3) AS risky
FROM auth_event e
JOIN auth_process p ON p.id = e.auth_process
WHERE p.auth_context_json->>'realm_id' = :'realm'
GROUP BY 1
ORDER BY 2 DESC;
