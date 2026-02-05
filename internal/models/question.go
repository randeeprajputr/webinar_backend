package models

import (
	"time"

	"github.com/google/uuid"
)

// Question represents an audience question in a webinar.
type Question struct {
	ID        uuid.UUID `json:"id"`
	WebinarID uuid.UUID `json:"webinar_id"`
	UserID    uuid.UUID `json:"user_id"`
	Content   string    `json:"content"`
	Approved  bool      `json:"approved"`
	Answered  bool      `json:"answered"`
	Votes     int       `json:"votes"`
	CreatedAt time.Time `json:"created_at"`
}
