package storage

import (
	"context"
	"database/sql"

	"github.com/PeacexF/Twork/internal/models"
)

// lists monitored chats, newest first
func (s *Store) ListChats(ctx context.Context) ([]models.Chat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, telegram_id, access_hash, kind, title, username, tag, paused,
		       resume_enabled, resume_interval_seconds, resume_text, created_at
		FROM chats
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Chat
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// looks up one monitored chat, or nil if not monitored
func (s *Store) GetChatByTelegramID(ctx context.Context, telegramID int64) (*models.Chat, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, telegram_id, access_hash, kind, title, username, tag, paused,
		       resume_enabled, resume_interval_seconds, resume_text, created_at
		FROM chats WHERE telegram_id = ?
	`, telegramID)
	c, err := scanChat(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// row is satisfied by both *sql.Row and *sql.Rows
type row interface {
	Scan(dest ...any) error
}

// scans one chats row, including its resume broadcasting config
func scanChat(r row) (models.Chat, error) {
	var c models.Chat
	var kind string
	var pausedInt, resumeEnabledInt int
	err := r.Scan(&c.ID, &c.TelegramID, &c.AccessHash, &kind, &c.Title, &c.Username, &c.Tag, &pausedInt,
		&resumeEnabledInt, &c.ResumeIntervalSeconds, &c.ResumeText, &c.CreatedAt)
	if err != nil {
		return models.Chat{}, err
	}
	c.Kind = models.ChatKind(kind)
	c.Paused = pausedInt != 0
	c.ResumeEnabled = resumeEnabledInt != 0
	return c, nil
}

// sets a chat's paused flag
func (s *Store) SetChatPaused(ctx context.Context, telegramID int64, paused bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE chats SET paused = ? WHERE telegram_id = ?`, boolToInt(paused), telegramID)
	return err
}

// updates a chat's tag
func (s *Store) SetChatTag(ctx context.Context, telegramID int64, tag string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE chats SET tag = ? WHERE telegram_id = ?`, tag, telegramID)
	return err
}

// stops monitoring a chat, keeping its indexed history
func (s *Store) RemoveChat(ctx context.Context, telegramID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chats WHERE telegram_id = ?`, telegramID)
	return err
}
