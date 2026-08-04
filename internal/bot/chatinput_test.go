package bot

import "testing"

// covers username/link/invite/folder classification and normalization
func TestParseChatInput(t *testing.T) {
	cases := []struct {
		in       string
		wantKind ChatInputKind
		wantVal  string
		wantErr  bool
	}{
		{"frilancru", ChatInputKindUsername, "frilancru", false},
		{"@frilancru", ChatInputKindUsername, "frilancru", false},
		{"https://t.me/frilancru", ChatInputKindUsername, "frilancru", false},
		{"http://t.me/frilancru", ChatInputKindUsername, "frilancru", false},
		{"t.me/frilancru", ChatInputKindUsername, "frilancru", false},
		{"t.me/frilancru/1234", ChatInputKindUsername, "frilancru", false},
		{"https://t.me/frilancru?foo=bar", ChatInputKindUsername, "frilancru", false},
		{"  @frilancru  ", ChatInputKindUsername, "frilancru", false},
		{"telegram.me/frilancru", ChatInputKindUsername, "frilancru", false},
		{"https://t.me/+AbCdEf123", ChatInputKindInvite, "AbCdEf123", false},
		{"t.me/+AbCdEf123", ChatInputKindInvite, "AbCdEf123", false},
		{"https://t.me/joinchat/AbCdEf123", ChatInputKindInvite, "AbCdEf123", false},
		{"https://t.me/addlist/SomeSlug", ChatInputKindFolder, "SomeSlug", false},
		{"t.me/addlist/SomeSlug", ChatInputKindFolder, "SomeSlug", false},
		{"", ChatInputKindUnknown, "", true},
		{"a", ChatInputKindUnknown, "", true},          // too short
		{"has spaces", ChatInputKindUnknown, "", true}, // invalid char
		{"t.me/addlist/", ChatInputKindUnknown, "", true},
	}

	for _, c := range cases {
		got, err := ParseChatInput(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseChatInput(%q): expected error, got %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseChatInput(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got.Kind != c.wantKind || got.Value != c.wantVal {
			t.Errorf("ParseChatInput(%q) = {%d, %q}, want {%d, %q}", c.in, got.Kind, got.Value, c.wantKind, c.wantVal)
		}
	}
}
