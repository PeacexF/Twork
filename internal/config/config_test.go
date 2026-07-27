package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writes cfg to a temp file and returns its path
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

const minimalMTProto = `
telegram:
  app_id: 123
  app_hash: abc
  phone: "+10000000000"
bot:
  token: "bot-token"
matching:
  positive:
    - golang
`

// a minimal mtproto config loads and gets every documented default
func TestLoad_AppliesDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalMTProto))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Source.Kind != SourceMTProto {
		t.Errorf("source.kind default = %q, want %q", cfg.Source.Kind, SourceMTProto)
	}
	if cfg.Telegram.Session != "session.session" {
		t.Errorf("telegram.session default = %q", cfg.Telegram.Session)
	}
	if cfg.RSSHub.PollIntervalSeconds != 120 {
		t.Errorf("rsshub.poll_interval_seconds default = %d, want 120", cfg.RSSHub.PollIntervalSeconds)
	}
	if cfg.Database.SQLite != "./data/twork.db" {
		t.Errorf("database.sqlite default = %q", cfg.Database.SQLite)
	}
	if cfg.Matching.Mode != MatchModeWholeWord {
		t.Errorf("matching.mode default = %q, want %q", cfg.Matching.Mode, MatchModeWholeWord)
	}
}

// explicit values are never overwritten by the defaults
func TestLoad_KeepsExplicitValues(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
source:
  kind: rsshub
rsshub:
  base_url: "http://localhost:1200/telegram/channel/{channel}"
  poll_interval_seconds: 30
bot:
  token: "bot-token"
database:
  sqlite: "/tmp/custom.db"
telegram:
  session: "custom.session"
matching:
  mode: substring
  positive:
    - golang
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Source.Kind != SourceRSSHub {
		t.Errorf("source.kind = %q", cfg.Source.Kind)
	}
	if cfg.RSSHub.PollIntervalSeconds != 30 {
		t.Errorf("poll interval = %d, want 30", cfg.RSSHub.PollIntervalSeconds)
	}
	if cfg.Database.SQLite != "/tmp/custom.db" {
		t.Errorf("sqlite path = %q", cfg.Database.SQLite)
	}
	if cfg.Telegram.Session != "custom.session" {
		t.Errorf("session = %q", cfg.Telegram.Session)
	}
	if cfg.Matching.Mode != MatchModeSubstring {
		t.Errorf("mode = %q", cfg.Matching.Mode)
	}
}

// keyword groups round-trip out of YAML
func TestLoad_ParsesKeywordGroups(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
telegram:
  app_id: 1
  app_hash: h
  phone: "+1"
bot:
  token: t
matching:
  positive:
    - golang
  positive_groups:
    - name: Go
      aliases: [go, golang]
      mode: substring
  negative_groups:
    - name: Seniority
      aliases: [senior]
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Matching.PositiveGroups) != 1 {
		t.Fatalf("expected 1 positive group, got %+v", cfg.Matching.PositiveGroups)
	}
	g := cfg.Matching.PositiveGroups[0]
	if g.Name != "Go" || len(g.Aliases) != 2 || g.Mode != MatchModeSubstring {
		t.Errorf("positive group mismatch: %+v", g)
	}
	if len(cfg.Matching.NegativeGroups) != 1 || cfg.Matching.NegativeGroups[0].Name != "Seniority" {
		t.Errorf("negative group mismatch: %+v", cfg.Matching.NegativeGroups)
	}
	// A group with no explicit mode inherits the global one at match time.
	if cfg.Matching.NegativeGroups[0].Mode != "" {
		t.Errorf("expected empty group mode to be preserved, got %q", cfg.Matching.NegativeGroups[0].Mode)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	if _, err := Load(writeConfig(t, "bot: [this is not a mapping")); err == nil {
		t.Fatal("expected a parse error for malformed YAML")
	}
}

// every validation rule rejects the config it's meant to reject
func TestLoad_ValidationErrors(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantHint string
	}{
		{"missing app_id", `
telegram: {app_hash: h, phone: "+1"}
bot: {token: t}
matching: {positive: [go]}
`, "telegram.app_id"},
		{"missing app_hash", `
telegram: {app_id: 1, phone: "+1"}
bot: {token: t}
matching: {positive: [go]}
`, "telegram.app_hash"},
		{"missing phone", `
telegram: {app_id: 1, app_hash: h}
bot: {token: t}
matching: {positive: [go]}
`, "telegram.phone"},
		{"rsshub without base_url", `
source: {kind: rsshub}
bot: {token: t}
matching: {positive: [go]}
`, "rsshub.base_url is required"},
		{"rsshub base_url without placeholder", `
source: {kind: rsshub}
rsshub: {base_url: "http://localhost:1200/telegram/channel/golang"}
bot: {token: t}
matching: {positive: [go]}
`, "{channel}"},
		{"unknown source kind", `
source: {kind: carrier_pigeon}
bot: {token: t}
matching: {positive: [go]}
`, "source.kind must be"},
		{"missing bot token", `
telegram: {app_id: 1, app_hash: h, phone: "+1"}
matching: {positive: [go]}
`, "bot.token is required"},
		{"no positive keywords", `
telegram: {app_id: 1, app_hash: h, phone: "+1"}
bot: {token: t}
`, "matching.positive"},
		{"invalid matching mode", `
telegram: {app_id: 1, app_hash: h, phone: "+1"}
bot: {token: t}
matching: {positive: [go], mode: fuzzy}
`, "matching.mode must be"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, c.body))
			if err == nil {
				t.Fatalf("expected a validation error mentioning %q", c.wantHint)
			}
			if !strings.Contains(err.Error(), c.wantHint) {
				t.Errorf("error = %v, want it to mention %q", err, c.wantHint)
			}
		})
	}
}

// the shipped example config is valid apart from the credentials the user fills in
func TestLoad_ExampleConfigParses(t *testing.T) {
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Skipf("config.example.yaml not readable: %v", err)
	}
	path := writeConfig(t, string(data))
	if _, err := Load(path); err != nil {
		// The example intentionally ships with placeholder credentials, so a
		// validation error is fine -- a *parse* error is not.
		if strings.Contains(err.Error(), "parsing config") {
			t.Fatalf("config.example.yaml does not parse: %v", err)
		}
	}
}
