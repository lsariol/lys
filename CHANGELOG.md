# Changelog

## v0.0.0 (unreleased)

The first release under the Lys name: a rework of BotSuite.

### Added
- Every deploy first runs gofmt, go vet and the tests with the race detector; a failure stops the deploy.
- Database migrations: Lys applies them on startup with the migrator role (`LYS_DATABASE_MIGRATOR_URL` in Cove), and refuses to start if its database is older than it needs. The first migration records the existing schema and changes nothing in it.
- `lys migrate status` lists the migrations and when each was applied; `lys migrate up` applies any that are pending.
- `!help` lists the commands, and `!help <command>` describes one.
- `/health` reports whether Twitch is connected, and answers 503 while it isn't, so Docker shows Lys as unhealthy.
- Channels, the people who use commands, and the bot account are stored platform-neutrally (communities, connections, platform users, bot accounts), ready for more platforms and for linking accounts on the website. Existing channels, prefixes and users are copied in; the old tables are kept and still written.
- `lys channel list`, `lys channel add`, `lys channel block` and `lys channel unblock` manage channels from the command line.
- `lys bot authorize` signs Lys in as its Twitch bot account and stores the tokens in Cove, replacing the manual token exchange.
- `lys help` lists every command and guide; `lys help <command>` shows its usage, flags and examples.
- `!remind <10m|1h30m|2d> <message>` posts a reminder in the same channel later (1 minute to 30 days, up to 10 waiting per person), with `!remind list` and `!remind cancel <id>`.
- `!timer add <every> <message>` posts a message on repeat (every 5 minutes to 24 hours, up to 10 per channel), for the channel's owner and moderators, with `!timer list` and `!timer remove <id>`.
- Reminders and timers survive restarts; a failed one is retried with backoff, and two Lys instances never run the same one.
- Lys announces new Letterboxd diary entries itself, replacing LetterboxdWatcher, with the same wording; the "see other entries" link now opens the member's activity page.
- Sleeper trades and waiver claims are announced in the league's channel, once each; a league's first check only records what already happened. `!ff alerts` shows which are announced, and `!ff alerts <trades|waivers> <on|off>` changes it.
- Adding, blocking or unblocking a channel (with `lys channel …` too) takes effect within seconds, without a restart.
- When Twitch says the bot was banned in a channel, or the channel's account is gone, Lys leaves it and `lys channel list` shows it as revoked; `lys channel add` brings it back.
- When Twitch revokes the bot account's own authorization, `/health` turns unhealthy and the log says to run `lys bot authorize`.
- An enrollment API for the website: `POST /channels` adds a channel (the bot joins within seconds) and `DELETE /channels/{platform}/{id}` turns Lys off in it, keeping its settings. It needs the shared token `SHARED_LYS_ENROLLMENT_TOKEN` and is only reachable on the server's internal network.

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
- Owner-only commands also accept the channel's broadcaster role, not only a matching Twitch ID.
- `LYS_TWITCH_BOT_ID` is only needed until Lys has recorded its bot account.
- Games, feature requests, Sleeper leagues and Letterboxd subscriptions are linked to channels and people by Lys's own IDs instead of Twitch IDs and names; existing rows are linked automatically at startup.
- `!oneup stats <name>` finds people by their current name, and their stats follow them through a rename; fastest and slowest show current names.
- `!request get` and `!request set` are limited to the bot's owner by Twitch ID instead of by display name.
- A Sleeper member who changes their Twitch name keeps their link, and is found by their new name.

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
- Two Lys instances starting at once no longer fail while preparing migrations.
- A Twitch connection that goes silent is noticed within its keepalive timeout and reopened, instead of leaving the bot deaf.
- A channel that refuses the bot's messages (banned, timed out, or missing its permission) is paused for 10 minutes instead of failing on every reply; a rate-limited message waits for Twitch's reset and is sent once more.
