package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"excelsior/pkg/llm"
	"excelsior/pkg/util"
)

// ponytail: single regex covers separators + traversal (was Contains checks + second regex + Rel check)
var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{1,63}$`)

func sanitizeID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("session validate: %w", ErrEmptySessionID)
	}
	if !validID.MatchString(id) {
		return "", fmt.Errorf("session validate %q: %w", id, ErrInvalidSessionID)
	}
	return id, nil
}

// DirStore implements [Store] persisting sessions as atomic JSON files on disk.
type DirStore struct {
	Dir string // Directory path e.g. .excelsior/sessions
	mu  sync.Mutex
	// lease state: writes require an owned store lease (see lease.go).
	holds  bool
	closed bool
}

// Compile-time interface check.
var _ Store = (*DirStore)(nil)

// NewDirStore returns a DirStore rooted at dir. It immediately acquires an
// OS-held exclusive lease on the store; a second process gets ErrStoreOwned
// on its first write (see LeaseError to fail fast).
func NewDirStore(dir string) *DirStore {
	s := &DirStore{Dir: dir}
	s.ensureLease()
	return s
}

func (s *DirStore) ensureLease() error {
	if s.closed {
		return fmt.Errorf("session store closed")
	}
	if s.holds {
		return nil
	}
	if strings.TrimSpace(s.Dir) == "" {
		return nil // path() reports ErrStoreDirEmpty on write
	}
	if err := acquireStoreLease(s.Dir); err != nil {
		return err
	}
	s.holds = true
	return nil
}

// LeaseError reports why writes to this store currently fail (nil if owned).
func (s *DirStore) LeaseError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureLease()
}

// Close releases the store lease. The OS also releases it when the process
// exits, so a crash never leaves a stale lock.
func (s *DirStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.holds {
		s.holds = false
		releaseStoreLease(s.Dir)
	}
}

func (s *DirStore) path(id string) (string, error) {
	safe, err := sanitizeID(id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s.Dir) == "" {
		return "", fmt.Errorf("session path %q: %w", id, ErrStoreDirEmpty)
	}
	return filepath.Join(s.Dir, safe+".json"), nil
}

// Save persists a session record atomically to disk.
func (s *DirStore) Save(rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLease(); err != nil {
		return fmt.Errorf("session save %q: %w", rec.ID, err)
	}
	p, err := s.path(rec.ID)
	if err != nil {
		return err
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	rec.UpdatedAt = time.Now().UTC()
	if rec.Messages == nil {
		rec.Messages = []llm.Message{}
	}

	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("session save %q: %w", rec.ID, err)
	}
	b = append(b, '\n')
	if err := util.WriteAtomic(p, b, 0o600); err != nil {
		return fmt.Errorf("session save %q: %w", rec.ID, err)
	}
	slog.Debug("session saved", "id", rec.ID, "title", rec.Title, "messages", len(rec.Messages))
	return nil
}

func decodeRecord(b []byte, id string) (Record, error) {
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil {
		return Record{}, err
	}
	if rec.ID == "" {
		rec.ID = id
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.CreatedAt
	}
	return rec, nil
}

// Load retrieves a session record by ID from disk.
func (s *DirStore) Load(id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLease(); err != nil {
		return Record{}, err
	}

	p, err := s.path(id)
	if err != nil {
		return Record{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, fmt.Errorf("session load %q: %w", id, ErrSessionNotFound)
		}
		return Record{}, fmt.Errorf("session load %q: %w", id, err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return Record{}, fmt.Errorf("session load %q: %w", id, ErrEmptySession)
	}
	rec, err := decodeRecord(b, id)
	if err != nil {
		return Record{}, fmt.Errorf("session load %q: %w", id, ErrCorruptedSession)
	}
	return rec, nil
}

// List returns metadata summaries for all sessions, sorted by UpdatedAt descending.
func (s *DirStore) List() ([]SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLease(); err != nil {
		return nil, err
	}

	if strings.TrimSpace(s.Dir) == "" {
		return nil, fmt.Errorf("session list: %w", ErrStoreDirEmpty)
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("session list: %w", err)
	}
	var metas []SessionMeta
	for _, e := range entries {
		if meta := s.metaForEntry(e); meta != nil {
			metas = append(metas, *meta)
		}
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
	})
	return metas, nil
}

func (s *DirStore) metaForEntry(e os.DirEntry) *SessionMeta {
	if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
		return nil
	}
	id := strings.TrimSuffix(e.Name(), ".json")
	if _, err := sanitizeID(id); err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	rec, err := decodeRecord(b, id)
	if err != nil {
		return nil
	}
	meta := rec.Metadata()
	return &meta
}

// Delete removes the session file for id. Missing files return nil (idempotent).
func (s *DirStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLease(); err != nil {
		return fmt.Errorf("session delete %q: %w", id, err)
	}
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("session delete %q: %w", id, err)
	}
	slog.Info("session deleted", "id", id)
	return nil
}
