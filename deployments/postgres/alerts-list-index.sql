-- List index for configuration_alerts, built without locking writes.
--
-- Alert listing filters by tenant_id and orders by first_detected DESC, id
-- DESC (alerts/postgres/repository.go). The Alert model declares a composite
-- index for that. On an environment whose configuration_alerts table already
-- holds rows, letting AutoMigrate build that index takes a SHARE lock for
-- the whole build. SHARE blocks writes and still allows reads.
--
-- Creating the index here first makes AutoMigrate's own creation a no-op: it
-- looks the index up by name, finds it, and skips it. Run this once per
-- environment, before deploying the release that introduces the index:
--
--   psql -U "${POSTGRES_USER}" -d monitoring -f alerts-list-index.sql
--
-- On a Compose server, run it through the postgres container instead; see
-- "Running the scripts" in README.md.
--
-- CREATE INDEX CONCURRENTLY cannot run inside a transaction block, so this
-- script opens none. Do not add BEGIN/COMMIT and do not run it with psql
-- --single-transaction.
--
-- configuration_alerts is a plain Postgres table, not a TimescaleDB
-- hypertable. This is an ordinary concurrent index build over one relation.
-- Listing stays LIMIT/OFFSET. The index lets Postgres walk one tenant's
-- alerts in order instead of sorting them. It does not turn the API into
-- keyset pagination.
--
-- A concurrent build that fails or is interrupted leaves an invalid index
-- behind, which still carries the name AutoMigrate looks for and would
-- therefore never be repaired by a deploy. The verification query at the end
-- reports that state; when it does, drop the index with
-- DROP INDEX CONCURRENTLY idx_alerts_tenant_first_detected_id and run this
-- script again.
--
\set ON_ERROR_STOP on

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_alerts_tenant_first_detected_id
    ON configuration_alerts (tenant_id, first_detected DESC, id DESC);

SELECT c.relname AS index_name, i.indisvalid AS is_valid
FROM pg_class c
JOIN pg_index i ON i.indexrelid = c.oid
WHERE c.relname = 'idx_alerts_tenant_first_detected_id';
