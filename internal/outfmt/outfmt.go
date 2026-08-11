package outfmt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Mode struct {
	JSON          bool
	Plain         bool
	WrapUntrusted bool
}

type ParseError struct{ msg string }

func (e *ParseError) Error() string { return e.msg }

func FromFlags(jsonOut bool, plainOut bool) (Mode, error) {
	if jsonOut && plainOut {
		return Mode{}, &ParseError{msg: "invalid output mode (cannot combine --json and --plain)"}
	}

	return Mode{JSON: jsonOut, Plain: plainOut}, nil
}

// FromFlagsFull builds Mode from all root output-related flags.
func FromFlagsFull(jsonOut, plainOut, wrapUntrusted bool) (Mode, error) {
	m, err := FromFlags(jsonOut, plainOut)
	if err != nil {
		return Mode{}, err
	}
	m.WrapUntrusted = wrapUntrusted
	return m, nil
}

func FromEnv() Mode {
	return Mode{
		JSON:          envBool("GOG_JSON"),
		Plain:         envBool("GOG_PLAIN"),
		WrapUntrusted: envBool("GOG_WRAP_UNTRUSTED"),
	}
}

type ctxKey struct{}

func WithMode(ctx context.Context, mode Mode) context.Context {
	return context.WithValue(ctx, ctxKey{}, mode)
}

func FromContext(ctx context.Context) Mode {
	if v := ctx.Value(ctxKey{}); v != nil {
		if m, ok := v.(Mode); ok {
			return m
		}
	}

	return Mode{}
}

func IsJSON(ctx context.Context) bool  { return FromContext(ctx).JSON }
func IsPlain(ctx context.Context) bool { return FromContext(ctx).Plain }

// WriteJSON encodes v as indented JSON to w.
// When process encode mode has WrapUntrusted set (via SetEncodeMode), free-text
// fields are wrapped before encoding. Wrapping is a no-op when that flag is off
// (the default). Plain/human output paths never call WriteJSON.
func WriteJSON(w io.Writer, v any) error {
	v = maybeWrapForEncode(v)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	return nil
}

func KeyValuePayload(key string, value any) map[string]any {
	return map[string]any{
		"key":   key,
		"value": value,
	}
}

func KeysPayload(keys []string) map[string]any {
	return map[string]any{
		"keys": keys,
	}
}

func PathPayload(path string) map[string]any {
	return map[string]any{
		"path": path,
	}
}

func envBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
