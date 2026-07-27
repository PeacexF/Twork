package matcher

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/PeacexF/Twork/internal/config"
)

type Matcher struct {
	positive []compiledGroup
	negative []compiledGroup
}

type compiledGroup struct {
	name      string
	wholeWord bool
	aliases   []string // all lowercased
}

// compiles a Matcher from groups, resolving each group's effective mode against the global default
func NewFromGroups(defaultMode string, positive, negative []config.KeywordGroup) *Matcher {
	return &Matcher{
		positive: compileGroups(positive, defaultMode),
		negative: compileGroups(negative, defaultMode),
	}
}

// lowercases and trims a group's aliases, resolving its effective matching mode
func compileGroups(groups []config.KeywordGroup, defaultMode string) []compiledGroup {
	out := make([]compiledGroup, 0, len(groups))
	for _, g := range groups {
		mode := g.Mode
		if mode == "" {
			mode = defaultMode
		}
		var aliases []string
		for _, a := range g.Aliases {
			a = strings.TrimSpace(strings.ToLower(a))
			if a != "" {
				aliases = append(aliases, a)
			}
		}
		if len(aliases) == 0 {
			continue
		}
		out = append(out, compiledGroup{
			name:      g.Name,
			wholeWord: mode == config.MatchModeWholeWord,
			aliases:   aliases,
		})
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
	MatchedKeywords []string // matched positive group names
	NegativeKeyword string   // name of the negative group that rejected, if any
}

// reports whether at least one positive group hit and no negative did
func (r Result) Matched() bool {
	return len(r.MatchedKeywords) > 0 && r.NegativeKeyword == ""
}

// checks text against the negative then positive groups
func (m *Matcher) Match(text string) Result {
	lowerText := strings.ToLower(text)

	for _, g := range m.negative {
		if g.matches(lowerText) {
			return Result{NegativeKeyword: g.name}
		}
	}

	var hits []string
	for _, g := range m.positive {
		if g.matches(lowerText) {
			hits = append(hits, g.name)
		}
	}
	return Result{MatchedKeywords: hits}
}

// reports whether any of the group's aliases occur in the lowercased text
func (g compiledGroup) matches(lowerText string) bool {
	for _, alias := range g.aliases {
		if g.wholeWord {
			if wholeWordContains(lowerText, alias) {
				return true
			}
		} else if strings.Contains(lowerText, alias) {
			return true
		}
	}
	return false
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
