package bot

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) send(chatID int64, text string) int {
	msg, err := b.api.Send(tgbotapi.NewMessage(chatID, text))
	if err != nil {
		log.Printf("twork: send failed: %v", err)
		return 0
	}
	return msg.MessageID
}

func (b *Bot) deleteMessage(chatID int64, messageID int) {
	if messageID == 0 {
		return
	}
	if _, err := b.api.Request(tgbotapi.NewDeleteMessage(chatID, messageID)); err != nil {

		log.Printf("twork: delete message %d failed: %v", messageID, err)
	}
}

func (b *Bot) answerCallback(id, text string) {
	if _, err := b.api.Request(tgbotapi.NewCallback(id, text)); err != nil {
		log.Printf("twork: answering callback failed: %v", err)
	}
}

func (b *Bot) openHome(ctx context.Context, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, homeDashboardText(ctx, b.store))
	msg.ReplyMarkup = homeDashboardKeyboard()
	sent, err := b.api.Send(msg)
	if err != nil {
		log.Printf("twork: opening home message failed: %v", err)
		return
	}
	b.sess.homeChatID = chatID
	b.sess.homeMsgID = sent.MessageID
	b.sess.view = viewNone
	b.sess.page = 0
}

func (b *Bot) editHome(ctx context.Context, text string, kb tgbotapi.InlineKeyboardMarkup) {
	if b.sess.homeMsgID == 0 {

		b.openHome(ctx, b.sess.homeChatID)
		return
	}
	edit := tgbotapi.NewEditMessageTextAndMarkup(b.sess.homeChatID, b.sess.homeMsgID, text, kb)
	if _, err := b.api.Request(edit); err != nil {
		log.Printf("twork: editing home message failed: %v", err)
	}
}

func (b *Bot) promptFor(ctx context.Context, kind pendingInput, question string) {
	b.sess.pending = kind
	b.sess.promptMsgID = b.send(b.sess.homeChatID, question)
}

func (b *Bot) clearPrompt(userMsg *tgbotapi.Message) {
	b.deleteMessage(userMsg.Chat.ID, userMsg.MessageID)
	b.deleteMessage(userMsg.Chat.ID, b.sess.promptMsgID)
	b.sess.pending = inputNone
	b.sess.promptMsgID = 0
}

func backButton(target string) tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardButtonData("⬅️ Back", target)
}
