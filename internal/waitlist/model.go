package waitlist

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Status for waitlist entries.
const (
	StatusWaiting   = "waiting"
	StatusPromoted  = "promoted"
	StatusCancelled = "cancelled"
)

// Entry represents a waitlist entry for a webinar.
type Entry struct {
	ID         uuid.UUID       `json:"id"`
	WebinarID  uuid.UUID       `json:"webinar_id"`
	Email      string          `json:"email"`
	FullName   string          `json:"full_name"`
	ExtraData  json.RawMessage `json:"extra_data,omitempty"`
	Status     string          `json:"status"`
	PromotedAt *time.Time      `json:"promoted_at,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}
