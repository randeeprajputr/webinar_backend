package ads

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// AdvertisementRepository handles advertisement, playlist, and schedule persistence.
type AdvertisementRepository struct {
	pool *pgxpool.Pool
}

// NewAdvertisementRepository creates an advertisement repository.
func NewAdvertisementRepository(pool *pgxpool.Pool) *AdvertisementRepository {
	return &AdvertisementRepository{pool: pool}
}

// CreateAdvertisement inserts a new advertisement.
func (r *AdvertisementRepository) CreateAdvertisement(ctx context.Context, a *models.Advertisement) error {
	const q = `INSERT INTO advertisements (id, webinar_id, file_url, file_type, file_size, duration, s3_key, is_active)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, a.WebinarID, a.FileURL, a.FileType, a.FileSize, a.Duration, a.S3Key, a.IsActive).
		Scan(&a.ID, &a.CreatedAt)
}

// GetAdvertisementByID returns an advertisement by ID.
func (r *AdvertisementRepository) GetAdvertisementByID(ctx context.Context, id uuid.UUID) (*models.Advertisement, error) {
	const q = `SELECT id, webinar_id, file_url, file_type, file_size, duration, COALESCE(s3_key,''), is_active, created_at
		FROM advertisements WHERE id = $1`
	var a models.Advertisement
	err := r.pool.QueryRow(ctx, q, id).Scan(&a.ID, &a.WebinarID, &a.FileURL, &a.FileType, &a.FileSize, &a.Duration, &a.S3Key, &a.IsActive, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListByWebinar returns all advertisements for a webinar.
func (r *AdvertisementRepository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]models.Advertisement, error) {
	const q = `SELECT id, webinar_id, file_url, file_type, file_size, duration, COALESCE(s3_key,''), is_active, created_at
		FROM advertisements WHERE webinar_id = $1 ORDER BY created_at`
	rows, err := r.pool.Query(ctx, q, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Advertisement
	for rows.Next() {
		var a models.Advertisement
		if err := rows.Scan(&a.ID, &a.WebinarID, &a.FileURL, &a.FileType, &a.FileSize, &a.Duration, &a.S3Key, &a.IsActive, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// ListActiveByWebinar returns active advertisements for a webinar (for rotation).
func (r *AdvertisementRepository) ListActiveByWebinar(ctx context.Context, webinarID uuid.UUID) ([]models.Advertisement, error) {
	const q = `SELECT id, webinar_id, file_url, file_type, file_size, duration, COALESCE(s3_key,''), is_active, created_at
		FROM advertisements WHERE webinar_id = $1 AND is_active = TRUE ORDER BY created_at`
	rows, err := r.pool.Query(ctx, q, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Advertisement
	for rows.Next() {
		var a models.Advertisement
		if err := rows.Scan(&a.ID, &a.WebinarID, &a.FileURL, &a.FileType, &a.FileSize, &a.Duration, &a.S3Key, &a.IsActive, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// ToggleActive flips is_active for an advertisement.
func (r *AdvertisementRepository) ToggleActive(ctx context.Context, id uuid.UUID) (bool, error) {
	const q = `UPDATE advertisements SET is_active = NOT is_active WHERE id = $1 RETURNING is_active`
	var active bool
	err := r.pool.QueryRow(ctx, q, id).Scan(&active)
	if err != nil {
		return false, err
	}
	return active, nil
}

// DeleteAdvertisement removes an advertisement by ID.
func (r *AdvertisementRepository) DeleteAdvertisement(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM advertisements WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// GetOrCreatePlaylist returns the playlist for a webinar, creating one if missing.
func (r *AdvertisementRepository) GetOrCreatePlaylist(ctx context.Context, webinarID uuid.UUID, rotationInterval int) (*models.AdPlaylist, error) {
	const getQ = `SELECT id, webinar_id, rotation_interval, is_running, created_at, updated_at FROM ad_playlists WHERE webinar_id = $1`
	var p models.AdPlaylist
	err := r.pool.QueryRow(ctx, getQ, webinarID).Scan(&p.ID, &p.WebinarID, &p.RotationInterval, &p.IsRunning, &p.CreatedAt, &p.UpdatedAt)
	if err == nil {
		return &p, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}
	const insQ = `INSERT INTO ad_playlists (id, webinar_id, rotation_interval, is_running)
		VALUES (gen_random_uuid(), $1, $2, FALSE)
		RETURNING id, webinar_id, rotation_interval, is_running, created_at, updated_at`
	err = r.pool.QueryRow(ctx, insQ, webinarID, rotationInterval).
		Scan(&p.ID, &p.WebinarID, &p.RotationInterval, &p.IsRunning, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SetPlaylistRunning sets is_running for a webinar's playlist.
func (r *AdvertisementRepository) SetPlaylistRunning(ctx context.Context, webinarID uuid.UUID, running bool) error {
	const q = `UPDATE ad_playlists SET is_running = $1, updated_at = NOW() WHERE webinar_id = $2`
	_, err := r.pool.Exec(ctx, q, running, webinarID)
	return err
}

// GetPlaylistByWebinar returns the playlist for a webinar (if any).
func (r *AdvertisementRepository) GetPlaylistByWebinar(ctx context.Context, webinarID uuid.UUID) (*models.AdPlaylist, error) {
	const q = `SELECT id, webinar_id, rotation_interval, is_running, created_at, updated_at FROM ad_playlists WHERE webinar_id = $1`
	var p models.AdPlaylist
	err := r.pool.QueryRow(ctx, q, webinarID).Scan(&p.ID, &p.WebinarID, &p.RotationInterval, &p.IsRunning, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListSchedulesByAdID returns schedules for an ad.
func (r *AdvertisementRepository) ListSchedulesByAdID(ctx context.Context, adID uuid.UUID) ([]models.AdSchedule, error) {
	const q = `SELECT id, ad_id, start_time, end_time, created_at FROM ad_schedule WHERE ad_id = $1`
	rows, err := r.pool.Query(ctx, q, adID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.AdSchedule
	for rows.Next() {
		var s models.AdSchedule
		if err := rows.Scan(&s.ID, &s.AdID, &s.StartTime, &s.EndTime, &s.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// CreateAdSchedule inserts a schedule for an ad.
func (r *AdvertisementRepository) CreateAdSchedule(ctx context.Context, adID uuid.UUID, startTime, endTime *time.Time) error {
	const q = `INSERT INTO ad_schedule (id, ad_id, start_time, end_time) VALUES (gen_random_uuid(), $1, $2, $3)`
	_, err := r.pool.Exec(ctx, q, adID, startTime, endTime)
	return err
}

// IsAdScheduledNow returns true if the ad is within any active schedule window.
func (r *AdvertisementRepository) IsAdScheduledNow(ctx context.Context, adID uuid.UUID, now time.Time) (bool, error) {
	schedules, err := r.ListSchedulesByAdID(ctx, adID)
	if err != nil {
		return false, err
	}
	if len(schedules) == 0 {
		return true, nil
	}
	for _, s := range schedules {
		if s.StartTime != nil && now.Before(*s.StartTime) {
			continue
		}
		if s.EndTime != nil && now.After(*s.EndTime) {
			continue
		}
		return true, nil
	}
	return false, nil
}
