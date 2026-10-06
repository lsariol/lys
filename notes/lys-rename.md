# Renaming botsuite to Lys

The runbook for renaming everything called "botsuite" to "Lys": the database, its roles and schema, the Cove secrets and token, the code, and the container.

**Status:** done on dev. Prod still to do.

This is a one-off manual change because Lys has no database migrations yet. When Lys gets goose migrations, the baseline should be taken **after** this rename, so it starts from the `lys` names. After that, schema changes like this belong in migrations.

---

## What changes

| Thing | Before | After |
|---|---|---|
| Database | `botsuite_db` | `lys_db` |
| Roles | `botsuite_owner`, `botsuite_migrator`, `botsuite_app`, `botsuite_reader` | `lys_owner`, `lys_migrator`, `lys_app`, `lys_reader` |
| Schema | `botsuite` | `lys` |
| Trigger functions | `create_twitch_settings_entry`, `log_twitch_channel_changes` (their code says `botsuite.`) | Same names, code says `lys.` |
| SQL in Lys's code | `botsuite.<table>` | `lys.<table>` |
| SQL in LetterboxdWatcher | `botsuite.letterboxd_feed_subscriptions` | `lys.letterboxd_feed_subscriptions` |
| Cove keys | `BOTSUITE_…` | `LYS_…` |
| Cove token | `botsuite` | `lys` |
| Go module, entry point | `github.com/lsariol/botsuite`, `cmd/botsuite` | `github.com/lsariol/lys`, `cmd/lys` |
| Compose project and container | `botsuite` | `lys` |

**LetterboxdWatcher changes too.** It connects to this database with Lys's URL and writes to `letterboxd_feed_subscriptions` in this schema. It must be deployed in the same window.

**What Postgres handles by itself.** Renaming is instant and copies no data. Tables, grants, default privileges, sequences and triggers refer to the schema and roles by internal ID, so they follow the rename automatically. Only two things are stored as plain text and need fixing by hand:

- the migrator's `role=botsuite_owner` setting, and
- the code inside the two trigger functions.

**Passwords survive.** The login roles use SCRAM passwords, which are unaffected by a rename. Postgres only clears a password on rename if it's an older `md5` one; the check in step 2 confirms this.

---

## 1. Code (PC, before the window)

Already done on branch `feature/LysRename` in both repos. Check before merging:

- **Lys:** no `botsuite` left in the Go code, the compose file or the Dockerfile (`git grep -il botsuite -- '*.go' '*.yaml' Dockerfile go.mod` finds nothing). The README and `.env.exmaple` are worth updating too. The folder is `cmd/lys`, matching the Dockerfile's `go build ./cmd/lys`. The compose file has `name: lys`, `container_name: lys` and `COVE_TOKEN: ${LYS_COVE_TOKEN}`.
- **LetterboxdWatcher:** its queries use `lys.letterboxd_feed_subscriptions`, and its compose file uses `${LYS_DATABASE_URL}` and `${LYS_NOTIFICATION_URL}`.
- **Both build** with `go build ./...`.

Do **not** push `main` yet. Pushing `main` deploys, and that has to happen after the database rename (step 7).

## 2. Read-only checks (pgAdmin, connected to `botsuite_db`)

```sql
-- 1. Password types: expect SCRAM for every login role (the owner has none).
SELECT rolname, left(rolpassword, 5) FROM pg_authid WHERE rolname LIKE 'botsuite%';

-- 2. Role settings: expect botsuite_migrator = {role=botsuite_owner}, others empty.
SELECT rolname, rolconfig FROM pg_roles WHERE rolname LIKE 'botsuite%';

-- 3. Functions whose code names the schema: expect the two trigger functions.
SELECT p.proname, pg_get_functiondef(p.oid) LIKE '%botsuite.%' AS names_schema
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'botsuite' AND p.prokind = 'f';
```

If query 1 shows `md5` for a role, add `ALTER ROLE <new name> PASSWORD '<same password>';` for it after step 5. If query 3 lists more functions, the step 6 fix handles them too.

## 3. Back up (server)

```bash
sudo mkdir -p /srv/backups
docker exec sparkdb pg_dump -U <sparkdb admin user> -d botsuite_db -Fc | sudo tee /srv/backups/botsuite_db-before-lys.dump > /dev/null
docker exec sparkdb pg_dumpall -U <sparkdb admin user> --globals-only | sudo tee /srv/backups/roles-before-lys.sql > /dev/null
sudo chmod 600 /srv/backups/*-before-lys*
ls -l /srv/backups
```

Both files should have a non-zero size. The admin user is the sparkdb superuser (`SPARKDB_DATABASE_USER` in Cove).

## 4. Stop the apps (server)

```bash
docker stop botsuite letterboxdwatcher
```

Then in pgAdmin, right-click `botsuite_db` → **Disconnect Database**, and don't click on it again until step 6. A database can't be renamed while anyone is connected, pgAdmin included.

From here until step 7 is done, Lys and LetterboxdWatcher are down.

## 5. Rename the database and roles (pgAdmin)

Open the Query Tool on **`AdminDB`** (not `botsuite_db`): Postgres can't rename the database you're connected to. The tab title shows which database the tool is using. Run the whole block at once:

```sql
SELECT pg_terminate_backend(pid) FROM pg_stat_activity
WHERE datname = 'botsuite_db' AND pid <> pg_backend_pid();

ALTER DATABASE botsuite_db RENAME TO lys_db;

ALTER ROLE botsuite_owner    RENAME TO lys_owner;
ALTER ROLE botsuite_migrator RENAME TO lys_migrator;
ALTER ROLE botsuite_app      RENAME TO lys_app;
ALTER ROLE botsuite_reader   RENAME TO lys_reader;

-- Stored as text, so it still names the old role. Point it at the new one:
ALTER ROLE lys_migrator SET role = 'lys_owner';
```

**If it says** `current database cannot be renamed`: the Query Tool is connected to `botsuite_db`; open a new one on `AdminDB`. **If it says** `is being accessed by other users`: an app or another pgAdmin tab is still connected; check step 4 and run the block again.

Refresh the server in pgAdmin's tree: it now shows `lys_db`.

## 6. Rename the schema and fix the functions (pgAdmin, Query Tool on `lys_db`)

```sql
ALTER SCHEMA botsuite RENAME TO lys;
```

**Preview** the fixed functions. This changes nothing:

```sql
WITH f AS MATERIALIZED (
  SELECT p.oid, p.proname
  FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
  WHERE n.nspname = 'lys' AND p.prokind = 'f'
)
SELECT proname, replace(pg_get_functiondef(oid), 'botsuite.', 'lys.') AS fixed
FROM f
WHERE pg_get_functiondef(oid) LIKE '%botsuite.%';
```

Expect two rows. Read each `fixed` value: only `botsuite.` references should have become `lys.`. (`MATERIALIZED` and `prokind = 'f'` matter: without them Postgres may run `pg_get_functiondef` on built-in aggregates such as `array_agg` and fail with `'array_agg' is an aggregate function`.)

**Apply:**

```sql
DO $$
DECLARE
  f   oid;
  def text;
BEGIN
  FOR f IN
    SELECT p.oid
    FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'lys' AND p.prokind = 'f'
  LOOP
    def := pg_get_functiondef(f);
    IF def LIKE '%botsuite.%' THEN
      EXECUTE replace(def, 'botsuite.', 'lys.');
    END IF;
  END LOOP;
END $$;
```

**Check:**

```sql
-- The preview query again: expect no rows.

SELECT rolname, rolconfig FROM pg_roles WHERE rolname LIKE 'lys%';      -- lys_migrator: {role=lys_owner}
SELECT nspname FROM pg_namespace WHERE nspname IN ('botsuite', 'lys');  -- only lys
```

Then connect once as `lys_migrator` (a new pgAdmin connection, or `psql`) and run `SELECT current_user;`. Expect `lys_owner`.

## 7. Cove, Lighthouse and the deploy

**Cove** (server: `docker exec -it cove /cove shell`). First list what exists:

```
list BOTSUITE_
```

Create Lys's token while the old one still works. Cove has no "rename token", so Lys gets a new one:

```
token create lys --allow 'LYS_*' --write LYS_TWITCH_ACCESS_TOKEN
create LYS_COVE_TOKEN <the token it just printed>
```

Rename every key `list` showed:

```
rename BOTSUITE_OPENAI_API_KEY LYS_OPENAI_API_KEY
rename BOTSUITE_TWITCH_CLIENT_ID LYS_TWITCH_CLIENT_ID
rename BOTSUITE_TWITCH_CLIENT_SECRET LYS_TWITCH_CLIENT_SECRET
rename BOTSUITE_TWITCH_BOT_ID LYS_TWITCH_BOT_ID
rename BOTSUITE_TWITCH_ACCESS_TOKEN LYS_TWITCH_ACCESS_TOKEN --yes
rename BOTSUITE_DATABASE_URL LYS_DATABASE_URL
rename BOTSUITE_NOTIFICATION_URL LYS_NOTIFICATION_URL
```

- The access-token rename needs `--yes`. The old token names that key directly; without `--yes` the CLI asks about updating it, and answering no cancels the rename.
- Rename the database password keys to the standard names too: `LYS_DATABASE_APP_PASSWORD`, `LYS_DATABASE_MIGRATOR_PASSWORD`, `LYS_DATABASE_READER_PASSWORD`, `LYS_DATABASE_OWNER_PASSWORD` (if present), whatever their current `BOTSUITE_…` names are.
- Leave `BOTSUITE_COVE_TOKEN` alone until step 9.

Point the database URL at the new names, with the same password as before:

```
update LYS_DATABASE_URL postgres://lys_app:<password>@sparkdb:5432/lys_db?sslmode=disable
```

Do the same for any migrator URL key (`lys_migrator`, `lys_db`).

**Lighthouse.** In its project list on the server (`/srv/server/storage/lighthouse/repos.json`), set the display name and container name to `lys`. If the GitHub repo was renamed, also set the three URLs to `lsariol/lys`.

**Deploy.**

1. On the server, remove the old container: `docker rm -f botsuite`. Otherwise the new `lys` container can't start, because port 2400 is still taken (or both run and the bot answers twice).
2. On the PC, merge `feature/LysRename` into `main` and push, in Lys and in LetterboxdWatcher. Lighthouse deploys both.

## 8. Check

- `docker ps` shows `lys` and `letterboxdwatcher` running, and no `botsuite`.
- The bot answers in Twitch, and a command that uses the database works (for example a feature request or a game).
- LetterboxdWatcher posts its next update.
- In Cove: `token list` shows `lys` with a recent "last used". After Lys's next Twitch token refresh, `history LYS_TWITCH_ACCESS_TOKEN` shows `lys` as the writer.
- `docker logs lys` shows no `not found` or `forbidden_key` errors from Cove, and no database errors.

## 9. Clean up

In Cove:

```
token revoke botsuite
delete BOTSUITE_COVE_TOKEN
```

- Log the change in `C:\Users\Lu\Documents\prod-rollout-plan.md`.
- Delete the step 3 backups after a few days: `sudo rm /srv/backups/*-before-lys*`.

---

## Way back

Every step is a rename, so going back is the same SQL with the names swapped.

1. Stop `lys` and `letterboxdwatcher`.
2. On `AdminDB`: rename `lys_db` to `botsuite_db`, the four roles back, and `ALTER ROLE botsuite_migrator SET role = 'botsuite_owner';`.
3. On `botsuite_db`: `ALTER SCHEMA lys RENAME TO botsuite;`, then the function fix with `'lys.'` and `'botsuite.'` swapped.
4. In Cove, rename the keys back.
5. Redeploy the previous commits of both projects.

If anything is worse than a rename can fix, the step 3 dump and roles file are the full way back.

---

## Lessons from the dev run

- Run step 5 from a Query Tool on `AdminDB`, not on the database being renamed.
- The function preview needs `MATERIALIZED` and `prokind = 'f'`, or it fails on built-in aggregates.
- `go build ./...` passes even when the Dockerfile's `./cmd/lys` path doesn't exist. Check the folder name, or build the image once locally (`docker build .`), before pushing.
