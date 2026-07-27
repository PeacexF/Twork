package bot

import (
	"context"
	"fmt"
	"log"
	"regexp"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/storage"
)

var digestTimeRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// routes settings: callbacks
func (b *Bot) handleSettingsCallback(ctx context.Context, data string) {
	switch data {
	case "settings:notify_toggle":
		enabled, _, err := b.store.GetNotificationsEnabled(ctx)
		if err != nil {
			log.Printf("twork: reading notification setting failed: %v", err)
		}
		if err := b.store.SetNotificationsEnabled(ctx, !enabled); err != nil {
			log.Printf("twork: saving notification setting failed: %v", err)
		}
	case "settings:mode_cycle":
		b.cycleNotificationMode(ctx)
	case "settings:digest_time":
		b.promptFor(ctx, inputDigestTime, "Send the daily digest time in 24h HH:MM format (server local time), e.g. 09:00:")
	}
	b.showSettingsMenu(ctx)
}

// cycles notification mode: live -> digest -> both -> live
func (b *Bot) cycleNotificationMode(ctx context.Context) {
	mode, err := b.store.GetNotificationMode(ctx)
	if err != nil {
		log.Printf("twork: reading notification mode failed: %v", err)
	}
	next := storage.NotifyModeLive
	switch mode {
	case storage.NotifyModeLive:
		next = storage.NotifyModeDigest
	case storage.NotifyModeDigest:
		next = storage.NotifyModeBoth
	case storage.NotifyModeBoth:
		next = storage.NotifyModeLive
	}
	if err := b.store.SetNotificationMode(ctx, next); err != nil {
		log.Printf("twork: saving notification mode failed: %v", err)
	}
}

// validates and stores a digest time entered by the user
func (b *Bot) setDigestTime(ctx context.Context, hhmm string) {
	if !digestTimeRe.MatchString(hhmm) {
		b.editHome(ctx, "⚠️ That's not a valid time. Use 24h HH:MM, e.g. 09:00 or 18:30.", tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(backButton("menu:settings")),
		))
		return
	}
	if err := b.store.SetDigestTime(ctx, hhmm); err != nil {
		log.Printf("twork: saving digest time failed: %v", err)
	}
	b.showSettingsMenu(ctx)
}

// renders the settings menu
func (b *Bot) showSettingsMenu(ctx context.Context) {
	enabled, _, err := b.store.GetNotificationsEnabled(ctx)
	if err != nil {
		log.Printf("twork: reading notification setting failed: %v", err)
	}
	mode, err := b.store.GetNotificationMode(ctx)
	if err != nil {
		log.Printf("twork: reading notification mode failed: %v", err)
	}
	digestTime, err := b.store.GetDigestTime(ctx)
	if err != nil {
		log.Printf("twork: reading digest time failed: %v", err)
	}

	toggleLabel := "🔕 Notifications: OFF"
	if enabled {
		toggleLabel = "🔔 Notifications: ON"
	}

	rows := [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData(toggleLabel, "settings:notify_toggle")},
		{tgbotapi.NewInlineKeyboardButtonData("📣 Mode: "+notifyModeLabel(mode), "settings:mode_cycle")},
	}
	// The digest time only matters when the mode actually sends a digest.
	if mode == storage.NotifyModeDigest || mode == storage.NotifyModeBoth {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("🕘 Digest time: "+digestTime, "settings:digest_time"),
		})
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{backButton("menu:home")})

	b.editHome(ctx, fmt.Sprintf("⚙️ Settings\n\nHow matches reach you: %s", notifyModeDescription(mode)), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// short label for a notification mode
func notifyModeLabel(mode string) string {
	switch mode {
	case storage.NotifyModeDigest:
		return "digest only"
	case storage.NotifyModeBoth:
		return "live + digest"
	default:
		return "live"
	}
}

// one-line description of what a mode does
func notifyModeDescription(mode string) string {
	switch mode {
	case storage.NotifyModeDigest:
		return "one daily digest, no live pings"
	case storage.NotifyModeBoth:
		return "a ping per match AND a daily digest"
	default:
		return "a ping for each match as it arrives"
	}
}
