package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/emaillogs"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/registrations"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/email"
	"github.com/aura-webinar/backend/pkg/queue"
)

const (
	// Scheduler runs every 5 minutes.
	reminderCheckInterval = 5 * time.Minute
	// Time window: ±5 minutes around target (e.g. 24h before = 23h55m to 24h5m).
	reminderWindowMargin = 5 * time.Minute
)

// ReminderScheduler enqueues reminder emails for webinars starting soon.
type ReminderScheduler struct {
	webinarRepo   *webinars.Repository
	regRepo       *registrations.Repository
	emailLogsRepo *emaillogs.Repository
	jobQueue      *queue.Queue
	frontendURL   string
	logger        *zap.Logger
}

// NewReminderScheduler creates a reminder scheduler.
func NewReminderScheduler(
	webinarRepo *webinars.Repository,
	regRepo *registrations.Repository,
	emailLogsRepo *emaillogs.Repository,
	q *queue.Queue,
	frontendURL string,
	logger *zap.Logger,
) *ReminderScheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReminderScheduler{
		webinarRepo:   webinarRepo,
		regRepo:       regRepo,
		emailLogsRepo: emailLogsRepo,
		jobQueue:      q,
		frontendURL:   frontendURL,
		logger:        logger,
	}
}

// Run starts the scheduler loop.
func (s *ReminderScheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(reminderCheckInterval)
	defer ticker.Stop()

	// Run once on start (after a short delay to let server settle)
	time.Sleep(30 * time.Second)
	s.checkAndEnqueue(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("reminder scheduler stopping")
			return
		case <-ticker.C:
			s.checkAndEnqueue(ctx)
		}
	}
}

func (s *ReminderScheduler) checkAndEnqueue(ctx context.Context) {
	now := time.Now()

	// 24h reminder: webinars starting between 23h55m and 24h5m from now
	s.enqueueForWindow(ctx, now, 24*time.Hour, models.EmailTypeReminder24h)

	// 1h reminder: webinars starting between 55m and 1h5m from now
	s.enqueueForWindow(ctx, now, 1*time.Hour, models.EmailTypeReminder1h)

	// 10m reminder: webinars starting between 5m and 15m from now
	s.enqueueForWindow(ctx, now, 10*time.Minute, models.EmailTypeReminder10m)
}

func (s *ReminderScheduler) enqueueForWindow(ctx context.Context, now time.Time, targetOffset time.Duration, emailType string) {
	windowStart := now.Add(targetOffset - reminderWindowMargin)
	windowEnd := now.Add(targetOffset + reminderWindowMargin)

	webinars, err := s.webinarRepo.ListStartingInWindow(ctx, windowStart, windowEnd)
	if err != nil {
		s.logger.Error("list webinars for reminder failed", zap.Error(err), zap.String("type", emailType))
		return
	}

	for _, w := range webinars {
		regs, err := s.regRepo.ListByWebinar(ctx, w.ID)
		if err != nil {
			s.logger.Warn("list registrations failed", zap.String("webinar_id", w.ID.String()), zap.Error(err))
			continue
		}

		for _, reg := range regs {
			sent, err := s.emailLogsRepo.AlreadySent(ctx, reg.ID, emailType)
			if err != nil || sent {
				continue
			}

			tok, err := s.regRepo.GetLatestTokenForRegistration(ctx, reg.ID)
			if err != nil || tok == nil {
				s.logger.Debug("no valid token for registration", zap.String("registration_id", reg.ID.String()))
				continue
			}

			joinURL := email.BuildJoinURL(s.frontendURL, w.ID.String(), tok.Token)
			payload := queue.EmailPayload{
				EmailType:       emailType,
				WebinarID:       w.ID,
				RegistrationID:  reg.ID,
				RecipientEmail:  reg.Email,
				RecipientName:   reg.FullName,
				WebinarTitle:    w.Title,
				WebinarStartsAt: w.StartsAt.Format(time.RFC3339),
				JoinURL:         joinURL,
				Subject:         s.subjectForType(emailType, w.Title),
			}

			if err := s.jobQueue.EnqueueEmail(ctx, payload); err != nil {
				s.logger.Warn("enqueue reminder failed", zap.String("email", reg.Email), zap.Error(err))
				continue
			}
			s.logger.Info("enqueued reminder", zap.String("type", emailType), zap.String("webinar", w.Title), zap.String("to", reg.Email))
		}
	}
}

func (s *ReminderScheduler) subjectForType(emailType, title string) string {
	switch emailType {
	case models.EmailTypeReminder24h:
		return fmt.Sprintf("Reminder: %s starts tomorrow", title)
	case models.EmailTypeReminder1h:
		return fmt.Sprintf("Starting soon: %s", title)
	case models.EmailTypeReminder10m:
		return fmt.Sprintf("Join now: %s", title)
	default:
		return title
	}
}
