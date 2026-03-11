package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/config"
	"github.com/aura-webinar/backend/internal/emaillogs"
	"github.com/aura-webinar/backend/pkg/email"
	"github.com/aura-webinar/backend/pkg/queue"
)

// EmailProcessor processes email jobs: send via SMTP, update email_logs.
type EmailProcessor struct {
	emailRepo *emaillogs.Repository
	queue     *queue.Queue
	cfg       config.EmailConfig
	logger    *zap.Logger
}

// NewEmailProcessor creates an email processor.
func NewEmailProcessor(emailRepo *emaillogs.Repository, q *queue.Queue, cfg config.EmailConfig, logger *zap.Logger) *EmailProcessor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EmailProcessor{emailRepo: emailRepo, queue: q, cfg: cfg, logger: logger}
}

// Process executes one email job.
func (p *EmailProcessor) Process(ctx context.Context, job *queue.Job) error {
	if job.Type != queue.JobTypeEmail {
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
	var payload queue.EmailPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	// Create log entry (pending) before sending (skip for verification - no webinar/registration)
	var regID, webID *uuid.UUID
	if payload.RegistrationID != uuid.Nil {
		regID = &payload.RegistrationID
	}
	if payload.WebinarID != uuid.Nil {
		webID = &payload.WebinarID
	}
	logEntry, err := p.emailRepo.Create(ctx, webID, regID, payload.EmailType, payload.RecipientEmail, payload.Subject)
	if err != nil {
		p.logger.Warn("create email log failed", zap.Error(err))
		// Continue to send; log is best-effort
	}

	cfg := email.Config{
		Host:     p.cfg.SMTPHost,
		Port:     p.cfg.SMTPPort,
		User:     p.cfg.SMTPUser,
		Password: p.cfg.SMTPPass,
		From:     p.cfg.FromAddress,
		FromName:  p.cfg.FromName,
	}
	body := payload.BodyHTML
	if body == "" {
		body = p.buildDefaultBody(payload)
	}
	subject := payload.Subject
	if subject == "" {
		subject = p.buildDefaultSubject(payload)
	}

	if err := email.Send(cfg, payload.RecipientEmail, subject, body); err != nil {
		p.logger.Error("send email failed", zap.String("to", payload.RecipientEmail), zap.Error(err))
		if logEntry != nil {
			_ = p.emailRepo.MarkFailed(ctx, logEntry.ID, err.Error())
		}
		return fmt.Errorf("send: %w", err)
	}

	if logEntry != nil {
		_ = p.emailRepo.MarkSent(ctx, logEntry.ID)
	}
	p.logger.Info("email sent", zap.String("type", payload.EmailType), zap.String("to", payload.RecipientEmail))
	return nil
}

func (p *EmailProcessor) buildDefaultSubject(payload queue.EmailPayload) string {
	switch payload.EmailType {
	case "email_verification":
		return "Verify your email address"
	case "speaker_invitation":
		return fmt.Sprintf("You're invited to speak: %s", payload.WebinarTitle)
	case "registration_confirmation":
		return fmt.Sprintf("You're registered: %s", payload.WebinarTitle)
	case "reminder_24h":
		return fmt.Sprintf("Reminder: %s starts tomorrow", payload.WebinarTitle)
	case "reminder_1h":
		return fmt.Sprintf("Starting soon: %s", payload.WebinarTitle)
	case "reminder_10m":
		return fmt.Sprintf("Join now: %s", payload.WebinarTitle)
	default:
		return payload.WebinarTitle
	}
}

func (p *EmailProcessor) buildDefaultBody(payload queue.EmailPayload) string {
	name := payload.RecipientName
	if name == "" {
		name = "there"
	}
	html := fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:sans-serif;max-width:600px;margin:0 auto;padding:20px;">
<h2>%s</h2>
<p>Hi %s,</p>`, p.buildDefaultSubject(payload), name)
	switch payload.EmailType {
	case "email_verification":
		html += fmt.Sprintf(`<p>Please verify your email by clicking the link below:</p>
<p><a href="%s" style="display:inline-block;padding:12px 24px;background:#0ea5e9;color:white;text-decoration:none;border-radius:8px;">Verify email</a></p>
<p style="word-break:break-all;font-size:12px;color:#666;">%s</p>
<p>This link expires in 24 hours.</p>`, payload.VerifyURL, payload.VerifyURL)
	case "speaker_invitation":
		html += fmt.Sprintf(`<p>You've been invited to speak at <strong>%s</strong>.</p>
<p><a href="%s" style="display:inline-block;padding:12px 24px;background:#0ea5e9;color:white;text-decoration:none;border-radius:8px;">Accept invitation</a></p>
<p style="word-break:break-all;font-size:12px;color:#666;">%s</p>
<p>Create an account or sign in to join as a speaker.</p>`, payload.WebinarTitle, payload.InviteURL, payload.InviteURL)
	case "registration_confirmation":
		html += fmt.Sprintf(`<p>You're registered for <strong>%s</strong>.</p>
<p><strong>When:</strong> %s</p>
<p>Save your personal join link:</p>
<p><a href="%s" style="display:inline-block;padding:12px 24px;background:#0ea5e9;color:white;text-decoration:none;border-radius:8px;">Join webinar</a></p>
<p style="word-break:break-all;font-size:12px;color:#666;">%s</p>
<p>We'll send you a reminder before the webinar starts.</p>`,
			payload.WebinarTitle, payload.WebinarStartsAt, payload.JoinURL, payload.JoinURL)
	case "reminder_24h", "reminder_1h", "reminder_10m":
		html += fmt.Sprintf(`<p><strong>%s</strong> starts %s.</p>
<p><a href="%s" style="display:inline-block;padding:12px 24px;background:#0ea5e9;color:white;text-decoration:none;border-radius:8px;">Join now</a></p>`,
			payload.WebinarTitle, payload.WebinarStartsAt, payload.JoinURL)
	default:
		url := payload.JoinURL
		if payload.InviteURL != "" {
			url = payload.InviteURL
		}
		if url == "" {
			url = payload.VerifyURL
		}
		html += fmt.Sprintf(`<p>%s</p><p><a href="%s">Continue</a></p>`, payload.WebinarTitle, url)
	}
	html += `</body></html>`
	return html
}

// Run starts the worker loop.
func (p *EmailProcessor) Run(ctx context.Context) {
	if p.cfg.SMTPHost == "" || p.cfg.SMTPUser == "" || p.cfg.SMTPPass == "" {
		p.logger.Warn("email worker disabled: SMTP not configured")
		return
	}
	for {
		select {
		case <-ctx.Done():
			p.logger.Info("email worker stopping")
			return
		default:
		}

		job, _, err := p.queue.DequeueEmail(ctx)
		if err != nil {
			p.logger.Warn("dequeue email error", zap.Error(err))
			time.Sleep(queue.RetryBackoff)
			continue
		}
		if job == nil {
			continue
		}

		p.logger.Debug("processing email job", zap.String("job_id", job.ID), zap.String("type", string(job.Type)))
		if err := p.Process(ctx, job); err != nil {
			p.logger.Error("email job failed", zap.String("job_id", job.ID), zap.Error(err))
			if reErr := p.queue.RetryEmail(ctx, job); reErr != nil {
				p.logger.Error("retry enqueue failed", zap.Error(reErr))
			}
			time.Sleep(queue.RetryBackoff)
			continue
		}
	}
}
