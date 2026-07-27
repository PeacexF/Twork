package rsshub

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var brTagRe = regexp.MustCompile(`(?i)<br\s*/?>`)
var whitespaceRe = regexp.MustCompile(`[ \t]+`)

// converts a feed entry's HTML to plain text: normalizes <br>, drops <img>,
// inlines <a href> as "text (url)"
func parseEntryHTML(raw string) string {
	if raw == "" {
		return ""
	}
	raw = brTagRe.ReplaceAllString(raw, "\n")

	doc, err := html.Parse(strings.NewReader("<html><body>" + raw + "</body></html>"))
	if err != nil {
		return normalizeLines(raw)
	}

	var sb strings.Builder

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "img":
				return // images aren't indexed; skip the node entirely
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

	return normalizeLines(sb.String())
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
