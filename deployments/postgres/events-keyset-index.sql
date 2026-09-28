-- Keyset-pagination index for the events table, built without locking writes.
--
-- The MCP list_events tool pages events in (tenant_id, timestamp DESC,
-- id DESC) order, and the Event model declares a composite index for it. On an
-- environment whose events table already holds rows, letting AutoMigrate build
-- that index takes an ACCESS EXCLUSIVE lock for the whole build and blocks
-- every write to the table until it finishes.
--
-- Creating the index here first makes AutoMigrate's own creation a no-op: it
-- looks the index up by name, finds it, and skips it. Run this once per
-- environment, before deploying the release that introduces the index:
--
--   psql -U "${POSTGRES_USER}" -d monitoring -f events-keyset-index.sql
--
-- CREATE INDEX CONCURRENTLY cannot run inside a transaction block, so this
-- script opens none. Do not add BEGIN/COMMIT and do not run it with psql
-- --single-transaction.
--
-- The events table is a plain Postgres table, not a TimescaleDB hypertable
-- (only keycloak_events, keycloak_metrics and keycloak_health are), so this is
-- an ordinary concurrent index build over one relation with no chunks
-- involved.
--
-- A concurrent build that fails or is interrupted leaves an invalid index
-- behind, which still carries the name AutoMigrate looks for and would
-- therefore never be repaired by a deploy. The verification query at the end
-- reports that state; when it does, drop the index with
-- DROP INDEX CONCURRENTLY idx_events_tenant_timestamp_id and run this script
-- again.

\set ON_ERROR_STOP on

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_events_tenant_timestamp_id
    ON events (tenant_id, timestamp DESC, id DESC);

SELECT c.relname AS index_name, i.indisvalid AS is_valid
FROM pg_class c
JOIN pg_index i ON i.indexrelid = c.oid
WHERE c.relname = 'idx_events_tenant_timestamp_id';
