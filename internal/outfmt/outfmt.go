package outfmt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
)

type Mode struct {
	JSON  bool
	Plain bool
}

type JSONConfig struct {
	ResultsOnly bool
	Select      []string
}

type primaryResult struct {
	output  any
	primary any
}

var (
	errInvalidSelectionPath = errors.New("invalid JSON selection path")
	errPrimaryResultMissing = errors.New("JSON output does not declare a primary result")
	errCannotSelectFields   = errors.New("cannot select fields from JSON")

	jsonConfigState struct {
		sync.RWMutex
		config JSONConfig
	}
)

// PrimaryResult declares the primary value in a JSON response envelope while
// preserving output unchanged when results-only mode is not requested.
func PrimaryResult(output any, primary any) any {
	return primaryResult{output: output, primary: primary}
}

type ParseError struct{ msg string }

func (e *ParseError) Error() string { return e.msg }

func FromFlags(jsonOut bool, plainOut bool) (Mode, error) {
	if jsonOut && plainOut {
		return Mode{}, &ParseError{msg: "invalid output mode (cannot combine --json and --plain)"}
	}

	return Mode{JSON: jsonOut, Plain: plainOut}, nil
}

func FromEnv() Mode {
	return Mode{
		JSON:  envBool("GOG_JSON"),
		Plain: envBool("GOG_PLAIN"),
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

func ConfigureJSON(config JSONConfig) (func(), error) {
	config, err := validateJSONConfig(config)
	if err != nil {
		return nil, err
	}

	jsonConfigState.Lock()
	previous := jsonConfigState.config
	jsonConfigState.config = config
	jsonConfigState.Unlock()

	var once sync.Once

	return func() {
		once.Do(func() {
			jsonConfigState.Lock()
			jsonConfigState.config = previous
			jsonConfigState.Unlock()
		})
	}, nil
}

func WriteJSON(w io.Writer, v any) error {
	jsonConfigState.RLock()
	config := jsonConfigState.config
	config.Select = append([]string(nil), config.Select...)

	jsonConfigState.RUnlock()

	return WriteJSONWithConfig(w, v, config)
}

func WriteJSONWithConfig(w io.Writer, v any, config JSONConfig) error {
	config, err := validateJSONConfig(config)
	if err != nil {
		return err
	}

	value, err := transformJSON(v, config)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	return nil
}

func validateJSONConfig(config JSONConfig) (JSONConfig, error) {
	config.Select = append([]string(nil), config.Select...)
	for i, path := range config.Select {
		path = strings.TrimSpace(path)

		parts := strings.Split(path, ".")
		for _, part := range parts {
			if part == "" {
				return JSONConfig{}, fmt.Errorf("%w %q", errInvalidSelectionPath, path)
			}
		}
		config.Select[i] = path
	}

	return config, nil
}

func transformJSON(v any, config JSONConfig) (any, error) {
	value := v
	if result, ok := v.(primaryResult); ok {
		value = result.output
		if config.ResultsOnly {
			value = result.primary
		}
	} else if config.ResultsOnly {
		return nil, errPrimaryResultMissing
	}

	if len(config.Select) == 0 {
		return value, nil
	}

	return projectJSON(value, config.Select)
}

func projectJSON(value any, paths []string) (any, error) {
	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && reflected.Kind() == reflect.Slice && reflected.IsNil() {
		return []any{}, nil
	}

	normalized, err := normalizeJSON(value)
	if err != nil {
		return nil, err
	}

	switch value := normalized.(type) {
	case map[string]any:
		return projectJSONObject(value, paths), nil
	case []any:
		projected := make([]any, 0, len(value))
		for i, item := range value {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w array element %d (%T)", errCannotSelectFields, i, item)
			}

			projected = append(projected, projectJSONObject(object, paths))
		}

		return projected, nil
	default:
		return nil, fmt.Errorf("%w %T", errCannotSelectFields, normalized)
	}
}

func normalizeJSON(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode JSON for selection: %w", err)
	}

	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("decode JSON for selection: %w", err)
	}

	return normalized, nil
}

func projectJSONObject(object map[string]any, paths []string) map[string]any {
	projected := make(map[string]any)

	for _, path := range paths {
		parts := strings.Split(path, ".")
		if value, ok := lookupJSONPath(object, parts); ok {
			setJSONPath(projected, parts, value)
		}
	}

	return projected
}

func lookupJSONPath(object map[string]any, parts []string) (any, bool) {
	var current any = object
	for _, part := range parts {
		next, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		current, ok = next[part]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

func setJSONPath(object map[string]any, parts []string, value any) {
	current := object
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
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
