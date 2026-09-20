package mailcompose

import (
	"net/url"
	"strconv"
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
	"frame": {}, "frameset": {}, "iframe": {}, "input": {}, "link": {},
	"math": {}, "meta": {}, "object": {}, "script": {}, "select": {},
	"style": {}, "svg": {}, "template": {}, "textarea": {}, "video": {},
}

const (
	droppedContentWarning   = "Removed active or embedded content from HTML."
	emptyImageSourceWarning = "Removed an image without a valid source."
	unsafeLinkWarning       = "Removed unsafe or unsupported HTML link destinations."
	remoteImageWarning      = "Remote images are referenced but not fetched; recipients may load them and mail clients may block or alter rendering."
	unsupportedCSSWarning   = "Removed unsupported inline CSS declarations."
)

// RemoteImageWarningName exposes the stable warning identifier used by callers
// that classify mail-composition diagnostics.
func RemoteImageWarningName() string {
	return remoteImageWarning
}

type htmlSanitizer struct {
	inlineContentIDs map[string]string
	styles           *[]StyleDeclaration
	remoteSeen       bool
}

// sanitizeHTML renders a bounded structural subset with parsed inline CSS and
// images that resolve to HTTPS references or supplied inline Content-IDs.
func sanitizeHTML(value string, inlineContentIDs map[string]string, styles *[]StyleDeclaration) (string, []string, error) {
	document, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return "", nil, validationError("HTML could not be parsed")
	}

	body := findHTMLElement(document, "body")
	if body == nil {
		body = document
	}

	sanitizer := &htmlSanitizer{inlineContentIDs: inlineContentIDs, styles: styles}
	var warnings []string
	var output strings.Builder

	children := []*html.Node{body}
	if body != document {
		children = nil
		for child := body.FirstChild; child != nil; child = child.NextSibling {
			children = append(children, child)
		}
	}

	for _, child := range children {
		if err := sanitizer.renderNode(&output, child, &warnings); err != nil {
			return "", nil, err
		}
	}

	if sanitizer.remoteSeen {
		warnings = append(warnings, remoteImageWarning)
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

func (s *htmlSanitizer) renderNode(output *strings.Builder, node *html.Node, warnings *[]string) error {
	switch node.Type {
	case html.TextNode:
		output.WriteString(html.EscapeString(node.Data))
		return nil
	case html.ElementNode:
		return s.renderElement(output, node, warnings)
	case html.CommentNode, html.DoctypeNode:
		return nil
	default:
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := s.renderNode(output, child, warnings); err != nil {
			return err
		}
	}

	return nil
}

func (s *htmlSanitizer) renderElement(output *strings.Builder, node *html.Node, warnings *[]string) error {
	name := strings.ToLower(node.Data)
	if name == "img" {
		return s.renderImage(output, node, warnings)
	}

	if _, dangerous := droppedHTMLElements[name]; dangerous {
		*warnings = append(*warnings, droppedContentWarning)
		return nil
	}

	if _, allowed := allowedHTMLElements[name]; !allowed {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := s.renderNode(output, child, warnings); err != nil {
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

	if style := attribute(node, "style"); style != "" {
		declarations, styleWarnings, err := parseStyleAttribute(style)
		if err != nil {
			return err
		}

		*warnings = append(*warnings, styleWarnings...)

		if s.styles != nil {
			*s.styles = append(*s.styles, declarations...)
		}

		output.WriteString(` style="` + html.EscapeString(styleAttribute(declarations)) + `"`)
	}

	output.WriteString(">")

	if isVoidElement(name) {
		return nil
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := s.renderNode(output, child, warnings); err != nil {
			return err
		}
	}

	output.WriteString("</" + name + ">")

	return nil
}

func (s *htmlSanitizer) renderImage(output *strings.Builder, node *html.Node, warnings *[]string) error {
	if node.FirstChild != nil {
		return validationError("img element contains unsupported child content")
	}

	source := attribute(node, "src")
	remote := false

	switch {
	case source == "":
		*warnings = append(*warnings, emptyImageSourceWarning)
		return nil
	case strings.HasPrefix(strings.ToLower(source), "cid:"):
		contentID, err := cidReference(source)
		if err != nil {
			return err
		}

		contentID = strings.TrimSpace(contentID)

		registeredID, exists := s.inlineContentIDs[strings.ToLower(contentID)]
		if !exists {
			return validationError("HTML references unknown inline attachment %s", contentID)
		}

		source = "cid:" + registeredID
	case safeHTTPSImageURL(source):
		remote = true
		s.remoteSeen = true
	default:
		*warnings = append(*warnings, droppedContentWarning)
		return nil
	}

	output.WriteString(`<img src="` + html.EscapeString(source) + `"`)

	if alt := attribute(node, "alt"); alt != "" {
		output.WriteString(` alt="` + html.EscapeString(alt) + `"`)
	}

	if width, ok := boundedHTMLDimension(attribute(node, "width")); ok {
		output.WriteString(` width="` + width + `"`)
	}

	if height, ok := boundedHTMLDimension(attribute(node, "height")); ok {
		output.WriteString(` height="` + height + `"`)
	}

	if style := attribute(node, "style"); style != "" {
		declarations, styleWarnings, err := parseStyleAttribute(style)
		if err != nil {
			return err
		}

		*warnings = append(*warnings, styleWarnings...)

		if s.styles != nil {
			*s.styles = append(*s.styles, declarations...)
		}

		output.WriteString(` style="` + html.EscapeString(styleAttribute(declarations)) + `"`)
	}

	output.WriteString(">")

	if remote {
		// Keep the warning close to image handling so callers do not confuse a
		// retained HTTPS reference with a locally embedded attachment.
		*warnings = append(*warnings, remoteImageWarning)
	}

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

func boundedHTMLDimension(value string) (string, bool) {
	if value == "" || len(value) > 4 {
		return "", false
	}

	for _, digit := range []byte(value) {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}

	dimension, err := strconv.Atoi(value)
	if err != nil || dimension < 1 {
		return "", false
	}

	return strconv.Itoa(dimension), true
}

func cidReference(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "cid") || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", validationError("invalid cid image reference")
	}

	value = parsed.Opaque
	if value == "" {
		value = strings.TrimPrefix(parsed.Path, "/")
	}

	contentID, err := url.PathUnescape(value)
	if err != nil {
		return "", validationError("invalid cid image reference")
	}

	return contentID, nil
}

func safeHTTPSImageURL(value string) bool {
	if strings.ContainsAny(value, "\r\n\x00\\") {
		return false
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil {
		return false
	}

	return true
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
	safe, _, err := sanitizeHTML(value, nil, nil)
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
		case "img":
			if alt := attribute(node, "alt"); strings.TrimSpace(alt) != "" {
				output.WriteString("[Image: " + alt + "]")
			} else {
				output.WriteString("[Image]")
			}

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
