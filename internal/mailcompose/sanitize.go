package mailcompose

import (
	"strings"

	"golang.org/x/net/html"
)

var allowedHTMLElements = map[string]struct{}{
	"a": {}, "b": {}, "blockquote": {}, "br": {}, "code": {}, "dd": {},
	"div": {}, "em": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {},
	"h6": {}, "hr": {}, "i": {}, "li": {}, "ol": {}, "p": {}, "pre": {},
	"s": {}, "small": {}, "span": {}, "strong": {}, "sub": {}, "sup": {},
	"table": {}, "tbody": {}, "td": {}, "th": {}, "thead": {}, "tr": {},
	"u": {}, "ul": {},
}

var droppedHTMLElements = map[string]struct{}{
	"audio": {}, "base": {}, "button": {}, "embed": {}, "form": {},
	"frame": {}, "frameset": {}, "iframe": {}, "img": {}, "input": {},
	"link": {}, "math": {}, "meta": {}, "object": {}, "script": {},
	"select": {}, "style": {}, "svg": {}, "template": {}, "textarea": {},
	"video": {},
}

const (
	droppedContentWarning = "Removed active, embedded, or remote content from HTML."
	unsafeLinkWarning     = "Removed unsafe or unsupported HTML link destinations."
)

// sanitizeHTML renders a deliberately small structural subset. Style, scripts,
// embedded frames, remote images, form controls, and event handlers are removed.
func sanitizeHTML(value string) (string, []string, error) {
	document, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return "", nil, validationError("HTML could not be parsed")
	}

	body := findHTMLElement(document, "body")
	if body == nil {
		body = document
	} else {
		// Render only the visible body. The parser may have created html and
		// head ancestors, and head text must not become message content.
		var output strings.Builder

		var warnings []string
		for child := body.FirstChild; child != nil; child = child.NextSibling {
			if err := renderSafeNode(&output, child, &warnings); err != nil {
				return "", nil, err
			}
		}

		return output.String(), uniqueWarnings(warnings), nil
	}

	var warnings []string

	var output strings.Builder
	if err := renderSafeNode(&output, body, &warnings); err != nil {
		return "", nil, err
	}

	return output.String(), uniqueWarnings(warnings), nil
}

func findHTMLElement(node *html.Node, name string) *html.Node {
	if node == nil {
		return nil
	}

	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElement(child, name); found != nil {
			return found
		}
	}

	return nil
}

func renderSafeNode(output *strings.Builder, node *html.Node, warnings *[]string) error {
	switch node.Type {
	case html.TextNode:
		output.WriteString(html.EscapeString(node.Data))
		return nil
	case html.ElementNode:
		return renderElement(output, node, warnings)
	case html.CommentNode, html.DoctypeNode:
		return nil
	case html.DocumentNode:
	default:
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := renderSafeNode(output, child, warnings); err != nil {
			return err
		}
	}

	return nil
}

func renderElement(output *strings.Builder, node *html.Node, warnings *[]string) error {
	name := strings.ToLower(node.Data)
	if name == "img" {
		*warnings = append(*warnings, droppedContentWarning)

		alt := attribute(node, "alt")
		if strings.TrimSpace(alt) != "" {
			output.WriteString("[Image: " + html.EscapeString(alt) + "]")
		}

		return nil
	}

	if _, dangerous := droppedHTMLElements[name]; dangerous {
		*warnings = append(*warnings, droppedContentWarning)
		return nil
	}

	if _, allowed := allowedHTMLElements[name]; !allowed {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := renderSafeNode(output, child, warnings); err != nil {
				return err
			}
		}

		return nil
	}

	output.WriteString("<" + name)

	if name == "a" {
		if href := attribute(node, "href"); href != "" && safeURL(href) {
			output.WriteString(` href="` + html.EscapeString(href) + `"`)
		} else if href != "" {
			*warnings = append(*warnings, unsafeLinkWarning)
		}

		if title := attribute(node, "title"); title != "" {
			output.WriteString(` title="` + html.EscapeString(title) + `"`)
		}
	}

	output.WriteString(">")

	if isVoidElement(name) {
		return nil
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := renderSafeNode(output, child, warnings); err != nil {
			return err
		}
	}

	output.WriteString("</" + name + ">")

	return nil
}

func attribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}

	return ""
}

func isVoidElement(name string) bool {
	return name == "br" || name == "hr"
}

// htmlToText converts already-sanitized HTML to a readable fallback. Link text
// is followed by the href when it is not already the visible text.
func htmlToText(value string) string {
	document, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return ""
	}

	body := findHTMLElement(document, "body")
	if body == nil {
		body = document
	}
	var output strings.Builder
	appendHTMLText(&output, body)

	text := strings.ReplaceAll(output.String(), "\r\n", "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return normalizeCRLF(strings.TrimSpace(text))
}

// PlainFromHTML sanitizes HTML before extracting a readable plain-text form.
func PlainFromHTML(value string) string {
	safe, _, err := sanitizeHTML(value)
	if err != nil {
		return ""
	}

	return htmlToText(safe)
}

func appendHTMLText(output *strings.Builder, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		output.WriteString(node.Data)
		return
	case html.CommentNode, html.DoctypeNode:
		return
	case html.ElementNode:
		name := strings.ToLower(node.Data)
		switch name {
		case "br":
			output.WriteString("\n")
			return
		case "hr":
			output.WriteString("\n---\n")
			return
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "li":
			output.WriteString("\n")
		}

		if name == "a" {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				appendHTMLText(output, child)
			}

			if href := attribute(node, "href"); safeURL(href) {
				if text := output.String(); !strings.HasSuffix(strings.TrimSpace(text), href) {
					output.WriteString(" (" + href + ")")
				}
			}

			if name == "li" || name == "p" || name == "div" {
				output.WriteString("\n")
			}

			return
		}
	default:
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendHTMLText(output, child)
	}

	switch strings.ToLower(node.Data) {
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "li":
		output.WriteString("\n")
	}
}
