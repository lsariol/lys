# Changelog

## v0.1.0 (unreleased)

### Added
- Lineup reminders for linked Sleeper leagues, in the league's channels during the NFL regular season: 1 day and 1 hour before the week's first game, and 1 hour before Sunday's first game. Kickoff times come from ESPN's schedule, shown in Eastern time. A reminder that's more than 30 minutes late (after a restart, say) isn't sent. `!ff alerts lineups off` turns them off.
- Each command says who may use it (everyone, VIPs, moderators or the channel's owner), and Lys checks it before running the command; `!help <command>` shows it. A refused command answers "Only moderators can use !autopost." and the like.
- Cooldowns and caps on slow commands: `!gpt` runs at most 16 at a time across all channels, with 4 seconds between one person's uses; `!catfact` and `!randomdog` at most 12, 3 seconds apart; Letterboxd and Sleeper at most 16. Up to 32 commands run at once in all (8 before), so a rush of slow commands never holds up the quick ones. Past a cap, Lys answers "Busy, try again in a moment."; within a cooldown it stays quiet. The channel's owner and moderators skip cooldowns.
- Messages too long for one Twitch message (a long `!gpt` answer, say) go out in parts instead of being refused: split at a space, in order, up to 4 parts; anything beyond ends with "…".
- `lys channel prefix <channel> <prefix>` changes a channel's prefix from the CLI.
- `lys shell`: the CLI's commands one after another, with line editing, history and Tab completion, and the environment in the prompt (`lys (prod)>`, prod in red). On the server: `docker exec -it lys /lys shell`, or `alias lys='docker exec -it lys /lys shell'`.
- Faster chat where the bot is a moderator, VIP or broadcaster: messages there go out 0.3 seconds apart instead of 1.5. Lys learns the bot's role from the badges on its own messages and remembers it across restarts.
- One limit for all of the bot's messages across channels, a little under Twitch's (20 per 30 seconds, 100 where the bot is privileged), so busy channels together never get the bot ignored for an hour. When messages have to wait, replies go first, then announcements and reminders, then autoposts. Helix's own rate limit is logged once a minute while Lys sends.

### Changed
- `!oneup stats <puzzle number>` and stats by name use their own indexes instead of reading every game.
- Built on pgx 5.11 and goose 3.29.
- In channels where the bot isn't privileged, messages go out 1.1 seconds apart instead of 1.5, as Twitch allows 1 a second there.
- Channels are joined 8 at a time instead of one by one (9 channels: 0.3 s instead of 1.4 s; 300: 6 s instead of 45 s), and adding or removing a channel no longer holds up the others.
- Letterboxd: a feed several channels follow is fetched once per round for all of them, an unchanged feed comes back as a short "not modified", and up to 4 feeds are fetched at once, so a round still fits its 25 minutes with many more feeds.
- The database pool holds up to 12 connections (unless the URL sets `pool_max_conns`), instead of a number that depended on the server's CPUs.
- Each command logs how long its reply took, from the chat message to the reply.
- The bot's Twitch token is refreshed only when it's about to run out (or Twitch refuses it), not on every join and twice at startup; Cove gets the refresh token only when it changed.
- Slow commands no longer hold up chat: Lys looks at every message as it arrives and only runs the command itself in the background, so a slow `!gpt` can't delay other commands, puzzle results or the activity autoposts count. When Lys can't keep up, it drops chat messages (and logs how many) rather than falling behind Twitch, which would disconnect it.
- The bot's owner no longer counts as the owner of other people's channels: `!settings prefix set`, `!ff link league`, `!ff unlink league` and changing `!ff alerts` are for each channel's owner (autoposts already were). `lys channel prefix` covers fixes.
- The `!sleeper` command and the Sleeper announcements share one Sleeper client, so the 15 MB player catalog is downloaded once a day instead of once each.

### Removed
- Lys no longer copies channel, prefix, game, request and Sleeper changes into the tables the old bot read; it reads and writes only its own. Letterboxd subscriptions are still copied until v0.1.1 drops those tables, so going back to v0.0.0 keeps them.
- The startup step that linked rows the old bot wrote.
- `LYS_TWITCH_BOT_ID` is no longer read: the bot account comes from `lys bot authorize`.

## v0.0.0 (2026-10-28)

The first release under the Lys name: a rework of BotSuite.

### Added
- Every deploy first runs gofmt, go vet and the tests with the race detector; a failure stops the deploy.
- Database migrations: Lys applies them on startup with the migrator role (`LYS_DATABASE_MIGRATOR_URL` in Cove), and refuses to start if its database is older than it needs. The first migration records the existing schema and changes nothing in it.
- `lys migrate status` lists the migrations and when each was applied; `lys migrate up` applies any that are pending.
- `!help` lists the commands, and `!help <command>` describes one.
- `/health` reports whether Twitch is connected, and answers 503 while it isn't, so Docker shows Lys as unhealthy. It also reports the version running (the release or commit Lighthouse deployed), which Lys logs at startup too.
- Channels, the people who use commands, and the bot account are stored platform-neutrally (communities, connections, platform users, bot accounts), ready for more platforms and for linking accounts on the website. Existing channels, prefixes and users are copied in; the old tables are kept and still written.
- `lys channel list`, `lys channel add`, `lys channel block` and `lys channel unblock` manage channels from the command line. A name that several channels share is refused (use the ID), and `channel list` tells a revoked channel from one that was never authorized.
- `lys bot authorize` signs Lys in as its Twitch bot account and stores the tokens in Cove, replacing the manual token exchange.
- `lys help` lists every command and guide; `lys help <command>` shows its usage, flags and examples.
- `!remind <10m|1h30m|2d> <message>` posts a reminder in the same channel later (1 minute to 30 days, up to 10 waiting per person), with `!remind list` and `!remind cancel <id>`.
- `!autopost add <how often> <message>` (e.g. `!autopost add 30m Follow!`) posts a message on repeat (every 5 minutes to 24 hours, up to 10 per channel), for the channel's owner and moderators. It only posts while chat is active, with at least 5 messages since its last post, so quiet and offline channels get nothing. `!autopost list` shows each autopost's number, message and next post; `!autopost remove <number>` stops one.
- Reminders and autoposts survive restarts; a failed one is retried with backoff, and two Lys instances never run the same one.
- Lys announces new Letterboxd diary entries itself, replacing LetterboxdWatcher, with the same wording; the "see other entries" link now opens the member's activity page.
- Sleeper trades and waiver claims are announced in the league's channel, once each; a league's first check only records what already happened. `!ff alerts` shows which are announced, and `!ff alerts <trades|waivers> <on|off>` changes it.
- Adding, blocking or unblocking a channel (with `lys channel …` too) takes effect within seconds, without a restart.
- When Twitch says the bot was banned in a channel, or the channel's account is gone, Lys leaves it and `lys channel list` shows it as revoked; `lys channel add` brings it back.
- When Twitch revokes the bot account's own authorization, `/health` turns unhealthy and the log says to run `lys bot authorize`.
- An enrollment API for the website: `POST /channels` with a platform and channel ID adds the channel (the bot joins within seconds), taking its name from Twitch and refusing IDs Twitch doesn't know; `DELETE /channels/{platform}/{id}` turns Lys off in it, keeping its settings. It needs the shared token `SHARED_LYS_ENROLLMENT_TOKEN` and is only reachable on the server's internal network.

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
