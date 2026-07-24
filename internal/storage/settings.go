package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const settingsSchema = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

const (
	settingKeywordsPositive     = "keywords.positive"
	settingKeywordsNegative     = "keywords.negative"
	settingKeywordsMode         = "keywords.mode"
	settingNotificationsEnabled = "notifications.enabled"
	settingBotOwnerID           = "bot.owner_id"
)

// reads one raw setting value
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// writes one raw setting value
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

type Keywords struct {
	Positive []string
	Negative []string
	Mode     string
}

// loads persisted keyword configuration
func (s *Store) GetKeywords(ctx context.Context) (kw Keywords, ok bool, err error) {
	pos, hasPos, err := s.GetSetting(ctx, settingKeywordsPositive)
	if err != nil {
		return kw, false, err
	}
	if !hasPos {
		return kw, false, nil
	}
	if err := json.Unmarshal([]byte(pos), &kw.Positive); err != nil {
		return kw, false, fmt.Errorf("decoding stored positive keywords: %w", err)
	}
	neg, _, err := s.GetSetting(ctx, settingKeywordsNegative)
	if err != nil {
		return kw, false, err
	}
	if neg != "" {
		if err := json.Unmarshal([]byte(neg), &kw.Negative); err != nil {
			return kw, false, fmt.Errorf("decoding stored negative keywords: %w", err)
		}
	}
	mode, _, err := s.GetSetting(ctx, settingKeywordsMode)
	if err != nil {
		return kw, false, err
	}
	kw.Mode = mode
	return kw, true, nil
}

// persists keyword configuration
func (s *Store) SetKeywords(ctx context.Context, kw Keywords) error {
	posJSON, err := json.Marshal(kw.Positive)
	if err != nil {
		return err
	}
	negJSON, err := json.Marshal(kw.Negative)
	if err != nil {
		return err
	}
	if err := s.SetSetting(ctx, settingKeywordsPositive, string(posJSON)); err != nil {
		return err
	}
	if err := s.SetSetting(ctx, settingKeywordsNegative, string(negJSON)); err != nil {
		return err
	}
	return s.SetSetting(ctx, settingKeywordsMode, kw.Mode)
}

// reads the notifications toggle
func (s *Store) GetNotificationsEnabled(ctx context.Context) (enabled bool, ok bool, err error) {
	v, ok, err := s.GetSetting(ctx, settingNotificationsEnabled)
	if err != nil || !ok {
		return false, ok, err
	}
	return v == "1", true, nil
}

// writes the notifications toggle
func (s *Store) SetNotificationsEnabled(ctx context.Context, enabled bool) error {
	v := "0"
	if enabled {
		v = "1"
	}
	return s.SetSetting(ctx, settingNotificationsEnabled, v)
}

// reads the claimed owner ID
func (s *Store) GetBotOwnerID(ctx context.Context) (int64, bool, error) {
	v, ok, err := s.GetSetting(ctx, settingBotOwnerID)
	if err != nil || !ok {
		return 0, ok, err
	}
	var id int64
	if _, err := fmt.Sscanf(v, "%d", &id); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// persists the claimed owner ID
func (s *Store) SetBotOwnerID(ctx context.Context, id int64) error {
	return s.SetSetting(ctx, settingBotOwnerID, fmt.Sprintf("%d", id))
}
