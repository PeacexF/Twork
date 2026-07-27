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
	"github.com/PeacexF/Twork/internal/discovery"
	"github.com/PeacexF/Twork/internal/matcher"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/rsshub"
	"github.com/PeacexF/Twork/internal/storage"
)

// parses flags and runs the app
func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	if err := run(*configPath); err != nil {
		log.Fatalf("twork: %v", err)
	}
}

// wires config, storage, matcher, collector, and bot together and blocks until shutdown
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
		id, err := store.InsertMessage(ctx, msg)
		if err != nil {
			if errors.Is(err, storage.ErrDuplicate) {
				return nil // global dedup: never index or match the same text twice
			}
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

	var source bot.ChatSource
	switch cfg.Source.Kind {
	case config.SourceRSSHub:
		source = rsshub.New(cfg.RSSHub, store, onMessage)
	default:
		source = collector.New(cfg.Telegram, store, cfg.Chats, onMessage)
	}

	var searcher discovery.Searcher
	if tg := discovery.NewTGStat(cfg.Discovery.TGStatToken); tg != nil {
		searcher = tg
	}

	built, err := bot.New(cfg.Bot.Token, cfg.Bot.OwnerID, store, matchStore, source, searcher)
	if err != nil {
		return fmt.Errorf("building bot: %w", err)
	}
	b = built

	log.Printf("twork: starting with source=%s", cfg.Source.Kind)
	return runConcurrently(ctx, stop, source.Run, b.Run, b.RunDigestScheduler)
}

// loads keywords from storage, seeding from config.yaml on first run
func bootstrapMatcher(ctx context.Context, store *storage.Store, cfg *config.Config) (*matcher.Store, error) {
	kw, ok, err := store.GetKeywords(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		kw = seedKeywordsFromConfig(cfg.Matching)
		if err := store.SetKeywords(ctx, kw); err != nil {
			return nil, err
		}
	}
	return matcher.NewStore(matcherFromStored(kw)), nil
}

// builds the initial stored keyword set from config, folding flat lists into single-alias groups
func seedKeywordsFromConfig(m config.MatchingConfig) storage.Keywords {
	kw := storage.Keywords{Mode: m.Mode}
	for _, w := range m.Positive {
		kw.PositiveGroups = append(kw.PositiveGroups, storage.KeywordGroup{Name: w, Aliases: []string{w}})
	}
	for _, g := range m.PositiveGroups {
		kw.PositiveGroups = append(kw.PositiveGroups, storage.KeywordGroup(g))
	}
	for _, w := range m.Negative {
		kw.NegativeGroups = append(kw.NegativeGroups, storage.KeywordGroup{Name: w, Aliases: []string{w}})
	}
	for _, g := range m.NegativeGroups {
		kw.NegativeGroups = append(kw.NegativeGroups, storage.KeywordGroup(g))
	}
	return kw
}

// builds a Matcher from stored keyword groups
func matcherFromStored(kw storage.Keywords) *matcher.Matcher {
	return matcher.NewFromGroups(kw.Mode, storedToConfigGroups(kw.PositiveGroups), storedToConfigGroups(kw.NegativeGroups))
}

// converts storage keyword groups to the config type the matcher consumes
func storedToConfigGroups(in []storage.KeywordGroup) []config.KeywordGroup {
	out := make([]config.KeywordGroup, len(in))
	for i, g := range in {
		out[i] = config.KeywordGroup(g)
	}
	return out
}

// seeds the notifications toggle from config.yaml on first run
func bootstrapNotifications(ctx context.Context, store *storage.Store, cfg *config.Config) error {
	if _, ok, err := store.GetNotificationsEnabled(ctx); err != nil {
		return err
	} else if !ok {
		return store.SetNotificationsEnabled(ctx, cfg.Notifications.Enabled)
	}
	return nil
}

// runs fns concurrently, cancelling all of them on the first return
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
