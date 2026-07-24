package collector

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/gotd/td/tg"

	"github.com/PeacexF/Twork/internal/models"
)

// resolves and starts monitoring a public chat by username
func (c *Collector) AddByUsername(ctx context.Context, username string) (*models.Chat, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, fmt.Errorf("username is empty")
	}

	resolved, err := c.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, fmt.Errorf("resolving @%s: %w", username, err)
	}

	var rc *resolvedChat
	for _, ch := range resolved.Chats {
		if candidate := chatToResolved(ch); candidate != nil {
			rc = candidate
			break
		}
	}
	if rc == nil {
		return nil, fmt.Errorf("@%s did not resolve to a channel or group (maybe it's a user account?)", username)
	}

	return c.addResolved(ctx, rc)
}

// joins (if needed) and monitors a private chat via invite link
func (c *Collector) AddByInviteLink(ctx context.Context, link string) (*models.Chat, error) {
	hash := parseInviteHash(link)
	if hash == "" {
		return nil, fmt.Errorf("couldn't find an invite hash in %q", link)
	}

	check, err := c.api.MessagesCheckChatInvite(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("checking invite: %w", err)
	}

	var chatClass tg.ChatClass
	switch v := check.(type) {
	case *tg.ChatInviteAlready:
		chatClass = v.Chat
	default:
		joinResult, err := c.api.MessagesImportChatInvite(ctx, hash)
		if err != nil {
			return nil, fmt.Errorf("joining via invite: %w", err)
		}
		ok, isOk := joinResult.(*tg.MessagesChatInviteJoinResultOk)
		if !isOk {
			return nil, fmt.Errorf("joining via invite: unsupported result type %T (e.g. a web-view invite)", joinResult)
		}
		chats := chatsFromUpdatesClass(ok.Updates)
		if len(chats) == 0 {
			return nil, fmt.Errorf("joined, but no chat info was returned")
		}
		chatClass = chats[0]
	}

	rc := chatToResolved(chatClass)
	if rc == nil {
		return nil, fmt.Errorf("invite did not resolve to a monitorable channel or group")
	}
	return c.addResolved(ctx, rc)
}

// joins and monitors every chat in a shared folder link
func (c *Collector) AddFolder(ctx context.Context, addlistLink string) ([]*models.Chat, error) {
	slug := parseChatlistSlug(addlistLink)
	if slug == "" {
		return nil, fmt.Errorf("couldn't find a folder slug in %q", addlistLink)
	}

	check, err := c.api.ChatlistsCheckChatlistInvite(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("checking folder invite: %w", err)
	}

	var (
		toJoin   []tg.PeerClass
		allChats []tg.ChatClass
		hashByID = make(map[int64]int64) // channel ID -> access hash, for building InputPeer
	)
	switch v := check.(type) {
	case *tg.ChatlistsChatlistInvite:
		toJoin = v.Peers
		allChats = v.Chats
	case *tg.ChatlistsChatlistInviteAlready:
		toJoin = v.MissingPeers
		allChats = v.Chats
	default:
		return nil, fmt.Errorf("unexpected folder invite response type %T", check)
	}
	for _, ch := range allChats {
		if channel, ok := ch.(*tg.Channel); ok {
			hashByID[channel.ID] = channel.AccessHash
		}
	}

	if len(toJoin) > 0 {
		inputPeers := make([]tg.InputPeerClass, 0, len(toJoin))
		for _, p := range toJoin {
			if pc, ok := p.(*tg.PeerChannel); ok {
				inputPeers = append(inputPeers, &tg.InputPeerChannel{ChannelID: pc.ChannelID, AccessHash: hashByID[pc.ChannelID]})
			}
		}
		if len(inputPeers) > 0 {
			if _, err := c.api.ChatlistsJoinChatlistInvite(ctx, &tg.ChatlistsJoinChatlistInviteRequest{
				Slug:  slug,
				Peers: inputPeers,
			}); err != nil {
				return nil, fmt.Errorf("joining folder chats: %w", err)
			}
		}
	}

	var added []*models.Chat
	for _, ch := range allChats {
		rc := chatToResolved(ch)
		if rc == nil {
			continue
		}
		m, err := c.addResolved(ctx, rc)
		if err != nil {
			log.Printf("twork: failed to add %q from folder: %v", rc.Title, err)
			continue
		}
		added = append(added, m)
	}
	return added, nil
}

// persists a resolved chat and kicks off its backfill
func (c *Collector) addResolved(ctx context.Context, rc *resolvedChat) (*models.Chat, error) {
	if err := c.store.UpsertChat(ctx, rc.Chat); err != nil {
		return nil, fmt.Errorf("saving chat: %w", err)
	}

	c.mu.Lock()
	c.resolved[rc.TelegramID] = rc
	c.mu.Unlock()

	c.backfillAsync(rc)

	chat := rc.Chat
	return &chat, nil
}

// runs backfill in the background using the collector's run context
func (c *Collector) backfillAsync(rc *resolvedChat) {
	if c.runCtx == nil {
		return
	}
	go func() {
		if err := c.backfill(c.runCtx, rc); err != nil {
			log.Printf("twork: backfill failed for %q: %v", rc.Title, err)
		}
	}()
}

// stops delivering new messages for a chat
func (c *Collector) Pause(ctx context.Context, telegramID int64) error {
	if err := c.store.SetChatPaused(ctx, telegramID, true); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if rc, ok := c.resolved[telegramID]; ok {
		rc.Paused = true
	}
	return nil
}

// re-enables a paused chat and re-triggers backfill
func (c *Collector) Resume(ctx context.Context, telegramID int64) error {
	if err := c.store.SetChatPaused(ctx, telegramID, false); err != nil {
		return err
	}
	c.mu.Lock()
	rc, ok := c.resolved[telegramID]
	if ok {
		rc.Paused = false
	}
	c.mu.Unlock()
	if ok {
		c.backfillAsync(rc)
	}
	return nil
}

// stops monitoring a chat without deleting its history
func (c *Collector) Remove(ctx context.Context, telegramID int64) error {
	if err := c.store.RemoveChat(ctx, telegramID); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.resolved, telegramID)
	c.mu.Unlock()
	return nil
}

// snapshots the currently monitored chats
func (c *Collector) ListResolved() []models.Chat {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]models.Chat, 0, len(c.resolved))
	for _, rc := range c.resolved {
		out = append(out, rc.Chat)
	}
	return out
}

// extracts the chat list from an Updates response
func chatsFromUpdatesClass(u tg.UpdatesClass) []tg.ChatClass {
	switch v := u.(type) {
	case *tg.Updates:
		return v.Chats
	case *tg.UpdatesCombined:
		return v.Chats
	default:
		return nil
	}
}

// extracts the hash from a private invite link
func parseInviteHash(link string) string {
	link = strings.TrimSpace(link)
	link = strings.TrimPrefix(link, "https://")
	link = strings.TrimPrefix(link, "http://")
	link = strings.TrimPrefix(link, "t.me/")
	link = strings.TrimPrefix(link, "telegram.me/")
	if strings.HasPrefix(link, "+") {
		return strings.TrimPrefix(link, "+")
	}
	if strings.HasPrefix(link, "joinchat/") {
		return strings.TrimPrefix(link, "joinchat/")
	}
	if !strings.Contains(link, "/") && link != "" {
		return link // looks like a bare hash already
	}
	return ""
}

// extracts the slug from a folder invite link
func parseChatlistSlug(link string) string {
	link = strings.TrimSpace(link)
	link = strings.TrimPrefix(link, "https://")
	link = strings.TrimPrefix(link, "http://")
	link = strings.TrimPrefix(link, "t.me/")
	link = strings.TrimPrefix(link, "telegram.me/")
	if strings.HasPrefix(link, "addlist/") {
		return strings.TrimPrefix(link, "addlist/")
	}
	if !strings.Contains(link, "/") && link != "" {
		return link
	}
	return ""
}
