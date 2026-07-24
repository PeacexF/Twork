package matcher

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/PeacexF/Twork/internal/config"
)

type Matcher struct {
	mode     string
	positive []compiledKeyword
	negative []compiledKeyword
}

type compiledKeyword struct {
	original string
	lower    string
}

// compiles a Matcher from matching config
func New(cfg config.MatchingConfig) (*Matcher, error) {
	m := &Matcher{mode: cfg.Mode}
	m.positive = compileKeywords(cfg.Positive)
	m.negative = compileKeywords(cfg.Negative)
	return m, nil
}

// trims and lowercases a keyword list
func compileKeywords(words []string) []compiledKeyword {
	out := make([]compiledKeyword, 0, len(words))
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		out = append(out, compiledKeyword{original: w, lower: strings.ToLower(w)})
	}
	return out
}

// reports whether r is a letter, digit, or underscore
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// finds needle in haystack via manual boundary checks instead of regexp \b
func wholeWordContains(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	searchFrom := 0
	for searchFrom < len(haystack) {
		idx := strings.Index(haystack[searchFrom:], needle)
		if idx == -1 {
			return false
		}
		start := searchFrom + idx
		end := start + len(needle)

		beforeOK := true
		if start > 0 {
			r, _ := utf8.DecodeLastRuneInString(haystack[:start])
			beforeOK = !isWordChar(r)
		}
		afterOK := true
		if end < len(haystack) {
			r, _ := utf8.DecodeRuneInString(haystack[end:])
			afterOK = !isWordChar(r)
		}
		if beforeOK && afterOK {
			return true
		}
		searchFrom = start + 1
	}
	return false
}

type Result struct {
	MatchedKeywords []string
	NegativeKeyword string
}

// reports whether at least one positive keyword hit and no negative did
func (r Result) Matched() bool {
	return len(r.MatchedKeywords) > 0 && r.NegativeKeyword == ""
}

// checks text against the negative then positive keyword lists
func (m *Matcher) Match(text string) Result {
	lowerText := strings.ToLower(text)

	for _, kw := range m.negative {
		if m.contains(lowerText, kw) {
			return Result{NegativeKeyword: kw.original}
		}
	}

	var hits []string
	for _, kw := range m.positive {
		if m.contains(lowerText, kw) {
			hits = append(hits, kw.original)
		}
	}
	return Result{MatchedKeywords: hits}
}

// checks one keyword against text in the configured mode
func (m *Matcher) contains(lowerText string, kw compiledKeyword) bool {
	if m.mode == config.MatchModeWholeWord {
		return wholeWordContains(lowerText, kw.lower)
	}
	return strings.Contains(lowerText, kw.lower)
}

type Store struct {
	mu      sync.RWMutex
	current *Matcher
}

// wraps an initial matcher for hot-swapping
func NewStore(initial *Matcher) *Store {
	return &Store{current: initial}
}

// returns the current matcher
func (s *Store) Get() *Matcher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// swaps in a new matcher
func (s *Store) Set(m *Matcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = m
}
