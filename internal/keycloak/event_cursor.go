package keycloak

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

const (
	// eventPollOverlap is how far behind the newest event a realm's cursor
	// sits after a complete fetch, so an event committed just after a poll is
	// still picked up. Events already seen are skipped, not processed again.
	eventPollOverlap = 30 * time.Second

	// Keycloak stamps a successful event when the flow succeeds but stores it
	// when the request's transaction commits, up to its transaction timeout
	// (transaction-default-timeout, 5m by default) later; failures are stored
	// at once. So a realm is swept every eventSweepInterval: one fetch reads
	// again from the previous sweep's start minus that timeout and
	// eventClockMargin. Whatever an event's commit delay within the timeout,
	// the first sweep starting after it became visible reaches back to it,
	// however long the sweeps take.
	eventSweepInterval = 5 * time.Minute
	// eventClockMargin absorbs clocks that differ between Keycloak nodes and
	// the monitor.
	eventClockMargin = time.Minute
	// defaultLateCommitWindow is Keycloak's default transaction timeout.
	defaultLateCommitWindow = 5 * time.Minute

	// eventTimingWarnEvery limits the late and future event warnings to one
	// per realm per this period, carrying the worst seen since the last one.
	eventTimingWarnEvery = 10 * time.Minute

	// eventFutureTolerance is how far ahead of the monitor's clock an event
	// may be stamped before it is reported as clock skew.
	eventFutureTolerance = 30 * time.Second

	// maxEventPagesPerCycle bounds one realm's fetch of new events per cycle,
	// and maxEventPagesPerSweepCycle its share of a sweep. Both continue from
	// where they stopped on the next cycle.
	maxEventPagesPerCycle      = 10
	maxEventPagesPerSweepCycle = 20

	// maxEventCoverage bounds the fetch history kept for the visibility delay.
	// prune keeps everything from a running sweep's position, so a sweep that
	// kept failing while the polls succeeded would grow it without end.
	maxEventCoverage = 1000

	// maxEventRetries bounds the events kept in memory for a retry after a
	// failed write. The oldest are dropped first.
	maxEventRetries = 10000
)

// openEnded is the upper bound of a fetch that read to the newest event.
const openEnded = math.MaxInt64

// realmEventCursor is one realm's position in its Keycloak event stream. Only
// the events worker touches it, so it is not locked.
type realmEventCursor struct {
	// since is the inclusive lower bound of the next fetch of new events.
	since time.Time
	// seen holds the time (ms) of every event fetched back to where the next
	// sweep starts, so the overlap, sweeps and page boundaries never process
	// an event twice.
	seen map[string]int64
	// retry holds events whose save, alert conversion or merge failed. Before
	// the cursor these were retried because every poll fetched the whole day.
	retry []*keycloakadmin.EventRepresentation

	// lastSweepStart is when the current or last sweep started (monitor
	// clock); the next sweep reads from it minus the late commit window.
	lastSweepStart time.Time
	// While sweeping, the sweep resumes at sweepFrom, offset sweepFirst.
	sweeping     bool
	sweepFrom    time.Time
	sweepFirst   int
	warnedBehind bool
	// backfilling marks the first sweep of a new cursor, which reads the
	// day so far while new events are fetched from now.
	backfilling bool

	// covered records which event times each fetch read, to tell how late an
	// event first seen now became visible.
	covered []coverage

	// timing gathers late and future events until the next warning.
	timing         eventFetchReport
	lastTimingWarn time.Time
}

// coverage is one fetch at at (monitor clock) that returned every event then
// visible with a time from from to upto (ms, inclusive).
type coverage struct {
	at         time.Time
	from, upto int64
}

// newRealmEventCursor fetches new events from now minus the overlap, and
// backfills the events from floor up to there as a sweep, a few pages per
// cycle, so a restart does not hold new events back behind a busy day.
func newRealmEventCursor(floor, now time.Time) *realmEventCursor {
	since := now.Add(-eventPollOverlap)
	if since.Before(floor) {
		since = floor
	}
	return &realmEventCursor{
		since:          since,
		seen:           make(map[string]int64),
		lastSweepStart: now,
		sweeping:       true,
		sweepFrom:      floor,
		backfilling:    true,
	}
}

// eventKey is the id an event is stored under: Keycloak's own id, or, for
// Keycloak before 26.2 which returns none, a hash of everything the event
// holds. The time and type alone (legacyEventKey) gave two failed logins in
// one millisecond, from different users or addresses, the same id, so the
// second was dropped as a duplicate. Events identical in every field still
// share an id; nothing tells them apart.
func eventKey(e *keycloakadmin.EventRepresentation) string {
	if e.ID != "" {
		return e.ID
	}
	h := sha256.New()
	for _, field := range []string{e.RealmID, strconv.FormatInt(e.Time, 10), e.Type, e.ClientID, e.UserID, e.SessionID, e.IPAddress, e.Error} {
		h.Write([]byte(field))
		h.Write([]byte{0})
	}
	keys := make([]string, 0, len(e.Details))
	for k := range e.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(e.Details[k]))
		h.Write([]byte{0})
	}
	return syntheticEventIDPrefix + hex.EncodeToString(h.Sum(nil))[:40]
}

// syntheticEventIDPrefix marks the ids eventKey makes for events Keycloak
// gave none.
const syntheticEventIDPrefix = "kcev-"

// legacyEventKey is the id events without a Keycloak id were stored under
// before eventKey hashed them. Rows stored so are matched by it (see
// Monitor.eventIDs), so re-reading them does not store them twice.
func legacyEventKey(e *keycloakadmin.EventRepresentation) string {
	return fmt.Sprintf("%s-%d-%s", e.RealmID, e.Time, e.Type)
}

// unseen returns the events of page not fetched before and marks them seen.
func (c *realmEventCursor) unseen(page []*keycloakadmin.EventRepresentation) []*keycloakadmin.EventRepresentation {
	fresh := make([]*keycloakadmin.EventRepresentation, 0, len(page))
	for _, e := range page {
		key := eventKey(e)
		if _, ok := c.seen[key]; ok {
			continue
		}
		c.seen[key] = e.Time
		fresh = append(fresh, e)
	}
	return fresh
}

// cover records that a fetch at at returned every event then visible from
// from to upto.
func (c *realmEventCursor) cover(at time.Time, from, upto int64) {
	c.covered = append(c.covered, coverage{at: at, from: from, upto: upto})
	if over := len(c.covered) - maxEventCoverage; over > 0 {
		c.covered = append([]coverage(nil), c.covered[over:]...)
	}
}

// visibilityDelay is the least time the event stamped t (ms) can have taken to
// become visible: the latest earlier fetch whose range held t did not return
// it. known is false when no fetch read t before, so nothing can be said. It
// includes any difference between the stamping node's clock and the monitor's.
func (c *realmEventCursor) visibilityDelay(t int64) (delay time.Duration, known bool) {
	stamped := time.UnixMilli(t)
	for _, cv := range c.covered {
		if t < cv.from || t > cv.upto {
			continue
		}
		if d := cv.at.Sub(stamped); !known || d > delay {
			delay, known = d, true
		}
	}
	return delay, known
}

// advance moves the cursor after a fetch whose newest event is at newest (ms).
// It never passes now: a Keycloak node whose clock runs ahead must not carry
// the cursor past events the other nodes are still writing. After a complete
// fetch it trails that by eventPollOverlap; after a partial one (page cap or
// error) it stops there, since later events are not fetched yet. It never
// moves back.
func (c *realmEventCursor) advance(newest int64, complete bool, now time.Time) {
	if newest == 0 {
		return
	}
	next := time.UnixMilli(newest)
	if now.Before(next) {
		next = now
	}
	if complete {
		next = next.Add(-eventPollOverlap)
	}
	if next.After(c.since) {
		c.since = next
	}
}

// sweepAnchor is the time the next sweep reaches back from: the last sweep's
// start, or now if the monitor's clock has since stepped back past it.
func (c *realmEventCursor) sweepAnchor(now time.Time) time.Time {
	if now.Before(c.lastSweepStart) {
		return now
	}
	return c.lastSweepStart
}

// startSweepIfDue starts a sweep when eventSweepInterval has passed since the
// last one started (or the clock stepped back past it) and none is running. It
// reads from that start minus the late commit window and eventClockMargin; it
// is not held to the lookback floor, which would cut off events committed
// late across midnight. behind reports, once per sweep, that a sweep other
// than the first backfill is still running when the next is due.
func (c *realmEventCursor) startSweepIfDue(now time.Time, lateCommitWindow time.Duration) (started, behind bool) {
	due := now.Sub(c.lastSweepStart) >= eventSweepInterval || now.Before(c.lastSweepStart)
	if c.sweeping {
		if due && !c.warnedBehind && !c.backfilling {
			c.warnedBehind = true
			return false, true
		}
		return false, false
	}
	if !due {
		return false, false
	}
	from := c.sweepAnchor(now).Add(-lateCommitWindow - eventClockMargin)
	c.sweeping, c.sweepFrom, c.sweepFirst, c.warnedBehind = true, from, 0, false
	c.lastSweepStart = now
	return true, false
}

// finishSweep records that the running sweep reached the cursor.
func (c *realmEventCursor) finishSweep() {
	c.sweeping, c.backfilling = false, false
}

// prune forgets seen events older than the next sweep's start, the running
// sweep's position and the cursor, whichever is earliest. A fetch made before
// that point cannot tell anything about the events still to come, which are
// all stamped after it, so its coverage goes too.
func (c *realmEventCursor) prune(lateCommitWindow time.Duration, now time.Time) {
	cutoff := c.sweepAnchor(now).Add(-lateCommitWindow - eventClockMargin)
	if c.sweeping && c.sweepFrom.Before(cutoff) {
		cutoff = c.sweepFrom
	}
	if c.since.Before(cutoff) {
		cutoff = c.since
	}
	ms := cutoff.UnixMilli()
	for key, t := range c.seen {
		if t < ms {
			delete(c.seen, key)
		}
	}
	kept := c.covered[:0]
	for _, cv := range c.covered {
		if cv.upto >= ms && !cv.at.Before(cutoff) {
			kept = append(kept, cv)
		}
	}
	c.covered = kept
}

// addRetries queues failed events for the next cycle and reports how many of
// the oldest were dropped to stay within maxEventRetries.
func (c *realmEventCursor) addRetries(failed []*keycloakadmin.EventRepresentation) (dropped int) {
	c.retry = append(c.retry, failed...)
	if over := len(c.retry) - maxEventRetries; over > 0 {
		c.retry = append([]*keycloakadmin.EventRepresentation(nil), c.retry[over:]...)
		return over
	}
	return 0
}

// takeRetries returns the queued events still at or after floor and empties
// the queue. expired counts the ones dropped for being older.
func (c *realmEventCursor) takeRetries(floor time.Time) (due []*keycloakadmin.EventRepresentation, expired int) {
	cutoff := floor.UnixMilli()
	for _, e := range c.retry {
		if e.Time < cutoff {
			expired++
			continue
		}
		due = append(due, e)
	}
	c.retry = nil
	return due, expired
}

// eventLookbackFloor is the oldest point events are fetched from after a start
// and retried until: the start of the day containing now minus the lookback.
// That is how far back the date-only query this replaced reached.
func eventLookbackFloor(now time.Time, lookback time.Duration) time.Time {
	t := now.Add(-lookback)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
