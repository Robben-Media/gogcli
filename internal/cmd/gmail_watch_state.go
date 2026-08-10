package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/steipete/gogcli/internal/config"
)

const unknownWatchAccount = "unknown"

type gmailWatchStore struct {
	path  string
	mu    sync.Mutex
	state gmailWatchState
}

// watchTempFile is the temp-file surface used by Save for injectable failure tests.
type watchTempFile interface {
	Name() string
	Write(p []byte) (int, error)
	Close() error
	Chmod(mode os.FileMode) error
}

var (
	watchCreateTemp = func(dir, pattern string) (watchTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}
	watchReplace = replaceFile
	watchRemove  = os.Remove
	watchLink    = os.Link
)

func gmailWatchStatePath(account string) (string, error) {
	dir, err := config.EnsureGmailWatchDir()
	if err != nil {
		return "", err
	}
	name := sanitizeAccountForPath(account)
	return filepath.Join(dir, name+".json"), nil
}

func sanitizeAccountForPath(account string) string {
	clean := normalizeWatchAccount(account)
	if clean == "" {
		return unknownWatchAccount
	}
	digest := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(digest[:])
}

func normalizeWatchAccount(account string) string {
	return strings.TrimSpace(strings.ToLower(account))
}

func legacySanitizeAccountForPath(account string) string {
	clean := normalizeWatchAccount(account)
	if clean == "" {
		return unknownWatchAccount
	}
	var b strings.Builder
	b.Grow(len(clean))
	for _, r := range clean {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == '@':
			b.WriteRune('_')
		case r > unicode.MaxASCII:
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func removeMatchingLegacyGmailWatchState(account, currentPath string) {
	legacyPath := filepath.Join(filepath.Dir(currentPath), legacySanitizeAccountForPath(account)+".json")
	if legacyPath == currentPath {
		return
	}
	data, err := os.ReadFile(legacyPath) //nolint:gosec // derived from the configured watch directory and sanitized account
	if err != nil {
		return
	}
	var state gmailWatchState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}
	if normalizeWatchAccount(state.Account) != normalizeWatchAccount(account) {
		return
	}
	_ = watchRemove(legacyPath)
}

func newGmailWatchStore(account string) (*gmailWatchStore, error) {
	path, err := gmailWatchStatePath(account)
	if err != nil {
		return nil, err
	}
	return &gmailWatchStore{path: path}, nil
}

func loadGmailWatchStore(account string) (*gmailWatchStore, error) {
	store, err := newGmailWatchStore(account)
	if err != nil {
		return nil, err
	}
	data, readErr := os.ReadFile(store.path)
	if errors.Is(readErr, os.ErrNotExist) {
		legacyPath := filepath.Join(filepath.Dir(store.path), legacySanitizeAccountForPath(account)+".json")
		data, readErr = os.ReadFile(legacyPath) //nolint:gosec // derived from the configured watch directory and sanitized account
		if errors.Is(readErr, os.ErrNotExist) {
			return nil, errors.New("watch state not found; run gmail watch start")
		}
		if readErr != nil {
			return nil, readErr
		}
		if unmarshalErr := json.Unmarshal(data, &store.state); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		if normalizeWatchAccount(store.state.Account) != normalizeWatchAccount(account) {
			return nil, errors.New("watch state not found; run gmail watch start")
		}
		if migrateErr := migrateGmailWatchState(legacyPath, store.path, data); migrateErr != nil {
			return nil, migrateErr
		}
		return store, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, err
	}
	return store, nil
}

func migrateGmailWatchState(oldPath, newPath string, data []byte) error {
	tmp, err := watchCreateTemp(filepath.Dir(newPath), "gmail-watch-migrate-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = watchRemove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := watchLink(tmpName, newPath); err != nil {
		return err
	}
	_ = watchRemove(oldPath)
	return nil
}

func (s *gmailWatchStore) Get() gmailWatchState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneGmailWatchState(s.state)
}

func cloneGmailWatchState(state gmailWatchState) gmailWatchState {
	state.Labels = append([]string(nil), state.Labels...)
	if state.Hook != nil {
		hook := *state.Hook
		state.Hook = &hook
	}
	return state
}

func (s *gmailWatchStore) Update(fn func(*gmailWatchState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneGmailWatchState(s.state)
	if err := fn(&next); err != nil {
		return err
	}
	if err := s.saveState(next); err != nil {
		return err
	}
	s.state = next
	return nil
}

func (s *gmailWatchStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveState(s.state)
}

// saveState persists state atomically. Caller must hold s.mu when coordinating
// with in-memory updates; Save holds the lock, Update holds it around save+publish.
func (s *gmailWatchStore) saveState(state gmailWatchState) error {
	if s.path == "" {
		return errors.New("missing watch state path")
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	dir := filepath.Dir(s.path)
	tmp, err := watchCreateTemp(dir, "gmail-watch-*.tmp")
	if err != nil {
		return fmt.Errorf("create watch state temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = watchRemove(tmpName)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod watch state temp: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write watch state temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close watch state temp: %w", err)
	}
	if err := watchReplace(tmpName, s.path); err != nil {
		return fmt.Errorf("replace watch state: %w", err)
	}
	cleanup = false
	return nil
}

func (s *gmailWatchStore) StartHistoryID(pushHistory string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pushID, pushOK, pushErr := parseHistoryIDOptional(pushHistory)

	// If no stored state, use push historyId
	if s.state.HistoryID == "" {
		if !pushOK {
			if pushErr != nil {
				return 0, pushErr
			}
			return 0, nil
		}
		if pushErr != nil {
			return 0, pushErr
		}
		next := s.state
		next.HistoryID = formatHistoryID(pushID)
		next.UpdatedAtMs = time.Now().UnixMilli()
		if err := s.saveState(next); err != nil {
			return 0, err
		}
		s.state = next
		return pushID, nil
	}

	storedID, storedOK, err := parseHistoryIDOptional(s.state.HistoryID)
	if err != nil {
		return 0, err
	}
	if !storedOK {
		return 0, nil
	}
	if pushErr != nil {
		return storedID, nil
	}
	if !pushOK {
		return storedID, nil
	}
	if pushID <= storedID {
		return 0, nil
	}

	return storedID, nil
}

func parseHistoryIDOptional(raw string) (uint64, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false, nil
	}
	id, err := parseHistoryID(trimmed)
	if err != nil {
		return 0, true, err
	}
	return id, true, nil
}

func compareHistoryIDs(storedRaw, candidateRaw string) (storedID, candidateID uint64, storedOK, candidateOK bool, err error) {
	storedID, storedOK, err = parseHistoryIDOptional(storedRaw)
	if err != nil {
		return 0, 0, false, false, err
	}
	candidateID, candidateOK, err = parseHistoryIDOptional(candidateRaw)
	if err != nil {
		return storedID, 0, storedOK, true, err
	}
	return storedID, candidateID, storedOK, candidateOK, nil
}

func shouldUpdateHistoryID(currentRaw, candidateRaw string) (bool, error) {
	currentID, candidateID, currentOK, candidateOK, err := compareHistoryIDs(currentRaw, candidateRaw)
	if err != nil {
		return false, err
	}
	if !candidateOK {
		return false, nil
	}
	if !currentOK {
		return true, nil
	}
	return candidateID >= currentID, nil
}

func isStaleHistoryID(currentRaw, candidateRaw string) (bool, error) {
	currentID, candidateID, currentOK, candidateOK, err := compareHistoryIDs(currentRaw, candidateRaw)
	if err != nil {
		return false, err
	}
	if !currentOK || !candidateOK {
		return false, nil
	}
	return candidateID <= currentID, nil
}

func parseHistoryID(raw string) (uint64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("historyId is required")
	}
	id, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid historyId %q", trimmed)
	}
	return id, nil
}

func formatHistoryID(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}
