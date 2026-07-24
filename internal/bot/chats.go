package bot

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/models"
)

// routes chat: callbacks
func (b *Bot) handleChatCallback(ctx context.Context, data string) {
	switch {
	case data == "chat:add":
		b.showAddChatOptions()
	case data == "chat:add_username":
		b.promptFor(ctx, inputAddUsername, "Send the channel/group @username (or a t.me/username link):")
	case data == "chat:add_invite":
		b.promptFor(ctx, inputAddInvite, "Send the private invite link (t.me/+... or t.me/joinchat/...):\n\nNote: this will join the account to the chat.")
	case data == "chat:add_folder":
		b.promptFor(ctx, inputAddFolder, "Send the shared folder link (t.me/addlist/...):\n\nNote: this will join every chat in the folder.")
	case strings.HasPrefix(data, "chat:page:"):
		n, _ := strconv.Atoi(strings.TrimPrefix(data, "chat:page:"))
		b.showChatsList(ctx, n)
	case strings.HasPrefix(data, "chat:view:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:view:"), 10, 64)
		b.showChatDetail(ctx, id)
	case strings.HasPrefix(data, "chat:pause:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:pause:"), 10, 64)
		_ = b.coll.Pause(ctx, id)
		b.showChatDetail(ctx, id)
	case strings.HasPrefix(data, "chat:resume:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:resume:"), 10, 64)
		_ = b.coll.Resume(ctx, id)
		b.showChatDetail(ctx, id)
	case strings.HasPrefix(data, "chat:remove_ask:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:remove_ask:"), 10, 64)
		b.showRemoveConfirm(ctx, id)
	case strings.HasPrefix(data, "chat:remove:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:remove:"), 10, 64)
		_ = b.coll.Remove(ctx, id)
		b.showChatsList(ctx, 0)
	case strings.HasPrefix(data, "chat:tag:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "chat:tag:"), 10, 64)
		b.sess.editingTagFor = id
		b.promptFor(ctx, inputEditTag, "Send a new tag/category for this chat (e.g. Backend, DevOps):")
	}
}

// renders the paginated chats list
func (b *Bot) showChatsList(ctx context.Context, page int) {
	chats := b.coll.ListResolved()
	sort.Slice(chats, func(i, j int) bool { return strings.ToLower(chats[i].Title) < strings.ToLower(chats[j].Title) })

	start, end := paginate(len(chats), page, pageSize)
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range chats[start:end] {
		label := c.Title
		if c.Paused {
			label = "⏸ " + label
		} else {
			label = "🟢 " + label
		}
		if c.Tag != "" {
			label += " (" + c.Tag + ")"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("chat:view:%d", c.TelegramID)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("➕ Add channel", "chat:add")))
	rows = append(rows, navRow("chat:page:", page, len(chats), pageSize))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(backButton("menu:home")))

	text := fmt.Sprintf("📡 Monitored chats (%d)", len(chats))
	if len(chats) == 0 {
		text += "\n\nNothing yet -- tap Add channel to get started."
	}
	b.editHome(ctx, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// shows the username/invite/folder add choices
func (b *Bot) showAddChatOptions() {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("👤 By username", "chat:add_username")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🔗 By invite link", "chat:add_invite")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("📁 By folder link", "chat:add_folder")),
		tgbotapi.NewInlineKeyboardRow(backButton("chat:page:0")),
	)
	b.editHome(context.Background(), "How do you want to add a channel or group?", kb)
}

// renders one chat's status and controls
func (b *Bot) showChatDetail(ctx context.Context, telegramID int64) {
	c, err := b.store.GetChatByTelegramID(ctx, telegramID)
	if err != nil || c == nil {
		b.showChatsList(ctx, 0)
		return
	}

	status := "🟢 monitoring"
	toggleLabel, toggleData := "⏸ Pause", fmt.Sprintf("chat:pause:%d", c.TelegramID)
	if c.Paused {
		status = "⏸ paused"
		toggleLabel, toggleData = "▶️ Resume", fmt.Sprintf("chat:resume:%d", c.TelegramID)
	}

	text := fmt.Sprintf("📡 %s\n\nStatus: %s", c.Title, status)
	if c.Username != "" {
		text += fmt.Sprintf("\nUsername: @%s", c.Username)
	}
	if c.Tag != "" {
		text += fmt.Sprintf("\nTag: %s", c.Tag)
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleLabel, toggleData),
			tgbotapi.NewInlineKeyboardButtonData("🏷 Edit tag", fmt.Sprintf("chat:tag:%d", c.TelegramID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Remove", fmt.Sprintf("chat:remove_ask:%d", c.TelegramID)),
		),
		tgbotapi.NewInlineKeyboardRow(backButton("chat:page:0")),
	)
	b.editHome(ctx, text, kb)
}

// asks for confirmation before removing a chat
func (b *Bot) showRemoveConfirm(ctx context.Context, telegramID int64) {
	c, err := b.store.GetChatByTelegramID(ctx, telegramID)
	if err != nil || c == nil {
		b.showChatsList(ctx, 0)
		return
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Yes, remove", fmt.Sprintf("chat:remove:%d", c.TelegramID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel", fmt.Sprintf("chat:view:%d", c.TelegramID)),
		),
	)
	b.editHome(ctx, fmt.Sprintf("Stop monitoring %q?\n\nAlready-indexed messages from it are kept -- this only stops watching for new ones.", c.Title), kb)
}

// shows the added chat, or an error
func (b *Bot) handleAddChatResult(ctx context.Context, chat *models.Chat, err error) {
	if err != nil {
		b.editHome(ctx, "Couldn't add that chat:\n"+err.Error(), tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(backButton("chat:page:0")),
		))
		return
	}
	b.showChatDetail(ctx, chat.TelegramID)
}

// clamps page and returns its slice bounds
func paginate(total, page, size int) (start, end int) {
	if size <= 0 {
		size = 1
	}
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / size
	}
	if page < 0 {
		page = 0
	}
	if page > maxPage {
		page = maxPage
	}
	start = page * size
	if start > total {
		start = total
	}
	end = start + size
	if end > total {
		end = total
	}
	return start, end
}

// builds a prev/position/next button row
func navRow(callbackPrefix string, page, total, size int) []tgbotapi.InlineKeyboardButton {
	if size <= 0 {
		size = 1
	}
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / size
	}
	label := fmt.Sprintf("%d/%d", page+1, maxPage+1)
	if total == 0 {
		label = "0/0"
	}
	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️", fmt.Sprintf("%s%d", callbackPrefix, page-1)),
		tgbotapi.NewInlineKeyboardButtonData(label, "noop"),
		tgbotapi.NewInlineKeyboardButtonData("➡️", fmt.Sprintf("%s%d", callbackPrefix, page+1)),
	)
}
