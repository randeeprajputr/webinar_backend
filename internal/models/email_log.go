package models

import (
	"time"

	"github.com/google/uuid"
)

// EmailType for automation.
const (
	EmailTypeRegistrationConfirmation = "registration_confirmation"
	EmailTypeReminder24h              = "reminder_24h"
	EmailTypeReminder1h               = "reminder_1h"
	EmailTypeThankYou                 = "thank_you"
	EmailTypeReplayAccess             = "replay_access"
)

// EmailLogStatus for delivery.
const (
	EmailLogStatusPending = "pending"
	EmailLogStatusSent    = "sent"
	EmailLogStatusFailed  = "failed"
)

// EmailLog records sent automation emails.
type EmailLog struct {
	ID             uuid.UUID  `json:"id"`
	WebinarID      *uuid.UUID `json:"webinar_id,omitempty"`
	RegistrationID *uuid.UUID `json:"registration_id,omitempty"`
	EmailType      string     `json:"email_type"`
	RecipientEmail string     `json:"recipient_email"`
	Subject        string     `json:"subject,omitempty"`
	Status         string     `json:"status"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}
