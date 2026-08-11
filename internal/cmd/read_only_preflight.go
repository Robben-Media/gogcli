package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/input"
)

// declaredWriteOperations is deliberately explicit: unknown mutations remain
// fail-closed at the transport layer while --readonly is active.
var declaredWriteOperations = map[string]string{
	"gmail:send":               "send Gmail message",
	"config:set":               "change CLI configuration",
	"config:unset":             "change CLI configuration",
	"policy:create":            "change CLI policy",
	"policy:delete":            "change CLI policy",
	"policy:exceptions.create": "persist readonly write exception",
	"policy:exceptions.revoke": "revoke readonly write exception",
	"auth:credentials.set":     "store OAuth client credentials",
	"auth:tokens.import":       "import refresh token",
	"auth:service.account.set": "store service account credentials",
	"auth:keep":                "store Keep service account credentials",
}

// preflightDeclaredWrite grants one declared operation after confirmation. It
// runs after command-policy enforcement, so a policy denial is always stricter.
func preflightDeclaredWrite(kctx *kong.Context, flags *RootFlags) (string, error) {
	if flags == nil || !flags.ReadOnly {
		return "", nil
	}
	action := commandActionID(kctx)
	description, ok := declaredWriteOperations[action]
	if !ok {
		return "", nil
	}
	if allowed, err := persistedWriteExceptionAllows(flags, action); err != nil {
		return "", err
	} else if allowed {
		return action, nil
	}
	if err := confirmReadOnlyWrite(context.Background(), flags, description); err != nil {
		return "", err
	}
	return action, nil
}

func persistedWriteExceptionAllows(flags *RootFlags, action string) (bool, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return false, fmt.Errorf("read config: %w", err)
	}
	account, err := requireAccount(flags)
	if err != nil {
		return false, err
	}
	client, err := resolveClientForEmail(account, flags)
	if err != nil {
		return false, err
	}
	service, operation, _ := strings.Cut(action, ":")
	return config.AllowsWrite(cfg.WriteExceptions, account, client, service, operation, ""), nil
}

func confirmReadOnlyWrite(ctx context.Context, flags *RootFlags, action string) error {
	if flags.NoInput {
		return usagef("refusing to %s in --readonly mode with --no-input", action)
	}
	line, err := input.PromptLine(ctx, fmt.Sprintf("Allow %s in --readonly mode? [y/N]: ", action))
	if err != nil && !errors.Is(err, os.ErrClosed) {
		if errors.Is(err, io.EOF) {
			return &ExitError{Code: 1, Err: errors.New("cancelled")}
		}
		return fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "y" || answer == "yes" {
		return nil
	}
	return &ExitError{Code: 1, Err: errors.New("cancelled")}
}
