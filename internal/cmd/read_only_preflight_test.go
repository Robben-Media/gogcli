package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestPreflightDeclaredWriteForceDoesNotBypassReadOnly(t *testing.T) {
	err := confirmReadOnlyWrite(context.Background(), &RootFlags{ReadOnly: true, Force: true, NoInput: true}, "send Gmail message")
	if err == nil || !strings.Contains(err.Error(), "--no-input") {
		t.Fatalf("force and no-input error = %v, want readonly denial", err)
	}
}

func TestPreflightDeclaredWriteNoInputRefuses(t *testing.T) {
	err := confirmReadOnlyWrite(context.Background(), &RootFlags{ReadOnly: true, NoInput: true}, "send Gmail message")
	if err == nil || !strings.Contains(err.Error(), "--no-input") {
		t.Fatalf("no-input error = %v, want readonly denial", err)
	}
}
