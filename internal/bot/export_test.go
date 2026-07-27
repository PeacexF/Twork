package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/storage"
)

// the exported document carries a header and one section per post
func TestRenderMarkdown(t *testing.T) {
	rows := []storage.MatchRow{
		{
			ChatTitle:       "Golang Jobs",
			Text:            "Backend Go developer",
			Link:            "https://t.me/golang_jobs/1",
			Timestamp:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
			MatchedKeywords: []string{"Go", "Backend"},
		},
		{
			ChatTitle: "Remote Work",
			Text:      "Platform engineer",
			Link:      "https://t.me/remote/2",
			Timestamp: time.Date(2026, 7, 24, 9, 30, 0, 0, time.UTC),
		},
	}

	md := renderMarkdown("Twork Matches", rows)

	for _, want := range []string{
		"# Twork Matches",
		"_2 post(s), newest first._",
		"## Golang Jobs",
		"## Remote Work",
		"**Matched:** Go, Backend",
		"2026-07-25 12:00 UTC",
		"[Open original](https://t.me/golang_jobs/1)",
		"[Open original](https://t.me/remote/2)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("export is missing %q:\n%s", want, md)
		}
	}

	// The unmatched row gets no Matched line -- only the matched one does.
	if strings.Count(md, "**Matched:**") != 1 {
		t.Errorf("expected exactly one Matched line, got %d", strings.Count(md, "**Matched:**"))
	}
	// One separator after the header plus one per post.
	if got := strings.Count(md, "\n---\n"); got != 3 {
		t.Errorf("expected 3 separators, got %d", got)
	}
}

// an export with no posts still renders a valid, self-describing document
func TestRenderMarkdown_Empty(t *testing.T) {
	md := renderMarkdown("Twork Favorites", nil)
	if !strings.Contains(md, "# Twork Favorites") {
		t.Errorf("missing title:\n%s", md)
	}
	if !strings.Contains(md, "_0 post(s)") {
		t.Errorf("missing count:\n%s", md)
	}
}

// filenames are sanitized down to characters that are safe everywhere
func TestExportFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Twork Matches", "Twork_Matches.md"},
		{"Twork Favorites", "Twork_Favorites.md"},
		{"Twork Search: go AND remote", "Twork_Search__go_AND_remote.md"},
		{"../../etc/passwd", "______etc_passwd.md"},
		{"emoji 🔍 title", "emoji___title.md"}, // strings.Map works per rune, so the emoji becomes one underscore
		{"", ".md"},
		{"already_safe123", "already_safe123.md"},
	}
	for _, c := range cases {
		if got := exportFilename(c.in); got != c.want {
			t.Errorf("exportFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// a sanitized filename can never escape its directory or hide an extension
func TestExportFilename_IsAlwaysASafeLeafName(t *testing.T) {
	for _, in := range []string{"../escape", "a/b/c", `..\windows`, "with space", "dot.dot"} {
		got := exportFilename(in)
		if strings.ContainsAny(got[:len(got)-len(".md")], `/\.`) {
			t.Errorf("exportFilename(%q) = %q, which still contains a path or dot character", in, got)
		}
		if !strings.HasSuffix(got, ".md") {
			t.Errorf("exportFilename(%q) = %q, want a .md suffix", in, got)
		}
	}
}
