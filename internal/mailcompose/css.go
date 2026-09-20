package mailcompose

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var errUnsupportedCSSValue = errors.New("unsupported inline CSS value")

var cssProperties = map[string]func(string) (string, error){
	"background-color": parseCSSColor,
	"border":           parseCSSBorder,
	"border-bottom":    parseCSSBorder,
	"border-color":     parseCSSColor,
	"border-left":      parseCSSBorder,
	"border-radius":    parseCSSLengthList,
	"border-right":     parseCSSBorder,
	"border-style":     parseCSSBorderStyleList,
	"border-top":       parseCSSBorder,
	"border-width":     parseCSSLengthList,
	"color":            parseCSSColor,
	"font-family":      parseCSSFontFamily,
	"font-size":        parseCSSFontSize,
	"font-style":       parseCSSKeyword("normal", "italic", "oblique"),
	"font-weight":      parseCSSFontWeight,
	"height":           parseCSSBoxLength,
	"letter-spacing":   parseCSSLength,
	"line-height":      parseCSSLineHeight,
	"margin":           parseCSSLengthList,
	"margin-bottom":    parseCSSLength,
	"margin-left":      parseCSSLength,
	"margin-right":     parseCSSLength,
	"margin-top":       parseCSSLength,
	"max-height":       parseCSSLength,
	"max-width":        parseCSSLength,
	"min-height":       parseCSSLength,
	"min-width":        parseCSSLength,
	"padding":          parseCSSLengthList,
	"padding-bottom":   parseCSSLength,
	"padding-left":     parseCSSLength,
	"padding-right":    parseCSSLength,
	"padding-top":      parseCSSLength,
	"text-align":       parseCSSKeyword("center", "justify", "left", "right"),
	"text-decoration":  parseCSSTextDecoration,
	"text-transform":   parseCSSKeyword("capitalize", "lowercase", "none", "uppercase"),
	"vertical-align":   parseCSSVerticalAlign,
	"white-space":      parseCSSKeyword("normal", "nowrap", "pre", "pre-wrap"),
	"width":            parseCSSBoxLength,
}

func parseCSSBoxLength(value string) (string, error) {
	lower := strings.ToLower(value)
	if _, err := parseCSSKeyword("auto", "inherit", "initial", "unset")(lower); err == nil {
		return lower, nil
	}

	return parseCSSLength(value)
}

// parseStyleAttribute parses inline CSS without evaluating it. It returns a
// canonical declaration list that can be embedded safely and hashed. Benign
// unsupported declarations are removed so valid signatures can still compose.
func parseStyleAttribute(value string) ([]StyleDeclaration, []string, error) {
	if len(value) > maxStyleBytes {
		return nil, nil, validationError("inline style is %d bytes, limit is %d", len(value), maxStyleBytes)
	}

	declarations, err := splitCSSDeclarations(value)
	if err != nil {
		return nil, nil, err
	}

	if len(declarations) > maxStyleDeclarations {
		return nil, nil, validationError("inline style has %d declarations, limit is %d", len(declarations), maxStyleDeclarations)
	}

	out := make([]StyleDeclaration, 0, len(declarations))
	var warnings []string

	for _, declaration := range declarations {
		property, rawValue, found := strings.Cut(declaration, ":")
		if !found {
			return nil, nil, validationError("inline style declaration is invalid")
		}

		property = strings.ToLower(strings.TrimSpace(property))
		rawValue = strings.TrimSpace(rawValue)

		if rawValue == "" {
			return nil, nil, validationError("inline style value is required")
		}

		if hasCSSForbiddenSyntax(property, rawValue) {
			return nil, nil, validationError("unsafe inline CSS declaration")
		}

		parse, supported := cssProperties[property]
		if !supported {
			warnings = append(warnings, unsupportedCSSWarning)

			continue
		}

		canonical, err := parse(rawValue)
		if errors.Is(err, errUnsupportedCSSValue) {
			warnings = append(warnings, unsupportedCSSWarning)
			continue
		}

		if err != nil {
			return nil, nil, err
		}

		// Margins, letter spacing and vertical alignment permit signed lengths;
		// dimensions, padding, borders, font size and line height do not.
		if property == "width" || property == "height" || property == "font-size" || property == "line-height" || strings.HasPrefix(property, "min-") || strings.HasPrefix(property, "max-") || strings.HasPrefix(property, "padding") || strings.HasPrefix(property, "border") {
			for _, token := range strings.Fields(canonical) {
				if strings.HasPrefix(token, "-") {
					return nil, nil, validationError("inline CSS dimension must not be negative")
				}
			}
		}
		out = append(out, StyleDeclaration{Property: property, Value: canonical})
	}

	return out, uniqueWarnings(warnings), nil
}

func styleAttribute(styles []StyleDeclaration) string {
	values := make([]string, 0, len(styles))
	for _, style := range styles {
		values = append(values, style.Property+":"+style.Value)
	}

	return strings.Join(values, ";")
}

func splitCSSDeclarations(value string) ([]string, error) {
	parts, err := splitCSSTopLevel(value, ';')
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}

	if len(out) == 0 {
		return nil, validationError("inline style contains no declarations")
	}

	return out, nil
}

func splitCSSTopLevel(value string, separator byte) ([]string, error) {
	var parts []string
	var current strings.Builder
	var quoted byte
	depth := 0

	for index := 0; index < len(value); index++ {
		char := value[index]
		switch {
		case quoted != 0:
			if char == quoted {
				quoted = 0
			}
		case char == '"' || char == '\'':
			quoted = char
		case char == '(':
			depth++
		case char == ')':
			depth--
			if depth < 0 {
				return nil, validationError("inline style contains unbalanced parentheses")
			}
		case char == separator && depth == 0:
			parts = append(parts, current.String())
			current.Reset()

			continue
		}

		current.WriteByte(char)
	}

	if quoted != 0 || depth != 0 {
		return nil, validationError("inline style contains an unterminated string or function")
	}

	parts = append(parts, current.String())

	return parts, nil
}

func hasCSSForbiddenSyntax(property, value string) bool {
	lower := strings.ToLower(value)
	forbidden := []string{"\\", "/*", "url(", "image-set(", "expression(", "javascript:", "data:", "<", ">"}

	for _, item := range forbidden {
		if strings.Contains(lower, item) {
			return true
		}
	}

	return property == "behavior" || property == "-moz-binding" || property == "" || strings.ContainsAny(property, " \t\r\n\\/*") || strings.ContainsAny(value, "\r\n\x00")
}

func cssValueTokens(value string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	depth := 0

	flush := func() {
		if current.Len() != 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for index := 0; index < len(value); index++ {
		char := value[index]
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, validationError("inline CSS contains unbalanced parentheses")
			}
		case ' ', '\t':
			if depth == 0 {
				flush()
				continue
			}
		}

		current.WriteByte(char)
	}

	if depth != 0 {
		return nil, validationError("inline CSS contains unbalanced parentheses")
	}

	flush()

	if len(tokens) == 0 {
		return nil, validationError("inline CSS value is required")
	}

	return tokens, nil
}

func parseCSSKeyword(values ...string) func(string) (string, error) {
	return func(value string) (string, error) {
		lower := strings.ToLower(value)
		for _, candidate := range values {
			if lower == candidate {
				return lower, nil
			}
		}

		return "", errUnsupportedCSSValue
	}
}

func parseCSSColor(value string) (string, error) {
	if len(value) > 128 {
		return "", validationError("inline CSS color is too large")
	}

	lower := strings.ToLower(value)
	if _, err := parseCSSKeyword("inherit", "initial", "unset")(lower); err == nil {
		return lower, nil
	}

	if _, err := parseCSSKeyword("aqua", "black", "blue", "fuchsia", "gray", "green", "lime", "maroon", "navy", "olive", "purple", "red", "silver", "teal", "white", "yellow", "transparent", "currentcolor")(lower); err == nil {
		return lower, nil
	}

	if strings.HasPrefix(lower, "#") {
		digits := lower[1:]
		if len(digits) != 3 && len(digits) != 4 && len(digits) != 6 && len(digits) != 8 {
			return "", validationError("inline CSS hexadecimal color is invalid")
		}

		for _, digit := range digits {
			if !strings.ContainsRune("0123456789abcdef", digit) {
				return "", validationError("inline CSS hexadecimal color is invalid")
			}
		}

		if len(digits) == 3 || len(digits) == 4 {
			expanded := make([]byte, 0, len(digits)*2)
			for _, digit := range []byte(digits) {
				expanded = append(expanded, digit, digit)
			}
			digits = string(expanded)
		}

		return "#" + digits, nil
	}

	function, arguments, found := strings.Cut(lower, "(")
	if !found || !strings.HasSuffix(arguments, ")") {
		return "", errUnsupportedCSSValue
	}

	argumentList, err := splitCSSTopLevel(strings.TrimSuffix(arguments, ")"), ',')
	if err != nil {
		return "", err
	}

	expected := 3
	if function == "rgba" {
		expected = 4
	}

	if (function != "rgb" && function != "rgba") || len(argumentList) != expected {
		return "", errUnsupportedCSSValue
	}

	channels := make([]int, 0, 3)

	for _, argument := range argumentList[:3] {
		var channel int

		channel, err = cssColorChannel(argument)
		if err != nil {
			return "", err
		}

		channels = append(channels, channel)
	}

	if expected == 3 {
		return "rgb(" + strconv.Itoa(channels[0]) + ", " + strconv.Itoa(channels[1]) + ", " + strconv.Itoa(channels[2]) + ")", nil
	}

	alpha, err := parseCSSFiniteNumber(strings.TrimSpace(argumentList[3]))
	if err != nil || alpha < 0 || alpha > 1 {
		return "", validationError("inline CSS color alpha is invalid")
	}

	return "rgba(" + strconv.Itoa(channels[0]) + ", " + strconv.Itoa(channels[1]) + ", " + strconv.Itoa(channels[2]) + ", " + strconv.FormatFloat(alpha, 'f', -1, 64) + ")", nil
}

func cssColorChannel(value string) (int, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		number, err := parseCSSFiniteNumber(strings.TrimSuffix(value, "%"))
		if err != nil || number < 0 || number > 100 {
			return 0, validationError("inline CSS color percentage is invalid")
		}

		return int(number*255/100 + 0.5), nil
	}

	number, err := parseCSSFiniteNumber(value)
	if err != nil || number < 0 || number > 255 {
		return 0, validationError("inline CSS color channel is invalid")
	}

	return int(number + 0.5), nil
}

func parseCSSLength(value string) (string, error) {
	if len(value) > 32 {
		return "", validationError("inline CSS length is too large")
	}

	lower := strings.ToLower(value)
	if lower == "0" {
		return lower, nil
	}

	if strings.HasSuffix(lower, "%") {
		return parseCSSNumberSuffix(lower[:len(lower)-1], "%", 1000)
	}

	for _, unit := range []string{"em", "pt", "px"} {
		if strings.HasSuffix(lower, unit) {
			return parseCSSNumberSuffix(lower[:len(lower)-len(unit)], unit, 10000)
		}
	}

	return "", errUnsupportedCSSValue
}

func parseCSSNumberSuffix(number, suffix string, maximum float64) (string, error) {
	value, err := parseCSSFiniteNumber(number)
	if err != nil || value < -maximum || value > maximum {
		return "", validationError("inline CSS length is invalid or too large")
	}

	return strconv.FormatFloat(value, 'f', -1, 64) + suffix, nil
}

func parseCSSLengthList(value string) (string, error) {
	tokens, err := cssValueTokens(value)
	if err != nil {
		return "", err
	}

	if len(tokens) > 4 {
		return "", validationError("inline CSS length list has too many values")
	}

	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parsed, err := parseCSSLength(token)
		if err != nil {
			return "", err
		}

		values = append(values, parsed)
	}

	return strings.Join(values, " "), nil
}

func parseCSSFontWeight(value string) (string, error) {
	lower := strings.ToLower(value)
	if lower == "bold" || lower == "normal" {
		return lower, nil
	}

	number, err := strconv.Atoi(lower)
	if err != nil || number < 100 || number > 900 || number%100 != 0 {
		return "", validationError("inline CSS font weight is invalid")
	}

	return lower, nil
}

func parseCSSLineHeight(value string) (string, error) {
	lower := strings.ToLower(value)
	if _, err := parseCSSKeyword("normal", "inherit", "initial", "unset")(lower); err == nil {
		return lower, nil
	}

	number, err := strconv.ParseFloat(value, 64)
	if err == nil {
		if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > 10 {
			return "", validationError("inline CSS line height is invalid")
		}

		return strconv.FormatFloat(number, 'f', -1, 64), nil
	}

	return parseCSSLength(value)
}

func parseCSSFontSize(value string) (string, error) {
	lower := strings.ToLower(value)
	if _, err := parseCSSKeyword(
		"inherit", "initial", "unset", "xx-small", "x-small", "small", "medium",
		"large", "x-large", "xx-large", "larger", "smaller",
	)(lower); err == nil {
		return lower, nil
	}

	return parseCSSLength(value)
}

func parseCSSFiniteNumber(value string) (float64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, validationError("inline CSS number must be finite")
	}

	return number, nil
}

func parseCSSVerticalAlign(value string) (string, error) {
	if _, err := parseCSSKeyword("baseline", "bottom", "middle", "top")(strings.ToLower(value)); err == nil {
		return strings.ToLower(value), nil
	}

	return parseCSSLength(value)
}

func parseCSSFontFamily(value string) (string, error) {
	families, err := splitCSSTopLevel(value, ',')
	if err != nil {
		return "", err
	}

	if len(families) == 0 || len(families) > 8 {
		return "", validationError("inline CSS font family list is invalid")
	}

	values := make([]string, 0, len(families))
	for _, family := range families {
		family = strings.TrimSpace(family)
		if family == "" || len(family) > 128 || strings.ContainsAny(family, "\\()<>{}[];") {
			return "", validationError("inline CSS font family is invalid")
		}

		if (strings.HasPrefix(family, "\"") && strings.HasSuffix(family, "\"")) || (strings.HasPrefix(family, "'") && strings.HasSuffix(family, "'")) {
			if len(family) < 3 || strings.ContainsAny(family[1:len(family)-1], "\"'") {
				return "", validationError("inline CSS font family is invalid")
			}
		}

		values = append(values, family)
	}

	return strings.Join(values, ", "), nil
}

func parseCSSTextDecoration(value string) (string, error) {
	tokens, err := cssValueTokens(value)
	if err != nil {
		return "", err
	}

	allowed := map[string]struct{}{"line-through": {}, "none": {}, "overline": {}, "underline": {}}
	seen := make(map[string]struct{}, len(tokens))
	values := make([]string, 0, len(tokens))

	for _, token := range tokens {
		token = strings.ToLower(token)
		if _, ok := allowed[token]; !ok {
			return "", errUnsupportedCSSValue
		}

		if _, ok := seen[token]; ok {
			return "", validationError("inline CSS text decoration contains duplicate values")
		}

		seen[token] = struct{}{}
		values = append(values, token)
	}

	if len(values) > 3 || (len(values) > 1 && containsString(values, "none")) {
		return "", validationError("inline CSS text decoration is invalid")
	}

	return strings.Join(values, " "), nil
}

func parseCSSBorderStyleList(value string) (string, error) {
	tokens, err := cssValueTokens(value)
	if err != nil {
		return "", err
	}

	if len(tokens) > 4 {
		return "", validationError("inline CSS border style list has too many values")
	}

	values := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parsed, err := parseCSSKeyword("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid")(token)
		if err != nil {
			return "", err
		}

		values = append(values, parsed)
	}

	return strings.Join(values, " "), nil
}

func parseCSSBorder(value string) (string, error) {
	tokens, err := cssValueTokens(value)
	if err != nil {
		return "", err
	}

	if len(tokens) < 2 || len(tokens) > 3 {
		return "", validationError("inline CSS border must contain a width, style, and optional color")
	}

	width := ""
	style := ""
	color := ""

	for _, token := range tokens {
		if style == "" {
			if parsed, styleErr := parseCSSKeyword("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid")(token); styleErr == nil {
				style = parsed
				continue
			}
		}

		if width == "" {
			if parsed, lengthErr := parseCSSLength(token); lengthErr == nil {
				width = parsed
				continue
			}
		}

		if color == "" && isCSSColorToken(token) {
			color, err = parseCSSColor(token)
			if err != nil {
				return "", err
			}

			continue
		}

		return "", errUnsupportedCSSValue
	}

	if width == "" {
		width, err = parseCSSLength(tokens[0])
		if err != nil {
			return "", err
		}
	}

	if style == "" {
		return "", validationError("inline CSS border style is required")
	}

	parts := []string{width, style}
	if color != "" {
		parts = append(parts, color)
	}

	return strings.Join(parts, " "), nil
}

func isCSSColorToken(value string) bool {
	_, err := parseCSSColor(value)

	return err == nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
