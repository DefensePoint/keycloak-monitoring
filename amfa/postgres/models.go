// Package postgres provides read-only GORM access to AMFA's PostgreSQL
// database. All models use the gorm:"->" read-only tag on every field so that
// GORM will never attempt INSERT/UPDATE/DELETE through these structs; this is
// defense in depth on top of DB-role-level least-privilege enforcement, and
// AutoMigrate is never invoked on this connection because the AMFA database
// schema is owned by AMFA, not KMT.
package postgres

import (
	"time"
)

// AuthEvent mirrors AMFA's auth_event table.
type AuthEvent struct {
	ID                  string    `gorm:"->;column:id;primaryKey"`
	AuthProcess         *string   `gorm:"->;column:auth_process"`
	UserID              *string   `gorm:"->;column:user_id"`
	EventType           string    `gorm:"->;column:event_type"`
	Details             *string   `gorm:"->;column:details"`
	AuthContextHash     *string   `gorm:"->;column:auth_context_hash"`
	DeviceInfoHash      *string   `gorm:"->;column:device_info_hash"`
	NetworkLocationHash *string   `gorm:"->;column:network_location_hash"`
	EventTime           time.Time `gorm:"->;column:event_time"`
	// Stamped directly by AMFA's login-event webhook at write time, so it is
	// present even for events whose auth_process never linked (see
	// realm_filter in repository.go).
	RealmID *string `gorm:"->;column:realm_id"`
}

// TableName returns AMFA's auth_event table name.
func (AuthEvent) TableName() string { return "auth_event" }

// AuthProcess mirrors AMFA's auth_process table.
type AuthProcess struct {
	ID                  string     `gorm:"->;column:id;primaryKey"`
	UserID              string     `gorm:"->;column:user_id"`
	AuthContextHash     string     `gorm:"->;column:auth_context_hash"`
	DeviceCredibility   *float64   `gorm:"->;column:device_credibility"`
	NetLocCredibility   *float64   `gorm:"->;column:net_loc_credibility"`
	AuthContextJSON     []byte     `gorm:"->;column:auth_context_json;type:jsonb"`
	PreAuthRiskDecision int        `gorm:"->;column:pre_auth_risk_decision"`
	FinalStatus         string     `gorm:"->;column:final_status"`
	StartedAt           time.Time  `gorm:"->;column:started_at"`
	FinishedAt          *time.Time `gorm:"->;column:finished_at"`
	// Backfilled from auth_context_json where recoverable; still NULL for
	// rows predating that backfill (see realm_filter in repository.go).
	RealmID *string `gorm:"->;column:realm_id"`
}

// TableName returns AMFA's auth_process table name.
func (AuthProcess) TableName() string { return "auth_process" }

// AuthContext mirrors AMFA's auth_context table.
type AuthContext struct {
	Hash             string   `gorm:"->;column:hash;primaryKey"`
	Client           string   `gorm:"->;column:client"`
	IPAddress        string   `gorm:"->;column:ip_address"`
	Lat              *float64 `gorm:"->;column:lat"`
	Long             *float64 `gorm:"->;column:long"`
	IsVPN            *bool    `gorm:"->;column:is_vpn"`
	CountryName      *string  `gorm:"->;column:country_name"`
	OperatingSystem  *string  `gorm:"->;column:operating_system"`
	Browser          *string  `gorm:"->;column:browser"`
	Device           *string  `gorm:"->;column:device"`
	SystemLanguage   *string  `gorm:"->;column:system_language"`
	ScreenResolution *string  `gorm:"->;column:screen_resolution"`
}

// TableName returns AMFA's auth_context table name.
func (AuthContext) TableName() string { return "auth_context" }
