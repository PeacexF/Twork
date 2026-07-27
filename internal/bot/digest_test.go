package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/storage"
)

// the digest reports the new-post count and lists each match
func TestFormatDigest_WithMatches(t *testing.T) {
	b := &Bot{}
	matches := []storage.MatchRow{
		{ChatTitle: "Go Jobs", Text: "Backend Go dev\nremote", Link: "https://t.me/x/1", Timestamp: time.Now()},
		{ChatTitle: "Remote Work", Text: "Platform engineer", Link: "https://t.me/y/2", Timestamp: time.Now()},
	}

	out := b.formatDigest(140, matches)
	if !strings.Contains(out, "140 new post(s)") {
		t.Errorf("expected new-post count in digest, got:\n%s", out)
	}
	if !strings.Contains(out, "2 matched") {
		t.Errorf("expected match count in digest, got:\n%s", out)
	}
	for _, want := range []string{"Go Jobs", "Remote Work", "https://t.me/x/1"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in digest, got:\n%s", want, out)
		}
	}
}

// a digest with no matches still reports the post count and says so
func TestFormatDigest_NoMatches(t *testing.T) {
	b := &Bot{}
	out := b.formatDigest(37, nil)
	if !strings.Contains(out, "37 new post(s)") {
		t.Errorf("expected post count, got:\n%s", out)
	}
	if !strings.Contains(out, "Nothing matched today") {
		t.Errorf("expected empty-state line, got:\n%s", out)
	}
}

// only the first line is shown, truncated when long
func TestFirstLine(t *testing.T) {
	if got := firstLine("first\nsecond\nthird", 100); got != "first" {
		t.Errorf("expected only the first line, got %q", got)
	}
	long := strings.Repeat("a", 200)
	got := firstLine(long, 50)
	if len([]rune(got)) != 51 { // 50 runes + ellipsis
		t.Errorf("expected truncation to 50 runes plus ellipsis, got %d runes", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
}
