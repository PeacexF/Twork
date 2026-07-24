package bot

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/storage"
)

// renders the stats dashboard text
func homeDashboardText(ctx context.Context, store *storage.Store) string {
	stats, err := store.GetStats(ctx)
	if err != nil {
		return "TWORK\n\n(couldn't load stats right now)"
	}
	return fmt.Sprintf(
		"🔧 TWORK\n\nChats monitored: %d\nMessages indexed: %d\nToday's matches: %d\nBookmarks: %d\nIgnored: %d",
		stats.ChatsMonitored, stats.MessagesIndexed, stats.TodayMatches, stats.Bookmarks, stats.Ignored,
	)
}

// builds the main menu keyboard
func homeDashboardKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Matches", "menu:matches"),
			tgbotapi.NewInlineKeyboardButtonData("⭐ Favorites", "menu:favorites"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 Search", "menu:search"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📡 Chats", "menu:chats"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔑 Keywords", "menu:keywords"),
			tgbotapi.NewInlineKeyboardButtonData("⚙️ Settings", "menu:settings"),
		),
	)
}
