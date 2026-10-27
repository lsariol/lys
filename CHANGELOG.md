# Changelog

## v0.0.0 (unreleased)

The first release under the Lys name: a rework of BotSuite.

### Added
- Every deploy first runs gofmt, go vet and the tests with the race detector; a failure stops the deploy.
- Database migrations: Lys applies them on startup with the migrator role (`LYS_DATABASE_MIGRATOR_URL` in Cove), and refuses to start if its database is older than it needs. The first migration records the existing schema and changes nothing in it.
- `lys migrate status` lists the migrations and when each was applied; `lys migrate up` applies any that are pending.
- `!help` lists the commands, and `!help <command>` describes one.
- `/health` reports whether Twitch is connected, and answers 503 while it isn't, so Docker shows Lys as unhealthy.

### Changed
- Renamed from BotSuite to Lys: the database, schema, roles, Cove keys and container are all `lys` now.
- The bot account's Twitch tokens are kept in Cove (`LYS_TWITCH_ACCESS_TOKEN`, `LYS_TWITCH_REFRESH_TOKEN`) instead of the database, refreshed at startup and written back after every refresh.
- The Twitch app token is fetched at startup and kept in memory only.
- Lys refuses to start if its Twitch tokens belong to an account other than the bot.
- Built with Go 1.27.3 on Alpine 3.24.
- Commands run side by side (up to 8 at once), each with its own time limit, so a slow command no longer holds up the others.
- Replies are paced per channel, one every 1.5 seconds, instead of one per second across every channel.
- The notification inlet only accepts requests with the shared token (`SHARED_LYS_NOTIFICATION_TOKEN`), and its port is no longer published on the server: only services on the `spark` network reach it.
- The bot ignores its own chat messages.

### Fixed
- A command that crashes now replies with an error instead of taking the bot down.
- A chat message rejected with a 401 is retried once with a new app token instead of forever, which froze the bot for every channel.
- `!ff trades` without a name no longer crashes the bot.
- `!gpt` no longer crashes the bot when OpenAI is slow, unreachable or returns an error.
- `!sleeper link user` no longer crashes when the Sleeper API fails, and says when no one by that name has a team.
- `!sleeper link league` works again; it always failed before.
- `!oneup stats <puzzle>` for a puzzle with no recorded games says so instead of replying with an error, and `!oneup stats <x> <puzzle>` shows the game count instead of a memory address.
- `!oneup leaders` and unknown `!oneup stats` forms reply instead of saying the command doesn't exist.
- Shutting down no longer panics.
- When Twitch asks the bot to move to a new connection and that fails, the bot opens a fresh connection instead of crashing.
- A failed connection to Twitch at startup reports the error instead of crashing.
- A channel without a settings row no longer stops the bot from starting; it uses the `!` prefix.
