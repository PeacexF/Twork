package bot

import "testing"

// covers username/link/invite/folder classification and normalization
func TestParseChatInput(t *testing.T) {
	cases := []struct {
		in       string
		wantKind chatInputKind
		wantVal  string
		wantErr  bool
	}{
		{"frilancru", inputKindUsername, "frilancru", false},
		{"@frilancru", inputKindUsername, "frilancru", false},
		{"https://t.me/frilancru", inputKindUsername, "frilancru", false},
		{"http://t.me/frilancru", inputKindUsername, "frilancru", false},
		{"t.me/frilancru", inputKindUsername, "frilancru", false},
		{"t.me/frilancru/1234", inputKindUsername, "frilancru", false},
		{"https://t.me/frilancru?foo=bar", inputKindUsername, "frilancru", false},
		{"  @frilancru  ", inputKindUsername, "frilancru", false},
		{"telegram.me/frilancru", inputKindUsername, "frilancru", false},
		{"https://t.me/+AbCdEf123", inputKindInvite, "AbCdEf123", false},
		{"t.me/+AbCdEf123", inputKindInvite, "AbCdEf123", false},
		{"https://t.me/joinchat/AbCdEf123", inputKindInvite, "AbCdEf123", false},
		{"https://t.me/addlist/SomeSlug", inputKindFolder, "SomeSlug", false},
		{"t.me/addlist/SomeSlug", inputKindFolder, "SomeSlug", false},
		{"", inputKindUnknown, "", true},
		{"a", inputKindUnknown, "", true},          // too short
		{"has spaces", inputKindUnknown, "", true}, // invalid char
		{"t.me/addlist/", inputKindUnknown, "", true},
	}

	for _, c := range cases {
		got, err := parseChatInput(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseChatInput(%q): expected error, got %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseChatInput(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got.kind != c.wantKind || got.value != c.wantVal {
			t.Errorf("parseChatInput(%q) = {%d, %q}, want {%d, %q}", c.in, got.kind, got.value, c.wantKind, c.wantVal)
		}
	}
}
