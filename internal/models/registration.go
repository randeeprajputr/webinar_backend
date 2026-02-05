package models

import (
	"time"

	"github.com/google/uuid"
)

// Registration is an attendee registration for a webinar.
type Registration struct {
	ID         uuid.UUID  `json:"id"`
	WebinarID  uuid.UUID  `json:"webinar_id"`
	Email      string     `json:"email"`
	FullName   string     `json:"full_name"`
	AttendedAt *time.Time `json:"attended_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// RegistrationToken is a unique join link token for a registration.
type RegistrationToken struct {
	ID             uuid.UUID  `json:"id"`
	RegistrationID uuid.UUID  `json:"registration_id"`
	Token          string     `json:"token"`
	ExpiresAt      time.Time  `json:"expires_at"`
	UsedAt         *time.Time `json:"used_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}
