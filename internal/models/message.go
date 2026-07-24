package models

import "time"

type Message struct {
	TelegramMessageID int
	ChatID            int64
	ChatTitle         string
	SenderID          int64
	SenderName        string
	Timestamp         time.Time
	Text              string
	Link              string
	ForwardFromTitle  string
	EditTimestamp     *time.Time
}

type MatchResult struct {
	MessageID       int64
	MatchedKeywords []string
	NegativeKeyword string
}

// reports whether the match has at least one keyword and no negative hit
func (m MatchResult) Matched() bool {
	return len(m.MatchedKeywords) > 0 && m.NegativeKeyword == ""
}

type Chat struct {
	ID         int64
	TelegramID int64
	AccessHash int64
	Kind       ChatKind
	Title      string
	Username   string
	Tag        string
	Paused     bool
	CreatedAt  time.Time
}

type ChatKind string

const (
	ChatKindChannel ChatKind = "channel"
	ChatKindGroup   ChatKind = "group"
)
