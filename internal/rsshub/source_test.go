package rsshub

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

// a minimal but valid RSS feed for one channel
func feedXML(title string, items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>` + title + `</title>
<link>https://t.me/s/golang_jobs</link>
` + strings.Join(items, "\n") + `
</channel></rss>`
}

func itemXML(guid, title, description, pubDate string) string {
	return fmt.Sprintf(`<item>
<guid>%s</guid>
<title>%s</title>
<description><![CDATA[%s]]></description>
<link>https://t.me/golang_jobs/1</link>
<pubDate>%s</pubDate>
</item>`, guid, title, description, pubDate)
}

// collects every message the source hands to its handler
type msgSink struct {
	mu   sync.Mutex
	msgs []models.Message
	live []bool
}

func (s *msgSink) handle(ctx context.Context, msg models.Message, live bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
	s.live = append(s.live, live)
	return nil
}

func (s *msgSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.msgs)
}

// builds a Source pointed at a test HTTP server, backed by a real temp store
func newTestSource(t *testing.T, handler http.HandlerFunc) (*Source, *storage.Store, *msgSink) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	store, err := storage.Open(filepath.Join(t.TempDir(), "rsshub.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	sink := &msgSink{}
	s := New(config.RSSHubConfig{
		BaseURL:             srv.URL + "/telegram/channel/{channel}",
		PollIntervalSeconds: 3600, // long enough that only the initial pass runs during a test
	}, store, sink.handle)

	return s, store, sink
}

// the poll interval falls back to 2 minutes when unset
func TestNew_DefaultPollInterval(t *testing.T) {
	s := New(config.RSSHubConfig{BaseURL: "http://x/{channel}"}, nil, nil)
	if s.pollInterval != 2*time.Minute {
		t.Errorf("poll interval = %v, want 2m", s.pollInterval)
	}

	s = New(config.RSSHubConfig{BaseURL: "http://x/{channel}", PollIntervalSeconds: 45}, nil, nil)
	if s.pollInterval != 45*time.Second {
		t.Errorf("poll interval = %v, want 45s", s.pollInterval)
	}
}

// AddByUsername validates the channel against RSSHub and persists it
func TestAddByUsername(t *testing.T) {
	s, store, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/golang_jobs") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, feedXML("Golang Jobs", itemXML("g1", "Post", "Go dev wanted", "Mon, 20 Jul 2026 09:00:00 GMT")))
	})
	ctx := context.Background()

	chat, err := s.AddByUsername(ctx, "@golang_jobs")
	if err != nil {
		t.Fatalf("AddByUsername: %v", err)
	}
	if chat.Username != "golang_jobs" {
		t.Errorf("username = %q, want golang_jobs (the @ should be stripped)", chat.Username)
	}
	if chat.Title != "Golang Jobs" {
		t.Errorf("title = %q, want the feed title", chat.Title)
	}
	if chat.Kind != models.ChatKindChannel {
		t.Errorf("kind = %q", chat.Kind)
	}
	if chat.TelegramID >= 0 {
		t.Errorf("synthetic chat id = %d, want a negative id so it can't collide with a real one", chat.TelegramID)
	}

	stored, err := store.GetChatByTelegramID(ctx, chat.TelegramID)
	if err != nil || stored == nil {
		t.Fatalf("expected the chat to be persisted, got %+v (%v)", stored, err)
	}
}

// an empty username is rejected before any HTTP request
func TestAddByUsername_Empty(t *testing.T) {
	s, _, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP request should be made for an empty username")
	})
	if _, err := s.AddByUsername(context.Background(), " @ "); err == nil {
		t.Error("expected an error for an empty username")
	}
}

// a channel RSSHub can't serve isn't added
func TestAddByUsername_UnreachableChannel(t *testing.T) {
	s, store, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ctx := context.Background()

	if _, err := s.AddByUsername(ctx, "ghost_channel"); err == nil {
		t.Fatal("expected an error when RSSHub returns a non-200")
	}
	chats, _ := store.ListChats(ctx)
	if len(chats) != 0 {
		t.Errorf("expected nothing to be persisted for a failed add, got %+v", chats)
	}
}

// a 200 that isn't a feed is a parse error, not a silent success
func TestAddByUsername_UnparseableFeed(t *testing.T) {
	s, _, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><body>not a feed</body></html>")
	})
	if _, err := s.AddByUsername(context.Background(), "golang_jobs"); err == nil {
		t.Error("expected a parse error for a non-feed response")
	}
}

// a feed with no title falls back to the username
func TestAddByUsername_UntitledFeedFallsBackToUsername(t *testing.T) {
	s, _, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title></title><link>x</link></channel></rss>`)
	})
	chat, err := s.AddByUsername(context.Background(), "golang_jobs")
	if err != nil {
		t.Fatalf("AddByUsername: %v", err)
	}
	if chat.Title != "golang_jobs" {
		t.Errorf("title = %q, want the username as a fallback", chat.Title)
	}
}

// invite links and folders have no RSSHub equivalent and say so clearly
func TestAddByInviteLinkAndFolder_Unsupported(t *testing.T) {
	s, _, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {})
	ctx := context.Background()

	chat, err := s.AddByInviteLink(ctx, "https://t.me/+abc")
	if err == nil {
		t.Fatal("expected AddByInviteLink to be unsupported")
	}
	if chat != nil {
		t.Errorf("expected no chat, got %+v", chat)
	}
	if !strings.Contains(err.Error(), "invite links") {
		t.Errorf("error should explain the limitation, got %v", err)
	}

	chats, err := s.AddFolder(ctx, "https://t.me/addlist/abc")
	if err == nil {
		t.Fatal("expected AddFolder to be unsupported")
	}
	if len(chats) != 0 {
		t.Errorf("expected no chats, got %+v", chats)
	}
}

// pause/resume/remove keep storage and the monitored set in sync
func TestPauseResumeRemove(t *testing.T) {
	s, store, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, feedXML("Golang Jobs"))
	})
	ctx := context.Background()

	chat, err := s.AddByUsername(ctx, "golang_jobs")
	if err != nil {
		t.Fatalf("AddByUsername: %v", err)
	}
	// Without a run context nothing polls, so register the chat by hand the
	// way Run would.
	s.monitored[chat.TelegramID] = &pollState{chat: *chat}

	if err := s.Pause(ctx, chat.TelegramID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if stored, _ := store.GetChatByTelegramID(ctx, chat.TelegramID); stored == nil || !stored.Paused {
		t.Error("expected the pause to be persisted")
	}

	if err := s.Resume(ctx, chat.TelegramID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if stored, _ := store.GetChatByTelegramID(ctx, chat.TelegramID); stored == nil || stored.Paused {
		t.Error("expected the resume to be persisted")
	}

	if got := s.ListResolved(); len(got) != 1 {
		t.Errorf("expected 1 monitored chat, got %d", len(got))
	}

	if err := s.Remove(ctx, chat.TelegramID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if stored, _ := store.GetChatByTelegramID(ctx, chat.TelegramID); stored != nil {
		t.Error("expected the chat to be removed from storage")
	}
	if got := s.ListResolved(); len(got) != 0 {
		t.Errorf("expected no monitored chats, got %+v", got)
	}
}

// resuming a chat the source doesn't know about is a no-op
func TestResume_UnknownChat(t *testing.T) {
	s, _, _ := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {})
	if err := s.Resume(context.Background(), 12345); err != nil {
		t.Errorf("Resume on an unknown chat: %v", err)
	}
}

// Run loads monitored chats from storage, skipping ones it can't poll
func TestRun_LoadsChatsFromStorage(t *testing.T) {
	s, store, sink := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, feedXML("Golang Jobs", itemXML("g1", "Post", "Go dev wanted", "Mon, 20 Jul 2026 09:00:00 GMT")))
	})
	ctx := context.Background()

	// One pollable chat, one paused, one with no username to poll by.
	if err := store.UpsertChat(ctx, models.Chat{TelegramID: -1, Kind: models.ChatKindChannel, Title: "Active", Username: "golang_jobs"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertChat(ctx, models.Chat{TelegramID: -2, Kind: models.ChatKindChannel, Title: "Paused", Username: "paused_jobs", Paused: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertChat(ctx, models.Chat{TelegramID: -3, Kind: models.ChatKindChannel, Title: "No username"}); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- s.Run(runCtx) }()

	// Wait for the active chat's initial (backfill) pass to deliver its item.
	waitFor(t, func() bool { return sink.count() >= 1 })

	cancel()
	if err := <-done; err != context.Canceled {
		t.Errorf("Run returned %v, want context.Canceled", err)
	}

	resolved := s.ListResolved()
	if len(resolved) != 2 {
		t.Fatalf("expected the active and paused chats to be tracked (not the one without a username), got %+v", resolved)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.live[0] {
		t.Error("expected a newly started chat's first pass to be treated as backfill, not live")
	}
	if sink.msgs[0].ChatTitle != "Active" {
		t.Errorf("expected only the active chat to be polled, got %q", sink.msgs[0].ChatTitle)
	}
}

// a feed item becomes a message with plain text and a stable synthetic ID
func TestNormalizeMessage(t *testing.T) {
	chat := models.Chat{TelegramID: -42, Title: "Golang Jobs"}
	published := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	item := &gofeed.Item{
		GUID:            "guid-1",
		Content:         "<b>Go dev</b><br>remote",
		Link:            "https://t.me/golang_jobs/7",
		PublishedParsed: &published,
	}

	got := normalizeMessage(item, chat)
	if got.ChatID != -42 || got.ChatTitle != "Golang Jobs" {
		t.Errorf("chat fields = %+v", got)
	}
	if got.Text != "Go dev\nremote" {
		t.Errorf("text = %q, want the HTML flattened", got.Text)
	}
	if got.Link != "https://t.me/golang_jobs/7" {
		t.Errorf("link = %q", got.Link)
	}
	if !got.Timestamp.Equal(published) {
		t.Errorf("timestamp = %v, want %v", got.Timestamp, published)
	}
	if got.TelegramMessageID != syntheticMessageID(item) {
		t.Errorf("message id = %d, want the synthetic id", got.TelegramMessageID)
	}
}

// Description stands in when Content is empty, and Updated when Published is
func TestNormalizeMessage_Fallbacks(t *testing.T) {
	chat := models.Chat{TelegramID: -1, Title: "Jobs"}
	updated := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)

	got := normalizeMessage(&gofeed.Item{
		GUID:          "g",
		Description:   "from description",
		UpdatedParsed: &updated,
	}, chat)
	if got.Text != "from description" {
		t.Errorf("text = %q, want the description as a fallback", got.Text)
	}
	if !got.Timestamp.Equal(updated) {
		t.Errorf("timestamp = %v, want the updated date as a fallback", got.Timestamp)
	}

	// With no date at all the message is stamped "now" rather than the epoch.
	before := time.Now().UTC().Add(-time.Second)
	got = normalizeMessage(&gofeed.Item{GUID: "g", Description: "x"}, chat)
	if got.Timestamp.Before(before) {
		t.Errorf("timestamp = %v, want roughly now", got.Timestamp)
	}
}

// chat IDs are stable, case-insensitive, and always negative
func TestSyntheticChatID(t *testing.T) {
	id := syntheticChatID("golang_jobs")
	if id != syntheticChatID("golang_jobs") {
		t.Error("expected the same username to always hash to the same id")
	}
	if id != syntheticChatID("GOLANG_JOBS") {
		t.Error("expected the hash to be case-insensitive")
	}
	if id >= 0 {
		t.Errorf("id = %d, want a negative id so it can't collide with a real Telegram channel id", id)
	}
	if id == syntheticChatID("rust_jobs") {
		t.Error("expected different usernames to hash differently")
	}
}

// message IDs prefer GUID, then link, then title, and are never zero
func TestSyntheticMessageID(t *testing.T) {
	guid := syntheticMessageID(&gofeed.Item{GUID: "g", Link: "l", Title: "t"})
	if guid != syntheticMessageID(&gofeed.Item{GUID: "g", Link: "other", Title: "other"}) {
		t.Error("expected the GUID to win over link and title")
	}

	link := syntheticMessageID(&gofeed.Item{Link: "l", Title: "t"})
	if link != syntheticMessageID(&gofeed.Item{Link: "l", Title: "other"}) {
		t.Error("expected the link to win over the title when there's no GUID")
	}

	title := syntheticMessageID(&gofeed.Item{Title: "t"})
	if title <= 0 {
		t.Errorf("title-derived id = %d, want a positive id", title)
	}

	// An item with nothing to hash still gets a usable id.
	if got := syntheticMessageID(&gofeed.Item{}); got <= 0 {
		t.Errorf("empty item id = %d, want a positive id", got)
	}
}

// ingest hands every item to the handler with the given live flag
func TestIngest(t *testing.T) {
	s, _, sink := newTestSource(t, func(w http.ResponseWriter, r *http.Request) {})

	feed := &gofeed.Feed{Items: []*gofeed.Item{
		{GUID: "a", Description: "first"},
		{GUID: "b", Description: "second"},
	}}
	s.ingest(context.Background(), models.Chat{TelegramID: -1, Title: "Jobs"}, feed, true)

	if sink.count() != 2 {
		t.Fatalf("expected both items to be handled, got %d", sink.count())
	}
	if !sink.live[0] || !sink.live[1] {
		t.Error("expected the live flag to be passed through")
	}
}

// a handler error is logged per item and doesn't abort the rest of the feed
func TestIngest_HandlerErrorDoesNotStopTheFeed(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "ingest.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	handled := 0
	s := New(config.RSSHubConfig{BaseURL: "http://x/{channel}"}, store, func(ctx context.Context, msg models.Message, live bool) error {
		handled++
		return fmt.Errorf("boom")
	})

	s.ingest(context.Background(), models.Chat{TelegramID: -1}, &gofeed.Feed{Items: []*gofeed.Item{
		{GUID: "a", Description: "first"},
		{GUID: "b", Description: "second"},
	}}, false)

	if handled != 2 {
		t.Errorf("expected both items to be attempted, got %d", handled)
	}
}

// polls until cond holds or the test times out
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the expected condition")
}
