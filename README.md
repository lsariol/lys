# Lys

Lys is a chat bot for creators' communities, on Twitch today. It answers chat commands (fantasy football through Sleeper, Letterboxd, OneUp puzzle stats, reminders, autoposts, ChatGPT), and posts what its sources find: Sleeper trades, waivers and lineup reminders, new Letterboxd reviews, and messages other services send it.

It runs on the `spark` server, deployed by Lighthouse from this repository's version tags.

## How it fits together

```
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
| `internal/sources` | What posts without being asked: `letterboxd`, `sleeper` (moves, lineup reminders and weekly recaps, sent from one queue per league), `inlet` (`POST /notifications`) |
| `internal/features` | Code a command and a source share, such as the Sleeper client |
| `internal/scheduler` | Jobs that run at a time: reminders, autoposts |
| `internal/connections`, `identity`, `botaccounts` | Channels, communities, the people in chat, the bot account |
| `internal/database` | The connection, and the goose migrations Lys applies at startup |
| `internal/cli` | The `lys` command line and its shell |

## Adding a command

1. A package in `internal/commands/cmds/<name>` with a type that has `Spec()` and `Run(ctx, req)`.
2. `Spec` says everything the router needs: the name and aliases, a description and usage for `!help`, and when it differs from the defaults, who may use it (`Access`, or `Sub` per subcommand), a `Timeout`, `Cooldown`/`UserCooldown` and `Concurrency`.
3. Add it to the list in `cmd/lys/main.go`.

The router does the rest: the prefix, the permission check, cooldowns, the identity of the sender, the timeout, panics, and sending the reply.

## Adding a source

A type with `Name()` and `Run(ctx, pub)` that publishes `core.Outbound` messages, added to the sources list in `cmd/lys/main.go`. Set `Priority` on what it publishes if it shouldn't count as an announcement (autoposts are low, so replies and announcements go first).

## Running it locally

- **Secrets** come from your dev Cove: copy `.env.example` to `.env` and fill in its token. Nothing secret is kept in the repository.
- **Database:** a dev Postgres, with the URLs in your dev Cove; Lys applies its migrations when it starts.
- `go run ./cmd/lys` starts the bot; `go run ./cmd/lys help` lists the CLI's commands.
- **Tests:** `sh scripts/test-db.sh up`, then `eval "$(sh scripts/test-db.sh env)"` and `go test ./...` (the database tests skip without it). `sh scripts/test-db.sh down` removes it.

## On the server

- **The CLI:** `docker exec -it lys /lys shell` (`alias lys='docker exec -it lys /lys shell'`), or one command at a time: `docker exec lys /lys channel list`. `lys help` shows the commands and guides, such as `lys help new-channel`.
- **Logs:** `docker logs -f lys 2>&1` (Lys logs on stderr).
- **Releasing:** a `## vX.Y.Z (date)` section in `CHANGELOG.md`, `main` fast-forwarded and pushed, then the `vX.Y.Z` tag. Lighthouse deploys the tag and publishes its GitHub Release.
- **A release with a migration** is rehearsed first on a copy of prod: `sh scripts/rehearse/rehearse.sh up <lys_db dump>`.
