package keycloakadmin

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// EventQueryOptions represents query options for fetching events
type EventQueryOptions struct {
	Types     []string  // Event types to filter (LOGIN, LOGOUT, etc.)
	Client    string    // Client ID to filter
	User      string    // User ID to filter
	IPAddress string    // IP address to filter
	DateFrom  time.Time // Start date for events
	DateTo    time.Time // End date for events
	// Since is an exact lower bound (inclusive), sent as epoch milliseconds.
	// It replaces DateFrom, which Keycloak only reads to the day.
	Since     time.Time
	Ascending bool // Oldest first, so paging with First is stable while new events arrive
	First     int  // Pagination: first result index
	Max       int  // Pagination: maximum number of results
}

// AdminEventQueryOptions represents query options for fetching admin events
type AdminEventQueryOptions struct {
	OperationTypes []string  // Operation types to filter (CREATE, UPDATE, DELETE, etc.)
	ResourcePath   string    // Resource path to filter
	DateFrom       time.Time // Start date for events
	DateTo         time.Time // End date for events
	First          int       // Pagination: first result index
	Max            int       // Pagination: maximum number of results
}

// formatKeycloakTime formats a time.Time to Keycloak's expected format (yyyy-MM-dd)
func formatKeycloakTime(t time.Time) string {
	return t.Format("2006-01-02")
}

// GetEvents fetches events from a realm
func (c *Client) GetEvents(ctx context.Context, realmName string, options *EventQueryOptions) ([]*EventRepresentation, error) {
	realmSeg, err := pathSegment(realmName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/admin/realms/%s/events", realmSeg)

	// Build query parameters
	if options != nil {
		params := url.Values{}

		// Keycloak reads each type value as one event type, so a list is
		// sent as the parameter repeated; a comma-joined value is a 500.
		for _, eventType := range options.Types {
			params.Add("type", eventType)
		}

		if options.Client != "" {
			params.Add("client", options.Client)
		}

		if options.User != "" {
			params.Add("user", options.User)
		}

		if options.IPAddress != "" {
			params.Add("ipAddress", options.IPAddress)
		}

		if !options.Since.IsZero() {
			params.Add("dateFrom", strconv.FormatInt(options.Since.UnixMilli(), 10))
		} else if !options.DateFrom.IsZero() {
			params.Add("dateFrom", formatKeycloakTime(options.DateFrom))
		}

		if !options.DateTo.IsZero() {
			params.Add("dateTo", formatKeycloakTime(options.DateTo))
		}

		if options.Ascending {
			params.Add("direction", "asc")
		}

		if options.First > 0 {
			params.Add("first", fmt.Sprintf("%d", options.First))
		}

		if options.Max > 0 {
			params.Add("max", fmt.Sprintf("%d", options.Max))
		}

		if len(params) > 0 {
			path = path + "?" + params.Encode()
		}
	}

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, &eventsReplyError{
			status: resp.StatusCode,
			body:   string(body),
			msg:    fmt.Sprintf("failed to get events, status: %d%s", resp.StatusCode, c.replyDetail("get events", resp.StatusCode, body)),
		}
	}

	var events []*EventRepresentation
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("failed to parse events: %w", err)
	}

	return events, nil
}

// GetAdminEvents fetches admin events from a realm
func (c *Client) GetAdminEvents(ctx context.Context, realmName string, options *AdminEventQueryOptions) ([]*AdminEventRepresentation, error) {
	realmSeg, err := pathSegment(realmName)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/admin/realms/%s/admin-events", realmSeg)

	// Build query parameters
	if options != nil {
		params := url.Values{}

		if len(options.OperationTypes) > 0 {
			for _, opType := range options.OperationTypes {
				params.Add("operationTypes", opType)
			}
		}

		if options.ResourcePath != "" {
			params.Add("resourcePath", options.ResourcePath)
		}

		if !options.DateFrom.IsZero() {
			params.Add("dateFrom", formatKeycloakTime(options.DateFrom))
		}

		if !options.DateTo.IsZero() {
			params.Add("dateTo", formatKeycloakTime(options.DateTo))
		}

		if options.First > 0 {
			params.Add("first", fmt.Sprintf("%d", options.First))
		}

		if options.Max > 0 {
			params.Add("max", fmt.Sprintf("%d", options.Max))
		}

		if len(params) > 0 {
			path = path + "?" + params.Encode()
		}
	}

	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get admin events: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get admin events, status: %d%s", resp.StatusCode, c.replyDetail("get admin events", resp.StatusCode, body))
	}

	var events []*AdminEventRepresentation
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("failed to parse admin events: %w", err)
	}

	return events, nil
}

// GetEventsSince fetches one page of the configured event types at or after
// since, oldest first, starting at offset first. full reports whether the page
// came back at the configured size, so more events may follow at the next
// offset. Since is sent as epoch milliseconds, which Keycloak reads exactly
// from 26.2; the yyyy-MM-dd form it replaces returned the whole day on every
// poll. Older versions get getEventsSinceByDate.
func (c *Client) GetEventsSince(ctx context.Context, realmName string, since time.Time, first int, read EventsRead) (events []*EventRepresentation, full bool, err error) {
	exact, decided := c.eventTimes(ctx)
	if decided && !exact {
		return c.getEventsSinceByDate(ctx, realmName, since, first, read)
	}
	types, err := c.queryEventTypes(ctx)
	if err != nil {
		return nil, false, err
	}
	options := &EventQueryOptions{
		Types:     types,
		Since:     since,
		Ascending: true,
		First:     first,
		Max:       c.config.MaxEventsPerPoll,
	}

	events, err = c.GetEvents(ctx, realmName, options)
	// Keycloak's reply outranks the version: before 26.2 it refuses an epoch
	// dateFrom with a 400, also when the version said otherwise (a rollback,
	// or another node behind the same URL). This poll is then served by date.
	if refused, named := refusesEpochDateFrom(err); refused {
		why := "Keycloak refused an epoch dateFrom"
		if !named {
			c.logger.Warn("Keycloak answered an epoch dateFrom with a 400 that does not name Date(From), perhaps rewritten by a proxy; polling events by date, which every version accepts",
				"keycloak", c.keycloakHost(), "error", err.Error())
			why = "Keycloak answered an epoch dateFrom with a 400"
		}
		c.decideEventTimes(false, why)
		return c.getEventsSinceByDate(ctx, realmName, since, first, read)
	}
	if !decided && err == nil {
		c.decideEventTimes(true, "Keycloak accepted an epoch dateFrom")
	}
	if err != nil {
		return nil, false, err
	}
	return events, options.Max > 0 && len(events) >= options.Max, nil
}

// EventsRead is why GetEventsSince is called. It changes only how a by-date
// read cut short at maxDatePagesPerRead is reported.
type EventsRead int

const (
	// EventsPoll reads events no earlier read returned.
	EventsPoll EventsRead = iota
	// EventsSweep reads again a window the polls already read, for events
	// Keycloak committed late.
	EventsSweep
)

// maxDatePagesPerRead bounds the pages one by-date read walks, so a backlog
// after a long outage does not become thousands of requests in one poll.
const maxDatePagesPerRead = 50

// getEventsSinceByDate serves GetEventsSince on Keycloak before 26.2, which
// answers an epoch dateFrom with a 400, ignores direction and lists events
// newest first. It asks from the day before since, as Keycloak reads the date
// in its own time zone, and pages back from the newest event until a page
// reaches an event older than since or ends the list. It returns the events at
// or after since, oldest first, never reported full: the oldest of them are
// only reached by reading every newer page first.
//
// Events written during the walk push older ones down the list, so an event
// can come back twice at a page boundary, never be skipped. The caller drops
// events it has seen. Only a walk cut short at maxDatePagesPerRead loses
// events, the oldest ones, and that is logged. For a sweep (read) those were
// read by the polls already, and only ones committed late are missed.
func (c *Client) getEventsSinceByDate(ctx context.Context, realmName string, since time.Time, first int, read EventsRead) ([]*EventRepresentation, bool, error) {
	types, err := c.queryEventTypes(ctx)
	if err != nil {
		return nil, false, err
	}
	from := since.UnixMilli()
	size := c.config.MaxEventsPerPoll
	var events []*EventRepresentation
	for page := 0; ; page++ {
		if page == maxDatePagesPerRead {
			msg := "Keycloak before 26.2 lists events newest first, and more were written after since than one read walks; the oldest of them were not read"
			if read == EventsSweep {
				msg = "Keycloak before 26.2 lists events newest first, and the late-commit sweep window holds more events than one read walks; the polls already read them, so only events committed late between since and oldest_read can be missed"
			}
			c.logger.Warn(msg,
				"keycloak", c.keycloakHost(), "realm", realmName,
				"since", since.UTC().Format(time.RFC3339Nano), "read", len(events), "max_pages", maxDatePagesPerRead,
				"oldest_read", time.UnixMilli(events[len(events)-1].Time).UTC().Format(time.RFC3339Nano))
			break
		}
		batch, err := c.GetEvents(ctx, realmName, &EventQueryOptions{
			Types:    types,
			DateFrom: since.Add(-24 * time.Hour),
			First:    page * size,
			Max:      size,
		})
		if err != nil {
			return nil, false, err
		}
		reachedSince := false
		for _, e := range batch {
			if e.Time >= from {
				events = append(events, e)
			} else {
				reachedSince = true
			}
		}
		if reachedSince || len(batch) < size {
			break
		}
	}
	slices.SortStableFunc(events, func(a, b *EventRepresentation) int { return cmp.Compare(a.Time, b.Time) })
	if first >= len(events) {
		return nil, false, nil
	}
	return events[first:], false, nil
}

// exactEventTimesFrom is the first Keycloak version whose events endpoint
// reads dateFrom as epoch milliseconds and honours direction.
var exactEventTimesFrom = [2]int{26, 2}

// eventTimesCheckEvery is how long a Keycloak version read from server info
// is trusted, so an upgrade or a rollback is picked up.
const eventTimesCheckEvery = time.Hour

// eventTimes reports whether Keycloak reads event times to the millisecond,
// and whether that is decided. It is decided from the version in server info,
// or, when server info does not give one, by Keycloak's reply to the next
// events request (see GetEventsSince). Keycloak 26.7 shows the version only
// to clients with the master admin role, not the minimal monitoring roles.
func (c *Client) eventTimes(ctx context.Context) (exact, decided bool) {
	c.eventTimesMu.Lock()
	defer c.eventTimesMu.Unlock()
	c.refreshServerEventsLocked(ctx)
	return c.eventTimesExact, c.eventTimesDecided
}

// decideEventTimes records what Keycloak's reply showed about event times,
// over what the version said, until the next server info refresh.
func (c *Client) decideEventTimes(exact bool, why string) {
	c.eventTimesMu.Lock()
	defer c.eventTimesMu.Unlock()
	if c.eventTimesDecided && c.eventTimesExact == exact {
		return
	}
	c.eventTimesExact, c.eventTimesDecided = exact, true
	version := c.eventTimesVersion
	if version == "" {
		version = "not shown to this client"
	}
	c.reportEventTimesLocked(exact, "Keycloak event time precision", "version", version, "exact", exact, "from", why)
}

// reportEventTimesLocked logs the event time precision when it differs from
// what was last logged, so the hourly refresh does not repeat it.
func (c *Client) reportEventTimesLocked(exact bool, msg string, fields ...any) {
	if c.eventTimesReported && c.eventTimesReportedExact == exact {
		return
	}
	c.eventTimesReported, c.eventTimesReportedExact = true, exact
	c.logger.Info(msg, fields...)
}

// eventsReplyError is a non-200 reply to an events request.
type eventsReplyError struct {
	status int
	body   string
	msg    string
}

func (e *eventsReplyError) Error() string { return e.msg }

// refusesEpochDateFrom reports whether err is a 400 to an events request
// with an epoch dateFrom, and whether it names the parameter as Keycloak
// before 26.2 does: "Invalid value for 'Date(From)', expected format is
// yyyy-MM-dd". The events endpoint gives no other 400 for what the poller
// sends (an unknown type is a 500), so a 400 whose text a proxy replaced is
// taken as the same refusal: polling by date works on every version.
func refusesEpochDateFrom(err error) (refused, named bool) {
	var reply *eventsReplyError
	if !errors.As(err, &reply) || reply.status != http.StatusBadRequest {
		return false, false
	}
	return true, strings.Contains(reply.body, "Date(From)")
}

// refreshServerEventsLocked reads, at most once per eventTimesCheckEvery, what
// server info says about events: the version, for how event times are read,
// and the event types, for which configured ones to query.
func (c *Client) refreshServerEventsLocked(ctx context.Context) {
	now := c.timeNow()
	if !c.eventTimesChecked.IsZero() && now.Sub(c.eventTimesChecked) < eventTimesCheckEvery {
		return
	}
	exact, version := false, ""
	var known map[string]bool
	if info, err := c.GetServerInfo(ctx); err != nil {
		c.logger.Warn("Could not read Keycloak server info; event time precision is taken from the reply to the next events request",
			"error", err.Error())
	} else {
		if info.SystemInfo != nil {
			version = info.SystemInfo.Version
		}
		known = knownEventTypes(info)
	}
	decided := version != ""
	if decided {
		exact = versionAtLeast(version, exactEventTimesFrom)
		c.reportEventTimesLocked(exact, "Keycloak event time precision", "version", version, "exact", exact)
	}
	c.eventTimesExact, c.eventTimesDecided, c.eventTimesChecked, c.eventTimesVersion = exact, decided, now, version
	c.setQueryEventTypesLocked(known)
}

// versionAtLeast reports whether a Keycloak version such as "26.2.5" or
// "26.0.6.vendor-00001" is at least major.minor. An unreadable one is not.
func versionAtLeast(version string, min [2]int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > min[0] || (major == min[0] && minor >= min[1])
}
