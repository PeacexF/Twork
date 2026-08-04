package broadcaster

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

// records every SendText call and can be told to fail for a given chat
type fakeSender struct {
	calls []sentCall
	fail  map[int64]bool
}

type sentCall struct {
	chatID int64
	text   string
}

func (f *fakeSender) SendText(ctx context.Context, chatID int64, text string) error {
	if f.fail[chatID] {
		return fmt.Errorf("boom")
	}
	f.calls = append(f.calls, sentCall{chatID, text})
	return nil
}

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func addGroup(t *testing.T, s *storage.Store, telegramID int64, title string) {
	t.Helper()
	if err := s.UpsertChat(context.Background(), models.Chat{
		TelegramID: telegramID, Kind: models.ChatKindGroup, Title: title,
	}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
}

// a due, enabled group gets the resume sent, and the send is recorded
func TestTick_SendsWhenDue(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	addGroup(t, s, 1, "Go Jobs")
	if err := s.SetChatResumeConfig(ctx, 1, true, 60, "my resume"); err != nil {
		t.Fatalf("SetChatResumeConfig: %v", err)
	}

	sender := &fakeSender{}
	b := New(s, sender)
	b.tick(ctx, time.Now())

	if len(sender.calls) != 1 || sender.calls[0].text != "my resume" {
		t.Fatalf("calls = %+v", sender.calls)
	}
}

// a chat with no per-chat text override falls back to the global resume text
func TestTick_FallsBackToGlobalText(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	addGroup(t, s, 1, "Go Jobs")
	if err := s.SetResumeGlobalText(ctx, "global pitch"); err != nil {
		t.Fatalf("SetResumeGlobalText: %v", err)
	}
	if err := s.SetChatResumeConfig(ctx, 1, true, 60, ""); err != nil {
		t.Fatalf("SetChatResumeConfig: %v", err)
	}

	sender := &fakeSender{}
	b := New(s, sender)
	b.tick(ctx, time.Now())

	if len(sender.calls) != 1 || sender.calls[0].text != "global pitch" {
		t.Fatalf("calls = %+v", sender.calls)
	}
}

// a per-chat interval below the configured floor is still clamped to the floor
func TestTick_RespectsGlobalMinDelayFloor(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	addGroup(t, s, 1, "Go Jobs")
	if err := s.SetChatResumeConfig(ctx, 1, true, 5, "text"); err != nil { // way below the floor
		t.Fatalf("SetChatResumeConfig: %v", err)
	}
	if err := s.SetResumeMinDelaySeconds(ctx, 3600); err != nil {
		t.Fatalf("SetResumeMinDelaySeconds: %v", err)
	}

	sender := &fakeSender{}
	b := New(s, sender)
	now := time.Now()
	b.tick(ctx, now)
	if len(sender.calls) != 1 {
		t.Fatalf("expected the first send to go through, calls = %+v", sender.calls)
	}

	b.tick(ctx, now.Add(10*time.Second)) // well inside the 1h floor
	if len(sender.calls) != 1 {
		t.Fatalf("expected the floor to suppress the second send, calls = %+v", sender.calls)
	}
}

// once the rolling hourly cap is hit, no further chats send this tick
func TestTick_GlobalHourlyCap(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.SetResumeMaxPerHour(ctx, 1); err != nil {
		t.Fatalf("SetResumeMaxPerHour: %v", err)
	}
	if err := s.SetResumeMinDelaySeconds(ctx, 0); err != nil {
		t.Fatalf("SetResumeMinDelaySeconds: %v", err)
	}
	addGroup(t, s, 1, "A")
	addGroup(t, s, 2, "B")
	if err := s.SetChatResumeConfig(ctx, 1, true, 1, "text"); err != nil {
		t.Fatalf("SetChatResumeConfig 1: %v", err)
	}
	if err := s.SetChatResumeConfig(ctx, 2, true, 1, "text"); err != nil {
		t.Fatalf("SetChatResumeConfig 2: %v", err)
	}

	sender := &fakeSender{}
	b := New(s, sender)
	b.tick(ctx, time.Now())

	if len(sender.calls) != 1 {
		t.Fatalf("expected the hourly cap to allow exactly 1 send, got %d: %+v", len(sender.calls), sender.calls)
	}
}

// a failed send isn't recorded, so it doesn't burn the chat's cooldown or the hourly budget
func TestTick_FailedSendNotRecorded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	addGroup(t, s, 1, "Go Jobs")
	if err := s.SetChatResumeConfig(ctx, 1, true, 0, "text"); err != nil {
		t.Fatalf("SetChatResumeConfig: %v", err)
	}
	if err := s.SetResumeMinDelaySeconds(ctx, 0); err != nil {
		t.Fatalf("SetResumeMinDelaySeconds: %v", err)
	}

	sender := &fakeSender{fail: map[int64]bool{1: true}}
	b := New(s, sender)
	b.tick(ctx, time.Now())

	n, err := s.CountResumeSendsSince(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CountResumeSendsSince: %v", err)
	}
	if n != 0 {
		t.Errorf("expected the failed send to not be recorded, got count = %d", n)
	}
}

// resume broadcasting can never even be enabled on a channel in the first
// place -- storage.SetChatResumeConfig refuses it (covered in
// internal/storage/resume_test.go), so there's no black-box way to get a
// channel into ListResumeEnabledChats for the scheduler to see. The
// scheduler's matching kind check in tick() is defense in depth for a state
// the real write path doesn't produce.

// with sender == nil (e.g. the RSSHub source, which can't send), Run must not
// try to send anything -- it should just idle until ctx is cancelled
func TestRun_NilSenderIsANoop(t *testing.T) {
	s := newTestStore(t)
	b := New(s, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := b.Run(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
}
