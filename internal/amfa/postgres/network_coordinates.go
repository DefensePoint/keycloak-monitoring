package postgres

import (
	"context"
	"sync"
	"time"
)

// networkCoordinatesRecheck is how long a "no" stays trusted. An AMFA upgraded
// while KMT runs gets the columns; a "yes" is never re-checked, because the
// engine never drops them.
const networkCoordinatesRecheck = 10 * time.Minute

// networkCoordinatesCheck remembers whether this AMFA database's
// location_network has lat/long.
type networkCoordinatesCheck struct {
	mu        sync.Mutex
	has       bool
	checkedAt time.Time
}

// networkCoordinates reports whether location_network has lat and long. A
// failed check answers no and is not remembered, so the next query asks again.
func (r *Repository) networkCoordinates(ctx context.Context) bool {
	r.schema.mu.Lock()
	defer r.schema.mu.Unlock()
	if r.schema.has || (!r.schema.checkedAt.IsZero() && time.Since(r.schema.checkedAt) < networkCoordinatesRecheck) {
		return r.schema.has
	}
	var n int64
	err := r.db.WithContext(ctx).Raw(`
        SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'location_network'
          AND column_name IN ('lat', 'long')`).Scan(&n).Error
	if err != nil {
		return false
	}
	r.schema.has, r.schema.checkedAt = n == 2, time.Now()
	return r.schema.has
}

func (r *Repository) coords(ctx context.Context) coordinates {
	return coordinatesFor(r.networkCoordinates(ctx))
}
