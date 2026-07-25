# Architecture

## Overview

Twork is split into two independent Telegram connections that never talk to Telegram on each other's behalf:

- **The MTProto account** (`internal/collector`) — your own Telegram user account, authenticated once via `my.telegram.org` credentials. It reads channels and groups, including private ones you're a member of, and can resolve/read public channels without joining them.
- **The Bot** (`internal/bot`) — a separate Telegram Bot (its own `@BotFather` token). This is what you actually talk to: menus, search, favorites, settings, notifications.

They're connected only through shared Go objects in `cmd/twork/main.go` — `*storage.Store` (SQLite) and `*matcher.Store` (the live keyword matcher) — plus the bot holding a reference to the collector so it can trigger chat add/pause/resume/remove.

Keeping them separate means the account that reads your channels never runs bot-command logic, and the bot never touches the sensitive MTProto session file.

## Data flow

```
Telegram (MTProto)                          Telegram (Bot API)
      |                                              |
      v                                              v
 collector.Collector                            bot.Bot
  - auth / session                          - dispatch loop
  - resolve chats                           - single "home" message,
  - backfill history                          edited in place per screen
  - live updates                             - free-text prompts for
      |                                        add-chat / search / keywords
      v                                              |
   Handler func(msg, live)  <----------- notifies on live matches ---+
      |                                                              |
      v                                                              |
 storage.Store (SQLite + FTS5)  -------------------------------------+
  - messages, chats, matches, settings
      |
      v
 matcher.Store (hot-swappable keyword matcher)
```

Every message the collector produces (backfilled or live) goes through one callback in `main.go`:

1. Check for an exact-text duplicate (`Store.IsDuplicate`) -- logged, not blocked.
2. Insert the message (`Store.InsertMessage`) -- idempotent on `(chat_id, telegram_message_id)`, so replaying history is always safe.
3. Run it through the current matcher (`matcher.Store.Get().Match`).
4. If it matched, record the match (`Store.RecordMatch`) and, only if the message arrived live (not during backfill), ask the bot to send a notification.

## Package responsibilities

| Package | Responsibility |
|---|---|
| `internal/config` | Parses and validates `config.yaml`. Only credentials and first-run seed data live here -- everything else the bot can change lives in the database. |
| `internal/models` | Shared structs (`Message`, `Chat`, `MatchResult`) with no behavior of their own. |
| `internal/matcher` | Deterministic keyword matching: no AI, no scoring. A message matches if it contains at least one positive keyword and no negative keyword. Wrapped in a `Store` so keyword edits from the bot apply without a restart. |
| `internal/storage` | SQLite persistence: message indexing, FTS5 full-text search, chats, bookmarks, and a key/value `settings` table for anything the bot can edit at runtime (keywords, notification toggle, claimed owner ID). |
| `internal/collector` | The MTProto side: login, chat resolution, incremental history backfill, live updates, and the dynamic add/pause/resume/remove operations the bot calls into. |
| `internal/bot` | The Bot API side: every menu, the single-message navigation pattern, search, favorites, keyword/settings editing, `.md` export, and live-match notifications. |

## Storage as the source of truth

`config.yaml`'s `chats`, `matching`, and `notifications` sections are **seed data only**. On first run, if the database has nothing yet, they're copied into SQLite; from then on the database is authoritative and `config.yaml`'s values are ignored. This is why the bot can add/remove chats and edit keywords without ever touching a config file or requiring a restart.

Only `telegram.*` (MTProto credentials) and `bot.*` (bot token, owner ID) stay purely in `config.yaml` -- those are secrets, not editable state.

## Pluggable chat source

The bot never depends on the MTProto collector directly -- it depends on a small `ChatSource` interface (`internal/bot/bot.go`): `Run`, `AddByUsername`, `AddByInviteLink`, `AddFolder`, `Pause`, `Resume`, `Remove`, `ListResolved`. `config.yaml`'s `source.kind` picks which implementation `cmd/twork/main.go` wires in:

- `mtproto` (default) -- `internal/collector`, described above.
- `rsshub` -- `internal/rsshub`, which polls a self-hosted [RSSHub](https://docs.rsshub.app/) instance's `telegram/channel/:channel` route instead of logging into Telegram at all. No account, no session file, no risk of anything happening to a real Telegram account -- but real tradeoffs come with it:
  - Only public channels/groups with a `@username` are reachable (RSSHub scrapes `t.me/s/<username>` preview pages); `AddByInviteLink`/`AddFolder` return a clear "not supported" error rather than silently failing.
  - RSSHub's feed only returns a recent window of posts, not full history, so there's no one-time deep backfill -- just a polling loop (`rsshub.poll_interval_seconds` in config). The very first poll of a newly added chat is still treated as a quiet backfill (no notification spam for posts that were already sitting in the feed).
  - Telegram message IDs don't exist in an RSS feed. Chats and messages get a stable synthetic ID instead, derived by hashing the username (for chats, always negative, so it can never collide with a real MTProto channel ID) or the entry's GUID/link (for messages) -- see `syntheticChatID`/`syntheticMessageID` in `internal/rsshub/source.go`. Re-ingesting the same item on every poll is still a no-op, because storage's `(chat_id, telegram_message_id)` uniqueness constraint doesn't care whether the ID came from Telegram or a hash.

## Why a separate MTProto client and Bot

A Telegram Bot (Bot API) cannot read the history of a channel or group it hasn't been added to as a member with the right permissions, and many job channels won't (and shouldn't need to) add a bot as an admin. A regular user account can read any public channel's history, and any private one it's already joined -- which is the entire point of "monitor without manual checking." The Bot API, on the other hand, is by far the simpler and more robust way to build an interactive menu (inline keyboards, callback queries, editing messages), so it's used only for that.

## Adding chats: three paths, three levels of consequence

- **By username** (`AddByUsername`) -- resolves via `contacts.resolveUsername`. No join happens; Telegram allows reading a public channel's recent history without being a member.
- **By invite link** (`AddByInviteLink`) -- private chats have no way to read history without membership, so this performs a real join via `messages.importChatInvite`.
- **By folder link** (`AddFolder`) -- same as invite link, multiplied across every chat in a shared `t.me/addlist/...` folder via `chatlists.joinChatlistInvite`.

Twork never leaves or removes the account from a chat automatically -- "Remove" in the bot only stops monitoring, since joining was already a deliberate, visible action and leaving should be too.

## The "no spam" UI pattern

Every menu screen in the bot is the same Telegram message, rewritten in place (`EditMessageText`/`EditMessageReplyMarkup`) as you navigate -- this is `session.homeMsgID` in `internal/bot/session.go`. The only messages sent fresh are:

- **Prompts** (add-chat, search query, add-keyword) -- deleted, along with your reply, the instant they're handled (`clearPrompt`).
- **Live-match notifications** -- standalone alerts with Save/Open/Dismiss, since they can arrive at any time independent of whatever menu is open.
- **`.md` exports** -- real files you're meant to keep.

## The Matches/Favorites/Search carousel

Rather than a paginated list of summaries, `internal/bot/carousel.go` shows one full post at a time with a prev / position / next row. All three views (Matches, Favorites, Search) share this renderer; only the underlying SQL query differs (`ListMatches`, `ListBookmarked`, `SearchPaged`). Exporting to `.md` pulls every row in the current view, not just the one on screen.

## Known limitations / deliberate tradeoffs

- **Single owner.** The bot only responds to one Telegram user ID (the first person to send `/start`, or `bot.owner_id` in config). This isn't a multi-tenant service.
- **No PTS gap recovery.** Live updates use `telegram.Options.UpdateHandler` directly rather than `gotd/td`'s `telegram/updates.Manager`. A dropped connection could in theory miss an update; the next backfill (on restart, or when a chat is resumed) catches up on anything missed.
- **CGO-based SQLite driver.** `mattn/go-sqlite3` needs a C toolchain at build time and the `sqlite_fts5` build tag.
- **Exact-text duplicate detection only.** By design (see `PLAN.md` section 7.9) -- no fuzzy matching, no similarity scoring, so "why was this flagged a duplicate" is always answerable with "it's byte-for-byte identical to another indexed message."