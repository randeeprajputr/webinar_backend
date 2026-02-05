package streams

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles stream_sessions persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a stream sessions repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create creates a new stream session for a webinar.
func (r *Repository) Create(ctx context.Context, webinarID uuid.UUID) (*models.StreamSession, error) {
	const q = `INSERT INTO stream_sessions (id, webinar_id, started_at, peak_viewers, total_viewers, total_watch_time, poll_participation_count, questions_count)
		VALUES (gen_random_uuid(), $1, NOW(), 0, 0, 0, 0, 0)
		RETURNING id, webinar_id, started_at, ended_at, peak_viewers, total_viewers, total_watch_time, poll_participation_count, questions_count, created_at, updated_at`
	var s models.StreamSession
	err := r.pool.QueryRow(ctx, q, webinarID).Scan(&s.ID, &s.WebinarID, &s.StartedAt, &s.EndedAt, &s.PeakViewers, &s.TotalViewers, &s.TotalWatchTime, &s.PollParticipationCount, &s.QuestionsCount, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetActiveByWebinar returns the active (no ended_at) stream session for a webinar.
func (r *Repository) GetActiveByWebinar(ctx context.Context, webinarID uuid.UUID) (*models.StreamSession, error) {
	const q = `SELECT id, webinar_id, started_at, ended_at, peak_viewers, total_viewers, total_watch_time, poll_participation_count, questions_count, created_at, updated_at
		FROM stream_sessions WHERE webinar_id = $1 AND ended_at IS NULL ORDER BY started_at DESC LIMIT 1`
	var s models.StreamSession
	err := r.pool.QueryRow(ctx, q, webinarID).Scan(&s.ID, &s.WebinarID, &s.StartedAt, &s.EndedAt, &s.PeakViewers, &s.TotalViewers, &s.TotalWatchTime, &s.PollParticipationCount, &s.QuestionsCount, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// GetOrCreateActive returns the active stream session for a webinar, creating one if none exists.
func (r *Repository) GetOrCreateActive(ctx context.Context, webinarID uuid.UUID) (*models.StreamSession, error) {
	s, err := r.GetActiveByWebinar(ctx, webinarID)
	if err != nil || s != nil {
		return s, err
	}
	return r.Create(ctx, webinarID)
}

// UpdatePeakViewers sets peak_viewers for a session (call when current viewers > peak).
func (r *Repository) UpdatePeakViewers(ctx context.Context, sessionID uuid.UUID, peak int) error {
	const q = `UPDATE stream_sessions SET peak_viewers = $1, updated_at = NOW() WHERE id = $2 AND $1 > peak_viewers`
	_, err := r.pool.Exec(ctx, q, peak, sessionID)
	return err
}

// End sets ended_at for a session.
func (r *Repository) End(ctx context.Context, sessionID uuid.UUID) error {
	const q = `UPDATE stream_sessions SET ended_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, sessionID)
	return err
}

// IncrementPollParticipation increments poll_participation_count.
func (r *Repository) IncrementPollParticipation(ctx context.Context, sessionID uuid.UUID) error {
	const q = `UPDATE stream_sessions SET poll_participation_count = poll_participation_count + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, sessionID)
	return err
}

// IncrementQuestions increments questions_count.
func (r *Repository) IncrementQuestions(ctx context.Context, sessionID uuid.UUID) error {
	const q = `UPDATE stream_sessions SET questions_count = questions_count + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, sessionID)
	return err
}

// UpdateTotalWatchTime adds to total_watch_time (in seconds).
func (r *Repository) UpdateTotalWatchTime(ctx context.Context, sessionID uuid.UUID, delta int64) error {
	const q = `UPDATE stream_sessions SET total_watch_time = total_watch_time + $1, updated_at = NOW() WHERE id = $2`
	_, err := r.pool.Exec(ctx, q, delta, sessionID)
	return err
}

// UpdateTotalViewers sets total_viewers (unique viewers count; can be updated when session ends).
func (r *Repository) UpdateTotalViewers(ctx context.Context, sessionID uuid.UUID, total int) error {
	const q = `UPDATE stream_sessions SET total_viewers = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.pool.Exec(ctx, q, total, sessionID)
	return err
}

// Aggregates holds aggregated stream session stats for a webinar.
type Aggregates struct {
	PeakViewers    int
	TotalWatchTime int64
	TotalViewers   int
	QuestionsCount int
}

// GetAggregatesByWebinar returns aggregated stream session stats for a webinar.
func (r *Repository) GetAggregatesByWebinar(ctx context.Context, webinarID uuid.UUID) (*Aggregates, error) {
	const q = `SELECT
		COALESCE(MAX(peak_viewers), 0),
		COALESCE(SUM(total_watch_time), 0),
		COALESCE(SUM(total_viewers), 0),
		COALESCE(SUM(questions_count), 0)
		FROM stream_sessions WHERE webinar_id = $1`
	var a Aggregates
	err := r.pool.QueryRow(ctx, q, webinarID).Scan(&a.PeakViewers, &a.TotalWatchTime, &a.TotalViewers, &a.QuestionsCount)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
