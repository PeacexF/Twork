package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/PeacexF/Twork/internal/models"
)

type Store struct {
	db *sql.DB
}

// opens the database and applies schema migrations
func Open(path string) (*Store, error) {

	dsn := fmt.Sprintf("file:%s?_fk=1&_journal=WAL&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite at %q: %w", path, err)
	}

	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating schema: %w", err)
	}
	return s, nil
}

// closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

const schema = `
CREATE TABLE IF NOT EXISTS chats (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	telegram_id   INTEGER NOT NULL UNIQUE,
	access_hash   INTEGER NOT NULL DEFAULT 0,
	kind          TEXT NOT NULL,
	title         TEXT NOT NULL DEFAULT '',
	username      TEXT NOT NULL DEFAULT '',
	tag           TEXT NOT NULL DEFAULT '',
	paused        INTEGER NOT NULL DEFAULT 0,
	created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	telegram_message_id  INTEGER NOT NULL,
	chat_id              INTEGER NOT NULL,
	chat_title           TEXT NOT NULL DEFAULT '',
	sender_id            INTEGER NOT NULL DEFAULT 0,
	sender_name          TEXT NOT NULL DEFAULT '',
	ts                   DATETIME NOT NULL,
	text                 TEXT NOT NULL DEFAULT '',
	link                 TEXT NOT NULL DEFAULT '',
	forward_from_title   TEXT NOT NULL DEFAULT '',
	edit_ts              DATETIME,
	norm_hash            TEXT NOT NULL DEFAULT '',
	created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(chat_id, telegram_message_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(ts);

-- Global dedup: a message is a duplicate if its normalized-text hash
-- matches one already indexed, regardless of which channel it came from.
CREATE INDEX IF NOT EXISTS idx_messages_norm_hash ON messages(norm_hash);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
	text,
	content='messages',
	content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
	INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
	INSERT INTO messages_fts(messages_fts, rowid, text) VALUES ('delete', old.id, old.text);
	INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TABLE IF NOT EXISTS matches (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	message_id       INTEGER NOT NULL UNIQUE REFERENCES messages(id) ON DELETE CASCADE,
	matched_keywords TEXT NOT NULL, -- JSON array, kept simple/explainable per PLAN.md section 4
	created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS message_status (
	message_id  INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
	bookmarked  INTEGER NOT NULL DEFAULT 0,
	saved       INTEGER NOT NULL DEFAULT 0,
	ignored     INTEGER NOT NULL DEFAULT 0,
	archived    INTEGER NOT NULL DEFAULT 0,
	is_read     INTEGER NOT NULL DEFAULT 0
);
`

// applies the schema
func (s *Store) migrate() error {
	// The ALTER must run before the schema block: the schema creates an
	// index on norm_hash, which fails if a pre-existing messages table
	// doesn't have that column yet.
	if err := s.migrateNormHash(); err != nil {
		return err
	}
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	_, err := s.db.Exec(settingsSchema)
	return err
}

// adds norm_hash to a pre-existing messages table; no-ops when the table doesn't exist yet or already has the column
func (s *Store) migrateNormHash() error {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='messages'`).Scan(&name)
	if err == sql.ErrNoRows {
		return nil // fresh database; the schema block creates the column
	}
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE messages ADD COLUMN norm_hash TEXT NOT NULL DEFAULT ''`); err != nil {
		if strings.Contains(err.Error(), "duplicate column") {
			return nil
		}
		return err
	}
	return nil
}

// returns the highest indexed message ID for a chat
func (s *Store) MaxTelegramMessageID(ctx context.Context, telegramChatID int64) (int, error) {
	var max sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(telegram_message_id) FROM messages WHERE chat_id = ?
	`, telegramChatID).Scan(&max)
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64), nil
}

// ErrDuplicate is returned by InsertMessage when a message with the
// same normalized text has already been indexed (global dedup).
var ErrDuplicate = fmt.Errorf("duplicate message")

// checks whether a message's normalized text has already been indexed in any channel
func (s *Store) IsDuplicate(ctx context.Context, text string) (bool, error) {
	hash := dedupHash(text)
	if hash == "" {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM messages WHERE norm_hash = ?`, hash).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// inserts a message, skipping global-duplicate text (ErrDuplicate) and idempotent on (chat, telegram message id)
func (s *Store) InsertMessage(ctx context.Context, m models.Message) (int64, error) {
	hash := dedupHash(m.Text)
	if hash != "" {
		var existing int64
		err := s.db.QueryRowContext(ctx, `SELECT id FROM messages WHERE norm_hash = ? LIMIT 1`, hash).Scan(&existing)
		if err == nil {
			return 0, ErrDuplicate
		}
		if err != sql.ErrNoRows {
			return 0, fmt.Errorf("checking duplicate: %w", err)
		}
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (telegram_message_id, chat_id, chat_title, sender_id, sender_name, ts, text, link, forward_from_title, edit_ts, norm_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id, telegram_message_id) DO NOTHING
	`, m.TelegramMessageID, m.ChatID, m.ChatTitle, m.SenderID, m.SenderName, m.Timestamp, m.Text, m.Link, m.ForwardFromTitle, m.EditTimestamp, hash)
	if err != nil {
		return 0, fmt.Errorf("inserting message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id != 0 {
		return id, nil
	}

	var existing int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM messages WHERE chat_id = ? AND telegram_message_id = ?`, m.ChatID, m.TelegramMessageID).Scan(&existing)
	if err != nil {
		return 0, fmt.Errorf("looking up existing message after conflict: %w", err)
	}
	return existing, nil
}

// stores or updates a message's match record
func (s *Store) RecordMatch(ctx context.Context, messageID int64, keywordsJSON string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO matches (message_id, matched_keywords) VALUES (?, ?)
		ON CONFLICT(message_id) DO UPDATE SET matched_keywords = excluded.matched_keywords
	`, messageID, keywordsJSON)
	return err
}

type SearchResult struct {
	MessageID int64
	ChatTitle string
	Text      string
	Link      string
	Timestamp time.Time
}

// runs an FTS5 query
func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.chat_title, m.text, m.link, m.ts
		FROM messages_fts f
		JOIN messages m ON m.id = f.rowid
		WHERE messages_fts MATCH ?
		ORDER BY m.ts DESC
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching: %w", err)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.MessageID, &r.ChatTitle, &r.Text, &r.Link, &r.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// inserts or updates a chat's metadata
func (s *Store) UpsertChat(ctx context.Context, c models.Chat) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chats (telegram_id, access_hash, kind, title, username, tag, paused)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(telegram_id) DO UPDATE SET
			access_hash = excluded.access_hash,
			kind = excluded.kind,
			title = excluded.title,
			username = excluded.username,
			tag = excluded.tag,
			paused = excluded.paused
	`, c.TelegramID, c.AccessHash, string(c.Kind), c.Title, c.Username, c.Tag, boolToInt(c.Paused))
	return err
}

type Stats struct {
	ChatsMonitored  int
	MessagesIndexed int
	TodayMatches    int
	Bookmarks       int
	Ignored         int
}

// computes the dashboard counters
func (s *Store) GetStats(ctx context.Context) (Stats, error) {
	var st Stats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM chats WHERE paused = 0`).Scan(&st.ChatsMonitored); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM messages`).Scan(&st.MessagesIndexed); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM matches WHERE created_at >= datetime('now', 'start of day')
	`).Scan(&st.TodayMatches); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM message_status WHERE bookmarked = 1`).Scan(&st.Bookmarks); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM message_status WHERE ignored = 1`).Scan(&st.Ignored); err != nil {
		return st, err
	}
	return st, nil
}

// converts bool to 0/1
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
