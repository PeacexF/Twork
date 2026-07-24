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

func (b *Bot) NotifyMatch(ctx context.Context, msg models.Message, messageID int64, matchedKeywords []string) {
	if b.ownerID == 0 {
		return
	}
	enabled, ok, err := b.store.GetNotificationsEnabled(ctx)
	if err != nil {
		log.Printf("twork: reading notification setting failed: %v", err)
	}
	if ok && !enabled {
		return
	}

	snippet := msg.Text
	if len(snippet) > maxSnippetLen {
		snippet = snippet[:maxSnippetLen] + "…"
	}
	text := fmt.Sprintf("🆕 New match -- %s\n\n%s\n\nMatched:\n", msg.ChatTitle, snippet)
	for _, k := range matchedKeywords {
		text += "✓ " + k + "\n"
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⭐ Save", fmt.Sprintf("notify:save:%d", messageID)),
			tgbotapi.NewInlineKeyboardButtonURL("🔗 Open", msg.Link),
			tgbotapi.NewInlineKeyboardButtonData("🗑 Dismiss", "notify:dismiss"),
		),
	)
	out := tgbotapi.NewMessage(b.ownerID, text)
	out.ReplyMarkup = kb
	if _, err := b.api.Send(out); err != nil {
		log.Printf("twork: sending match notification failed: %v", err)
	}
}

func (b *Bot) handleNotifyCallback(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	switch {
	case data == "notify:dismiss":
		b.deleteMessage(cq.Message.Chat.ID, cq.Message.MessageID)
	case strings.HasPrefix(data, "notify:save:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "notify:save:"), 10, 64)
		if _, err := b.store.ToggleBookmark(ctx, id); err != nil {
			log.Printf("twork: toggling bookmark from notification failed: %v", err)
		}
		b.deleteMessage(cq.Message.Chat.ID, cq.Message.MessageID)
	}
}
