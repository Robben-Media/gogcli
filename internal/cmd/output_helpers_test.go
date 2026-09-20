package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestWriteTableRowSanitizesPlainFieldsOnly(t *testing.T) {
	fields := []string{"plain\tfield", "line\nfeed", "carriage\rreturn", "windows\r\nline"}

	var plain bytes.Buffer
	plainCtx := outfmt.WithMode(context.Background(), outfmt.Mode{Plain: true})
	writeTableRow(plainCtx, &plain, fields)
	if got, want := plain.String(), "plain field\tline feed\tcarriage return\twindows line\n"; got != want {
		t.Fatalf("plain row = %q, want %q", got, want)
	}

	var human bytes.Buffer
	writeTableRow(context.Background(), &human, fields)
	if got, want := human.String(), "plain\tfield\tline\nfeed\tcarriage\rreturn\twindows\r\nline\n"; got != want {
		t.Fatalf("human row = %q, want %q", got, want)
	}
}
