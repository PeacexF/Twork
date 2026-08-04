package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/PeacexF/Twork/internal/models"
)

// parses a timestamp string in whatever format the sqlite3 driver stored it
// in -- needed for columns the driver can't auto-convert itself, i.e.
// computed/subquery expressions rather than a real DATETIME column
func parseSQLiteTime(s string) (time.Time, error) {
	var lastErr error
	for _, layout := range sqlite3.SQLiteTimestampFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

// a monitored chat with its resume broadcasting schedule state
type ResumeChat struct {
	models.Chat
	LastSentAt *time.Time
}

// reads the global resume text, defaulting to "" if unset
func (s *Store) GetResumeGlobalText(ctx context.Context) (string, error) {
	v, _, err := s.GetSetting(ctx, settingResumeGlobalText)
	return v, err
}

// reports whether the compliance limits have ever been explicitly set (by
// config-seed bootstrap or the bot/dashboard), as opposed to just defaulting
func (s *Store) ResumeComplianceConfigured(ctx context.Context) (bool, error) {
	_, ok, err := s.GetSetting(ctx, settingResumeMinDelaySeconds)
	return ok, err
}

// writes the global resume text
func (s *Store) SetResumeGlobalText(ctx context.Context, text string) error {
	return s.SetSetting(ctx, settingResumeGlobalText, text)
}

// reads the minimum seconds between two resume sends into the same chat, defaulting to defaultVal if unset
func (s *Store) GetResumeMinDelaySeconds(ctx context.Context, defaultVal int) (int, error) {
	return s.getResumeIntSetting(ctx, settingResumeMinDelaySeconds, defaultVal)
}

// writes the minimum seconds between two resume sends into the same chat
func (s *Store) SetResumeMinDelaySeconds(ctx context.Context, seconds int) error {
	return s.SetSetting(ctx, settingResumeMinDelaySeconds, strconv.Itoa(seconds))
}

// reads the max resume sends allowed per rolling hour across all chats combined, defaulting to defaultVal if unset
func (s *Store) GetResumeMaxPerHour(ctx context.Context, defaultVal int) (int, error) {
	return s.getResumeIntSetting(ctx, settingResumeMaxPerHour, defaultVal)
}

// writes the max resume sends allowed per rolling hour across all chats combined
func (s *Store) SetResumeMaxPerHour(ctx context.Context, n int) error {
	return s.SetSetting(ctx, settingResumeMaxPerHour, strconv.Itoa(n))
}

// reads an integer setting, falling back to defaultVal if unset or unparsable
func (s *Store) getResumeIntSetting(ctx context.Context, key string, defaultVal int) (int, error) {
	v, ok, err := s.GetSetting(ctx, key)
	if err != nil {
		return defaultVal, err
	}
	if !ok || v == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal, nil
	}
	return n, nil
}

// updates a chat's resume broadcasting config. Refuses to enable it on
// anything but a group -- channels are broadcast-only (a regular member
// can't post, and an admin "sending" would blast every subscriber) and
// DMs are never addressable here in the first place.
func (s *Store) SetChatResumeConfig(ctx context.Context, telegramID int64, enabled bool, intervalSeconds int, text string) error {
	if enabled {
		chat, err := s.GetChatByTelegramID(ctx, telegramID)
		if err != nil {
			return err
		}
		if chat == nil {
			return fmt.Errorf("chat %d is not monitored", telegramID)
		}
		if chat.Kind != models.ChatKindGroup {
			return fmt.Errorf("resume broadcasting only works in groups, not %s chats", chat.Kind)
		}
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE chats SET resume_enabled = ?, resume_interval_seconds = ?, resume_text = ?
		WHERE telegram_id = ?
	`, boolToInt(enabled), intervalSeconds, text, telegramID)
	return err
}

// lists chats with resume broadcasting enabled, each with its most recent send time
func (s *Store) ListResumeEnabledChats(ctx context.Context) ([]ResumeChat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.telegram_id, c.access_hash, c.kind, c.title, c.username, c.tag, c.paused,
		       c.resume_enabled, c.resume_interval_seconds, c.resume_text, c.created_at,
		       (SELECT MAX(sent_at) FROM resume_sends rs WHERE rs.chat_id = c.telegram_id)
		FROM chats c
		WHERE c.resume_enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ResumeChat
	for rows.Next() {
		var kind string
		var pausedInt, resumeEnabledInt int
		// A computed subquery column has no declared SQLite column type, so
		// the driver can't auto-convert it to time.Time the way it does for
		// c.created_at (a real DATETIME column) -- it comes back as text.
		var lastSent sql.NullString
		var rc ResumeChat
		if err := rows.Scan(&rc.ID, &rc.TelegramID, &rc.AccessHash, &kind, &rc.Title, &rc.Username, &rc.Tag, &pausedInt,
			&resumeEnabledInt, &rc.ResumeIntervalSeconds, &rc.ResumeText, &rc.CreatedAt, &lastSent); err != nil {
			return nil, err
		}
		rc.Kind = models.ChatKind(kind)
		rc.Paused = pausedInt != 0
		rc.ResumeEnabled = resumeEnabledInt != 0
		if lastSent.Valid {
			t, err := parseSQLiteTime(lastSent.String)
			if err != nil {
				return nil, fmt.Errorf("parsing last resume send time: %w", err)
			}
			rc.LastSentAt = &t
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// records a successful resume send
func (s *Store) RecordResumeSend(ctx context.Context, telegramID int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO resume_sends (chat_id) VALUES (?)`, telegramID)
	return err
}

// counts resume sends across all chats combined since the given time
func (s *Store) CountResumeSendsSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM resume_sends WHERE sent_at >= ?`, since.UTC()).Scan(&n)
	return n, err
}
