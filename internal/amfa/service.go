package amfa

import (
	"context"
)

type service struct {
	registry   *Registry
	enrichment *Enrichment
}

// NewService wires registry + enrichment into a Service implementation.
func NewService(reg *Registry, enr *Enrichment) Service {
	return &service{registry: reg, enrichment: enr}
}

// Compile-time check
var _ Service = (*service)(nil)

func (s *service) ListEvents(ctx context.Context, tenantID string, opts ListEventsOptions) ([]Event, int64, error) {
	repo, err := s.registry.RepositoryFor(tenantID)
	if err != nil {
		return nil, 0, err
	}
	res, err := repo.ListEvents(ctx, opts)
	if err != nil {
		return nil, 0, err
	}

	// Collect unique non-nil user IDs
	uids := make([]string, 0, len(res.Items))
	seen := make(map[string]struct{}, len(res.Items))
	for _, row := range res.Items {
		if row.UserID == nil || *row.UserID == "" {
			continue
		}
		if _, dup := seen[*row.UserID]; dup {
			continue
		}
		seen[*row.UserID] = struct{}{}
		uids = append(uids, *row.UserID)
	}

	// Enrich (best-effort — failures just leave Username/Email nil)
	var enriched map[string]KeycloakUser
	if s.enrichment != nil && len(uids) > 0 {
		enriched, _ = s.enrichment.BatchEnrich(ctx, tenantID, opts.RealmID, uids)
	}

	events := make([]Event, 0, len(res.Items))
	for _, row := range res.Items {
		e := Event{
			EventID:     row.EventID,
			EventTime:   row.EventTime,
			EventType:   row.EventType,
			UserID:      row.UserID,
			Client:      row.Client,
			IPAddress:   row.IPAddress,
			Country:     row.Country,
			Lat:         row.Lat,
			Long:        row.Long,
			IsVPN:       row.IsVPN,
			RiskLevel:   row.RiskLevel,
			FinalStatus: row.FinalStatus,

			City:             row.City,
			OperatingSystem:  row.OperatingSystem,
			Browser:          row.Browser,
			Device:           row.Device,
			SystemLanguage:   row.SystemLanguage,
			ScreenResolution: row.ScreenResolution,
		}
		if row.UserID != nil {
			if u, ok := enriched[*row.UserID]; ok {
				e.Username = u.Username
				e.Email = u.Email
			}
		}
		events = append(events, e)
	}
	return events, res.Total, nil
}

func (s *service) GetStats(ctx context.Context, tenantID string, opts StatsOptions) (Stats, error) {
	repo, err := s.registry.RepositoryFor(tenantID)
	if err != nil {
		return Stats{}, err
	}
	return repo.GetStats(ctx, opts)
}

func (s *service) GetGeoBuckets(ctx context.Context, tenantID string, opts GeoOptions) ([]GeoBucket, error) {
	repo, err := s.registry.RepositoryFor(tenantID)
	if err != nil {
		return nil, err
	}
	return repo.GetGeoBuckets(ctx, opts)
}
