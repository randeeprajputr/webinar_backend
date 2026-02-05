package models

import (
	"time"

	"github.com/google/uuid"
)

// Advertisement is an ad creative (image/gif/video) stored in S3.
type Advertisement struct {
	ID        uuid.UUID `json:"id"`
	WebinarID uuid.UUID `json:"webinar_id"`
	FileURL   string    `json:"file_url"`
	FileType  string    `json:"file_type"`
	FileSize  int64     `json:"file_size"`
	Duration  int       `json:"duration"`
	S3Key     string    `json:"s3_key,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// AdPlaylist holds rotation config for a webinar.
type AdPlaylist struct {
	ID               uuid.UUID `json:"id"`
	WebinarID        uuid.UUID `json:"webinar_id"`
	RotationInterval int       `json:"rotation_interval"`
	IsRunning        bool      `json:"is_running"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// AdSchedule holds optional start/end time for an ad.
type AdSchedule struct {
	ID        uuid.UUID  `json:"id"`
	AdID      uuid.UUID  `json:"ad_id"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
