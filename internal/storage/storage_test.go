package storage

import (
	"context"
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

// re-inserting the same Telegram message returns the same row id
func TestInsertMessage_IsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	msg := sampleMessage()

	id1, err := s.InsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("first insert error = %v", err)
	}
	id2, err := s.InsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("second insert error = %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected re-inserting the same telegram message to return the same row id, got %d and %d", id1, id2)
	}
}

// duplicate detection is exact-text only, no fuzzy matching
func TestIsDuplicate_ExactTextOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	msg := sampleMessage()

	dup, err := s.IsDuplicate(ctx, msg.Text)
	if err != nil {
		t.Fatalf("IsDuplicate error = %v", err)
	}
	if dup {
		t.Fatalf("expected not a duplicate before insert")
	}

	if _, err := s.InsertMessage(ctx, msg); err != nil {
		t.Fatalf("insert error = %v", err)
	}

	dup, err = s.IsDuplicate(ctx, msg.Text)
	if err != nil {
		t.Fatalf("IsDuplicate error = %v", err)
	}
	if !dup {
		t.Fatalf("expected exact text match to be reported as duplicate")
	}

	dup, err = s.IsDuplicate(ctx, msg.Text+" ")
	if err != nil {
		t.Fatalf("IsDuplicate error = %v", err)
	}
	if dup {
		t.Fatalf("expected near-identical (but not exact) text to NOT be flagged as duplicate")
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

	results, err := s.Search(ctx, "Docker AND PostgreSQL", 10)
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ChatTitle != "Golang Jobs" {
		t.Fatalf("expected chat title to be preserved, got %q", results[0].ChatTitle)
	}

	results, err = s.Search(ctx, "Kubernetes", 10)
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for absent term, got %d", len(results))
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
