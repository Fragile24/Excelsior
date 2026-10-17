package session

import (
	"strings"
	"time"

	"excelsior/pkg/llm"
	"excelsior/pkg/util"
)

// Record is the domain model representing a persisted conversation session.
type Record struct {
	ID        string        `json:"id"`
	Title     string        `json:"title,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt,omitempty"`
	Messages  []llm.Message `json:"messages"`
}

// SessionMeta provides lightweight metadata summary for session listings.
type SessionMeta struct {
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	MsgCount  int       `json:"msgCount,omitempty"`
}

// DisplayTitle uses a custom title or the first non-empty user message.
func (r Record) DisplayTitle() string {
	title := strings.TrimSpace(r.Title)
	if title == "" {
		for _, message := range r.Messages {
			if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
				title = strings.TrimSpace(message.Content)
				break
			}
		}
	}
	if title == "" {
		title = "New Chat"
	}
	return util.Truncate(title, 40)
}

// Metadata derives a display title while the record is already loaded.
func (r Record) Metadata() SessionMeta {
	updatedAt := r.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = r.CreatedAt
	}
	return SessionMeta{
		ID: r.ID, Title: r.DisplayTitle(), CreatedAt: r.CreatedAt,
		UpdatedAt: updatedAt, MsgCount: len(r.Messages),
	}
}

// Store defines the storage port for persisting and retrieving chat sessions.
// Implementations must be safe for concurrent use.
type Store interface {
	// Save creates or updates the session record.
	Save(rec Record) error

	// Load retrieves a session record by ID. Returns ErrSessionNotFound if missing.
	Load(id string) (Record, error)

	// List returns metadata with display titles, ordered by most recently updated.
	List() ([]SessionMeta, error)

	// Delete removes a session record by ID. Delete is idempotent (missing ID is not an error).
	Delete(id string) error
}

// CheckLease must succeed before any persistent read/modify/write operation.
// In-memory and injected stores do not require an OS ownership lease.
func CheckLease(s Store) error {
	if leased, ok := s.(interface{ LeaseError() error }); ok {
		return leased.LeaseError()
	}
	return nil
}
