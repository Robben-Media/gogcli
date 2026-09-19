package accountconnect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Registry stores connection records without refresh tokens.
type Registry interface {
	Get(_ context.Context, accountID string) (Record, bool, error)
	List(_ context.Context, principalID string) ([]Record, error)
	Upsert(_ context.Context, record Record) error
	Delete(_ context.Context, accountID string) error
	// Commit uniquely stores rec for (PrincipalID, ClientName, Subject).
	// If a record with that triple exists, its AccountID is reused.
	// existed reports whether that unique record was already present.
	// previous is the pre-commit clone when existed is true.
	// FileRegistry is owned by one process; concurrent processes are unsupported.
	Commit(_ context.Context, rec Record) (committed Record, previous Record, existed bool, err error)
}

// MemoryRegistry is a concurrency-safe in-memory registry for tests.
type MemoryRegistry struct {
	mu      sync.Mutex
	records map[string]Record
}

func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{records: make(map[string]Record)}
}

func (r *MemoryRegistry) Get(_ context.Context, accountID string) (Record, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.records[accountID]
	if !ok {
		return Record{}, false, nil
	}

	return cloneRecord(rec), true, nil
}

func (r *MemoryRegistry) List(_ context.Context, principalID string) ([]Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Record, 0, len(r.records))
	for _, rec := range r.records {
		if principalID != "" && rec.PrincipalID != principalID {
			continue
		}

		out = append(out, cloneRecord(rec))
	}

	return out, nil
}

func (r *MemoryRegistry) Upsert(_ context.Context, record Record) error {
	if record.AccountID == "" {
		return ErrUnknownAccount
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.records[record.AccountID] = cloneRecord(record)

	return nil
}

func (r *MemoryRegistry) Delete(_ context.Context, accountID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.records, accountID)

	return nil
}

func (r *MemoryRegistry) Commit(_ context.Context, rec Record) (Record, Record, bool, error) {
	if rec.PrincipalID == "" || rec.ClientName == "" || rec.Subject == "" {
		return Record{}, Record{}, false, ErrUnverifiedIdentity
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return commitRecords(r.records, rec)
}

func cloneRecord(r Record) Record {
	r.Scopes = append([]string(nil), r.Scopes...)
	return r
}

type fileSnapshot struct {
	Records []Record `json:"records"`
}

// FileRegistry persists records as JSON. Tokens are never written here.
type FileRegistry struct {
	path string
	mu   sync.Mutex
}

func NewFileRegistry(path string) (*FileRegistry, error) {
	if path == "" {
		return nil, ErrEmptyRegistryPath
	}

	return &FileRegistry{path: path}, nil
}

func (r *FileRegistry) Get(_ context.Context, accountID string) (Record, bool, error) {
	records, err := r.load()
	if err != nil {
		return Record{}, false, err
	}

	rec, ok := records[accountID]
	if !ok {
		return Record{}, false, nil
	}

	return cloneRecord(rec), true, nil
}

func (r *FileRegistry) List(_ context.Context, principalID string) ([]Record, error) {
	records, err := r.load()
	if err != nil {
		return nil, err
	}

	out := make([]Record, 0, len(records))
	for _, rec := range records {
		if principalID != "" && rec.PrincipalID != principalID {
			continue
		}

		out = append(out, cloneRecord(rec))
	}

	return out, nil
}

func (r *FileRegistry) Upsert(_ context.Context, record Record) error {
	if record.AccountID == "" {
		return ErrUnknownAccount
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.loadLocked()
	if err != nil {
		return err
	}

	records[record.AccountID] = cloneRecord(record)

	return r.saveLocked(records)
}

func (r *FileRegistry) Delete(_ context.Context, accountID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.loadLocked()
	if err != nil {
		return err
	}

	delete(records, accountID)

	return r.saveLocked(records)
}

func (r *FileRegistry) Commit(_ context.Context, rec Record) (Record, Record, bool, error) {
	if rec.PrincipalID == "" || rec.ClientName == "" || rec.Subject == "" {
		return Record{}, Record{}, false, ErrUnverifiedIdentity
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.loadLocked()
	if err != nil {
		return Record{}, Record{}, false, err
	}

	committed, previous, existed, err := commitRecords(records, rec)
	if err != nil {
		return Record{}, Record{}, false, err
	}

	if err := r.saveLocked(records); err != nil {
		return Record{}, Record{}, false, err
	}

	return committed, previous, existed, nil
}

func commitRecords(records map[string]Record, rec Record) (Record, Record, bool, error) {
	rec = cloneRecord(rec)

	var (
		winner   string
		previous Record
		existed  bool
	)

	for id, existing := range records {
		if existing.ClientName != rec.ClientName {
			continue
		}

		sameSubject := existing.Subject == rec.Subject

		sameEmail := existing.Email != "" && rec.Email != "" && existing.Email == rec.Email
		if !sameSubject && !sameEmail {
			continue
		}

		if existing.PrincipalID != rec.PrincipalID {
			return Record{}, Record{}, false, ErrAccountOwned
		}

		if sameSubject || winner == "" {
			winner = id
			previous = cloneRecord(existing)
			existed = true
		}
	}

	if existed {
		rec.AccountID = winner
		rec.Generation = previous.Generation + 1
		records[winner] = cloneRecord(rec)

		return cloneRecord(rec), previous, true, nil
	}

	if rec.AccountID == "" {
		return Record{}, Record{}, false, ErrUnknownAccount
	}

	if rec.Generation == 0 {
		rec.Generation = 1
	}

	records[rec.AccountID] = cloneRecord(rec)

	return cloneRecord(rec), Record{}, false, nil
}

func (r *FileRegistry) load() (map[string]Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.loadLocked()
}

func (r *FileRegistry) loadLocked() (map[string]Record, error) {
	data, err := os.ReadFile(r.path) //nolint:gosec // G703: registry path is process configuration
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Record{}, nil
		}

		return nil, fmt.Errorf("read account registry: %w", err)
	}

	var snap fileSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode account registry: %w", err)
	}

	out := make(map[string]Record, len(snap.Records))
	for _, rec := range snap.Records {
		out[rec.AccountID] = cloneRecord(rec)
	}

	return out, nil
}

func (r *FileRegistry) saveLocked(records map[string]Record) error {
	snap := fileSnapshot{Records: make([]Record, 0, len(records))}
	for _, rec := range records {
		snap.Records = append(snap.Records, cloneRecord(rec))
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("encode account registry: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil { //nolint:gosec // G703: registry path is process configuration
		return fmt.Errorf("ensure account registry dir: %w", err)
	}

	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil { //nolint:gosec // G703: registry path is process configuration
		return fmt.Errorf("write account registry: %w", err)
	}

	if err := os.Rename(tmp, r.path); err != nil { //nolint:gosec // G703: registry path is process configuration
		return fmt.Errorf("commit account registry: %w", err)
	}

	return nil
}
