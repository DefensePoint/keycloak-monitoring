package postgres

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
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

// A query must not wait for a probe another query is running: it goes on with
// the last answer. The repository has no database, so reaching it would panic.
func TestNetworkCoordinates_DoesNotWaitForARunningProbe(t *testing.T) {
	repo := &Repository{}
	repo.schema.probing.Lock()
	defer repo.schema.probing.Unlock()

	done := make(chan bool, 1)
	go func() { done <- repo.networkCoordinates(context.Background()) }()
	select {
	case has := <-done:
		if has {
			t.Fatal("answered yes with no check done")
		}
	case <-time.After(time.Second):
		t.Fatal("a query waited for the running probe")
	}
}

// The probe gives up on its own, whatever the caller's context: the events
// mirror's has no deadline. The server accepts the connection and never
// answers, as a hung AMFA database would.
func TestNetworkCoordinates_ProbeTimesOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	db, err := gorm.Open(gormpg.Open(fmt.Sprintf("host=127.0.0.1 port=%d user=kmt dbname=amfa sslmode=disable", port)),
		&gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo := NewRepository(db)

	done := make(chan bool, 1)
	go func() { done <- repo.networkCoordinates(context.Background()) }()
	select {
	case has := <-done:
		if has {
			t.Fatal("a probe that timed out answered yes")
		}
	case <-time.After(networkCoordinatesProbeTimeout + 5*time.Second):
		t.Fatal("the probe kept waiting for a database that does not answer")
	}
	if !repo.schema.checkedAt.IsZero() {
		t.Fatal("a probe that timed out was remembered")
	}
}
