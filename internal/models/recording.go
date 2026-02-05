package models

import (
	"time"

	"github.com/google/uuid"
)

// RecordingStatus represents recording lifecycle.
const (
	RecordingStatusRecording  = "recording"
	RecordingStatusProcessing = "processing"
	RecordingStatusCompleted = "completed"
	RecordingStatusFailed     = "failed"
)

// Recording is a webinar recording (provider → S3).
type Recording struct {
	ID                 uuid.UUID `json:"id"`
	WebinarID          uuid.UUID `json:"webinar_id"`
	ProviderRecordingID string   `json:"provider_recording_id,omitempty"`
	OriginalURL        string   `json:"original_url,omitempty"`
	S3URL              string   `json:"s3_url,omitempty"`
	S3Key              string   `json:"s3_key,omitempty"`
	Duration           int      `json:"duration"`
	FileSize           int64    `json:"file_size"`
	Status             string   `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
