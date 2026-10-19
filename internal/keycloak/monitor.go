package keycloak

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	eventsource "github.com/DefensePoint/keycloak-monitoring/internal/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// Monitor represents the Keycloak monitoring service
type Monitor struct {
	tenantID       string
	tenantName     string
	client         AdminAPI
	instanceConfig *config.KeycloakInstanceConfig
	logger         *logger.Logger

	// Keycloak-specific repositories (using minimal Monitor interfaces)
	eventRepo   MonitorEventRepository
	metricsRepo MonitorMetricsRepository
	healthRepo  MonitorHealthRepository
	realmRepo   MonitorRealmRepository

	// Generic event repo (interface for dashboard events)
	generalEventRepo EventSaver

	// Event to alert converter
	eventAlertConverter EventAlertConverter

	// Health alert manager
	healthAlertManager HealthAlertManager

	// Metrics recorder (never nil; NopRecorder when metrics are disabled)
	metrics metrics.Recorder

	// Optional: keeps the tenant record's health_status current (best-effort).
	healthUpdater TenantHealthUpdater

	// Optional: told how each events cycle's writes went.
	eventStorage EventStorageReporter

	// Each realm's position in its event stream, so a poll fetches only the
	// events after the last one seen. Only the events worker touches it.
	eventCursors map[string]*realmEventCursor
	// now is the clock the events worker reads; tests replace it.
	now func() time.Time
	// eventIDsHashedFrom is when this monitor started: events without a
	// Keycloak id stamped before it may be stored under their legacy id.
	eventIDsHashedFrom time.Time
	// The newest event (ms) stored without a Keycloak id: before the start
	// (idlessStored, read once) and seen since (idlessSeen). See eventIDs.
	idlessStoredRead bool
	legacyStored     int64
	idlessStored     int64
	idlessSeen       int64

	// workers tracks the polling workers Start launches so Stop can drain
	// them.
	workers lifecycle.Workers

	// unreadableRealms holds the realms whose settings the monitoring client
	// could not view at the last realm-info collection. Only the realm-info
	// worker touches it.
	unreadableRealms map[string]bool
}

// MonitorOption configures optional Monitor behavior.
type MonitorOption func(*Monitor)

// WithTenantHealthUpdater wires an updater that keeps the tenant record's
// health_status/last_error in sync with each health poll.
func WithTenantHealthUpdater(u TenantHealthUpdater) MonitorOption {
	return func(m *Monitor) { m.healthUpdater = u }
}

// WithEventStorageReporter wires a reporter told, after each events cycle, how
// storing the polled events went.
func WithEventStorageReporter(r EventStorageReporter) MonitorOption {
	return func(m *Monitor) { m.eventStorage = r }
}

// mapHealthStatusToTenant translates poller status (UP/DOWN/...) to the
// tenant record vocabulary (healthy/unhealthy/degraded).
func mapHealthStatusToTenant(status string) string {
	switch status {
	case "UP":
		return "healthy"
	case "DOWN":
		return "unhealthy"
	default:
		return "degraded"
	}
}

// NewMonitor creates a new Keycloak monitor
func NewMonitor(
	tenantID string,
	tenantName string,
	client AdminAPI,
	instanceCfg *config.KeycloakInstanceConfig,
	log *logger.Logger,
	eventRepo MonitorEventRepository,
	generalEventRepo EventSaver,
	metricsRepo MonitorMetricsRepository,
	healthRepo MonitorHealthRepository,
	realmRepo MonitorRealmRepository,
	eventAlertConverter EventAlertConverter,
	healthAlertManager HealthAlertManager,
	opts ...MonitorOption,
) *Monitor {
	m := &Monitor{
		eventCursors:        make(map[string]*realmEventCursor),
		now:                 time.Now,
		eventIDsHashedFrom:  time.Now(),
		tenantID:            tenantID,
		tenantName:          tenantName,
		client:              client,
		instanceConfig:      instanceCfg,
		logger:              log,
		eventRepo:           eventRepo,
		generalEventRepo:    generalEventRepo,
		metricsRepo:         metricsRepo,
		healthRepo:          healthRepo,
		realmRepo:           realmRepo,
		eventAlertConverter: eventAlertConverter,
		healthAlertManager:  healthAlertManager,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start begins the monitoring process
func (m *Monitor) Start(ctx context.Context) error {
	m.logger.Info("Starting Keycloak monitoring service")

	// Start all polling workers
	for _, poll := range []func(context.Context){
		m.pollEvents,
		m.pollMetrics,
		m.pollHealth,
		m.pollRealmInfo,
	} {
		poll := poll
		m.workers.Go(func() { poll(ctx) })
	}

	m.logger.Info("Keycloak monitoring service started successfully")
	return nil
}

// Stop gracefully stops the monitoring process
func (m *Monitor) Stop() error {
	m.logger.Info("Stopping Keycloak monitoring service")
	// Wait for all workers to finish (with timeout). A done channel used to
	// stand in for this, but nothing ever closed it, so every shutdown sat
	// here for the full 10s and then logged a spurious stop timeout.
	if m.workers.Drain(10 * time.Second) {
		m.logger.Info("Keycloak monitoring service stopped gracefully")
	} else {
		m.logger.Warn("Keycloak monitoring service stop timeout")
	}

	return nil
}

// pollEvents periodically fetches and stores Keycloak events
// SetMetrics attaches a metrics recorder. Passing nil installs a no-op
// recorder so call sites never need a nil check.
func (m *Monitor) SetMetrics(r metrics.Recorder) {
	if r == nil {
		r = metrics.NopRecorder{}
	}
	m.metrics = r
}

// recorder returns the configured recorder, falling back to a no-op when the
// monitor was constructed without one.
func (m *Monitor) recorder() metrics.Recorder {
	if m.metrics == nil {
		return metrics.NopRecorder{}
	}
	return m.metrics
}

func (m *Monitor) pollEvents(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.EventsInterval)
	defer ticker.Stop()

	m.logger.Info("Starting events polling worker",
		logger.Str("interval", m.instanceConfig.Polling.EventsInterval.String()))

	// Do first collection immediately
	m.collectEvents(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectEvents(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Events polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Events polling worker cancelled")
			return
		}
	}
}

// collectEvents fetches and stores events from Keycloak
func (m *Monitor) collectEvents(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "events", time.Since(start))
	}()

	m.logger.Debug("Starting event collection cycle",
		logger.Str("tenant_id", m.tenantID),
		logger.Str("tenant_name", m.tenantName))

	realms := m.getRealmsToMonitor(ctx)
	m.logger.Debug("Realms to monitor",
		logger.Str("tenant_id", m.tenantID),
		logger.Int("realm_count", len(realms)),
		logger.Strs("realms", realms))
	if len(realms) == 0 {
		m.logger.Warn("No realms to monitor")
		return
	}

	var storage EventStorageOutcome
	defer func() {
		if m.eventStorage != nil {
			m.eventStorage.ReportEventStorage(ctx, m.tenantID, m.tenantName, storage)
		}
	}()

	now := m.now()
	floor := eventLookbackFloor(now, m.instanceConfig.Events.LookbackDuration)
	lateCommitWindow := m.lateCommitWindow()
	m.forgetUnmonitoredRealms(realms)

	for _, realmName := range realms {
		cursor, ok := m.eventCursors[realmName]
		if !ok {
			cursor = newRealmEventCursor(floor, now)
			m.eventCursors[realmName] = cursor
		}

		due, expired := cursor.takeRetries(floor)
		if expired > 0 {
			m.logger.Warn("Dropped Keycloak events that kept failing to save; they are older than the lookback window",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realmName),
				logger.Int("count", expired))
		}
		if len(due) > 0 {
			m.queueEventRetries(realmName, cursor, m.processEvents(ctx, realmName, due, &storage))
		}

		report := &eventFetchReport{}
		if _, behind := cursor.startSweepIfDue(now, lateCommitWindow); behind {
			m.logger.Warn("Keycloak event sweep is still running when the next one is due; this realm writes events faster than they are swept, or Keycloak is failing the requests",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realmName))
		}
		if cursor.sweeping {
			r := m.readEvents(ctx, realmName, cursor, cursor.sweepFrom, cursor.sweepFirst, maxEventPagesPerSweepCycle, cursor.since.UnixMilli(), now, report, &storage)
			if r.complete {
				cursor.finishSweep()
			} else {
				cursor.sweepFrom, cursor.sweepFirst = r.next, r.nextFirst
			}
		}

		r := m.readEvents(ctx, realmName, cursor, cursor.since, 0, maxEventPagesPerCycle, 0, now, report, &storage)
		cursor.advance(r.newest, r.complete, now)
		cursor.prune(lateCommitWindow, now)
		m.reportEventFetch(realmName, cursor, report, lateCommitWindow, now)
	}
}

// lateCommitWindow is the tenant's Keycloak transaction timeout: how long
// after its time a successful event may become visible.
func (m *Monitor) lateCommitWindow() time.Duration {
	if w := m.instanceConfig.Events.LateCommitWindow; w > 0 {
		return w
	}
	return defaultLateCommitWindow
}

// eventRead is where one readEvents call got to.
type eventRead struct {
	newest   int64 // newest event time read (ms), 0 if none
	complete bool  // reached the newest event, or stopAt
	// next and nextFirst continue a read that did not complete.
	next      time.Time
	nextFirst int
}

// eventFetchReport collects what a realm's fetches in one cycle saw about
// event timing.
type eventFetchReport struct {
	maxDelay   time.Duration // longest least visibility delay of a new event
	late       int           // new events seen with a known delay
	maxAhead   time.Duration // furthest an event was stamped ahead of now
	aheadCount int
}

// readEvents pages through the realm's events from since (offset first), up to
// maxPages pages or, when stopAt is set, until a page reaches that time (ms).
// It processes the events not seen before and records what it read.
func (m *Monitor) readEvents(ctx context.Context, realmName string, cursor *realmEventCursor, since time.Time, first, maxPages int, stopAt int64, now time.Time, report *eventFetchReport, storage *EventStorageOutcome) eventRead {
	r := eventRead{next: since, nextFirst: first}
	// The startup backfill reads the day for the first time, so a read of it
	// cut short is a loss like a poll's, not a sweep's.
	read := keycloakadmin.EventsPoll
	if stopAt > 0 && !cursor.backfilling {
		read = keycloakadmin.EventsSweep
	}
	for page := 0; page < maxPages; page++ {
		events, full, err := m.client.GetEventsSince(ctx, realmName, r.next, r.nextFirst, read)
		if err != nil {
			m.logger.Error("Failed to fetch events from Keycloak API",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realmName),
				logger.Err(err))
			return r
		}
		var pageNewest int64
		for _, e := range events {
			if e.Time > pageNewest {
				pageNewest = e.Time
			}
		}
		if pageNewest > r.newest {
			r.newest = pageNewest
		}

		fresh := cursor.unseen(events)
		for _, e := range fresh {
			if ahead := time.UnixMilli(e.Time).Sub(now); ahead > eventFutureTolerance {
				report.aheadCount++
				if ahead > report.maxAhead {
					report.maxAhead = ahead
				}
			}
			if delay, known := cursor.visibilityDelay(e.Time); known && delay > 0 {
				report.late++
				if delay > report.maxDelay {
					report.maxDelay = delay
				}
				m.recorder().ObserveEventVisibilityDelay(m.tenantID, realmName, delay)
			}
		}
		m.logger.Debug("Fetched events from Keycloak API",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("event_count", len(events)),
			logger.Int("new_event_count", len(fresh)))
		if len(fresh) > 0 {
			m.queueEventRetries(realmName, cursor, m.processEvents(ctx, realmName, fresh, storage))
		}

		// Recorded after the delays above, which compare with earlier reads.
		// A full page may stop partway through its newest millisecond, so it
		// only vouches for the ones before.
		upto := pageNewest - 1
		if !full {
			upto = openEnded
		}
		cursor.cover(now, r.next.UnixMilli(), upto)

		if !full || (stopAt > 0 && pageNewest >= stopAt) {
			r.complete = true
			return r
		}
		// The next page starts at this page's newest time rather than at an
		// offset: Keycloak orders by time alone, so events sharing a
		// millisecond can swap places between requests, and an offset could
		// skip one at a page boundary. The ones fetched again are in seen.
		// Only a page entirely inside one millisecond falls back to the offset.
		if pageNewest > r.next.UnixMilli() {
			r.next, r.nextFirst = time.UnixMilli(pageNewest), 0
		} else {
			r.nextFirst += len(events)
		}
	}
	return r
}

// reportEventFetch warns when events became visible late enough to approach
// the late commit window, or were stamped ahead of the monitor's clock. A
// realm warns at most once per eventTimingWarnEvery, with the worst seen since.
func (m *Monitor) reportEventFetch(realmName string, cursor *realmEventCursor, report *eventFetchReport, lateCommitWindow time.Duration, now time.Time) {
	t := &cursor.timing
	if report.maxDelay > lateCommitWindow/2 {
		t.late += report.late
		if report.maxDelay > t.maxDelay {
			t.maxDelay = report.maxDelay
		}
	}
	t.aheadCount += report.aheadCount
	if report.maxAhead > t.maxAhead {
		t.maxAhead = report.maxAhead
	}
	if t.late == 0 && t.aheadCount == 0 {
		return
	}
	if !cursor.lastTimingWarn.IsZero() && now.Sub(cursor.lastTimingWarn) < eventTimingWarnEvery {
		return
	}
	if t.late > 0 {
		m.logger.Warn("Keycloak events became visible long after their time; if this nears late_commit_window, raise it to Keycloak's transaction-default-timeout or events can be missed",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Str("max_delay", t.maxDelay.Round(time.Second).String()),
			logger.Str("late_commit_window", lateCommitWindow.String()),
			logger.Int("late_events", t.late))
	}
	if t.aheadCount > 0 {
		m.logger.Warn("Keycloak events are stamped ahead of this server's clock; check the clocks of the Keycloak nodes and this server",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Str("max_ahead", t.maxAhead.Round(time.Second).String()),
			logger.Int("count", t.aheadCount))
	}
	cursor.timing, cursor.lastTimingWarn = eventFetchReport{}, now
}

// queueEventRetries keeps failed events for the next cycle.
func (m *Monitor) queueEventRetries(realmName string, cursor *realmEventCursor, failed []*keycloakadmin.EventRepresentation) {
	if dropped := cursor.addRetries(failed); dropped > 0 {
		m.logger.Warn("Too many Keycloak events waiting for a retry; dropped the oldest",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("count", dropped))
	}
}

// forgetUnmonitoredRealms drops the cursors of realms no longer monitored. A
// realm that comes back starts over like after a restart.
func (m *Monitor) forgetUnmonitoredRealms(realms []string) {
	keep := make(map[string]bool, len(realms))
	for _, r := range realms {
		keep[r] = true
	}
	for r, cursor := range m.eventCursors {
		if keep[r] {
			continue
		}
		if len(cursor.retry) > 0 {
			m.logger.Warn("Realm is no longer monitored; dropping its Keycloak events waiting for a retry",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", r),
				logger.Int("count", len(cursor.retry)))
		}
		delete(m.eventCursors, r)
	}
}

// eventIDs returns the id each event is stored under: eventKey, unless the
// event may already be stored under an id the poller made because Keycloak
// gave it none, in which case that id is kept so the event is not stored
// twice:
//   - An event without a Keycloak id may be stored under its legacyEventKey,
//     as every such event was before the hashed id, up to the newest legacy
//     row or shortly after this monitor started (a Keycloak clock ahead of
//     the monitor's).
//   - An event with a Keycloak id stamped no later than the newest event
//     stored without one may be stored under the id it had then (Keycloak
//     26.2 started returning ids, also for events stored before).
//
// Events stamped later cannot be stored so, and need no lookup. When the
// lookup fails, the events it was for are in unresolved, to be retried rather
// than stored under a guess that could store them twice.
func (m *Monitor) eventIDs(ctx context.Context, realmName string, events []*keycloakadmin.EventRepresentation) (ids []string, unresolved map[int]bool, err error) {
	ids = make([]string, len(events))
	legacyUntil, idlessUntil := m.idlessEventsUntil(ctx, realmName)
	ceiling := m.now().Add(eventFutureTolerance).UnixMilli()
	// candidates[i] are the ids event i may be stored under, in order.
	candidates := make(map[int][]string)
	var lookup []string
	asked := make(map[string]bool)
	var from, to int64
	for i, e := range events {
		ids[i] = eventKey(e)
		var keys []string
		switch {
		case e.ID == "" && e.Time <= legacyUntil:
			keys = []string{legacyEventKey(e)}
		case e.ID != "" && e.Time <= idlessUntil:
			bare := *e
			bare.ID = ""
			keys = []string{eventKey(&bare), legacyEventKey(e)}
		}
		if e.ID == "" && e.Time > m.idlessSeen {
			m.idlessSeen = min(e.Time, ceiling)
		}
		if len(keys) == 0 {
			continue
		}
		candidates[i] = keys
		if len(lookup) == 0 || e.Time < from {
			from = e.Time
		}
		if len(lookup) == 0 || e.Time > to {
			to = e.Time
		}
		for _, key := range keys {
			if !asked[key] {
				asked[key] = true
				lookup = append(lookup, key)
			}
		}
	}
	if len(lookup) == 0 {
		return ids, nil, nil
	}
	stored, err := m.eventRepo.StoredEventIDs(ctx, m.tenantID, lookup, time.UnixMilli(from), time.UnixMilli(to))
	if err != nil {
		m.logger.Warn("Could not look up events stored under an id the poller made; retrying them on the next cycle",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("count", len(candidates)),
			logger.Err(err))
		unresolved = make(map[int]bool, len(candidates))
		for i := range candidates {
			unresolved[i] = true
		}
		return ids, unresolved, err
	}
	for i, keys := range candidates {
		for _, key := range keys {
			if stored[key] {
				ids[i] = key
				break
			}
		}
	}
	return ids, nil, nil
}

// idlessEventsUntil returns up to when (ms) events may be stored under an id
// the poller made: legacy ids, and any such id. They come from the rows
// stored before this monitor started, as far back as a poll reads again, read
// once, and from the events seen since. Times ahead of the clock by more than
// eventFutureTolerance are capped, so one wrong stamp does not send every
// later event to a lookup. If the read fails, every event from before the
// start is checked.
func (m *Monitor) idlessEventsUntil(ctx context.Context, realmName string) (legacyUntil, idlessUntil int64) {
	started := m.eventIDsHashedFrom
	if !m.idlessStoredRead {
		m.idlessStoredRead = true
		// Back to anything a poll can read again after the start: the day's
		// backfill from the lookback floor, or the first sweep, which reaches
		// back the late commit window and is not held to the floor.
		since := eventLookbackFloor(started, m.instanceConfig.Events.LookbackDuration)
		if sweep := started.Add(-m.lateCommitWindow() - eventClockMargin); sweep.Before(since) {
			since = sweep
		}
		legacy, hashed, err := m.eventRepo.IDlessEventTimes(ctx, m.tenantID, since)
		if err != nil {
			m.logger.Warn("Could not look up events stored without a Keycloak id; checking every event from before the start",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realmName),
				logger.Err(err))
			legacy, hashed = started, started
		}
		ceiling := m.now().Add(eventFutureTolerance)
		cap := func(t time.Time) int64 {
			if t.IsZero() {
				return 0
			}
			if t.After(ceiling) {
				t = ceiling
			}
			return t.UnixMilli()
		}
		m.legacyStored, m.idlessStored = cap(legacy), max(cap(legacy), cap(hashed))
	}
	// Shortly after the start too: the old monitor's last rows can be stamped
	// a little ahead of this monitor's clock.
	legacyUntil = max(m.legacyStored, started.Add(eventClockMargin).UnixMilli())
	return legacyUntil, max(m.idlessStored, m.idlessSeen)
}

// processEvents saves events to keycloak_events, converts them to alerts and
// merges them into the events table. It returns the events any step failed
// for, so they can be retried.
func (m *Monitor) processEvents(ctx context.Context, realmName string, events []*keycloakadmin.EventRepresentation, storage *EventStorageOutcome) []*keycloakadmin.EventRepresentation {
	// Convert Keycloak events to domain types
	keycloakEvents := make([]*domain.KeycloakEvent, 0, len(events))
	generalEvents := make([]*domain.Event, 0, len(events))

	ids, unresolved, lookupErr := m.eventIDs(ctx, realmName, events)
	var held []*keycloakadmin.EventRepresentation
	if len(unresolved) > 0 {
		var keep []*keycloakadmin.EventRepresentation
		var keepIDs []string
		for i, e := range events {
			if unresolved[i] {
				held = append(held, e)
			} else {
				keep = append(keep, e)
				keepIDs = append(keepIDs, ids[i])
			}
		}
		events, ids = keep, keepIDs
		// Not written yet; counted as failed so a lookup that keeps failing
		// raises the storage alert.
		storage.add(len(held), len(held), lookupErr)
		m.recorder().AddEventWritesFailed(m.tenantID, "keycloak_events", len(held))
		if len(events) == 0 {
			return held
		}
	}
	for i, event := range events {
		eventTime := time.UnixMilli(event.Time)
		eventID := ids[i]

		// Use username/email from the event payload only. Keycloak records
		// "username" in the details for user-initiated events (LOGIN,
		// LOGIN_ERROR, etc.). Token-refresh and code-exchange events
		// (REFRESH_TOKEN, CODE_TO_TOKEN) do not include a username in
		// Details, so those events will have an empty username field.
		// A fallback GET /users/{id} call is intentionally omitted: on a
		// realm backed by a federated user storage provider it triggers a
		// lookup on that provider while a service-account session is
		// active in context, which makes the provider log a warning for
		// every event polled. Where an event has an AMFA counterpart the
		// name is recovered instead by ReconcileAMFAMerges, which copies
		// it from the row AMFA already resolved.
		username := event.Details["username"]
		email := event.Details["email"]

		// Create Keycloak-specific event (using local Event type)
		kcEvent := &domain.KeycloakEvent{
			TenantID:   m.tenantID,
			EventID:    eventID,
			Time:       eventTime,
			RealmID:    event.RealmID,
			RealmName:  realmName,
			ClientID:   event.ClientID,
			SessionID:  event.SessionID,
			IPAddress:  event.IPAddress,
			EventType:  event.Type,
			EventError: event.Error,
			UserID:     event.UserID,
			Username:   username,
			Email:      email,
			Details:    event.Details,
			Success:    event.Error == "",
		}
		keycloakEvents = append(keycloakEvents, kcEvent)

		// Create general event for dashboard visibility
		description := fmt.Sprintf("Keycloak %s event in realm %s", event.Type, realmName)
		if event.Error != "" {
			description += fmt.Sprintf(" (Error: %s)", event.Error)
		}

		severity := "info"
		if event.Error != "" {
			severity = "error"
		} else if event.Type == "LOGIN" || event.Type == "LOGOUT" {
			severity = "info"
		}

		generalEvent := &domain.Event{
			TenantID:     m.tenantID,
			EventID:      eventID,
			Timestamp:    eventTime,
			Type:         event.Type,
			Category:     "authentication",
			Severity:     severity,
			Description:  description,
			Source:       eventsource.SourceForKeycloakRealm(realmName),
			SourceIP:     event.IPAddress,
			SourceSystem: eventsource.SourceSystemKeycloak,
			UserID:       event.UserID,
			Username:     username,
			Email:        email,
			ClientID:     event.ClientID,
			Location:     "",
			RawData:      "",
			Status:       "processed",
			CreatedAt:    eventTime,
			UpdatedAt:    eventTime,
		}
		// The Adaptive MFA Keycloak extension stamps these onto interactive
		// login events. amfa_event_id is the merge key with AMFA; risk_level
		// is already computed, so we surface it directly without an AMFA join.
		if amfaID := event.Details["amfa_event_id"]; amfaID != "" {
			generalEvent.AMFAEventID = &amfaID
		}
		if rl := event.Details["risk_level"]; rl != "" {
			if riskVal, perr := strconv.Atoi(rl); perr == nil {
				generalEvent.RiskLevel = &riskVal
			}
		}
		generalEvents = append(generalEvents, generalEvent)
	}

	// Save Keycloak events to database
	m.logger.Debug("Saving Keycloak events to keycloak_events table",
		logger.Str("tenant_id", m.tenantID),
		logger.Str("realm", realmName),
		logger.Int("count", len(keycloakEvents)))

	if err := m.eventRepo.SaveEvents(ctx, keycloakEvents); err != nil {
		m.recorder().IncPollCycleError(m.tenantID, "events")
		m.recorder().AddEventWritesFailed(m.tenantID, "keycloak_events", len(keycloakEvents))
		storage.add(len(keycloakEvents), len(keycloakEvents), err)
		m.logger.Error("Failed to save Keycloak events; they are retried on the next cycle while inside the lookback window",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("count", len(keycloakEvents)),
			logger.Err(err))
		return append(events, held...)
	}
	storage.add(len(keycloakEvents), 0, nil)

	m.recorder().AddEventsCollected(m.tenantID, realmName, len(keycloakEvents))

	m.logger.Debug("Keycloak events saved successfully to keycloak_events table",
		logger.Str("tenant_id", m.tenantID),
		logger.Str("realm", realmName),
		logger.Int("count", len(keycloakEvents)))

	// Convert events to alerts
	failedAt := make(map[int]bool)
	if m.eventAlertConverter != nil {
		for i, kcEvent := range keycloakEvents {
			if err := m.eventAlertConverter.ConvertEvent(ctx, kcEvent); err != nil {
				failedAt[i] = true
				m.logger.Error("Failed to convert event to alert",
					logger.Str("event_id", kcEvent.EventID),
					logger.Str("event_type", kcEvent.EventType),
					logger.Err(err))
				// Continue processing other events even if one fails
			}
		}
	}

	// Save general events to database for dashboard visibility
	m.logger.Debug("Saving general events to events table",
		logger.Str("tenant_id", m.tenantID),
		logger.Str("realm", realmName),
		logger.Int("count", len(generalEvents)))

	savedCount := 0
	var firstErr error
	var firstFailedID string
	for i, generalEvent := range generalEvents {
		if err := m.generalEventRepo.MergeKeycloakEvent(ctx, generalEvent); err != nil {
			// One line per event at debug; the summary below is the error.
			m.logger.Debug("Failed to save event to the events table",
				logger.Str("tenant_id", m.tenantID),
				logger.Int("index", i),
				logger.Str("event_id", generalEvent.EventID),
				logger.Str("source", generalEvent.Source),
				logger.Err(err))
			failedAt[i] = true
			if firstErr == nil {
				firstErr, firstFailedID = err, generalEvent.EventID
			}
			// Continue with other events
		} else {
			savedCount++
		}
	}
	failed := len(generalEvents) - savedCount
	storage.add(len(generalEvents), failed, firstErr)
	if failed > 0 {
		m.recorder().AddEventWritesFailed(m.tenantID, "events", failed)
		m.logger.Error("Failed to save events to the events table; they are retried on the next cycle while inside the lookback window",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Int("failed", failed),
			logger.Int("total", len(generalEvents)),
			logger.Str("first_failed_event_id", firstFailedID),
			logger.Err(firstErr))
	}

	m.logger.Debug("Event collection cycle completed",
		logger.Str("tenant_id", m.tenantID),
		logger.Str("realm", realmName),
		logger.Int("keycloak_events_saved", len(keycloakEvents)),
		logger.Int("general_events_saved", savedCount))

	failedEvents := held
	for i := range events {
		if failedAt[i] {
			failedEvents = append(failedEvents, events[i])
		}
	}
	return failedEvents

}

// pollMetrics periodically fetches and stores Keycloak metrics
func (m *Monitor) pollMetrics(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.MetricsInterval)
	defer ticker.Stop()

	m.logger.Info("Starting metrics polling worker",
		logger.Str("interval", m.instanceConfig.Polling.MetricsInterval.String()))

	// Do first collection immediately
	m.collectMetrics(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectMetrics(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Metrics polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Metrics polling worker cancelled")
			return
		}
	}
}

// collectMetrics fetches and stores metrics from Keycloak
func (m *Monitor) collectMetrics(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "metrics", time.Since(start))
	}()

	m.logger.Debug("Collecting Keycloak metrics")

	realms := m.getRealmsToMonitor(ctx)
	if len(realms) == 0 {
		m.logger.Warn("No realms to monitor")
		return
	}

	now := time.Now()

	for _, realmName := range realms {
		// Get user counts. enabled/disabled come from COUNT-only queries
		// (/users/count?enabled=true|false); total is derived from their sum so
		// all three always reconcile and share one scope, while avoiding a
		// separate unfiltered /users/count round-trip.
		enabledUsers, disabledUsers, err := m.client.GetEnabledUsersCount(ctx, realmName)
		if err != nil {
			m.logger.Error("Failed to get users count",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}
		totalUsers := enabledUsers + disabledUsers

		// Get session counts
		activeSessions, offlineSessions, err := m.client.GetSessionsCount(ctx, realmName)
		if err != nil {
			m.logger.Warn("Failed to get sessions count",
				logger.Str("realm", realmName),
				logger.Err(err))
			// Continue with 0 values
		}

		// Get client count
		clients, err := m.client.GetClients(ctx, realmName)
		if err != nil {
			m.logger.Error("Failed to get clients",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}
		totalClients := len(clients)

		// Count recent events by type
		lookback := time.Now().Add(-m.instanceConfig.Polling.MetricsInterval)
		loginEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "LOGIN", lookback, now)
		logoutEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "LOGOUT", lookback, now)
		failedLoginEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "LOGIN_ERROR", lookback, now)
		registerEvents, _ := m.eventRepo.CountEventsByType(ctx, m.tenantID, realmName, "REGISTER", lookback, now)

		// Create metrics snapshot (using local Metrics type)
		metrics := &domain.KeycloakMetrics{
			TenantID:          m.tenantID,
			Time:              now,
			RealmName:         realmName,
			TotalUsers:        totalUsers,
			EnabledUsers:      enabledUsers,
			DisabledUsers:     disabledUsers,
			ActiveSessions:    activeSessions,
			OfflineSessions:   offlineSessions,
			TotalClients:      totalClients,
			LoginEvents:       int(loginEvents),
			LogoutEvents:      int(logoutEvents),
			FailedLoginEvents: int(failedLoginEvents),
			RegisterEvents:    int(registerEvents),
		}

		// Save metrics
		if err := m.metricsRepo.SaveMetrics(ctx, metrics); err != nil {
			m.logger.Error("Failed to save metrics",
				logger.Str("realm", realmName),
				logger.Err(err))
			continue
		}

		m.logger.Info("Collected metrics",
			logger.Str("realm", realmName),
			logger.Int("total_users", totalUsers),
			logger.Int("active_sessions", activeSessions),
			logger.Int("total_clients", totalClients))
	}
}

// pollHealth periodically checks Keycloak server health
func (m *Monitor) pollHealth(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.HealthInterval)
	defer ticker.Stop()

	m.logger.Info("Starting health polling worker",
		logger.Str("interval", m.instanceConfig.Polling.HealthInterval.String()))

	// Do first check immediately
	m.collectHealth(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectHealth(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Health polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Health polling worker cancelled")
			return
		}
	}
}

// collectHealth performs health check and stores result
func (m *Monitor) collectHealth(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "health", time.Since(start))
	}()

	m.logger.Debug("Performing Keycloak health check")

	healthMetrics := m.client.GetHealthMetrics(ctx)

	health := &domain.KeycloakHealth{
		TenantID:      m.tenantID,
		Time:          time.Now(),
		Status:        healthMetrics.Status,
		ResponseTime:  healthMetrics.ResponseTime,
		ServerVersion: healthMetrics.ServerVersion,
		UptimeMillis:  healthMetrics.UptimeMillis,
		MemoryUsed:    healthMetrics.MemoryUsed,
		MemoryMax:     healthMetrics.MemoryMax,
		MemoryFree:    healthMetrics.MemoryFree,
		ErrorMessage:  healthMetrics.Error,
	}

	if err := m.healthRepo.SaveHealth(ctx, health); err != nil {
		m.logger.Error("Failed to save health check", logger.Err(err))
		return
	}

	m.logger.Info("Health check completed",
		logger.Str("status", health.Status),
		logger.Int64("response_time_ms", health.ResponseTime),
		logger.Str("server_version", health.ServerVersion))

	// Process health status for alerting
	if m.healthAlertManager != nil {
		if err := m.healthAlertManager.ProcessHealthStatus(
			ctx,
			m.tenantID,
			m.tenantName,
			health.Status,
			health.ErrorMessage,
			int(health.ResponseTime),
		); err != nil {
			m.logger.Error("Failed to process health status for alerting",
				logger.Err(err),
				logger.Str("tenant_id", m.tenantID))
			// Don't return - health check was saved successfully
		}
	}

	// Keep the tenant record's health status current so the UI can distinguish
	// a broken connection from a genuinely empty Keycloak. Best-effort.
	if m.healthUpdater != nil {
		tenantStatus := mapHealthStatusToTenant(health.Status)
		message := "Keycloak reachable"
		if health.ErrorMessage != "" {
			message = health.ErrorMessage
		}
		if err := m.healthUpdater.UpdateHealth(ctx, m.tenantID, tenantStatus, message); err != nil {
			m.logger.Warn("Failed to update tenant health status",
				logger.Str("tenant_id", m.tenantID), logger.Err(err))
		}
		if health.Status == "DOWN" && health.ErrorMessage != "" {
			if err := m.healthUpdater.UpdateError(ctx, m.tenantID, health.ErrorMessage); err != nil {
				m.logger.Warn("Failed to update tenant last_error",
					logger.Str("tenant_id", m.tenantID), logger.Err(err))
			}
		} else if health.Status == "UP" {
			// Recovered: clear any stale error so the tenant is no longer flagged.
			if err := m.healthUpdater.UpdateError(ctx, m.tenantID, ""); err != nil {
				m.logger.Warn("Failed to clear tenant last_error",
					logger.Str("tenant_id", m.tenantID), logger.Err(err))
			}
		}
	}
}

// pollRealmInfo periodically updates realm information
func (m *Monitor) pollRealmInfo(ctx context.Context) {
	ticker := time.NewTicker(m.instanceConfig.Polling.RealmInfoInterval)
	defer ticker.Stop()

	m.logger.Info("Starting realm info polling worker",
		logger.Str("interval", m.instanceConfig.Polling.RealmInfoInterval.String()))

	// Do first collection immediately
	m.collectRealmInfo(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectRealmInfo(ctx)
		case <-m.workers.Stopping():
			m.logger.Info("Realm info polling worker stopped")
			return
		case <-ctx.Done():
			m.logger.Info("Realm info polling worker cancelled")
			return
		}
	}
}

// collectRealmInfo fetches and stores realm information
func (m *Monitor) collectRealmInfo(ctx context.Context) {
	start := time.Now()
	defer func() {
		m.recorder().ObservePollCycle(m.tenantID, "realm_info", time.Since(start))
	}()

	m.logger.Debug("Collecting realm information")

	var realms []*keycloakadmin.RealmRepresentation
	var err error
	access := realmAccess{read: map[string]bool{}, unreadable: map[string]bool{}, gone: map[string]bool{}}

	// Get realms to monitor
	if len(m.instanceConfig.Realms) > 0 {
		// Fetch specific realms
		for _, realmName := range m.instanceConfig.Realms {
			realm, err := m.client.GetRealmInfo(ctx, realmName)
			if err != nil {
				if errors.Is(err, keycloakadmin.ErrNotFound) {
					access.gone[realmName] = true
				}
				m.logger.Error("Failed to get realm info",
					logger.Str("realm", realmName),
					logger.Err(err))
				continue
			}
			realms = append(realms, realm)
		}
	} else {
		// Fetch all realms
		realms, err = m.client.GetAllRealms(ctx)
		if err != nil {
			m.logger.Error("Failed to get all realms", logger.Err(err))
			return
		}
	}

	now := time.Now()

	for _, realm := range realms {
		// Keycloak answers for a realm the client may not view with a stub
		// that has no id and none of the settings. Saving it would replace
		// the realm's stored row with an empty one, so the row is left as it
		// was until the client can read the realm again.
		if realm.ID == "" {
			access.unreadable[realm.Realm] = true
			continue
		}
		access.read[realm.Realm] = true

		realmInfo := &domain.KeycloakRealmInfo{
			TenantID:                  m.tenantID,
			RealmID:                   realm.ID,
			RealmName:                 realm.Realm,
			DisplayName:               realm.DisplayName,
			Enabled:                   realm.Enabled,
			SslRequired:               realm.SslRequired,
			RegistrationAllowed:       realm.RegistrationAllowed,
			RememberMe:                realm.RememberMe,
			VerifyEmail:               realm.VerifyEmail,
			LoginWithEmailAllowed:     realm.LoginWithEmailAllowed,
			DuplicateEmailsAllowed:    realm.DuplicateEmailsAllowed,
			ResetPasswordAllowed:      realm.ResetPasswordAllowed,
			EditUsernameAllowed:       realm.EditUsernameAllowed,
			BruteForceProtected:       realm.BruteForceProtected,
			EventsEnabled:             realm.EventsEnabled,
			EventsListeners:           realm.EventsListeners,
			EnabledEventTypes:         realm.EnabledEventTypes,
			AdminEventsEnabled:        realm.AdminEventsEnabled,
			AdminEventsDetailsEnabled: realm.AdminEventsDetailsEnabled,
			LastChecked:               now,
			IsHealthy:                 realm.Enabled,
			HealthMessage:             "",
		}

		if !realm.Enabled {
			realmInfo.HealthMessage = "Realm is disabled"
		}

		// Check if events are properly configured
		if realm.Enabled && !realm.EventsEnabled {
			if realmInfo.HealthMessage != "" {
				realmInfo.HealthMessage += "; "
			}
			realmInfo.HealthMessage += "Events are not enabled"
		}
		if realm.Enabled && realm.EventsEnabled && len(realm.EventsListeners) == 0 {
			if realmInfo.HealthMessage != "" {
				realmInfo.HealthMessage += "; "
			}
			realmInfo.HealthMessage += "No event listeners configured"
		}

		if err := m.realmRepo.SaveRealm(ctx, realmInfo); err != nil {
			m.logger.Error("Failed to save realm info",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realm.Realm),
				logger.Err(err))
			continue
		}

		m.logger.Info("Updated realm info",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realm.Realm),
			logger.Bool("enabled", realm.Enabled))
	}

	// Only GetAllRealms returns Keycloak's complete list, so only then is a
	// realm missing from it known to be deleted. With configured realms each
	// one is fetched separately and a failed fetch is skipped above, so absence
	// proves nothing and nothing is retired. The ids come from what Keycloak
	// listed, not from what saved: a realm whose save failed still exists.
	if len(m.instanceConfig.Realms) == 0 {
		m.retireDeletedRealms(ctx, realms, access)
	}
	m.markUnreadable(ctx, access, now)
	m.reportRealmAccess(access)
}

// markUnreadable records on the stored rows that the client could not read
// these realms, so the pages showing them can say their data is not current.
// The rows keep the settings last read, and the next full read clears the
// mark. A realm never read has no row, and nothing to mark.
func (m *Monitor) markUnreadable(ctx context.Context, access realmAccess, now time.Time) {
	if len(access.unreadable) == 0 {
		return
	}
	names := make([]string, 0, len(access.unreadable))
	for realm := range access.unreadable {
		names = append(names, realm)
	}
	slices.Sort(names)
	if err := m.realmRepo.MarkRealmsUnreadable(ctx, m.tenantID, names, now); err != nil {
		m.logger.Error("Failed to mark unreadable realms",
			logger.Str("tenant_id", m.tenantID),
			logger.Strs("realms", names),
			logger.Err(err))
	}
}

// realmAccess records what one realm-info collection learned about each
// realm: read with its settings, answered only as a stub, or deleted.
type realmAccess struct {
	read       map[string]bool
	unreadable map[string]bool
	gone       map[string]bool
}

// retireDeletedRealms soft-deletes the tenant's realms that Keycloak no longer
// lists, so a realm deleted there stops being listed, shown as healthy and
// polled here. A realm the client cannot view is not deleted: Keycloak lists
// it as a stub, or, when the client lost all its roles there, leaves it out
// of the list but still answers 200 for it. Only a 404 says a realm is gone,
// so a stored realm missing from the list is looked up before it is retired.
func (m *Monitor) retireDeletedRealms(ctx context.Context, realms []*keycloakadmin.RealmRepresentation, access realmAccess) {
	ids := make([]string, 0, len(realms))
	for _, realm := range realms {
		if realm != nil && realm.ID != "" {
			ids = append(ids, realm.ID)
		}
	}
	stored, err := m.realmRepo.GetRealms(ctx, m.tenantID)
	if err != nil {
		// Without the stored rows an unreadable realm's row cannot be told
		// from a deleted one, so nothing is retired this time.
		m.logger.Error("Failed to load stored realms; not retiring any",
			logger.Str("tenant_id", m.tenantID),
			logger.Err(err))
		return
	}
	for _, row := range stored {
		if row.RealmID == "" || slices.Contains(ids, row.RealmID) || access.read[row.RealmName] {
			continue
		}
		if access.unreadable[row.RealmName] || m.stillExists(ctx, row.RealmName, access) {
			ids = append(ids, row.RealmID)
		}
	}
	retired, err := m.realmRepo.RetireRealmsNotIn(ctx, m.tenantID, ids)
	if err != nil {
		m.logger.Error("Failed to retire realms deleted in Keycloak",
			logger.Str("tenant_id", m.tenantID),
			logger.Err(err))
		return
	}
	if retired > 0 {
		m.logger.Info("Retired realms deleted in Keycloak",
			logger.Str("tenant_id", m.tenantID),
			logger.Int64("realms", retired))
	}
}

// stillExists reports whether a realm Keycloak did not list is still there,
// recording in access whether it is gone or unreadable. A lookup that fails
// for another reason counts as existing: retiring on a guess would drop a
// realm that may only be unreadable.
func (m *Monitor) stillExists(ctx context.Context, realmName string, access realmAccess) bool {
	realm, err := m.client.GetRealmInfo(ctx, realmName)
	switch {
	case errors.Is(err, keycloakadmin.ErrNotFound):
		access.gone[realmName] = true
		return false
	case err != nil:
		m.logger.Warn("Could not tell whether an unlisted realm still exists; keeping it",
			logger.Str("tenant_id", m.tenantID),
			logger.Str("realm", realmName),
			logger.Err(err))
	case realm.ID == "":
		access.unreadable[realmName] = true
	}
	return true
}

// reportRealmAccess logs a realm once when the client stops being able to
// view it and once when it can again, rather than on every collection. A
// realm this collection learned nothing about keeps its previous state; a
// deleted one is dropped without claiming it is readable again.
func (m *Monitor) reportRealmAccess(access realmAccess) {
	for realm := range access.unreadable {
		if !m.unreadableRealms[realm] {
			m.logger.Warn("Monitoring client cannot view this realm's settings; keeping its stored realm information as it was",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realm))
		}
	}
	next := make(map[string]bool, len(access.unreadable))
	for realm := range m.unreadableRealms {
		switch {
		case access.read[realm]:
			m.logger.Info("Monitoring client can view this realm's settings again",
				logger.Str("tenant_id", m.tenantID),
				logger.Str("realm", realm))
		case access.gone[realm]:
		default:
			next[realm] = true
		}
	}
	for realm := range access.unreadable {
		next[realm] = true
	}
	m.unreadableRealms = next
}

// getRealmsToMonitor returns the list of realms to monitor
func (m *Monitor) getRealmsToMonitor(ctx context.Context) []string {
	if len(m.instanceConfig.Realms) > 0 {
		return m.instanceConfig.Realms
	}

	// If no realms configured, try to fetch all realms from database
	realms, err := m.realmRepo.GetRealms(ctx, m.tenantID)
	if err != nil {
		m.logger.Warn("Failed to get realms from database", logger.Err(err))
		return []string{}
	}

	realmNames := make([]string, 0, len(realms))
	for _, realm := range realms {
		if realm.Enabled {
			realmNames = append(realmNames, realm.RealmName)
		}
	}

	return realmNames
}
