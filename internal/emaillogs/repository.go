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

// Create inserts an email log (pending).
func (r *Repository) Create(ctx context.Context, webinarID, registrationID *uuid.UUID, emailType, recipientEmail, subject string) (*models.EmailLog, error) {
	const q = `INSERT INTO email_logs (id, webinar_id, registration_id, email_type, recipient_email, subject, status)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'pending')
		RETURNING id, webinar_id, registration_id, email_type, recipient_email, subject, status, sent_at, error_message, created_at`
	var el models.EmailLog
	var errMsg *string
	err := r.pool.QueryRow(ctx, q, webinarID, registrationID, emailType, recipientEmail, subject).
		Scan(&el.ID, &el.WebinarID, &el.RegistrationID, &el.EmailType, &el.RecipientEmail, &el.Subject, &el.Status, &el.SentAt, &errMsg, &el.CreatedAt)
	if err != nil {
		return nil, err
	}
	if errMsg != nil {
		el.ErrorMessage = *errMsg
	}
	return &el, nil
}

// MarkSent updates log to sent.
func (r *Repository) MarkSent(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE email_logs SET status = 'sent', sent_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// MarkFailed updates log to failed with error message.
func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	const q = `UPDATE email_logs SET status = 'failed', error_message = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, errMsg)
	return err
}

// AlreadySent checks if an email of the given type was already sent to this registration.
func (r *Repository) AlreadySent(ctx context.Context, registrationID uuid.UUID, emailType string) (bool, error) {
	const q = `SELECT EXISTS(
		SELECT 1 FROM email_logs
		WHERE registration_id = $1 AND email_type = $2 AND status = 'sent'
	)`
	var exists bool
	err := r.pool.QueryRow(ctx, q, registrationID, emailType).Scan(&exists)
	return exists, err
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
