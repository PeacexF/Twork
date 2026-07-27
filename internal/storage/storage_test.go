package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/models"
)

// opens a temp-file store for a test
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// a reusable sample message fixture
func sampleMessage() models.Message {
	return models.Message{
		TelegramMessageID: 42,
		ChatID:            1001,
		ChatTitle:         "Golang Jobs",
		SenderID:          555,
		SenderName:        "recruiter",
		Timestamp:         time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Text:              "Go Backend Developer needed, Docker and PostgreSQL required",
		Link:              "https://t.me/golang_jobs/42",
	}
}

// re-inserting the identical message is rejected as a duplicate
func TestInsertMessage_IsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	msg := sampleMessage()

	if _, err := s.InsertMessage(ctx, msg); err != nil {
		t.Fatalf("first insert error = %v", err)
	}
	if _, err := s.InsertMessage(ctx, msg); err != ErrDuplicate {
		t.Fatalf("expected ErrDuplicate on re-inserting identical message, got %v", err)
	}
}

// duplicate detection normalizes text: whitespace-only differences still count as duplicates
func TestInsertMessage_DedupUsesNormalizedText(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	msg := sampleMessage()

	if _, err := s.InsertMessage(ctx, msg); err != nil {
		t.Fatalf("first insert error = %v", err)
	}

	// Trailing whitespace is normalized away, so this IS a duplicate even
	// though the raw text and the (chat, message id) pair both differ.
	variant := sampleMessage()
	variant.ChatID = msg.ChatID + 1
	variant.TelegramMessageID = msg.TelegramMessageID + 1
	variant.Text = msg.Text + "   \n"
	if _, err := s.InsertMessage(ctx, variant); err != ErrDuplicate {
		t.Fatalf("expected whitespace-only variation to be rejected after normalization, got %v", err)
	}
}

// FTS5 search finds an inserted message and ignores absent terms
func TestSearch_FTS5_FindsInsertedMessage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	msg := sampleMessage()

	if _, err := s.InsertMessage(ctx, msg); err != nil {
		t.Fatalf("insert error = %v", err)
	}

	results, total, err := s.SearchPaged(ctx, "Docker AND PostgreSQL", 10, 0)
	if err != nil {
		t.Fatalf("SearchPaged error = %v", err)
	}
	if len(results) != 1 || total != 1 {
		t.Fatalf("expected 1 result (total 1), got %d (total %d)", len(results), total)
	}
	if results[0].ChatTitle != "Golang Jobs" {
		t.Fatalf("expected chat title to be preserved, got %q", results[0].ChatTitle)
	}

	results, total, err = s.SearchPaged(ctx, "Kubernetes", 10, 0)
	if err != nil {
		t.Fatalf("SearchPaged error = %v", err)
	}
	if len(results) != 0 || total != 0 {
		t.Fatalf("expected 0 results for absent term, got %d (total %d)", len(results), total)
	}
}

// recording a match updates the dashboard stats
func TestRecordMatchAndStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	msg := sampleMessage()

	id, err := s.InsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("insert error = %v", err)
	}
	if err := s.RecordMatch(ctx, id, `["Go","Docker","PostgreSQL"]`); err != nil {
		t.Fatalf("RecordMatch error = %v", err)
	}

	if err := s.UpsertChat(ctx, models.Chat{TelegramID: 1001, Kind: models.ChatKindChannel, Title: "Golang Jobs"}); err != nil {
		t.Fatalf("UpsertChat error = %v", err)
	}

	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats error = %v", err)
	}
	if stats.MessagesIndexed != 1 {
		t.Fatalf("expected 1 indexed message, got %d", stats.MessagesIndexed)
	}
	if stats.ChatsMonitored != 1 {
		t.Fatalf("expected 1 monitored chat, got %d", stats.ChatsMonitored)
	}
}

// a cross-channel repost of the same text is rejected as a global duplicate
func TestInsertMessage_GlobalDedup(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m1 := sampleMessage()
	if _, err := s.InsertMessage(ctx, m1); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Same text, DIFFERENT channel and telegram id -> should be a duplicate.
	m2 := sampleMessage()
	m2.ChatID = m1.ChatID + 999
	m2.TelegramMessageID = m1.TelegramMessageID + 999
	m2.ChatTitle = "Some Other Channel"
	_, err := s.InsertMessage(ctx, m2)
	if err != ErrDuplicate {
		t.Fatalf("expected ErrDuplicate for cross-channel repost, got %v", err)
	}
}

// opening a database that predates the norm_hash column adds it without error
func TestMigrate_AddsNormHashToLegacyDB(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/legacy.db"

	// Create a messages table WITHOUT norm_hash, simulating an old install.
	raw, err := openRawLegacy(path)
	if err != nil {
		t.Fatalf("creating legacy db: %v", err)
	}
	raw.Close()

	// Opening through the normal path should migrate it in.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open on legacy db failed to migrate: %v", err)
	}
	defer s.Close()

	// A normal insert (which writes norm_hash) must now succeed.
	if _, err := s.InsertMessage(context.Background(), sampleMessage()); err != nil {
		t.Fatalf("insert after migration failed: %v", err)
	}
}

// creates a messages table lacking norm_hash, to simulate a pre-migration database
func openRawLegacy(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		telegram_message_id INTEGER NOT NULL,
		chat_id INTEGER NOT NULL,
		chat_title TEXT NOT NULL DEFAULT '',
		sender_id INTEGER NOT NULL DEFAULT 0,
		sender_name TEXT NOT NULL DEFAULT '',
		ts DATETIME NOT NULL,
		text TEXT NOT NULL DEFAULT '',
		link TEXT NOT NULL DEFAULT '',
		forward_from_title TEXT NOT NULL DEFAULT '',
		edit_ts DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(chat_id, telegram_message_id)
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
