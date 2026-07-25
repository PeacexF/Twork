package bot

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/discovery"
	"github.com/PeacexF/Twork/internal/matcher"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

const pageSize = 8

// backs chat monitoring; the MTProto collector and RSSHub poller both satisfy this
type ChatSource interface {
	Run(ctx context.Context) error
	AddByUsername(ctx context.Context, username string) (*models.Chat, error)
	AddByInviteLink(ctx context.Context, link string) (*models.Chat, error)
	AddFolder(ctx context.Context, link string) ([]*models.Chat, error)
	Pause(ctx context.Context, telegramID int64) error
	Resume(ctx context.Context, telegramID int64) error
	Remove(ctx context.Context, telegramID int64) error
	ListResolved() []models.Chat
}

type Bot struct {
	api        *tgbotapi.BotAPI
	store      *storage.Store
	matchStore *matcher.Store
	source     ChatSource
	searcher   discovery.Searcher // nil if channel discovery isn't configured

	configuredOwnerID int64
	ownerID           int64
	sess              *session
}

// builds a Bot around the given token and dependencies
func New(token string, ownerID int64, store *storage.Store, matchStore *matcher.Store, source ChatSource, searcher discovery.Searcher) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("connecting to Telegram Bot API: %w", err)
	}
	return &Bot{
		api:               api,
		store:             store,
		matchStore:        matchStore,
		source:            source,
		searcher:          searcher,
		configuredOwnerID: ownerID,
		sess:              &session{},
	}, nil
}

// resolves the owner, then processes updates until ctx is cancelled
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

// routes one update to the message or callback handler
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

// handles /start, /help, and replies to an active prompt
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

// reports whether userID is the claimed owner
func (b *Bot) authorized(userID int64) bool {
	return b.ownerID != 0 && userID == b.ownerID
}

// claims ownership if unclaimed, then opens the home menu
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

// sends the help text
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

// routes a button press by its callback_data
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

// routes callback_data by its namespace prefix
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

// reports whether s starts with prefix
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
