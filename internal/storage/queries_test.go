package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PeacexF/Twork/internal/models"
)

// inserts n distinct messages, newest last, and returns their storage IDs
func seedMessages(t *testing.T, s *Store, n int) []int64 {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	ids := make([]int64, 0, n)
	for i := range n {
		id, err := s.InsertMessage(ctx, models.Message{
			TelegramMessageID: 1000 + i,
			ChatID:            1001,
			ChatTitle:         "Golang Jobs",
			Timestamp:         base.Add(time.Duration(i) * time.Hour),
			Text:              fmt.Sprintf("Backend vacancy number %d, Go and Docker", i),
			Link:              fmt.Sprintf("https://t.me/golang_jobs/%d", 1000+i),
		})
		if err != nil {
			t.Fatalf("seeding message %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// matches come back newest first, one page at a time, with the total alongside
func TestListMatches_PagesNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := seedMessages(t, s, 3)
	for _, id := range ids {
		if err := s.RecordMatch(ctx, id, `["Go"]`); err != nil {
			t.Fatalf("RecordMatch: %v", err)
		}
	}

	rows, total, err := s.ListMatches(ctx, 1, 0)
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(rows) != 1 {
		t.Fatalf("expected a single-row page, got %d", len(rows))
	}
	if rows[0].MessageID != ids[2] {
		t.Errorf("expected the newest message first, got id %d", rows[0].MessageID)
	}
	if len(rows[0].MatchedKeywords) != 1 || rows[0].MatchedKeywords[0] != "Go" {
		t.Errorf("matched keywords not decoded: %+v", rows[0].MatchedKeywords)
	}
	if rows[0].Bookmarked {
		t.Error("expected a fresh match to be unbookmarked")
	}

	// Offset walks backwards through time.
	rows, _, err = s.ListMatches(ctx, 1, 2)
	if err != nil {
		t.Fatalf("ListMatches(offset=2): %v", err)
	}
	if len(rows) != 1 || rows[0].MessageID != ids[0] {
		t.Errorf("expected the oldest match at offset 2, got %+v", rows)
	}

	// Past the end is an empty page, not an error.
	rows, total, err = s.ListMatches(ctx, 1, 99)
	if err != nil {
		t.Fatalf("ListMatches(offset=99): %v", err)
	}
	if len(rows) != 0 || total != 3 {
		t.Errorf("expected an empty page with total 3, got %d rows total %d", len(rows), total)
	}
}

// a non-positive limit falls back to the default page size
func TestPagedMatchQuery_DefaultsLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := seedMessages(t, s, 12)
	for _, id := range ids {
		if err := s.RecordMatch(ctx, id, `["Go"]`); err != nil {
			t.Fatalf("RecordMatch: %v", err)
		}
	}

	rows, total, err := s.ListMatches(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if total != 12 {
		t.Errorf("total = %d, want 12", total)
	}
	if len(rows) != 10 {
		t.Errorf("expected limit<=0 to fall back to 10 rows, got %d", len(rows))
	}
}

// bookmarking flips state and moves the message in and out of Favorites
func TestToggleBookmarkAndListBookmarked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := seedMessages(t, s, 2)

	rows, total, err := s.ListBookmarked(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListBookmarked: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Fatalf("expected no favorites initially, got %d (total %d)", len(rows), total)
	}

	on, err := s.ToggleBookmark(ctx, ids[0])
	if err != nil {
		t.Fatalf("ToggleBookmark: %v", err)
	}
	if !on {
		t.Fatal("expected the first toggle to bookmark the message")
	}

	rows, total, err = s.ListBookmarked(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListBookmarked: %v", err)
	}
	if len(rows) != 1 || total != 1 {
		t.Fatalf("expected 1 favorite, got %d (total %d)", len(rows), total)
	}
	if rows[0].MessageID != ids[0] || !rows[0].Bookmarked {
		t.Errorf("unexpected favorite row: %+v", rows[0])
	}
	// An unmatched message still lists, with an empty keyword set.
	if len(rows[0].MatchedKeywords) != 0 {
		t.Errorf("expected no matched keywords for an unmatched favorite, got %+v", rows[0].MatchedKeywords)
	}

	off, err := s.ToggleBookmark(ctx, ids[0])
	if err != nil {
		t.Fatalf("second ToggleBookmark: %v", err)
	}
	if off {
		t.Error("expected the second toggle to un-bookmark the message")
	}
	_, total, _ = s.ListBookmarked(ctx, 10, 0)
	if total != 0 {
		t.Errorf("expected the message to leave Favorites, total = %d", total)
	}

	// The stats counter tracks the same flag.
	if _, err := s.ToggleBookmark(ctx, ids[1]); err != nil {
		t.Fatalf("ToggleBookmark: %v", err)
	}
	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.Bookmarks != 1 {
		t.Errorf("stats.Bookmarks = %d, want 1", stats.Bookmarks)
	}
}

// GetMatchRow returns a single message with its match/bookmark state
func TestGetMatchRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := seedMessages(t, s, 1)
	if err := s.RecordMatch(ctx, ids[0], `["Go","Docker"]`); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}
	if _, err := s.ToggleBookmark(ctx, ids[0]); err != nil {
		t.Fatalf("ToggleBookmark: %v", err)
	}

	row, err := s.GetMatchRow(ctx, ids[0])
	if err != nil {
		t.Fatalf("GetMatchRow: %v", err)
	}
	if row.ChatTitle != "Golang Jobs" || !row.Bookmarked {
		t.Errorf("unexpected row: %+v", row)
	}
	if len(row.MatchedKeywords) != 2 {
		t.Errorf("expected 2 matched keywords, got %+v", row.MatchedKeywords)
	}

	if _, err := s.GetMatchRow(ctx, 99999); err == nil {
		t.Error("expected an error for a missing message id")
	}
}

// re-recording a match for the same message overwrites its keyword list
func TestRecordMatch_OverwritesKeywords(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := seedMessages(t, s, 1)
	if err := s.RecordMatch(ctx, ids[0], `["Go"]`); err != nil {
		t.Fatalf("first RecordMatch: %v", err)
	}
	if err := s.RecordMatch(ctx, ids[0], `["Rust","Docker"]`); err != nil {
		t.Fatalf("second RecordMatch: %v", err)
	}

	_, total, err := s.ListMatches(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected the match to be updated, not duplicated; total = %d", total)
	}
	row, err := s.GetMatchRow(ctx, ids[0])
	if err != nil {
		t.Fatalf("GetMatchRow: %v", err)
	}
	if len(row.MatchedKeywords) != 2 || row.MatchedKeywords[0] != "Rust" {
		t.Errorf("expected the keyword list to be replaced, got %+v", row.MatchedKeywords)
	}
}

// the digest queries only count what arrived inside their window
func TestDigestCountsAndMatchesSince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := seedMessages(t, s, 3)
	for _, id := range ids {
		if err := s.RecordMatch(ctx, id, `["Go"]`); err != nil {
			t.Fatalf("RecordMatch: %v", err)
		}
	}

	// created_at is set by SQLite at insert time, so a window that starts in
	// the past includes everything and one that starts in the future includes
	// nothing.
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	if n, err := s.CountMessagesSince(ctx, past); err != nil || n != 3 {
		t.Errorf("CountMessagesSince(past) = %d, %v; want 3, nil", n, err)
	}
	if n, err := s.CountMessagesSince(ctx, future); err != nil || n != 0 {
		t.Errorf("CountMessagesSince(future) = %d, %v; want 0, nil", n, err)
	}
	if n, err := s.CountMatchesSince(ctx, past); err != nil || n != 3 {
		t.Errorf("CountMatchesSince(past) = %d, %v; want 3, nil", n, err)
	}
	if n, err := s.CountMatchesSince(ctx, future); err != nil || n != 0 {
		t.Errorf("CountMatchesSince(future) = %d, %v; want 0, nil", n, err)
	}

	rows, err := s.MatchesSince(ctx, past, 2)
	if err != nil {
		t.Fatalf("MatchesSince: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected the limit to cap the digest at 2 rows, got %d", len(rows))
	}
	if rows[0].MessageID != ids[2] {
		t.Errorf("expected newest first, got id %d", rows[0].MessageID)
	}

	// A non-positive limit falls back to the 100-row default rather than
	// returning nothing.
	rows, err = s.MatchesSince(ctx, past, 0)
	if err != nil {
		t.Fatalf("MatchesSince(limit=0): %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected limit<=0 to default to 100, got %d rows", len(rows))
	}

	rows, err = s.MatchesSince(ctx, future, 10)
	if err != nil {
		t.Fatalf("MatchesSince(future): %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected no matches inside a future window, got %d", len(rows))
	}
}

// search pages results and reports the full total for the query
func TestSearchPaged_Paginates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedMessages(t, s, 5)

	rows, total, err := s.SearchPaged(ctx, "vacancy", 2, 0)
	if err != nil {
		t.Fatalf("SearchPaged: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(rows) != 2 {
		t.Errorf("expected a 2-row page, got %d", len(rows))
	}

	rows, _, err = s.SearchPaged(ctx, "vacancy", 2, 4)
	if err != nil {
		t.Fatalf("SearchPaged(offset=4): %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 row on the last page, got %d", len(rows))
	}
}

// malformed FTS5 syntax surfaces as an error rather than a panic
func TestSearchPaged_InvalidQuery(t *testing.T) {
	s := newTestStore(t)
	seedMessages(t, s, 1)

	if _, _, err := s.SearchPaged(context.Background(), `"unbalanced AND (`, 10, 0); err == nil {
		t.Error("expected malformed FTS5 syntax to return an error")
	}
}

// MaxTelegramMessageID drives incremental backfill: 0 when nothing is stored
func TestMaxTelegramMessageID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if got, err := s.MaxTelegramMessageID(ctx, 1001); err != nil || got != 0 {
		t.Fatalf("MaxTelegramMessageID on an empty chat = %d, %v; want 0, nil", got, err)
	}

	seedMessages(t, s, 3) // telegram ids 1000..1002 in chat 1001

	got, err := s.MaxTelegramMessageID(ctx, 1001)
	if err != nil {
		t.Fatalf("MaxTelegramMessageID: %v", err)
	}
	if got != 1002 {
		t.Errorf("MaxTelegramMessageID = %d, want 1002", got)
	}

	// Scoped per chat: another chat's history doesn't leak in.
	if got, err := s.MaxTelegramMessageID(ctx, 2002); err != nil || got != 0 {
		t.Errorf("MaxTelegramMessageID for another chat = %d, %v; want 0, nil", got, err)
	}
}

// deleting a message cascades to its match and status rows
func TestDeleteMessage_CascadesToMatchAndStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids := seedMessages(t, s, 1)
	if err := s.RecordMatch(ctx, ids[0], `["Go"]`); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}
	if _, err := s.ToggleBookmark(ctx, ids[0]); err != nil {
		t.Fatalf("ToggleBookmark: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, ids[0]); err != nil {
		t.Fatalf("deleting message: %v", err)
	}

	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.MessagesIndexed != 0 || stats.Bookmarks != 0 || stats.TodayMatches != 0 {
		t.Errorf("expected the delete to cascade, got %+v", stats)
	}
}

// GetStats counts only unpaused chats and today's matches
func TestGetStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChat(ctx, sampleChat(1001, "golang_jobs")); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	paused := sampleChat(1002, "paused_jobs")
	paused.Paused = true
	if err := s.UpsertChat(ctx, paused); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	ids := seedMessages(t, s, 2)
	if err := s.RecordMatch(ctx, ids[0], `["Go"]`); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}

	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.ChatsMonitored != 1 {
		t.Errorf("ChatsMonitored = %d, want 1 (paused chats excluded)", stats.ChatsMonitored)
	}
	if stats.MessagesIndexed != 2 {
		t.Errorf("MessagesIndexed = %d, want 2", stats.MessagesIndexed)
	}
	if stats.TodayMatches != 1 {
		t.Errorf("TodayMatches = %d, want 1", stats.TodayMatches)
	}
	if stats.Bookmarks != 0 || stats.Ignored != 0 {
		t.Errorf("expected no bookmarks/ignored, got %+v", stats)
	}
}
