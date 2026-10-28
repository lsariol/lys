# Lys performance review

Measured 2026-10-28 on `release/v0.0.0` (`bfeb27b`). Nothing in the code was changed; the measurements used temporary test files, since removed.

## How it was measured

- **In-process (Go benchmarks and timed tests):** the router with stub commands, the EventSub join with a fake Twitch that answers each call in 150 ms, the Sleeper player catalog decode.
- **Database:** the throwaway test Postgres (`scripts/test-db.sh`) seeded to a "grown" size: 1,000 channels, 50,000 viewers, 500,000 puzzle results, 5,200 jobs. The queries are copied from the code and timed with `EXPLAIN ANALYZE`; the startup steps are timed from Go. Prod today has 9 channels, 27 viewers and 915 game events, so every query there is under a millisecond.
- **Network (from this PC, no credentials):** Twitch Helix and id endpoints (401 answers, so the round trip without the work), the EventSub connect and welcome, Sleeper, and Letterboxd RSS.
- **Not measured:** a real chat → reply round trip through Twitch (it needs the bot running), OpenAI response times, Cove write latency, and the server's CPU count. The estimates below say where they rely on these.

## What isn't a bottleneck

| Flow | Measured |
|---|---|
| Router, per plain chat message (activity tracker, prefix, puzzle pattern) | 0.1 µs, no allocations |
| Router, per command (lookup, identity stub, reply queued) | 0.45 µs |
| Router, puzzle result (pattern match) | 1 µs |
| `identity.Resolve` upsert (once per viewer per 10 minutes, cached otherwise) | 0.7 ms |
| `connections.List` (channel reload, sync, enrollment), 1,000 channels | 2 ms |
| Scheduler claim with nothing due (every 5 s) | 0.02 ms |
| `isPuzzleRecorded`, channel puzzle stats | 0.02 ms, 0.5 ms (indexed) |
| Migrations when up to date, connect, schema check | 7–15 ms, 4 ms, 1 ms |
| EventSub connect and welcome | 170–200 ms |
| Twitch Helix round trip | 110–130 ms (new connection), ~70 ms of it Twitch |
| Sleeper player catalog (once a day) | 14.7 MB of JSON, 2.6 MB with gzip (Go asks for it): 0.13 s download, 70 ms decode, 37 MB allocated briefly |
| Sleeper NFL state | 55–65 ms |
| Letterboxd RSS feed | 0.16–0.45 s, 44–224 KB |

Lys's own processing is microseconds; everything that takes time is waiting on Twitch, an external API, or a queue.

---

## Bottlenecks, ranked by impact

Impact weighs how likely it is to happen, how many people notice, and whether it can cascade. "Today" is prod's current size; "growth" is hundreds of channels.

### 1. Slow commands stall all of chat (high, today)

**What happens.** Every incoming message, command or not, goes through one pool of 8 workers (`workers = 8` in `cmd/lys/main.go`). A command holds its worker until it finishes: `!gpt` up to 30 s, `!catfact` and `!dog` up to 20 s, Letterboxd and Sleeper up to 10 s. When 8 slow commands are in flight, nothing else is handled: other commands, puzzle results, and the activity tracker that timers rely on.

**Measured.** 8 commands waiting on a 3 s API: a `!ping`-style command queued behind them replied after **3.0 s**, and a plain chat message reached the observers after **3.0 s**. In a flood of 20,000 messages with 40 slow (2 s) commands mixed in, the median message waited **2.0 s** and the slowest **4.0 s**.

**Why it can cascade.** When the workers are busy, the router's inbox (256) fills, then the Twitch ingest loop blocks, then EventSub's buffer (100) fills, and then the read loop stops reading the socket. A socket that isn't read doesn't answer Twitch's pings, and Twitch disconnects clients that miss them (close code 4002). Lys then reconnects and rejoins every channel, which is bottleneck 2. Eight people asking `!gpt` at once in one busy chat is enough to start this.

**Ways to improve:**
- Keep the fast path separate: observers, matching and command lookup run on the reader's side, and only the command itself runs on a worker.
- Cap slow commands separately: a `Concurrency` field in `Spec` (e.g. `!gpt` 2 at a time), with replies like "busy, try again in a moment" past it, so one command can't take every worker. This keeps commands simple to add: one optional field.
- Per-user and per-channel cooldowns for expensive commands (it fits the planned per-channel command settings).
- Never let the EventSub read loop block: if the buffer is full, drop the message and count the drops (logged once a minute), so the socket keeps answering pings.

### 2. Joining channels is one at a time, and the socket isn't read meanwhile (high at growth, low today)

**What happens.** `JoinAllChannels` subscribes to each channel with its own Helix call, in sequence, while holding the channel lock. At startup it runs before the read loop starts. On a reconnect it runs inside the read loop. Either way nothing reads the socket until every channel is joined, so chat waits and pings go unanswered (bottleneck 1's 4002 risk again).

**Measured** (fake Twitch, 150 ms per subscribe, close to the measured round trip):

| Channels | One at a time (today) | 8 at a time |
|---|---|---|
| 9 | 1.4 s | 0.3 s |
| 100 | 15 s | 2.0 s |
| 300 | 45 s | 5.7 s |

**Limits to plan for.** Twitch allows 300 subscriptions per WebSocket and 3 WebSockets per token, so a single connection can never hold more than 300 channels.

**Ways to improve:**
- Start the read loop first, then subscribe, so pings and joined channels' chat keep flowing while the rest join.
- Subscribe with bounded concurrency (about 8), staying under Helix's rate limit (it's reported in `Ratelimit-Remaining` on each answer).
- Don't hold the channel lock across HTTP calls: take it only to record each result.
- Past 300 channels: shard across up to 3 connections, or move to EventSub conduits, which Twitch provides for bots in many channels.

### 3. Chat is paced per channel, but Twitch limits the whole account (high at growth, possibly today)

**What happens.** `internal/adapters/twitch/chat` spaces messages 1.5 s apart *per channel*. Twitch's chat limits are per account across all channels: 20 messages per 30 s when the bot isn't a moderator or VIP (100 when it is), and 1 per second per channel. Breaking them makes Twitch **ignore the bot for an hour** in every channel where it isn't a mod. Nothing in Lys counts messages across channels, so timers, Letterboxd and Sleeper announcements and replies in several channels can add up. 21 timers that come due in the same 30 s are enough.

**Caveat.** Twitch says the Send Chat Message API (what Lys uses) has a separate bucket from the regular one, with its own size in the `Ratelimit-Limit` header. That bucket may be larger, so whether this bites today is unknown until the headers are read.

**Ways to improve:**
- First, log `Ratelimit-Limit` and `Ratelimit-Remaining` from Helix's answers for a few days, to learn the real bucket.
- One account-wide limiter in front of the per-channel queues, fed by those headers. Replies go first, then announcements and timers.
- Mod-aware pacing (already planned): faster where the bot is a moderator or VIP, using `twitch_connection_details.bot_is_moderator`.

### 4. The bot's token is refreshed on every join, twice at startup (medium)

**What happens.** `RefreshUserAccessToken` (a Twitch call, then two Cove writes) runs in `auth.Initilize`, again in `JoinAllChannels` right after, on every reconnect, and on every channel change that `Sync` acts on. The token is valid for about 4 hours.

**Cost.** About 0.2–0.4 s before each join (one Twitch round trip plus two Cove writes; Cove's latency wasn't measured). It also sits between the EventSub welcome and the first subscribe, inside Twitch's 10 s window. And each refresh adds two versions to Cove's history, so restarts, reconnects and enrollments keep growing it.

**Ways to improve:**
- Refresh only when needed: keep the expiry from the refresh answer (or `/validate`) and refresh when less than about 10 minutes remain, or on a 401.
- Write to Cove only when the token actually changed.

### 5. The Letterboxd poller slows down linearly as feeds are added (medium at growth)

**What happens.** It checks one subscription at a time, spread across 25 minutes, but never closer than 5 s apart. Past 300 subscriptions a round takes longer than 25 minutes: 1,000 subscriptions take 83 minutes, so new diary entries are announced over an hour late. A feed that several channels follow is fetched once per channel.

**Ways to improve:**
- Fetch each feed URL once per round and announce it in every channel that follows it.
- Conditional requests (`If-None-Match` / `If-Modified-Since`): unchanged feeds come back as a small 304 instead of up to 224 KB.
- A few fetches at a time (e.g. 4) instead of one, while keeping a polite gap for Letterboxd.

### 6. Two puzzle stats queries scan whole tables (low today, medium at growth)

**Measured at 500,000 results / 50,000 viewers:**
- **Stats by puzzle number:** **140 ms**, three parallel full scans of `game_events`. The query reads `(details->'puzzle_id')::INT`, which doesn't match the index's `(details->>'puzzle_id')::int`, and no index leads with the puzzle number anyway. It grows with the table: about 1.4 s at 5 million rows, against a 3 s command timeout.
- **Stats by viewer name:** **12 ms**, a full scan of `platform_users` for `lower(display_name)`.

**Ways to improve (one migration, additive):**
- Write the puzzle query with `->>`, and add `CREATE INDEX ... ON lys.game_events (((details->>'puzzle_id')::int), ((details->>'time_seconds')::int)) WHERE game_code = 'oneuppuzzle' AND NOT is_flagged`.
- Add `CREATE INDEX ... ON lys.platform_users (platform, lower(display_name))`.

### 7. Startup scans the old columns on every start (low, temporary)

**Measured.** `LinkLegacyRows` with nothing to link takes **0.2–0.3 s** at 500,000 rows (several full scans for `connection_id IS NULL` and `platform_user_id IS NULL`). Today it's a few milliseconds.

**Ways to improve.** It goes away with the planned removal of the rollback copies (step 1 removes `LinkLegacyRows`). Until then, nothing needs doing at prod's size.

### 8. The database pool size isn't set (low, depends on the server)

**What happens.** pgxpool defaults to the larger of 4 and the number of CPUs. The channel-change listener holds one connection permanently, leaving the rest for 8 command workers, the scheduler, the pollers and the enrollment API. On a 4-CPU server that's 3 connections, which is fine at today's millisecond queries, but a slow query (bottleneck 6) makes commands queue for a connection.

**Ways to improve.** Set `pool_max_conns` explicitly (e.g. 12) in `LYS_DATABASE_URL`, or in code from a setting, so it doesn't depend on the host.

### 9. Every chat message is printed to the container log, with no rotation set (low, operational)

**What happens.** `logMessage` prints every message from every channel to stdout. The compose file sets no `logging:` options, so unless the server's Docker daemon rotates logs, the log file grows forever. It costs disk space, not speed, but it's the one thing here that grows without limit. It also keeps viewers' messages on disk.

**Ways to improve.** Add `logging: {driver: json-file, options: {max-size: "10m", max-file: "5"}}` to the compose file. Or log only commands and errors, with a debug setting to print all chat.

### 10. Reminders run up to 5 s late (very low)

**What happens.** The scheduler checks every 5 s. Each idle check costs 0.02 ms, and a batch of 20 due jobs is about 40 small queries (one claim update and one finish per job).

**Ways to improve (only if it ever matters).** Sleep until the next `run_at` (capped at 5 s), and wake on a `pg_notify` when a job is added. Claim with a single `UPDATE ... RETURNING` instead of one update per job.

---

## Suggested order

1. **Bottleneck 1** (worker isolation, a concurrency cap for slow commands, a non-blocking EventSub reader). It's the only one that can disrupt chat today, and it removes the cascade into reconnects.
2. **Bottleneck 3, first step:** log the Helix rate-limit headers. It's a few lines, and it settles whether the account-wide limiter is urgent.
3. **Bottlenecks 2 and 4 together:** both are in the join path, and reading first, subscribing in parallel and refreshing only when needed fit in one change.
4. **Bottleneck 9:** a compose-file change, before logs grow on the server.
5. **Bottlenecks 6, 5 and 8** as the data and channel counts grow. **Bottleneck 7** disappears with the planned cleanup, and **bottleneck 10** only matters if second-level timing ever does.

To measure the real end-to-end time later, the cheapest step is a log line per command with its total time (message timestamp from Twitch → reply accepted by Helix). The router already times commands that fail.
