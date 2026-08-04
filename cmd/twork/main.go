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
	"github.com/PeacexF/Twork/internal/broadcaster"
	"github.com/PeacexF/Twork/internal/collector"
	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/discovery"
	"github.com/PeacexF/Twork/internal/matcher"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/rsshub"
	"github.com/PeacexF/Twork/internal/storage"
	"github.com/PeacexF/Twork/internal/web"
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
	if err := bootstrapCompliance(ctx, store, cfg); err != nil {
		return fmt.Errorf("bootstrapping compliance settings: %w", err)
	}
	printComplianceWarning(cfg)

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

	// Resume broadcasting only works with a source that can actually post
	// into a chat -- the MTProto collector can, RSSHub (read-only) can't.
	var sender broadcaster.Sender
	if s, ok := source.(broadcaster.Sender); ok {
		sender = s
	} else {
		log.Printf("twork: resume broadcasting is unavailable with source=%s (read-only)", cfg.Source.Kind)
	}
	broadcasterSvc := broadcaster.New(store, sender)

	runners := []func(context.Context) error{source.Run, b.Run, b.RunDigestScheduler, broadcasterSvc.Run}
	if cfg.Web.Enabled {
		webSvc := web.New(store, source, sender, cfg.Web)
		runners = append(runners, webSvc.Run)
		log.Printf("twork: web dashboard listening on %s", cfg.Web.Addr)
	}

	log.Printf("twork: starting with source=%s", cfg.Source.Kind)
	return runConcurrently(ctx, stop, runners...)
}

// prints a startup warning about the resume broadcasting feature's spam/ban
// risk -- always shown, not configurable away
func printComplianceWarning(cfg *config.Config) {
	log.Printf(`twork: ================================================================
twork:  Resume broadcasting can post messages into Telegram groups on
twork:  your behalf. Do NOT use this to spam. Telegram can permanently
twork:  limit or ban accounts for unsolicited or repetitive messages --
twork:  this is a real risk to the account running Twork.
twork:
twork:  Current limits: minimum %ds between sends into the same group,
twork:  maximum %d sends/hour across all groups combined. Lowering these
twork:  is strongly discouraged, even on a Telegram Premium account.
twork: ================================================================`,
		cfg.Compliance.MinDelaySeconds, cfg.Compliance.MaxPerHour)
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
	return matcher.NewStore(bot.MatcherFromKeywords(kw)), nil
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

// seeds the notifications toggle from config.yaml on first run
func bootstrapNotifications(ctx context.Context, store *storage.Store, cfg *config.Config) error {
	if _, ok, err := store.GetNotificationsEnabled(ctx); err != nil {
		return err
	} else if !ok {
		return store.SetNotificationsEnabled(ctx, cfg.Notifications.Enabled)
	}
	return nil
}

// seeds the resume broadcasting compliance limits from config.yaml on first
// run; after that the bot/web dashboard are authoritative
func bootstrapCompliance(ctx context.Context, store *storage.Store, cfg *config.Config) error {
	configured, err := store.ResumeComplianceConfigured(ctx)
	if err != nil {
		return err
	}
	if configured {
		return nil
	}
	if err := store.SetResumeMinDelaySeconds(ctx, cfg.Compliance.MinDelaySeconds); err != nil {
		return err
	}
	return store.SetResumeMaxPerHour(ctx, cfg.Compliance.MaxPerHour)
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
