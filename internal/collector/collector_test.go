package collector

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"

	"github.com/PeacexF/Twork/internal/models"
)

// a public channel becomes a monitorable channel-kind chat
func TestChatToResolved_Channel(t *testing.T) {
	rc := chatToResolved(&tg.Channel{
		ID:         777,
		AccessHash: 999,
		Title:      "Golang Jobs",
		Username:   "golang_jobs",
		Broadcast:  true,
	})
	if rc == nil {
		t.Fatal("expected a joined broadcast channel to resolve")
	}
	if rc.TelegramID != 777 || rc.AccessHash != 999 || rc.Title != "Golang Jobs" || rc.Username != "golang_jobs" {
		t.Errorf("chat fields not carried over: %+v", rc.Chat)
	}
	if rc.Kind != models.ChatKindChannel {
		t.Errorf("kind = %q, want %q", rc.Kind, models.ChatKindChannel)
	}
	peer, ok := rc.InputPeer.(*tg.InputPeerChannel)
	if !ok {
		t.Fatalf("InputPeer = %T, want *tg.InputPeerChannel", rc.InputPeer)
	}
	if peer.ChannelID != 777 || peer.AccessHash != 999 {
		t.Errorf("InputPeer = %+v", peer)
	}
}

// a non-broadcast channel (a supergroup) resolves as a group
func TestChatToResolved_Supergroup(t *testing.T) {
	rc := chatToResolved(&tg.Channel{ID: 1, AccessHash: 2, Title: "Devs", Broadcast: false})
	if rc == nil {
		t.Fatal("expected a supergroup to resolve")
	}
	if rc.Kind != models.ChatKindGroup {
		t.Errorf("kind = %q, want %q", rc.Kind, models.ChatKindGroup)
	}
}

// a legacy basic group resolves without an access hash
func TestChatToResolved_BasicGroup(t *testing.T) {
	rc := chatToResolved(&tg.Chat{ID: 55, Title: "Old Group"})
	if rc == nil {
		t.Fatal("expected a basic group to resolve")
	}
	if rc.Kind != models.ChatKindGroup || rc.TelegramID != 55 {
		t.Errorf("unexpected chat: %+v", rc.Chat)
	}
	if _, ok := rc.InputPeer.(*tg.InputPeerChat); !ok {
		t.Errorf("InputPeer = %T, want *tg.InputPeerChat", rc.InputPeer)
	}
}

// chats the account isn't in (or that are gone) aren't monitorable
func TestChatToResolved_Unmonitorable(t *testing.T) {
	cases := []struct {
		name string
		in   tg.ChatClass
	}{
		{"left channel", &tg.Channel{ID: 1, Left: true}},
		{"left group", &tg.Chat{ID: 2, Left: true}},
		{"deactivated group", &tg.Chat{ID: 3, Deactivated: true}},
		{"forbidden channel", &tg.ChannelForbidden{ID: 4}},
		{"forbidden group", &tg.ChatForbidden{ID: 5}},
		{"empty chat", &tg.ChatEmpty{ID: 6}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rc := chatToResolved(c.in); rc != nil {
				t.Errorf("expected nil, got %+v", rc.Chat)
			}
		})
	}
}

// a stored chat's access hash decides which InputPeer shape it needs
func TestInputPeerFor(t *testing.T) {
	channel := inputPeerFor(models.Chat{TelegramID: 10, AccessHash: 20})
	peer, ok := channel.(*tg.InputPeerChannel)
	if !ok {
		t.Fatalf("with an access hash: got %T, want *tg.InputPeerChannel", channel)
	}
	if peer.ChannelID != 10 || peer.AccessHash != 20 {
		t.Errorf("channel peer = %+v", peer)
	}

	basic := inputPeerFor(models.Chat{TelegramID: 30})
	chatPeer, ok := basic.(*tg.InputPeerChat)
	if !ok {
		t.Fatalf("without an access hash: got %T, want *tg.InputPeerChat", basic)
	}
	if chatPeer.ChatID != 30 {
		t.Errorf("chat peer = %+v", chatPeer)
	}
}

func TestPeerToInput(t *testing.T) {
	if p, ok := peerToInput(&tg.PeerChannel{ChannelID: 1}).(*tg.InputPeerChannel); !ok || p.ChannelID != 1 {
		t.Errorf("PeerChannel -> %T %+v", p, p)
	}
	if p, ok := peerToInput(&tg.PeerChat{ChatID: 2}).(*tg.InputPeerChat); !ok || p.ChatID != 2 {
		t.Errorf("PeerChat -> %T %+v", p, p)
	}
	if p, ok := peerToInput(&tg.PeerUser{UserID: 3}).(*tg.InputPeerUser); !ok || p.UserID != 3 {
		t.Errorf("PeerUser -> %T %+v", p, p)
	}
	if _, ok := peerToInput(nil).(*tg.InputPeerEmpty); !ok {
		t.Error("expected an unknown peer to map to InputPeerEmpty")
	}
}

func TestPeerChatID(t *testing.T) {
	cases := []struct {
		name string
		in   tg.PeerClass
		want int64
	}{
		{"channel", &tg.PeerChannel{ChannelID: 11}, 11},
		{"chat", &tg.PeerChat{ChatID: 22}, 22},
		{"user", &tg.PeerUser{UserID: 33}, 33},
		{"unknown", nil, 0},
	}
	for _, c := range cases {
		if got := peerChatID(c.in); got != c.want {
			t.Errorf("peerChatID(%s) = %d, want %d", c.name, got, c.want)
		}
	}
}

// public channels get a /username link, private ones the /c/<id> form
func TestMessageLink(t *testing.T) {
	withUsername := &resolvedChat{Chat: models.Chat{TelegramID: 42, Username: "golang_jobs"}}
	if got := messageLink(withUsername, 7); got != "https://t.me/golang_jobs/7" {
		t.Errorf("public link = %q", got)
	}

	private := &resolvedChat{Chat: models.Chat{TelegramID: 42}}
	if got := messageLink(private, 7); got != "https://t.me/c/42/7" {
		t.Errorf("private link = %q", got)
	}
}

// a raw Telegram message maps onto Twork's model, including sender/forward/edit
func TestNormalizeMessage(t *testing.T) {
	rc := &resolvedChat{Chat: models.Chat{TelegramID: 1001, Title: "Golang Jobs", Username: "golang_jobs"}}

	m := &tg.Message{
		ID:      88,
		Date:    1_700_000_000,
		Message: "Go backend developer wanted",
	}
	m.SetFromID(&tg.PeerUser{UserID: 500})
	m.SetEditDate(1_700_000_500)
	fwd := tg.MessageFwdHeader{}
	fwd.SetFromName("Original Poster")
	m.SetFwdFrom(fwd)

	got := normalizeMessage(m, rc)

	if got.TelegramMessageID != 88 || got.ChatID != 1001 || got.ChatTitle != "Golang Jobs" {
		t.Errorf("identity fields wrong: %+v", got)
	}
	if got.Text != "Go backend developer wanted" {
		t.Errorf("text = %q", got.Text)
	}
	if got.Link != "https://t.me/golang_jobs/88" {
		t.Errorf("link = %q", got.Link)
	}
	if !got.Timestamp.Equal(time.Unix(1_700_000_000, 0).UTC()) {
		t.Errorf("timestamp = %v", got.Timestamp)
	}
	if got.SenderID != 500 {
		t.Errorf("sender id = %d, want 500", got.SenderID)
	}
	if got.ForwardFromTitle != "Original Poster" {
		t.Errorf("forward title = %q", got.ForwardFromTitle)
	}
	if got.EditTimestamp == nil || !got.EditTimestamp.Equal(time.Unix(1_700_000_500, 0).UTC()) {
		t.Errorf("edit timestamp = %v", got.EditTimestamp)
	}
}

// a plain, never-edited, never-forwarded message leaves the optional fields empty
func TestNormalizeMessage_MinimalMessage(t *testing.T) {
	rc := &resolvedChat{Chat: models.Chat{TelegramID: 1001, Title: "Jobs"}}
	got := normalizeMessage(&tg.Message{ID: 1, Date: 1, Message: "hi"}, rc)

	if got.SenderID != 0 || got.ForwardFromTitle != "" || got.EditTimestamp != nil {
		t.Errorf("expected empty optional fields, got %+v", got)
	}
}

// a forward with only a post author falls back to it for the title
func TestNormalizeMessage_ForwardPostAuthorFallback(t *testing.T) {
	rc := &resolvedChat{Chat: models.Chat{TelegramID: 1, Title: "Jobs"}}
	m := &tg.Message{ID: 1, Date: 1, Message: "x"}
	fwd := tg.MessageFwdHeader{}
	fwd.SetPostAuthor("Channel Admin")
	m.SetFwdFrom(fwd)

	if got := normalizeMessage(m, rc); got.ForwardFromTitle != "Channel Admin" {
		t.Errorf("forward title = %q, want the post author", got.ForwardFromTitle)
	}
}

// an edit_date of 0 means "never edited", not "edited at the epoch"
func TestNormalizeMessage_ZeroEditDate(t *testing.T) {
	rc := &resolvedChat{Chat: models.Chat{TelegramID: 1, Title: "Jobs"}}
	m := &tg.Message{ID: 1, Date: 1, Message: "x"}
	m.SetEditDate(0)

	if got := normalizeMessage(m, rc); got.EditTimestamp != nil {
		t.Errorf("expected no edit timestamp for edit_date 0, got %v", got.EditTimestamp)
	}
}

func TestExtractMessages(t *testing.T) {
	one := []tg.MessageClass{&tg.Message{ID: 1}}

	cases := []struct {
		name string
		in   tg.MessagesMessagesClass
		want int
	}{
		{"messages", &tg.MessagesMessages{Messages: one}, 1},
		{"slice", &tg.MessagesMessagesSlice{Messages: one}, 1},
		{"channel messages", &tg.MessagesChannelMessages{Messages: one}, 1},
		{"not modified", &tg.MessagesMessagesNotModified{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractMessages(c.in); len(got) != c.want {
				t.Errorf("extractMessages = %d messages, want %d", len(got), c.want)
			}
		})
	}
}

// a full (non-sliced) dialogs page is the last page
func TestExtractDialogs_FullPageIsDone(t *testing.T) {
	dialogs, chats, msgs, done := extractDialogs(&tg.MessagesDialogs{
		Dialogs:  []tg.DialogClass{&tg.Dialog{}},
		Chats:    []tg.ChatClass{&tg.Channel{ID: 1}},
		Messages: []tg.MessageClass{&tg.Message{ID: 5, Date: 100, PeerID: &tg.PeerChannel{ChannelID: 1}}},
	})
	if !done {
		t.Error("expected a non-sliced dialogs response to end pagination")
	}
	if len(dialogs) != 1 || len(chats) != 1 {
		t.Errorf("dialogs=%d chats=%d, want 1 and 1", len(dialogs), len(chats))
	}
	if len(msgs) != 1 || msgs[0].id != 5 || msgs[0].date != 100 {
		t.Fatalf("pagination refs = %+v", msgs)
	}
	if _, ok := msgs[0].inputPeer.(*tg.InputPeerChannel); !ok {
		t.Errorf("inputPeer = %T, want *tg.InputPeerChannel", msgs[0].inputPeer)
	}
}

// a sliced response means there may be more pages
func TestExtractDialogs_SliceIsNotDone(t *testing.T) {
	_, chats, _, done := extractDialogs(&tg.MessagesDialogsSlice{
		Dialogs: []tg.DialogClass{&tg.Dialog{}},
		Chats:   []tg.ChatClass{&tg.Chat{ID: 2}},
	})
	if done {
		t.Error("expected a sliced dialogs response to continue pagination")
	}
	if len(chats) != 1 {
		t.Errorf("chats = %d, want 1", len(chats))
	}
}

// "not modified" ends pagination with nothing to process
func TestExtractDialogs_NotModified(t *testing.T) {
	dialogs, chats, msgs, done := extractDialogs(&tg.MessagesDialogsNotModified{})
	if !done {
		t.Error("expected NotModified to end pagination")
	}
	if len(dialogs) != 0 || len(chats) != 0 || len(msgs) != 0 {
		t.Error("expected NotModified to yield nothing")
	}
}

// non-*tg.Message entries are skipped when building pagination refs
func TestExtractDialogs_SkipsNonMessages(t *testing.T) {
	_, _, msgs, _ := extractDialogs(&tg.MessagesDialogs{
		Messages: []tg.MessageClass{&tg.MessageEmpty{ID: 1}, &tg.MessageService{ID: 2}},
	})
	if len(msgs) != 0 {
		t.Errorf("expected empty/service messages to be skipped, got %+v", msgs)
	}
}

func TestChatsFromUpdatesClass(t *testing.T) {
	chats := []tg.ChatClass{&tg.Channel{ID: 1}}

	if got := chatsFromUpdatesClass(&tg.Updates{Chats: chats}); len(got) != 1 {
		t.Errorf("Updates -> %d chats, want 1", len(got))
	}
	if got := chatsFromUpdatesClass(&tg.UpdatesCombined{Chats: chats}); len(got) != 1 {
		t.Errorf("UpdatesCombined -> %d chats, want 1", len(got))
	}
	if got := chatsFromUpdatesClass(&tg.UpdatesTooLong{}); got != nil {
		t.Errorf("expected nil for an unsupported updates type, got %+v", got)
	}
}

// every accepted spelling of a private invite link yields the bare hash
func TestParseInviteHash(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://t.me/+AbCdEf123", "AbCdEf123"},
		{"http://t.me/+AbCdEf123", "AbCdEf123"},
		{"t.me/+AbCdEf123", "AbCdEf123"},
		{"telegram.me/joinchat/AbCdEf123", "AbCdEf123"},
		{"https://t.me/joinchat/AbCdEf123", "AbCdEf123"},
		{"  https://t.me/+AbCdEf123  ", "AbCdEf123"},
		{"AbCdEf123", "AbCdEf123"}, // already a bare hash
		{"", ""},
		{"https://t.me/golang_jobs/123", ""}, // a public post link isn't an invite
	}
	for _, c := range cases {
		if got := parseInviteHash(c.in); got != c.want {
			t.Errorf("parseInviteHash(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// every accepted spelling of a folder link yields the bare slug
func TestParseChatlistSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://t.me/addlist/SomeSlug", "SomeSlug"},
		{"http://t.me/addlist/SomeSlug", "SomeSlug"},
		{"t.me/addlist/SomeSlug", "SomeSlug"},
		{"telegram.me/addlist/SomeSlug", "SomeSlug"},
		{"  https://t.me/addlist/SomeSlug  ", "SomeSlug"},
		{"SomeSlug", "SomeSlug"}, // already a bare slug
		{"", ""},
		{"https://t.me/golang_jobs/123", ""},
	}
	for _, c := range cases {
		if got := parseChatlistSlug(c.in); got != c.want {
			t.Errorf("parseChatlistSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// New copies config into the collector and starts with an empty resolved set
func TestNew(t *testing.T) {
	c := New(configFixture(), nil, nil, nil)
	if c.appID != 12345 || c.appHash != "hash" || c.phone != "+10000000000" || c.session != "s.session" {
		t.Errorf("config not carried over: %+v", c)
	}
	if c.resolved == nil {
		t.Error("expected the resolved map to be initialized")
	}
	if got := c.ListResolved(); len(got) != 0 {
		t.Errorf("expected no resolved chats initially, got %+v", got)
	}
}

// ListResolved snapshots what's currently monitored
func TestListResolved(t *testing.T) {
	c := New(configFixture(), nil, nil, nil)
	c.resolved[1] = &resolvedChat{Chat: models.Chat{TelegramID: 1, Title: "A"}}
	c.resolved[2] = &resolvedChat{Chat: models.Chat{TelegramID: 2, Title: "B"}}

	got := c.ListResolved()
	if len(got) != 2 {
		t.Fatalf("expected 2 chats, got %d", len(got))
	}

	// The snapshot is a copy: mutating it doesn't affect the collector.
	got[0].Title = "mutated"
	for _, rc := range c.resolved {
		if rc.Title == "mutated" {
			t.Error("ListResolved leaked a reference to the collector's own state")
		}
	}
}

// backfillAsync is a no-op before Run has established a context
func TestBackfillAsync_NoRunContext(t *testing.T) {
	c := New(configFixture(), nil, nil, nil)
	c.backfillAsync(&resolvedChat{Chat: models.Chat{TelegramID: 1, Title: "A"}}) // must not panic
}
