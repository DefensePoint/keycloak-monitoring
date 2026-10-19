package database

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestAlertListIndexes(t *testing.T) {
	parsed, err := schema.Parse(&Alert{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse Alert: %v", err)
	}

	assertIndex(t, parsed, "idx_alerts_tenant_first_detected_id", []indexCol{
		{name: "tenant_id"},
		{name: "first_detected", sort: "desc"},
		{name: "id", sort: "desc"},
	})
	assertIndex(t, parsed, "idx_alerts_tenant_status_first_detected_id", []indexCol{
		{name: "tenant_id"},
		{name: "status"},
		{name: "first_detected", sort: "desc"},
		{name: "id", sort: "desc"},
	})
}

type indexCol struct {
	name string
	sort string
}

func assertIndex(t *testing.T, parsed *schema.Schema, name string, want []indexCol) {
	t.Helper()
	idx := parsed.LookIndex(name)
	if idx == nil {
		t.Fatalf("missing index %s", name)
	}
	if len(idx.Fields) != len(want) {
		t.Fatalf("index %s has %d columns, want %d", name, len(idx.Fields), len(want))
	}
	for i, col := range want {
		got := idx.Fields[i]
		if got.DBName != col.name || got.Sort != col.sort {
			t.Errorf("index %s column %d = %s %s, want %s %s", name, i, got.DBName, got.Sort, col.name, col.sort)
		}
	}
}
