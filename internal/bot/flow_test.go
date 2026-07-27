package bot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

// a ChatSource that records what the bot asked of it
type fakeSource struct {
	chats []models.Chat

	addUsernameCalled string
	addInviteCalled   string
	addFolderCalled   string
	paused, resumed   []int64
	removed           []int64

	addErr error
}

func (f *fakeSource) Run(ctx context.Context) error { return nil }

func (f *fakeSource) AddByUsername(ctx context.Context, username string) (*models.Chat, error) {
	f.addUsernameCalled = username
	if f.addErr != nil {
		return nil, f.addErr
	}
	c := models.Chat{TelegramID: 900, Kind: models.ChatKindChannel, Title: "Added", Username: username}
	f.chats = append(f.chats, c)
	return &c, nil
}

func (f *fakeSource) AddByInviteLink(ctx context.Context, link string) (*models.Chat, error) {
	f.addInviteCalled = link
	if f.addErr != nil {
		return nil, f.addErr
	}
	c := models.Chat{TelegramID: 901, Kind: models.ChatKindGroup, Title: "Private"}
	f.chats = append(f.chats, c)
	return &c, nil
}

func (f *fakeSource) AddFolder(ctx context.Context, link string) ([]*models.Chat, error) {
	f.addFolderCalled = link
	if f.addErr != nil {
		return nil, f.addErr
	}
	c := models.Chat{TelegramID: 902, Kind: models.ChatKindChannel, Title: "From folder"}
	f.chats = append(f.chats, c)
	return []*models.Chat{&c}, nil
}

func (f *fakeSource) Pause(ctx context.Context, id int64) error {
	f.paused = append(f.paused, id)
	return nil
}

func (f *fakeSource) Resume(ctx context.Context, id int64) error {
	f.resumed = append(f.resumed, id)
	return nil
}

func (f *fakeSource) Remove(ctx context.Context, id int64) error {
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeSource) ListResolved() []models.Chat { return f.chats }

// a callback query from the owner
func ownerCallback(data string) *tgbotapi.CallbackQuery {
	return &tgbotapi.CallbackQuery{
		ID:      "cb-1",
		From:    &tgbotapi.User{ID: 500},
		Data:    data,
		Message: &tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: 500}},
	}
}

// a plain text message from the owner
func ownerMessage(text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID: 9,
		From:      &tgbotapi.User{ID: 500},
		Chat:      &tgbotapi.Chat{ID: 500},
		Text:      text,
	}
}

// every top-level menu button renders its screen
func TestHandleCallback_MenuRouting(t *testing.T) {
	b, fake := newTestBot(t)
	b.source = &fakeSource{}
	ctx := context.Background()

	cases := []struct {
		data string
		want string
	}{
		{"menu:home", "TWORK"},
		{"menu:chats", "Monitored chats"},
		{"menu:matches", "No matches yet"},
		{"menu:favorites", "No favorites yet"},
		{"menu:keywords", "Keyword groups"},
		{"menu:settings", "Settings"},
	}
	for _, c := range cases {
		t.Run(c.data, func(t *testing.T) {
			fake.reset()
			b.handleCallback(ctx, ownerCallback(c.data))
			fake.assertScreenContains(t, "editMessageText", c.want)
		})
	}
}

// the Search button asks for a query instead of rendering a screen
func TestHandleCallback_SearchPrompts(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	b.handleCallback(ctx, ownerCallback("menu:search"))

	if b.sess.pending != inputSearchQuery {
		t.Errorf("pending input = %q, want %q", b.sess.pending, inputSearchQuery)
	}
	fake.assertScreenContains(t, "sendMessage", "search query")
}

// a non-owner gets acknowledged but nothing is rendered or changed
func TestHandleCallback_RejectsNonOwner(t *testing.T) {
	b, fake := newTestBot(t)

	cq := ownerCallback("menu:settings")
	cq.From = &tgbotapi.User{ID: 999}
	b.handleCallback(context.Background(), cq)

	if len(fake.callsTo("editMessageText")) != 0 {
		t.Error("expected no screen to be rendered for a non-owner")
	}
	if len(fake.callsTo("answerCallbackQuery")) != 1 {
		t.Errorf("expected the callback to still be acknowledged, got %+v", fake.methodNames())
	}
}

// unknown callback data is ignored rather than crashing the dispatch loop
func TestHandleCallback_UnknownData(t *testing.T) {
	b, fake := newTestBot(t)
	b.source = &fakeSource{}

	b.handleCallback(context.Background(), ownerCallback("noop"))

	if len(fake.callsTo("editMessageText")) != 0 {
		t.Error("expected unknown callback data to render nothing")
	}
}

// /start claims ownership on first use and opens the menu
func TestHandleStart_ClaimsOwnership(t *testing.T) {
	b, fake := newTestBot(t)
	b.ownerID = 0 // unclaimed
	ctx := context.Background()

	msg := ownerMessage("/start")
	msg.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 6}}
	b.handleMessage(ctx, msg)

	if b.ownerID != 500 {
		t.Errorf("owner id = %d, want 500", b.ownerID)
	}
	stored, ok, err := b.store.GetBotOwnerID(ctx)
	if err != nil || !ok || stored != 500 {
		t.Errorf("expected the owner to be persisted, got (%d, %v, %v)", stored, ok, err)
	}

	sends := fake.callsTo("sendMessage")
	if len(sends) < 2 {
		t.Fatalf("expected a claim notice and the home menu, got %+v", fake.methodNames())
	}
	if !strings.Contains(sends[0].text(), "owner of this Twork bot") {
		t.Errorf("first message = %q, want the ownership notice", sends[0].text())
	}
	if !strings.Contains(sends[1].text(), "TWORK") {
		t.Errorf("second message = %q, want the home dashboard", sends[1].text())
	}
}

// a second user's /start is turned away without disturbing the owner
func TestHandleStart_RejectsSecondUser(t *testing.T) {
	b, fake := newTestBot(t) // owner is already 500
	ctx := context.Background()

	msg := ownerMessage("/start")
	msg.From = &tgbotapi.User{ID: 999}
	msg.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 6}}
	b.handleMessage(ctx, msg)

	if b.ownerID != 500 {
		t.Errorf("owner id changed to %d", b.ownerID)
	}
	fake.assertScreenContains(t, "sendMessage", "private")
}

// /help answers the owner and nobody else
func TestHandleHelp(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	msg := ownerMessage("/help")
	msg.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 5}}
	b.handleMessage(ctx, msg)
	fake.assertScreenContains(t, "sendMessage", "Matches", "Keywords", "Settings")

	fake.reset()
	msg.From = &tgbotapi.User{ID: 999}
	b.handleMessage(ctx, msg)
	if len(fake.callsTo("sendMessage")) != 0 {
		t.Error("expected /help to be ignored for a non-owner")
	}
}

// free text is ignored unless the bot is waiting for a reply
func TestHandleMessage_IgnoresTextWithoutAPendingPrompt(t *testing.T) {
	b, fake := newTestBot(t)

	b.handleMessage(context.Background(), ownerMessage("hello?"))

	if len(fake.snapshot()) != 0 {
		t.Errorf("expected unsolicited text to be ignored, got calls %+v", fake.methodNames())
	}
}

// dispatch routes messages and callbacks, and survives a handler panic
func TestDispatch(t *testing.T) {
	b, fake := newTestBot(t)
	b.source = &fakeSource{}
	ctx := context.Background()

	b.dispatch(ctx, tgbotapi.Update{CallbackQuery: ownerCallback("menu:home")})
	if len(fake.callsTo("editMessageText")) != 1 {
		t.Errorf("expected a callback to be routed, got %+v", fake.methodNames())
	}

	fake.reset()
	msg := ownerMessage("/help")
	msg.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 5}}
	b.dispatch(ctx, tgbotapi.Update{Message: msg})
	if len(fake.callsTo("sendMessage")) != 1 {
		t.Errorf("expected a message to be routed, got %+v", fake.methodNames())
	}

	// An update with neither is a no-op.
	fake.reset()
	b.dispatch(ctx, tgbotapi.Update{})
	if len(fake.snapshot()) != 0 {
		t.Errorf("expected an empty update to be ignored, got %+v", fake.methodNames())
	}
}

// a panic inside a handler is recovered so one bad update can't kill the bot
func TestDispatch_RecoversFromPanic(t *testing.T) {
	b, _ := newTestBot(t)
	b.sess = nil // makes handleCallback dereference a nil session and panic

	// The test passes as long as this doesn't take the process down.
	b.dispatch(context.Background(), tgbotapi.Update{CallbackQuery: ownerCallback("menu:chats")})
}

// creating a group, adding aliases, cycling its mode, then deleting it
func TestKeywordGroupLifecycle(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	// New positive group.
	b.handleKeywordsCallback(ctx, "kw:new_pos")
	if b.sess.pending != inputNewGroup || !b.sess.editingGroupPositive {
		t.Fatalf("expected a positive-group prompt, got pending=%q positive=%v", b.sess.pending, b.sess.editingGroupPositive)
	}
	b.createGroup(ctx, "Go")

	kw := b.loadKeywords(ctx)
	if len(kw.PositiveGroups) != 1 || kw.PositiveGroups[0].Name != "Go" {
		t.Fatalf("group not created: %+v", kw.PositiveGroups)
	}
	// A new group starts with its own name as the first alias.
	if len(kw.PositiveGroups[0].Aliases) != 1 || kw.PositiveGroups[0].Aliases[0] != "Go" {
		t.Errorf("expected the name to seed the alias list, got %+v", kw.PositiveGroups[0].Aliases)
	}

	// Add aliases.
	b.handleKeywordsCallback(ctx, "kw:addalias:p0")
	if b.sess.editingGroupToken != "p0" {
		t.Fatalf("editing token = %q, want p0", b.sess.editingGroupToken)
	}
	b.addAliasesToPending(ctx, "golang, gopher")

	kw = b.loadKeywords(ctx)
	if len(kw.PositiveGroups[0].Aliases) != 3 {
		t.Fatalf("expected 3 aliases, got %+v", kw.PositiveGroups[0].Aliases)
	}

	// The live matcher is hot-swapped as keywords change.
	if !b.matchStore.Get().Match("we need a gopher").Matched() {
		t.Error("expected the newly added alias to match immediately, without a restart")
	}

	// Remove one alias.
	b.handleKeywordsCallback(ctx, "kw:rmalias:p0:0")
	kw = b.loadKeywords(ctx)
	if len(kw.PositiveGroups[0].Aliases) != 2 || kw.PositiveGroups[0].Aliases[0] != "golang" {
		t.Errorf("expected the first alias to be removed, got %+v", kw.PositiveGroups[0].Aliases)
	}

	// Cycle the group's mode: default -> whole_word -> substring -> default.
	for _, want := range []string{config.MatchModeWholeWord, config.MatchModeSubstring, ""} {
		b.handleKeywordsCallback(ctx, "kw:togglemode:p0")
		kw = b.loadKeywords(ctx)
		if kw.PositiveGroups[0].Mode != want {
			t.Fatalf("group mode = %q, want %q", kw.PositiveGroups[0].Mode, want)
		}
	}

	// Delete it.
	b.handleKeywordsCallback(ctx, "kw:delete:p0")
	kw = b.loadKeywords(ctx)
	if len(kw.PositiveGroups) != 0 {
		t.Errorf("expected the group to be deleted, got %+v", kw.PositiveGroups)
	}
}

// a negative group is created in the negative list
func TestCreateGroup_Negative(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	b.handleKeywordsCallback(ctx, "kw:new_neg")
	if b.sess.editingGroupPositive {
		t.Fatal("expected the new group to be negative")
	}
	b.createGroup(ctx, "Seniority")

	kw := b.loadKeywords(ctx)
	if len(kw.NegativeGroups) != 1 || kw.NegativeGroups[0].Name != "Seniority" {
		t.Errorf("negative groups = %+v", kw.NegativeGroups)
	}
	if len(kw.PositiveGroups) != 0 {
		t.Errorf("expected nothing in the positive list, got %+v", kw.PositiveGroups)
	}
}

// an empty group name is rejected rather than creating a nameless group
func TestCreateGroup_EmptyName(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	b.createGroup(ctx, "   ")

	if kw := b.loadKeywords(ctx); len(kw.PositiveGroups) != 0 || len(kw.NegativeGroups) != 0 {
		t.Errorf("expected no group to be created, got %+v / %+v", kw.PositiveGroups, kw.NegativeGroups)
	}
}

// callbacks addressing a group that no longer exists fall back to the menu
func TestKeywordCallbacks_StaleToken(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	for _, data := range []string{"kw:group:p9", "kw:delete:p9", "kw:togglemode:p9", "kw:rmalias:p9:0"} {
		fake.reset()
		b.handleKeywordsCallback(ctx, data) // must not panic
	}

	// A malformed remove-alias payload is dropped outright.
	fake.reset()
	b.handleKeywordsCallback(ctx, "kw:rmalias:p0")
	b.handleKeywordsCallback(ctx, "kw:rmalias:p0:notanumber")
	if len(fake.callsTo("editMessageText")) != 0 {
		t.Error("expected a malformed remove-alias payload to be ignored")
	}
}

// the global default mode toggles between whole word and substring
func TestToggleGlobalMode(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	b.handleKeywordsCallback(ctx, "kw:mode_toggle")
	if got := b.loadKeywords(ctx).Mode; got != config.MatchModeSubstring {
		t.Errorf("mode = %q, want substring", got)
	}

	b.handleKeywordsCallback(ctx, "kw:mode_toggle")
	if got := b.loadKeywords(ctx).Mode; got != config.MatchModeWholeWord {
		t.Errorf("mode = %q, want whole_word", got)
	}
}

// the keywords menu lists every group with a button apiece
func TestShowKeywordsMenu(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	if err := b.store.SetKeywords(ctx, storage.Keywords{
		PositiveGroups: []storage.KeywordGroup{{Name: "Go", Aliases: []string{"go", "golang"}}},
		NegativeGroups: []storage.KeywordGroup{{Name: "Seniority", Aliases: []string{"senior"}}},
		Mode:           config.MatchModeWholeWord,
	}); err != nil {
		t.Fatal(err)
	}

	b.showKeywordsMenu(ctx)
	fake.assertScreenContains(t, "editMessageText", "Positive groups: 1", "Negative groups: 1", "whole_word")

	markup := fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
	for _, want := range []string{"kw:group:p0", "kw:group:n0", "kw:new_pos", "kw:new_neg", "kw:mode_toggle"} {
		if !strings.Contains(markup, want) {
			t.Errorf("keyboard is missing %q:\n%s", want, markup)
		}
	}
}

// opening a group shows its aliases and per-alias remove buttons
func TestOpenGroupByToken(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	if err := b.store.SetKeywords(ctx, storage.Keywords{
		PositiveGroups: []storage.KeywordGroup{{Name: "Go", Aliases: []string{"go", "golang"}}},
		Mode:           config.MatchModeWholeWord,
	}); err != nil {
		t.Fatal(err)
	}

	b.handleKeywordsCallback(ctx, "kw:group:p0")
	fake.assertScreenContains(t, "editMessageText", "Group: Go", "inherits default", "• go", "• golang")

	markup := fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
	for _, want := range []string{"kw:rmalias:p0:0", "kw:rmalias:p0:1", "kw:addalias:p0", "kw:delete:p0"} {
		if !strings.Contains(markup, want) {
			t.Errorf("keyboard is missing %q:\n%s", want, markup)
		}
	}
}

// the settings screen offers a digest time only in the modes that send one
func TestShowSettingsMenu_DigestTimeVisibility(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	for _, c := range []struct {
		mode      string
		wantShown bool
	}{
		{storage.NotifyModeLive, false},
		{storage.NotifyModeDigest, true},
		{storage.NotifyModeBoth, true},
	} {
		if err := b.store.SetNotificationMode(ctx, c.mode); err != nil {
			t.Fatal(err)
		}
		fake.reset()
		b.showSettingsMenu(ctx)

		markup := fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
		if shown := strings.Contains(markup, "settings:digest_time"); shown != c.wantShown {
			t.Errorf("mode %q: digest-time button shown = %v, want %v", c.mode, shown, c.wantShown)
		}
	}
}

// the notifications toggle flips and persists
func TestHandleSettingsCallback_NotifyToggle(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	b.handleSettingsCallback(ctx, "settings:notify_toggle")
	enabled, ok, err := b.store.GetNotificationsEnabled(ctx)
	if err != nil || !ok || !enabled {
		t.Fatalf("expected notifications to be turned on, got (%v, %v, %v)", enabled, ok, err)
	}

	b.handleSettingsCallback(ctx, "settings:notify_toggle")
	enabled, _, _ = b.store.GetNotificationsEnabled(ctx)
	if enabled {
		t.Error("expected notifications to be turned back off")
	}
}

// the digest-time button asks for a time instead of setting one directly
func TestHandleSettingsCallback_DigestTimePrompts(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	b.handleSettingsCallback(ctx, "settings:digest_time")

	if b.sess.pending != inputDigestTime {
		t.Errorf("pending input = %q, want %q", b.sess.pending, inputDigestTime)
	}
	if len(fake.callsTo("sendMessage")) == 0 {
		t.Error("expected a prompt to be sent")
	}
}

// each pending-input kind is routed to the right handler
func TestHandlePendingInput_Routing(t *testing.T) {
	ctx := context.Background()

	t.Run("add chat", func(t *testing.T) {
		b, _ := newTestBot(t)
		src := &fakeSource{}
		b.source = src
		b.sess.pending = inputAddChat
		b.handlePendingInput(ctx, ownerMessage("@golang_jobs"))

		if src.addUsernameCalled != "golang_jobs" {
			t.Errorf("AddByUsername called with %q", src.addUsernameCalled)
		}
		if b.sess.pending != inputNone {
			t.Error("expected the prompt to be cleared")
		}
	})

	t.Run("search query", func(t *testing.T) {
		b, fake := newTestBot(t)
		b.sess.pending = inputSearchQuery
		b.handlePendingInput(ctx, ownerMessage("golang AND remote"))

		if b.sess.searchQuery != "golang AND remote" {
			t.Errorf("search query = %q", b.sess.searchQuery)
		}
		if b.sess.view != viewSearch {
			t.Errorf("view = %q, want %q", b.sess.view, viewSearch)
		}
		fake.assertScreenContains(t, "editMessageText", "No results")
	})

	t.Run("edit tag", func(t *testing.T) {
		b, _ := newTestBot(t)
		b.source = &fakeSource{}
		if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 55, Kind: models.ChatKindChannel, Title: "Jobs"}); err != nil {
			t.Fatal(err)
		}
		b.sess.pending = inputEditTag
		b.sess.editingTagFor = 55
		b.handlePendingInput(ctx, ownerMessage("DevOps"))

		c, err := b.store.GetChatByTelegramID(ctx, 55)
		if err != nil || c == nil {
			t.Fatalf("GetChatByTelegramID: %+v %v", c, err)
		}
		if c.Tag != "DevOps" {
			t.Errorf("tag = %q, want DevOps", c.Tag)
		}
	})

	t.Run("digest time", func(t *testing.T) {
		b, _ := newTestBot(t)
		b.sess.pending = inputDigestTime
		b.handlePendingInput(ctx, ownerMessage("07:15"))

		if got, _ := b.store.GetDigestTime(ctx); got != "07:15" {
			t.Errorf("digest time = %q, want 07:15", got)
		}
	})

	t.Run("new group", func(t *testing.T) {
		b, _ := newTestBot(t)
		b.sess.pending = inputNewGroup
		b.sess.editingGroupPositive = true
		b.handlePendingInput(ctx, ownerMessage("Rust"))

		if kw := b.loadKeywords(ctx); len(kw.PositiveGroups) != 1 {
			t.Errorf("expected the group to be created, got %+v", kw.PositiveGroups)
		}
	})
}

// the add-chat prompt routes each kind of pasted link to the right source method
func TestAddChatFromInput_Routing(t *testing.T) {
	ctx := context.Background()

	t.Run("username", func(t *testing.T) {
		b, _ := newTestBot(t)
		src := &fakeSource{}
		b.source = src
		b.addChatFromInput(ctx, "https://t.me/golang_jobs")
		if src.addUsernameCalled != "golang_jobs" {
			t.Errorf("AddByUsername called with %q", src.addUsernameCalled)
		}
	})

	t.Run("invite link", func(t *testing.T) {
		b, _ := newTestBot(t)
		src := &fakeSource{}
		b.source = src
		b.addChatFromInput(ctx, "https://t.me/+AbCdEf123")
		if src.addInviteCalled != "AbCdEf123" {
			t.Errorf("AddByInviteLink called with %q", src.addInviteCalled)
		}
	})

	t.Run("folder link", func(t *testing.T) {
		b, _ := newTestBot(t)
		src := &fakeSource{}
		b.source = src
		b.addChatFromInput(ctx, "https://t.me/addlist/SomeSlug")
		if src.addFolderCalled != "SomeSlug" {
			t.Errorf("AddFolder called with %q", src.addFolderCalled)
		}
	})

	t.Run("unparseable input", func(t *testing.T) {
		b, fake := newTestBot(t)
		src := &fakeSource{}
		b.source = src
		b.addChatFromInput(ctx, "not a link!!")

		if src.addUsernameCalled != "" || src.addInviteCalled != "" || src.addFolderCalled != "" {
			t.Error("expected nothing to be added for unparseable input")
		}
		fake.assertScreenContains(t, "editMessageText", "⚠️")
	})

	t.Run("source error is surfaced", func(t *testing.T) {
		b, fake := newTestBot(t)
		b.source = &fakeSource{addErr: fmt.Errorf("channel not found")}
		b.addChatFromInput(ctx, "@ghost_channel")
		fake.assertScreenContains(t, "editMessageText", "couldn't add that chat", "channel not found")
	})
}

// chat callbacks reach the source and re-render the right screen
func TestHandleChatCallback(t *testing.T) {
	b, fake := newTestBot(t)
	src := &fakeSource{chats: []models.Chat{{TelegramID: 55, Title: "Jobs", Username: "jobs"}}}
	b.source = src
	ctx := context.Background()

	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 55, Kind: models.ChatKindChannel, Title: "Jobs", Username: "jobs"}); err != nil {
		t.Fatal(err)
	}

	b.handleChatCallback(ctx, "chat:pause:55")
	if len(src.paused) != 1 || src.paused[0] != 55 {
		t.Errorf("Pause calls = %+v", src.paused)
	}

	b.handleChatCallback(ctx, "chat:resume:55")
	if len(src.resumed) != 1 || src.resumed[0] != 55 {
		t.Errorf("Resume calls = %+v", src.resumed)
	}

	fake.reset()
	b.handleChatCallback(ctx, "chat:view:55")
	fake.assertScreenContains(t, "editMessageText", "Jobs", "@jobs")

	fake.reset()
	b.handleChatCallback(ctx, "chat:remove_ask:55")
	fake.assertScreenContains(t, "editMessageText", "Stop monitoring")
	markup := fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
	if !strings.Contains(markup, "chat:remove:55") || !strings.Contains(markup, "chat:view:55") {
		t.Errorf("expected confirm and cancel buttons, got:\n%s", markup)
	}

	b.handleChatCallback(ctx, "chat:remove:55")
	if len(src.removed) != 1 || src.removed[0] != 55 {
		t.Errorf("Remove calls = %+v", src.removed)
	}

	// The tag button prompts rather than editing directly.
	b.handleChatCallback(ctx, "chat:tag:55")
	if b.sess.pending != inputEditTag || b.sess.editingTagFor != 55 {
		t.Errorf("expected a tag prompt for chat 55, got pending=%q for=%d", b.sess.pending, b.sess.editingTagFor)
	}
}

// viewing a chat that isn't monitored falls back to the list
func TestShowChatDetail_UnknownChat(t *testing.T) {
	b, fake := newTestBot(t)
	b.source = &fakeSource{}

	b.showChatDetail(context.Background(), 12345)
	fake.assertScreenContains(t, "editMessageText", "Monitored chats")
}

// the chats list shows status markers, tags, and paging controls
func TestShowChatsList(t *testing.T) {
	b, fake := newTestBot(t)
	b.source = &fakeSource{chats: []models.Chat{
		{TelegramID: 1, Title: "Alpha Jobs", Tag: "Go"},
		{TelegramID: 2, Title: "Beta Jobs", Paused: true},
	}}

	b.showChatsList(context.Background(), 0)
	fake.assertScreenContains(t, "editMessageText", "Monitored chats (2)")

	markup := fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
	for _, want := range []string{"Alpha Jobs", "(Go)", "chat:view:1", "chat:view:2", "chat:add", "chat:find"} {
		if !strings.Contains(markup, want) {
			t.Errorf("keyboard is missing %q:\n%s", want, markup)
		}
	}
}

// with nothing monitored the list explains how to get started
func TestShowChatsList_Empty(t *testing.T) {
	b, fake := newTestBot(t)
	b.source = &fakeSource{}

	b.showChatsList(context.Background(), 0)
	fake.assertScreenContains(t, "editMessageText", "Monitored chats (0)", "Nothing yet")
}

// channel search explains itself when no discovery token is configured
func TestFindChannels_NotConfigured(t *testing.T) {
	b, fake := newTestBot(t) // searcher is nil

	b.findChannels(context.Background(), "golang jobs")
	fake.assertScreenContains(t, "editMessageText", "isn't set up", "tgstat_token")
}

// the matches carousel renders the current post and its controls
func TestRenderCarousel(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	id, err := b.store.InsertMessage(ctx, models.Message{
		TelegramMessageID: 1,
		ChatID:            10,
		ChatTitle:         "Golang Jobs",
		Timestamp:         time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Text:              "Backend Go developer",
		Link:              "https://t.me/golang_jobs/1",
	})
	if err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	if err := b.store.RecordMatch(ctx, id, `["Go"]`); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}

	b.openCarousel(ctx, viewMatches, 0)
	fake.assertScreenContains(t, "editMessageText", "📋 Match", "Golang Jobs", "Backend Go developer", "✓ Go")

	// Saving from the carousel toggles the bookmark and re-renders.
	fake.reset()
	b.handleCarouselCallback(ctx, fmt.Sprintf("list:bookmark:%d", id))
	row, err := b.store.GetMatchRow(ctx, id)
	if err != nil {
		t.Fatalf("GetMatchRow: %v", err)
	}
	if !row.Bookmarked {
		t.Error("expected the post to be bookmarked")
	}
	if !strings.Contains(fake.lastCallTo(t, "editMessageText").params.Get("reply_markup"), "⭐ Saved") {
		t.Error("expected the save button to switch to its saved label")
	}
}

// paging past the end of a list clamps back onto the last item
func TestRenderCarousel_ClampsPagePastTheEnd(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	id, err := b.store.InsertMessage(ctx, models.Message{
		TelegramMessageID: 1, ChatID: 10, ChatTitle: "Jobs",
		Timestamp: time.Now(), Text: "only post", Link: "https://t.me/x/1",
	})
	if err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	if err := b.store.RecordMatch(ctx, id, `["Go"]`); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}

	b.openCarousel(ctx, viewMatches, 0)
	fake.reset()
	b.handleCarouselCallback(ctx, "list:page:99")

	if b.sess.page != 0 {
		t.Errorf("page = %d, want it clamped back to 0", b.sess.page)
	}
	fake.assertScreenContains(t, "editMessageText", "only post")
}

// with no active view there's nothing to render
func TestRenderCarousel_NoActiveView(t *testing.T) {
	b, fake := newTestBot(t)

	b.renderCarousel(context.Background())
	fake.assertScreenContains(t, "editMessageText", "Something went wrong")
}

// exporting the current view sends a .md document
func TestExportCurrentView(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	id, err := b.store.InsertMessage(ctx, models.Message{
		TelegramMessageID: 1, ChatID: 10, ChatTitle: "Golang Jobs",
		Timestamp: time.Now(), Text: "Backend Go developer", Link: "https://t.me/golang_jobs/1",
	})
	if err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	if err := b.store.RecordMatch(ctx, id, `["Go"]`); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}

	b.sess.view = viewMatches
	fake.reset()
	b.handleCarouselCallback(ctx, "list:export")

	call := fake.lastCallTo(t, "sendDocument")
	if !strings.Contains(call.params.Get("caption"), "1 post(s) exported") {
		t.Errorf("caption = %q", call.params.Get("caption"))
	}
}

// an export of an empty view sends nothing at all
func TestExportCurrentView_Empty(t *testing.T) {
	b, fake := newTestBot(t)

	b.sess.view = viewMatches
	b.exportCurrentView(context.Background())

	if len(fake.callsTo("sendDocument")) != 0 {
		t.Error("expected no document for an empty view")
	}
}

// collectViewRows pages through every row, not just the first page
func TestCollectViewRows_PagesThroughEverything(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	const total = exportPageSize + 5
	for i := range total {
		id, err := b.store.InsertMessage(ctx, models.Message{
			TelegramMessageID: i,
			ChatID:            10,
			ChatTitle:         "Golang Jobs",
			Timestamp:         time.Now().Add(time.Duration(i) * time.Minute),
			Text:              fmt.Sprintf("vacancy %d", i),
			Link:              fmt.Sprintf("https://t.me/x/%d", i),
		})
		if err != nil {
			t.Fatalf("InsertMessage %d: %v", i, err)
		}
		if err := b.store.RecordMatch(ctx, id, `["Go"]`); err != nil {
			t.Fatalf("RecordMatch: %v", err)
		}
	}

	b.sess.view = viewMatches
	rows, title, err := b.collectViewRows(ctx)
	if err != nil {
		t.Fatalf("collectViewRows: %v", err)
	}
	if len(rows) != total {
		t.Errorf("collected %d rows, want %d", len(rows), total)
	}
	if title != "Twork Matches" {
		t.Errorf("title = %q", title)
	}
}

// exporting with no active view is an error, not an empty file
func TestCollectViewRows_NoActiveView(t *testing.T) {
	b, _ := newTestBot(t)
	if _, _, err := b.collectViewRows(context.Background()); err == nil {
		t.Error("expected an error with no active view")
	}
}

// a live match reaches the owner with Save/Open/Dismiss controls
func TestNotifyMatch(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	msg := models.Message{ChatTitle: "Golang Jobs", Text: "Backend Go developer", Link: "https://t.me/golang_jobs/1"}
	b.NotifyMatch(ctx, msg, 7, []string{"Go", "Backend"})

	call := fake.lastCallTo(t, "sendMessage")
	for _, want := range []string{"New match", "Golang Jobs", "Backend Go developer", "✓ Go", "✓ Backend"} {
		if !strings.Contains(call.text(), want) {
			t.Errorf("notification is missing %q:\n%s", want, call.text())
		}
	}
	markup := call.params.Get("reply_markup")
	for _, want := range []string{"notify:save:7", "notify:dismiss", "https://t.me/golang_jobs/1"} {
		if !strings.Contains(markup, want) {
			t.Errorf("notification keyboard is missing %q:\n%s", want, markup)
		}
	}
}

// notifications are suppressed when they're turned off or digest-only
func TestNotifyMatch_Suppressed(t *testing.T) {
	ctx := context.Background()
	msg := models.Message{ChatTitle: "Jobs", Text: "post", Link: "https://t.me/x/1"}

	t.Run("no owner yet", func(t *testing.T) {
		b, fake := newTestBot(t)
		b.ownerID = 0
		b.NotifyMatch(ctx, msg, 1, []string{"Go"})
		if len(fake.callsTo("sendMessage")) != 0 {
			t.Error("expected no notification before an owner is claimed")
		}
	})

	t.Run("notifications off", func(t *testing.T) {
		b, fake := newTestBot(t)
		if err := b.store.SetNotificationsEnabled(ctx, false); err != nil {
			t.Fatal(err)
		}
		b.NotifyMatch(ctx, msg, 1, []string{"Go"})
		if len(fake.callsTo("sendMessage")) != 0 {
			t.Error("expected no notification when notifications are off")
		}
	})

	t.Run("digest-only mode", func(t *testing.T) {
		b, fake := newTestBot(t)
		if err := b.store.SetNotificationMode(ctx, storage.NotifyModeDigest); err != nil {
			t.Fatal(err)
		}
		b.NotifyMatch(ctx, msg, 1, []string{"Go"})
		if len(fake.callsTo("sendMessage")) != 0 {
			t.Error("expected digest-only mode to suppress live pings")
		}
	})

	t.Run("both mode still pings", func(t *testing.T) {
		b, fake := newTestBot(t)
		if err := b.store.SetNotificationMode(ctx, storage.NotifyModeBoth); err != nil {
			t.Fatal(err)
		}
		b.NotifyMatch(ctx, msg, 1, []string{"Go"})
		if len(fake.callsTo("sendMessage")) != 1 {
			t.Error("expected \"both\" mode to still send a live ping")
		}
	})
}

// a very long post is truncated before it's sent as a notification
func TestNotifyMatch_TruncatesLongPosts(t *testing.T) {
	b, fake := newTestBot(t)

	b.NotifyMatch(context.Background(), models.Message{
		ChatTitle: "Jobs",
		Text:      strings.Repeat("x", maxSnippetLen*2),
		Link:      "https://t.me/x/1",
	}, 1, []string{"Go"})

	got := fake.lastCallTo(t, "sendMessage").text()
	if strings.Count(got, "x") != maxSnippetLen {
		t.Errorf("expected the post truncated to %d characters, got %d", maxSnippetLen, strings.Count(got, "x"))
	}
}

// Save on a notification bookmarks the post and clears the alert
func TestHandleNotifyCallback(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	id, err := b.store.InsertMessage(ctx, models.Message{
		TelegramMessageID: 1, ChatID: 10, ChatTitle: "Jobs",
		Timestamp: time.Now(), Text: "post", Link: "https://t.me/x/1",
	})
	if err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	b.handleNotifyCallback(ctx, ownerCallback(""), fmt.Sprintf("notify:save:%d", id))
	row, err := b.store.GetMatchRow(ctx, id)
	if err != nil {
		t.Fatalf("GetMatchRow: %v", err)
	}
	if !row.Bookmarked {
		t.Error("expected Save to bookmark the post")
	}
	if len(fake.callsTo("deleteMessage")) != 1 {
		t.Errorf("expected the notification to be deleted, got %+v", fake.methodNames())
	}

	fake.reset()
	b.handleNotifyCallback(ctx, ownerCallback(""), "notify:dismiss")
	if len(fake.callsTo("deleteMessage")) != 1 {
		t.Errorf("expected Dismiss to delete the notification, got %+v", fake.methodNames())
	}
}

// the digest fires once, at the configured time, in a mode that wants one
func TestShouldSendDigest(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	at := func(hhmm string) time.Time {
		parsed, err := time.Parse("15:04", hhmm)
		if err != nil {
			t.Fatalf("parsing %q: %v", hhmm, err)
		}
		return time.Date(2026, 7, 25, parsed.Hour(), parsed.Minute(), 0, 0, time.Local)
	}

	if err := b.store.SetNotificationMode(ctx, storage.NotifyModeDigest); err != nil {
		t.Fatal(err)
	}
	if err := b.store.SetDigestTime(ctx, "09:00"); err != nil {
		t.Fatal(err)
	}

	if !b.shouldSendDigest(ctx, at("09:00"), "") {
		t.Error("expected the digest to fire at the configured time")
	}
	if b.shouldSendDigest(ctx, at("08:59"), "") {
		t.Error("expected no digest a minute early")
	}
	if b.shouldSendDigest(ctx, at("09:01"), "") {
		t.Error("expected no digest a minute late")
	}
	if b.shouldSendDigest(ctx, at("09:00"), "2026-07-25") {
		t.Error("expected at most one digest per calendar day")
	}
	// Yesterday's send doesn't block today's.
	if !b.shouldSendDigest(ctx, at("09:00"), "2026-07-24") {
		t.Error("expected a new day to allow another digest")
	}

	// Live-only mode never sends one.
	if err := b.store.SetNotificationMode(ctx, storage.NotifyModeLive); err != nil {
		t.Fatal(err)
	}
	if b.shouldSendDigest(ctx, at("09:00"), "") {
		t.Error("expected live-only mode to skip the digest")
	}

	// "both" does.
	if err := b.store.SetNotificationMode(ctx, storage.NotifyModeBoth); err != nil {
		t.Fatal(err)
	}
	if !b.shouldSendDigest(ctx, at("09:00"), "") {
		t.Error("expected \"both\" mode to send the digest")
	}

	// And nothing is sent before an owner has claimed the bot.
	b.ownerID = 0
	if b.shouldSendDigest(ctx, at("09:00"), "") {
		t.Error("expected no digest before an owner is claimed")
	}
}

// sendDigest reports the day's counts and posts to the owner
func TestSendDigest(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	id, err := b.store.InsertMessage(ctx, models.Message{
		TelegramMessageID: 1, ChatID: 10, ChatTitle: "Golang Jobs",
		Timestamp: time.Now(), Text: "Backend Go developer\nremote", Link: "https://t.me/golang_jobs/1",
	})
	if err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	if err := b.store.RecordMatch(ctx, id, `["Go"]`); err != nil {
		t.Fatalf("RecordMatch: %v", err)
	}

	b.sendDigest(ctx, time.Now())

	got := fake.lastCallTo(t, "sendMessage").text()
	for _, want := range []string{"Daily digest", "1 new post(s)", "1 matched", "Golang Jobs", "https://t.me/golang_jobs/1"} {
		if !strings.Contains(got, want) {
			t.Errorf("digest is missing %q:\n%s", want, got)
		}
	}
	// Only the first line of a multi-line post is shown.
	if strings.Contains(got, "remote") {
		t.Errorf("expected only the post's first line:\n%s", got)
	}
}

// the scheduler stops when its context is cancelled
func TestRunDigestScheduler_StopsOnCancel(t *testing.T) {
	b, _ := newTestBot(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.RunDigestScheduler(ctx) }()

	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("RunDigestScheduler returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunDigestScheduler did not stop after its context was cancelled")
	}
}
