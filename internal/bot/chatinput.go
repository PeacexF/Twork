package bot

import (
	"fmt"
	"regexp"
	"strings"
)

// chatInputKind classifies what the user pasted into the add-chat prompt.
type chatInputKind int

const (
	inputKindUsername chatInputKind = iota
	inputKindInvite
	inputKindFolder
	inputKindUnknown
)

// parsedChatInput is the normalized result of whatever the user typed.
type parsedChatInput struct {
	kind  chatInputKind
	value string // bare username, invite hash, or folder slug depending on kind
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{4,32}$`)

// classifies and normalizes a pasted username/link into a username, invite hash, or folder slug
func parseChatInput(raw string) (parsedChatInput, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return parsedChatInput{}, fmt.Errorf("nothing was entered")
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
			return parsedChatInput{}, fmt.Errorf("that folder link is missing its code")
		}
		return parsedChatInput{kind: inputKindFolder, value: slug}, nil

	case strings.HasPrefix(s, "+"):
		return parsedChatInput{kind: inputKindInvite, value: strings.TrimPrefix(s, "+")}, nil

	case strings.HasPrefix(s, "joinchat/"):
		return parsedChatInput{kind: inputKindInvite, value: strings.TrimPrefix(s, "joinchat/")}, nil
	}

	// Strip a leading @ and anything after the username (e.g. t.me/name/123 -> name).
	s = strings.TrimPrefix(s, "@")
	if i := strings.IndexAny(s, "/?"); i != -1 {
		s = s[:i]
	}

	if s == "" {
		return parsedChatInput{}, fmt.Errorf("couldn't find a username in that")
	}
	if !usernameRe.MatchString(s) {
		return parsedChatInput{}, fmt.Errorf("%q doesn't look like a valid Telegram username or link", raw)
	}
	return parsedChatInput{kind: inputKindUsername, value: s}, nil
}
