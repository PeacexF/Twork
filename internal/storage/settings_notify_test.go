package storage

import (
	"context"
	"testing"
)

// an unset raw setting reports not-found instead of an error
func TestGetSetting_Unset(t *testing.T) {
	s := newSettingsStore(t)
	v, ok, err := s.GetSetting(context.Background(), "nothing.here")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if ok || v != "" {
		t.Errorf("expected an unset key to report (\"\", false), got (%q, %v)", v, ok)
	}
}

// writing the same key twice replaces the value rather than failing
func TestSetSetting_Overwrites(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	if err := s.SetSetting(ctx, "k", "first"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if err := s.SetSetting(ctx, "k", "second"); err != nil {
		t.Fatalf("SetSetting (overwrite): %v", err)
	}
	v, ok, err := s.GetSetting(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("GetSetting: ok=%v err=%v", ok, err)
	}
	if v != "second" {
		t.Errorf("value = %q, want \"second\"", v)
	}
}

// on a fresh database there are no keywords to load yet
func TestGetKeywords_Unset(t *testing.T) {
	s := newSettingsStore(t)
	_, ok, err := s.GetKeywords(context.Background())
	if err != nil {
		t.Fatalf("GetKeywords: %v", err)
	}
	if ok {
		t.Error("expected no stored keywords on a fresh database")
	}
}

// a corrupt stored value surfaces as a decode error, not silent data loss
func TestGetKeywords_CorruptJSON(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	if err := s.SetSetting(ctx, settingKeywordsPositiveGroups, `{not json`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GetKeywords(ctx); err == nil {
		t.Error("expected a decode error for a corrupt group list")
	}

	s2 := newSettingsStore(t)
	if err := s2.SetSetting(ctx, settingKeywordsPositive, `{not json`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s2.GetKeywords(ctx); err == nil {
		t.Error("expected a decode error for a corrupt legacy list")
	}
}

// saving an empty keyword set is legal and reads back as "stored, but empty"
func TestSetKeywords_Empty(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	if err := s.SetKeywords(ctx, Keywords{Mode: "substring"}); err != nil {
		t.Fatalf("SetKeywords: %v", err)
	}
	got, ok, err := s.GetKeywords(ctx)
	if err != nil || !ok {
		t.Fatalf("GetKeywords: ok=%v err=%v", ok, err)
	}
	if len(got.PositiveGroups) != 0 || len(got.NegativeGroups) != 0 {
		t.Errorf("expected empty groups, got %+v", got)
	}
	if got.Mode != "substring" {
		t.Errorf("mode = %q, want substring", got.Mode)
	}
}

// once group keys exist they win; the legacy flat lists are ignored
func TestGetKeywords_GroupsWinOverLegacyFlatLists(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	if err := s.SetSetting(ctx, settingKeywordsPositive, `["Legacy"]`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKeywords(ctx, Keywords{
		PositiveGroups: []KeywordGroup{{Name: "Go", Aliases: []string{"go"}}},
		Mode:           "whole_word",
	}); err != nil {
		t.Fatalf("SetKeywords: %v", err)
	}

	got, ok, err := s.GetKeywords(ctx)
	if err != nil || !ok {
		t.Fatalf("GetKeywords: ok=%v err=%v", ok, err)
	}
	if len(got.PositiveGroups) != 1 || got.PositiveGroups[0].Name != "Go" {
		t.Errorf("expected the group list to take precedence, got %+v", got.PositiveGroups)
	}
}

// the notifications toggle reports "never set" until it's written once
func TestNotificationsEnabled_RoundTrip(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	if _, ok, err := s.GetNotificationsEnabled(ctx); err != nil || ok {
		t.Fatalf("expected the toggle to be unset on a fresh db, got ok=%v err=%v", ok, err)
	}

	if err := s.SetNotificationsEnabled(ctx, true); err != nil {
		t.Fatalf("SetNotificationsEnabled(true): %v", err)
	}
	enabled, ok, err := s.GetNotificationsEnabled(ctx)
	if err != nil || !ok || !enabled {
		t.Fatalf("expected (true, true, nil), got (%v, %v, %v)", enabled, ok, err)
	}

	if err := s.SetNotificationsEnabled(ctx, false); err != nil {
		t.Fatalf("SetNotificationsEnabled(false): %v", err)
	}
	enabled, ok, err = s.GetNotificationsEnabled(ctx)
	if err != nil || !ok || enabled {
		t.Fatalf("expected (false, true, nil), got (%v, %v, %v)", enabled, ok, err)
	}
}

// the notification mode defaults to live, including when the stored value is junk
func TestNotificationMode_DefaultsAndRoundTrip(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	mode, err := s.GetNotificationMode(ctx)
	if err != nil {
		t.Fatalf("GetNotificationMode: %v", err)
	}
	if mode != NotifyModeLive {
		t.Errorf("default mode = %q, want %q", mode, NotifyModeLive)
	}

	for _, want := range []string{NotifyModeDigest, NotifyModeBoth, NotifyModeLive} {
		if err := s.SetNotificationMode(ctx, want); err != nil {
			t.Fatalf("SetNotificationMode(%q): %v", want, err)
		}
		got, err := s.GetNotificationMode(ctx)
		if err != nil {
			t.Fatalf("GetNotificationMode: %v", err)
		}
		if got != want {
			t.Errorf("mode = %q, want %q", got, want)
		}
	}

	// An unrecognized stored value falls back to live rather than propagating.
	if err := s.SetSetting(ctx, settingNotificationMode, "telepathy"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetNotificationMode(ctx); err != nil || got != NotifyModeLive {
		t.Errorf("unknown stored mode = %q, %v; want live, nil", got, err)
	}
}

// the digest time defaults to 09:00 until set
func TestDigestTime_DefaultsAndRoundTrip(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	got, err := s.GetDigestTime(ctx)
	if err != nil {
		t.Fatalf("GetDigestTime: %v", err)
	}
	if got != "09:00" {
		t.Errorf("default digest time = %q, want 09:00", got)
	}

	if err := s.SetDigestTime(ctx, "18:30"); err != nil {
		t.Fatalf("SetDigestTime: %v", err)
	}
	if got, err = s.GetDigestTime(ctx); err != nil || got != "18:30" {
		t.Errorf("digest time = %q, %v; want 18:30, nil", got, err)
	}

	// An empty stored value falls back to the default.
	if err := s.SetSetting(ctx, settingDigestTime, ""); err != nil {
		t.Fatal(err)
	}
	if got, err = s.GetDigestTime(ctx); err != nil || got != "09:00" {
		t.Errorf("empty digest time = %q, %v; want 09:00, nil", got, err)
	}
}

// the claimed owner ID round-trips through the settings table
func TestBotOwnerID_RoundTrip(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	if _, ok, err := s.GetBotOwnerID(ctx); err != nil || ok {
		t.Fatalf("expected no owner on a fresh db, got ok=%v err=%v", ok, err)
	}

	if err := s.SetBotOwnerID(ctx, 987654321); err != nil {
		t.Fatalf("SetBotOwnerID: %v", err)
	}
	id, ok, err := s.GetBotOwnerID(ctx)
	if err != nil || !ok {
		t.Fatalf("GetBotOwnerID: ok=%v err=%v", ok, err)
	}
	if id != 987654321 {
		t.Errorf("owner id = %d, want 987654321", id)
	}

	// A non-numeric stored value is reported as an error, not as owner 0.
	if err := s.SetSetting(ctx, settingBotOwnerID, "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.GetBotOwnerID(ctx); err == nil || ok {
		t.Errorf("expected a parse error for a corrupt owner id, got ok=%v err=%v", ok, err)
	}
}

// opening a database in a directory that doesn't exist fails cleanly
func TestOpen_UnwritablePath(t *testing.T) {
	if _, err := Open(t.TempDir() + "/no/such/dir/twork.db"); err == nil {
		t.Error("expected Open to fail for a path whose directory doesn't exist")
	}
}

// Close releases the handle and further queries fail rather than hang
func TestClose(t *testing.T) {
	s, err := Open(t.TempDir() + "/close.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.GetStats(context.Background()); err == nil {
		t.Error("expected queries on a closed store to fail")
	}
}
