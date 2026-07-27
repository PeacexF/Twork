package bot

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/storage"
)

func sampleRow() storage.MatchRow {
	return storage.MatchRow{
		MessageID:       7,
		ChatTitle:       "Golang Jobs",
		Text:            "Backend Go developer, remote",
		Link:            "https://t.me/golang_jobs/7",
		Timestamp:       time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		MatchedKeywords: []string{"Go", "Remote"},
	}
}

// each view labels its items differently but renders the same fields
func TestCarouselText(t *testing.T) {
	cases := []struct {
		view      viewKind
		wantTitle string
	}{
		{viewMatches, "📋 Match"},
		{viewFavorites, "⭐ Favorite"},
		{viewSearch, "🔍 Result"},
	}
	for _, c := range cases {
		got := carouselText(c.view, sampleRow())
		if !strings.HasPrefix(got, c.wantTitle) {
			t.Errorf("view %q text starts %q, want prefix %q", c.view, got, c.wantTitle)
		}
		for _, want := range []string{"Golang Jobs", "Backend Go developer, remote", "✓ Go", "✓ Remote", "2026-07-25 12:00 UTC"} {
			if !strings.Contains(got, want) {
				t.Errorf("view %q text is missing %q:\n%s", c.view, want, got)
			}
		}
	}
}

// a post with no matched keywords (a search hit) omits the Matched block
func TestCarouselText_NoMatchedKeywords(t *testing.T) {
	row := sampleRow()
	row.MatchedKeywords = nil

	got := carouselText(viewSearch, row)
	if strings.Contains(got, "Matched:") {
		t.Errorf("expected no Matched block for an unmatched post:\n%s", got)
	}
	if !strings.Contains(got, "Backend Go developer, remote") {
		t.Errorf("expected the post body, got:\n%s", got)
	}
}

// long posts are truncated so the rendered message stays inside Telegram's limit
func TestCarouselText_TruncatesLongPosts(t *testing.T) {
	row := sampleRow()
	row.Text = strings.Repeat("x", maxSnippetLen*2)

	got := carouselText(viewMatches, row)
	if !strings.Contains(got, "…") {
		t.Error("expected an ellipsis on a truncated post")
	}
	if strings.Count(got, "x") != maxSnippetLen {
		t.Errorf("expected exactly %d body characters, got %d", maxSnippetLen, strings.Count(got, "x"))
	}
}

// a post of exactly the snippet limit isn't truncated
func TestCarouselText_ExactLimitIsNotTruncated(t *testing.T) {
	row := sampleRow()
	row.Text = strings.Repeat("x", maxSnippetLen)

	if strings.Contains(carouselText(viewMatches, row), "…") {
		t.Error("expected a post at exactly the limit to be left alone")
	}
}

// every view has its own empty-state message
func TestViewEmptyText(t *testing.T) {
	cases := []struct {
		view viewKind
		want string
	}{
		{viewMatches, "No matches yet"},
		{viewFavorites, "No favorites yet"},
		{viewSearch, "No results"},
		{viewNone, "Nothing to show"},
	}
	for _, c := range cases {
		if got := viewEmptyText(c.view); !strings.Contains(got, c.want) {
			t.Errorf("viewEmptyText(%q) = %q, want it to mention %q", c.view, got, c.want)
		}
	}
}

// the carousel keyboard wires save/open/export/back to the item on screen
func TestCarouselKeyboard(t *testing.T) {
	kb := carouselKeyboard(sampleRow(), 1, 5)

	var callbacks []string
	var urls []string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil {
				callbacks = append(callbacks, *btn.CallbackData)
			}
			if btn.URL != nil {
				urls = append(urls, *btn.URL)
			}
		}
	}

	for _, want := range []string{"list:page:0", "list:page:2", "list:bookmark:7", "list:export", "menu:home"} {
		if !slices.Contains(callbacks, want) {
			t.Errorf("keyboard is missing callback %q; got %+v", want, callbacks)
		}
	}
	if len(urls) != 1 || urls[0] != "https://t.me/golang_jobs/7" {
		t.Errorf("expected exactly one Open link to the original post, got %+v", urls)
	}
}

// the save button reflects whether the post is already bookmarked
func TestCarouselKeyboard_SaveLabelReflectsState(t *testing.T) {
	unsaved := carouselKeyboard(sampleRow(), 0, 1)
	if got := unsaved.InlineKeyboard[1][0].Text; got != "☆ Save" {
		t.Errorf("unsaved label = %q, want \"☆ Save\"", got)
	}

	row := sampleRow()
	row.Bookmarked = true
	saved := carouselKeyboard(row, 0, 1)
	if got := saved.InlineKeyboard[1][0].Text; got != "⭐ Saved" {
		t.Errorf("saved label = %q, want \"⭐ Saved\"", got)
	}
}
