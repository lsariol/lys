-- Lys database: the one-time Admin setup on sparkdb.
--
-- Creates what's missing and fixes ownership and access:
--   roles     lys_owner (NOLOGIN), lys_migrator (member of the owner,
--             acting as it), lys_app, lys_reader
--   database  lys_db, owned by lys_owner; only the three login
--             roles may connect
--   schema    lys, if it exists already: owned by lys_owner
--   citext    the extension, in lys_db's public schema (only a superuser can
--             add it, and the baseline migration needs it)
-- Everything inside the schema (tables, grants) comes from Lys's goose
-- migrations, never from here.
--
-- Safe to run again. It never changes the password of a role that exists; a
-- password is only needed for a role that doesn't exist yet, and every missing
-- one is checked before anything changes.
--
-- Run as the Admin superuser on a new server (prod's v0.0.0 cutover does the
-- same by hand: notes/v0.0.0-cutover.md, window step 3):
--   docker exec -i sparkdb psql -U Admin -d postgres \
--     -v migrator_password="..." -v app_password="..." -v reader_password="..." \
--     -f - < scripts/db/setup.sql

\set ON_ERROR_STOP on
SET client_min_messages = warning;

SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lys_owner')    AS need_owner,
       NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lys_migrator') AS need_migrator,
       NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lys_app')      AS need_app,
       NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lys_reader')   AS need_reader,
       NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'lys_db')    AS need_db,
       current_setting('is_superuser') = 'on' AS is_superuser
\gset

-- 1. Preconditions, before anything changes.

\if :is_superuser
\else
DO $$ BEGIN RAISE EXCEPTION 'Run this as the Admin superuser (psql -U Admin).'; END $$;
\endif

\if :need_migrator
\if :{?migrator_password}
\else
DO $$ BEGIN RAISE EXCEPTION 'lys_migrator doesn''t exist: pass its password with -v migrator_password=... Nothing was changed.'; END $$;
\endif
\endif

\if :need_app
\if :{?app_password}
\else
DO $$ BEGIN RAISE EXCEPTION 'lys_app doesn''t exist: pass its password with -v app_password=... Nothing was changed.'; END $$;
\endif
\endif

\if :need_reader
\if :{?reader_password}
\else
DO $$ BEGIN RAISE EXCEPTION 'lys_reader doesn''t exist: pass its password with -v reader_password=... Nothing was changed.'; END $$;
\endif
\endif

-- 2. Roles.

\if :need_owner
\echo 'Creating lys_owner'
CREATE ROLE lys_owner NOLOGIN;
\else
-- Nothing logs in as the owner; its old password (if any) goes too.
ALTER ROLE lys_owner NOLOGIN PASSWORD NULL;
\endif

\if :need_migrator
\echo 'Creating lys_migrator'
CREATE ROLE lys_migrator LOGIN PASSWORD :'migrator_password';
\else
ALTER ROLE lys_migrator LOGIN;
\endif

\if :need_app
\echo 'Creating lys_app'
CREATE ROLE lys_app LOGIN PASSWORD :'app_password';
\else
ALTER ROLE lys_app LOGIN;
\endif

\if :need_reader
\echo 'Creating lys_reader'
CREATE ROLE lys_reader LOGIN PASSWORD :'reader_password';
\else
ALTER ROLE lys_reader LOGIN;
\endif

-- The migrator acts as the owner, so everything the migrations create is
-- owned by lys_owner, not by whoever ran them.
GRANT lys_owner TO lys_migrator;
ALTER ROLE lys_migrator SET role = 'lys_owner';

-- 3. The database.

\if :need_db
\echo 'Creating lys_db'
CREATE DATABASE lys_db OWNER lys_owner;
\else
ALTER DATABASE lys_db OWNER TO lys_owner;
\endif

REVOKE ALL ON DATABASE lys_db FROM PUBLIC;
GRANT CONNECT ON DATABASE lys_db TO lys_migrator, lys_app, lys_reader;

-- 4. Inside the database: the lys schema (if an earlier setup made
-- it) belongs to the owner, citext is installed, and nobody else may create
-- objects in public.

\connect lys_db
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'lys') THEN
    ALTER SCHEMA lys OWNER TO lys_owner;
  END IF;
END $$;
CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

\echo
\echo 'Done. Check: every row below matches its "want".'
\echo
\connect postgres
SELECT r.rolname AS role, r.rolcanlogin AS can_login,
       coalesce((SELECT string_agg(b.rolname, ', ') FROM pg_auth_members m JOIN pg_roles b ON b.oid = m.roleid
                  WHERE m.member = r.oid), '') AS member_of,
       coalesce(array_to_string(r.rolconfig, '; '), '') AS settings,
       CASE r.rolname
         WHEN 'lys_owner'    THEN 'NOLOGIN'
         WHEN 'lys_migrator' THEN 'LOGIN, member of lys_owner, role=lys_owner'
         ELSE 'LOGIN'
       END AS want
  FROM pg_roles r
 WHERE r.rolname IN ('lys_owner', 'lys_migrator', 'lys_app', 'lys_reader')
 ORDER BY r.rolname;
SELECT datname AS database, pg_get_userbyid(datdba) AS owner, array_to_string(datacl, ' ') AS access,
       'owner lys_owner; c (connect) for migrator, app, reader' AS want
  FROM pg_database WHERE datname = 'lys_db';
