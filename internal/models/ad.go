package models

import (
	"time"

	"github.com/google/uuid"
)

// Ad represents an advertisement for rotation in a webinar.
type Ad struct {
	ID        uuid.UUID `json:"id"`
	WebinarID uuid.UUID `json:"webinar_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	ImageURL  string    `json:"image_url,omitempty"`
	LinkURL   string    `json:"link_url,omitempty"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}
