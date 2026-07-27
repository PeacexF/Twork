package bot

import (
	"strings"
	"testing"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/storage"
)

// group tokens round-trip through their callback_data encoding
func TestGroupToken_RoundTrip(t *testing.T) {
	cases := []struct {
		positive bool
		index    int
		want     string
	}{
		{true, 0, "p0"},
		{true, 3, "p3"},
		{false, 0, "n0"},
		{false, 12, "n12"},
	}
	for _, c := range cases {
		token := groupToken(c.positive, c.index)
		if token != c.want {
			t.Errorf("groupToken(%v, %d) = %q, want %q", c.positive, c.index, token, c.want)
		}
		positive, index, ok := parseGroupToken(token)
		if !ok {
			t.Fatalf("parseGroupToken(%q) reported failure", token)
		}
		if positive != c.positive || index != c.index {
			t.Errorf("parseGroupToken(%q) = (%v, %d), want (%v, %d)", token, positive, index, c.positive, c.index)
		}
	}
}

// malformed tokens are rejected instead of silently addressing group 0
func TestParseGroupToken_Invalid(t *testing.T) {
	for _, token := range []string{"", "p", "n", "px", "p1x", "pp1", "1"} {
		if _, _, ok := parseGroupToken(token); ok {
			t.Errorf("parseGroupToken(%q) should have failed", token)
		}
	}
}

// anything that isn't a "p" prefix addresses the negative list
func TestParseGroupToken_NonPPrefixIsNegative(t *testing.T) {
	positive, index, ok := parseGroupToken("n7")
	if !ok || positive || index != 7 {
		t.Errorf("parseGroupToken(\"n7\") = (%v, %d, %v)", positive, index, ok)
	}
}

// writeGroup replaces the group at its index in the right list
func TestWriteGroup(t *testing.T) {
	kw := storage.Keywords{
		PositiveGroups: []storage.KeywordGroup{{Name: "Go"}, {Name: "Rust"}},
		NegativeGroups: []storage.KeywordGroup{{Name: "Senior"}},
	}

	writeGroup(&kw, true, 1, storage.KeywordGroup{Name: "Rust", Aliases: []string{"rust", "rustlang"}})
	if len(kw.PositiveGroups[1].Aliases) != 2 {
		t.Errorf("positive group not written: %+v", kw.PositiveGroups[1])
	}
	if kw.PositiveGroups[0].Name != "Go" {
		t.Error("writing one group disturbed another")
	}

	writeGroup(&kw, false, 0, storage.KeywordGroup{Name: "Seniority"})
	if kw.NegativeGroups[0].Name != "Seniority" {
		t.Errorf("negative group not written: %+v", kw.NegativeGroups[0])
	}
}

// an out-of-range index is a no-op, never a panic or an appended group
func TestWriteGroup_OutOfRangeIsNoOp(t *testing.T) {
	kw := storage.Keywords{
		PositiveGroups: []storage.KeywordGroup{{Name: "Go"}},
		NegativeGroups: []storage.KeywordGroup{{Name: "Senior"}},
	}

	writeGroup(&kw, true, 5, storage.KeywordGroup{Name: "Ghost"})
	writeGroup(&kw, true, -1, storage.KeywordGroup{Name: "Ghost"})
	writeGroup(&kw, false, 5, storage.KeywordGroup{Name: "Ghost"})
	writeGroup(&kw, false, -1, storage.KeywordGroup{Name: "Ghost"})

	if len(kw.PositiveGroups) != 1 || kw.PositiveGroups[0].Name != "Go" {
		t.Errorf("positive groups changed: %+v", kw.PositiveGroups)
	}
	if len(kw.NegativeGroups) != 1 || kw.NegativeGroups[0].Name != "Senior" {
		t.Errorf("negative groups changed: %+v", kw.NegativeGroups)
	}
}

// aliases are split on commas and newlines, trimmed, and emptied entries dropped
func TestParseKeywordInput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single", "golang", []string{"golang"}},
		{"comma separated", "go, golang, gopher", []string{"go", "golang", "gopher"}},
		{"newline separated", "go\ngolang\ngopher", []string{"go", "golang", "gopher"}},
		{"mixed separators", "go, golang\ngopher", []string{"go", "golang", "gopher"}},
		{"whitespace is trimmed", "  go  ,  golang  ", []string{"go", "golang"}},
		{"empty entries are dropped", "go,,golang,", []string{"go", "golang"}},
		{"only separators", ",,\n,", nil},
		{"empty input", "", nil},
		{"whitespace only", "   ", nil},
		{"multi-word aliases survive", "senior engineer, tech lead", []string{"senior engineer", "tech lead"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseKeywordInput(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("parseKeywordInput(%q) = %+v, want %+v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("parseKeywordInput(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

// a group is labelled by name plus its aliases, or just its name when it has none
func TestGroupLabel(t *testing.T) {
	withAliases := groupLabel(storage.KeywordGroup{Name: "Go", Aliases: []string{"go", "golang"}})
	if withAliases != "Go (go, golang)" {
		t.Errorf("label = %q", withAliases)
	}

	if got := groupLabel(storage.KeywordGroup{Name: "Empty"}); got != "Empty" {
		t.Errorf("label with no aliases = %q, want \"Empty\"", got)
	}
}

func TestBulletList(t *testing.T) {
	if got := bulletList(nil); got != "(none)" {
		t.Errorf("empty list = %q, want (none)", got)
	}
	if got := bulletList([]string{}); got != "(none)" {
		t.Errorf("empty slice = %q, want (none)", got)
	}

	got := bulletList([]string{"go", "golang"})
	if got != "• go\n• golang" {
		t.Errorf("bulletList = %q", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("expected no trailing newline")
	}
}

// the global-mode toggle label always names the mode that's active now
func TestModeToggleLabel(t *testing.T) {
	if got := modeToggleLabel(config.MatchModeWholeWord); !strings.Contains(got, "whole word") {
		t.Errorf("label = %q, want it to mention whole word", got)
	}
	if got := modeToggleLabel(config.MatchModeSubstring); !strings.Contains(got, "substring") {
		t.Errorf("label = %q, want it to mention substring", got)
	}
	// Anything unrecognized reads as substring rather than blank.
	if got := modeToggleLabel(""); !strings.Contains(got, "substring") {
		t.Errorf("label for an unset mode = %q", got)
	}
}

// a group with no mode of its own is labelled as inheriting the default
func TestGroupModeLabel(t *testing.T) {
	cases := []struct{ mode, want string }{
		{config.MatchModeWholeWord, "Mode: whole word"},
		{config.MatchModeSubstring, "Mode: substring"},
		{"", "Mode: default"},
		{"nonsense", "Mode: default"},
	}
	for _, c := range cases {
		if got := groupModeLabel(c.mode); got != c.want {
			t.Errorf("groupModeLabel(%q) = %q, want %q", c.mode, got, c.want)
		}
	}
}

// stored groups convert to the config type the matcher consumes, field for field
func TestStoredGroupsToConfig(t *testing.T) {
	in := []storage.KeywordGroup{
		{Name: "Go", Aliases: []string{"go", "golang"}, Mode: config.MatchModeSubstring},
		{Name: "Rust", Aliases: []string{"rust"}},
	}
	got := storedGroupsToConfig(in)

	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got))
	}
	if got[0].Name != "Go" || len(got[0].Aliases) != 2 || got[0].Mode != config.MatchModeSubstring {
		t.Errorf("group 0 = %+v", got[0])
	}
	if got[1].Mode != "" {
		t.Errorf("expected an unset mode to stay unset, got %q", got[1].Mode)
	}

	if got := storedGroupsToConfig(nil); len(got) != 0 {
		t.Errorf("expected no groups for nil input, got %+v", got)
	}
}

// the matcher built from stored keywords honours groups, negatives, and modes
func TestMatcherFromKeywords(t *testing.T) {
	m := MatcherFromKeywords(storage.Keywords{
		Mode: config.MatchModeWholeWord,
		PositiveGroups: []storage.KeywordGroup{
			{Name: "Go", Aliases: []string{"go", "golang"}},
		},
		NegativeGroups: []storage.KeywordGroup{
			{Name: "Seniority", Aliases: []string{"senior"}},
		},
	})

	// Any alias in the group hits, and the group's *name* is what's reported.
	res := m.Match("We need a golang developer")
	if !res.Matched() {
		t.Fatalf("expected a match, got %+v", res)
	}
	if len(res.MatchedKeywords) != 1 || res.MatchedKeywords[0] != "Go" {
		t.Errorf("matched keywords = %+v, want [Go]", res.MatchedKeywords)
	}

	// A negative group vetoes an otherwise-matching post.
	res = m.Match("Senior golang developer")
	if res.Matched() {
		t.Error("expected the negative group to reject the post")
	}
	if res.NegativeKeyword != "Seniority" {
		t.Errorf("negative keyword = %q, want Seniority", res.NegativeKeyword)
	}

	// whole_word means "gopher" doesn't contain the keyword "go".
	if m.Match("gopher gathering").Matched() {
		t.Error("expected whole-word mode to reject a substring hit")
	}
}

// a per-group mode overrides the global default
func TestMatcherFromKeywords_PerGroupModeOverride(t *testing.T) {
	m := MatcherFromKeywords(storage.Keywords{
		Mode: config.MatchModeWholeWord,
		PositiveGroups: []storage.KeywordGroup{
			{Name: "Go", Aliases: []string{"go"}, Mode: config.MatchModeSubstring},
		},
	})
	if !m.Match("gopher gathering").Matched() {
		t.Error("expected the group's substring mode to override the global whole-word default")
	}
}

// an empty keyword set matches nothing rather than everything
func TestMatcherFromKeywords_Empty(t *testing.T) {
	m := MatcherFromKeywords(storage.Keywords{Mode: config.MatchModeWholeWord})
	if m.Match("any text at all").Matched() {
		t.Error("expected no matches with no positive groups configured")
	}
}
