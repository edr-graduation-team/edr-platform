// Package service — transactional email sender used by MFA verification.
//
// Implementation notes:
//   - Uses the Go stdlib (net/smtp + crypto/tls). No third-party dependency
//     is added; this matches the project's "minimise external surface area"
//     stance for security-sensitive components.
//   - Two transport modes are supported:
//       * Implicit TLS (UseTLS=true)   → typically port 465. We dial a TLS
//         socket directly and authenticate over it.
//       * STARTTLS    (StartTLS=true)  → typically port 587. We dial plain
//         TCP, run smtp.Hello, then upgrade with smtp.StartTLS.
//   - When SMTP is disabled or misconfigured the service degrades gracefully:
//     Send returns an error that callers can log and surface to operators
//     without blocking authentication code paths.
package service

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ErrEmailDisabled is returned when SMTP is not enabled in config.
var ErrEmailDisabled = errors.New("email: SMTP is not enabled")

// EmailMessage describes one outbound message.
type EmailMessage struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// EmailSender is the contract used by other services (MFA, etc.). Defined
// as an interface so unit tests can stub it out without an SMTP server.
type EmailSender interface {
	Send(msg EmailMessage) error
	Enabled() bool
	From() string
}

// SMTPSettings is the minimal config slice consumed by the email service.
// We accept a struct rather than the full config.Config to keep the package
// dependency graph clean (no import of /config from /service).
type SMTPSettings struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
	StartTLS bool
	Enabled  bool
}

// EmailService is the production implementation of EmailSender.
type EmailService struct {
	cfg    SMTPSettings
	logger *logrus.Logger
}

// NewEmailService constructs an EmailService.
func NewEmailService(cfg SMTPSettings, logger *logrus.Logger) *EmailService {
	return &EmailService{cfg: cfg, logger: logger}
}

// Enabled reports whether the service will actually attempt SMTP delivery.
// Callers can use this to short-circuit MFA-required logic in dev
// environments where outbound email is intentionally disabled.
func (s *EmailService) Enabled() bool {
	if s == nil {
		return false
	}
	return s.cfg.Enabled && s.cfg.Host != "" && s.cfg.Port > 0 && s.cfg.From != ""
}

// From returns the configured envelope-from address.
func (s *EmailService) From() string {
	if s == nil {
		return ""
	}
	return s.cfg.From
}

// Send delivers msg through the configured SMTP server. Returns an error
// from any stage of the SMTP conversation (dial, auth, MAIL FROM, RCPT TO,
// DATA). Callers are expected to log + surface the error; this function
// does not retry.
func (s *EmailService) Send(msg EmailMessage) error {
	if !s.Enabled() {
		return ErrEmailDisabled
	}
	if strings.TrimSpace(msg.To) == "" {
		return errors.New("email: To address is empty")
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	body := buildRFC5322(s.cfg.From, msg)

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	// ── Implicit TLS path (port 465 typical) ──────────────────────────────
	if s.cfg.UseTLS {
		dialer := &net.Dialer{Timeout: 15 * time.Second}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		})
		if err != nil {
			return fmt.Errorf("email: TLS dial %s: %w", addr, err)
		}
		client, err := smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("email: SMTP NewClient: %w", err)
		}
		defer client.Quit() //nolint:errcheck
		return s.deliverOnClient(client, auth, msg.To, body)
	}

	// ── STARTTLS path (port 587 typical) ──────────────────────────────────
	if s.cfg.StartTLS {
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("email: dial %s: %w", addr, err)
		}
		defer client.Quit() //nolint:errcheck
		if err := client.Hello("localhost"); err != nil {
			return fmt.Errorf("email: HELO: %w", err)
		}
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("email: server does not advertise STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("email: STARTTLS: %w", err)
		}
		return s.deliverOnClient(client, auth, msg.To, body)
	}

	// ── Plain (TEST ONLY) ─────────────────────────────────────────────────
	// Most SMTP relays will refuse plain auth; we still honour the call so
	// local test servers (e.g. mailhog) can be used without TLS.
	return smtp.SendMail(addr, auth, addressOnly(s.cfg.From), []string{msg.To}, body)
}

// deliverOnClient runs auth + MAIL/RCPT/DATA on an already-connected SMTP
// client. Used by both TLS paths.
func (s *EmailService) deliverOnClient(client *smtp.Client, auth smtp.Auth, to string, body []byte) error {
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email: AUTH: %w", err)
		}
	}
	if err := client.Mail(addressOnly(s.cfg.From)); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close body: %w", err)
	}
	return nil
}

// buildRFC5322 produces a minimal multipart/alternative MIME message so both
// HTML- and text-capable clients render correctly. We do not attempt to
// support attachments — MFA codes don't need them.
func buildRFC5322(from string, m EmailMessage) []byte {
	var sb strings.Builder
	boundary := "edr-mfa-" + fmt.Sprintf("%d", time.Now().UnixNano())

	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + m.To + "\r\n")
	sb.WriteString("Subject: " + m.Subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	sb.WriteString(`Content-Type: multipart/alternative; boundary="` + boundary + `"` + "\r\n")
	sb.WriteString("\r\n")

	text := m.Text
	if text == "" {
		text = stripTags(m.HTML)
	}
	if text != "" {
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		sb.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		sb.WriteString(text)
		sb.WriteString("\r\n")
	}
	if m.HTML != "" {
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		sb.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
		sb.WriteString(m.HTML)
		sb.WriteString("\r\n")
	}
	sb.WriteString("--" + boundary + "--\r\n")
	return []byte(sb.String())
}

// stripTags is a deliberately tiny HTML→text fallback so messages still have
// a plaintext part when the caller only supplies HTML. It is NOT a sanitiser.
func stripTags(s string) string {
	out := make([]rune, 0, len(s))
	in := false
	for _, r := range s {
		switch r {
		case '<':
			in = true
		case '>':
			in = false
		default:
			if !in {
				out = append(out, r)
			}
		}
	}
	return strings.TrimSpace(string(out))
}

// addressOnly extracts the bare email portion from a From-header value such
// as `"Protosoft" <no-reply@protosoft.cloud>`. The MAIL FROM SMTP command
// wants only the address, never the display name.
func addressOnly(from string) string {
	from = strings.TrimSpace(from)
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.LastIndex(from, ">"); j > i {
			return from[i+1 : j]
		}
	}
	return from
}
