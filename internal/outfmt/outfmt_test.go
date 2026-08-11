package outfmt

import (
	"bytes"
	"context"
	"testing"
)

func TestFromFlags(t *testing.T) {
	if _, err := FromFlags(true, true); err == nil {
		t.Fatalf("expected error when combining --json and --plain")
	}

	got, err := FromFlags(true, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !got.JSON || got.Plain {
		t.Fatalf("unexpected mode: %#v", got)
	}
}

func TestContextMode(t *testing.T) {
	ctx := context.Background()

	if IsJSON(ctx) || IsPlain(ctx) {
		t.Fatalf("expected default text")
	}
	ctx = WithMode(ctx, Mode{JSON: true})

	if !IsJSON(ctx) || IsPlain(ctx) {
		t.Fatalf("expected json-only")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(context.Background(), &buf, map[string]any{"ok": true}); err != nil {
		t.Fatalf("err: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatalf("expected output")
	}
}

func TestFromEnvAndParseError(t *testing.T) {
	t.Setenv("GOG_JSON", "yes")
	t.Setenv("GOG_PLAIN", "0")
	t.Setenv("GOG_WRAP_UNTRUSTED", "")
	mode := FromEnv()

	if !mode.JSON || mode.Plain || mode.WrapUntrusted {
		t.Fatalf("unexpected env mode: %#v", mode)
	}

	t.Setenv("GOG_WRAP_UNTRUSTED", "true")

	mode = FromEnv()
	if !mode.WrapUntrusted {
		t.Fatalf("expected wrap from env: %#v", mode)
	}

	if err := (&ParseError{msg: "boom"}).Error(); err != "boom" {
		t.Fatalf("unexpected parse error: %q", err)
	}
}

func TestFromFlagsFull(t *testing.T) {
	got, err := FromFlagsFull(true, false, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !got.JSON || got.Plain || !got.WrapUntrusted {
		t.Fatalf("unexpected mode: %#v", got)
	}

	if _, err := FromFlagsFull(true, true, true); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestIsWrapUntrusted(t *testing.T) {
	ctx := WithMode(context.Background(), Mode{WrapUntrusted: true})
	if !IsWrapUntrusted(ctx) {
		t.Fatalf("expected wrap true")
	}

	if IsWrapUntrusted(context.Background()) {
		t.Fatalf("expected wrap false by default")
	}
}

func TestFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "nope")
	if got := FromContext(ctx); got != (Mode{}) {
		t.Fatalf("expected zero mode, got %#v", got)
	}
}
