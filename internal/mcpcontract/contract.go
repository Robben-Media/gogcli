// Package mcpcontract owns the transport-independent native Google contracts.
package mcpcontract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

// Identity is a registry snapshot resolved from trusted caller grants, never tool arguments.
type Identity struct {
	AccountID   string    `json:"account_id"`
	Subject     string    `json:"subject"`
	Email       string    `json:"email"`
	Label       string    `json:"label"`
	PrincipalID string    `json:"principal_id"`
	ClientName  string    `json:"client_name"`
	AuthMode    string    `json:"auth_mode"`
	Scopes      []string  `json:"scopes"`
	Generation  uint64    `json:"generation"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Clone detaches the scope slice before a registry snapshot crosses a boundary.
func (id Identity) Clone() Identity {
	id.Scopes = append([]string(nil), id.Scopes...)
	return id
}

type RetryClass string

const (
	SafeRead           RetryClass = "safe_read"
	ProviderKeyedWrite RetryClass = "provider_keyed_write"
	NonReplayableWrite RetryClass = "non_replayable_write"
)

type CallOptions struct {
	Operation string
	Retry     RetryClass
}

// ClientProvider supplies OAuth-only clients without choosing a default account.
type ClientProvider interface {
	HTTPClient(context.Context, Identity, CallOptions) (*http.Client, error)
}

type Selection struct {
	AccountID string `json:"account_id" jsonschema:"Opaque account ID returned by accounts_list; required for every Google operation"`
}

func (s Selection) GetAccountID() string { return s.AccountID }

type AccountRequest interface{ GetAccountID() string }

type Failure struct {
	SourceID string `json:"source_id"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type Result[T any] struct {
	AccountID       string    `json:"account_id"`
	AccountLabel    string    `json:"account_label"`
	Data            T         `json:"data"`
	NextPageToken   string    `json:"next_page_token,omitempty"`
	Truncated       bool      `json:"truncated"`
	FetchedAt       time.Time `json:"fetched_at"`
	PartialFailures []Failure `json:"partial_failures,omitempty"`
}

// Envelope restricts native outputs to the account/freshness/result contract.
type Envelope interface{ resultEnvelope() }

func (Result[T]) resultEnvelope() {}

func NewResult[T any](id Identity, data T) Result[T] {
	return Result[T]{AccountID: id.AccountID, AccountLabel: id.Label, Data: data, FetchedAt: time.Now().UTC()}
}

type ErrorCategory string

const (
	InvalidInput       ErrorCategory = "invalid_input"
	Forbidden          ErrorCategory = "forbidden_operation"
	AuthRequired       ErrorCategory = "authentication_required"
	InsufficientScope  ErrorCategory = "insufficient_scope"
	NotFound           ErrorCategory = "not_found"
	Conflict           ErrorCategory = "conflict"
	PreconditionFailed ErrorCategory = "precondition_failed"
	BudgetExhausted    ErrorCategory = "budget_exhausted"
	QuotaExhausted     ErrorCategory = "quota_exhausted"
	DeadlineExceeded   ErrorCategory = "deadline_exceeded"
	UpstreamFailure    ErrorCategory = "upstream_failure"
	OutcomeUnknown     ErrorCategory = "outcome_unknown"
)

// Error contains only safe public details; upstream bodies and tokens do not belong here.
type Error struct {
	Category  ErrorCategory `json:"category"`
	Message   string        `json:"message"`
	Retryable bool          `json:"retryable"`
}

func (e *Error) Error() string     { return string(e.Category) + ": " + e.Message }
func Invalid(message string) error { return &Error{Category: InvalidInput, Message: message} }

// Call is created by strict decoding before authorization. Run is invoked only after all Actions pass.
type Call struct {
	AccountID string
	Actions   []string
	Run       func(context.Context, Identity) (any, error)
}
type Operation struct {
	Definition   Definition
	InputSchema  *jsonschema.Schema
	OutputSchema *jsonschema.Schema
	Decode       func(json.RawMessage) (Call, error)
}

// NewOperation derives schemas from the actual Go input/output types. Invalid static schemas panic at startup.
func NewOperation[In AccountRequest, Out Envelope](name string, validate func(In) error, run func(context.Context, Identity, In) (Out, error)) Operation {
	def, ok := Lookup(name)
	if !ok || def.Local {
		panic("unknown Google operation: " + name)
	}

	input, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("input schema %s: %v", name, err))
	}

	output, err := jsonschema.For[Out](nil)
	if err != nil {
		panic(fmt.Sprintf("output schema %s: %v", name, err))
	}

	publicDef, _ := Lookup(name)

	return Operation{Definition: publicDef, InputSchema: input, OutputSchema: output, Decode: func(raw json.RawMessage) (Call, error) {
		var in In
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()

		if err := dec.Decode(&in); err != nil {
			return Call{}, Invalid("arguments must match the tool input schema")
		}

		if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
			return Call{}, Invalid("arguments must contain one JSON object")
		}

		if in.GetAccountID() == "" {
			return Call{}, Invalid("account_id is required")
		}

		if validate != nil {
			if err := validate(in); err != nil {
				var safe *Error
				if errors.As(err, &safe) {
					return Call{}, safe
				}

				return Call{}, Invalid("arguments failed validation")
			}
		}

		actions := append([]string(nil), def.Actions...)

		if name == "analytics_metadata" {
			var selector struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(raw, &selector); err != nil {
				return Call{}, Invalid("kind is required")
			}

			switch selector.Kind {
			case "dimensions":
				actions = []string{"analytics:dimensions"}
			case "metrics":
				actions = []string{"analytics:metrics"}
			case "both":
			default:
				return Call{}, Invalid("kind must be dimensions, metrics, or both")
			}
		}

		return Call{AccountID: in.GetAccountID(), Actions: actions, Run: func(ctx context.Context, id Identity) (any, error) { return run(ctx, id, in) }}, nil
	}}
}

// Principal is supplied by the trusted transport, never by MCP request metadata.
type Principal struct {
	ID string `json:"id"`
}

// Grant is trusted startup configuration. Empty slices grant no access.
type Grant struct {
	PrincipalID string   `json:"principal_id"`
	AccountIDs  []string `json:"account_ids"`
	ClientNames []string `json:"client_names"`
	Operations  []string `json:"operations"`
}
