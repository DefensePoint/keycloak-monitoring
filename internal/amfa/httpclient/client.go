// Package httpclient implements amfa.Repository over AMFA's read-only
// monitoring HTTP API, replacing the direct PostgreSQL connection in
// amfa/postgres.
//
// It is a drop-in alternative: both satisfy the same amfa.Repository
// interface, so the service, handlers, alert rules and events mirror are
// unchanged by the switch.
//
// AMFA computes the aggregates (stats, geo buckets, risky-user counts, event
// counts) server-side. Each reduces a potentially unbounded number of rows to a
// handful of numbers, so fetching rows to count them here would mean pulling a
// realm's whole history over the wire to produce four integers — the same
// reason KMT prefers Keycloak's /users/count over listing and counting in
// memory.
package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

const (
	defaultTimeout = 30 * time.Second

	// Page size for incremental pulls. Matches AMFA's own ceiling, so a request
	// for more is not silently reduced.
	defaultPageSize = 500

	// Cap on pages walked in one ListEventsSince call. A realm with a long
	// backlog is drained across successive calls rather than in one unbounded
	// loop that could hold memory and a connection indefinitely.
	maxPagesPerCall = 20

	// Cap on pages walked to satisfy ListEvents' Offset. The handler allows
	// Offset up to 50,000, sized for the database transport's native SQL
	// OFFSET; over HTTP each page is a round trip, so the same value here
	// could mean thousands of sequential requests. 50 pages at the UI's own
	// page size comfortably covers real navigation while still failing fast,
	// with a clear error, on an offset this transport can't serve cheaply.
	maxOffsetWalkPages = 50
)

// Client reads one tenant's AMFA data over HTTP.
type Client struct {
	baseURL     string
	tokens      TokenSource
	httpClient  *http.Client
	logger      *logger.Logger
	lookbackDay int
}

// Config describes one tenant's AMFA API endpoint.
type Config struct {
	BaseURL      string
	Timeout      time.Duration
	LookbackDays int
}

// Compile-time check: this must remain interchangeable with amfa/postgres.
var _ amfa.Repository = (*Client)(nil)

// Option adjusts a Client at construction.
type Option func(*Client)

// WithHTTPClient supplies the HTTP client to use. Callers pass one built from
// the platform's outbound policy so AMFA traffic obeys the same SSRF and TLS
// rules as every other request.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// New builds a Client. A nil logger is tolerated for tests.
func New(cfg Config, tokens TokenSource, log *logger.Logger, opts ...Option) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("amfa httpclient: base_url is required")
	}
	if tokens == nil {
		return nil, fmt.Errorf("amfa httpclient: token source is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	lookback := cfg.LookbackDays
	if lookback <= 0 {
		lookback = defaultLookbackDays
	}
	if lookback > maxLookbackDays {
		lookback = maxLookbackDays
	}

	c := &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		tokens:      tokens,
		httpClient:  &http.Client{Timeout: timeout},
		logger:      log,
		lookbackDay: lookback,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// get issues an authenticated GET and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, realm, path string, params url.Values, op string, out any) error {
	if realm == "" {
		return fmt.Errorf("%s: realm is required", op)
	}
	// url.PathEscape leaves "/" untouched, so a realm containing one would
	// silently retarget the request at a different endpoint. Realm names come
	// from tenant configuration, so reject the character rather than encode it:
	// no Keycloak realm can contain a slash, and a request built from one is a
	// configuration error, not something to paper over.
	if strings.ContainsAny(realm, "/\\") {
		return fmt.Errorf("%s: realm %q contains a path separator", op, realm)
	}

	endpoint := fmt.Sprintf("%s/%s/monitoring/%s", c.baseURL, url.PathEscape(realm), path)
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return classifyHTTPError(err, op)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return classifyHTTPError(err, op)
	}
	if err := classifyStatus(resp.StatusCode, body, op); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: decode response: %w", op, err)
	}
	return nil
}

// eventsPage mirrors AMFA's MonitoringEventsPage.
type eventsPage struct {
	Items      []eventRow `json:"items"`
	Total      int64      `json:"total"`
	NextCursor *string    `json:"next_cursor"`
}

// eventRow mirrors AMFA's MonitoringEventRow. Field names match the API
// contract; the mapping to amfa.EventRow is spelled out in toEventRow so a
// rename on either side fails to compile rather than silently dropping a field.
type eventRow struct {
	EventID          string    `json:"event_id"`
	EventTime        time.Time `json:"event_time"`
	EventType        string    `json:"event_type"`
	UserID           *string   `json:"user_id"`
	Client           string    `json:"client"`
	IPAddress        string    `json:"ip_address"`
	Country          *string   `json:"country"`
	City             *string   `json:"city"`
	Lat              *float64  `json:"lat"`
	Long             *float64  `json:"long"`
	IsVPN            bool      `json:"is_vpn"`
	RiskLevel        *int      `json:"risk_level"`
	FinalStatus      *string   `json:"final_status"`
	OperatingSystem  *string   `json:"operating_system"`
	Browser          *string   `json:"browser"`
	Device           *string   `json:"device"`
	SystemLanguage   *string   `json:"system_language"`
	ScreenResolution *string   `json:"screen_resolution"`
}

func (r eventRow) toEventRow() amfa.EventRow {
	return amfa.EventRow{
		EventID:          r.EventID,
		EventTime:        r.EventTime,
		EventType:        r.EventType,
		UserID:           r.UserID,
		Client:           r.Client,
		IPAddress:        r.IPAddress,
		Country:          r.Country,
		City:             r.City,
		Lat:              r.Lat,
		Long:             r.Long,
		IsVPN:            r.IsVPN,
		RiskLevel:        r.RiskLevel,
		FinalStatus:      r.FinalStatus,
		OperatingSystem:  r.OperatingSystem,
		Browser:          r.Browser,
		Device:           r.Device,
		SystemLanguage:   r.SystemLanguage,
		ScreenResolution: r.ScreenResolution,
	}
}

func toEventRows(rows []eventRow) []amfa.EventRow {
	out := make([]amfa.EventRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toEventRow())
	}
	return out
}

// setWindow writes the time bounds every query accepts.
//
// AMFA treats `since` as exclusive and `until` as inclusive, which is what lets
// an incremental consumer pass back the last timestamp it saw without
// re-reading that row.
func setWindow(params url.Values, start, end time.Time) {
	params.Set("since", start.UTC().Format(time.RFC3339Nano))
	params.Set("until", end.UTC().Format(time.RFC3339Nano))
}

// ListEvents returns one page of events for the UI list, newest first.
func (c *Client) ListEvents(ctx context.Context, opts amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
	if opts.RealmID == "" {
		return nil, fmt.Errorf("amfa.ListEvents: realm_id is required")
	}
	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime, c.lookbackDay)

	params := url.Values{}
	setWindow(params, start, end)
	params.Set("limit", strconv.Itoa(limit))
	// Newest first for the UI; the incremental consumers below walk forward.
	params.Set("ascending", "false")
	if opts.RiskLevel != nil {
		params.Set("min_risk", strconv.Itoa(*opts.RiskLevel))
	}
	if opts.EventType != "" {
		params.Set("event_type", opts.EventType)
	}

	// The offset the UI passes cannot be expressed as a keyset cursor, so it is
	// walked forward one page at a time, keeping whatever part of each page
	// falls at or after Offset until Limit rows are collected. Bounded by
	// maxOffsetWalkPages: each page costs a round trip, unlike the database
	// transport's native OFFSET, so a caller asking for a very deep page gets
	// a clear error rather than a request that quietly eats the timeout.
	params.Set("include_total", "true")
	kept := make([]eventRow, 0, limit)
	seen := 0
	var total int64
	for pages := 0; ; pages++ {
		if pages >= maxOffsetWalkPages {
			return nil, fmt.Errorf("amfa.ListEvents: %w: offset %d needs more than %d pages",
				amfa.ErrOffsetTooLarge, opts.Offset, maxOffsetWalkPages)
		}
		var page eventsPage
		if err := c.get(ctx, opts.RealmID, "events", params, "amfa list events", &page); err != nil {
			return nil, err
		}
		if pages == 0 {
			// Total doesn't change across pages of the same filtered query;
			// take it from the first fetch rather than the last, since a
			// continuation page may not have recomputed it.
			total = page.Total
		}

		pageStart := seen
		seen += len(page.Items)
		if seen > opts.Offset {
			keepFrom := opts.Offset - pageStart
			if keepFrom < 0 {
				keepFrom = 0
			}
			kept = append(kept, page.Items[keepFrom:]...)
		}

		if len(kept) >= limit || page.NextCursor == nil {
			break
		}
		params.Set("cursor", *page.NextCursor)
		params.Set("include_total", "false")
	}
	if len(kept) > limit {
		kept = kept[:limit]
	}

	return &amfa.ListEventsResult{Items: toEventRows(kept), Total: total}, nil
}

// GetStats returns the four KPIs.
func (c *Client) GetStats(ctx context.Context, opts amfa.StatsOptions) (amfa.Stats, error) {
	start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime, c.lookbackDay)

	params := url.Values{}
	setWindow(params, start, end)

	// An empty RealmID means "all realms". The path is realm-scoped and the
	// token is realm-bound, so there is no all-realms endpoint to call: AMFA
	// would reject it. Report that distinctly from ErrAmfaNotConfigured: AMFA
	// is configured and working here, this one aggregate just isn't available
	// over this transport, and the handler needs to tell those two apart.
	if opts.RealmID == "" {
		return amfa.Stats{}, fmt.Errorf("amfa.GetStats: %w", amfa.ErrAllRealmsUnsupported)
	}

	var stats amfa.Stats
	if err := c.get(ctx, opts.RealmID, "stats", params, "amfa stats query", &stats); err != nil {
		return amfa.Stats{}, err
	}
	return stats, nil
}

// GetGeoBuckets returns map buckets for the realm.
func (c *Client) GetGeoBuckets(ctx context.Context, opts amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	if opts.RealmID == "" {
		return nil, fmt.Errorf("amfa.GetGeoBuckets: realm_id is required")
	}
	start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime, c.lookbackDay)

	params := url.Values{}
	setWindow(params, start, end)

	var buckets []amfa.GeoBucket
	if err := c.get(ctx, opts.RealmID, "geo", params, "amfa geo query", &buckets); err != nil {
		return nil, err
	}
	return buckets, nil
}

// ListRejectedEventsSince returns risk-rejected events newer than since,
// oldest first.
func (c *Client) ListRejectedEventsSince(ctx context.Context, realm string, since time.Time) ([]amfa.EventRow, error) {
	params := url.Values{}
	params.Set("since", since.UTC().Format(time.RFC3339Nano))
	params.Set("ascending", "true")
	params.Set("limit", strconv.Itoa(defaultPageSize))
	// Exactly 4, not ">= 4": the rule fires on risk-rejected logins
	// specifically, and a future level 5 must not silently join them.
	params.Set("risk_decision", "4")

	return c.drain(ctx, realm, params, "amfa list rejected events")
}

// ListVPNRiskyEventsSince returns VPN-flagged events at or above minRisk newer
// than since, oldest first.
func (c *Client) ListVPNRiskyEventsSince(ctx context.Context, realm string, since time.Time, minRisk int) ([]amfa.EventRow, error) {
	params := url.Values{}
	params.Set("since", since.UTC().Format(time.RFC3339Nano))
	params.Set("ascending", "true")
	params.Set("limit", strconv.Itoa(defaultPageSize))
	params.Set("is_vpn", "true")
	params.Set("min_risk", strconv.Itoa(minRisk))

	return c.drain(ctx, realm, params, "amfa list vpn risky events")
}

// ListEventsSince returns events newer than since, oldest first, capped at
// limit rows. Used by the events mirror, which pages by watermark.
func (c *Client) ListEventsSince(ctx context.Context, realm string, since time.Time, limit int) ([]amfa.EventRow, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}

	params := url.Values{}
	params.Set("since", since.UTC().Format(time.RFC3339Nano))
	params.Set("ascending", "true")
	params.Set("limit", strconv.Itoa(min(limit, defaultPageSize)))

	var page eventsPage
	if err := c.get(ctx, realm, "events", params, "amfa list events since", &page); err != nil {
		return nil, err
	}
	// Deliberately one page only. The mirror advances its watermark from the
	// rows it saves and re-invokes, so returning a single page keeps the
	// watermark and the returned rows in step; draining here would hand back
	// more than the caller asked for.
	return toEventRows(page.Items), nil
}

// drain walks pages until the realm is exhausted or the page cap is reached.
//
// The alert rules need every matching event since their last run, not the first
// page: a rule that saw only page one would silently stop alerting during a
// burst, which is exactly when it matters.
func (c *Client) drain(ctx context.Context, realm string, params url.Values, op string) ([]amfa.EventRow, error) {
	var all []amfa.EventRow

	for page := 0; page < maxPagesPerCall; page++ {
		var got eventsPage
		if err := c.get(ctx, realm, "events", params, op, &got); err != nil {
			return nil, err
		}
		all = append(all, toEventRows(got.Items)...)

		if got.NextCursor == nil || len(got.Items) == 0 {
			return all, nil
		}
		params.Set("cursor", *got.NextCursor)
	}

	// Hitting the cap means there is more to read. Say so: a silent truncation
	// here reads downstream as "no further events", which is indistinguishable
	// from a quiet period.
	if c.logger != nil {
		c.logger.Warn("AMFA event pull hit the page cap; remaining events will be read on the next cycle",
			logger.Str("realm", realm),
			logger.Int("pages", maxPagesPerCall),
			logger.Int("events", len(all)))
	}
	return all, nil
}

// CountRepeatedRiskyByUser returns users at or above the risky-event threshold.
func (c *Client) CountRepeatedRiskyByUser(ctx context.Context, realm string, since time.Time, minRisk, threshold int) ([]amfa.UserRiskyCount, error) {
	params := url.Values{}
	params.Set("since", since.UTC().Format(time.RFC3339Nano))
	params.Set("min_risk", strconv.Itoa(minRisk))
	params.Set("threshold", strconv.Itoa(threshold))

	// Decoded through a local wire type rather than straight into
	// amfa.UserRiskyCount: that type carries no JSON tags, so `user_id` would
	// not bind to UserID and every row would arrive with an empty user. Count
	// happens to match case-insensitively, which is what makes the failure
	// partial and easy to miss.
	var wire []struct {
		UserID string `json:"user_id"`
		Count  int64  `json:"count"`
	}
	if err := c.get(ctx, realm, "risky-users", params, "amfa count repeated risky by user", &wire); err != nil {
		return nil, err
	}

	counts := make([]amfa.UserRiskyCount, 0, len(wire))
	for _, row := range wire {
		counts = append(counts, amfa.UserRiskyCount{UserID: row.UserID, Count: row.Count})
	}
	return counts, nil
}

// CountByEventTypeInWindow returns how many events of eventType fall in
// [start, end].
func (c *Client) CountByEventTypeInWindow(ctx context.Context, realm, eventType string, start, end time.Time) (int64, error) {
	params := url.Values{}
	params.Set("event_type", eventType)
	setWindow(params, start, end)

	var result struct {
		EventType string `json:"event_type"`
		Count     int64  `json:"count"`
	}
	if err := c.get(ctx, realm, "event-counts", params, "amfa count by event type in window", &result); err != nil {
		return 0, err
	}
	return result.Count, nil
}

// CountByEventTypeAndClientInWindow returns, per OAuth client in the realm,
// the count of eventType events in [start, end], keeping only clients whose
// count >= threshold.
//
// Unlike the four aggregates AMFA computes server-side (stats, geo buckets,
// risky-user counts, single event-type counts), there is no server endpoint
// that groups an arbitrary event type's counts by client, so this walks the
// matching rows over /events and aggregates client-side — the same technique
// ListRejectedEventsSince/ListVPNRiskyEventsSince already use for
// window-bounded, event-type-filtered queries.
func (c *Client) CountByEventTypeAndClientInWindow(ctx context.Context, realm, eventType string, start, end time.Time, threshold int) ([]amfa.ClientEventCount, error) {
	params := url.Values{}
	setWindow(params, start, end)
	params.Set("event_type", eventType)
	params.Set("ascending", "true")
	params.Set("limit", strconv.Itoa(defaultPageSize))

	rows, err := c.drain(ctx, realm, params, "amfa count by event type and client in window")
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, row := range rows {
		if row.Client == "" {
			continue
		}
		counts[row.Client]++
	}

	out := make([]amfa.ClientEventCount, 0, len(counts))
	for client, count := range counts {
		if count >= int64(threshold) {
			out = append(out, amfa.ClientEventCount{Client: client, Count: count})
		}
	}
	return out, nil
}

// CountDistinctRejectedUsersSince returns the number of distinct users with
// at least one risk-rejected (risk_decision = 4) event in the realm since
// `since`. Built on ListRejectedEventsSince, which already fetches exactly
// this row set.
func (c *Client) CountDistinctRejectedUsersSince(ctx context.Context, realm string, since time.Time) (int64, error) {
	rows, err := c.ListRejectedEventsSince(ctx, realm, since)
	if err != nil {
		return 0, err
	}

	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.UserID == nil || *row.UserID == "" {
			continue
		}
		seen[*row.UserID] = struct{}{}
	}
	return int64(len(seen)), nil
}

// CountByEventTypeByUser returns, per user in the realm, the count of
// eventType events since `since`, keeping only users whose count >=
// threshold. Same client-side aggregation rationale as
// CountByEventTypeAndClientInWindow: no server-side endpoint groups an
// arbitrary event type's counts by user (only by risk level, via
// CountRepeatedRiskyByUser's /risky-users).
func (c *Client) CountByEventTypeByUser(ctx context.Context, realm, eventType string, since time.Time, threshold int) ([]amfa.UserRiskyCount, error) {
	params := url.Values{}
	params.Set("since", since.UTC().Format(time.RFC3339Nano))
	params.Set("event_type", eventType)
	params.Set("ascending", "true")
	params.Set("limit", strconv.Itoa(defaultPageSize))

	rows, err := c.drain(ctx, realm, params, "amfa count by event type by user")
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, row := range rows {
		if row.UserID == nil || *row.UserID == "" {
			continue
		}
		counts[*row.UserID]++
	}

	out := make([]amfa.UserRiskyCount, 0, len(counts))
	for userID, count := range counts {
		if count >= int64(threshold) {
			out = append(out, amfa.UserRiskyCount{UserID: userID, Count: count})
		}
	}
	return out, nil
}
