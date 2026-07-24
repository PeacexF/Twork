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
	case inputAddUsername:
		chat, err := b.coll.AddByUsername(ctx, text)
		b.handleAddChatResult(ctx, chat, err)

	case inputAddInvite:
		chat, err := b.coll.AddByInviteLink(ctx, text)
		b.handleAddChatResult(ctx, chat, err)

	case inputAddFolder:
		chats, err := b.coll.AddFolder(ctx, text)
		if err != nil || len(chats) == 0 {
			b.handleAddChatResult(ctx, nil, err)
			return
		}
		b.showChatsList(ctx, 0)

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
