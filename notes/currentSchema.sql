   Index "lys.channel_events_pkey"
  Column  | Type | Key? | Definition
----------+------+------+------------
 event_id | uuid | yes  | event_id
primary key, btree, for table "lys.twitch_channel_events"


     Index "lys.channels_pkey"
 Column  | Type | Key? | Definition
---------+------+------+------------
 user_id | text | yes  | user_id
primary key, btree, for table "lys.twitch_channels"


   Index "lys.channels_username_key"
  Column  |  Type  | Key? | Definition
----------+--------+------+------------
 username | citext | yes  | username
unique, btree, for table "lys.twitch_channels"


                                           Table "lys.feature_reque
sts"
    Column    |           Type           | Collation | Nullable |
                   Default
--------------+--------------------------+-----------+----------+--
------------------------------------------------
 id           | bigint                   |           | not null | n
extval('lys.feature_requests_id_seq'::regclass)
 occurred_at  | timestamp with time zone |           | not null | n
ow()
 user_id      | text                     |           | not null |
 username     | text                     |           | not null |
 channel_id   | text                     |           | not null |
 channel_name | text                     |           | not null |
 platform     | text                     |           | not null |
 body         | text                     |           | not null |
 status       | lys.request_status       |           | not null | '
new'::lys.request_status
Indexes:
    "feature_requests_pkey" PRIMARY KEY, btree (id)
    "feature_requests_occurred_at_desc_idx" btree (occurred_at DESC
)


                    Sequence "lys.feature_requests_id_seq"
  Type  | Start | Minimum |       Maximum       | Increment | Cycle
s? | Cache
--------+-------+---------+---------------------+-----------+------
---+-------
 bigint |     1 |       1 | 9223372036854775807 |         1 | no
   |     1
Owned by: lys.feature_requests.id


      Index "lys.feature_requests_occurred_at_desc_idx"
   Column    |           Type           | Key? | Definition
-------------+--------------------------+------+-------------
 occurred_at | timestamp with time zone | yes  | occurred_at
btree, for table "lys.feature_requests"


  Index "lys.feature_requests_pkey"
 Column |  Type  | Key? | Definition
--------+--------+------+------------
 id     | bigint | yes  | id
primary key, btree, for table "lys.feature_requests"


   Index "lys.feeds_feed_url_key"
  Column  | Type | Key? | Definition
----------+------+------+------------
 feed_url | text | yes  | feed_url
unique, btree, for table "lys.letterboxd_feed_subscriptions"


                         Sequence "lys.feeds_id_seq"
  Type  | Start | Minimum |       Maximum       | Increment | Cycle
s? | Cache
--------+-------+---------+---------------------+-----------+------
---+-------
 bigint |     1 |       1 | 9223372036854775807 |         1 | no
   |     1
Owned by: lys.letterboxd_feed_subscriptions.id


       Index "lys.feeds_pkey"
 Column |  Type  | Key? | Definition
--------+--------+------+------------
 id     | bigint | yes  | id
primary key, btree, for table "lys.letterboxd_feed_subscriptions"


             Index "lys.feeds_user_id_letterboxd_username_key"
       Column        |          Type          | Key? |     Definiti
on
---------------------+------------------------+------+-------------
--------
 user_id             | text                   | yes  | user_id
 letterboxd_username | character varying(255) | yes  | letterboxd_u
sername
unique, btree, for table "lys.letterboxd_feed_subscriptions"


                                           Table "lys.game_events"
    Column    |           Type           | Collation | Nullable |
                 Default
--------------+--------------------------+-----------+----------+--
-------------------------------------------
 id           | bigint                   |           | not null | n
extval('lys.game_events_id_seq'::regclass)
 occurred_at  | timestamp with time zone |           | not null | n
ow()
 user_id      | text                     |           | not null |
 username     | citext                   |           | not null |
 channel_id   | text                     |           | not null |
 channel_name | text                     |           | not null |
 game_code    | text                     |           | not null |
 raw_message  | text                     |           | not null |
 details      | jsonb                    |           |          |
 is_flagged   | boolean                  |           | not null | f
alse
Indexes:
    "game_events_pkey" PRIMARY KEY, btree (id)
    "game_events_oneuppuzzle_channel_time_idx" btree (channel_id, (
(details ->> 'time_seconds'::text)::integer)) WHERE game_code = 'on
euppuzzle'::text AND is_flagged = false
    "game_events_oneuppuzzle_user_puzzle_idx" btree (user_id, ((det
ails ->> 'puzzle_id'::text)::integer)) WHERE game_code = 'oneuppuzz
le'::text
    "game_events_oneuppuzzle_username_idx" btree (username) WHERE g
ame_code = 'oneuppuzzle'::text AND is_flagged = false


                      Sequence "lys.game_events_id_seq"
  Type  | Start | Minimum |       Maximum       | Increment | Cycle
s? | Cache
--------+-------+---------+---------------------+-----------+------
---+-------
 bigint |     1 |       1 | 9223372036854775807 |         1 | no
   |     1
Owned by: lys.game_events.id


            Index "lys.game_events_oneuppuzzle_channel_time_idx"
   Column   |  Type   | Key? |                  Definition

------------+---------+------+-------------------------------------
----------
 channel_id | text    | yes  | channel_id
 int4       | integer | yes  | ((details ->> 'time_seconds'::text):
:integer)
btree, for table "lys.game_events", predicate (game_code = 'oneuppu
zzle'::text AND is_flagged = false)


          Index "lys.game_events_oneuppuzzle_user_puzzle_idx"
 Column  |  Type   | Key? |                 Definition

---------+---------+------+----------------------------------------
----
 user_id | text    | yes  | user_id
 int4    | integer | yes  | ((details ->> 'puzzle_id'::text)::integ
er)
btree, for table "lys.game_events", predicate (game_code = 'oneuppu
zzle'::text)


Index "lys.game_events_oneuppuzzle_username_idx"
  Column  |  Type  | Key? | Definition
----------+--------+------+------------
 username | citext | yes  | username
btree, for table "lys.game_events", predicate (game_code = 'oneuppu
zzle'::text AND is_flagged = false)


    Index "lys.game_events_pkey"
 Column |  Type  | Key? | Definition
--------+--------+------+------------
 id     | bigint | yes  | id
primary key, btree, for table "lys.game_events"


            Index "lys.idx_channel_events_recent"
   Column    |           Type           | Key? | Definition
-------------+--------------------------+------+-------------
 occurred_at | timestamp with time zone | yes  | occurred_at
btree, for table "lys.twitch_channel_events"


   Index "lys.idx_channels_active"
 Column |  Type   | Key? | Definition
--------+---------+------+------------
 active | boolean | yes  | active
btree, for table "lys.twitch_channels"


                                   Table "lys.letterboxd_feed_subsc
riptions"
       Column        |           Type           | Collation | Nulla
ble |                Default
---------------------+--------------------------+-----------+------
----+---------------------------------------
 id                  | bigint                   |           | not n
ull | nextval('lys.feeds_id_seq'::regclass)
 username            | text                     |           | not n
ull |
 user_id             | text                     |           | not n
ull |
 letterboxd_username | character varying(255)   |           | not n
ull |
 feed_url            | text                     |           | not n
ull |
 last_seen_guid      | text                     |           |
    |
 last_alerted_at     | timestamp with time zone |           |
    |
 created_at          | timestamp with time zone |           |
    | now()
 alert_channels      | text[]                   |           | not n
ull | '{}'::text[]
Indexes:
    "feeds_pkey" PRIMARY KEY, btree (id)
    "feeds_feed_url_key" UNIQUE CONSTRAINT, btree (feed_url)
    "feeds_user_id_letterboxd_username_key" UNIQUE CONSTRAINT, btre
e (user_id, letterboxd_username)


                             Table "lys.sleeper_connections"
      Column       |           Type           | Collation | Nullabl
e |      Default
-------------------+--------------------------+-----------+--------
--+-------------------
 id                | uuid                     |           | not nul
l | gen_random_uuid()
 league_id         | uuid                     |           | not nul
l |
 twitch_user_id    | text                     |           | not nul
l |
 sleeper_user_id   | text                     |           | not nul
l |
 sleeper_username  | text                     |           |
  |
 sleeper_roster_id | integer                  |           | not nul
l |
 created_at        | timestamp with time zone |           | not nul
l | now()
 updated_at        | timestamp with time zone |           | not nul
l | now()
 twitch_username   | text                     |           |
  |
Indexes:
    "sleeper_connections_pkey" PRIMARY KEY, btree (id)
    "sleeper_connections_league_id_sleeper_roster_id_key" UNIQUE CO
NSTRAINT, btree (league_id, sleeper_roster_id)
    "sleeper_connections_league_id_twitch_user_id_key" UNIQUE CONST
RAINT, btree (league_id, twitch_user_id)
    "sleeper_connections_league_twitch_username_idx" btree (league_
id, lower(twitch_username))
Foreign-key constraints:
    "sleeper_connections_league_id_fkey" FOREIGN KEY (league_id) RE
FERENCES lys.sleeper_leagues(id) ON DELETE CASCADE


Index "lys.sleeper_connections_league_id_sleeper_roster_id_key"
      Column       |  Type   | Key? |    Definition
-------------------+---------+------+-------------------
 league_id         | uuid    | yes  | league_id
 sleeper_roster_id | integer | yes  | sleeper_roster_id
unique, btree, for table "lys.sleeper_connections"


Index "lys.sleeper_connections_league_id_twitch_user_id_key"
     Column     | Type | Key? |   Definition
----------------+------+------+----------------
 league_id      | uuid | yes  | league_id
 twitch_user_id | text | yes  | twitch_user_id
unique, btree, for table "lys.sleeper_connections"


Index "lys.sleeper_connections_league_twitch_username_idx"
  Column   | Type | Key? |       Definition
-----------+------+------+------------------------
 league_id | uuid | yes  | league_id
 lower     | text | yes  | lower(twitch_username)
btree, for table "lys.sleeper_connections"


Index "lys.sleeper_connections_pkey"
 Column | Type | Key? | Definition
--------+------+------+------------
 id     | uuid | yes  | id
primary key, btree, for table "lys.sleeper_connections"


                               Table "lys.sleeper_events"
      Column       |           Type           | Collation | Nullabl
e |      Default
-------------------+--------------------------+-----------+--------
--+-------------------
 id                | uuid                     |           | not nul
l | gen_random_uuid()
 sleeper_league_id | text                     |           | not nul
l |
 event_type        | text                     |           | not nul
l |
 external_event_id | text                     |           |
  |
 event_data        | jsonb                    |           |
  |
 occurred_at       | timestamp with time zone |           | not nul
l |
 published_at      | timestamp with time zone |           |
  |
 created_at        | timestamp with time zone |           | not nul
l | now()
Indexes:
    "sleeper_events_pkey" PRIMARY KEY, btree (id)
    "sleeper_events_sleeper_league_id_event_type_external_event__ke
y" UNIQUE CONSTRAINT, btree (sleeper_league_id, event_type, externa
l_event_id)


  Index "lys.sleeper_events_pkey"
 Column | Type | Key? | Definition
--------+------+------+------------
 id     | uuid | yes  | id
primary key, btree, for table "lys.sleeper_events"


Index "lys.sleeper_events_sleeper_league_id_event_type_external_eve
nt__key"
      Column       | Type | Key? |    Definition
-------------------+------+------+-------------------
 sleeper_league_id | text | yes  | sleeper_league_id
 event_type        | text | yes  | event_type
 external_event_id | text | yes  | external_event_id
unique, btree, for table "lys.sleeper_events"


                                Table "lys.sleeper_leagues"
       Column        |           Type           | Collation | Nulla
ble |      Default
---------------------+--------------------------+-----------+------
----+-------------------
 id                  | uuid                     |           | not n
ull | gen_random_uuid()
 twitch_channel_id   | text                     |           | not n
ull |
 sleeper_league_id   | text                     |           | not n
ull |
 league_name         | text                     |           |
    |
 season              | text                     |           |
    |
 enabled             | boolean                  |           | not n
ull | true
 current_week        | integer                  |           |
    |
 last_polled_at      | timestamp with time zone |           |
    |
 last_transaction_at | timestamp with time zone |           |
    |
 last_waiver_week    | integer                  |           |
    |
 last_recap_week     | integer                  |           |
    |
 notifications       | jsonb                    |           | not n
ull | '{}'::jsonb
 created_at          | timestamp with time zone |           | not n
ull | now()
 updated_at          | timestamp with time zone |           | not n
ull | now()
Indexes:
    "sleeper_leagues_pkey" PRIMARY KEY, btree (id)
    "sleeper_leagues_twitch_channel_id_sleeper_league_id_key" UNIQU
E CONSTRAINT, btree (twitch_channel_id, sleeper_league_id)
Referenced by:
    TABLE "lys.sleeper_connections" CONSTRAINT "sleeper_connections
_league_id_fkey" FOREIGN KEY (league_id) REFERENCES lys.sleeper_lea
gues(id) ON DELETE CASCADE


 Index "lys.sleeper_leagues_pkey"
 Column | Type | Key? | Definition
--------+------+------+------------
 id     | uuid | yes  | id
primary key, btree, for table "lys.sleeper_leagues"


Index "lys.sleeper_leagues_twitch_channel_id_sleeper_league_id_key"
      Column       | Type | Key? |    Definition
-------------------+------+------+-------------------
 twitch_channel_id | text | yes  | twitch_channel_id
 sleeper_league_id | text | yes  | sleeper_league_id
unique, btree, for table "lys.sleeper_leagues"


                         Table "lys.twitch_channel_events"
   Column    |           Type           | Collation | Nullable |
   Default
-------------+--------------------------+-----------+----------+---
----------------
 event_id    | uuid                     |           | not null | ge
n_random_uuid()
 user_id     | text                     |           | not null |
 username    | citext                   |           | not null |
 action      | lys.channel_action       |           | not null |
 occurred_at | timestamp with time zone |           | not null | no
w()
Indexes:
    "channel_events_pkey" PRIMARY KEY, btree (event_id)
    "idx_channel_events_recent" btree (occurred_at DESC)
Foreign-key constraints:
    "twitch_channel_events_user_id_fkey" FOREIGN KEY (user_id) REFE
RENCES lys.twitch_channels(user_id) ON DELETE CASCADE


                        Table "lys.twitch_channels"
    Column     |           Type           | Collation | Nullable |
Default
---------------+--------------------------+-----------+----------+-
--------
 user_id       | text                     |           | not null |
 username      | citext                   |           | not null |
 access_token  | text                     |           | not null |
 refresh_token | text                     |           | not null |
 date_joined   | timestamp with time zone |           | not null |
now()
 last_joined   | timestamp with time zone |           | not null |
now()
 active        | boolean                  |           | not null |
true
 times_joined  | integer                  |           | not null |
1
 banned        | boolean                  |           | not null |
false
Indexes:
    "channels_pkey" PRIMARY KEY, btree (user_id)
    "channels_username_key" UNIQUE CONSTRAINT, btree (username)
    "idx_channels_active" btree (active)
Referenced by:
    TABLE "lys.twitch_channel_events" CONSTRAINT "twitch_channel_ev
ents_user_id_fkey" FOREIGN KEY (user_id) REFERENCES lys.twitch_chan
nels(user_id) ON DELETE CASCADE
    TABLE "lys.twitch_settings" CONSTRAINT "twitch_settings_user_id
_fkey" FOREIGN KEY (user_id) REFERENCES lys.twitch_channels(user_id
) ON DELETE CASCADE
Triggers:
    twitch_channels_after_insert AFTER INSERT ON lys.twitch_channel
s FOR EACH ROW EXECUTE FUNCTION lys.create_twitch_settings_entry()
    twitch_channels_after_insert_log AFTER INSERT ON lys.twitch_cha
nnels FOR EACH ROW EXECUTE FUNCTION lys.log_twitch_channel_changes(
)
    twitch_channels_after_update_log AFTER UPDATE OF active, banned
 ON lys.twitch_channels FOR EACH ROW WHEN (old.active IS DISTINCT F
ROM new.active OR old.banned IS DISTINCT FROM new.banned) EXECUTE F
UNCTION lys.log_twitch_channel_changes()


                           Table "lys.twitch_settings"
     Column     |            Type             | Collation | Nullabl
e |  Default
----------------+-----------------------------+-----------+--------
--+-----------
 user_id        | text                        |           | not nul
l |
 command_prefix | text                        |           | not nul
l | '!'::text
 date_created   | timestamp without time zone |           |
  | now()
 last_modified  | timestamp without time zone |           |
  | now()
Indexes:
    "twitch_settings_pkey" PRIMARY KEY, btree (user_id)
Foreign-key constraints:
    "twitch_settings_user_id_fkey" FOREIGN KEY (user_id) REFERENCES
 lys.twitch_channels(user_id) ON DELETE CASCADE


  Index "lys.twitch_settings_pkey"
 Column  | Type | Key? | Definition
---------+------+------+------------
 user_id | text | yes  | user_id
primary key, btree, for table "lys.twitch_settings"
