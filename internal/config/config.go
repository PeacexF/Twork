package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram      TelegramConfig      `yaml:"telegram"`
	Bot           BotConfig           `yaml:"bot"`
	Database      DatabaseConfig      `yaml:"database"`
	Matching      MatchingConfig      `yaml:"matching"`
	Notifications NotificationsConfig `yaml:"notifications"`

	Chats []ChatConfig `yaml:"chats"`
}

type BotConfig struct {
	Token string `yaml:"token"`

	OwnerID int64 `yaml:"owner_id"`
}

type TelegramConfig struct {
	AppID   int    `yaml:"app_id"`
	AppHash string `yaml:"app_hash"`
	Phone   string `yaml:"phone"`
	Session string `yaml:"session"`
}

type DatabaseConfig struct {
	SQLite string `yaml:"sqlite"`
}

type MatchingConfig struct {
	Positive []string `yaml:"positive"`
	Negative []string `yaml:"negative"`

	Mode string `yaml:"mode"`
}

const (
	MatchModeWholeWord = "whole_word"
	MatchModeSubstring = "substring"
)

type NotificationsConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ChatConfig struct {
	Username string `yaml:"username"`
	ID       int64  `yaml:"id"`
	Tag      string `yaml:"tag"`
	Paused   bool   `yaml:"paused"`
}

// reads, defaults, and validates config.yaml
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}

	if cfg.Telegram.Session == "" {
		cfg.Telegram.Session = "session.session"
	}
	if cfg.Database.SQLite == "" {
		cfg.Database.SQLite = "./data/twork.db"
	}

	if cfg.Matching.Mode == "" {
		cfg.Matching.Mode = MatchModeWholeWord
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// checks required fields
func (c *Config) validate() error {
	if c.Telegram.AppID == 0 {
		return fmt.Errorf("telegram.app_id is required (get one at https://my.telegram.org/apps)")
	}
	if c.Telegram.AppHash == "" {
		return fmt.Errorf("telegram.app_hash is required (get one at https://my.telegram.org/apps)")
	}
	if c.Telegram.Phone == "" {
		return fmt.Errorf("telegram.phone is required for the first interactive login")
	}
	if c.Bot.Token == "" {
		return fmt.Errorf("bot.token is required (create a bot with @BotFather and paste its token here)")
	}
	if len(c.Matching.Positive) == 0 {
		return fmt.Errorf("matching.positive must contain at least one keyword")
	}
	if c.Matching.Mode != MatchModeWholeWord && c.Matching.Mode != MatchModeSubstring {
		return fmt.Errorf("matching.mode must be %q or %q, got %q", MatchModeWholeWord, MatchModeSubstring, c.Matching.Mode)
	}
	return nil
}
