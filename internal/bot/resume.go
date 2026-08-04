package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/models"
)

// routes chat:rb* callbacks -- resume broadcasting config for one chat.
// Named "rb" (not "resume") to avoid colliding with the existing
// chat:resume:<id> callback, which un-pauses monitoring.
func (b *Bot) handleChatResumeCallback(ctx context.Context, data string) {
	switch {
	case strings.HasPrefix(data, "chat:rb_toggle:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:rb_toggle:"), 10, 64)
		b.toggleChatResume(ctx, id)
	case strings.HasPrefix(data, "chat:rb_interval:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:rb_interval:"), 10, 64)
		b.sess.editingResumeFor = id
		b.promptFor(ctx, inputResumeInterval, "Send how often to re-post the resume into this chat, in minutes (e.g. 120 for every 2h). Never goes below the configured minimum delay.")
	case strings.HasPrefix(data, "chat:rb_text:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:rb_text:"), 10, 64)
		b.sess.editingResumeFor = id
		b.promptFor(ctx, inputResumeText, "Send the resume text to use for THIS chat. Send \"-\" to clear it and fall back to the global text.")
	case strings.HasPrefix(data, "chat:rb:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:rb:"), 10, 64)
		b.showChatResumeConfig(ctx, id)
	}
}

// renders one chat's resume broadcasting config -- group chats only
func (b *Bot) showChatResumeConfig(ctx context.Context, telegramID int64) {
	c, err := b.store.GetChatByTelegramID(ctx, telegramID)
	if err != nil || c == nil {
		b.showChatsList(ctx, 0)
		return
	}
	if c.Kind != models.ChatKindGroup {
		// Channels never get this screen from the bot's own menus, but
		// guard anyway in case a stale callback_data button is tapped.
		b.showChatDetail(ctx, telegramID)
		return
	}

	status, toggleLabel := "🔕 OFF", "▶️ Turn ON"
	if c.ResumeEnabled {
		status, toggleLabel = "📨 ON", "⏸ Turn OFF"
	}

	interval := "not set (uses the configured minimum delay)"
	if c.ResumeIntervalSeconds > 0 {
		interval = fmt.Sprintf("every %dm", c.ResumeIntervalSeconds/60)
	}

	textNote := "using the global resume text"
	if c.ResumeText != "" {
		textNote = "using a custom text for this chat"
	}

	text := fmt.Sprintf("📨 Resume broadcasting -- %s\n\nStatus: %s\nInterval: %s\nText: %s",
		c.Title, status, interval, textNote)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleLabel, fmt.Sprintf("chat:rb_toggle:%d", c.TelegramID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏱ Set interval", fmt.Sprintf("chat:rb_interval:%d", c.TelegramID)),
			tgbotapi.NewInlineKeyboardButtonData("✏️ Edit text", fmt.Sprintf("chat:rb_text:%d", c.TelegramID)),
		),
		tgbotapi.NewInlineKeyboardRow(backButton(fmt.Sprintf("chat:view:%d", c.TelegramID))),
	)
	b.editHome(ctx, text, kb)
}

// flips a chat's resume broadcasting on/off, keeping its current interval/text
func (b *Bot) toggleChatResume(ctx context.Context, telegramID int64) {
	c, err := b.store.GetChatByTelegramID(ctx, telegramID)
	if err != nil || c == nil {
		b.showChatsList(ctx, 0)
		return
	}
	if err := b.store.SetChatResumeConfig(ctx, telegramID, !c.ResumeEnabled, c.ResumeIntervalSeconds, c.ResumeText); err != nil {
		log.Printf("twork: toggling resume broadcasting failed: %v", err)
	}
	b.showChatResumeConfig(ctx, telegramID)
}

// parses and stores the resume interval (given in minutes) for the chat
// pending_input.go routed this to
func (b *Bot) setChatResumeInterval(ctx context.Context, text string) {
	telegramID := b.sess.editingResumeFor
	minutes, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || minutes <= 0 {
		b.showChatResumeConfig(ctx, telegramID)
		return
	}
	c, err := b.store.GetChatByTelegramID(ctx, telegramID)
	if err != nil || c == nil {
		b.showChatsList(ctx, 0)
		return
	}
	if err := b.store.SetChatResumeConfig(ctx, telegramID, c.ResumeEnabled, minutes*60, c.ResumeText); err != nil {
		log.Printf("twork: setting resume interval failed: %v", err)
	}
	b.showChatResumeConfig(ctx, telegramID)
}

// prompts for the global resume text -- the default used by any chat
// without its own per-chat override
func (b *Bot) promptGlobalResumeText(ctx context.Context) {
	b.sess.editingResumeFor = 0
	b.promptFor(ctx, inputResumeText, "Send the global resume/pitch text to broadcast into groups (used by any chat without its own override):")
}

// stores the resume text for whichever target is pending: the chat in
// editingResumeFor, or the global text if it's 0
func (b *Bot) setResumeText(ctx context.Context, text string) {
	telegramID := b.sess.editingResumeFor
	text = strings.TrimSpace(text)

	if telegramID == 0 {
		if err := b.store.SetResumeGlobalText(ctx, text); err != nil {
			log.Printf("twork: setting global resume text failed: %v", err)
		}
		b.showSettingsMenu(ctx)
		return
	}

	if text == "-" {
		text = ""
	}
	c, err := b.store.GetChatByTelegramID(ctx, telegramID)
	if err != nil || c == nil {
		b.showChatsList(ctx, 0)
		return
	}
	if err := b.store.SetChatResumeConfig(ctx, telegramID, c.ResumeEnabled, c.ResumeIntervalSeconds, text); err != nil {
		log.Printf("twork: setting chat resume text failed: %v", err)
	}
	b.showChatResumeConfig(ctx, telegramID)
}
