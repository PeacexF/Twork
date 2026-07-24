package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/storage"
)

const maxSnippetLen = 800

// routes list: callbacks (page/bookmark/export)
func (b *Bot) handleCarouselCallback(ctx context.Context, data string) {
	switch {
	case strings.HasPrefix(data, "list:page:"):
		n, _ := strconv.Atoi(strings.TrimPrefix(data, "list:page:"))
		b.sess.page = n
		b.renderCarousel(ctx)
	case strings.HasPrefix(data, "list:bookmark:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "list:bookmark:"), 10, 64)
		if _, err := b.store.ToggleBookmark(ctx, id); err != nil {
			log.Printf("twork: toggling bookmark failed: %v", err)
		}
		b.renderCarousel(ctx)
	case data == "list:export":
		b.exportCurrentView(ctx)
	}
}

// switches to view v starting at page
func (b *Bot) openCarousel(ctx context.Context, v viewKind, page int) {
	b.sess.view = v
	b.sess.page = page
	b.renderCarousel(ctx)
}

// fetches the single row at the current page for the active view
func (b *Bot) fetchCarouselItem(ctx context.Context) (*storage.MatchRow, int, error) {
	var (
		rows  []storage.MatchRow
		total int
		err   error
	)
	switch b.sess.view {
	case viewMatches:
		rows, total, err = b.store.ListMatches(ctx, 1, b.sess.page)
	case viewFavorites:
		rows, total, err = b.store.ListBookmarked(ctx, 1, b.sess.page)
	case viewSearch:
		rows, total, err = b.store.SearchPaged(ctx, b.sess.searchQuery, 1, b.sess.page)
	default:
		return nil, 0, fmt.Errorf("no active list view")
	}
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return nil, total, nil
	}
	return &rows[0], total, nil
}

// renders the current carousel item and keyboard
func (b *Bot) renderCarousel(ctx context.Context) {
	item, total, err := b.fetchCarouselItem(ctx)
	if err != nil {
		b.editHome(ctx, "Something went wrong loading that list.", homeOnlyKeyboard())
		return
	}
	if total == 0 {
		b.editHome(ctx, viewEmptyText(b.sess.view), homeOnlyKeyboard())
		return
	}

	if b.sess.page >= total {
		b.sess.page = total - 1
	}
	if b.sess.page < 0 {
		b.sess.page = 0
	}
	if item == nil {

		item, total, err = b.fetchCarouselItem(ctx)
		if err != nil || item == nil {
			b.editHome(ctx, viewEmptyText(b.sess.view), homeOnlyKeyboard())
			return
		}
	}

	text := carouselText(b.sess.view, *item)
	kb := carouselKeyboard(*item, b.sess.page, total)
	b.editHome(ctx, text, kb)
}

// returns the empty-state message for a view
func viewEmptyText(v viewKind) string {
	switch v {
	case viewMatches:
		return "📋 No matches yet.\n\nThey'll show up here as your monitored chats get new posts."
	case viewFavorites:
		return "⭐ No favorites yet.\n\nSave a post from Matches or Search to see it here."
	case viewSearch:
		return "🔍 No results for that search."
	default:
		return "Nothing to show."
	}
}

// formats a match row into carousel display text
func carouselText(v viewKind, item storage.MatchRow) string {
	var title string
	switch v {
	case viewMatches:
		title = "📋 Match"
	case viewFavorites:
		title = "⭐ Favorite"
	case viewSearch:
		title = "🔍 Result"
	}

	snippet := item.Text
	if len(snippet) > maxSnippetLen {
		snippet = snippet[:maxSnippetLen] + "…"
	}

	text := fmt.Sprintf("%s -- %s\n\n%s", title, item.ChatTitle, snippet)
	if len(item.MatchedKeywords) > 0 {
		text += "\n\nMatched:\n"
		for _, k := range item.MatchedKeywords {
			text += "✓ " + k + "\n"
		}
	}
	text += "\n" + item.Timestamp.Format("2006-01-02 15:04 MST")
	return text
}

// builds the prev/next/save/open/export keyboard
func carouselKeyboard(item storage.MatchRow, page, total int) tgbotapi.InlineKeyboardMarkup {
	saveLabel := "☆ Save"
	if item.Bookmarked {
		saveLabel = "⭐ Saved"
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		navRow("list:page:", page, total, 1),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(saveLabel, fmt.Sprintf("list:bookmark:%d", item.MessageID)),
			tgbotapi.NewInlineKeyboardButtonURL("🔗 Open", item.Link),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📄 Export all to .md", "list:export"),
		),
		tgbotapi.NewInlineKeyboardRow(backButton("menu:home")),
	)
}

// keyboard with just a Back button
func homeOnlyKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(backButton("menu:home")))
}
