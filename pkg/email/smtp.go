package email

import (
	"fmt"
	"net/smtp"
	"net/url"
	"strings"
)

// Config holds SMTP settings.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	FromName string
}

// Send sends an email via SMTP.
func Send(cfg Config, to, subject, bodyHTML string) error {
	if cfg.Host == "" || cfg.User == "" || cfg.Password == "" {
		return fmt.Errorf("smtp not configured")
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
	from := cfg.From
	if cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", cfg.FromName, cfg.From)
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, to, subject, bodyHTML)
	return smtp.SendMail(addr, auth, cfg.From, []string{to}, []byte(msg))
}

// BuildJoinURL returns the full audience join URL.
func BuildJoinURL(baseURL, webinarID, token string) string {
	base := strings.TrimSuffix(baseURL, "/")
	return fmt.Sprintf("%s/audience?webinar_id=%s&token=%s", base, webinarID, url.QueryEscape(token))
}
