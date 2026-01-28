// Package email provides SMTP email sending functionality.
//
// When configured, it uses STARTTLS and authentication.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Config holds SMTP configuration.
type Config struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
}

// Enabled returns true if SMTP is configured with at least a host.
func (c *Config) Enabled() bool {
	return c.Host != ""
}

// Validate checks that required fields are set and applies defaults.
func (c *Config) Validate() error {
	if c.Host == "" {
		return errors.New("smtp: host is required")
	}
	if c.Username == "" {
		return errors.New("smtp: username is required")
	}
	if c.Password == "" {
		return errors.New("smtp: password is required")
	}
	if c.From == "" {
		return errors.New("smtp: from is required")
	}
	if c.Port == "" {
		c.Port = "587"
	}
	return nil
}

// Service provides email sending functionality.
type Service struct {
	Config Config
}

// Send sends an email.
func (s *Service) Send(ctx context.Context, to, subject, body string) error {
	return s.sendMail(ctx, []string{to}, subject, body)
}

// SendVerification sends an email verification email with a magic link.
func (s *Service) SendVerification(ctx context.Context, to, name, verifyURL string, locale Locale) error {
	subject, body := VerificationEmail(locale, name, verifyURL)
	return s.Send(ctx, to, subject, body)
}

// SendOrgInvitation sends an organization invitation email.
func (s *Service) SendOrgInvitation(ctx context.Context, to, orgName, inviterName, role, acceptURL string, locale Locale) error {
	subject, body := OrgInvitationEmail(locale, orgName, inviterName, role, acceptURL)
	return s.Send(ctx, to, subject, body)
}

// SendWSInvitation sends a workspace invitation email.
func (s *Service) SendWSInvitation(ctx context.Context, to, wsName, orgName, inviterName, role, acceptURL string, locale Locale) error {
	subject, body := WSInvitationEmail(locale, wsName, orgName, inviterName, role, acceptURL)
	return s.Send(ctx, to, subject, body)
}

// SendMultiple sends an email to multiple recipients.
func (s *Service) SendMultiple(ctx context.Context, to []string, subject, body string) error {
	return s.sendMail(ctx, to, subject, body)
}

func (s *Service) sendMail(ctx context.Context, to []string, subject, body string) error {
	addr := net.JoinHostPort(s.Config.Host, s.Config.Port)

	// Connect with timeout
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	client, err := smtp.NewClient(conn, s.Config.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() {
		if err := client.Quit(); err != nil {
			slog.WarnContext(ctx, "SMTP quit failed", "err", err)
		}
	}()

	// STARTTLS
	tlsConfig := &tls.Config{
		ServerName: s.Config.Host,
		MinVersion: tls.VersionTLS12,
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	// Auth
	auth := smtp.PlainAuth("", s.Config.Username, s.Config.Password, s.Config.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	// Set sender
	if err := client.Mail(s.Config.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}

	// Set recipients
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt to %s: %w", rcpt, err)
		}
	}

	// Send message
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}

	msg := buildMessage(s.Config.From, to, subject, body)
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	slog.InfoContext(ctx, "Email sent", "to", to, "subject", subject)
	return nil
}

func buildMessage(from string, to []string, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: ")
	sb.WriteString(from)
	sb.WriteString("\r\n")
	sb.WriteString("To: ")
	sb.WriteString(strings.Join(to, ", "))
	sb.WriteString("\r\n")
	sb.WriteString("Subject: ")
	sb.WriteString(subject)
	sb.WriteString("\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}
