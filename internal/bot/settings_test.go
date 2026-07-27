package bot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeacexF/Twork/internal/matcher"
	"github.com/PeacexF/Twork/internal/storage"
)

// builds a Bot with a real temp-file store and a fake Telegram Bot API
func newTestBot(t *testing.T) (*Bot, *fakeAPI) {
	t.Helper()

	store, err := storage.Open(filepath.Join(t.TempDir(), "bot.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	api, fake := newFakeBotAPI(t)
	b := &Bot{
		api:        api,
		store:      store,
		matchStore: matcher.NewStore(MatcherFromKeywords(storage.Keywords{})),
		ownerID:    500,
		sess:       &session{homeChatID: 500, homeMsgID: 1},
	}
	return b, fake
}

// the mode cycles live -> digest -> both -> live and persists each step
func TestCycleNotificationMode(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	// Unset reads as live, so the first cycle lands on digest.
	want := []string{storage.NotifyModeDigest, storage.NotifyModeBoth, storage.NotifyModeLive, storage.NotifyModeDigest}
	for i, expected := range want {
		b.cycleNotificationMode(ctx)
		got, err := b.store.GetNotificationMode(ctx)
		if err != nil {
			t.Fatalf("GetNotificationMode: %v", err)
		}
		if got != expected {
			t.Fatalf("cycle %d: mode = %q, want %q", i+1, got, expected)
		}
	}
}

// an unrecognized stored mode cycles back to a known one rather than sticking
func TestCycleNotificationMode_RecoversFromAnUnknownStoredMode(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	if err := b.store.SetSetting(ctx, "notifications.mode", "telepathy"); err != nil {
		t.Fatal(err)
	}
	b.cycleNotificationMode(ctx)

	got, err := b.store.GetNotificationMode(ctx)
	if err != nil {
		t.Fatalf("GetNotificationMode: %v", err)
	}
	// The unknown value reads back as live, so the cycle moves it to digest.
	if got != storage.NotifyModeDigest {
		t.Errorf("mode = %q, want %q", got, storage.NotifyModeDigest)
	}
}

// each mode gets a distinct short label and description
func TestNotifyModeLabelAndDescription(t *testing.T) {
	cases := []struct {
		mode      string
		wantLabel string
		descHint  string
	}{
		{storage.NotifyModeLive, "live", "each match as it arrives"},
		{storage.NotifyModeDigest, "digest only", "no live pings"},
		{storage.NotifyModeBoth, "live + digest", "AND a daily digest"},
		{"", "live", "each match as it arrives"}, // unset falls back to live
	}
	for _, c := range cases {
		if got := notifyModeLabel(c.mode); got != c.wantLabel {
			t.Errorf("notifyModeLabel(%q) = %q, want %q", c.mode, got, c.wantLabel)
		}
		if got := notifyModeDescription(c.mode); !strings.Contains(got, c.descHint) {
			t.Errorf("notifyModeDescription(%q) = %q, want it to mention %q", c.mode, got, c.descHint)
		}
	}
}

// only a well-formed 24h HH:MM is accepted as a digest time
func TestDigestTimeRegex(t *testing.T) {
	valid := []string{"00:00", "09:00", "09:59", "13:45", "23:59", "10:30"}
	for _, s := range valid {
		if !digestTimeRe.MatchString(s) {
			t.Errorf("%q should be a valid digest time", s)
		}
	}

	invalid := []string{
		"24:00",  // hour out of range
		"23:60",  // minute out of range
		"9:00",   // hour not zero-padded
		"09:0",   // minute not zero-padded
		"0900",   // missing separator
		"09.00",  // wrong separator
		"9am",    // not 24h
		"",       // empty
		" 09:00", // stray whitespace
		"09:00 ",
		"109:00",
		"09:001",
	}
	for _, s := range invalid {
		if digestTimeRe.MatchString(s) {
			t.Errorf("%q should be rejected as a digest time", s)
		}
	}
}

// a valid time is stored; an invalid one leaves the previous value alone
func TestSetDigestTime(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	b.setDigestTime(ctx, "18:30")
	got, err := b.store.GetDigestTime(ctx)
	if err != nil {
		t.Fatalf("GetDigestTime: %v", err)
	}
	if got != "18:30" {
		t.Fatalf("digest time = %q, want 18:30", got)
	}

	b.setDigestTime(ctx, "not a time")
	got, err = b.store.GetDigestTime(ctx)
	if err != nil {
		t.Fatalf("GetDigestTime: %v", err)
	}
	if got != "18:30" {
		t.Errorf("expected the invalid input to be rejected and 18:30 kept, got %q", got)
	}
}

// the dashboard reports the live counters, and degrades gracefully when it can't
func TestHomeDashboardText(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	got := homeDashboardText(ctx, b.store)
	for _, want := range []string{"TWORK", "Chats monitored: 0", "Messages indexed: 0", "Bookmarks: 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("dashboard is missing %q:\n%s", want, got)
		}
	}

	b.store.Close() // force GetStats to fail
	if got := homeDashboardText(ctx, b.store); !strings.Contains(got, "couldn't load stats") {
		t.Errorf("expected a graceful fallback when stats can't be read, got:\n%s", got)
	}
}

// loadKeywords defaults the mode when nothing is stored yet
func TestLoadKeywords_DefaultsMode(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	kw := b.loadKeywords(ctx)
	if kw.Mode != "whole_word" {
		t.Errorf("mode = %q, want whole_word by default", kw.Mode)
	}
	if len(kw.PositiveGroups) != 0 {
		t.Errorf("expected no groups on a fresh database, got %+v", kw.PositiveGroups)
	}
}

// a stored mode is used as-is rather than being overwritten by the default
func TestLoadKeywords_KeepsStoredMode(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	if err := b.store.SetKeywords(ctx, storage.Keywords{
		PositiveGroups: []storage.KeywordGroup{{Name: "Go", Aliases: []string{"go"}}},
		Mode:           "substring",
	}); err != nil {
		t.Fatalf("SetKeywords: %v", err)
	}

	kw := b.loadKeywords(ctx)
	if kw.Mode != "substring" {
		t.Errorf("mode = %q, want substring", kw.Mode)
	}
	if len(kw.PositiveGroups) != 1 {
		t.Errorf("expected the stored group, got %+v", kw.PositiveGroups)
	}
}
