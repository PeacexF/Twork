package storage

import (
	"context"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/models"
)

// enabling resume broadcasting on a group succeeds and round-trips
func TestSetChatResumeConfig_Group(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Go Jobs"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.SetChatResumeConfig(ctx, 1, true, 120, "my resume"); err != nil {
		t.Fatalf("SetChatResumeConfig: %v", err)
	}

	got, err := s.GetChatByTelegramID(ctx, 1)
	if err != nil || got == nil {
		t.Fatalf("GetChatByTelegramID: %+v, %v", got, err)
	}
	if !got.ResumeEnabled || got.ResumeIntervalSeconds != 120 || got.ResumeText != "my resume" {
		t.Errorf("chat = %+v", got)
	}
}

// enabling resume broadcasting on a channel is refused, since a regular
// member usually can't post there, and an admin "sending" would blast every subscriber
func TestSetChatResumeConfig_RefusesChannel(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindChannel, Title: "Vacancies"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.SetChatResumeConfig(ctx, 1, true, 120, "my resume"); err == nil {
		t.Fatal("expected an error enabling resume broadcasting on a channel, got nil")
	}

	got, err := s.GetChatByTelegramID(ctx, 1)
	if err != nil || got == nil {
		t.Fatalf("GetChatByTelegramID: %+v, %v", got, err)
	}
	if got.ResumeEnabled {
		t.Error("expected resume_enabled to remain false after a rejected enable")
	}
}

// disabling never requires the group check, regardless of chat kind
func TestSetChatResumeConfig_DisableAlwaysAllowed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindChannel, Title: "Vacancies"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.SetChatResumeConfig(ctx, 1, false, 0, ""); err != nil {
		t.Fatalf("SetChatResumeConfig(disable): %v", err)
	}
}

// SetChatResumeConfig on an unmonitored chat is an error, not a silent no-op
func TestSetChatResumeConfig_UnknownChat(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetChatResumeConfig(context.Background(), 999, true, 60, "text"); err == nil {
		t.Fatal("expected an error for an unmonitored chat, got nil")
	}
}

// ListResumeEnabledChats only returns enabled chats, and correctly parses
// LastSentAt after a recorded send (regression coverage for the computed
// subquery column not auto-converting to time.Time the way a real DATETIME
// column does)
func TestListResumeEnabledChats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Enabled"}); err != nil {
		t.Fatalf("UpsertChat 1: %v", err)
	}
	if err := s.UpsertChat(ctx, models.Chat{TelegramID: 2, Kind: models.ChatKindGroup, Title: "Disabled"}); err != nil {
		t.Fatalf("UpsertChat 2: %v", err)
	}
	if err := s.SetChatResumeConfig(ctx, 1, true, 60, "text"); err != nil {
		t.Fatalf("SetChatResumeConfig: %v", err)
	}

	list, err := s.ListResumeEnabledChats(ctx)
	if err != nil {
		t.Fatalf("ListResumeEnabledChats: %v", err)
	}
	if len(list) != 1 || list[0].TelegramID != 1 {
		t.Fatalf("list = %+v", list)
	}
	if list[0].LastSentAt != nil {
		t.Errorf("expected LastSentAt to be nil before any send, got %v", list[0].LastSentAt)
	}

	if err := s.RecordResumeSend(ctx, 1); err != nil {
		t.Fatalf("RecordResumeSend: %v", err)
	}
	list, err = s.ListResumeEnabledChats(ctx)
	if err != nil {
		t.Fatalf("ListResumeEnabledChats (after send): %v", err)
	}
	if len(list) != 1 || list[0].LastSentAt == nil {
		t.Fatalf("expected LastSentAt to be populated after RecordResumeSend, list = %+v", list)
	}
	if time.Since(*list[0].LastSentAt) > time.Minute {
		t.Errorf("LastSentAt = %v, expected close to now", *list[0].LastSentAt)
	}
}

// CountResumeSendsSince only counts sends at or after the cutoff, across all chats
func TestCountResumeSendsSince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.RecordResumeSend(ctx, 1); err != nil {
		t.Fatalf("RecordResumeSend: %v", err)
	}
	if err := s.RecordResumeSend(ctx, 2); err != nil {
		t.Fatalf("RecordResumeSend: %v", err)
	}

	n, err := s.CountResumeSendsSince(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountResumeSendsSince: %v", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}

	n, err = s.CountResumeSendsSince(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CountResumeSendsSince (future cutoff): %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0 for a cutoff in the future", n)
	}
}

// the global resume text round-trips through settings
func TestResumeGlobalText_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.GetResumeGlobalText(ctx)
	if err != nil || got != "" {
		t.Fatalf("GetResumeGlobalText (unset) = %q, %v", got, err)
	}

	if err := s.SetResumeGlobalText(ctx, "Experienced Go developer..."); err != nil {
		t.Fatalf("SetResumeGlobalText: %v", err)
	}
	got, err = s.GetResumeGlobalText(ctx)
	if err != nil || got != "Experienced Go developer..." {
		t.Fatalf("GetResumeGlobalText = %q, %v", got, err)
	}
}

// the compliance limits default until explicitly set, then round-trip
func TestResumeComplianceSettings_DefaultsAndRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if v, err := s.GetResumeMinDelaySeconds(ctx, 300); err != nil || v != 300 {
		t.Fatalf("GetResumeMinDelaySeconds (unset) = %d, %v", v, err)
	}
	if v, err := s.GetResumeMaxPerHour(ctx, 10); err != nil || v != 10 {
		t.Fatalf("GetResumeMaxPerHour (unset) = %d, %v", v, err)
	}

	if err := s.SetResumeMinDelaySeconds(ctx, 600); err != nil {
		t.Fatalf("SetResumeMinDelaySeconds: %v", err)
	}
	if err := s.SetResumeMaxPerHour(ctx, 5); err != nil {
		t.Fatalf("SetResumeMaxPerHour: %v", err)
	}

	if v, err := s.GetResumeMinDelaySeconds(ctx, 300); err != nil || v != 600 {
		t.Fatalf("GetResumeMinDelaySeconds = %d, %v", v, err)
	}
	if v, err := s.GetResumeMaxPerHour(ctx, 10); err != nil || v != 5 {
		t.Fatalf("GetResumeMaxPerHour = %d, %v", v, err)
	}
}

// a fresh DB gets the new chats columns and the resume_sends table directly
// from the schema block (no ALTER TABLE involved)
func TestMigrate_FreshDBHasResumeSchema(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Fresh"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.SetChatResumeConfig(ctx, 1, true, 60, "text"); err != nil {
		t.Fatalf("SetChatResumeConfig: %v", err)
	}
	if err := s.RecordResumeSend(ctx, 1); err != nil {
		t.Fatalf("RecordResumeSend: %v", err)
	}
}
