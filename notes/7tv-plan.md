# Lys — 7TV Editor-Controlled Emote Management

**Corrected implementation handoff — October 29, 2026**  
**Repository:** https://github.com/lsariol/lys  
**Supersedes:** the earlier handoff that authorized Twitch moderators and excluded switching sets.  
**Status:** planned for Lys v0.3.0; not started. Reviewed and extended 2026-10-29.

**Decisions (Lu, 2026-10-29):**
- Fully hands-off for the operator: Lys accepts its own editor invites (§2C), and onboarding runs through the lys.mobasity.com website.
- Rename stays in v1.
- **Never delete anything on 7TV:** no set, emote or editor deletes, ever (§1).
- Changes are logged, not shown anywhere; the logs feed the central logging system, and later the streamer's account on the Lys website.
- The bot account is renamed `bot_moba` → `LysLink` on Twitch before this ships, since Lys's 7TV account is the bot's Twitch account.

## 1. What to build

Lys is a Twitch-chat interface for an existing streamer's **7TV editor permissions**. It should **not** invent a second independent editor roster based on Twitch moderator status.

A streamer maintains their editors in **7TV**. When a Twitch user issues a `!7tv` command in that streamer's chat, Lys matches their Twitch numeric ID to the Twitch connection of a 7TV account, verifies that account is the streamer or an **accepted 7TV editor** with the relevant permissions, and then performs the requested operation on the streamer's 7TV account.

### User experience

The commands (decided 2026-10-29). The streamer can always do anything an editor can.

| Command | Who | What it does |
|---|---|---|
| `!7tv add <name or link> [local name]` | Editors with Manage emote sets | Adds an emote to the **currently active** set; with a local name, under that name |
| `!7tv remove <name>` | Editors with Manage emote sets | Removes one emote from the active set, by its name there |
| `!7tv rename <name> <new name>` | Editors with Manage emote sets | Changes its name in the active set (not the emote's own name) |
| `!7tv undo` | Whoever could make that change | Undoes the channel's last change (below) |
| `!7tv sets` | Everyone | The streamer's normal sets, the active one marked |
| `!7tv set <name>` | Editors with Manage profile | Switches the active set |
| `!7tv search <term>` | Everyone | The 3 or 4 most used emotes with that name, with links |
| `!7tv editors` | Everyone | The channel's accepted 7TV editors and what each may do |
| `!7tv emote <name>` | Everyone | What an emote in this channel's set is: its own name, author and link |
| `!7tv status` | Everyone | The active set, slots used (412/1000), and whether Lys has the permissions it needs |

**`add` by name or by link:**
- **A link** (`https://7tv.app/emotes/<ID>`) or a bare 7TV ID adds that exact emote. The ID is read from the link locally; Lys never fetches the link.
- **A name** adds the most popular emote with **exactly** that name (7TV search, `exactMatch`, sorted by all-time top), skipping NSFW, unlisted and deleted emotes. Lu chose this over refusing ambiguous names, so the reply always names the emote it picked and links it: `Added KEKW (7tv.app/emotes/…) · 412/1000 slots`. A wrong pick is then obvious, and `!7tv undo` reverts it.
- **A local name** after either form adds it under that name.

**`!7tv undo`** reverts the channel's last change made through Lys, if it was within the last hour: it removes what was just added, re-adds what was just removed under its old name, reverts a rename, or switches back to the previous set. It's checked like the change it reverts (the sender needs that change's permission now), and it only reaches back one change. The last change per channel is kept in memory; a restart forgets it.

**Search links:** some channels block links from anyone but moderators, so where the bot isn't a mod they may be removed. The reply also gives each emote's name and author, so it's useful without them.

**Not in v1:** creating sets (with the never-delete rule they'd pile up, and 7TV limits sets per account; it belongs on 7TV's website, where they can also be deleted), deleting anything, bulk changes, emote flags (zero-width and the like), and managing editors.

**Do not add a `!7tv connect` step for each editor.** Users use their normal Twitch chat identities; Lys resolves the linked 7TV accounts. Linking a user's Twitch identity to their own 7TV account remains a 7TV-side requirement if it is not already linked.

Do not include global source-emote upload/deletion, editor-list administration, mass edits, or arbitrary-channel targeting in v1. `remove` takes one alias at a time.

**Never delete.** 7TV's API has delete operations for emote sets (`emoteSet(...).delete`), emotes and editor links (`userEditors.editor(...).delete`). Lys's 7TV code contains **none** of them, and a test fails the build if a delete operation ever appears in its GraphQL documents.

## 2. Authorization: two independent checks

**A. The human chat sender is authorized by the streamer's 7TV roster:**
1. Read the numeric Twitch channel ID from `req.Channel.ID` (not a supplied target name).
2. Resolve the broadcaster's 7TV user via `users.userByConnection(platform: TWITCH, platformId: <channel-id>)`.
3. Find the command sender in the streamer's editor list by Twitch ID: each editor record's `editor { connections { platform platformId } }` carries their Twitch numeric ID, so compare it with `req.User.ID`. One query returns the streamer, the active set and every editor with their state and permissions (verified live, without logging in; §4).
4. Permit if the sender **is the streamer** or is an **accepted** editor of that 7TV account with the specific permission:
   - **Add/remove/rename:** 7TV `emoteSet.manage`.
   - **Switch active set:** 7TV `user.manageProfile` (distinct, broader permission).
5. Deny if identity is missing, editor invitation is pending/rejected, permissions are insufficient, or the 7TV lookup fails. Do not grant access merely because `req.User.Roles` includes Twitch moderator.

**B. Lys itself is technically authorized to execute the API action:**
- The streamer must also grant the **7TV account operated by Lys** editor access.
- Lys authenticates to 7TV as **its own 7TV account**; its 7TV token is distinct from Twitch's bot token.
- To add/remove/rename, Lys requires 7TV `emoteSet.manage` on that streamer account.
- To switch the streamer's active set, Lys additionally needs 7TV `user.manageProfile`.
- The 7TV API authorizes the **Lys account**, not the human who typed in Twitch. **Therefore Lys must always enforce check A before making the API call.**

**C. Lys accepts its own editor invites (hands-off onboarding):**
- Every few minutes, Lys lists its pending invites (`users.me { editorFor { userId state } }`, authenticated).
- It accepts one (`userEditors.editor(editorId: <Lys>, userId: <streamer>).updateState(state: ACCEPT)`) only when the inviting 7TV user's Twitch connection is a channel Lys serves (an enabled, authorized connection in `platform_connections`). Anything else stays pending, untouched, and is logged once.
- It never rejects or deletes invites, and never changes the permissions it was given.

**Privilege caveat:** 7TV's `Manage Profile` permission is broader than switching emote sets. If the streamer will not grant it to the Lys account, `!7tv set` must be disabled for that channel, while emote add/remove can still work. Do **not** silently infer that `Manage emote sets` includes `Manage Profile`.

## 3. Command flow

```text
Twitch chat: !7tv add <emote>
        |
        v
Lys existing Twitch adapter -> router -> seventv command
        |
        v
Identify Twitch channel + sender (numeric IDs)
        |
        v
Resolve 7TV streamer + sender; check 7TV editor status/permission
        |
        +-- denied -> explain permission/linking issue; no mutation
        |
        v
Read current 7TV active emote set (or chosen set for switch)
        |
        v
Validate target, ownership, aliases, duplicates, set availability
        |
        v
Execute 7TV GraphQL mutation with Lys's own 7TV session
        |
        v
Check GraphQL errors, read back if ambiguous, reply in chat
```

### Trusted IDs and boundaries

- Only affect the streamer whose **Twitch channel received the message**.
- Always compare numeric Twitch IDs, never Twitch display names or `7TV` display names.
- Fetch roster/current set reasonably close to a mutation; do not trust stale long-lived permissions.
- Prevent editors from changing another streamer's set by supplying a different username, set owner, or URL.
- Do not assume all editor accounts have a Twitch connection to map; fail closed and tell them to link their Twitch account to 7TV.

## 4. Current upstream API facts (verified from 7TV source)

**GraphQL v4:** `POST https://api.7tv.app/v4/gql` with a JSON GraphQL body; authenticated writes use `Authorization: Bearer <7TV session>`.

- Streamer/sender lookup: `users.userByConnection(platform: TWITCH, platformId: ...)`.
- Streamer roster: `User.editors { editorId state permissions { emoteSet { manage } user { manageProfile } } ... }`.
- **One request does a command's reading:** `userByConnection` for the streamer, with `style { activeEmoteSetId }`, `emoteSets { id name }` (for `!7tv sets`) and `editors { ... editor { connections { platform platformId } } }`.
- Active set: `User.style.activeEmoteSetId` and `activeEmoteSet`.
- Set operations: `emoteSets.emoteSet(id: ...).addEmote(...)`, `.removeEmote(...)`, `.updateEmoteAlias(...)`.
- Active-set switch: `users.user(id: ...).activeEmoteSet(emoteSetId: ...)`.
- For a set switch, the upstream mutation explicitly checks accepted editor status and `EditorUserPermission::ManageProfile`. It also checks that the requested set is of kind `Normal`.
- **The editor list is public.** `User.editors` has no permission guard (only billing does, in `apps/api/src/http/v4/gql/types/user/mod.rs`), and `UserEditor` includes `state` and `permissions`. Checked live on 2026-10-29 without logging in: a real channel's 7TV user, active set, and two accepted editors with their permissions and Twitch connections all came back. So Lys needs no extra permission to run check A.
- **Accepting an invite:** `userEditors.editor(editorId, userId).updateState(state: ACCEPT)`; states are `PENDING`, `ACCEPTED`, `REJECTED`.
- **Sessions:** a website login creates a session that lasts exactly 30 days (`auth.rs`). Using it never extends it: the session updater only writes `last_used_at`. 7TV staff can create a session with any expiry through `userSessions.create(userId, expiresAt)`, which needs the staff-only `ManageSessions` permission (`mutations/user_session.rs`).

**Source links:**
- 7TV schema: https://github.com/SevenTV/SevenTV/blob/main/apps/website/schema.graphql
- 7TV website's set mutations: https://github.com/SevenTV/SevenTV/blob/main/apps/website/src/lib/setMutations.ts
- 7TV active-set mutation permission check: https://github.com/SevenTV/SevenTV/blob/main/apps/api/src/http/v4/gql/mutations/user/operation.rs
- 7TV set permission checks: https://github.com/SevenTV/SevenTV/blob/main/apps/api/src/http/v4/gql/mutations/emote_set/operation.rs
- 7TV editor permission UI: https://github.com/SevenTV/SevenTV/blob/main/apps/website/src/components/dialogs/editor-permissions-dialog.svelte

**Authentication caveat:** 7TV's current website login creates a 7TV-specific session with a 30-day expiry. Research did **not** establish an officially supported permanent third-party bot OAuth + refresh flow. Do not scrape another user's credentials or request tokens via chat.

- **Ask 7TV first** whether a bot may use its own website session, and for a long-lived session for Lys's account (they can already create one, see §4): `opensource@7tv.app` (their README's contact), and their Discord, `discord.gg/7TV`, linked from 7tv.app. Without a yes, the fallback is the operator logging in as LysLink once a month, which isn't hands-off.
- **Cove key:** `LYS_SEVENTV_TOKEN`, entered in `cove shell`.
- **Rotation without a redeploy:** Lys reads the key again from Cove after a 401 (at most every few minutes), so a new session takes effect on its own.
- **Expiry warnings:** the session is a JWT whose `exp` claim is its expiry (Fumobot's script reads it the same way). Lys reads it (never logging the token) and warns a week ahead in the logs and in `lys 7tv status` (`Needs attention: 7TV session expires 2026-11-28 10:00.`).
- Redact tokens from logs; on expiry, only this feature stops, with errors that start `7TV:` and say how to fix it.

Relevant source:
- https://github.com/SevenTV/SevenTV/blob/main/apps/api/src/http/v4/rest/auth.rs
- https://github.com/SevenTV/SevenTV/blob/main/apps/api/src/http/middleware/session.rs

**The API isn't a published contract.** v4 is the API 7TV's own website uses, so it can change without notice. Lys makes a break obvious: a `7TV:` error naming the operation and what came back, not a vague failure. `lys 7tv check` runs the read-only lookups (`users.me`, a test channel's streamer, set and editors) and says what failed; run it after 7TV changes something, or when errors start.

**Not used: automated website logins.** Fumobot renews its token every 5 minutes by logging into 7TV automatically. It stores the bot's Twitch website cookies (as powerful as the account's password), presents itself as the Chrome browser at the network level to pass bot protection, and scrapes the login pages. Lys doesn't do this; the long-lived session from 7TV is the route.

The ready-made GraphQL for each operation is in `notes/7tv-api-reference.md`; where it and this plan differ, this plan wins.

## 5. How it fits Lys

Lys already has:
- `internal/commands/command.go` (`Spec`, `Run`, `Request`, `Reply`).
- `internal/router/router.go` (Twitch chat matching, job timeouts/concurrency, replies).
- `cmd/lys/main.go` (command registration and shared HTTP client).
- Cove-backed config.

Add:
```text
internal/features/seventv/
    client.go         # minimal typed GraphQL client
    identity.go       # Twitch-ID -> 7TV account and editor-list lookup
    emotes.go         # add/remove/rename
    sets.go           # list/switch active set
    service.go        # authorization, validation, errors
    *_test.go
internal/commands/cmds/seventv/
    seventv.go        # !7tv command parsing/chat replies
    seventv_test.go
```

**Permission enforcement:** Do not use `Spec.Sub = Moderator` or Lys's Twitch `req.Allowed(commands.Moderator)` as the gate. The whole command may be registered as `Everyone`; its handler must perform **7TV-specific** checks for every action. It is acceptable to make `status`/`sets` readable by everyone while restricting mutations.

**Storage:** No per-editor token database and no mirrored roster is necessary. Resolve from 7TV on demand with a short TTL if performance requires it. An optional mapping/cache from Twitch broadcaster ID to 7TV user ID is fine but should not override current upstream permission state. No database migration should be necessary for an initial version unless a configuration need actually emerges.

## 6. Safety and behavior details

- Use exact emote IDs or URLs for additions; don't silently choose among multiple search hits with the same name.
- Remove by exact active-set alias (resolve it to an emote ID first). Handle aliases and duplicate IDs safely. Check 7TV upstream bug https://github.com/SevenTV/SevenTV/issues/216 before implementing add.
- Read the streamer's **current** active set on each write to avoid editing a set that has since changed.
- **Check permissions fresh before every change.** Only the Twitch-to-7TV user lookup may be cached briefly; the editor list and states are read again before each mutation, so a revoked editor is stopped at once.
- **Writes to one set take turns:** a lock per 7TV set ID (in memory; Lys runs as one instance). Two editors adding at the same moment could otherwise add the same emote twice (bug #216). `Spec.Concurrency` is across all channels, so it doesn't replace this.
- For `!7tv set`, offer only the streamer's eligible sets and verify that the chosen set is allowed for that streamer. Use a unique ID when names are ambiguous.
- Handle GraphQL HTTP errors **and** GraphQL `errors` in successful HTTP responses.
- Respect rate limiting and bounded timeouts. Never blindly repeat writes after an ambiguous timeout; check current state first.
- Provide concise messages: `You aren't a 7TV editor for this channel`, `You need 7TV Manage Profile to switch sets`, `Lys needs 7TV permissions from the streamer`, `7TV authentication needs renewal`, etc.
- Keep 7TV dependency optional: a failed 7TV integration must not prevent Twitch chat/Lys from starting.
- Log actor Twitch ID, streamer Twitch ID, 7TV user ID, action, target set/emote, and success/failure, one `[7TV]` line per change. Never log bearer tokens. Nothing is shown in chat beyond the reply; the lines go to the central logging system later.

## 7. Phases and acceptance

**Before phase 1:** ask 7TV about the bot session (§4); Lu renames the bot to LysLink on Twitch.

**Phase 1 — 7TV integration/auth feasibility:** Validate 7TV session provisioning, `users.me`, listing and accepting a pending invite from a test channel, active-set lookup, and minimum Lys editor permissions. (Reading the editor roster is already confirmed.)

**Phase 2 — Emote actions:** Add/remove/rename using existing normal set with authorization based **only** on 7TV's editor permission grants.

**Phase 3 — Set switching:** Add `sets`/`set`, with separate 7TV `Manage Profile` permission, broadcaster choice, and safe verification.

**Phase 4 — Hardening:** Tests, 7TV errors, 429s, revoked editors, session expiry, alias conflicts, race conditions, and documentation.

**Release (v0.3.0):** after the lys.mobasity.com onboarding has its 7TV step ("add LysLink as an editor with Manage emote sets, and Manage profile if you want set switching", with a link to the streamer's 7TV editors page), and `lys help new-channel` says the same.

**Acceptance tests:**
1. 7TV editor with Manage emote sets, but **without Twitch moderator**, can add/remove in this streamer's chat.
2. Twitch moderator **not** on 7TV's editor list cannot mutate.
3. Pending/rejected editor cannot mutate.
4. Accepted 7TV editor without `Manage emote sets` cannot mutate.
5. Accepted 7TV editor with `Manage emote sets` but no `Manage Profile` can edit emotes but **cannot switch** sets.
6. Accepted editor with `Manage Profile` can switch to one of the streamer's permitted normal sets, if Lys has the same permission.
7. A 7TV editor cannot mutate a different streamer's account by supplying a target name/ID.
8. Revoked editors lose permission on the next validation (respect short cache expiry).
9. Stale active set, duplicate emotes, alias collisions, rate limits, unknown outcomes, and expired session are handled without uncontrolled changes.
10. Existing Lys commands and its Twitch connection remain operational if 7TV fails.
11. Lys accepts a pending invite from a streamer whose channel it serves, and leaves one from anyone else pending.
12. No GraphQL document in Lys's 7TV code contains a delete operation (a test).
13. Two adds to one set at the same moment don't add the same emote twice.
14. A new session stored in Cove is used after the next 401, without a restart.
15. `add <name>` picks the most popular emote with exactly that name, never an NSFW, unlisted or deleted one, and the reply names and links it.
16. `add <link> <local name>` adds that exact emote under the local name; a link to anywhere but 7tv.app is refused, and never fetched.
17. `undo` reverts each kind of change (add, remove, rename, set switch), only the channel's last one within the hour, and only for someone allowed to make that change.
18. `editors`, `sets`, `search`, `emote` and `status` work for everyone and change nothing.

## 8. What is *not* required

- **Not** a separate 7TV editor management website.
- **Not** a token for each human editor.
- **Not** 7TV permissions mirrored into Lys's Postgres database.
- **Not** Twitch moderator access by default.
- **Not** a dedicated microservice.
- **Not** extra 7TV account/profile admin permissions unless the streamer wants active-set switching.

**Summary:** 7TV remains the source of truth for who can manage the streamer's emotes. Lys is just the Twitch-chat interface and an API executor with its own permitted 7TV account.
