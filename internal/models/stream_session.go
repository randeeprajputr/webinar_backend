package models

import (
	"time"

	"github.com/google/uuid"
)

// StreamSession tracks stream metadata for a webinar.
type StreamSession struct {
	ID                     uuid.UUID  `json:"id"`
	WebinarID              uuid.UUID  `json:"webinar_id"`
	StartedAt              time.Time  `json:"started_at"`
	EndedAt                *time.Time `json:"ended_at,omitempty"`
	PeakViewers            int       `json:"peak_viewers"`
	TotalViewers           int       `json:"total_viewers"`
	TotalWatchTime         int64     `json:"total_watch_time"`
	PollParticipationCount int       `json:"poll_participation_count"`
	QuestionsCount         int       `json:"questions_count"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}
