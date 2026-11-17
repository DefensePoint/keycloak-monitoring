package postgres

import (
	"strings"
	"testing"
)

// The coordinate expressions for an AMFA whose location_network has lat/long,
// and for one without them, which must not name the columns at all: a query
// naming a missing column fails outright.
func TestCoordinateExpressions(t *testing.T) {
	with, without := coordinatesFor(true), coordinatesFor(false)

	for _, sql := range []string{without.lat, without.long, without.hasGeo(), eventRowSelect(without), geoQuery(without)} {
		if strings.Contains(sql, "nl.lat") || strings.Contains(sql, "nl.long") {
			t.Errorf("an AMFA without coordinates on location_network would be asked for them:\n%s", sql)
		}
	}
	for _, sql := range []string{with.hasGeo(), eventRowSelect(with), geoQuery(with)} {
		if !strings.Contains(sql, "nl.lat") || !strings.Contains(sql, "nl.long") {
			t.Errorf("coordinates are not read from location_network:\n%s", sql)
		}
	}
	// Read last: a scored login keeps its snapshot's coordinates.
	if !strings.HasSuffix(with.lat, "nl.lat)") || !strings.HasSuffix(with.long, "nl.long)") {
		t.Errorf("location_network must be the last source: %s / %s", with.lat, with.long)
	}
}
