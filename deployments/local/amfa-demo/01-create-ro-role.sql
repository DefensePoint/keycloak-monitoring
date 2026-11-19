-- Creates the read-only role KMT uses to read a local AMFA database.
--
-- Neither the AMFA engine stack nor its Alembic migrations create this role,
-- so a freshly started local AMFA Postgres always rejects KMT's connection
-- with "role kmt_ro does not exist". The failure is quiet: the registry logs
-- a warning and comes up with zero AMFA tenants, and the AMFA page then
-- renders as "not configured" rather than as an error.
--
-- seed.sh runs this as a superuser on the AMFA database. The password must
-- match the tenant's amfa.database.password in config.yaml.

-- :ro_password and :db are supplied by seed.sh. Do not \set them here; a
-- literal assignment would override whatever the driver passes.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'kmt_ro') THEN
        CREATE ROLE kmt_ro LOGIN;
        RAISE NOTICE 'created role kmt_ro';
    ELSE
        RAISE NOTICE 'role kmt_ro already exists; refreshing grants';
    END IF;
END
$$;

ALTER ROLE kmt_ro WITH LOGIN PASSWORD :'ro_password';

-- Least privilege: connect, look, and nothing else. KMT's GORM models all
-- carry the `gorm:"->"` read-only tag, but that is application-level defence;
-- this is the part the database actually enforces. These are the grants
-- docs/amfa-integration.md asks for on a real AMFA database.
-- :"db" rather than a literal, so an AMFA_DB override grants on the database
-- actually being seeded instead of silently granting on the default one.
GRANT CONNECT ON DATABASE :"db" TO kmt_ro;
GRANT USAGE ON SCHEMA public TO kmt_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO kmt_ro;

-- Alembic adds tables on upgrade; without this the role silently loses access
-- to anything created after today.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO kmt_ro;

-- KMT's schema check reads alembic_version to compare against the tenant's
-- configured expected_schema_version, so it must be readable too. Covered by
-- the blanket grant above; asserted here so a future narrowing does not
-- silently break the check.
-- Tolerate the table being absent: has_table_privilege() raises on a missing
-- relation rather than returning false, and some AMFA builds create their
-- schema without Alembic. KMT's CheckAmfaSchema already treats a missing
-- alembic_version as a warning, so this must not be fatal either.
DO $$
BEGIN
    IF to_regclass('public.alembic_version') IS NULL THEN
        RAISE NOTICE 'alembic_version not present; KMT will log a schema-check warning (non-fatal)';
    ELSIF NOT has_table_privilege('kmt_ro', 'public.alembic_version', 'SELECT') THEN
        RAISE EXCEPTION 'kmt_ro cannot read alembic_version; the KMT schema check will fail';
    END IF;
END
$$;
