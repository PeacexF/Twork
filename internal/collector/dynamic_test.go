package collector

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

// a reusable Telegram config fixture
func configFixture() config.TelegramConfig {
	return config.TelegramConfig{
		AppID:   12345,
		AppHash: "hash",
		Phone:   "+10000000000",
		Session: "s.session",
	}
}

// builds a collector backed by a real temp-file store, with a message sink
func newTestCollector(t *testing.T) (*Collector, *storage.Store, *[]models.Message) {
	t.Helper()

	store, err := storage.Open(filepath.Join(t.TempDir(), "collector.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	var seen []models.Message
	c := New(configFixture(), store, nil, func(ctx context.Context, msg models.Message, live bool) error {
		seen = append(seen, msg)
		return nil
	})
	return c, store, &seen
}

// addResolved persists the chat and registers it for monitoring
func TestAddResolved(t *testing.T) {
	c, store, _ := newTestCollector(t)
	ctx := context.Background()

	rc := chatToResolved(&tg.Channel{ID: 900, AccessHash: 5, Title: "Go Jobs", Username: "go_jobs", Broadcast: true})
	chat, err := c.addResolved(ctx, rc)
	if err != nil {
		t.Fatalf("addResolved: %v", err)
	}
	if chat.TelegramID != 900 {
		t.Errorf("returned chat = %+v", chat)
	}

	stored, err := store.GetChatByTelegramID(ctx, 900)
	if err != nil || stored == nil {
		t.Fatalf("expected the chat to be persisted, got %+v (%v)", stored, err)
	}
	if len(c.ListResolved()) != 1 {
		t.Error("expected the chat to be registered for monitoring")
	}
}

// pause/resume update both storage and the in-memory monitoring state
func TestPauseResumeRemove(t *testing.T) {
	c, store, _ := newTestCollector(t)
	ctx := context.Background()

	rc := chatToResolved(&tg.Channel{ID: 901, AccessHash: 5, Title: "Go Jobs", Broadcast: true})
	if _, err := c.addResolved(ctx, rc); err != nil {
		t.Fatalf("addResolved: %v", err)
	}

	if err := c.Pause(ctx, 901); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if stored, _ := store.GetChatByTelegramID(ctx, 901); stored == nil || !stored.Paused {
		t.Error("expected the pause to be persisted")
	}
	if !c.resolved[901].Paused {
		t.Error("expected the in-memory chat to be paused")
	}

	if err := c.Resume(ctx, 901); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if stored, _ := store.GetChatByTelegramID(ctx, 901); stored == nil || stored.Paused {
		t.Error("expected the resume to be persisted")
	}
	if c.resolved[901].Paused {
		t.Error("expected the in-memory chat to be unpaused")
	}

	if err := c.Remove(ctx, 901); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if stored, _ := store.GetChatByTelegramID(ctx, 901); stored != nil {
		t.Error("expected the chat to be removed from storage")
	}
	if len(c.ListResolved()) != 0 {
		t.Error("expected the chat to stop being monitored")
	}
}

// pausing or resuming a chat that isn't monitored doesn't panic
func TestPauseResume_UnknownChat(t *testing.T) {
	c, _, _ := newTestCollector(t)
	ctx := context.Background()

	if err := c.Pause(ctx, 4242); err != nil {
		t.Errorf("Pause on an unknown chat: %v", err)
	}
	if err := c.Resume(ctx, 4242); err != nil {
		t.Errorf("Resume on an unknown chat: %v", err)
	}
}

// AddByUsername rejects empty input before touching the network
func TestAddByUsername_EmptyUsername(t *testing.T) {
	c, _, _ := newTestCollector(t)
	if _, err := c.AddByUsername(context.Background(), "  @  "); err == nil {
		t.Error("expected an error for an empty username")
	}
}

// links with no invite hash / folder slug are rejected before any API call
func TestAddByInviteLinkAndFolder_RejectMalformedLinks(t *testing.T) {
	c, _, _ := newTestCollector(t)
	ctx := context.Background()

	if _, err := c.AddByInviteLink(ctx, "https://t.me/golang_jobs/12"); err == nil {
		t.Error("expected an error for a link with no invite hash")
	}
	if _, err := c.AddFolder(ctx, "https://t.me/golang_jobs/12"); err == nil {
		t.Error("expected an error for a link with no folder slug")
	}
}

// resolveChats loads what's already in storage instead of hitting the network
func TestResolveChats_LoadsFromStorage(t *testing.T) {
	c, store, _ := newTestCollector(t)
	ctx := context.Background()

	if err := store.UpsertChat(ctx, models.Chat{
		TelegramID: 700, AccessHash: 8, Kind: models.ChatKindChannel, Title: "Stored", Username: "stored",
	}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	if err := c.resolveChats(ctx); err != nil {
		t.Fatalf("resolveChats: %v", err)
	}
	rc, ok := c.resolved[700]
	if !ok {
		t.Fatal("expected the stored chat to be monitored")
	}
	if rc.Title != "Stored" {
		t.Errorf("chat = %+v", rc.Chat)
	}
	if _, ok := rc.InputPeer.(*tg.InputPeerChannel); !ok {
		t.Errorf("InputPeer = %T, want *tg.InputPeerChannel", rc.InputPeer)
	}
}

// with an empty database and no seed chats there's nothing to resolve
func TestResolveChats_NoStoredChatsNoSeed(t *testing.T) {
	c, _, _ := newTestCollector(t)
	if err := c.resolveChats(context.Background()); err != nil {
		t.Fatalf("resolveChats: %v", err)
	}
	if len(c.ListResolved()) != 0 {
		t.Error("expected nothing to be resolved")
	}
}

// live updates are forwarded only for monitored, unpaused chats
func TestHandleUpdates(t *testing.T) {
	c, _, seen := newTestCollector(t)
	ctx := context.Background()

	c.resolved[10] = &resolvedChat{Chat: models.Chat{TelegramID: 10, Title: "Watched", Username: "watched"}}
	c.resolved[11] = &resolvedChat{Chat: models.Chat{TelegramID: 11, Title: "Paused", Paused: true}}

	newMsg := func(chatID int64, id int, text string) tg.UpdateClass {
		return &tg.UpdateNewChannelMessage{Message: &tg.Message{
			ID: id, Date: 1, Message: text, PeerID: &tg.PeerChannel{ChannelID: chatID},
		}}
	}

	err := c.handleUpdates(ctx, &tg.Updates{Updates: []tg.UpdateClass{
		newMsg(10, 1, "watched post"),
		newMsg(11, 2, "paused post"),
		newMsg(12, 3, "unmonitored post"),
		&tg.UpdateUserTyping{},                                 // not a message
		&tg.UpdateNewMessage{Message: &tg.MessageEmpty{ID: 4}}, // not a *tg.Message
	}})
	if err != nil {
		t.Fatalf("handleUpdates: %v", err)
	}

	if len(*seen) != 1 {
		t.Fatalf("expected only the watched chat's post, got %d: %+v", len(*seen), *seen)
	}
	got := (*seen)[0]
	if got.Text != "watched post" || got.ChatTitle != "Watched" {
		t.Errorf("forwarded message = %+v", got)
	}
	if got.Link != "https://t.me/watched/1" {
		t.Errorf("link = %q", got.Link)
	}
}

// UpdateShort wraps a single update; unsupported envelopes are ignored
func TestHandleUpdates_Envelopes(t *testing.T) {
	c, _, seen := newTestCollector(t)
	ctx := context.Background()
	c.resolved[10] = &resolvedChat{Chat: models.Chat{TelegramID: 10, Title: "Watched"}}

	short := &tg.UpdateShort{Update: &tg.UpdateNewChannelMessage{Message: &tg.Message{
		ID: 1, Date: 1, Message: "short", PeerID: &tg.PeerChannel{ChannelID: 10},
	}}}
	if err := c.handleUpdates(ctx, short); err != nil {
		t.Fatalf("handleUpdates(UpdateShort): %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("expected UpdateShort to be unwrapped, got %d messages", len(*seen))
	}

	combined := &tg.UpdatesCombined{Updates: []tg.UpdateClass{
		&tg.UpdateNewChannelMessage{Message: &tg.Message{
			ID: 2, Date: 1, Message: "combined", PeerID: &tg.PeerChannel{ChannelID: 10},
		}},
	}}
	if err := c.handleUpdates(ctx, combined); err != nil {
		t.Fatalf("handleUpdates(UpdatesCombined): %v", err)
	}
	if len(*seen) != 2 {
		t.Fatalf("expected UpdatesCombined to be handled, got %d messages", len(*seen))
	}

	if err := c.handleUpdates(ctx, &tg.UpdatesTooLong{}); err != nil {
		t.Errorf("expected an unsupported envelope to be ignored, got %v", err)
	}
	if len(*seen) != 2 {
		t.Errorf("expected no extra messages, got %d", len(*seen))
	}
}

// SendText refuses to address a chat the collector hasn't resolved --
// notably, this is also what keeps it from ever reaching a DM: chatToResolved
// never converts a bare user peer into a resolved entry in the first place.
func TestSendText_UnresolvedChat(t *testing.T) {
	c, _, _ := newTestCollector(t)
	if err := c.SendText(context.Background(), 12345, "hi"); err == nil {
		t.Error("expected an error for an unmonitored chat, got nil")
	}
}
