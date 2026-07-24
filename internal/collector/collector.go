package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/models"
	"github.com/PeacexF/Twork/internal/storage"
)

type Handler func(ctx context.Context, msg models.Message, live bool) error

type Collector struct {
	appID   int
	appHash string
	phone   string
	session string
	seed    []config.ChatConfig
	onMsg   Handler
	store   *storage.Store

	mu       sync.RWMutex
	resolved map[int64]*resolvedChat

	runCtx context.Context
	client *telegram.Client
	api    *tg.Client
}

type resolvedChat struct {
	models.Chat
	InputPeer tg.InputPeerClass
}

func New(cfg config.TelegramConfig, store *storage.Store, seed []config.ChatConfig, onMsg Handler) *Collector {
	return &Collector{
		appID:    cfg.AppID,
		appHash:  cfg.AppHash,
		phone:    cfg.Phone,
		session:  cfg.Session,
		seed:     seed,
		onMsg:    onMsg,
		store:    store,
		resolved: make(map[int64]*resolvedChat),
	}
}

func (c *Collector) Run(ctx context.Context) error {
	client := telegram.NewClient(c.appID, c.appHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: c.session},
		UpdateHandler:  telegram.UpdateHandlerFunc(c.handleUpdates),
	})
	c.client = client

	return client.Run(ctx, func(ctx context.Context) error {
		c.runCtx = ctx
		if err := c.ensureAuthorized(ctx); err != nil {
			return fmt.Errorf("authorizing: %w", err)
		}
		c.api = client.API()

		if err := c.resolveChats(ctx); err != nil {
			return fmt.Errorf("resolving configured chats: %w", err)
		}

		c.mu.RLock()
		toBackfill := make([]*resolvedChat, 0, len(c.resolved))
		for _, rc := range c.resolved {
			if !rc.Paused {
				toBackfill = append(toBackfill, rc)
			}
		}
		c.mu.RUnlock()
		for _, rc := range toBackfill {
			if err := c.backfill(ctx, rc); err != nil {

				fmt.Fprintf(os.Stderr, "twork: backfill failed for %q: %v\n", rc.Title, err)
			}
		}

		<-ctx.Done()
		return ctx.Err()
	})
}

func (c *Collector) ensureAuthorized(ctx context.Context) error {
	status, err := c.client.Auth().Status(ctx)
	if err != nil {
		return err
	}
	if status.Authorized {
		return nil
	}

	fmt.Println("twork: no existing Telegram session found, starting interactive login.")
	fmt.Println("twork: a login code will be sent to", c.phone, "via Telegram.")

	codePrompt := func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
		fmt.Print("Enter the code you received: ")
		return readLine()
	}
	passwordPrompt := func(ctx context.Context) (string, error) {
		fmt.Print("Two-factor password (leave blank if not enabled): ")
		return readLine()
	}

	flow := auth.NewFlow(
		promptAuth{phone: c.phone, code: auth.CodeAuthenticatorFunc(codePrompt), password: passwordPrompt},
		auth.SendCodeOptions{},
	)
	return flow.Run(ctx, c.client.Auth())
}

func readLine() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

type promptAuth struct {
	phone    string
	code     auth.CodeAuthenticator
	password func(ctx context.Context) (string, error)
}

func (p promptAuth) Phone(ctx context.Context) (string, error) { return p.phone, nil }
func (p promptAuth) Password(ctx context.Context) (string, error) {
	return p.password(ctx)
}
func (p promptAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	return p.code.Code(ctx, sentCode)
}
func (p promptAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return nil
}
func (p promptAuth) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("account sign-up is not supported by twork; the phone number must already have a Telegram account")
}

func (c *Collector) resolveChats(ctx context.Context) error {
	stored, err := c.store.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("loading monitored chats from storage: %w", err)
	}
	if len(stored) > 0 {
		c.mu.Lock()
		for _, sc := range stored {
			c.resolved[sc.TelegramID] = &resolvedChat{Chat: sc, InputPeer: inputPeerFor(sc)}
		}
		c.mu.Unlock()
		return nil
	}
	if len(c.seed) == 0 {
		return nil
	}

	byUsername := make(map[string]*resolvedChat)
	byID := make(map[int64]*resolvedChat)

	var (
		offsetDate int
		offsetID   int
		offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}
	)
	for {
		resp, err := c.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			OffsetPeer: offsetPeer,
			Limit:      100,
		})
		if err != nil {
			return fmt.Errorf("fetching dialogs: %w", err)
		}

		dialogs, chats, msgs, done := extractDialogs(resp)
		for _, ch := range chats {
			rc := chatToResolved(ch)
			if rc == nil {
				continue
			}
			byID[rc.TelegramID] = rc
			if rc.Username != "" {
				byUsername[strings.ToLower(rc.Username)] = rc
			}
		}
		if done || len(dialogs) == 0 {
			break
		}

		last := msgs[len(msgs)-1]
		offsetDate = last.date
		offsetID = last.id
		offsetPeer = last.inputPeer
		if len(dialogs) < 100 {
			break
		}
	}

	for _, wanted := range c.seed {
		var rc *resolvedChat
		if wanted.Username != "" {
			rc = byUsername[strings.ToLower(strings.TrimPrefix(wanted.Username, "@"))]
		} else if wanted.ID != 0 {
			rc = byID[wanted.ID]
		}
		if rc == nil {
			fmt.Fprintf(os.Stderr, "twork: seed chat %q (id=%d) was not found in this account's dialog list -- join/open it in Telegram first, or add it later via the bot\n", wanted.Username, wanted.ID)
			continue
		}
		rc.Tag = wanted.Tag
		rc.Paused = wanted.Paused
		if err := c.store.UpsertChat(ctx, rc.Chat); err != nil {
			return fmt.Errorf("persisting seeded chat %q: %w", rc.Title, err)
		}
		c.mu.Lock()
		c.resolved[rc.TelegramID] = rc
		c.mu.Unlock()
	}
	return nil
}

func inputPeerFor(chat models.Chat) tg.InputPeerClass {
	if chat.AccessHash != 0 {
		return &tg.InputPeerChannel{ChannelID: chat.TelegramID, AccessHash: chat.AccessHash}
	}
	return &tg.InputPeerChat{ChatID: chat.TelegramID}
}

type dialogMsgRef struct {
	date      int
	id        int
	inputPeer tg.InputPeerClass
}

func extractDialogs(resp tg.MessagesDialogsClass) (dialogs []tg.DialogClass, chats []tg.ChatClass, msgs []dialogMsgRef, done bool) {
	var rawMsgs []tg.MessageClass
	switch d := resp.(type) {
	case *tg.MessagesDialogs:
		dialogs, chats, rawMsgs = d.Dialogs, d.Chats, d.Messages
		done = true
	case *tg.MessagesDialogsSlice:
		dialogs, chats, rawMsgs = d.Dialogs, d.Chats, d.Messages
	case *tg.MessagesDialogsNotModified:
		done = true
	}
	for _, mc := range rawMsgs {
		m, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		msgs = append(msgs, dialogMsgRef{date: m.Date, id: m.ID, inputPeer: peerToInput(m.PeerID)})
	}
	return
}

func chatToResolved(ch tg.ChatClass) *resolvedChat {
	switch v := ch.(type) {
	case *tg.Channel:
		if v.Left {
			return nil
		}
		kind := models.ChatKindGroup
		if v.Broadcast {
			kind = models.ChatKindChannel
		}
		return &resolvedChat{
			Chat: models.Chat{
				TelegramID: v.ID,
				AccessHash: v.AccessHash,
				Kind:       kind,
				Title:      v.Title,
				Username:   v.Username,
			},
			InputPeer: &tg.InputPeerChannel{ChannelID: v.ID, AccessHash: v.AccessHash},
		}
	case *tg.Chat:
		if v.Left || v.Deactivated {
			return nil
		}
		return &resolvedChat{
			Chat: models.Chat{
				TelegramID: v.ID,
				Kind:       models.ChatKindGroup,
				Title:      v.Title,
			},
			InputPeer: &tg.InputPeerChat{ChatID: v.ID},
		}
	default:
		return nil
	}
}

func peerToInput(p tg.PeerClass) tg.InputPeerClass {
	switch v := p.(type) {
	case *tg.PeerChannel:
		return &tg.InputPeerChannel{ChannelID: v.ChannelID}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: v.ChatID}
	case *tg.PeerUser:
		return &tg.InputPeerUser{UserID: v.UserID}
	default:
		return &tg.InputPeerEmpty{}
	}
}

func (c *Collector) backfill(ctx context.Context, rc *resolvedChat) error {
	minID, err := c.store.MaxTelegramMessageID(ctx, rc.TelegramID)
	if err != nil {
		return fmt.Errorf("looking up last seen message: %w", err)
	}

	offsetID := 0
	const pageSize = 100
	for {
		resp, err := c.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     rc.InputPeer,
			OffsetID: offsetID,
			Limit:    pageSize,
		})
		if err != nil {
			return fmt.Errorf("fetching history page: %w", err)
		}
		batch := extractMessages(resp)
		if len(batch) == 0 {
			return nil
		}

		reachedKnown := false
		for _, mc := range batch {
			m, ok := mc.(*tg.Message)
			if !ok {
				continue
			}
			if m.ID <= minID {
				reachedKnown = true
				break
			}
			if err := c.onMsg(ctx, normalizeMessage(m, rc), false); err != nil {
				return fmt.Errorf("handling backfilled message %d: %w", m.ID, err)
			}
		}
		if reachedKnown || len(batch) < pageSize {
			return nil
		}

		last, ok := batch[len(batch)-1].(*tg.Message)
		if !ok {
			return nil
		}
		offsetID = last.ID
	}
}

func extractMessages(resp tg.MessagesMessagesClass) []tg.MessageClass {
	switch m := resp.(type) {
	case *tg.MessagesMessages:
		return m.Messages
	case *tg.MessagesMessagesSlice:
		return m.Messages
	case *tg.MessagesChannelMessages:
		return m.Messages
	default:
		return nil
	}
}

func (c *Collector) handleUpdates(ctx context.Context, u tg.UpdatesClass) error {
	var updates []tg.UpdateClass
	switch v := u.(type) {
	case *tg.Updates:
		updates = v.Updates
	case *tg.UpdatesCombined:
		updates = v.Updates
	case *tg.UpdateShort:
		updates = []tg.UpdateClass{v.Update}
	default:
		return nil
	}

	for _, one := range updates {
		var msgClass tg.MessageClass
		switch v := one.(type) {
		case *tg.UpdateNewMessage:
			msgClass = v.Message
		case *tg.UpdateNewChannelMessage:
			msgClass = v.Message
		default:
			continue
		}
		m, ok := msgClass.(*tg.Message)
		if !ok {
			continue
		}
		chatID := peerChatID(m.PeerID)
		c.mu.RLock()
		rc, monitored := c.resolved[chatID]
		c.mu.RUnlock()
		if !monitored || rc.Paused {
			continue
		}
		if err := c.onMsg(ctx, normalizeMessage(m, rc), true); err != nil {
			return fmt.Errorf("handling live message %d: %w", m.ID, err)
		}
	}
	return nil
}

func peerChatID(p tg.PeerClass) int64 {
	switch v := p.(type) {
	case *tg.PeerChannel:
		return v.ChannelID
	case *tg.PeerChat:
		return v.ChatID
	case *tg.PeerUser:
		return v.UserID
	default:
		return 0
	}
}

func normalizeMessage(m *tg.Message, rc *resolvedChat) models.Message {
	msg := models.Message{
		TelegramMessageID: m.ID,
		ChatID:            rc.TelegramID,
		ChatTitle:         rc.Title,
		Timestamp:         time.Unix(int64(m.Date), 0).UTC(),
		Text:              m.Message,
		Link:              messageLink(rc, m.ID),
	}
	if senderID, ok := m.GetFromID(); ok {
		msg.SenderID = peerChatID(senderID)
	}
	if fwd, ok := m.GetFwdFrom(); ok {
		if name, ok := fwd.GetFromName(); ok && name != "" {
			msg.ForwardFromTitle = name
		} else if author, ok := fwd.GetPostAuthor(); ok {
			msg.ForwardFromTitle = author
		}
	}
	if editDate, ok := m.GetEditDate(); ok && editDate != 0 {
		t := time.Unix(int64(editDate), 0).UTC()
		msg.EditTimestamp = &t
	}
	return msg
}

func messageLink(rc *resolvedChat, msgID int) string {
	if rc.Username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", rc.Username, msgID)
	}
	return fmt.Sprintf("https://t.me/c/%d/%d", rc.TelegramID, msgID)
}

func ParseChatIdentifier(s string) (username string, id int64) {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return "", n
	}
	return strings.TrimPrefix(s, "@"), 0
}
