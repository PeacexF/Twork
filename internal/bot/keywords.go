package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/matcher"
	"github.com/PeacexF/Twork/internal/storage"
)

func (b *Bot) handleKeywordsCallback(ctx context.Context, data string) {
	switch {
	case data == "kw:add_pos":
		b.promptFor(ctx, inputAddPositiveKw, "Send keyword(s) to add to the POSITIVE list (comma or newline separated):")
	case data == "kw:add_neg":
		b.promptFor(ctx, inputAddNegativeKw, "Send keyword(s) to add to the NEGATIVE list (comma or newline separated):")
	case data == "kw:rm_pos_menu":
		b.showRemoveKeywordMenu(ctx, true)
	case data == "kw:rm_neg_menu":
		b.showRemoveKeywordMenu(ctx, false)
	case strings.HasPrefix(data, "kw:rmpos:"):
		i, _ := strconv.Atoi(strings.TrimPrefix(data, "kw:rmpos:"))
		b.removeKeyword(ctx, true, i)
	case strings.HasPrefix(data, "kw:rmneg:"):
		i, _ := strconv.Atoi(strings.TrimPrefix(data, "kw:rmneg:"))
		b.removeKeyword(ctx, false, i)
	case data == "kw:mode_toggle":
		b.toggleMode(ctx)
	}
}

func (b *Bot) loadKeywords(ctx context.Context) storage.Keywords {
	kw, ok, err := b.store.GetKeywords(ctx)
	if err != nil {
		log.Printf("twork: loading keywords failed: %v", err)
	}
	if !ok {
		kw.Mode = config.MatchModeWholeWord
	}
	return kw
}

func (b *Bot) showKeywordsMenu(ctx context.Context) {
	kw := b.loadKeywords(ctx)

	text := fmt.Sprintf("🔑 Keywords\n\nMode: %s\n\nPositive (%d):\n%s\n\nNegative (%d):\n%s",
		kw.Mode, len(kw.Positive), bulletList(kw.Positive), len(kw.Negative), bulletList(kw.Negative))

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Add positive", "kw:add_pos"),
			tgbotapi.NewInlineKeyboardButtonData("➕ Add negative", "kw:add_neg"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➖ Remove positive", "kw:rm_pos_menu"),
			tgbotapi.NewInlineKeyboardButtonData("➖ Remove negative", "kw:rm_neg_menu"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(modeToggleLabel(kw.Mode), "kw:mode_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(backButton("menu:home")),
	)
	b.editHome(ctx, text, kb)
}

func bulletList(words []string) string {
	if len(words) == 0 {
		return "(none)"
	}
	var sb strings.Builder
	for _, w := range words {
		sb.WriteString("• " + w + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func modeToggleLabel(mode string) string {
	if mode == config.MatchModeWholeWord {
		return "🔁 Mode: whole word (tap for substring)"
	}
	return "🔁 Mode: substring (tap for whole word)"
}

func (b *Bot) showRemoveKeywordMenu(ctx context.Context, positive bool) {
	kw := b.loadKeywords(ctx)
	words := kw.Negative
	prefix := "kw:rmneg:"
	label := "Negative"
	if positive {
		words = kw.Positive
		prefix = "kw:rmpos:"
		label = "Positive"
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, w := range words {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ "+w, fmt.Sprintf("%s%d", prefix, i)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(backButton("menu:keywords")))

	text := fmt.Sprintf("Tap a %s keyword to remove it.", label)
	if len(words) == 0 {
		text = fmt.Sprintf("No %s keywords to remove.", label)
	}
	b.editHome(ctx, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func (b *Bot) removeKeyword(ctx context.Context, positive bool, index int) {
	kw := b.loadKeywords(ctx)
	list := &kw.Negative
	if positive {
		list = &kw.Positive
	}
	if index >= 0 && index < len(*list) {
		*list = append((*list)[:index], (*list)[index+1:]...)
	}
	b.saveKeywords(ctx, kw)
	b.showRemoveKeywordMenu(ctx, positive)
}

func (b *Bot) toggleMode(ctx context.Context) {
	kw := b.loadKeywords(ctx)
	if kw.Mode == config.MatchModeWholeWord {
		kw.Mode = config.MatchModeSubstring
	} else {
		kw.Mode = config.MatchModeWholeWord
	}
	b.saveKeywords(ctx, kw)
	b.showKeywordsMenu(ctx)
}

func (b *Bot) saveKeywords(ctx context.Context, kw storage.Keywords) {
	if err := b.store.SetKeywords(ctx, kw); err != nil {
		log.Printf("twork: saving keywords failed: %v", err)
		return
	}
	m, err := matcher.New(config.MatchingConfig{Positive: kw.Positive, Negative: kw.Negative, Mode: kw.Mode})
	if err != nil {
		log.Printf("twork: rebuilding matcher failed: %v", err)
		return
	}
	b.matchStore.Set(m)
}

func parseKeywordInput(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '\n' })
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}
