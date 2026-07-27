package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/storage"
)

// runs the daily-digest scheduler until ctx is cancelled
func (b *Bot) RunDigestScheduler(ctx context.Context) error {
	// Re-check every minute rather than sleeping until the target time,
	// so a digest-time change from Settings takes effect within a
	// minute instead of only on the next restart.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	var lastSentDay string // "2006-01-02" of the last digest, to fire once per day

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			if b.shouldSendDigest(ctx, now, lastSentDay) {
				b.sendDigest(ctx, now)
				lastSentDay = now.Format("2006-01-02")
			}
		}
	}
}

// reports whether the digest is due now: mode includes digest, the
// configured HH:MM has arrived (server local time), and none has been
// sent yet today
func (b *Bot) shouldSendDigest(ctx context.Context, now time.Time, lastSentDay string) bool {
	if b.ownerID == 0 {
		return false
	}
	if now.Format("2006-01-02") == lastSentDay {
		return false
	}
	mode, err := b.store.GetNotificationMode(ctx)
	if err != nil || (mode != storage.NotifyModeDigest && mode != storage.NotifyModeBoth) {
		return false
	}
	target, err := b.store.GetDigestTime(ctx)
	if err != nil {
		return false
	}
	return now.Format("15:04") == target
}

// builds and sends the last-24h digest to the owner
func (b *Bot) sendDigest(ctx context.Context, now time.Time) {
	since := now.Add(-24 * time.Hour)

	newPosts, err := b.store.CountMessagesSince(ctx, since)
	if err != nil {
		log.Printf("twork: digest count failed: %v", err)
		return
	}
	matches, err := b.store.MatchesSince(ctx, since, 50)
	if err != nil {
		log.Printf("twork: digest matches failed: %v", err)
		return
	}

	text := b.formatDigest(newPosts, matches)
	msg := tgbotapi.NewMessage(b.ownerID, text)
	msg.DisableWebPagePreview = true
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("twork: sending digest failed: %v", err)
	}
}

// renders the digest text: a header count plus a list of the day's matches
func (b *Bot) formatDigest(newPosts int, matches []storage.MatchRow) string {
	header := fmt.Sprintf("📰 Daily digest\n\n%d new post(s) in the last 24h, %d matched your keywords.", newPosts, len(matches))
	if len(matches) == 0 {
		return header + "\n\nNothing matched today."
	}

	var sb = []byte(header + "\n")
	for i, m := range matches {
		snippet := firstLine(m.Text, 120)
		entry := fmt.Sprintf("\n%d. %s\n%s", i+1, m.ChatTitle, snippet)
		if m.Link != "" {
			entry += "\n" + m.Link
		}
		sb = append(sb, entry...)
		sb = append(sb, '\n')
	}
	return string(sb)
}

// returns the first line of text, truncated to max runes with an ellipsis
func firstLine(text string, max int) string {
	for i, r := range text {
		if r == '\n' {
			text = text[:i]
			break
		}
	}
	runes := []rune(text)
	if len(runes) > max {
		return string(runes[:max]) + "…"
	}
	return text
}
