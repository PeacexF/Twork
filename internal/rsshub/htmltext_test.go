package rsshub

import "testing"

// covers the HTML-to-plain-text rules applied to every feed entry
func TestParseEntryHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain text", "Go backend developer", "Go backend developer"},
		{"br becomes a newline", "line one<br>line two", "line one\nline two"},
		{"self-closing br", "line one<br/>line two", "line one\nline two"},
		{"spaced br", "line one<br />line two", "line one\nline two"},
		{"uppercase br", "line one<BR>line two", "line one\nline two"},
		{"images are dropped", `before<img src="x.png">after`, "beforeafter"},
		{"markup is stripped", "<b>Bold</b> and <i>italic</i>", "Bold and italic"},
		{"runs of spaces collapse", "too     many   spaces", "too many spaces"},
		{"tabs collapse", "tab\tseparated", "tab separated"},
		{"blank lines are dropped", "first<br><br><br>second", "first\nsecond"},
		{"surrounding whitespace is trimmed", "   padded   ", "padded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseEntryHTML(c.in); got != c.want {
				t.Errorf("parseEntryHTML(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// links are inlined as "text (url)", or bare when the text adds nothing
func TestParseEntryHTML_Links(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"labelled link keeps both",
			`Apply <a href="https://jobs.example/1">here</a>`,
			"Apply here (https://jobs.example/1)",
		},
		{
			"link text equal to the href isn't repeated",
			`<a href="https://jobs.example/1">https://jobs.example/1</a>`,
			"https://jobs.example/1",
		},
		{
			"link text that's a suffix of the href isn't repeated",
			`<a href="https://t.me/golang_jobs">golang_jobs</a>`,
			"https://t.me/golang_jobs",
		},
		{
			"empty link text falls back to the href",
			`<a href="https://jobs.example/1"></a>`,
			"https://jobs.example/1",
		},
		{
			"a link with no href keeps just its text",
			`<a>plain</a>`,
			"plain",
		},
		{
			"nested markup inside a link is flattened",
			`<a href="https://jobs.example/1"><b>Apply</b> now</a>`,
			"Apply now (https://jobs.example/1)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseEntryHTML(c.in); got != c.want {
				t.Errorf("parseEntryHTML(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// a realistic RSSHub entry survives the whole pipeline
func TestParseEntryHTML_RealisticEntry(t *testing.T) {
	in := `<b>Backend Go Developer</b><br><br>` +
		`Remote, $5k-7k<br>` +
		`Stack: Go, Docker, PostgreSQL<br><br>` +
		`<img src="https://cdn.example/logo.png">` +
		`Contact: <a href="https://t.me/recruiter">@recruiter</a>`

	want := "Backend Go Developer\n" +
		"Remote, $5k-7k\n" +
		"Stack: Go, Docker, PostgreSQL\n" +
		"Contact: @recruiter (https://t.me/recruiter)"

	if got := parseEntryHTML(in); got != want {
		t.Errorf("parseEntryHTML() =\n%q\nwant\n%q", got, want)
	}
}

func TestNormalizeLines(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"  \n  \n  ", ""},
		{"a\n\n\nb", "a\nb"},
		{"  a  b  ", "a b"},
		{"a \n b \n c", "a\nb\nc"},
	}
	for _, c := range cases {
		if got := normalizeLines(c.in); got != c.want {
			t.Errorf("normalizeLines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
