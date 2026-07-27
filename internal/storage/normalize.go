package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

var urlRe = regexp.MustCompile(`https?://\S+|t\.me/\S+|@\w+`)

// normalizes text for global dedup: lowercase, strip urls/@mentions,
// drop punctuation and emoji, collapse whitespace
func normalizeForDedup(text string) string {
	s := strings.ToLower(text)
	s = urlRe.ReplaceAllString(s, " ")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			// punctuation, symbols, emoji -> dropped
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// returns the dedup hash of a message's text, or "" if the normalized text is empty
func dedupHash(text string) string {
	norm := normalizeForDedup(text)
	if norm == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}
