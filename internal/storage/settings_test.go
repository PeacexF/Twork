package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func newSettingsStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// groups round-trip through storage intact
func TestKeywords_GroupsRoundTrip(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	in := Keywords{
		PositiveGroups: []KeywordGroup{{Name: "Go", Aliases: []string{"go", "golang"}, Mode: "substring"}},
		NegativeGroups: []KeywordGroup{{Name: "Seniority", Aliases: []string{"senior", "lead"}}},
		Mode:           "whole_word",
	}
	if err := s.SetKeywords(ctx, in); err != nil {
		t.Fatalf("SetKeywords: %v", err)
	}

	got, ok, err := s.GetKeywords(ctx)
	if err != nil || !ok {
		t.Fatalf("GetKeywords: ok=%v err=%v", ok, err)
	}
	if len(got.PositiveGroups) != 1 || got.PositiveGroups[0].Name != "Go" || len(got.PositiveGroups[0].Aliases) != 2 {
		t.Fatalf("positive groups round-trip mismatch: %+v", got.PositiveGroups)
	}
	if got.PositiveGroups[0].Mode != "substring" {
		t.Fatalf("expected per-group mode preserved, got %q", got.PositiveGroups[0].Mode)
	}
	if got.NegativeGroups[0].Name != "Seniority" {
		t.Fatalf("negative group round-trip mismatch: %+v", got.NegativeGroups)
	}
}

// a legacy flat keyword list is migrated to single-alias groups on read
func TestKeywords_LegacyFlatListMigrates(t *testing.T) {
	s := newSettingsStore(t)
	ctx := context.Background()

	// Simulate an older install: only the flat keys are set, no group keys.
	if err := s.SetSetting(ctx, settingKeywordsPositive, `["Go","Rust"]`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, settingKeywordsNegative, `["Senior"]`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, settingKeywordsMode, "whole_word"); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.GetKeywords(ctx)
	if err != nil || !ok {
		t.Fatalf("GetKeywords: ok=%v err=%v", ok, err)
	}
	if len(got.PositiveGroups) != 2 {
		t.Fatalf("expected 2 migrated positive groups, got %+v", got.PositiveGroups)
	}
	if got.PositiveGroups[0].Name != "Go" || len(got.PositiveGroups[0].Aliases) != 1 || got.PositiveGroups[0].Aliases[0] != "Go" {
		t.Fatalf("expected Go migrated to single-alias group, got %+v", got.PositiveGroups[0])
	}
	if len(got.NegativeGroups) != 1 || got.NegativeGroups[0].Name != "Senior" {
		t.Fatalf("expected Senior migrated, got %+v", got.NegativeGroups)
	}
}
