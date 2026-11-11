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
-- It only reads, and prints counts, never the email addresses, subjects or
-- usernames themselves.
--
-- 1. Which of the three indexes exist today, and in which shape.
--    "live-only": the server leaves it alone. "plain": the server narrows it,
--    which can never fail. "missing": create it yourself before starting, with
--    the statement for that index below. Left missing, the server's
--    AutoMigrate creates it over the whole column first, which fails on rows
--    the final index allows (several empty values, a soft-deleted account's
--    value reused), and the server does not start.
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
--    index is "missing" means its CREATE above fails until the duplicates are
--    resolved. For a "plain" or "live-only" index the count is always 0.
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
