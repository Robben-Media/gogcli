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
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/input"
)

// operationSpec is the single command-level readonly declaration. Unknown
// commands are deliberately not declared: the transport still fails closed.
type readOnlyApprovalContextKey struct{}

func withReadOnlyApproval(ctx context.Context) context.Context {
	return context.WithValue(ctx, readOnlyApprovalContextKey{}, true)
}

func hasReadOnlyApproval(ctx context.Context) bool {
	approved, _ := ctx.Value(readOnlyApprovalContextKey{}).(bool)
	return approved
}

type operationSpec struct {
	Service   string
	Operation string
	Target    string
	Summary   string
}

// mutationWords classify every current command family centrally. The action ID
// comes from Kong metadata, so aliases and new nested commands use one path.
var mutationWords = map[string]struct{}{
	"add": {}, "append": {}, "apply": {}, "clear": {}, "copy": {}, "create": {}, "delete": {}, "empty": {}, "format": {}, "import": {}, "insert": {}, "mkdir": {}, "modify": {}, "move": {}, "patch": {}, "publish": {}, "remove": {}, "rename": {}, "replace": {}, "respond": {}, "revoke": {}, "send": {}, "set": {}, "share": {}, "start": {}, "stop": {}, "trash": {}, "untrash": {}, "update": {}, "upload": {}, "watch": {}, "write": {},
}

var explicitMutations = map[string]struct{}{
	"auth:add": {}, "auth:credentials.set": {}, "auth:keyring.set": {}, "auth:service.account.set": {}, "auth:service.account.unset": {}, "auth:keep": {}, "auth:tokens.import": {}, "auth:tokens.delete": {}, "auth:remove": {}, "auth:alias": {}, "config:set": {}, "config:unset": {}, "policy:create": {}, "policy:delete": {}, "policy:exceptions.create": {}, "policy:exceptions.revoke": {}, "gmail:settings.autoforward": {}, "gmail:settings.vacation": {}, "gmail:settings.imap": {}, "gmail:settings.pop": {}, "gmail:settings.language": {}, "calendar:propose.time": {},
}

func declaredOperation(kctx *kong.Context) (operationSpec, bool) {
	action := commandActionID(kctx)
	if action == "" {
		return operationSpec{}, false
	}
	service, operation, _ := strings.Cut(action, ":")
	if _, ok := explicitMutations[action]; !ok {
		matched := false
		for _, part := range strings.Split(operation, ".") {
			if _, ok := mutationWords[part]; ok {
				matched = true
				break
			}
		}
		if !matched {
			return operationSpec{}, false
		}
	}

	return operationSpec{Service: service, Operation: operation, Summary: fmt.Sprintf("%s %s", service, strings.ReplaceAll(operation, ".", " "))}, true
}

// preflightDeclaredWrite runs after policy enforcement. It accepts persisted
// matches, explicit service exemptions, or a typed one-shot/persistent grant.
func preflightDeclaredWrite(kctx *kong.Context, flags *RootFlags) ([]googleapi.WriteGrant, error) {
	if flags == nil || !flags.ReadOnly {
		return nil, nil
	}

	spec, declared := declaredOperation(kctx)
	if !declared {
		return nil, nil
	}

	if readOnlyServiceExempt(flags.ReadOnlyExcept, spec.Service) {
		return []googleapi.WriteGrant{{Service: spec.Service}}, nil
	}

	if allowed, err := persistedWriteExceptionAllows(flags, spec); err != nil {
		return nil, err
	} else if allowed {
		return []googleapi.WriteGrant{{Service: spec.Service, Operation: spec.Operation, Target: spec.Target}}, nil
	}

	grant, err := confirmReadOnlyWrite(context.Background(), flags, spec)
	if err != nil {
		return nil, err
	}

	return []googleapi.WriteGrant{grant}, nil
}

func readOnlyServiceExempt(raw, service string) bool {
	for _, candidate := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), service) {
			return true
		}
	}
	return false
}

func persistedWriteExceptionAllows(flags *RootFlags, spec operationSpec) (bool, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return false, fmt.Errorf("read config: %w", err)
	}

	account, client := "", ""
	if flags != nil && strings.TrimSpace(flags.Account) != "" {
		account, err = requireAccount(flags)
		if err != nil {
			return false, err
		}
		client, err = resolveClientForEmail(account, flags)
		if err != nil {
			return false, err
		}
	}

	return config.AllowsWrite(cfg.WriteExceptions, account, client, spec.Service, spec.Operation, spec.Target), nil
}

func confirmReadOnlyWrite(ctx context.Context, flags *RootFlags, spec operationSpec) (googleapi.WriteGrant, error) {
	if flags.NoInput {
		return googleapi.WriteGrant{}, usagef("refusing to %s in --readonly mode with --no-input", spec.Summary)
	}

	line, err := input.PromptLine(ctx, fmt.Sprintf("Readonly: %s. Choose [1] deny, [2] allow once, [3] always: ", spec.Summary))
	if err != nil && !errors.Is(err, os.ErrClosed) {
		if errors.Is(err, io.EOF) {
			return googleapi.WriteGrant{}, &ExitError{Code: 2, Err: errors.New("readonly write denied")}
		}
		return googleapi.WriteGrant{}, fmt.Errorf("read readonly confirmation: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "2", "once", "y", "yes":
		return googleapi.WriteGrant{Service: spec.Service, Operation: spec.Operation, Target: spec.Target}, nil
	case "3", always:
		return persistReadOnlyWrite(ctx, flags, spec)
	default:
		return googleapi.WriteGrant{}, &ExitError{Code: 2, Err: errors.New("readonly write denied")}
	}
}

func persistReadOnlyWrite(ctx context.Context, flags *RootFlags, spec operationSpec) (googleapi.WriteGrant, error) {
	line, err := input.PromptLine(ctx, "Persist scope [1] target, [2] operation, [3] service: ")
	if err != nil && !errors.Is(err, os.ErrClosed) {
		return googleapi.WriteGrant{}, fmt.Errorf("read persisted scope: %w", err)
	}
	grant := googleapi.WriteGrant{Service: spec.Service, Operation: spec.Operation, Target: spec.Target}
	switch strings.TrimSpace(line) {
	case "1", "target":
		if grant.Target == "" {
			return googleapi.WriteGrant{}, usagef("target is not available for %s", spec.Summary)
		}
	case "2", "operation", "":
		grant.Target = ""
	case "3", "service":
		grant.Operation, grant.Target = "", ""
	default:
		return googleapi.WriteGrant{}, &ExitError{Code: 2, Err: errors.New("readonly write denied")}
	}

	identity, err := input.PromptLine(ctx, "Identity scope [1] this account and client, [2] all identities: ")
	if err != nil && !errors.Is(err, os.ErrClosed) {
		return googleapi.WriteGrant{}, fmt.Errorf("read identity scope: %w", err)
	}
	exception := config.WriteException{Name: "readonly-" + strings.ReplaceAll(spec.Service+"-"+spec.Operation, ".", "-") + "-always", Service: grant.Service, Operation: grant.Operation, Target: grant.Target}
	if strings.TrimSpace(identity) != "2" && strings.TrimSpace(identity) != "all" && flags != nil && strings.TrimSpace(flags.Account) != "" {
		account, accountErr := requireAccount(flags)
		if accountErr != nil {
			return googleapi.WriteGrant{}, accountErr
		}
		client, clientErr := resolveClientForEmail(account, flags)
		if clientErr != nil {
			return googleapi.WriteGrant{}, clientErr
		}
		exception.Account, exception.Client = account, client
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		return googleapi.WriteGrant{}, fmt.Errorf("read config: %w", err)
	}
	if err := config.UpsertWriteException(&cfg, exception, true); err != nil {
		return googleapi.WriteGrant{}, err
	}
	if err := config.WriteConfig(cfg); err != nil {
		return googleapi.WriteGrant{}, err
	}

	return grant, nil
}
