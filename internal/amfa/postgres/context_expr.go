package postgres

// Shared SQL expressions for reading a login's context fields.
//
// AMFA has a denormalized auth_context table, but the adaptive-auth service
// never writes rows to it: its AuthContextService only computes and returns a
// hash. The values KMT displays therefore have to fall back to the
// auth_context_json snapshot that auth_process always carries.
//
// These expressions exist so every query applies that fallback identically.
// Spelling the COALESCE out per query is what let GetGeoBuckets and the
// Flagged IPs KPI keep reading only the (always empty) table, so the map and
// that KPI silently returned nothing no matter how much real traffic AMFA had
// recorded.
//
// Every query using them must join auth_event e -> auth_process p and
// LEFT JOIN auth_context ac ON ac.hash = e.auth_context_hash.
const (
	exprClient  = `COALESCE(ac.client, p.auth_context_json->>'client', '')`
	exprIP      = `COALESCE(ac.ip_address, p.auth_context_json->>'ip_address', '')`
	exprCountry = `COALESCE(NULLIF(ac.country_name, ''), NULLIF(p.auth_context_json->>'country_name', ''))`
	exprLat     = `COALESCE(ac.lat, (p.auth_context_json->>'lat')::float8)`
	exprLong    = `COALESCE(ac.long, (p.auth_context_json->>'long')::float8)`
	exprIsVPN   = `COALESCE(ac.is_vpn, (p.auth_context_json->>'is_vpn')::boolean, false)`

	// Additional device/agent context. auth_context has no city column, so
	// city has no table-first fallback to apply; the other four read the
	// table first like every field above, falling back to the JSON snapshot
	// only when auth_context has nothing (which is every row adaptive-auth
	// writes today, but not necessarily every row forever - a query that
	// skips the table outright would silently ignore real data the moment
	// something does populate it, exactly as GetGeoBuckets and the Flagged
	// IPs KPI did before this file's COALESCE pattern was applied to them).
	exprCity             = `NULLIF(p.auth_context_json->>'city_name', '')`
	exprOS               = `COALESCE(NULLIF(ac.operating_system, ''), NULLIF(p.auth_context_json->>'operating_system', ''))`
	exprBrowser          = `COALESCE(NULLIF(ac.browser, ''), NULLIF(p.auth_context_json->>'browser', ''))`
	exprDevice           = `COALESCE(NULLIF(ac.device, ''), NULLIF(p.auth_context_json->>'device', ''))`
	exprSystemLanguage   = `COALESCE(NULLIF(ac.system_language, ''), NULLIF(p.auth_context_json->>'system_language', ''))`
	exprScreenResolution = `COALESCE(NULLIF(ac.screen_resolution, ''), NULLIF(p.auth_context_json->>'screen_resolution', ''))`
)

// realmFilterExpr restricts a query to one realm.
//
// Checks auth_event.realm_id first: AMFA's login-event webhook stamps it
// directly at write time, so it is present even for a LOGIN_ERROR event whose
// auth_process never linked (an error before password entry, for instance) -
// a row build_stats/ListEvents/etc would otherwise silently drop, since it has
// no auth_process to read a JSON snapshot's realm_id from at all. Falls back
// to auth_process's own attribution only when auth_event.realm_id is absent
// (a row predating that column, or one whose backfill couldn't recover it).
//
// Consumes two `?` placeholders; the caller must pass the realm value twice,
// in this order. Verified live against a real Keycloak + AMFA SPI login: a
// LOGIN_ERROR row with no auth_process was invisible to every query in this
// package until this fallback was added, while AMFA's own HTTP API (which
// already applies this same OR) served it correctly.
const realmFilterExpr = `(
            NULLIF(e.realm_id, '') = ?
            OR (
                NULLIF(e.realm_id, '') IS NULL
                AND p.auth_context_json->>'realm_id' = ?
            )
        )`

// realmAttributedExpr reports whether a row's realm is known from either
// source. Used by the "all realms" aggregate (an empty RealmID), which must
// exclude only rows with no realm attribution at all - not rows that happen
// to be attributed via auth_event.realm_id rather than auth_process's JSON
// snapshot, or the KPI would newly undercount relative to what the per-realm
// queries above and the events mirror (which applies the same realmFilterExpr
// fallback) already return for the realms they do know.
const realmAttributedExpr = `COALESCE(NULLIF(e.realm_id, ''), p.auth_context_json->>'realm_id') IS NOT NULL`

// exprHasGeo restricts a query to logins with a usable coordinate pair.
//
// AMFA's geolocation helper writes lat/long 0 whenever the ipinfo lookup fails
// or exceeds its 0.3s timeout, which is the normal outcome for private IPs (a
// browser login against a local stack reports the Docker gateway address). That
// 0,0 sentinel has to be excluded, or every un-geolocatable login piles onto a
// single marker in the Gulf of Guinea and reads as real activity.
const exprHasGeo = `(` + exprLat + `) IS NOT NULL
          AND (` + exprLong + `) IS NOT NULL
          AND NOT ((` + exprLat + `) = 0 AND (` + exprLong + `) = 0)`
