package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PeacexF/Twork/internal/bot"
	"github.com/PeacexF/Twork/internal/collector"
	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/matcher"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	if err := run(*configPath); err != nil {
		log.Fatalf("twork: %v", err)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, err := storage.Open(cfg.Database.SQLite)
	if err != nil {
		return fmt.Errorf("opening storage: %w", err)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	matchStore, err := bootstrapMatcher(ctx, store, cfg)
	if err != nil {
		return fmt.Errorf("bootstrapping matcher: %w", err)
	}
	if err := bootstrapNotifications(ctx, store, cfg); err != nil {
		return fmt.Errorf("bootstrapping notification settings: %w", err)
	}

	var b *bot.Bot

	onMessage := func(ctx context.Context, msg models.Message, live bool) error {

		if dup, err := store.IsDuplicate(ctx, msg.Text); err == nil && dup {
			log.Printf("twork: duplicate text detected (chat=%s, msg=%d)", msg.ChatTitle, msg.TelegramMessageID)
		}

		id, err := store.InsertMessage(ctx, msg)
		if err != nil {
			return fmt.Errorf("inserting message: %w", err)
		}

		res := matchStore.Get().Match(msg.Text)
		if !res.Matched() {
			return nil
		}

		keywordsJSON, err := json.Marshal(res.MatchedKeywords)
		if err != nil {
			return fmt.Errorf("encoding matched keywords: %w", err)
		}
		if err := store.RecordMatch(ctx, id, string(keywordsJSON)); err != nil {
			return fmt.Errorf("recording match: %w", err)
		}

		if live && b != nil {
			b.NotifyMatch(ctx, msg, id, res.MatchedKeywords)
		}
		return nil
	}

	coll := collector.New(cfg.Telegram, store, cfg.Chats, onMessage)

	built, err := bot.New(cfg.Bot.Token, cfg.Bot.OwnerID, store, matchStore, coll)
	if err != nil {
		return fmt.Errorf("building bot: %w", err)
	}
	b = built

	log.Printf("twork: starting")
	return runConcurrently(ctx, stop, coll.Run, b.Run)
}

func bootstrapMatcher(ctx context.Context, store *storage.Store, cfg *config.Config) (*matcher.Store, error) {
	kw, ok, err := store.GetKeywords(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		kw = storage.Keywords{Positive: cfg.Matching.Positive, Negative: cfg.Matching.Negative, Mode: cfg.Matching.Mode}
		if err := store.SetKeywords(ctx, kw); err != nil {
			return nil, err
		}
	}
	m, err := matcher.New(config.MatchingConfig{Positive: kw.Positive, Negative: kw.Negative, Mode: kw.Mode})
	if err != nil {
		return nil, err
	}
	return matcher.NewStore(m), nil
}

func bootstrapNotifications(ctx context.Context, store *storage.Store, cfg *config.Config) error {
	if _, ok, err := store.GetNotificationsEnabled(ctx); err != nil {
		return err
	} else if !ok {
		return store.SetNotificationsEnabled(ctx, cfg.Notifications.Enabled)
	}
	return nil
}

func runConcurrently(ctx context.Context, stop context.CancelFunc, fns ...func(context.Context) error) error {
	errCh := make(chan error, len(fns))
	for _, fn := range fns {
		fn := fn
		go func() { errCh <- fn(ctx) }()
	}

	first := <-errCh
	stop()
	for i := 1; i < len(fns); i++ {
		<-errCh
	}

	if first != nil && !errors.Is(first, context.Canceled) {
		return first
	}
	return nil
}
