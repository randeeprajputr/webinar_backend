package models

import (
	"time"

	"github.com/google/uuid"
)

// PaymentProvider is Stripe or Razorpay.
const (
	PaymentProviderStripe   = "stripe"
	PaymentProviderRazorpay = "razorpay"
)

// PaymentStatus for payments.
const (
	PaymentStatusPending           = "pending"
	PaymentStatusCompleted         = "completed"
	PaymentStatusFailed            = "failed"
	PaymentStatusRefunded          = "refunded"
	PaymentStatusPartiallyRefunded = "partially_refunded"
)

// Payment represents a payment for a paid webinar.
type Payment struct {
	ID                uuid.UUID  `json:"id"`
	WebinarID         uuid.UUID  `json:"webinar_id"`
	RegistrationID    *uuid.UUID `json:"registration_id,omitempty"`
	Provider          string     `json:"provider"`
	ProviderPaymentID string     `json:"provider_payment_id,omitempty"`
	ProviderOrderID   string     `json:"provider_order_id,omitempty"`
	AmountCents       int        `json:"amount_cents"`
	Currency          string     `json:"currency"`
	Status            string     `json:"status"`
	Metadata          []byte     `json:"metadata,omitempty"`
	RefundedAt        *time.Time `json:"refunded_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
