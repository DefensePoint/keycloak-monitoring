-- Read-only pre-flight for upgrading a database created by an older release,
-- before the unique indexes on users (idx_users_email, idx_users_subject,
-- idx_users_username) were scoped to live, non-empty rows. Run it on the
-- target database before the first start of this release:
--
--   psql -U "${POSTGRES_USER}" -d monitoring -f users-index-preflight.sql
--
-- On a Compose server, run it through the postgres container instead; see
-- "Running the scripts" in README.md.
--
-- It only reads, and prints counts and user ids, never the email addresses,
-- subjects or usernames themselves.
--
-- 1. Which of the three indexes exist today, and in which shape.
--    "live-only": the server leaves it alone. "plain": the server narrows it,
--    in one transaction, which cannot fail on the data. "missing": the server
--    creates it on start with its final predicate, which fails only if live
--    accounts share a value (query 2); the server then does not start until
--    they are resolved. Releases before this one built a missing index over
--    the whole column instead and could not start at all; on those, create it
--    yourself before starting, with the statement for that index below.
--
--      CREATE UNIQUE INDEX idx_users_email ON users(email)
--        WHERE deleted_at IS NULL AND email IS NOT NULL AND email <> '';
--      CREATE UNIQUE INDEX idx_users_subject ON users(subject)
--        WHERE deleted_at IS NULL AND subject IS NOT NULL AND subject <> '';
--      CREATE UNIQUE INDEX idx_users_username ON users(username)
--        WHERE deleted_at IS NULL AND username IS NOT NULL AND username <> '';
SELECT n.name AS index_name,
       CASE WHEN i.indexdef IS NULL THEN 'missing'
            WHEN i.indexdef LIKE '%deleted_at IS NULL%' THEN 'live-only'
            ELSE 'plain' END AS shape
FROM (VALUES ('idx_users_email'), ('idx_users_subject'), ('idx_users_username')) AS n(name)
LEFT JOIN pg_indexes i ON i.indexname = n.name AND i.tablename = 'users'
ORDER BY 1;

-- 2. How many values are duplicated among live, non-empty rows, which is
--    exactly what those statements forbid. A non-zero count on a column whose
--    index is "missing" means creating it fails until the duplicates are
--    resolved, by the server or by hand. For a "plain" or "live-only" index
--    the count is always 0.
SELECT 'email' AS column_name, count(*) AS duplicated_values FROM (
    SELECT email FROM users WHERE deleted_at IS NULL AND email IS NOT NULL AND email <> ''
    GROUP BY email HAVING count(*) > 1) d
UNION ALL
SELECT 'subject', count(*) FROM (
    SELECT subject FROM users WHERE deleted_at IS NULL AND subject IS NOT NULL AND subject <> ''
    GROUP BY subject HAVING count(*) > 1) d
UNION ALL
SELECT 'username', count(*) FROM (
    SELECT username FROM users WHERE deleted_at IS NULL AND username IS NOT NULL AND username <> ''
    GROUP BY username HAVING count(*) > 1) d;

-- 3. The accounts behind those counts: one row per shared value, with the ids
--    of the live users sharing it, oldest first. "Resolving duplicate
--    accounts" in README.md says which one to keep and how to retire or
--    rename the others.
SELECT 'email' AS column_name, string_agg(id::text, ',' ORDER BY id) AS user_ids
FROM users WHERE deleted_at IS NULL AND email IS NOT NULL AND email <> ''
GROUP BY email HAVING count(*) > 1
UNION ALL
SELECT 'subject', string_agg(id::text, ',' ORDER BY id)
FROM users WHERE deleted_at IS NULL AND subject IS NOT NULL AND subject <> ''
GROUP BY subject HAVING count(*) > 1
UNION ALL
SELECT 'username', string_agg(id::text, ',' ORDER BY id)
FROM users WHERE deleted_at IS NULL AND username IS NOT NULL AND username <> ''
GROUP BY username HAVING count(*) > 1
ORDER BY 1, 2;
