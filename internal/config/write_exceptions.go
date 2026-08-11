package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	errWriteExceptionMissingScope = errors.New("write exception requires service, operation, or target")
	errWriteExceptionExists       = errors.New("write exception already exists")
	errWriteExceptionNotFound     = errors.New("write exception not found")
)

// WriteException is a persisted readonly-mode grant. Account and Client are
// intentionally part of the default identity scope; empty values are broader
// scopes and must be selected explicitly by the caller.
type WriteException struct {
	Name      string `json:"name"`
	Account   string `json:"account,omitempty"`
	Client    string `json:"client,omitempty"`
	Service   string `json:"service,omitempty"`
	Operation string `json:"operation,omitempty"`
	Target    string `json:"target,omitempty"`
}

func NormalizeWriteException(value WriteException) (WriteException, error) {
	name, err := NormalizePolicyName(value.Name)
	if err != nil {
		return WriteException{}, err
	}

	value.Name = name
	value.Account = strings.ToLower(strings.TrimSpace(value.Account))
	value.Client = strings.TrimSpace(value.Client)

	if value.Client != "" {
		value.Client, err = NormalizeClientNameOrDefault(value.Client)
		if err != nil {
			return WriteException{}, err
		}
	}

	value.Service = strings.ToLower(strings.TrimSpace(value.Service))
	value.Operation = strings.ToLower(strings.TrimSpace(value.Operation))
	value.Target = strings.TrimSpace(value.Target)

	if value.Service == "" && value.Operation == "" && value.Target == "" {
		return WriteException{}, errWriteExceptionMissingScope
	}

	return value, nil
}

func UpsertWriteException(cfg *File, value WriteException, replace bool) error {
	if cfg == nil {
		return errNilConfig
	}

	value, err := NormalizeWriteException(value)
	if err != nil {
		return err
	}

	for i := range cfg.WriteExceptions {
		if cfg.WriteExceptions[i].Name != value.Name {
			continue
		}

		if !replace {
			return fmt.Errorf("%w: %s", errWriteExceptionExists, value.Name)
		}

		cfg.WriteExceptions[i] = value
		sortWriteExceptions(cfg)

		return nil
	}

	cfg.WriteExceptions = append(cfg.WriteExceptions, value)
	sortWriteExceptions(cfg)

	return nil
}

func DeleteWriteException(cfg *File, name string) error {
	if cfg == nil {
		return errNilConfig
	}

	name, err := NormalizePolicyName(name)
	if err != nil {
		return err
	}

	for i := range cfg.WriteExceptions {
		if cfg.WriteExceptions[i].Name == name {
			cfg.WriteExceptions = append(cfg.WriteExceptions[:i], cfg.WriteExceptions[i+1:]...)

			return nil
		}
	}

	return fmt.Errorf("%w: %s", errWriteExceptionNotFound, name)
}

// AllowsWrite returns true only for an exception matching every non-empty scope.
func AllowsWrite(exceptions []WriteException, account, client, service, operation, target string) bool {
	for _, exception := range exceptions {
		if exception.Account != "" && !strings.EqualFold(exception.Account, account) {
			continue
		}

		if exception.Client != "" && !strings.EqualFold(exception.Client, client) {
			continue
		}

		if exception.Service != "" && !strings.EqualFold(exception.Service, service) {
			continue
		}

		if exception.Operation != "" && !strings.EqualFold(exception.Operation, operation) {
			continue
		}

		if exception.Target != "" && exception.Target != target {
			continue
		}

		return true
	}

	return false
}

func sortWriteExceptions(cfg *File) {
	slices.SortFunc(cfg.WriteExceptions, func(a, b WriteException) int { return strings.Compare(a.Name, b.Name) })
}
