package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds application configuration loaded from environment.
type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	JWT       JWTConfig
	WebRTC    WebRTCConfig
	AWS       AWSConfig
	Recording RecordingConfig
	Stripe    StripeConfig
	Razorpay  RazorpayConfig
	Email     EmailConfig
}

// StripeConfig for global payments.
type StripeConfig struct {
	SecretKey      string
	WebhookSecret  string
}

// RazorpayConfig for India payments.
type RazorpayConfig struct {
	KeyID      string
	KeySecret  string
	WebhookSecret string
}

// EmailConfig for SMTP / sendgrid etc.
type EmailConfig struct {
	FromAddress string
	FromName    string
	SMTPHost    string
	SMTPPort    int
	SMTPUser    string
	SMTPPass    string
	APIKey      string // optional e.g. SendGrid
}

// RecordingConfig holds in-app recording (speaker view) settings.
type RecordingConfig struct {
	OutputDir string // directory for temp recording files; empty = os.TempDir()
}

// WebRTCConfig holds STUN/TURN ICE server URLs for WebRTC.
type WebRTCConfig struct {
	ICEUrls []string // e.g. stun:stun.l.google.com:19302 (comma-separated in env)
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port               string
	ReadTimeout        int
	WriteTimeout       int
	CORSAllowedOrigins string // comma-separated, or "*" for all (e.g. http://localhost:3000,http://localhost:3001)
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	URL      string // if set, used as-is (e.g. postgres://localhost:5432/webinar?sslmode=disable)
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// JWTConfig holds JWT signing and validation settings.
type JWTConfig struct {
	Secret      string
	ExpireHours int
}

// AWSConfig holds AWS credentials and S3 bucket names.
type AWSConfig struct {
	Region              string
	AccessKeyID         string
	SecretAccessKey     string
	AdsBucket           string
	RecordingsBucket    string
	PresignExpireMinutes int
}

// DSN returns the PostgreSQL connection string.
// If DatabaseConfig.URL is set (e.g. DATABASE_URL env), it is used as-is; otherwise built from components.
func (c DatabaseConfig) DSN() string {
	if c.URL != "" {
		return c.URL
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode,
	)
}

// Load reads configuration from environment, with optional .env file.
func Load() (*Config, error) {
	_ = godotenv.Load()   // .env
	_ = godotenv.Load("env") // env (no leading dot)

	readTimeout, _ := strconv.Atoi(getEnv("READ_TIMEOUT_SEC", "30"))
	writeTimeout, _ := strconv.Atoi(getEnv("WRITE_TIMEOUT_SEC", "30"))
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	jwtExpire, _ := strconv.Atoi(getEnv("JWT_EXPIRE_HOURS", "24"))

	cfg := &Config{
		Server: ServerConfig{
			Port:               getEnv("PORT", "8080"),
			ReadTimeout:        readTimeout,
			WriteTimeout:       writeTimeout,
			CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001"),
		},
		Database: DatabaseConfig{
			URL:      getEnv("DATABASE_URL", "postgres://localhost:5432/webinar?sslmode=disable"),
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			DBName:   getEnv("DB_NAME", "webinar"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		JWT: JWTConfig{
			Secret:      getEnv("JWT_SECRET", "change-me-in-production"),
			ExpireHours: jwtExpire,
		},
		WebRTC: WebRTCConfig{
			ICEUrls: splitTrim(getEnv("WEBRTC_ICE_URLS", "stun:stun.l.google.com:19302"), ","),
		},
		AWS: AWSConfig{
			Region:               getEnv("AWS_REGION", "us-east-1"),
			AccessKeyID:          getEnv("AWS_ACCESS_KEY_ID", ""),
			SecretAccessKey:      getEnv("AWS_SECRET_ACCESS_KEY", ""),
			AdsBucket:            getEnv("AWS_S3_ADS_BUCKET", "webinar-ads-bucket"),
			RecordingsBucket:     getEnv("AWS_S3_RECORDINGS_BUCKET", "webinar-recordings-bucket"),
			PresignExpireMinutes: getEnvInt("AWS_PRESIGN_EXPIRE_MINUTES", 15),
		},
		Recording: RecordingConfig{
			OutputDir: getEnv("RECORDING_OUTPUT_DIR", ""),
		},
		Stripe: StripeConfig{
			SecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
			WebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		},
		Razorpay: RazorpayConfig{
			KeyID:         getEnv("RAZORPAY_KEY_ID", ""),
			KeySecret:     getEnv("RAZORPAY_KEY_SECRET", ""),
			WebhookSecret: getEnv("RAZORPAY_WEBHOOK_SECRET", ""),
		},
		Email: EmailConfig{
			FromAddress: getEnv("EMAIL_FROM_ADDRESS", "noreply@example.com"),
			FromName:    getEnv("EMAIL_FROM_NAME", "Aura Webinar"),
			SMTPHost:    getEnv("SMTP_HOST", ""),
			SMTPPort:    getEnvInt("SMTP_PORT", 587),
			SMTPUser:    getEnv("SMTP_USER", ""),
			SMTPPass:    getEnv("SMTP_PASS", ""),
			APIKey:      getEnv("EMAIL_API_KEY", ""),
		},
	}
	return cfg, nil
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func splitTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, v := range strings.Split(s, sep) {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
