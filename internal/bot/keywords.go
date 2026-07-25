package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/config"
	"github.com/PeacexF/Twork/internal/storage"
)

// routes kw: callbacks
func (b *Bot) handleKeywordsCallback(ctx context.Context, data string) {
	switch {
	case data == "kw:menu":
		b.showKeywordsMenu(ctx)
	case data == "kw:mode_toggle":
		b.toggleGlobalMode(ctx)
	case data == "kw:new_pos":
		b.sess.editingGroupPositive = true
		b.promptFor(ctx, inputNewGroup, "Send a name for the new POSITIVE group (e.g. Go):")
	case data == "kw:new_neg":
		b.sess.editingGroupPositive = false
		b.promptFor(ctx, inputNewGroup, "Send a name for the new NEGATIVE group (e.g. Seniority):")
	case strings.HasPrefix(data, "kw:group:"):
		b.openGroupByToken(ctx, strings.TrimPrefix(data, "kw:group:"))
	case strings.HasPrefix(data, "kw:addalias:"):
		b.sess.editingGroupToken = strings.TrimPrefix(data, "kw:addalias:")
		b.promptFor(ctx, inputAddAlias, "Send alias(es) to add (comma or newline separated):")
	case strings.HasPrefix(data, "kw:rmalias:"):
		b.removeAlias(ctx, strings.TrimPrefix(data, "kw:rmalias:"))
	case strings.HasPrefix(data, "kw:togglemode:"):
		b.toggleGroupMode(ctx, strings.TrimPrefix(data, "kw:togglemode:"))
	case strings.HasPrefix(data, "kw:delete:"):
		b.deleteGroup(ctx, strings.TrimPrefix(data, "kw:delete:"))
	}
}

// loads keyword groups, defaulting the mode if unset
func (b *Bot) loadKeywords(ctx context.Context) storage.Keywords {
	kw, ok, err := b.store.GetKeywords(ctx)
	if err != nil {
		log.Printf("twork: loading keywords failed: %v", err)
	}
	if !ok || kw.Mode == "" {
		kw.Mode = config.MatchModeWholeWord
	}
	return kw
}

// renders the list of positive and negative groups
func (b *Bot) showKeywordsMenu(ctx context.Context) {
	kw := b.loadKeywords(ctx)

	text := fmt.Sprintf("🔑 Keyword groups\n\nA group matches if ANY of its aliases is found.\nDefault mode: %s\n\nPositive groups: %d\nNegative groups: %d",
		kw.Mode, len(kw.PositiveGroups), len(kw.NegativeGroups))

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, g := range kw.PositiveGroups {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ "+groupLabel(g), "kw:group:"+groupToken(true, i)),
		))
	}
	for i, g := range kw.NegativeGroups {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚫 "+groupLabel(g), "kw:group:"+groupToken(false, i)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ New positive", "kw:new_pos"),
		tgbotapi.NewInlineKeyboardButtonData("➕ New negative", "kw:new_neg"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(modeToggleLabel(kw.Mode), "kw:mode_toggle"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(backButton("menu:home")))
	b.editHome(ctx, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// summarizes a group as "Name (alias1, alias2)"
func groupLabel(g storage.KeywordGroup) string {
	if len(g.Aliases) == 0 {
		return g.Name
	}
	return fmt.Sprintf("%s (%s)", g.Name, strings.Join(g.Aliases, ", "))
}

// encodes a group's list and index into a callback token like "p3" or "n1"
func groupToken(positive bool, index int) string {
	if positive {
		return "p" + strconv.Itoa(index)
	}
	return "n" + strconv.Itoa(index)
}

// decodes a group token back into its list and index
func parseGroupToken(token string) (positive bool, index int, ok bool) {
	if len(token) < 2 {
		return false, 0, false
	}
	i, err := strconv.Atoi(token[1:])
	if err != nil {
		return false, 0, false
	}
	return token[0] == 'p', i, true
}

// looks up a group by its token; returns a copy and whether it was found
func (b *Bot) groupByToken(ctx context.Context, token string) (kw storage.Keywords, positive bool, index int, g storage.KeywordGroup, ok bool) {
	kw = b.loadKeywords(ctx)
	positive, index, valid := parseGroupToken(token)
	if !valid {
		return kw, false, 0, g, false
	}
	list := kw.NegativeGroups
	if positive {
		list = kw.PositiveGroups
	}
	if index < 0 || index >= len(list) {
		return kw, positive, index, g, false
	}
	return kw, positive, index, list[index], true
}

// renders one group's aliases and controls
func (b *Bot) openGroupByToken(ctx context.Context, token string) {
	_, _, _, g, ok := b.groupByToken(ctx, token)
	if !ok {
		b.showKeywordsMenu(ctx)
		return
	}

	modeLabel := "inherits default"
	if g.Mode != "" {
		modeLabel = g.Mode
	}
	text := fmt.Sprintf("Group: %s\n\nMatching mode: %s\n\nAliases (%d) — matches if any is found:\n%s",
		g.Name, modeLabel, len(g.Aliases), bulletList(g.Aliases))

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, a := range g.Aliases {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ "+a, "kw:rmalias:"+token+":"+strconv.Itoa(i)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Add alias", "kw:addalias:"+token),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔁 "+groupModeLabel(g.Mode), "kw:togglemode:"+token),
		tgbotapi.NewInlineKeyboardButtonData("🗑 Delete group", "kw:delete:"+token),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(backButton("kw:menu")))
	b.editHome(ctx, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// labels the per-group mode cycle button
func groupModeLabel(mode string) string {
	switch mode {
	case config.MatchModeWholeWord:
		return "Mode: whole word"
	case config.MatchModeSubstring:
		return "Mode: substring"
	default:
		return "Mode: default"
	}
}

// formats strings as a bullet list
func bulletList(words []string) string {
	if len(words) == 0 {
		return "(none)"
	}
	var sb strings.Builder
	for _, w := range words {
		sb.WriteString("• ")
		sb.WriteString(w)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// labels the global default-mode toggle
func modeToggleLabel(mode string) string {
	if mode == config.MatchModeWholeWord {
		return "🔁 Default mode: whole word"
	}
	return "🔁 Default mode: substring"
}

// creates a new group with the given name in the pending polarity
func (b *Bot) createGroup(ctx context.Context, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		b.showKeywordsMenu(ctx)
		return
	}
	kw := b.loadKeywords(ctx)
	g := storage.KeywordGroup{Name: name, Aliases: []string{name}}
	if b.sess.editingGroupPositive {
		kw.PositiveGroups = append(kw.PositiveGroups, g)
	} else {
		kw.NegativeGroups = append(kw.NegativeGroups, g)
	}
	b.saveKeywords(ctx, kw)
	b.showKeywordsMenu(ctx)
}

// adds parsed aliases to the group identified by the pending token
func (b *Bot) addAliasesToPending(ctx context.Context, text string) {
	token := b.sess.editingGroupToken
	toAdd := parseKeywordInput(text)
	kw, positive, index, g, ok := b.groupByToken(ctx, token)
	if !ok || len(toAdd) == 0 {
		b.openGroupByToken(ctx, token)
		return
	}
	g.Aliases = append(g.Aliases, toAdd...)
	writeGroup(&kw, positive, index, g)
	b.saveKeywords(ctx, kw)
	b.openGroupByToken(ctx, token)
}

// removes one alias, addressed by a "token:index" callback payload
func (b *Bot) removeAlias(ctx context.Context, payload string) {
	token, idxStr, found := strings.Cut(payload, ":")
	if !found {
		return
	}
	aliasIdx, err := strconv.Atoi(idxStr)
	if err != nil {
		return
	}
	kw, positive, index, g, ok := b.groupByToken(ctx, token)
	if !ok {
		return
	}
	if aliasIdx >= 0 && aliasIdx < len(g.Aliases) {
		g.Aliases = append(g.Aliases[:aliasIdx], g.Aliases[aliasIdx+1:]...)
	}
	writeGroup(&kw, positive, index, g)
	b.saveKeywords(ctx, kw)
	b.openGroupByToken(ctx, token)
}

// cycles a group's mode: default -> whole_word -> substring -> default
func (b *Bot) toggleGroupMode(ctx context.Context, token string) {
	kw, positive, index, g, ok := b.groupByToken(ctx, token)
	if !ok {
		return
	}
	switch g.Mode {
	case "":
		g.Mode = config.MatchModeWholeWord
	case config.MatchModeWholeWord:
		g.Mode = config.MatchModeSubstring
	default:
		g.Mode = ""
	}
	writeGroup(&kw, positive, index, g)
	b.saveKeywords(ctx, kw)
	b.openGroupByToken(ctx, token)
}

// deletes the group identified by the token
func (b *Bot) deleteGroup(ctx context.Context, token string) {
	kw, positive, index, _, ok := b.groupByToken(ctx, token)
	if !ok {
		b.showKeywordsMenu(ctx)
		return
	}
	if positive {
		kw.PositiveGroups = append(kw.PositiveGroups[:index], kw.PositiveGroups[index+1:]...)
	} else {
		kw.NegativeGroups = append(kw.NegativeGroups[:index], kw.NegativeGroups[index+1:]...)
	}
	b.saveKeywords(ctx, kw)
	b.showKeywordsMenu(ctx)
}

// writes a modified group back into the right list at its index
func writeGroup(kw *storage.Keywords, positive bool, index int, g storage.KeywordGroup) {
	if positive {
		if index >= 0 && index < len(kw.PositiveGroups) {
			kw.PositiveGroups[index] = g
		}
		return
	}
	if index >= 0 && index < len(kw.NegativeGroups) {
		kw.NegativeGroups[index] = g
	}
}

// flips the global default matching mode
func (b *Bot) toggleGlobalMode(ctx context.Context) {
	kw := b.loadKeywords(ctx)
	if kw.Mode == config.MatchModeWholeWord {
		kw.Mode = config.MatchModeSubstring
	} else {
		kw.Mode = config.MatchModeWholeWord
	}
	b.saveKeywords(ctx, kw)
	b.showKeywordsMenu(ctx)
}

// persists keyword groups and hot-swaps the live matcher
func (b *Bot) saveKeywords(ctx context.Context, kw storage.Keywords) {
	if err := b.store.SetKeywords(ctx, kw); err != nil {
		log.Printf("twork: saving keywords failed: %v", err)
		return
	}
	b.matchStore.Set(matcherFromStoredKeywords(kw))
}

// splits comma/newline separated input into trimmed non-empty tokens
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
