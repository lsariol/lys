# Lys

Lys is a chat bot for streamers' communities, on Twitch today: it answers chat commands and posts what its sources find.

## Features

- **Fantasy football (`!ff`)**: links a channel to its Sleeper league, and shows teams, matchups, scores, standings, rosters, trades, the draft and the playoffs. It announces trades, waiver claims and free-agent moves, reminds the league to set lineups, and posts a weekly recap.
- **7TV (`!7tv`)**: the channel's 7TV editors add, remove and rename emotes, undo a change and switch emote sets from chat. Searching, listing sets and editors, and showing an emote are open to everyone.
- **Letterboxd (`!letterboxd`, `!lb`)**: announces new diary entries from the feeds viewers follow.
- **OneUp (`!oneup`)**: records the OneUp puzzle results posted in chat and shows stats.
- **Reminders and autoposts (`!remind`, `!autopost`)**: a reminder in the channel later, and messages posted on a schedule while chat is active.
- **ChatGPT (`!gpt`)**: answers a question in chat.
- **Fun and utilities**: `!catfact`, `!randomcat`, `!randomdog`, `!ping`, `!uptime`, `!request` for feature requests, `!settings` for the channel's prefix, and `!help`.
- **Permissions and limits**: each command says who may use it (everyone, VIPs, moderators or the channel's owner), with cooldowns and caps on the slow ones.
- **Inbound API**: other services can post messages to channels (`POST /notifications`) and enroll or remove channels (`POST /channels`, `DELETE /channels/{platform}/{id}`), each with its own bearer token.
- **Command line**: `lys` manages channels, the bot account, 7TV and migrations, one command at a time or in a shell with history and Tab completion.

## Requirements

- Go 1.27.3
- PostgreSQL 16 or newer, with the `citext` extension
- A [Cove](https://github.com/lsariol/cove) vault for the secrets
- A Twitch application, and a Twitch account for the bot
- Docker, for the test database and the test stage

## Getting started

1. Create the database and its roles: `scripts/db/setup.sql`, run as a superuser, sets up `lys_db` with an owner, a migrator, an app and a reader role.
2. Store the secrets in Cove (see [Configuration](#configuration)).
3. Copy `.env.example` to `.env` and set `COVE_URL` and `COVE_TOKEN`.
4. Sign the bot in and store its tokens:

   ```bash
   go run ./cmd/lys bot authorize
   ```

5. Add a channel, then start the bot:

   ```bash
   go run ./cmd/lys channel add <login>
   go run ./cmd/lys
   ```

Lys applies its migrations when it starts. `go run ./cmd/lys help` lists the other commands, and `go run ./cmd/lys help new-channel` walks through bringing Lys into a channel.

## Configuration

Settings are read from the environment:

| Variable | Default | Meaning |
|---|---|---|
| `APP_ENV` | | `prod`, or `dev` for readable logs |
| `COVE_URL` | | Address of the Cove vault |
| `COVE_TOKEN` | | Lys's Cove token, which may also write (the bot's refreshed tokens) |
| `NOTIFICATION_PORT` | | Port the HTTP server listens on (`2400` in the compose file) |
| `LYS_VERSION` | `dev` | Version it reports on `/health` |
| `SHARED_LYS_NOTIFICATION_TOKEN` | | Bearer token for `POST /notifications`; empty refuses every request |
| `SHARED_LYS_ENROLLMENT_TOKEN` | | Bearer token for the enrollment API; empty refuses every request |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `json` | `json` or `text` (`text` when `APP_ENV=dev`) |
| `LOG_CHAT` | `false` | `true` logs every chat message, not only commands |

Secrets are read from Cove:

| Key | Meaning |
|---|---|
| `LYS_DATABASE_URL` | Postgres connection string for the app role |
| `LYS_DATABASE_MIGRATOR_URL` | Connection string for the migrator role; without it, migrations are skipped |
| `LYS_TWITCH_CLIENT_ID`, `LYS_TWITCH_CLIENT_SECRET` | The Twitch application |
| `LYS_TWITCH_ACCESS_TOKEN`, `LYS_TWITCH_REFRESH_TOKEN` | The bot account's tokens, stored by `lys bot authorize` |
| `LYS_OPENAI_API_KEY` | For `!gpt` |
| `LYS_SEVENTV_TOKEN` | The bot's 7TV session, for `!7tv`; optional |

Logs are one JSON record per line on stderr. Each command use is one `command` record with the command, channel, outcome, `duration_ms` and `reply_ms`; its arguments are never logged.

## Development

```bash
sh scripts/test-db.sh up                 # a throwaway Postgres on 127.0.0.1:55433
eval "$(sh scripts/test-db.sh env)"
go test -race ./...                      # the database tests skip without it
sh scripts/test-db.sh down
docker build --target test .             # gofmt, vet and the tests, as before a deploy
```

A release that adds a migration can be rehearsed on a copy of a database first: `sh scripts/rehearse/rehearse.sh up <dump>`.

### How it fits together

```text
adapters ──messages──▶ router ──▶ commands
   ▲                     │
   └──────replies────────┘◀── sources (pollers, reminders, the inlet)
```

| Folder | What's there |
|---|---|
| `cmd/lys` | Startup: wires everything below, and runs the CLI when given arguments |
| `internal/adapters/twitch` | Twitch: EventSub for chat in, Helix for chat out (paced per channel and for the whole account), the bot's tokens |
| `internal/router` | Takes every message, checks who may use a command, its cooldowns and caps, and runs it on a worker |
| `internal/commands` | The command contract (`Spec`, `Request`, `Reply`, access tiers); the commands themselves in `cmds/` |
| `internal/sources` | What posts without being asked: Letterboxd, Sleeper, 7TV invites, and the inlet |
| `internal/features` | Clients a command and a source share: Sleeper and 7TV |
| `internal/scheduler` | Jobs that run at a time: reminders, autoposts |
| `internal/connections`, `identity`, `botaccounts` | Channels, communities, the people in chat, the bot account |
| `internal/database` | The connection, and the goose migrations Lys applies at startup |
| `internal/cli` | The `lys` command line and its shell |

### Adding a command

1. A package in `internal/commands/cmds/<name>` with a type that has `Spec()` and `Run(ctx, req)`.
2. `Spec` says everything the router needs: the name and aliases, a description and usage for `!help`, and when it differs from the defaults, who may use it (`Access`, or `Sub` per subcommand), a `Timeout`, `Cooldown`/`UserCooldown` and `Concurrency`.
3. Add it to the list in `cmd/lys/main.go`.

The router does the rest: the prefix, the permission check, cooldowns, the identity of the sender, the timeout, panics, sending the reply and logging the use.

### Adding a source

A type with `Name()` and `Run(ctx, pub)` that publishes `core.Outbound` messages, added to the sources list in `cmd/lys/main.go`. Set `Priority` on what it publishes if it shouldn't count as an announcement (autoposts are low, so replies and announcements go first).

## Deployment

`docker-compose.yaml` is written for [Lighthouse](https://github.com/lsariol/lighthouse), which deploys each version tag. It expects an external Docker network named `spark`, with Cove and Postgres on it, and reads its secrets from the environment. In the container, `docker exec -it lys /lys shell` opens the command line, and `/lys health` is the healthcheck. Changes are listed in [CHANGELOG.md](CHANGELOG.md).

## License

See [LICENSE](LICENSE).
