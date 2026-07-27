package storage

import (
	"context"
	"testing"

	"github.com/PeacexF/Twork/internal/models"
)

// a sample chat fixture
func sampleChat(id int64, title string) models.Chat {
	return models.Chat{
		TelegramID: id,
		AccessHash: id * 7,
		Kind:       models.ChatKindChannel,
		Title:      title,
		Username:   title,
		Tag:        "Backend",
	}
}

// UpsertChat inserts once, then updates in place on the same telegram_id
func TestUpsertChat_InsertsThenUpdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := sampleChat(100, "golang_jobs")
	if err := s.UpsertChat(ctx, c); err != nil {
		t.Fatalf("first UpsertChat: %v", err)
	}

	c.Title = "Golang Jobs (renamed)"
	c.Tag = "Go"
	c.Paused = true
	c.Kind = models.ChatKindGroup
	if err := s.UpsertChat(ctx, c); err != nil {
		t.Fatalf("second UpsertChat: %v", err)
	}

	chats, err := s.ListChats(ctx)
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("expected the upsert to update in place, got %d rows", len(chats))
	}
	got := chats[0]
	if got.Title != "Golang Jobs (renamed)" || got.Tag != "Go" || !got.Paused || got.Kind != models.ChatKindGroup {
		t.Errorf("updated fields not persisted: %+v", got)
	}
}

// GetChatByTelegramID returns the stored row, or nil (not an error) when unmonitored
func TestGetChatByTelegramID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, sampleChat(200, "remote_jobs")); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	got, err := s.GetChatByTelegramID(ctx, 200)
	if err != nil {
		t.Fatalf("GetChatByTelegramID: %v", err)
	}
	if got == nil {
		t.Fatal("expected the stored chat, got nil")
	}
	if got.Title != "remote_jobs" || got.AccessHash != 1400 || got.Kind != models.ChatKindChannel {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Paused {
		t.Error("expected a freshly inserted chat to be unpaused")
	}

	missing, err := s.GetChatByTelegramID(ctx, 999)
	if err != nil {
		t.Fatalf("expected no error for an unmonitored chat, got %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for an unmonitored chat, got %+v", missing)
	}
}

// pause/resume and tag edits persist
func TestSetChatPausedAndTag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, sampleChat(300, "devops_jobs")); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	if err := s.SetChatPaused(ctx, 300, true); err != nil {
		t.Fatalf("SetChatPaused(true): %v", err)
	}
	c, _ := s.GetChatByTelegramID(ctx, 300)
	if !c.Paused {
		t.Error("expected the chat to be paused")
	}

	if err := s.SetChatPaused(ctx, 300, false); err != nil {
		t.Fatalf("SetChatPaused(false): %v", err)
	}
	c, _ = s.GetChatByTelegramID(ctx, 300)
	if c.Paused {
		t.Error("expected the chat to be resumed")
	}

	if err := s.SetChatTag(ctx, 300, "DevOps"); err != nil {
		t.Fatalf("SetChatTag: %v", err)
	}
	c, _ = s.GetChatByTelegramID(ctx, 300)
	if c.Tag != "DevOps" {
		t.Errorf("tag = %q, want DevOps", c.Tag)
	}
}

// updating a chat that isn't monitored is a silent no-op, not an error
func TestSetChatPausedAndTag_UnknownChatIsNoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetChatPaused(ctx, 424242, true); err != nil {
		t.Errorf("SetChatPaused on an unknown chat: %v", err)
	}
	if err := s.SetChatTag(ctx, 424242, "Nope"); err != nil {
		t.Errorf("SetChatTag on an unknown chat: %v", err)
	}
	if err := s.RemoveChat(ctx, 424242); err != nil {
		t.Errorf("RemoveChat on an unknown chat: %v", err)
	}
}

// removing a chat stops monitoring it but keeps its indexed messages
func TestRemoveChat_KeepsIndexedMessages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, sampleChat(1001, "golang_jobs")); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if _, err := s.InsertMessage(ctx, sampleMessage()); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	if err := s.RemoveChat(ctx, 1001); err != nil {
		t.Fatalf("RemoveChat: %v", err)
	}

	chats, err := s.ListChats(ctx)
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 0 {
		t.Errorf("expected the chat to be gone, got %+v", chats)
	}

	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.MessagesIndexed != 1 {
		t.Errorf("expected indexed history to survive removal, got %d messages", stats.MessagesIndexed)
	}
}

// ListChats returns an empty result (not an error) on a fresh database
func TestListChats_Empty(t *testing.T) {
	s := newTestStore(t)
	chats, err := s.ListChats(context.Background())
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 0 {
		t.Errorf("expected no chats on a fresh db, got %+v", chats)
	}
}
