package emaillogs

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles email_logs persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates an email logs repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListByWebinar returns email logs for a webinar, newest first.
func (r *Repository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]*models.EmailLog, error) {
	const q = `SELECT id, webinar_id, registration_id, email_type, recipient_email, subject, status, sent_at, error_message, created_at
		FROM email_logs
		WHERE webinar_id = $1
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*models.EmailLog
	for rows.Next() {
		var el models.EmailLog
		var subject, errMsg *string
		if err := rows.Scan(&el.ID, &el.WebinarID, &el.RegistrationID, &el.EmailType, &el.RecipientEmail, &subject, &el.Status, &el.SentAt, &errMsg, &el.CreatedAt); err != nil {
			return nil, err
		}
		if subject != nil {
			el.Subject = *subject
		}
		if errMsg != nil {
			el.ErrorMessage = *errMsg
		}
		list = append(list, &el)
	}
	return list, rows.Err()
}
