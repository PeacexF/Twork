package bot

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/collector"
	"github.com/PeacexF/Twork/internal/matcher"
	"github.com/PeacexF/Twork/internal/storage"
)

const pageSize = 8

type Bot struct {
	api        *tgbotapi.BotAPI
	store      *storage.Store
	matchStore *matcher.Store
	coll       *collector.Collector

	configuredOwnerID int64
	ownerID           int64
	sess              *session
}

func New(token string, ownerID int64, store *storage.Store, matchStore *matcher.Store, coll *collector.Collector) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("connecting to Telegram Bot API: %w", err)
	}
	return &Bot{
		api:               api,
		store:             store,
		matchStore:        matchStore,
		coll:              coll,
		configuredOwnerID: ownerID,
		sess:              &session{},
	}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	if b.configuredOwnerID != 0 {
		b.ownerID = b.configuredOwnerID
		if err := b.store.SetBotOwnerID(ctx, b.configuredOwnerID); err != nil {
			return fmt.Errorf("persisting configured owner id: %w", err)
		}
	} else if id, ok, err := b.store.GetBotOwnerID(ctx); err != nil {
		return fmt.Errorf("loading bot owner: %w", err)
	} else if ok {
		b.ownerID = id
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)

	log.Printf("twork: bot @%s is running", b.api.Self.UserName)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return ctx.Err()
		case update := <-updates:
			b.dispatch(ctx, update)
		}
	}
}

func (b *Bot) dispatch(ctx context.Context, update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("twork: bot handler panic recovered: %v", r)
		}
	}()

	switch {
	case update.Message != nil:
		b.handleMessage(ctx, update.Message)
	case update.CallbackQuery != nil:
		b.handleCallback(ctx, update.CallbackQuery)
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			b.handleStart(ctx, msg)
		case "help":
			b.handleHelp(ctx, msg)
		}
		return
	}

	if !b.authorized(msg.From.ID) {
		return
	}
	if b.sess.pending == inputNone {
		return
	}
	b.handlePendingInput(ctx, msg)
}

func (b *Bot) authorized(userID int64) bool {
	return b.ownerID != 0 && userID == b.ownerID
}

func (b *Bot) handleStart(ctx context.Context, msg *tgbotapi.Message) {
	if b.ownerID == 0 {
		b.ownerID = msg.From.ID
		if err := b.store.SetBotOwnerID(ctx, msg.From.ID); err != nil {
			log.Printf("twork: failed to persist claimed owner id: %v", err)
		}
		b.send(msg.Chat.ID, "You're now the owner of this Twork bot. Only you can control it from here on.")
	} else if !b.authorized(msg.From.ID) {
		b.send(msg.Chat.ID, "This Twork bot is private.")
		return
	}
	b.sess.homeChatID = msg.Chat.ID
	b.openHome(ctx, msg.Chat.ID)
}

func (b *Bot) handleHelp(ctx context.Context, msg *tgbotapi.Message) {
	if !b.authorized(msg.From.ID) {
		return
	}
	b.send(msg.Chat.ID, "Twork is controlled entirely with buttons -- send /start to open the menu.\n\n"+
		"📋 Matches -- everything that matched your keywords\n"+
		"⭐ Favorites -- posts you've saved\n"+
		"🔍 Search -- full-text search across every indexed message\n"+
		"📡 Chats -- add, pause, resume, or remove monitored channels/groups\n"+
		"🔑 Keywords -- edit your positive/negative keyword lists\n"+
		"⚙️ Settings -- notifications and matching options")
}

func (b *Bot) handleCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) {
	if !b.authorized(cq.From.ID) {
		b.answerCallback(cq.ID, "")
		return
	}

	defer b.answerCallback(cq.ID, "")

	data := cq.Data
	switch {
	case data == "menu:home":
		b.editHome(ctx, homeDashboardText(ctx, b.store), homeDashboardKeyboard())
	case data == "menu:chats":
		b.showChatsList(ctx, 0)
	case data == "menu:matches":
		b.openCarousel(ctx, viewMatches, 0)
	case data == "menu:favorites":
		b.openCarousel(ctx, viewFavorites, 0)
	case data == "menu:search":
		b.promptFor(ctx, inputSearchQuery, "Send the search query (supports AND / OR / NOT and \"quoted phrases\"):")
	case data == "menu:keywords":
		b.showKeywordsMenu(ctx)
	case data == "menu:settings":
		b.showSettingsMenu(ctx)
	default:
		b.routeNamespaced(ctx, cq, data)
	}
}

func (b *Bot) routeNamespaced(ctx context.Context, cq *tgbotapi.CallbackQuery, data string) {
	switch {
	case hasPrefix(data, "chat:"):
		b.handleChatCallback(ctx, data)
	case hasPrefix(data, "list:"):
		b.handleCarouselCallback(ctx, data)
	case hasPrefix(data, "kw:"):
		b.handleKeywordsCallback(ctx, data)
	case hasPrefix(data, "settings:"):
		b.handleSettingsCallback(ctx, data)
	case hasPrefix(data, "notify:"):
		b.handleNotifyCallback(ctx, cq, data)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
