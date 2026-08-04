// Package broadcaster periodically re-posts the user's resume into monitored
// GROUPS -- never channels (broadcast-only, wrong blast radius) and never
// DMs (structurally unreachable -- see collector.Collector.SendText).
package broadcaster

import (
	"context"
	"log"
	"time"

	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

const tickInterval = 30 * time.Second

// Sender posts a text message into a chat the account already monitors.
// Only the MTProto collector implements this -- RSSHub is read-only.
type Sender interface {
	SendText(ctx context.Context, telegramChatID int64, text string) error
}

// periodically re-posts the resume into every group with broadcasting enabled
type Broadcaster struct {
	store  *storage.Store
	sender Sender // nil when the active source can't send (e.g. RSSHub)
}

// builds a Broadcaster; sender may be nil if the active chat source can't send
func New(store *storage.Store, sender Sender) *Broadcaster {
	return &Broadcaster{store: store, sender: sender}
}

// runs the broadcast scheduler until ctx is cancelled; a no-op if sender is nil
func (b *Broadcaster) Run(ctx context.Context) error {
	if b.sender == nil {
		<-ctx.Done()
		return ctx.Err()
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			b.tick(ctx, now)
		}
	}
}

// sends the resume into every due chat, respecting the per-chat cooldown
// (never below the configured floor) and the rolling global hourly cap
func (b *Broadcaster) tick(ctx context.Context, now time.Time) {
	minDelay, err := b.store.GetResumeMinDelaySeconds(ctx, 300)
	if err != nil {
		log.Printf("twork/broadcaster: reading min delay failed: %v", err)
		return
	}
	maxPerHour, err := b.store.GetResumeMaxPerHour(ctx, 10)
	if err != nil {
		log.Printf("twork/broadcaster: reading max per hour failed: %v", err)
		return
	}
	sentThisHour, err := b.store.CountResumeSendsSince(ctx, now.Add(-time.Hour))
	if err != nil {
		log.Printf("twork/broadcaster: counting recent sends failed: %v", err)
		return
	}
	chats, err := b.store.ListResumeEnabledChats(ctx)
	if err != nil {
		log.Printf("twork/broadcaster: listing resume-enabled chats failed: %v", err)
		return
	}
	globalText, err := b.store.GetResumeGlobalText(ctx)
	if err != nil {
		log.Printf("twork/broadcaster: reading global resume text failed: %v", err)
		return
	}
	floor := time.Duration(minDelay) * time.Second

	for _, c := range chats {
		if sentThisHour >= maxPerHour {
			return // global cap reached -- nothing else sends until the window rolls
		}

		// Defense in depth: storage.SetChatResumeConfig already refuses to
		// enable broadcasting on anything but a group, so this should be
		// unreachable -- but the scheduler must never trust that alone. A
		// channel (or, structurally impossible today, a DM) must never
		// receive a broadcast.
		if c.Kind != models.ChatKindGroup {
			log.Printf("twork/broadcaster: refusing to send to chat %d (%q): resume broadcasting is group-only, got kind %q", c.TelegramID, c.Title, c.Kind)
			continue
		}

		interval := time.Duration(c.ResumeIntervalSeconds) * time.Second
		if interval < floor {
			interval = floor
		}
		if c.LastSentAt != nil && now.Sub(*c.LastSentAt) < interval {
			continue
		}

		text := c.ResumeText
		if text == "" {
			text = globalText
		}
		if text == "" {
			continue // nothing configured to send yet
		}

		if err := b.sender.SendText(ctx, c.TelegramID, text); err != nil {
			log.Printf("twork/broadcaster: sending resume to %q failed: %v", c.Title, err)
			continue // a failed send shouldn't burn the chat's cooldown or the hourly budget
		}
		if err := b.store.RecordResumeSend(ctx, c.TelegramID); err != nil {
			log.Printf("twork/broadcaster: recording resume send for %q failed: %v", c.Title, err)
		}
		sentThisHour++
	}
}
