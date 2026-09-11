package amfamirror

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/lifecycle"
)

// watermarkOverlap is subtracted from the watermark on each query so events
// with identical/adjacent timestamps at the boundary are never skipped; the
// event_id upsert absorbs the resulting re-reads.
const watermarkOverlap = 2 * time.Second

// defaultCyclePageSize caps rows pulled from AMFA per query; the cycle loops
// until a short page, so a burst larger than this is still fully drained.
const defaultCyclePageSize = 500

// Service mirrors one tenant's AMFA events into the platform events table.
type Service interface {
	// Start begins polling: one immediate cycle, then on a ticker.
	Start(ctx context.Context) error
	// Stop gracefully stops polling (10s drain).
	Stop() error
	// RunOnce executes a single mirror cycle synchronously, unless a cycle is
	// already in flight, in which case it returns nil without doing anything:
	// cycles must not overlap. A caller wanting a guaranteed run has to retry.
	RunOnce(ctx context.Context) error
}

type service struct {
	tenantID     string
	realmsFn     RealmsFunc
	amfaRepo     amfa.Repository
	store        EventStore
	enricher     Enricher
	logger       Logger
	pollInterval time.Duration
	backfill     time.Duration

	// cycleMu serialises mirror cycles. It is held for the whole of a cycle,
	// which is also what makes the watermarks map below safe without a lock of
	// its own: every read and write of it happens inside runCycleLocked.
	cycleMu sync.Mutex

	// watermarks holds the newest mirrored event_time per realm. Lost on
	// restart by design: it is re-derived from the events table.
	watermarks map[string]time.Time

	// workers tracks every goroutine Start launches, including the immediate
	// first cycle, so Stop drains all of them.
	workers lifecycle.Workers
}

// NewService creates a per-tenant AMFA events mirror.
func NewService(
	tenantID string,
	realmsFn RealmsFunc,
	amfaRepo amfa.Repository,
	store EventStore,
	enricher Enricher,
	log Logger,
	pollInterval time.Duration,
	backfill time.Duration,
) Service {
	return &service{
		tenantID:     tenantID,
		realmsFn:     realmsFn,
		amfaRepo:     amfaRepo,
		store:        store,
		enricher:     enricher,
		logger:       log.WithComponent("amfa_mirror"),
		pollInterval: pollInterval,
		backfill:     backfill,
		watermarks:   make(map[string]time.Time),
	}
}

func (s *service) Start(ctx context.Context) error {
	s.logger.Info("amfamirror service starting",
		"tenant_id", s.tenantID,
		"poll_interval", s.pollInterval.String(),
		"backfill", s.backfill.String())

	s.workers.Go(func() {
		_ = s.runCycle(ctx) // best-effort: errors are logged; next tick retries
	})

	s.workers.Go(func() {
		s.poll(ctx)
	})
	return nil
}

func (s *service) Stop() error {
	s.logger.Info("Stopping amfamirror service", "tenant_id", s.tenantID)
	// Drain every worker, not just the poller. The immediate cycle Start
	// launches does not watch the stop signal, so this waits it out rather
	// than interrupting it; a cycle still going after the timeout is
	// abandoned, as the poller always was.
	if s.workers.Drain(10 * time.Second) {
		s.logger.Info("amfamirror service stopped gracefully", "tenant_id", s.tenantID)
	} else {
		s.logger.Warn("amfamirror service stop timeout", "tenant_id", s.tenantID)
	}
	return nil
}

func (s *service) RunOnce(ctx context.Context) error {
	return s.runCycle(ctx)
}

func (s *service) poll(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = s.runCycle(ctx)
		case <-s.workers.Stopping():
			s.logger.Info("amfamirror polling stopped", "tenant_id", s.tenantID)
			return
		case <-ctx.Done():
			s.logger.Info("amfamirror polling cancelled", "tenant_id", s.tenantID)
			return
		}
	}
}

// runCycle mirrors new events for every realm, one cycle at a time.
//
// Cycles must not overlap. Start launches an immediate cycle in its own
// goroutine while poll tickers further ones, so a first backfill that outlives
// one poll interval used to put two cycles in flight together. That raced on
// the watermarks map, and a concurrent map write in Go is a fatal panic that
// takes the process down, not a recoverable error. Overlapping cycles also
// duplicated every AMFA read and let the slower goroutine write back a staler
// position.
//
// TryLock rather than Lock so a tick that arrives mid-cycle is skipped instead
// of queued: the poll goroutine returns straight to its select and stays
// responsive to the stop signal, which a blocking wait would delay past the
// drain timeout in Stop.
func (s *service) runCycle(ctx context.Context) error {
	if !s.cycleMu.TryLock() {
		// Warn, not Debug: a cycle that regularly outlives its poll interval
		// means this tenant is mirroring less often than configured, and a
		// silent skip leaves an operator no way to see that.
		s.logger.Warn("amfamirror: previous cycle still running; skipped this poll",
			"tenant_id", s.tenantID, "poll_interval", s.pollInterval.String())
		return nil
	}
	defer s.cycleMu.Unlock()
	return s.runCycleLocked(ctx)
}

// runCycleLocked is runCycle's body. Callers must hold cycleMu, which is what
// makes the unsynchronised watermarks map safe: every access to it happens
// inside this call tree.
func (s *service) runCycleLocked(ctx context.Context) error {
	realms, err := s.realmsFn(ctx)
	if err != nil {
		s.logger.Warn("amfamirror: failed to list realms; skipping cycle",
			"tenant_id", s.tenantID, "error", err.Error())
		return err
	}

	var errs []error
	for _, realm := range realms {
		if err := s.mirrorRealm(ctx, realm); err != nil {
			s.logger.Warn("amfamirror: realm cycle failed",
				"tenant_id", s.tenantID, "realm", realm, "error", err.Error())
			errs = append(errs, fmt.Errorf("%s: %w", realm, err))
		}
	}

	// Converge any AMFA rows onto their Keycloak twin. Runs every cycle so a
	// login whose halves arrived out of order (or raced) still collapses to one
	// row within a poll interval.
	if absorbed, err := s.store.ReconcileAMFAMerges(ctx); err != nil {
		s.logger.Warn("amfamirror: reconcile failed",
			"tenant_id", s.tenantID, "error", err.Error())
		errs = append(errs, fmt.Errorf("reconcile: %w", err))
	} else if absorbed > 0 {
		// The sweep is table-wide, not tenant-scoped: the events table has no
		// tenant column to scope by, so each tenant's mirror reconciles every
		// tenant's rows. The count therefore spans all tenants, and tenant_id
		// names the mirror that ran the sweep rather than the owner of the rows
		// it absorbed. Logging it as a bare "count" alongside tenant_id read as
		// this tenant's merge total, which over-reports each tenant and lets N
		// mirrors each claim the same merges.
		s.logger.Info("amfamirror: reconcile sweep absorbed amfa rows into keycloak twins (table-wide, spans all tenants)",
			"tenant_id", s.tenantID, "rows_absorbed_all_tenants", absorbed)
	}
	return errors.Join(errs...)
}

func (s *service) mirrorRealm(ctx context.Context, realm string) error {
	watermark, err := s.watermarkFor(ctx, realm)
	if err != nil {
		return fmt.Errorf("derive watermark: %w", err)
	}
	// startWatermark is the boundary before this cycle. Rows at or before it are
	// re-reads from the overlap window; only rows strictly after it are new.
	startWatermark := watermark

	since := watermark.Add(-watermarkOverlap)
	newlySaved := 0
	for {
		rows, err := s.amfaRepo.ListEventsSince(ctx, realm, since, defaultCyclePageSize)
		if err != nil {
			return fmt.Errorf("list amfa events: %w", err)
		}
		if len(rows) == 0 {
			break
		}

		users := s.enrichUsers(ctx, realm, rows)
		for _, row := range rows {
			var user *amfa.KeycloakUser
			if row.UserID != nil {
				if u, ok := users[*row.UserID]; ok {
					user = &u
				}
			}
			if err := s.store.MergeAmfaEvent(ctx, ToDomainEvent(row, s.tenantID, realm, user)); err != nil {
				// Stop the realm cycle at the first failed save so the
				// watermark never advances past an unsaved event.
				return fmt.Errorf("save event %s: %w", row.EventID, err)
			}
			if row.EventTime.After(startWatermark) {
				newlySaved++
			}
			if row.EventTime.After(watermark) {
				watermark = row.EventTime
			}
		}
		s.watermarks[realm] = watermark

		if len(rows) < defaultCyclePageSize {
			break
		}
		since = watermark
	}

	// Persist only a genuine advance. Every cycle re-reads the overlap window,
	// so writing unconditionally would mean a pointless write per realm per
	// poll on an idle deployment.
	if watermark.After(startWatermark) {
		s.recordWatermark(ctx, realm, watermark)
	}

	if newlySaved > 0 {
		s.logger.Info("amfamirror: mirrored events",
			"tenant_id", s.tenantID, "realm", realm, "count", newlySaved,
			"watermark", watermark.Format(time.RFC3339))
	} else {
		s.logger.Debug("amfamirror: no new events",
			"tenant_id", s.tenantID, "realm", realm)
	}
	return nil
}

// watermarkFor returns the in-memory watermark for the realm, loading this
// tenant's persisted position (or the backfill window) on first use after
// startup.
func (s *service) watermarkFor(ctx context.Context, realm string) (time.Time, error) {
	if wm, ok := s.watermarks[realm]; ok {
		return wm, nil
	}
	latest, err := s.store.AmfaMirrorWatermark(ctx, s.tenantID, realm)
	if err != nil {
		return time.Time{}, err
	}
	if latest.IsZero() {
		latest = time.Now().Add(-s.backfill)
		s.logger.Info("amfamirror: no recorded position for this tenant and realm; backfilling",
			"tenant_id", s.tenantID, "realm", realm,
			"from", latest.Format(time.RFC3339))
	}
	s.watermarks[realm] = latest
	return latest, nil
}

// recordWatermark persists how far this tenant has read for the realm.
//
// Best-effort on purpose: the in-memory watermark already governs this
// process, so a failed write costs at most a bounded re-read after the next
// restart, and every save upserts on event_id. Failing the cycle here would
// turn a cosmetic problem into a stalled mirror.
func (s *service) recordWatermark(ctx context.Context, realm string, watermark time.Time) {
	if err := s.store.SaveAmfaMirrorWatermark(ctx, s.tenantID, realm, watermark); err != nil {
		s.logger.Warn("amfamirror: failed to persist read position; a restart may re-read this realm",
			"tenant_id", s.tenantID, "realm", realm, "error", err.Error())
	}
}

// enrichUsers resolves usernames/emails for the batch, best-effort.
func (s *service) enrichUsers(ctx context.Context, realm string, rows []amfa.EventRow) map[string]amfa.KeycloakUser {
	if s.enricher == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.UserID == nil || *row.UserID == "" {
			continue
		}
		if _, dup := seen[*row.UserID]; dup {
			continue
		}
		seen[*row.UserID] = struct{}{}
		ids = append(ids, *row.UserID)
	}
	if len(ids) == 0 {
		return nil
	}
	users, err := s.enricher.BatchEnrich(ctx, s.tenantID, realm, ids)
	if err != nil {
		s.logger.Debug("amfamirror: user enrichment failed; continuing without identities",
			"tenant_id", s.tenantID, "realm", realm, "error", err.Error())
		return nil
	}
	return users
}
