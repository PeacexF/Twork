package bot

import (
	"fmt"
	"regexp"
	"strings"
)

// ChatInputKind classifies what the user pasted into the add-chat prompt.
type ChatInputKind int

const (
	ChatInputKindUsername ChatInputKind = iota
	ChatInputKindInvite
	ChatInputKindFolder
	ChatInputKindUnknown
)

// ParsedChatInput is the normalized result of whatever the user typed.
// Exported so internal/web can drive the same add-chat flow the bot uses.
type ParsedChatInput struct {
	Kind  ChatInputKind
	Value string // bare username, invite hash, or folder slug depending on Kind
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{4,32}$`)

// ParseChatInput classifies and normalizes a pasted username/link into a
// username, invite hash, or folder slug.
func ParseChatInput(raw string) (ParsedChatInput, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ParsedChatInput{}, fmt.Errorf("nothing was entered")
	}

	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	s = strings.TrimPrefix(s, "t.me/")
	s = strings.TrimPrefix(s, "telegram.me/")
	s = strings.TrimPrefix(s, "telegram.dog/")
	s = strings.TrimPrefix(s, "/")

	switch {
	case strings.HasPrefix(s, "addlist/"):
		slug := strings.TrimPrefix(s, "addlist/")
		if slug == "" {
			return ParsedChatInput{}, fmt.Errorf("that folder link is missing its code")
		}
		return ParsedChatInput{Kind: ChatInputKindFolder, Value: slug}, nil

	case strings.HasPrefix(s, "+"):
		return ParsedChatInput{Kind: ChatInputKindInvite, Value: strings.TrimPrefix(s, "+")}, nil

	case strings.HasPrefix(s, "joinchat/"):
		return ParsedChatInput{Kind: ChatInputKindInvite, Value: strings.TrimPrefix(s, "joinchat/")}, nil
	}

	// Strip a leading @ and anything after the username (e.g. t.me/name/123 -> name).
	s = strings.TrimPrefix(s, "@")
	if i := strings.IndexAny(s, "/?"); i != -1 {
		s = s[:i]
	}

	if s == "" {
		return ParsedChatInput{}, fmt.Errorf("couldn't find a username in that")
	}
	if !usernameRe.MatchString(s) {
		return ParsedChatInput{}, fmt.Errorf("%q doesn't look like a valid Telegram username or link", raw)
	}
	return ParsedChatInput{Kind: ChatInputKindUsername, Value: s}, nil
}
