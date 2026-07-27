package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type MatchRow struct {
	MessageID       int64
	ChatTitle       string
	Text            string
	Link            string
	Timestamp       time.Time
	MatchedKeywords []string
	Bookmarked      bool
}

// pages keyword-matched messages, newest first
func (s *Store) ListMatches(ctx context.Context, limit, offset int) ([]MatchRow, int, error) {
	return s.pagedMatchQuery(ctx, `
		SELECT m.id, m.chat_title, m.text, m.link, m.ts, ma.matched_keywords,
		       COALESCE(st.bookmarked, 0)
		FROM matches ma
		JOIN messages m ON m.id = ma.message_id
		LEFT JOIN message_status st ON st.message_id = m.id
		ORDER BY m.ts DESC
		LIMIT ? OFFSET ?
	`, `SELECT COUNT(1) FROM matches`, limit, offset)
}

// pages saved/favorited messages, newest first
func (s *Store) ListBookmarked(ctx context.Context, limit, offset int) ([]MatchRow, int, error) {
	return s.pagedMatchQuery(ctx, `
		SELECT m.id, m.chat_title, m.text, m.link, m.ts,
		       COALESCE(ma.matched_keywords, '[]'), st.bookmarked
		FROM message_status st
		JOIN messages m ON m.id = st.message_id
		LEFT JOIN matches ma ON ma.message_id = m.id
		WHERE st.bookmarked = 1
		ORDER BY m.ts DESC
		LIMIT ? OFFSET ?
	`, `SELECT COUNT(1) FROM message_status WHERE bookmarked = 1`, limit, offset)
}

// pages FTS5 search results
func (s *Store) SearchPaged(ctx context.Context, query string, limit, offset int) ([]MatchRow, int, error) {
	return s.pagedMatchQuery(ctx, `
		SELECT m.id, m.chat_title, m.text, m.link, m.ts,
		       COALESCE(ma.matched_keywords, '[]'), COALESCE(st.bookmarked, 0)
		FROM messages_fts f
		JOIN messages m ON m.id = f.rowid
		LEFT JOIN matches ma ON ma.message_id = m.id
		LEFT JOIN message_status st ON st.message_id = m.id
		WHERE messages_fts MATCH ?
		ORDER BY m.ts DESC
		LIMIT ? OFFSET ?
	`, `SELECT COUNT(1) FROM messages_fts WHERE messages_fts MATCH ?`, limit, offset, query)
}

// runs a page query plus its matching count query
func (s *Store) pagedMatchQuery(ctx context.Context, query, countQuery string, limit, offset int, extraArgs ...string) ([]MatchRow, int, error) {
	if limit <= 0 {
		limit = 10
	}
	args := make([]any, 0, len(extraArgs)+2)
	for _, a := range extraArgs {
		args = append(args, a)
	}
	mainArgs := append(append([]any{}, args...), limit, offset)

	rows, err := s.db.QueryContext(ctx, query, mainArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying: %w", err)
	}
	defer rows.Close()

	var out []MatchRow
	for rows.Next() {
		var r MatchRow
		var keywordsJSON string
		var bookmarkedInt int
		if err := rows.Scan(&r.MessageID, &r.ChatTitle, &r.Text, &r.Link, &r.Timestamp, &keywordsJSON, &bookmarkedInt); err != nil {
			return nil, 0, err
		}
		if keywordsJSON != "" {
			_ = json.Unmarshal([]byte(keywordsJSON), &r.MatchedKeywords)
		}
		r.Bookmarked = bookmarkedInt != 0
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting: %w", err)
	}
	return out, total, nil
}

// fetches one message with its match/bookmark status
func (s *Store) GetMatchRow(ctx context.Context, messageID int64) (*MatchRow, error) {
	var r MatchRow
	var keywordsJSON string
	var bookmarkedInt int
	err := s.db.QueryRowContext(ctx, `
		SELECT m.id, m.chat_title, m.text, m.link, m.ts,
		       COALESCE(ma.matched_keywords, '[]'), COALESCE(st.bookmarked, 0)
		FROM messages m
		LEFT JOIN matches ma ON ma.message_id = m.id
		LEFT JOIN message_status st ON st.message_id = m.id
		WHERE m.id = ?
	`, messageID).Scan(&r.MessageID, &r.ChatTitle, &r.Text, &r.Link, &r.Timestamp, &keywordsJSON, &bookmarkedInt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(keywordsJSON), &r.MatchedKeywords)
	r.Bookmarked = bookmarkedInt != 0
	return &r, nil
}

// flips a message's saved flag and returns the new state
func (s *Store) ToggleBookmark(ctx context.Context, messageID int64) (bool, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO message_status (message_id, bookmarked) VALUES (?, 1)
		ON CONFLICT(message_id) DO UPDATE SET bookmarked = 1 - bookmarked
	`, messageID)
	if err != nil {
		return false, err
	}
	var bookmarkedInt int
	if err := s.db.QueryRowContext(ctx, `SELECT bookmarked FROM message_status WHERE message_id = ?`, messageID).Scan(&bookmarkedInt); err != nil {
		return false, err
	}
	return bookmarkedInt != 0, nil
}

// returns matches recorded since the given time, newest first, for the digest
func (s *Store) MatchesSince(ctx context.Context, since time.Time, limit int) ([]MatchRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.chat_title, m.text, m.link, m.ts,
		       ma.matched_keywords, COALESCE(st.bookmarked, 0)
		FROM matches ma
		JOIN messages m ON m.id = ma.message_id
		LEFT JOIN message_status st ON st.message_id = m.id
		WHERE ma.created_at >= ?
		ORDER BY m.ts DESC
		LIMIT ?
	`, since.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("querying digest matches: %w", err)
	}
	defer rows.Close()

	var out []MatchRow
	for rows.Next() {
		var r MatchRow
		var keywordsJSON string
		var bookmarkedInt int
		if err := rows.Scan(&r.MessageID, &r.ChatTitle, &r.Text, &r.Link, &r.Timestamp, &keywordsJSON, &bookmarkedInt); err != nil {
			return nil, err
		}
		if keywordsJSON != "" {
			_ = json.Unmarshal([]byte(keywordsJSON), &r.MatchedKeywords)
		}
		r.Bookmarked = bookmarkedInt != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// counts messages indexed since the given time, for the digest header
func (s *Store) CountMessagesSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM messages WHERE created_at >= ?`, since.UTC()).Scan(&n)
	return n, err
}

// counts matches recorded since the given time
func (s *Store) CountMatchesSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM matches WHERE created_at >= ?`, since.UTC()).Scan(&n)
	return n, err
}
