package models

import (
	"time"

	"github.com/google/uuid"
)

// Webinar represents a webinar session.
type Webinar struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at,omitempty"`
	CreatedBy        uuid.UUID  `json:"created_by"`
	OrganizationID   *uuid.UUID `json:"organization_id,omitempty"`
	IsPaid           bool       `json:"is_paid"`
	TicketPriceCents int        `json:"ticket_price_cents"`
	TicketCurrency   string     `json:"ticket_currency"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// WebinarSpeaker links a user as speaker to a webinar.
type WebinarSpeaker struct {
	WebinarID uuid.UUID `json:"webinar_id"`
	UserID    uuid.UUID `json:"user_id"`
	AddedAt   time.Time `json:"added_at"`
}
