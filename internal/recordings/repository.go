package recordings

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles recording persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a recordings repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new recording (e.g. when recording starts).
func (r *Repository) Create(ctx context.Context, rec *models.Recording) error {
	const q = `INSERT INTO recordings (id, webinar_id, provider_recording_id, original_url, s3_url, s3_key, duration, file_size, status)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, q, rec.WebinarID, rec.ProviderRecordingID, rec.OriginalURL, rec.S3URL, rec.S3Key, rec.Duration, rec.FileSize, rec.Status).
		Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
}

// GetByID returns a recording by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Recording, error) {
	const q = `SELECT id, webinar_id, COALESCE(provider_recording_id,''), COALESCE(original_url,''), COALESCE(s3_url,''), COALESCE(s3_key,''), duration, file_size, status, created_at, updated_at
		FROM recordings WHERE id = $1`
	var rec models.Recording
	err := r.pool.QueryRow(ctx, q, id).Scan(&rec.ID, &rec.WebinarID, &rec.ProviderRecordingID, &rec.OriginalURL, &rec.S3URL, &rec.S3Key, &rec.Duration, &rec.FileSize, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListByWebinar returns all recordings for a webinar.
func (r *Repository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]models.Recording, error) {
	const q = `SELECT id, webinar_id, COALESCE(provider_recording_id,''), COALESCE(original_url,''), COALESCE(s3_url,''), COALESCE(s3_key,''), duration, file_size, status, created_at, updated_at
		FROM recordings WHERE webinar_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Recording
	for rows.Next() {
		var rec models.Recording
		if err := rows.Scan(&rec.ID, &rec.WebinarID, &rec.ProviderRecordingID, &rec.OriginalURL, &rec.S3URL, &rec.S3Key, &rec.Duration, &rec.FileSize, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	return list, rows.Err()
}

// GetByProviderID returns a recording by provider_recording_id.
func (r *Repository) GetByProviderID(ctx context.Context, providerID string) (*models.Recording, error) {
	const q = `SELECT id, webinar_id, COALESCE(provider_recording_id,''), COALESCE(original_url,''), COALESCE(s3_url,''), COALESCE(s3_key,''), duration, file_size, status, created_at, updated_at
		FROM recordings WHERE provider_recording_id = $1`
	var rec models.Recording
	err := r.pool.QueryRow(ctx, q, providerID).Scan(&rec.ID, &rec.WebinarID, &rec.ProviderRecordingID, &rec.OriginalURL, &rec.S3URL, &rec.S3Key, &rec.Duration, &rec.FileSize, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateStatus sets recording status.
func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	const q = `UPDATE recordings SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.pool.Exec(ctx, q, status, id)
	return err
}

// UpdateS3Result sets S3 URL and key and status to completed.
func (r *Repository) UpdateS3Result(ctx context.Context, id uuid.UUID, s3URL, s3Key string, fileSize int64, duration int) error {
	const q = `UPDATE recordings SET s3_url = $1, s3_key = $2, file_size = $3, duration = $4, status = $5, updated_at = NOW() WHERE id = $6`
	_, err := r.pool.Exec(ctx, q, s3URL, s3Key, fileSize, duration, models.RecordingStatusCompleted, id)
	return err
}

// CreateFromWebinarStart creates a recording row when webinar recording starts (status = recording).
func (r *Repository) CreateFromWebinarStart(ctx context.Context, webinarID uuid.UUID, providerRecordingID string) (*models.Recording, error) {
	rec := &models.Recording{
		WebinarID:          webinarID,
		ProviderRecordingID: providerRecordingID,
		Status:             models.RecordingStatusRecording,
	}
	if err := r.Create(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// FindByWebinarStatus returns a recording for webinar with status (e.g. recording) if any.
func (r *Repository) FindByWebinarStatus(ctx context.Context, webinarID uuid.UUID, status string) (*models.Recording, error) {
	const q = `SELECT id, webinar_id, COALESCE(provider_recording_id,''), COALESCE(original_url,''), COALESCE(s3_url,''), COALESCE(s3_key,''), duration, file_size, status, created_at, updated_at
		FROM recordings WHERE webinar_id = $1 AND status = $2 ORDER BY created_at DESC LIMIT 1`
	var rec models.Recording
	err := r.pool.QueryRow(ctx, q, webinarID, status).Scan(&rec.ID, &rec.WebinarID, &rec.ProviderRecordingID, &rec.OriginalURL, &rec.S3URL, &rec.S3Key, &rec.Duration, &rec.FileSize, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

// UpdateOriginalURL sets original_url (from provider webhook).
func (r *Repository) UpdateOriginalURL(ctx context.Context, id uuid.UUID, originalURL string) error {
	const q = `UPDATE recordings SET original_url = $1, status = $2, updated_at = NOW() WHERE id = $3`
	_, err := r.pool.Exec(ctx, q, originalURL, models.RecordingStatusProcessing, id)
	return err
}
