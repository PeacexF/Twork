// Package rsshub implements Twork's alternative, non-MTProto chat source: polling a self-hosted RSSHub instance
package rsshub

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

// live is false only for a chat's first poll (its current feed window is a backfill, not new posts)
type Handler func(ctx context.Context, msg models.Message, live bool) error

type pollState struct {
	chat   models.Chat
	cancel context.CancelFunc
}

// polls RSSHub feeds on an interval, standing in for the MTProto collector
type Source struct {
	baseURL      string
	pollInterval time.Duration
	store        *storage.Store
	onMsg        Handler
	httpClient   *http.Client

	mu        sync.RWMutex
	monitored map[int64]*pollState

	runCtx context.Context
}

// builds a Source from RSSHub config
func New(cfg config.RSSHubConfig, store *storage.Store, onMsg Handler) *Source {
	interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	return &Source{
		baseURL:      cfg.BaseURL,
		pollInterval: interval,
		store:        store,
		onMsg:        onMsg,
		httpClient:   &http.Client{Timeout: 20 * time.Second},
		monitored:    make(map[int64]*pollState),
	}
}

// loads monitored chats from storage and starts polling each one until ctx is cancelled
func (s *Source) Run(ctx context.Context) error {
	s.runCtx = ctx

	chats, err := s.store.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("loading monitored chats: %w", err)
	}
	for _, c := range chats {
		if c.Username == "" {
			continue // this row wasn't added through the RSSHub source (or has no public username) -- nothing to poll
		}
		if c.Paused {
			s.mu.Lock()
			s.monitored[c.TelegramID] = &pollState{chat: c}
			s.mu.Unlock()
			continue
		}
		s.startPolling(ctx, c, nil)
	}

	<-ctx.Done()
	return ctx.Err()
}

// resolves a public channel/group by username, validating it against RSSHub before monitoring it
func (s *Source) AddByUsername(ctx context.Context, username string) (*models.Chat, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, fmt.Errorf("username is empty")
	}

	feed, err := s.fetchFeed(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("checking @%s against RSSHub: %w", username, err)
	}

	title := username
	if feed.Title != "" {
		title = feed.Title
	}

	chat := models.Chat{
		TelegramID: syntheticChatID(username),
		Kind:       models.ChatKindChannel,
		Title:      title,
		Username:   username,
	}
	if err := s.store.UpsertChat(ctx, chat); err != nil {
		return nil, fmt.Errorf("saving chat: %w", err)
	}

	if s.runCtx != nil {
		s.startPolling(s.runCtx, chat, feed) // reuse the fetch we already did as the first (backfill) pass
	}

	return &chat, nil
}

// private invite links have no RSSHub equivalent -- only public usernames can be polled this way
func (s *Source) AddByInviteLink(ctx context.Context, link string) (*models.Chat, error) {
	return nil, fmt.Errorf("the RSSHub source only supports public @usernames, not invite links (RSSHub's telegram route can't read private chats)")
}

// shared folders have no RSSHub equivalent either
func (s *Source) AddFolder(ctx context.Context, link string) ([]*models.Chat, error) {
	return nil, fmt.Errorf("the RSSHub source doesn't support folder imports -- add channels one at a time by username")
}

// stops polling a chat without forgetting it
func (s *Source) Pause(ctx context.Context, telegramID int64) error {
	if err := s.store.SetChatPaused(ctx, telegramID, true); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.monitored[telegramID]; ok && st.cancel != nil {
		st.cancel()
		st.cancel = nil
		st.chat.Paused = true
	}
	return nil
}

// resumes polling a paused chat
func (s *Source) Resume(ctx context.Context, telegramID int64) error {
	if err := s.store.SetChatPaused(ctx, telegramID, false); err != nil {
		return err
	}
	s.mu.RLock()
	st, ok := s.monitored[telegramID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	st.chat.Paused = false
	if s.runCtx != nil {
		s.startPolling(s.runCtx, st.chat, nil)
	}
	return nil
}

// stops polling a chat and removes it from the monitored list, keeping its indexed history
func (s *Source) Remove(ctx context.Context, telegramID int64) error {
	if err := s.store.RemoveChat(ctx, telegramID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.monitored[telegramID]; ok && st.cancel != nil {
		st.cancel()
	}
	delete(s.monitored, telegramID)
	return nil
}

// snapshots the currently monitored chats
func (s *Source) ListResolved() []models.Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Chat, 0, len(s.monitored))
	for _, st := range s.monitored {
		out = append(out, st.chat)
	}
	return out
}

// registers a chat and launches its polling goroutine, reusing initialFeed as the first pass if already fetched
func (s *Source) startPolling(parentCtx context.Context, chat models.Chat, initialFeed *gofeed.Feed) {
	pollCtx, cancel := context.WithCancel(parentCtx)

	s.mu.Lock()
	s.monitored[chat.TelegramID] = &pollState{chat: chat, cancel: cancel}
	s.mu.Unlock()

	go func() {
		if initialFeed != nil {
			s.ingest(pollCtx, chat, initialFeed, false)
		} else if feed, err := s.fetchFeed(pollCtx, chat.Username); err == nil {
			s.ingest(pollCtx, chat, feed, false)
		} else {
			log.Printf("twork/rsshub: initial poll failed for %q: %v", chat.Title, err)
		}

		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				feed, err := s.fetchFeed(pollCtx, chat.Username)
				if err != nil {
					log.Printf("twork/rsshub: poll failed for %q: %v", chat.Title, err)
					continue
				}
				s.ingest(pollCtx, chat, feed, true)
			}
		}
	}()
}

// fetches and parses one chat's RSSHub feed
func (s *Source) fetchFeed(ctx context.Context, username string) (*gofeed.Feed, error) {
	url := strings.ReplaceAll(s.baseURL, "{channel}", username)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Twork (RSSHub Telegram collector)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RSSHub returned HTTP %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	feed, err := gofeed.NewParser().ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("parsing feed: %w", err)
	}
	return feed, nil
}

// normalizes and hands every item in a fetched feed to onMsg
func (s *Source) ingest(ctx context.Context, chat models.Chat, feed *gofeed.Feed, live bool) {
	for _, item := range feed.Items {
		msg := normalizeMessage(item, chat)
		if err := s.onMsg(ctx, msg, live); err != nil {
			log.Printf("twork/rsshub: handling item failed for %q: %v", chat.Title, err)
		}
	}
}

// converts one parsed feed item into Twork's message model
func normalizeMessage(item *gofeed.Item, chat models.Chat) models.Message {
	raw := item.Content
	if raw == "" {
		raw = item.Description
	}
	ts := time.Now().UTC()
	if item.PublishedParsed != nil {
		ts = item.PublishedParsed.UTC()
	} else if item.UpdatedParsed != nil {
		ts = item.UpdatedParsed.UTC()
	}

	return models.Message{
		TelegramMessageID: syntheticMessageID(item),
		ChatID:            chat.TelegramID,
		ChatTitle:         chat.Title,
		Timestamp:         ts,
		Text:              parseEntryHTML(raw),
		Link:              item.Link,
	}
}

// hashes a username into a stable, always-negative chat ID (RSSHub has no real Telegram ID)
func syntheticChatID(username string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(username)))
	v := int64(h.Sum64() >> 1) // clear the sign bit
	return -v - 1
}

// hashes an item's GUID/link/title into a stable synthetic message ID
func syntheticMessageID(item *gofeed.Item) int {
	key := item.GUID
	if key == "" {
		key = item.Link
	}
	if key == "" {
		key = item.Title
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	v := int(h.Sum32() >> 1) // fits safely as a positive value in Go's (>=32-bit) int
	if v == 0 {
		v = 1
	}
	return v
}
