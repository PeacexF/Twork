package bot

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// routes settings: callbacks
func (b *Bot) handleSettingsCallback(ctx context.Context, data string) {
	if data == "settings:notify_toggle" {
		enabled, _, err := b.store.GetNotificationsEnabled(ctx)
		if err != nil {
			log.Printf("twork: reading notification setting failed: %v", err)
		}
		if err := b.store.SetNotificationsEnabled(ctx, !enabled); err != nil {
			log.Printf("twork: saving notification setting failed: %v", err)
		}
	}
	b.showSettingsMenu(ctx)
}

// renders the settings menu
func (b *Bot) showSettingsMenu(ctx context.Context) {
	enabled, _, err := b.store.GetNotificationsEnabled(ctx)
	if err != nil {
		log.Printf("twork: reading notification setting failed: %v", err)
	}
	label := "🔕 Notifications: OFF (tap to enable)"
	if enabled {
		label = "🔔 Notifications: ON (tap to disable)"
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(label, "settings:notify_toggle")),
		tgbotapi.NewInlineKeyboardRow(backButton("menu:home")),
	)
	b.editHome(ctx, "⚙️ Settings", kb)
}
