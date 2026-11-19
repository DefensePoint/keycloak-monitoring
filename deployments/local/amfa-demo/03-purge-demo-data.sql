-- Removes every row 02-seed-demo-data.sql wrote, and nothing else.
--
-- Safe to run against an AMFA database that also holds real login history:
-- every demo row carries a marker in its primary key, and these deletes match
-- only those markers. Real engine rows use uuid_generate_v4() and will not
-- collide with the fixed 'de300001-de30-4de3-8de3-' style prefixes.
--
--   docker exec -i amfa-postgres-1 \
--     psql -U keycloak -d adaptive_mfa -f - < 03-purge-demo-data.sql
--
-- This does NOT remove the demo-* Keycloak users. Use teardown.sh for both.

\set ON_ERROR_STOP on

\set process_prefix 'de300001-de30-4de3-8de3-'
\set event_prefix   'de300002-de30-4de3-8de3-'
\set params_id      'de300003-de30-4de3-8de3-000000000001'
\set device_prefix  'de30de30dev-'
\set netloc_prefix  'de30de30net-'

BEGIN;

-- FK order: auth_event depends on auth_process, device and location_network;
-- auth_process depends on device, location_network and decision_params_config.
DELETE FROM auth_event   WHERE id::text LIKE :'event_prefix'   || '%';
DELETE FROM auth_process WHERE id::text LIKE :'process_prefix' || '%';
DELETE FROM device            WHERE hash LIKE :'device_prefix' || '%';
DELETE FROM location_network  WHERE hash LIKE :'netloc_prefix' || '%';
DELETE FROM decision_params_config WHERE id = :'params_id'::uuid;

COMMIT;

\echo ''
\echo '=== Remaining rows (demo markers should all be 0) ==='

SELECT
    (SELECT count(*) FROM auth_event   WHERE id::text LIKE :'event_prefix'   || '%') AS demo_events,
    (SELECT count(*) FROM auth_process WHERE id::text LIKE :'process_prefix' || '%') AS demo_processes,
    (SELECT count(*) FROM auth_event)   AS total_events,
    (SELECT count(*) FROM auth_process) AS total_processes;
