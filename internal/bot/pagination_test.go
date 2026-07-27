package bot

import "testing"

// paginate clamps out-of-range pages and never returns bounds a slice can't take
func TestPaginate(t *testing.T) {
	cases := []struct {
		name               string
		total, page, size  int
		wantStart, wantEnd int
	}{
		{"first page of a full set", 20, 0, 8, 0, 8},
		{"middle page", 20, 1, 8, 8, 16},
		{"last, partial page", 20, 2, 8, 16, 20},
		{"page past the end clamps to the last one", 20, 99, 8, 16, 20},
		{"negative page clamps to the first", 20, -5, 8, 0, 8},
		{"empty set", 0, 0, 8, 0, 0},
		{"empty set, page past the end", 0, 7, 8, 0, 0},
		{"exactly one full page", 8, 0, 8, 0, 8},
		{"one item", 1, 0, 8, 0, 1},
		{"non-positive size falls back to 1", 3, 1, 0, 1, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := paginate(c.total, c.page, c.size)
			if start != c.wantStart || end != c.wantEnd {
				t.Errorf("paginate(%d, %d, %d) = (%d, %d), want (%d, %d)",
					c.total, c.page, c.size, start, end, c.wantStart, c.wantEnd)
			}
			// Whatever comes back must be a legal slice expression.
			if start < 0 || end < start || end > c.total {
				t.Errorf("bounds (%d, %d) are not slicable against a length of %d", start, end, c.total)
			}
		})
	}
}

// the bounds paginate returns always slice a real list without panicking
func TestPaginate_BoundsAreAlwaysSlicable(t *testing.T) {
	for total := 0; total <= 20; total++ {
		list := make([]int, total)
		for page := -2; page <= total+2; page++ {
			start, end := paginate(total, page, pageSize)
			_ = list[start:end] // panics on bad bounds
		}
	}
}

// the nav row shows a 1-based position and wires prev/next to the right pages
func TestNavRow(t *testing.T) {
	row := navRow("chat:page:", 1, 20, 8)
	if len(row) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(row))
	}
	if got := *row[0].CallbackData; got != "chat:page:0" {
		t.Errorf("prev = %q, want chat:page:0", got)
	}
	if row[1].Text != "2/3" {
		t.Errorf("position label = %q, want 2/3", row[1].Text)
	}
	if got := *row[2].CallbackData; got != "chat:page:2" {
		t.Errorf("next = %q, want chat:page:2", got)
	}
}

// an empty list shows 0/0 rather than 1/1
func TestNavRow_Empty(t *testing.T) {
	row := navRow("list:page:", 0, 0, 8)
	if row[1].Text != "0/0" {
		t.Errorf("position label = %q, want 0/0", row[1].Text)
	}
}

// a non-positive page size is treated as 1, so every item is its own page
func TestNavRow_NonPositiveSize(t *testing.T) {
	row := navRow("list:page:", 0, 3, 0)
	if row[1].Text != "1/3" {
		t.Errorf("position label = %q, want 1/3", row[1].Text)
	}
}

// the prev button on page 0 points at -1; paginate clamps it back on the way in
func TestNavRow_PrevFromFirstPageIsClampedOnUse(t *testing.T) {
	row := navRow("chat:page:", 0, 20, 8)
	if got := *row[0].CallbackData; got != "chat:page:-1" {
		t.Fatalf("prev = %q, want chat:page:-1", got)
	}
	if start, end := paginate(20, -1, 8); start != 0 || end != 8 {
		t.Errorf("paginate should clamp page -1 back to the first page, got (%d, %d)", start, end)
	}
}

func TestHasPrefix(t *testing.T) {
	cases := []struct {
		s, prefix string
		want      bool
	}{
		{"chat:add", "chat:", true},
		{"chat:", "chat:", true},
		{"cha", "chat:", false}, // shorter than the prefix
		{"list:page:1", "chat:", false},
		{"", "", true},
		{"anything", "", true},
	}
	for _, c := range cases {
		if got := hasPrefix(c.s, c.prefix); got != c.want {
			t.Errorf("hasPrefix(%q, %q) = %v, want %v", c.s, c.prefix, got, c.want)
		}
	}
}

// every namespace the callback router knows is recognized by its prefix
func TestHasPrefix_CoversEveryCallbackNamespace(t *testing.T) {
	for _, ns := range []string{"chat:", "list:", "kw:", "settings:", "notify:"} {
		data := ns + "something"
		if !hasPrefix(data, ns) {
			t.Errorf("%q should match namespace %q", data, ns)
		}
		if hasPrefix(data, "unrelated:") {
			t.Errorf("%q should not match an unrelated namespace", data)
		}
	}
}

func TestHumanCount(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0k"},
		{12500, "12.5k"},
		{999_999, "1000.0k"},
		{1_000_000, "1.0M"},
		{2_500_000, "2.5M"},
	}
	for _, c := range cases {
		if got := humanCount(c.in); got != c.want {
			t.Errorf("humanCount(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// authorized only accepts the claimed owner, and nobody at all before /start
func TestAuthorized(t *testing.T) {
	b := &Bot{}
	if b.authorized(123) {
		t.Error("expected nobody to be authorized before an owner is claimed")
	}
	if b.authorized(0) {
		t.Error("expected user 0 to be unauthorized when no owner is claimed")
	}

	b.ownerID = 123
	if !b.authorized(123) {
		t.Error("expected the owner to be authorized")
	}
	if b.authorized(456) {
		t.Error("expected a non-owner to be rejected")
	}
}

// the home keyboard exposes exactly the documented menu entries
func TestHomeDashboardKeyboard(t *testing.T) {
	kb := homeDashboardKeyboard()

	want := map[string]bool{
		"menu:matches": false, "menu:favorites": false, "menu:search": false,
		"menu:chats": false, "menu:keywords": false, "menu:settings": false,
	}
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				t.Fatalf("menu button %q has no callback data", btn.Text)
			}
			if _, ok := want[*btn.CallbackData]; !ok {
				t.Errorf("unexpected menu button %q", *btn.CallbackData)
				continue
			}
			want[*btn.CallbackData] = true
		}
	}
	for data, seen := range want {
		if !seen {
			t.Errorf("menu is missing %q", data)
		}
	}
}

// the Back button always carries the target it was given
func TestBackButton(t *testing.T) {
	for _, target := range []string{"menu:home", "chat:page:0", "kw:menu", "menu:settings"} {
		btn := backButton(target)
		if btn.CallbackData == nil || *btn.CallbackData != target {
			t.Errorf("backButton(%q) callback = %v", target, btn.CallbackData)
		}
		if btn.Text == "" {
			t.Error("expected the Back button to have a label")
		}
	}
}

// the empty-list keyboard offers a single way out: home
func TestHomeOnlyKeyboard(t *testing.T) {
	kb := homeOnlyKeyboard()
	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("expected a single button, got %+v", kb.InlineKeyboard)
	}
	if got := *kb.InlineKeyboard[0][0].CallbackData; got != "menu:home" {
		t.Errorf("callback = %q, want menu:home", got)
	}
}

// the paginated chat list never renders more than one page of buttons
func TestChatListPaging_NeverExceedsPageSize(t *testing.T) {
	for total := 0; total < 30; total++ {
		for page := 0; page*pageSize <= total; page++ {
			start, end := paginate(total, page, pageSize)
			if n := end - start; n > pageSize {
				t.Fatalf("total=%d page=%d produced %d rows, more than the page size %d",
					total, page, n, pageSize)
			}
		}
	}
}
