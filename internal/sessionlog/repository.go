package sessionlog

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AttendeeRow is one row for GET /webinars/:id/attendees.
type AttendeeRow struct {
	UserID       *uuid.UUID `json:"user_id,omitempty"`
	RegistrationID *uuid.UUID `json:"registration_id,omitempty"`
	JoinedAt     time.Time  `json:"joined_at"`
	LeftAt       *time.Time `json:"left_at,omitempty"`
	WatchSeconds int64      `json:"watch_seconds"`
}

// Repository handles user_session_logs.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a session log repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// LogJoin inserts a row when a client joins a webinar (audience/speaker).
func (r *Repository) LogJoin(ctx context.Context, webinarID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_session_logs (webinar_id, user_id, joined_at) VALUES ($1, $2, NOW())`,
		webinarID, userID)
	return err
}

// LogLeave updates the most recent open session for this user in this webinar.
func (r *Repository) LogLeave(ctx context.Context, webinarID, userID uuid.UUID, _ time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE user_session_logs u SET left_at = NOW(), watch_seconds = GREATEST(0, EXTRACT(EPOCH FROM (NOW() - u.joined_at))::BIGINT)
		 FROM (SELECT id FROM user_session_logs WHERE webinar_id = $1 AND user_id = $2 AND left_at IS NULL ORDER BY joined_at DESC LIMIT 1) AS sub
		 WHERE u.id = sub.id`,
		webinarID, userID)
	return err
}

// WatchTimeAggregates holds sum of watch_seconds and distinct user count for a webinar.
type WatchTimeAggregates struct {
	TotalWatchSeconds int64
	DistinctUsers     int
}

// GetWatchTimeAggregates returns total watch time and distinct user count from session logs for analytics.
func (r *Repository) GetWatchTimeAggregates(ctx context.Context, webinarID uuid.UUID) (*WatchTimeAggregates, error) {
	const q = `SELECT COALESCE(SUM(watch_seconds), 0), COUNT(DISTINCT user_id) FROM user_session_logs WHERE webinar_id = $1 AND left_at IS NOT NULL`
	var agg WatchTimeAggregates
	err := r.pool.QueryRow(ctx, q, webinarID).Scan(&agg.TotalWatchSeconds, &agg.DistinctUsers)
	if err != nil {
		return nil, err
	}
	return &agg, nil
}

// ListByWebinar returns attendees for a webinar (join time, leave time, watch duration).
func (r *Repository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]AttendeeRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, registration_id, joined_at, left_at, watch_seconds
		 FROM user_session_logs WHERE webinar_id = $1 ORDER BY joined_at DESC`,
		webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AttendeeRow
	for rows.Next() {
		var row AttendeeRow
		if err := rows.Scan(&row.UserID, &row.RegistrationID, &row.JoinedAt, &row.LeftAt, &row.WatchSeconds); err != nil {
			return nil, err
		}
		list = append(list, row)
	}
	return list, rows.Err()
}
