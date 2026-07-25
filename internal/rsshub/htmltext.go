package rsshub

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var brTagRe = regexp.MustCompile(`(?i)<br\s*/?>`)
var whitespaceRe = regexp.MustCompile(`[ \t]+`)

type parsedHTML struct {
	Text      string
	ImageURLs []string
}

// normalizes <br>, extracts <img src>, inlines <a href> as "text (url)"
func parseEntryHTML(raw string) parsedHTML {
	if raw == "" {
		return parsedHTML{}
	}
	raw = brTagRe.ReplaceAllString(raw, "\n")

	doc, err := html.Parse(strings.NewReader("<html><body>" + raw + "</body></html>"))
	if err != nil {
		return parsedHTML{Text: normalizeLines(raw)}
	}

	var images []string
	var sb strings.Builder

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "img":
				if src := attr(n, "src"); src != "" {
					images = append(images, strings.TrimSpace(src))
				}
				return
			case "a":
				href := strings.TrimSpace(attr(n, "href"))
				text := strings.TrimSpace(nodeText(n))
				switch {
				case href == "":
					sb.WriteString(text)
				case text == "" || text == href || strings.HasSuffix(href, text):
					sb.WriteString(" ")
					sb.WriteString(href)
					sb.WriteString(" ")
				default:
					sb.WriteString(" ")
					sb.WriteString(text)
					sb.WriteString(" (")
					sb.WriteString(href)
					sb.WriteString(") ")
				}
				return
			}
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return parsedHTML{Text: normalizeLines(sb.String()), ImageURLs: images}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// collects only the visible text of a node, ignoring markup
func nodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// collapses runs of spaces/tabs per line and drops empty lines.
func normalizeLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = whitespaceRe.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
