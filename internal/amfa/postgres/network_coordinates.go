package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// networkCoordinatesRecheck is how long a "no" stays trusted, so an AMFA
// upgraded while KMT runs is picked up. A "yes" is kept until a query finds
// the columns gone: the engine's downgrade drops them (withCoordinates).
const networkCoordinatesRecheck = 10 * time.Minute

// networkCoordinatesProbeTimeout bounds the probe, whatever the caller's
// context: the events mirror's has no deadline.
const networkCoordinatesProbeTimeout = 5 * time.Second

// networkCoordinatesCheck remembers whether this AMFA database's
// location_network has lat/long.
type networkCoordinatesCheck struct {
	mu        sync.Mutex // guards has and checkedAt; never held across a query
	probing   sync.Mutex // held by the one probe running
	has       bool
	checkedAt time.Time
}

func (c *networkCoordinatesCheck) answer() (has, fresh bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.has, c.has || (!c.checkedAt.IsZero() && time.Since(c.checkedAt) < networkCoordinatesRecheck)
}

func (c *networkCoordinatesCheck) remember(has bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.has, c.checkedAt = has, time.Now()
}

// networkCoordinates reports whether location_network has lat and long. A
// query never waits for the probe: while one runs, the others go on with the
// last answer. A failed probe keeps the last answer and is not remembered, so
// the next query asks again.
func (r *Repository) networkCoordinates(ctx context.Context) bool {
	has, fresh := r.schema.answer()
	if fresh || !r.schema.probing.TryLock() {
		return has
	}
	defer r.schema.probing.Unlock()

	ctx, cancel := context.WithTimeout(ctx, networkCoordinatesProbeTimeout)
	defer cancel()
	var n int64
	err := r.db.WithContext(ctx).Raw(`
        SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'location_network'
          AND column_name IN ('lat', 'long')`).Scan(&n).Error
	if err != nil {
		return has
	}
	r.schema.remember(n == 2)
	return n == 2
}

// withCoordinates runs a query that reads coordinates with the expressions this
// AMFA's schema allows. When the columns were dropped since the last check, the
// query fails with undefined_column: the "yes" is then forgotten and the query
// runs once more without them, so a downgrade costs neither a failed query nor
// a restart.
func (r *Repository) withCoordinates(ctx context.Context, query func(coordinates) error) error {
	has := r.networkCoordinates(ctx)
	err := query(coordinatesFor(has))
	var pgErr *pgconn.PgError
	if has && errors.As(err, &pgErr) && pgErr.Code == "42703" {
		r.schema.remember(false)
		err = query(coordinatesFor(false))
	}
	return err
}
