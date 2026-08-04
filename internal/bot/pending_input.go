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

	case inputNewGroup:
		b.createGroup(ctx, text)

	case inputAddAlias:
		b.addAliasesToPending(ctx, text)

	case inputEditTag:
		_ = b.store.SetChatTag(ctx, b.sess.editingTagFor, text)
		b.showChatDetail(ctx, b.sess.editingTagFor)

	case inputDigestTime:
		b.setDigestTime(ctx, text)

	case inputResumeInterval:
		b.setChatResumeInterval(ctx, text)

	case inputResumeText:
		b.setResumeText(ctx, text)
	}
}
