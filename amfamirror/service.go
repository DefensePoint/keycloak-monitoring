package amfamirror

import (
	"context"
	"errors"
	"fmt"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
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
	// RunOnce executes a single mirror cycle synchronously.
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

	// watermarks holds the newest mirrored event_time per realm. Lost on
	// restart by design: it is re-derived from the events table.
	watermarks map[string]time.Time

	stopChan chan struct{}
	doneChan chan struct{}
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
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

func (s *service) Start(ctx context.Context) error {
	s.logger.Info("amfamirror service starting",
		"tenant_id", s.tenantID,
		"poll_interval", s.pollInterval.String(),
		"backfill", s.backfill.String())

	go func() {
		_ = s.runCycle(ctx) // best-effort: errors are logged; next tick retries
	}()
	go s.poll(ctx)
	return nil
}

func (s *service) Stop() error {
	s.logger.Info("Stopping amfamirror service", "tenant_id", s.tenantID)
	close(s.stopChan)
	select {
	case <-s.doneChan:
		s.logger.Info("amfamirror service stopped gracefully", "tenant_id", s.tenantID)
	case <-time.After(10 * time.Second):
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
	defer close(s.doneChan)

	for {
		select {
		case <-ticker.C:
			_ = s.runCycle(ctx)
		case <-s.stopChan:
			s.logger.Info("amfamirror polling stopped", "tenant_id", s.tenantID)
			return
		case <-ctx.Done():
			s.logger.Info("amfamirror polling cancelled", "tenant_id", s.tenantID)
			return
		}
	}
}

// runCycle mirrors new events for every realm. Per-realm failures are logged
// and do not block other realms; AMFA unavailability skips the cycle for that
// realm without advancing its watermark (same posture as amfacheck).
func (s *service) runCycle(ctx context.Context) error {
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
		s.logger.Info("amfamirror: merged amfa rows into keycloak twins",
			"tenant_id", s.tenantID, "count", absorbed)
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
			if err := s.store.MergeAmfaEvent(ctx, ToDomainEvent(row, realm, user)); err != nil {
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

// watermarkFor returns the in-memory watermark for the realm, deriving it from
// the events table (or the backfill window) on first use after startup.
func (s *service) watermarkFor(ctx context.Context, realm string) (time.Time, error) {
	if wm, ok := s.watermarks[realm]; ok {
		return wm, nil
	}
	latest, err := s.store.LatestTimestampBySource(ctx, SourceForRealm(realm))
	if err != nil {
		return time.Time{}, err
	}
	if latest.IsZero() {
		latest = time.Now().Add(-s.backfill)
		s.logger.Info("amfamirror: no mirrored events yet; backfilling",
			"tenant_id", s.tenantID, "realm", realm,
			"from", latest.Format(time.RFC3339))
	}
	s.watermarks[realm] = latest
	return latest, nil
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
