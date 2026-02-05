package models

import (
	"time"

	"github.com/google/uuid"
)

// UserSessionLog tracks join/leave and watch duration per attendee.
type UserSessionLog struct {
	ID             uuid.UUID  `json:"id"`
	WebinarID      uuid.UUID  `json:"webinar_id"`
	RegistrationID *uuid.UUID `json:"registration_id,omitempty"`
	UserID         *uuid.UUID `json:"user_id,omitempty"`
	JoinedAt       time.Time  `json:"joined_at"`
	LeftAt         *time.Time `json:"left_at,omitempty"`
	WatchSeconds   int64      `json:"watch_seconds"`
	CreatedAt      time.Time  `json:"created_at"`
}

// EngagementMetrics holds aggregated analytics per webinar/session.
type EngagementMetrics struct {
	ID                      uuid.UUID  `json:"id"`
	WebinarID               uuid.UUID  `json:"webinar_id"`
	StreamSessionID         *uuid.UUID `json:"stream_session_id,omitempty"`
	TotalRegistrations      int        `json:"total_registrations"`
	TotalAttended           int        `json:"total_attended"`
	TotalNoShow             int        `json:"total_no_show"`
	PeakLiveViewers         int        `json:"peak_live_viewers"`
	AvgWatchSeconds         int64      `json:"avg_watch_seconds"`
	PollParticipationCount  int        `json:"poll_participation_count"`
	PollParticipationPercent float64   `json:"poll_participation_percent"`
	QuestionsCount          int        `json:"questions_count"`
	RecordedAt              time.Time  `json:"recorded_at"`
	CreatedAt               time.Time  `json:"created_at"`
}
