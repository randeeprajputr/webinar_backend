package models

import (
	"time"

	"github.com/google/uuid"
)

// Poll represents a multiple-choice poll in a webinar.
type Poll struct {
	ID        uuid.UUID `json:"id"`
	WebinarID uuid.UUID `json:"webinar_id"`
	Question  string    `json:"question"`
	OptionA   string    `json:"option_a"`
	OptionB   string    `json:"option_b"`
	OptionC   string    `json:"option_c"`
	OptionD   string    `json:"option_d"`
	Launched  bool      `json:"launched"`
	Closed    bool      `json:"closed"`
	CreatedAt time.Time `json:"created_at"`
}

// PollAnswer represents a user's answer to a poll (A/B/C/D).
type PollAnswer struct {
	PollID   uuid.UUID `json:"poll_id"`
	UserID   uuid.UUID `json:"user_id"`
	Option   string    `json:"option"` // "A", "B", "C", "D"
	AnsweredAt time.Time `json:"answered_at"`
}
