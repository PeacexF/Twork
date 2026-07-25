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
	settingKeywordsPositive       = "keywords.positive"
	settingKeywordsNegative       = "keywords.negative"
	settingKeywordsMode           = "keywords.mode"
	settingKeywordsPositiveGroups = "keywords.positive_groups"
	settingKeywordsNegativeGroups = "keywords.negative_groups"
	settingNotificationsEnabled   = "notifications.enabled"
	settingBotOwnerID             = "bot.owner_id"
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

// a named set of aliases with an optional per-group matching mode
type KeywordGroup struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Mode    string   `json:"mode,omitempty"`
}

type Keywords struct {
	PositiveGroups []KeywordGroup
	NegativeGroups []KeywordGroup
	Mode           string
}

// loads persisted keyword groups, migrating legacy flat keyword lists if present
func (s *Store) GetKeywords(ctx context.Context) (kw Keywords, ok bool, err error) {
	mode, _, err := s.GetSetting(ctx, settingKeywordsMode)
	if err != nil {
		return kw, false, err
	}
	kw.Mode = mode

	posGroups, hasPosGroups, err := s.getGroups(ctx, settingKeywordsPositiveGroups)
	if err != nil {
		return kw, false, err
	}
	if hasPosGroups {
		negGroups, _, err := s.getGroups(ctx, settingKeywordsNegativeGroups)
		if err != nil {
			return kw, false, err
		}
		kw.PositiveGroups = posGroups
		kw.NegativeGroups = negGroups
		return kw, true, nil
	}

	// No group keys yet: fall back to legacy flat lists (from an older
	// version) and migrate each keyword into a single-alias group.
	pos, hasPos, err := s.getFlatAsGroups(ctx, settingKeywordsPositive)
	if err != nil {
		return kw, false, err
	}
	if !hasPos {
		return kw, false, nil
	}
	neg, _, err := s.getFlatAsGroups(ctx, settingKeywordsNegative)
	if err != nil {
		return kw, false, err
	}
	kw.PositiveGroups = pos
	kw.NegativeGroups = neg
	return kw, true, nil
}

// reads and decodes a JSON group list from one setting key
func (s *Store) getGroups(ctx context.Context, key string) ([]KeywordGroup, bool, error) {
	raw, ok, err := s.GetSetting(ctx, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	var groups []KeywordGroup
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &groups); err != nil {
			return nil, false, fmt.Errorf("decoding stored groups %q: %w", key, err)
		}
	}
	return groups, true, nil
}

// reads a legacy flat JSON string list and converts each entry to a single-alias group
func (s *Store) getFlatAsGroups(ctx context.Context, key string) ([]KeywordGroup, bool, error) {
	raw, ok, err := s.GetSetting(ctx, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	var words []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &words); err != nil {
			return nil, false, fmt.Errorf("decoding legacy keywords %q: %w", key, err)
		}
	}
	groups := make([]KeywordGroup, 0, len(words))
	for _, w := range words {
		groups = append(groups, KeywordGroup{Name: w, Aliases: []string{w}})
	}
	return groups, true, nil
}

// persists keyword groups
func (s *Store) SetKeywords(ctx context.Context, kw Keywords) error {
	posJSON, err := json.Marshal(kw.PositiveGroups)
	if err != nil {
		return err
	}
	negJSON, err := json.Marshal(kw.NegativeGroups)
	if err != nil {
		return err
	}
	if err := s.SetSetting(ctx, settingKeywordsPositiveGroups, string(posJSON)); err != nil {
		return err
	}
	if err := s.SetSetting(ctx, settingKeywordsNegativeGroups, string(negJSON)); err != nil {
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
