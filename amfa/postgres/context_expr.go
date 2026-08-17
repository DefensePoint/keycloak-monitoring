package postgres

// Shared SQL expressions for reading a login's context fields.
//
// AMFA has a denormalized auth_context table, but the adaptive-auth service
// never writes rows to it: its AuthContextService only computes and returns a
// hash. The values POINT displays therefore have to fall back to the
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

	// Additional device/agent context. AMFA never populates the auth_context
	// table (see above), so these read straight from the JSON snapshot.
	exprCity             = `NULLIF(p.auth_context_json->>'city_name', '')`
	exprOS               = `NULLIF(p.auth_context_json->>'operating_system', '')`
	exprBrowser          = `NULLIF(p.auth_context_json->>'browser', '')`
	exprDevice           = `NULLIF(p.auth_context_json->>'device', '')`
	exprSystemLanguage   = `NULLIF(p.auth_context_json->>'system_language', '')`
	exprScreenResolution = `NULLIF(p.auth_context_json->>'screen_resolution', '')`
)

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
