package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestDeclaredOperationCoversRepresentativeMutations(t *testing.T) {
	for _, args := range [][]string{
		{"gmail", "send", "--to", "x@example.com", "--subject", "x", "--body", "x"},
		{"drive", "delete", "file-id"},
		{"calendar", "delete", "primary", "event-id"},
		{"sheets", "clear", "spreadsheet-id", "Sheet1!A1"},
		{"config", "set", "default_timezone", "UTC"},
	} {
		parser, _, err := newParser("test")
		if err != nil {
			t.Fatal(err)
		}

		kctx, err := parser.Parse(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}

		if _, ok := declaredOperation(kctx); !ok {
			t.Fatalf("%v was not declared as a mutation", args)
		}
	}
}

func TestReadOnlyForceAndNoInputDoNotBypass(t *testing.T) {
	_, err := confirmReadOnlyWrite(context.Background(), &RootFlags{ReadOnly: true, Force: true, NoInput: true}, operationSpec{Service: "gmail", Operation: "send", Summary: "gmail send"})
	if err == nil || !strings.Contains(err.Error(), "--no-input") {
		t.Fatalf("force and no-input error = %v, want readonly denial", err)
	}
}

func TestReadOnlyServiceExemption(t *testing.T) {
	if !readOnlyServiceExempt("drive,calendar", "drive") {
		t.Fatal("drive exemption not recognized")
	}

	if readOnlyServiceExempt("drive", "gmail") {
		t.Fatal("unlisted service exemption recognized")
	}
}
