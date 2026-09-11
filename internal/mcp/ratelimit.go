package mcp

import (
	"net"
	"net/http"
	"net/netip"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

// maxTrackedUsers and maxTrackedIPs bound the limiter caches. Past these, the
// least-recently-seen key is evicted and restarts with a full burst, which only
// ever admits more traffic, so a flood of distinct keys costs bounded memory
// instead of weakening any active bucket.
const (
	maxTrackedUsers = 8192
	maxTrackedIPs   = 65536
)

// keyLimiter is a bounded collection of per-key token buckets. A nil
// keyLimiter admits everything.
type keyLimiter[K comparable] struct {
	mu      sync.Mutex
	buckets *lru.Cache[K, *rate.Limiter]
	rate    rate.Limit
	burst   int
}

// newKeyLimiter builds a limiter that refills perMinute tokens per minute with
// the given burst, tracking at most capacity distinct keys. A perMinute of 0
// or less disables limiting by returning nil.
func newKeyLimiter[K comparable](perMinute, burst, capacity int) *keyLimiter[K] {
	if perMinute <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = 1
	}
	if capacity <= 0 {
		capacity = 1
	}
	buckets, _ := lru.New[K, *rate.Limiter](capacity)
	return &keyLimiter[K]{
		buckets: buckets,
		rate:    rate.Limit(float64(perMinute) / 60),
		burst:   burst,
	}
}

// Allow reports whether one more request for key fits its bucket.
func (l *keyLimiter[K]) Allow(key K) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	bucket, ok := l.buckets.Get(key)
	if !ok {
		bucket = rate.NewLimiter(l.rate, l.burst)
		l.buckets.Add(key, bucket)
	}
	l.mu.Unlock()
	return bucket.Allow()
}

// ipBucketKey derives the limiter key from a connection's RemoteAddr. IPv4
// keys per address; IPv6 keys on the /64 prefix, because a single host
// commonly owns an entire /64 and per-address buckets would hand it 2^64
// fresh budgets. An unparseable host keys on the raw string.
func ipBucketKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}

// rateLimitByIP rejects requests over the per-IP budget before authentication
// runs, blunting credential stuffing and unauthenticated abuse. The client is
// always keyed by the direct RemoteAddr: X-Forwarded-For is deliberately not
// honored because the MCP server does not sit behind a trusted reverse proxy
// by default, and a spoofable header would let a client pick its own bucket.
func rateLimitByIP(ips *keyLimiter[string], next http.Handler) http.Handler {
	if ips == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ips.Allow(ipBucketKey(r.RemoteAddr)) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
