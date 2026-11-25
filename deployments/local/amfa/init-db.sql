-- First start of the demo's AMFA PostgreSQL: the engine's database next to
-- Keycloak's. The engine creates its tables in it on startup.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE DATABASE adaptive_mfa;
\c adaptive_mfa
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
