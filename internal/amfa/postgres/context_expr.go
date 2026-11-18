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
// IP, country and VPN flag have one more source, location_network, read
// last. A LOGIN_ERROR that AMFA stored without a risk decision (a wrong
// password or an unknown user, which fail before AMFA's step runs) has no
// auth_process and so no snapshot, but the engine still wrote its address,
// country and VPN flag there. Reading only the first two sources showed such a
// failure with no IP, no country and no VPN flag. The engine stores a country
// it could not resolve as the text "None". Coordinates come from there too, but
// only on an AMFA whose location_network has them: see coordinatesFor.
//
// Every query using them must join auth_event e -> auth_process p and
// LEFT JOIN auth_context ac ON ac.hash = e.auth_context_hash. A query using
// exprIP, exprCountry or exprIsVPN must also add joinNetworkLocation.
const (
	exprClient  = `COALESCE(ac.client, p.auth_context_json->>'client', '')`
	exprIP      = `COALESCE(ac.ip_address, NULLIF(p.auth_context_json->>'ip_address', ''), NULLIF(nl.ip_address, ''), '')`
	exprCountry = `COALESCE(NULLIF(ac.country_name, ''), NULLIF(p.auth_context_json->>'country_name', ''), NULLIF(NULLIF(nl.country, ''), 'None'))`
	exprIsVPN   = `COALESCE(ac.is_vpn, (p.auth_context_json->>'is_vpn')::boolean, nl.is_vpn_flag <> 0, false)`

	// Joined only for an event with no auth_process: a scored event has its
	// snapshot, which every expression reads first, and joining every row cost
	// the stats query 70% to 140% more time on 300,000 events.
	joinNetworkLocation = `LEFT JOIN location_network nl ON p.id IS NULL AND nl.hash = e.network_location_hash`

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

// coordinates are the expressions a query reads a login's coordinates with.
//
// location_network has coordinates only from engine migration 5ae8978564f9 on.
// On an older AMFA, Point's included, or after the engine's downgrade drops
// them, a query naming the columns fails outright, so they are read only where
// they exist (Repository.withCoordinates). They are the last source, like the
// other location_network fields: a scored login keeps its snapshot's.
type coordinates struct {
	lat, long string
}

func coordinatesFor(networkHasCoordinates bool) coordinates {
	if networkHasCoordinates {
		return coordinates{
			lat:  `COALESCE(ac.lat, (p.auth_context_json->>'lat')::float8, nl.lat)`,
			long: `COALESCE(ac.long, (p.auth_context_json->>'long')::float8, nl.long)`,
		}
	}
	return coordinates{
		lat:  `COALESCE(ac.lat, (p.auth_context_json->>'lat')::float8)`,
		long: `COALESCE(ac.long, (p.auth_context_json->>'long')::float8)`,
	}
}

// hasGeo restricts a query to logins with a usable coordinate pair.
//
// AMFA's geolocation helper writes lat/long 0 whenever the ipinfo lookup fails
// or exceeds its 0.3s timeout, which is the normal outcome for private IPs (a
// browser login against a local stack reports the Docker gateway address). That
// 0,0 sentinel has to be excluded, or every un-geolocatable login piles onto a
// single marker in the Gulf of Guinea and reads as real activity.
func (c coordinates) hasGeo() string {
	return `(` + c.lat + `) IS NOT NULL
          AND (` + c.long + `) IS NOT NULL
          AND NOT ((` + c.lat + `) = 0 AND (` + c.long + `) = 0)`
}
