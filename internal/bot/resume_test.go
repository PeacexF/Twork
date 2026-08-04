package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/PeacexF/Twork/internal/models"
)

// a group chat's detail screen offers the resume broadcasting button; a
// channel's does not -- there's no path to it from the bot for channels
func TestShowChatDetail_ResumeButtonIsGroupOnly(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Go Jobs Group"}); err != nil {
		t.Fatalf("UpsertChat (group): %v", err)
	}
	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 2, Kind: models.ChatKindChannel, Title: "Go Jobs Channel"}); err != nil {
		t.Fatalf("UpsertChat (channel): %v", err)
	}

	b.showChatDetail(ctx, 1)
	markup := fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
	if !strings.Contains(markup, "chat:rb:1") {
		t.Errorf("expected a resume broadcasting button for a group, markup:\n%s", markup)
	}

	fake.reset()
	b.showChatDetail(ctx, 2)
	markup = fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
	if strings.Contains(markup, "chat:rb:2") {
		t.Errorf("expected no resume broadcasting button for a channel, markup:\n%s", markup)
	}
}

// opening the resume config screen for a channel (e.g. a stale button tap)
// falls back to the normal chat detail screen instead of rendering it
func TestShowChatResumeConfig_RefusesChannel(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 2, Kind: models.ChatKindChannel, Title: "Go Jobs Channel"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	b.showChatResumeConfig(ctx, 2)
	fake.assertScreenContains(t, "editMessageText", "Go Jobs Channel")
	markup := fake.lastCallTo(t, "editMessageText").params.Get("reply_markup")
	if strings.Contains(markup, "chat:rb_toggle:") {
		t.Errorf("expected the resume config screen to be refused for a channel, markup:\n%s", markup)
	}
}

// the toggle button flips resume broadcasting on, then off, for a group
func TestHandleChatResumeCallback_Toggle(t *testing.T) {
	b, fake := newTestBot(t)
	ctx := context.Background()

	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Go Jobs"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	b.handleChatResumeCallback(ctx, "chat:rb_toggle:1")
	c, err := b.store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || !c.ResumeEnabled {
		t.Fatalf("expected resume broadcasting enabled, chat = %+v, err = %v", c, err)
	}
	fake.assertScreenContains(t, "editMessageText", "📨 ON")

	b.handleChatResumeCallback(ctx, "chat:rb_toggle:1")
	c, err = b.store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || c.ResumeEnabled {
		t.Fatalf("expected resume broadcasting disabled, chat = %+v, err = %v", c, err)
	}
	fake.assertScreenContains(t, "editMessageText", "🔕 OFF")
}

// the interval button prompts, and the reply (in minutes) is stored in seconds
func TestSetChatResumeInterval(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Go Jobs"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	b.handleChatResumeCallback(ctx, "chat:rb_interval:1")
	if b.sess.pending != inputResumeInterval || b.sess.editingResumeFor != 1 {
		t.Fatalf("expected an interval prompt for chat 1, pending=%q for=%d", b.sess.pending, b.sess.editingResumeFor)
	}

	b.setChatResumeInterval(ctx, "120")
	c, err := b.store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || c.ResumeIntervalSeconds != 120*60 {
		t.Fatalf("expected a 7200s interval, chat = %+v, err = %v", c, err)
	}

	// a non-numeric reply is ignored rather than corrupting the stored value
	b.setChatResumeInterval(ctx, "not a number")
	c, err = b.store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || c.ResumeIntervalSeconds != 120*60 {
		t.Fatalf("expected the interval to be unchanged, chat = %+v, err = %v", c, err)
	}
}

// a per-chat text override is stored, then "-" clears it back to the global fallback
func TestSetResumeText_PerChatOverrideAndClear(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Go Jobs"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	b.sess.editingResumeFor = 1

	b.setResumeText(ctx, "Backend dev, 5 YOE, available now.")
	c, err := b.store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || c.ResumeText != "Backend dev, 5 YOE, available now." {
		t.Fatalf("chat = %+v, err = %v", c, err)
	}

	b.sess.editingResumeFor = 1
	b.setResumeText(ctx, "-")
	c, err = b.store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || c.ResumeText != "" {
		t.Fatalf("expected \"-\" to clear the override, chat = %+v, err = %v", c, err)
	}
}

// editingResumeFor == 0 targets the global resume text, not any specific chat
func TestSetResumeText_Global(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	b.promptGlobalResumeText(ctx)
	if b.sess.pending != inputResumeText || b.sess.editingResumeFor != 0 {
		t.Fatalf("expected a global resume text prompt, pending=%q for=%d", b.sess.pending, b.sess.editingResumeFor)
	}

	b.setResumeText(ctx, "Experienced Go developer available for freelance work.")
	got, err := b.store.GetResumeGlobalText(ctx)
	if err != nil || got != "Experienced Go developer available for freelance work." {
		t.Fatalf("GetResumeGlobalText = %q, %v", got, err)
	}
}

// the settings menu routes to the global resume text prompt
func TestHandleSettingsCallback_ResumeText(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	b.handleSettingsCallback(ctx, "settings:resume_text")
	if b.sess.pending != inputResumeText || b.sess.editingResumeFor != 0 {
		t.Fatalf("expected a global resume text prompt, pending=%q for=%d", b.sess.pending, b.sess.editingResumeFor)
	}
}

// pending free-text replies route to the right handler for both new input kinds
func TestHandlePendingInput_Resume(t *testing.T) {
	b, _ := newTestBot(t)
	ctx := context.Background()

	if err := b.store.UpsertChat(ctx, models.Chat{TelegramID: 1, Kind: models.ChatKindGroup, Title: "Go Jobs"}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	b.sess.pending = inputResumeInterval
	b.sess.editingResumeFor = 1
	b.handlePendingInput(ctx, ownerMessage("45"))
	c, err := b.store.GetChatByTelegramID(ctx, 1)
	if err != nil || c == nil || c.ResumeIntervalSeconds != 45*60 {
		t.Fatalf("chat = %+v, err = %v", c, err)
	}

	b.sess.pending = inputResumeText
	b.sess.editingResumeFor = 0
	b.handlePendingInput(ctx, ownerMessage("pitch text"))
	got, err := b.store.GetResumeGlobalText(ctx)
	if err != nil || got != "pitch text" {
		t.Fatalf("GetResumeGlobalText = %q, %v", got, err)
	}
}
