package feedback

import (
	"time"

	"github.com/google/uuid"
)

// Entry represents attendee feedback for a webinar.
type Entry struct {
	ID                  uuid.UUID `json:"id"`
	WebinarID           uuid.UUID `json:"webinar_id"`
	RegistrationID      uuid.UUID `json:"registration_id"`
	Rating              int       `json:"rating"` // 1-5
	SpeakerEffectiveness *int     `json:"speaker_effectiveness,omitempty"` // 1-5
	ContentUsefulness   *int     `json:"content_usefulness,omitempty"`   // 1-5
	Suggestions         string   `json:"suggestions,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}
