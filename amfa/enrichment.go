package amfa

import (
	"context"
	"errors"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/singleflight"
)

// ErrKeycloakUserNotFound is what the enrichment layer expects when a user
// is not found in Keycloak. The adapter to pkg/keycloakadmin translates
// that package's 404-equivalent error into this sentinel.
var ErrKeycloakUserNotFound = errors.New("amfa enrichment: keycloak user not found")

// DefaultEnrichmentCacheSize bounds the number of (tenant, realm, userID)
// entries the enrichment cache will hold. Past this, the least-recently-used
// entry is evicted. Sized for a deployment with up to ~10k distinct active
// users per tenant across a handful of tenants/realms; tune via
// NewEnrichmentWithSize if you expect much higher cardinality.
const DefaultEnrichmentCacheSize = 50_000

// KeycloakUser is the minimal projection KMT needs for display.
type KeycloakUser struct {
	Username *string
	Email    *string
}

// KeycloakClient is the small interface Enrichment depends on. The concrete
// adapter wrapping pkg/keycloakadmin is wired in the fx module.
type KeycloakClient interface {
	GetUser(ctx context.Context, tenantID, realm, userID string) (*KeycloakUser, error)
}

type cacheEntry struct {
	user      *KeycloakUser // nil = negative cache (404)
	expiresAt time.Time
}

// cacheKey is a typed composite map key for the enrichment cache. Using a
// struct instead of a delimited string avoids the cross-tenant/cross-realm
// collision that any delimiter would have when one of the components happens
// to contain the delimiter (e.g. an attacker- or operator-controlled realm
// name with `|` in it).
type cacheKey struct {
	tenantID string
	realm    string
	userID   string
}

// Enrichment looks up Keycloak users for AMFA events with a per-process
// bounded LRU cache. Successful lookups are cached for successTTL; 404s
// are cached for the shorter notFoundTTL to prevent retry storms. The LRU
// also caps total memory: once the cache reaches its configured size the
// least-recently-used entry is evicted on the next insert, so memory cannot
// grow without bound even if the deployment encounters arbitrarily many
// distinct user IDs.
type Enrichment struct {
	client      KeycloakClient
	successTTL  time.Duration
	notFoundTTL time.Duration
	cache       *lru.Cache[cacheKey, cacheEntry]
	// inflight coalesces concurrent Keycloak lookups for the same cacheKey
	// so that N parallel BatchEnrich callers missing the same user only
	// fire one upstream admin-API call. Without this, a cold cache or a
	// TTL boundary on a busy page produces N parallel requests for each
	// distinct userID, defeating the negative cache TTL especially.
	inflight singleflight.Group
}

// NewEnrichment returns an Enrichment with the given TTLs and the default
// cache capacity (DefaultEnrichmentCacheSize). Recommended TTLs:
// successTTL=5*time.Minute, notFoundTTL=30*time.Second.
func NewEnrichment(client KeycloakClient, successTTL, notFoundTTL time.Duration) *Enrichment {
	return NewEnrichmentWithSize(client, successTTL, notFoundTTL, DefaultEnrichmentCacheSize)
}

// NewEnrichmentWithSize is NewEnrichment with a configurable cache capacity.
// A size of <= 0 is treated as DefaultEnrichmentCacheSize.
func NewEnrichmentWithSize(client KeycloakClient, successTTL, notFoundTTL time.Duration, size int) *Enrichment {
	if size <= 0 {
		size = DefaultEnrichmentCacheSize
	}
	// lru.New only errors on size <= 0, which we just guarded against, so
	// the error is genuinely impossible here.
	cache, _ := lru.New[cacheKey, cacheEntry](size)
	return &Enrichment{
		client:      client,
		successTTL:  successTTL,
		notFoundTTL: notFoundTTL,
		cache:       cache,
	}
}

// BatchEnrich looks up each unique user ID and returns a map keyed by userID.
// Cache hits avoid Keycloak calls. Per-batch dedup is automatic.
// Empty user IDs are skipped silently.
// Errors other than ErrKeycloakUserNotFound (e.g. transient network) are not
// cached; future calls retry. The function never returns an error itself —
// any failed enrichment simply omits that user from the returned map, and the
// caller falls back to the raw UUID in the UI.
func (e *Enrichment) BatchEnrich(ctx context.Context, tenantID, realm string, userIDs []string) (map[string]KeycloakUser, error) {
	seen := make(map[string]struct{}, len(userIDs))
	out := make(map[string]KeycloakUser, len(userIDs))
	now := time.Now()

	for _, uid := range userIDs {
		if uid == "" {
			continue
		}
		if _, dup := seen[uid]; dup {
			continue
		}
		seen[uid] = struct{}{}

		key := cacheKey{tenantID: tenantID, realm: realm, userID: uid}

		if entry, hit := e.cache.Get(key); hit && entry.expiresAt.After(now) {
			if entry.user != nil {
				out[uid] = *entry.user
			}
			// negative cache hit: skip silently
			continue
		}

		// singleflight key is the same shape as the cache key; concurrent
		// misses for the same (tenant, realm, user) coalesce into one upstream call.
		sfKey := key.tenantID + "\x00" + key.realm + "\x00" + key.userID
		entryAny, _, _ := e.inflight.Do(sfKey, func() (interface{}, error) {
			// Another caller may have populated the cache while we were
			// waiting for the inflight slot — re-check before hitting upstream.
			if cached, hit := e.cache.Get(key); hit && cached.expiresAt.After(time.Now()) {
				return cached, nil
			}
			user, err := e.client.GetUser(ctx, tenantID, realm, uid)
			fetched := time.Now()
			switch {
			case errors.Is(err, ErrKeycloakUserNotFound):
				ce := cacheEntry{user: nil, expiresAt: fetched.Add(e.notFoundTTL)}
				e.cache.Add(key, ce)
				return ce, nil
			case err == nil && user != nil:
				ce := cacheEntry{user: user, expiresAt: fetched.Add(e.successTTL)}
				e.cache.Add(key, ce)
				return ce, nil
			default:
				// Transient error: don't cache. Return a zero entry so the
				// caller can detect there's nothing to merge into `out`.
				return cacheEntry{}, nil
			}
		})

		entry := entryAny.(cacheEntry)
		if entry.user != nil {
			out[uid] = *entry.user
		}
	}
	return out, nil
}
