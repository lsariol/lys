# Lys × 7TV API Integration — Findings & Implementation Reference

**Prepared:** October 29, 2026  
**Project:** Lys (Twitch bot; bot account: `LysLink`)  
**Scope:** Authenticate with 7TV, authorize chat users using a streamer's 7TV editor list, manage emotes, and change active emote sets.  
**Evidence level:** API operations and payload shapes were checked against 7TV's public v4 GraphQL schema and first-party website code. **Authenticated requests have not yet been exercised with LysLink's token.**

> **Bottom line:** Lys can use **one 7TV bearer token** belonging to **LysLink** and **one GraphQL endpoint** for nearly all required operations. Each streamer must separately give LysLink the necessary 7TV editor permissions. Chat commands should additionally verify that the *requesting chat user* has the correct 7TV editor privileges for that streamer.

> **How to read this:** a reference for the 7TV v4 API calls Lys makes, kept next to `notes/7tv-plan.md`. Where they differ, the plan wins. In particular, the editor list is read fresh before every change, not cached for minutes (§6); Lys doesn't create sets (§5.10); and Fumobot's automated login (§7) isn't used.

---

## 1. Key findings

| Finding | Status | Impact on Lys |
|---|---|---|
| The current 7TV website uses GraphQL v4 at `https://api.7tv.app/v4/gql`. | **Source-verified** | Build one HTTP/GraphQL client rather than separate REST integrations. |
| Requests use `POST`, JSON (`query` + optional `variables`) and a 7TV bearer token. | **Source-verified in implementations** | Keep all token handling inside the 7TV client; load the secret from Cove. |
| User lookup by linked Twitch ID is supported. | **Schema-verified** | Reuse Twitch IDs already known by Lys. |
| Streamer editors and their permissions are queryable. | **Schema-verified** | Reuse 7TV's editor model; don't create a parallel editor-role database. |
| Add, remove, rename, create-set and switch-set operations exist. | **Schema/first-party-code verified** | Required core features are possible through v4. |
| Managing emote sets and changing a user's active set require **different editor permissions**. | **Source-verified** | `emoteSet.manage` for set edits; `user.manageProfile` to switch the active set. |
| A 7TV token is distinct from a Twitch OAuth token. | **Source-verified** | The existing Lys Twitch token does not replace 7TV authentication. |
| Automated token renewal can be attempted by replaying a Twitch login session (Fumobot approach). | **Observed in public third-party code; not validated today** | Do **not** make unattended browser-cookie login a prerequisite for initial delivery. |

### Important distinction: authorization happens twice

1. **Requester authorization:** Is Alice (the chatter issuing `!7tv add`) the broadcaster or an **accepted** 7TV editor with emote-set management privileges for this streamer?
2. **Service authorization:** Has the broadcaster granted **LysLink itself** permission to make that change through 7TV?

Passing the first check does not guarantee the second check. Keep an audit trail identifying the requesting chatter, because 7TV will see the mutation performed by LysLink's token.

---

## 2. Endpoint and HTTP contract

**Endpoint for all GraphQL examples below:**

```http
POST https://api.7tv.app/v4/gql
Authorization: Bearer <LYSLINK_7TV_BEARER_TOKEN>
Content-Type: application/json
```

**Request envelope:**

```json
{
  "query": "query or mutation text goes here",
  "variables": {
    "name": "value"
  }
}
```

**Quick token test (read-only):**

```bash
# SEVENTV_TOKEN must be set in the shell from a secure secret source.
curl --fail-with-body -sS 'https://api.7tv.app/v4/gql' \
  -H "Authorization: Bearer $SEVENTV_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"query":"query { users { me { id } } }"}'
```

A valid response is expected to include `data.users.me.id`. If `me` is null or errors are returned, treat the token as invalid/insufficient until investigated. **Do not log or print live bearer tokens.**

**GraphQL error handling:** GraphQL can return HTTP 200 with an `errors` array. A successful HTTP status is *not* proof that the operation succeeded. Check HTTP status, JSON decoding, the GraphQL `errors` array, and the expected `data` fields.

**Version note:** Some older bots use `https://7tv.io/v3/gql` and older mutation names. Do not mix v3 payloads with the v4 endpoint.

---

## 3. Permissions Lys needs

| Action | 7TV editor permission LysLink needs | Permission the requesting editor should have |
|---|---|---|
| Read public stream/emote data | Typically no write privilege | N/A |
| Add/remove/rename emotes in a set | `emoteSet.manage` | `emoteSet.manage` |
| Change set metadata such as name | `emoteSet.manage` | `emoteSet.manage` |
| Create a new set | `emoteSet.create` (check actual account entitlement too) | `emoteSet.create` |
| Change the streamer's **active** set | `user.manageProfile` | `user.manageProfile` |

**Editor records must be `state: ACCEPTED`.** A `PENDING` or `REJECTED` editor is not authorized. The streamer (owner) can be allowed directly after confirming that the command channel maps to their linked 7TV user.

**Least-privilege guidance:** Ask for `manage` first; only request `manageProfile` from streamers who want active-set switching. `manageProfile` covers more than just switching sets, so it should not be described to streamers as a narrow set-only permission. Do not assume that simply being named an editor is enough.

---

## 4. Identifier mapping

| ID | Example placeholder | Source | Where it is used |
|---|---|---|---|
| Twitch user ID | `TWITCH_USER_ID` | Existing Lys/Twitch channel metadata | Lookup streamer, match chatting editors |
| 7TV user ID | `7TV_USER_ID` | `users.userByConnection` | Editor list, active-set mutation, set creation |
| 7TV emote set ID | `EMOTE_SET_ID` | `style.activeEmoteSetId` or `emoteSets` | Read/edit a set |
| 7TV emote ID | `EMOTE_ID` | `emotes.search`, emote URL, or set contents | Add/remove/rename a specific emote |

**Use stable Twitch IDs for comparisons, not usernames.** `platformId` is typed as a string in the v4 GraphQL schema even when its content is a numeric-looking Twitch ID.

---

## 5. API operation reference

The following examples are **GraphQL documents plus variables**. Send each as a JSON object with `query` and `variables` to the endpoint in §2. The field names and arguments are derived from the v4 schema. Placeholder IDs are illustrative, not real account data.

### 5.1 Verify the authenticated account

```graphql
query WhoAmI {
  users {
    me {
      id
    }
  }
}
```

Expected path: `data.users.me.id`. Confirm that this resolves to the intended LysLink 7TV account before allowing writes.

### 5.2 Find a streamer by their Twitch ID

```graphql
query GetStreamer($twitchId: String!) {
  users {
    userByConnection(platform: TWITCH, platformId: $twitchId) {
      id
      style {
        activeEmoteSetId
      }
      emoteSets {
        id
        name
      }
    }
  }
}
```

```json
{
  "twitchId": "TWITCH_USER_ID"
}
```

This returns the linked 7TV account, its active set ID, and visible/accessible sets. Handle `null` if the streamer has no matching 7TV linkage.

### 5.3 Read a streamer's editors and effective privileges

```graphql
query GetEditors($userId: Id!) {
  users {
    user(id: $userId) {
      editors {
        editorId
        state
        permissions {
          emoteSet {
            create
            manage
          }
          user {
            manageProfile
          }
        }
        editor {
          connections {
            platform
            platformId
            platformUsername
          }
        }
      }
    }
  }
}
```

```json
{
  "userId": "7TV_USER_ID"
}
```

**Authorization logic:** Find the accepted editor with `connections[].platform == TWITCH` and `connections[].platformId == requester's Twitch ID`; then check the field required for the specific command. The broadcaster is handled separately. You may also query `users.me.editorFor` to discover the channels where LysLink is an editor.

### 5.4 Read all emotes in a specific set

```graphql
query GetEmoteSet($setId: Id!, $page: Int!) {
  emoteSets {
    emoteSet(id: $setId) {
      id
      name
      capacity
      emotes(page: $page, perPage: 50) {
        totalCount
        pageCount
        items {
          alias
          emote {
            id
            defaultName
          }
        }
      }
    }
  }
}
```

```json
{
  "setId": "EMOTE_SET_ID",
  "page": 1
}
```

**Pagination:** Iterate until all pages have been fetched; do not assume one page includes the whole set. The alias (the name used *in this set*) can differ from the emote's default name.

### 5.5 Search 7TV emotes by name

```graphql
query SearchEmotes($term: String!, $sort: Sort!) {
  emotes {
    search(query: $term, sort: $sort, page: 1, perPage: 10) {
      totalCount
      items {
        id
        defaultName
      }
    }
  }
}
```

```json
{
  "term": "KEKW",
  "sort": {
    "sortBy": "TOP_ALL_TIME",
    "order": "DESCENDING"
  }
}
```

The `sort` argument is **required** by the v4 schema. Resolve the selected emote's ID before calling an add mutation. Name matching may be ambiguous; consider accepting direct 7TV emote IDs/URLs.

### 5.6 Add an emote to a set

**Mutation:**

```graphql
mutation AddEmote($setId: Id!, $emote: EmoteSetEmoteId!) {
  emoteSets {
    emoteSet(id: $setId) {
      addEmote(id: $emote) {
        id
      }
    }
  }
}
```

**Variables:**

```json
{
  "setId": "EMOTE_SET_ID",
  "emote": {
    "emoteId": "EMOTE_ID",
    "alias": "KEKW"
  }
}
```

**Complete HTTP JSON body (ready to adapt):**

```json
{
  "query": "mutation AddEmote($setId: Id!, $emote: EmoteSetEmoteId!) { emoteSets { emoteSet(id: $setId) { addEmote(id: $emote) { id } } } }",
  "variables": {
    "setId": "EMOTE_SET_ID",
    "emote": {
      "emoteId": "EMOTE_ID",
      "alias": "KEKW"
    }
  }
}
```

The `alias` field is optional; the server can use the emote's default name when omitted. If the alias conflicts with an existing entry, surface an understandable error in Twitch chat.

### 5.7 Remove an emote from a set

```graphql
mutation RemoveEmote($setId: Id!, $emote: EmoteSetEmoteId!) {
  emoteSets {
    emoteSet(id: $setId) {
      removeEmote(id: $emote) {
        id
      }
    }
  }
}
```

```json
{
  "setId": "EMOTE_SET_ID",
  "emote": {
    "emoteId": "EMOTE_ID",
    "alias": "KEKW"
  }
}
```

Resolve the **existing** set entry before removal. The optional alias can matter when the same underlying emote is present under multiple aliases.

### 5.8 Rename an emote alias within a set

```graphql
mutation RenameEmote(
  $setId: Id!
  $emote: EmoteSetEmoteId!
  $alias: String!
) {
  emoteSets {
    emoteSet(id: $setId) {
      updateEmoteAlias(id: $emote, alias: $alias) {
        id
        alias
      }
    }
  }
}
```

```json
{
  "setId": "EMOTE_SET_ID",
  "emote": {
    "emoteId": "EMOTE_ID",
    "alias": "OldName"
  },
  "alias": "NewName"
}
```

This changes the **alias in that specific set**, not the emote's global/default name.

### 5.9 Change a streamer's active emote set

```graphql
mutation SwitchSet($userId: Id!, $setId: Id) {
  users {
    user(id: $userId) {
      activeEmoteSet(emoteSetId: $setId) {
        id
        style {
          activeEmoteSetId
        }
      }
    }
  }
}
```

```json
{
  "userId": "7TV_USER_ID",
  "setId": "NEW_EMOTE_SET_ID"
}
```

**Permission:** `user.manageProfile` on the streamer's 7TV account. The v4 API source explicitly checks this permission for non-owner requests. The mutation accepts a nullable `setId` in the schema; Lys should nevertheless restrict chat commands to valid, intentionally selected sets.

### 5.10 Create a new emote set

```graphql
mutation CreateSet($userId: Id!, $name: String!) {
  emoteSets {
    create(ownerId: $userId, name: $name, tags: []) {
      id
      name
    }
  }
}
```

```json
{
  "userId": "7TV_USER_ID",
  "name": "Halloween"
}
```

**Permission:** Separate set-creation permission (`emoteSet.create`), and possibly account/entitlement restrictions. Creation does **not** automatically make the set active; use §5.9 for that.

### 5.11 Other supported set operations

The v4 `emoteSets.emoteSet(id: ...)` mutation also exposes `name`, `tags`, `capacity`, `updateEmoteFlags`, and `delete`. These can be deferred until Lys has its basic add/remove/switch functionality. Capacity changes and destructive deletion should receive additional confirmation/authorization controls if eventually exposed through chat.

---

## 6. Recommended Go integration boundary

Keep GraphQL out of Lys's individual Twitch command handlers. Use a 7TV client with a single internal HTTP transport/helper and typed operations:

```go
type SevenTVClient interface {
    WhoAmI(ctx context.Context) (string, error)
    GetUserByTwitchID(ctx context.Context, twitchID string) (*User, error)
    GetEditors(ctx context.Context, userID string) ([]Editor, error)
    GetEmoteSets(ctx context.Context, userID string) ([]EmoteSet, error)
    GetEmotes(ctx context.Context, setID string) ([]Emote, error)
    SearchEmotes(ctx context.Context, query string) ([]Emote, error)
    AddEmote(ctx context.Context, setID, emoteID, alias string) error
    RemoveEmote(ctx context.Context, setID, emoteID, alias string) error
    RenameEmote(ctx context.Context, setID, emoteID, oldAlias, newAlias string) error
    SwitchSet(ctx context.Context, userID, setID string) error
    CreateSet(ctx context.Context, userID, name string) (string, error)
}
```

*Suggested internal helper* (signature may change to suit Lys's existing dependencies):

```go
func (c *Client) graphql(
    ctx context.Context,
    query string,
    variables any,
    out any,
) error
```

**Transport responsibilities:** securely retrieve/use bearer token; set authorization/JSON headers; time out requests; bound response bodies; decode typed results; inspect GraphQL `errors`; classify unauthorized, forbidden, rate-limited, not-found, and temporary failures; never emit credential values into logs.

### Suggested chat command workflow

```text
Incoming chat message: !7tv add KEKW
  -> identify current Twitch channel and chatter by stable Twitch IDs
  -> resolve channel's 7TV user and active set
  -> authorize chatter (broadcaster or accepted 7TV editor with manage privilege)
  -> ensure LysLink can modify the streamer's target set
  -> resolve emote name/URL to 7TV emote ID
  -> check set contents for conflicts/duplicates
  -> send mutation using LysLink's single bearer token
  -> inspect data + errors
  -> audit (channel ID, requester Twitch ID, operation, set ID, emote ID, outcome)
  -> respond in Twitch chat
```

### Data/cache design

- Store **LysLink's 7TV bearer token in Cove**, not in Git or plaintext logs.
- Cache streamer's 7TV user ID and active emote set ID, but refresh set IDs after a switch.
- Cache editor lists briefly (e.g., **2–5 minutes**, a design recommendation, not an API-prescribed TTL); avoid letting revoked editors retain permission indefinitely.
- Fail closed when permissions cannot be reliably established. A previous successful permission lookup should not grant permanent access.
- Do not copy 7TV editor privileges into a second authoritative permissions system. Persist only what Lys needs for caching, observability, or audit.

---

## 7. Security and token lifecycle

### What is confirmed

- The bearer token authorizes requests as **LysLink's 7TV account**.
- It is separate from the Twitch OAuth access token used for chat/Helix.
- It must be considered a sensitive credential and stored in Cove.

### What requires further validation

- The exact lifetime/refresh behavior of 7TV login-session tokens in the current deployment.
- Whether an official, supported long-lived third-party service token mechanism becomes available.
- Whether an automated login/renewal mechanism is stable and compliant with 7TV/Twitch expectations.

**Fumobot reference:** [`melon095/Fumobot/Scripts/cron/SevenTVToken.py`](https://github.com/melon095/Fumobot/blob/main/Scripts/cron/SevenTVToken.py) uses a stored Twitch browser session to perform a new 7TV login and write an updated token. A systemd timer invokes it every five minutes. This demonstrates an *implementation approach*, **not a validated or officially supported refresh protocol**. It also increases risk because it requires highly sensitive Twitch session cookies; do **not** copy this into Lys without separate evaluation.

**Suggested initial strategy:** Manually acquire and securely store a token, validate it with `users.me`, record expiry when available, and explicitly report authentication failures. Add renewal only after investigating a supported mechanism.

---

## 8. Implementation plan and test matrix

### Phase 1 — Authentication and read operations

- [ ] Load the token via Cove; never include it in log fields.
- [ ] Send v4 `WhoAmI` and ensure the returned identity is LysLink.
- [ ] Look up a test Twitch streamer via `userByConnection`.
- [ ] Fetch active set, available sets, emote contents, and editors.
- [ ] Test a user without a 7TV linkage, an invalid token, and a missing set.

### Phase 2 — Authorization

- [ ] Match editor Twitch IDs using `connections[].platformId`.
- [ ] Enforce `state == ACCEPTED` and action-specific permissions.
- [ ] Grant the streamer/owner access to their own channel when identity mapping is verified.
- [ ] Verify a revoked editor loses access after cache refresh.
- [ ] Reject modifications when LysLink lacks the required editor rights.

### Phase 3 — Core mutations

- [ ] Add an emote by direct ID/URL and by searched name.
- [ ] Handle duplicates, name collisions, unlisted/unavailable emotes, and full sets.
- [ ] Remove by an emote's **current set alias**, resolving ID first.
- [ ] Rename alias; verify alias changes only within the target set.
- [ ] Validate 7TV mutation results instead of treating HTTP 200 as success.

### Phase 4 — Set management

- [ ] List selectable sets and switch active set (requires `manageProfile`).
- [ ] Confirm LysLink and requester both have appropriate permissions.
- [ ] Update cached active-set ID after successful switching.
- [ ] Optionally add create-set support if editor creation permissions are granted.

### Phase 5 — Reliability

- [ ] Classify/handle 401/403, GraphQL permission errors, rate limits, timeouts, and partial responses.
- [ ] Add sensible request timeouts, optional bounded retries on *safe/transient* failures, and structured audit events.
- [ ] Determine an acceptable token-renewal procedure before relying on the integration unattended.

**Do not assume live success from schema inspection alone.** Start with read-only operations, then verify every mutation against a disposable/test emote set before enabling public chat commands.

---

## 9. Limitations and questions to resolve

| Question | Current assessment | Next action |
|---|---|---|
| Will the retrieved 7TV token work against v4? | Expected from bearer-token usage; **not live-tested here**. | Run `WhoAmI` with LysLink token. |
| Does LysLink currently have the necessary editor permissions in each channel? | Unknown; account-specific. | Inspect each streamer's editor record and test a controlled mutation. |
| Does 7TV expose an officially supported refresh grant for bots? | Not established by the inspected source. | Check current official documentation/issues before automating. |
| Does an emote search always return the intended emote? | No; names can be ambiguous. | Prefer IDs/URLs where possible, and define a search ranking/selection policy. |
| Are there explicit published rate limits for these mutations? | Not established in this review. | Measure/respond to server rate-limit signals rather than inventing a fixed quota. |
| What happens if 7TV changes its internal v4 schema? | No stable-contract guarantee was established. | Maintain integration tests and surface clear compatibility failures. |

---

## 10. Primary sources (public repository)

**7TV first-party sources — strongest evidence:**

1. [7TV GraphQL schema (v4 fields, input types, permissions)](https://github.com/SevenTV/SevenTV/blob/main/apps/website/schema.graphql)
2. [7TV website endpoint configuration (`PUBLIC_GQL_API_V4`)](https://github.com/SevenTV/SevenTV/blob/main/apps/website/.env)
3. [7TV first-party emote-set mutations](https://github.com/SevenTV/SevenTV/blob/main/apps/website/src/lib/setMutations.ts)
4. [7TV first-party user/active-set mutations](https://github.com/SevenTV/SevenTV/blob/main/apps/website/src/lib/userMutations.ts)
5. [7TV v4 active-set permission enforcement](https://github.com/SevenTV/SevenTV/blob/main/apps/api/src/http/v4/gql/mutations/user/operation.rs)
6. [7TV v4 set-operation permission enforcement](https://github.com/SevenTV/SevenTV/blob/main/apps/api/src/http/v4/gql/mutations/emote_set/operation.rs)

**Third-party implementations — useful context, not canonical API documentation:**

7. [Fumobot token renewal approach](https://github.com/melon095/Fumobot/blob/main/Scripts/cron/SevenTVToken.py)
8. [Fumobot editor-fetching job](https://github.com/melon095/Fumobot/blob/main/src/Fumo.BackgroundJobs/SevenTV/FetchChannelEditorsJob.cs)
9. [RukuBot 7TV integration](https://github.com/rukukun/rukubot/blob/master/seventv.js)

---

## 11. Recommended implementation decision

**Build this as a small, isolated 7TV v4 client for Lys**, backed by the existing Cove secret store, and initially support the following sequence:

**Authenticate → resolve streamer → retrieve editor permissions → retrieve active set → add/remove emotes → switch active set.**

Use 7TV's editor list as the authority for which Twitch users can act, but enforce the correct permission **per action**, rather than treating all editor roles alike. Defer automatic token renewal and broad set-admin features until the core integration has been tested against 7TV's live API.

*This report is an implementation reference and handoff, not proof that authenticated v4 calls succeeded in production.*
