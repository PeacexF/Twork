package bot

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handles a free-text reply to the active prompt
func (b *Bot) handlePendingInput(ctx context.Context, msg *tgbotapi.Message) {
	pending := b.sess.pending
	text := strings.TrimSpace(msg.Text)
	b.clearPrompt(msg)

	switch pending {
	case inputAddChat:
		b.addChatFromInput(ctx, text)

	case inputFindChats:
		b.findChannels(ctx, text)

	case inputSearchQuery:
		b.sess.searchQuery = text
		b.openCarousel(ctx, viewSearch, 0)

	case inputAddPositiveKw:
		b.addKeywords(ctx, true, text)

	case inputAddNegativeKw:
		b.addKeywords(ctx, false, text)

	case inputEditTag:
		_ = b.store.SetChatTag(ctx, b.sess.editingTagFor, text)
		b.showChatDetail(ctx, b.sess.editingTagFor)
	}
}

// parses and appends keywords, then re-renders the menu
func (b *Bot) addKeywords(ctx context.Context, positive bool, text string) {
	toAdd := parseKeywordInput(text)
	if len(toAdd) == 0 {
		b.showKeywordsMenu(ctx)
		return
	}
	kw := b.loadKeywords(ctx)
	if positive {
		kw.Positive = append(kw.Positive, toAdd...)
	} else {
		kw.Negative = append(kw.Negative, toAdd...)
	}
	b.saveKeywords(ctx, kw)
	b.showKeywordsMenu(ctx)
}
