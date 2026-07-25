package matcher

import (
	"testing"

	"github.com/PeacexF/Twork/internal/config"
)

// builds a Matcher or fails the test
func mustNew(t *testing.T, cfg config.MatchingConfig) *Matcher {
	t.Helper()
	m, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return m
}

// positive keywords match on word boundaries
func TestMatch_WholeWord_Basic(t *testing.T) {
	m := mustNew(t, config.MatchingConfig{
		Positive: []string{"Go", "Docker", "PostgreSQL"},
		Negative: []string{"Senior"},
		Mode:     config.MatchModeWholeWord,
	})

	res := m.Match("Looking for a Go backend developer with Docker and PostgreSQL experience")
	if !res.Matched() {
		t.Fatalf("expected match, got %+v", res)
	}
	want := map[string]bool{"Go": true, "Docker": true, "PostgreSQL": true}
	if len(res.MatchedKeywords) != len(want) {
		t.Fatalf("expected %d matched keywords, got %v", len(want), res.MatchedKeywords)
	}
	for _, k := range res.MatchedKeywords {
		if !want[k] {
			t.Fatalf("unexpected keyword %q in result %v", k, res.MatchedKeywords)
		}
	}
}

// whole-word mode must not match inside a longer word
func TestMatch_WholeWord_AvoidsSubstringFalsePositive(t *testing.T) {
	m := mustNew(t, config.MatchingConfig{
		Positive: []string{"Go"},
		Mode:     config.MatchModeWholeWord,
	})

	res := m.Match("We use Google Cloud and Golang tooling")
	if res.Matched() {
		t.Fatalf("expected no match (Go should not match inside Google/Golang), got %+v", res)
	}
}

// substring mode matches inside a longer word
func TestMatch_Substring_MatchesInsideWords(t *testing.T) {
	m := mustNew(t, config.MatchingConfig{
		Positive: []string{"Go"},
		Mode:     config.MatchModeSubstring,
	})

	res := m.Match("We use Golang tooling")
	if !res.Matched() {
		t.Fatalf("expected substring match inside Golang, got %+v", res)
	}
}

// a negative keyword rejects the message and hides positive hits
func TestMatch_NegativeKeywordRejects(t *testing.T) {
	m := mustNew(t, config.MatchingConfig{
		Positive: []string{"Go", "Docker"},
		Negative: []string{"Senior"},
		Mode:     config.MatchModeWholeWord,
	})

	res := m.Match("Senior Go and Docker engineer needed")
	if res.Matched() {
		t.Fatalf("expected rejection due to negative keyword, got %+v", res)
	}
	if res.NegativeKeyword != "Senior" {
		t.Fatalf("expected NegativeKeyword = Senior, got %q", res.NegativeKeyword)
	}
	if len(res.MatchedKeywords) != 0 {
		t.Fatalf("expected no reported positive matches on rejection, got %v", res.MatchedKeywords)
	}
}

// keyword matching is case-insensitive
func TestMatch_CaseInsensitive(t *testing.T) {
	m := mustNew(t, config.MatchingConfig{
		Positive: []string{"docker"},
		Mode:     config.MatchModeWholeWord,
	})

	res := m.Match("We need DOCKER experience")
	if !res.Matched() {
		t.Fatalf("expected case-insensitive match, got %+v", res)
	}
}

// no configured keyword present means no match
func TestMatch_NoPositiveKeywordFound(t *testing.T) {
	m := mustNew(t, config.MatchingConfig{
		Positive: []string{"Rust"},
		Mode:     config.MatchModeWholeWord,
	})

	res := m.Match("Looking for a Python developer")
	if res.Matched() {
		t.Fatalf("expected no match, got %+v", res)
	}
}

// keywords with regex metacharacters (C++, C#) still match correctly
func TestMatch_KeywordWithRegexMetacharacters(t *testing.T) {
	m := mustNew(t, config.MatchingConfig{
		Positive: []string{"C++", "C#"},
		Mode:     config.MatchModeWholeWord,
	})

	res := m.Match("Backend role in C++ and some C# tooling")
	if !res.Matched() {
		t.Fatalf("expected match on regex-metacharacter keywords, got %+v", res)
	}
	if len(res.MatchedKeywords) != 2 {
		t.Fatalf("expected both C++ and C# to match, got %v", res.MatchedKeywords)
	}
}

// Store.Set swaps the matcher returned by Get
func TestStore_GetSet(t *testing.T) {
	m1 := mustNew(t, config.MatchingConfig{Positive: []string{"Go"}, Mode: config.MatchModeWholeWord})
	m2 := mustNew(t, config.MatchingConfig{Positive: []string{"Rust"}, Mode: config.MatchModeWholeWord})

	store := NewStore(m1)
	if store.Get() != m1 {
		t.Fatalf("expected Get() to return initial matcher")
	}
	store.Set(m2)
	if store.Get() != m2 {
		t.Fatalf("expected Get() to return updated matcher after Set")
	}
}

// a group matches when any alias hits, and reports the group name not the alias
func TestMatch_Group_AnyAliasHits(t *testing.T) {
	m := NewFromGroups(config.MatchModeWholeWord,
		[]config.KeywordGroup{{Name: "Go", Aliases: []string{"go", "golang", "gopher"}}},
		nil,
	)

	for _, text := range []string{
		"We need a Golang dev",
		"gopher wanted",
		"backend in Go",
	} {
		res := m.Match(text)
		if !res.Matched() {
			t.Fatalf("expected match for %q, got %+v", text, res)
		}
		if len(res.MatchedKeywords) != 1 || res.MatchedKeywords[0] != "Go" {
			t.Fatalf("expected matched group name [Go] for %q, got %v", text, res.MatchedKeywords)
		}
	}
}

// a negative group rejects the post and reports the group name
func TestMatch_NegativeGroup_Rejects(t *testing.T) {
	m := NewFromGroups(config.MatchModeWholeWord,
		[]config.KeywordGroup{{Name: "Go", Aliases: []string{"go"}}},
		[]config.KeywordGroup{{Name: "Seniority", Aliases: []string{"senior", "lead", "staff"}}},
	)

	res := m.Match("Senior Go engineer")
	if res.Matched() {
		t.Fatalf("expected rejection, got %+v", res)
	}
	if res.NegativeKeyword != "Seniority" {
		t.Fatalf("expected negative group name Seniority, got %q", res.NegativeKeyword)
	}
}

// a per-group mode overrides the global default
func TestMatch_PerGroupModeOverride(t *testing.T) {
	// Global default is whole_word, but this group forces substring, so
	// "js" should match inside "nodejs".
	m := NewFromGroups(config.MatchModeWholeWord,
		[]config.KeywordGroup{{Name: "JS", Aliases: []string{"js"}, Mode: config.MatchModeSubstring}},
		nil,
	)
	if !m.Match("we use nodejs here").Matched() {
		t.Fatalf("expected substring override to match js inside nodejs")
	}

	// Same alias without the override should NOT match inside nodejs.
	m2 := NewFromGroups(config.MatchModeWholeWord,
		[]config.KeywordGroup{{Name: "JS", Aliases: []string{"js"}}},
		nil,
	)
	if m2.Match("we use nodejs here").Matched() {
		t.Fatalf("expected whole-word default to NOT match js inside nodejs")
	}
}

// flat keyword lists migrate to single-alias groups named after themselves
func TestNew_FlatListsMigrateToGroups(t *testing.T) {
	m, err := New(config.MatchingConfig{
		Positive: []string{"Rust"},
		Mode:     config.MatchModeWholeWord,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := m.Match("Rust developer wanted")
	if !res.Matched() || res.MatchedKeywords[0] != "Rust" {
		t.Fatalf("expected flat keyword Rust to migrate to a group, got %+v", res)
	}
}
