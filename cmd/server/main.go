// Package main runs the webinar platform HTTP server with WebSocket and graceful shutdown.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	webrtc "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/aura-webinar/backend/config"
	"github.com/aura-webinar/backend/internal/ads"
	"github.com/aura-webinar/backend/internal/analytics"
	"github.com/aura-webinar/backend/internal/auth"
	"github.com/aura-webinar/backend/internal/emaillogs"
	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/polls"
	"github.com/aura-webinar/backend/internal/questions"
	"github.com/aura-webinar/backend/internal/organizations"
	"github.com/aura-webinar/backend/internal/recorder"
	"github.com/aura-webinar/backend/internal/realtime"
	"github.com/aura-webinar/backend/internal/recordings"
	"github.com/aura-webinar/backend/internal/sessionlog"
	"github.com/aura-webinar/backend/internal/registrations"
	"github.com/aura-webinar/backend/internal/streams"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/internal/worker"
	"github.com/aura-webinar/backend/internal/zego"
	"github.com/aura-webinar/backend/pkg/database"
	"github.com/aura-webinar/backend/pkg/queue"
	"github.com/aura-webinar/backend/pkg/redis"
	"github.com/aura-webinar/backend/pkg/response"
	"github.com/aura-webinar/backend/pkg/storage"
)

func main() {
	logger := newLogger()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg.Database.DSN(), logger)
	if err != nil {
		logger.Fatal("database", zap.Error(err))
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		logger.Fatal("migrate", zap.Error(err))
	}

	rdb, err := redis.NewClient(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, logger)
	if err != nil {
		logger.Fatal("redis", zap.Error(err))
	}
	defer rdb.Close()

	var s3Client *storage.S3
	if cfg.AWS.Region != "" {
		s3Cfg := storage.S3Config{
			Region:               cfg.AWS.Region,
			AccessKeyID:          cfg.AWS.AccessKeyID,
			SecretAccessKey:      cfg.AWS.SecretAccessKey,
			AdsBucket:            cfg.AWS.AdsBucket,
			RecordingsBucket:     cfg.AWS.RecordingsBucket,
			PresignExpireMinutes: cfg.AWS.PresignExpireMinutes,
		}
		s3Client, err = storage.NewS3(ctx, s3Cfg, logger)
		if err != nil {
			logger.Warn("s3 disabled", zap.Error(err))
		}
	}

	jwtService := auth.NewJWTService(cfg.JWT.Secret, cfg.JWT.ExpireHours)
	redisPubSub := realtime.NewRedisPubSub(rdb.Client, logger)
	hub := realtime.NewHub(logger, redisPubSub, redisPubSub)

	iceServers := make([]webrtc.ICEServer, 0, len(cfg.WebRTC.ICEUrls))
	for _, u := range cfg.WebRTC.ICEUrls {
		if u != "" {
			iceServers = append(iceServers, webrtc.ICEServer{URLs: []string{u}})
		}
	}
	sfu := realtime.NewSFU(logger, iceServers)

	// Auth
	authRepo := auth.NewRepository(pool)
	authHandler := auth.NewHandler(authRepo, jwtService, logger)

	// Webinars
	webinarRepo := webinars.NewRepository(pool)
	webinarHandler := webinars.NewHandler(webinarRepo)
	zegoHandler := zego.NewHandler(webinarRepo, cfg.Zego, logger)

	// Organizations (Phase 2)
	orgRepo := organizations.NewRepository(pool)
	orgHandler := organizations.NewHandler(orgRepo)

	// Registrations (Phase 2)
	registrationRepo := registrations.NewRepository(pool)
	registrationHandler := registrations.NewHandler(registrationRepo, webinarRepo, logger)

	// Questions
	questionRepo := questions.NewRepository(pool)
	questionHandler := questions.NewHandler(questionRepo, hub)

	// Polls
	pollRepo := polls.NewRepository(pool)
	pollHandler := polls.NewHandler(pollRepo, webinarRepo, hub)

	// Ads (legacy)
	adRepo := ads.NewRepository(pool)
	adHandler := ads.NewHandler(adRepo, webinarRepo, hub)

	// Advanced Ads (S3-backed advertisements, playlists, rotation)
	advertisementRepo := ads.NewAdvertisementRepository(pool)
	rotatorRegistry := ads.NewRotatorRegistry()
	advertisementHandler := ads.NewAdvertisementHandler(advertisementRepo, webinarRepo, s3Client, hub, rotatorRegistry, logger)

	// Recordings
	recordingRepo := recordings.NewRepository(pool)
	recordingHandler := recordings.NewHandler(recordingRepo, webinarRepo, s3Client, logger)
	jobQueue := queue.NewQueue(rdb.Client, logger)
	recordingWebhook := recordings.NewWebhookHandler(recordingRepo, jobQueue, logger)
	recordingProcessor := worker.NewRecordingProcessor(recordingRepo, s3Client, jobQueue, logger)

	// In-app recording (speaker view via SFU + ffmpeg)
	recorderSvc := recorder.NewService(sfu, cfg.Recording.OutputDir, logger)
	recordingHandler.SetRecordingService(recorderSvc)

	// Stream metadata (peak viewers)
	streamRepo := streams.NewRepository(pool)
	hub.SetAudienceChangeHandler(func(webinarID uuid.UUID, count int) {
		session, err := streamRepo.GetOrCreateActive(ctx, webinarID)
		if err != nil {
			return
		}
		if session != nil && count > session.PeakViewers {
			_ = streamRepo.UpdatePeakViewers(ctx, session.ID, count)
		}
	})

	// Attendee list (join/leave session logs) and mark registration as attended when user joins livestream
	sessionLogRepo := sessionlog.NewRepository(pool)
	sessionLogHandler := sessionlog.NewHandler(sessionLogRepo)
	hub.SetSessionLogger(
		func(webinarID, userID uuid.UUID) {
			ctx := context.Background()
			_ = sessionLogRepo.LogJoin(ctx, webinarID, userID)
			// Mark registration as attended so analytics "Attended" count is correct
			u, err := authRepo.GetByID(ctx, userID)
			if err != nil || u == nil {
				return
			}
			reg, err := registrationRepo.GetRegistrationByWebinarAndEmail(ctx, webinarID, u.Email)
			if err != nil || reg == nil {
				return
			}
			_ = registrationRepo.MarkAttended(ctx, reg.ID)
		},
		func(webinarID, userID uuid.UUID, joinedAt time.Time) { _ = sessionLogRepo.LogLeave(context.Background(), webinarID, userID, joinedAt) },
	)

	// Analytics (admin or webinar org access)
	analyticsHandler := analytics.NewHandler(pool, registrationRepo, questionRepo, streamRepo, webinarRepo, sessionLogRepo)

	emailLogsRepo := emaillogs.NewRepository(pool)
	emailLogsHandler := emaillogs.NewHandler(emailLogsRepo)

	jwtValidate := func(token string) (userID, role string, err error) {
		claims, err := jwtService.Validate(token)
		if err != nil {
			return "", "", err
		}
		return claims.UserID.String(), claims.Role, nil
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS(cfg.Server.CORSAllowedOrigins))
	router.Use(middleware.Logger(logger))

	// Health
	router.GET("/health", func(c *gin.Context) { response.OK(c, gin.H{"status": "ok"}) })

	// Public: webinar registration and token validation (Phase 2)
	router.POST("/webinars/:id/register", registrationHandler.Register)
	router.GET("/registrations/:token/validate", registrationHandler.ValidateToken)

	// Auth (public)
	authGroup := router.Group("/auth")
	{
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/register", authHandler.Register)
	}

	// Protected API (JWT required)
	api := router.Group("")
	api.Use(middleware.JWT(jwtService))
	{
		// Users (admin only; for speaker assignment etc.)
		api.GET("/users", middleware.RequireRole("admin"), authHandler.List)

		// Organizations (create, join, list my orgs; list members for org access)
		api.GET("/organizations", orgHandler.ListMyOrganizations)
		api.POST("/organizations", orgHandler.CreateOrganization)
		api.POST("/organizations/join", orgHandler.JoinOrganization)
		api.GET("/organizations/:id/members", orgHandler.ListMembers)

		// Webinars
		api.GET("/webinars", webinarHandler.List)
		api.POST("/webinars", middleware.RequireRole("admin"), webinarHandler.Create)
		api.GET("/webinars/:id", webinarHandler.GetByID)
		api.GET("/webinars/:id/analytics", webinars.RequireWebinarOrgAccess(webinarRepo, orgRepo), analyticsHandler.GetByWebinar)
		api.GET("/webinars/:id/emails", webinars.RequireWebinarOrgAccess(webinarRepo, orgRepo), emailLogsHandler.ListByWebinar)
		api.POST("/webinars/:id/emails/resend", webinars.RequireWebinarOrgAccess(webinarRepo, orgRepo), emailLogsHandler.Resend)
		api.PATCH("/webinars/:id", webinars.RequireWebinarOrgAccess(webinarRepo, orgRepo), webinarHandler.Update)
		api.PUT("/webinars/:id/registration-form", webinars.RequireWebinarOrgAccess(webinarRepo, orgRepo), webinarHandler.UpdateRegistrationForm)
		api.DELETE("/webinars/:id", webinarHandler.Delete)
		api.POST("/webinars/:id/speakers", middleware.RequireRole("admin", "speaker"), webinarHandler.AddSpeaker)
		api.GET("/webinars/:id/audience_count", webinarHandler.AudienceCount(hub))
		api.GET("/webinars/:id/attendees", middleware.RequireRole("admin", "speaker"), sessionLogHandler.GetAttendees)
		api.GET("/webinars/:id/zego-token", zegoHandler.GetToken)

		// Questions
		api.POST("/webinars/:id/questions", questionHandler.Create)
		api.GET("/webinars/:id/questions", middleware.RequireRole("admin", "speaker"), questionHandler.ListByWebinar)
		api.PATCH("/questions/:id/approve", middleware.RequireRole("admin", "speaker"), questionHandler.Approve)
		api.PATCH("/questions/:id/answer", middleware.RequireRole("admin", "speaker"), questionHandler.Answer)
		api.POST("/questions/:id/upvote", questionHandler.Upvote)

		// Polls
		api.POST("/webinars/:id/polls", middleware.RequireRole("admin", "speaker"), pollHandler.Create)
		api.POST("/polls/:id/launch", middleware.RequireRole("admin", "speaker"), pollHandler.Launch)
		api.POST("/polls/:id/close", middleware.RequireRole("admin", "speaker"), pollHandler.Close)
		api.POST("/polls/:id/answer", pollHandler.Answer)

		// Ads (legacy activate only; create is via advertisement handler below)
		api.PATCH("/ads/:id/activate", middleware.RequireRole("admin", "speaker"), adHandler.Activate)

		// Advertisements (S3-backed; admin only). Use /ads/upload for public bucket (no presigned URL, no CORS).
		api.POST("/webinars/:id/ads/upload", middleware.RequireRole("admin"), advertisementHandler.UploadAd)
		api.POST("/webinars/:id/ads/generate-upload-url", middleware.RequireRole("admin"), advertisementHandler.GenerateUploadURL)
		api.POST("/webinars/:id/ads", middleware.RequireRole("admin"), advertisementHandler.CreateAdvertisement)
		api.GET("/webinars/:id/ads", advertisementHandler.ListAdvertisements)
		api.GET("/webinars/:id/ads/:adId/image", middleware.RequireRole("admin", "speaker"), advertisementHandler.GetAdImage)
		api.PATCH("/ads/:id/toggle", middleware.RequireRole("admin"), advertisementHandler.ToggleAdvertisement)
		api.DELETE("/ads/:id", middleware.RequireRole("admin"), advertisementHandler.DeleteAdvertisement)
		api.POST("/webinars/:id/ads/playlist/start", middleware.RequireRole("admin"), advertisementHandler.StartPlaylist)
		api.POST("/webinars/:id/ads/playlist/stop", middleware.RequireRole("admin"), advertisementHandler.StopPlaylist)

		// Recordings
		api.GET("/webinars/:id/recordings", recordingHandler.ListByWebinar)
		api.GET("/recordings/:id/download-url", recordingHandler.GenerateDownloadURL)
		api.POST("/webinars/:id/recording/start", recordingHandler.StartRecording)
		api.POST("/webinars/:id/recording/stop", recordingHandler.StopRecording)
	}

	// Webhooks (no JWT; validate webhook signature in handler when configured)
	router.POST("/webhooks/recording-ready", recordingWebhook.RecordingReady)

	// WebSocket (token in query; no Authorization header required)
	router.GET("/ws", func(c *gin.Context) {
		realtime.ServeWs(hub, logger, jwtValidate, sfu)(c)
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	// Background worker (recording upload to S3)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	if s3Client != nil {
		go recordingProcessor.Run(workerCtx)
		logger.Info("recording worker started")
	}

	go func() {
		logger.Info("server listening", zap.String("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	workerCancel()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown", zap.Error(err))
	}
	logger.Info("server stopped")
}

func newLogger() *zap.Logger {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, _ := config.Build()
	return logger
}
