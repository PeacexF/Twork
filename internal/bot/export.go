package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PeacexF/Twork/internal/storage"
)

const exportPageSize = 200

// exports the active view to a .md file and sends it
func (b *Bot) exportCurrentView(ctx context.Context) {
	rows, title, err := b.collectViewRows(ctx)
	if err != nil {
		log.Printf("twork: export failed: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	md := renderMarkdown(title, rows)
	doc := tgbotapi.NewDocument(b.sess.homeChatID, tgbotapi.FileBytes{
		Name:  exportFilename(title),
		Bytes: []byte(md),
	})
	doc.Caption = fmt.Sprintf("%d post(s) exported.", len(rows))
	if _, err := b.api.Send(doc); err != nil {
		log.Printf("twork: sending export document failed: %v", err)
	}
}

// fetches every row of the active view, paged
func (b *Bot) collectViewRows(ctx context.Context) ([]storage.MatchRow, string, error) {
	var (
		all   []storage.MatchRow
		title string
	)
	offset := 0
	for {
		var (
			page  []storage.MatchRow
			total int
			err   error
		)
		switch b.sess.view {
		case viewMatches:
			title = "Twork Matches"
			page, total, err = b.store.ListMatches(ctx, exportPageSize, offset)
		case viewFavorites:
			title = "Twork Favorites"
			page, total, err = b.store.ListBookmarked(ctx, exportPageSize, offset)
		case viewSearch:
			title = fmt.Sprintf("Twork Search: %s", b.sess.searchQuery)
			page, total, err = b.store.SearchPaged(ctx, b.sess.searchQuery, exportPageSize, offset)
		default:
			return nil, "", fmt.Errorf("no active list view")
		}
		if err != nil {
			return nil, "", err
		}
		all = append(all, page...)
		offset += len(page)
		if len(page) == 0 || offset >= total {
			break
		}
	}
	return all, title, nil
}

// renders rows as a Markdown document
func renderMarkdown(title string, rows []storage.MatchRow) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", title)
	fmt.Fprintf(&sb, "_%d post(s), newest first._\n\n---\n\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(&sb, "## %s\n\n", r.ChatTitle)
		fmt.Fprintf(&sb, "%s\n\n", r.Timestamp.Format("2006-01-02 15:04 MST"))
		if len(r.MatchedKeywords) > 0 {
			fmt.Fprintf(&sb, "**Matched:** %s\n\n", strings.Join(r.MatchedKeywords, ", "))
		}
		fmt.Fprintf(&sb, "%s\n\n", r.Text)
		fmt.Fprintf(&sb, "[Open original](%s)\n\n---\n\n", r.Link)
	}
	return sb.String()
}

// sanitizes a title into a safe filename
func exportFilename(title string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, title)
	return safe + ".md"
}
