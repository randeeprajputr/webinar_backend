package models

import (
	"time"

	"github.com/google/uuid"
)

// CouponDiscountType is percent or fixed.
const (
	CouponDiscountPercent = "percent"
	CouponDiscountFixed   = "fixed"
)

// Coupon is a discount code for a webinar.
type Coupon struct {
	ID           uuid.UUID  `json:"id"`
	WebinarID    uuid.UUID  `json:"webinar_id"`
	Code         string     `json:"code"`
	DiscountType string     `json:"discount_type"`
	DiscountValue int       `json:"discount_value"`
	MaxUses      int        `json:"max_uses"`
	UsedCount    int        `json:"used_count"`
	ValidFrom    time.Time  `json:"valid_from"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
