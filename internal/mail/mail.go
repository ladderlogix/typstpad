// Package mail sends transactional email over SMTP (works with Amazon SES SMTP
// credentials, or any SMTP relay). Uses the stdlib net/smtp with STARTTLS.
package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPConfig supplies live SMTP settings (implemented by settings.Service so
// changes made in the admin UI take effect without a restart).
type SMTPConfig interface {
	SMTPHost() string
	SMTPPort() int
	SMTPUsername() string
	SMTPPassword() string
	SMTPFrom() string
	SMTPFromName() string
	SMTPEnabled() bool
}

type Mailer struct {
	cfg SMTPConfig
}

func New(cfg SMTPConfig) *Mailer {
	return &Mailer{cfg: cfg}
}

func (m *Mailer) Enabled() bool { return m.cfg.SMTPEnabled() }

// SendVerification emails a verification link to a newly registered user.
func (m *Mailer) SendVerification(toEmail, toName, link string) error {
	subject := "Verify your TypstPad email"
	text := fmt.Sprintf(
		"Hi %s,\n\nConfirm your email address to finish creating your TypstPad account:\n\n%s\n\n"+
			"This link expires in 24 hours. If you didn't sign up, you can ignore this message.\n",
		toName, link)
	html := fmt.Sprintf(
		`<p>Hi %s,</p><p>Confirm your email address to finish creating your TypstPad account:</p>`+
			`<p><a href="%s">Verify my email</a></p>`+
			`<p style="color:#666;font-size:13px">Or paste this link: %s<br>This link expires in 24 hours. `+
			`If you didn't sign up, you can ignore this message.</p>`,
		toName, link, link)
	return m.send(toEmail, subject, text, html)
}

// SendPasswordReset emails a password-reset link.
func (m *Mailer) SendPasswordReset(toEmail, toName, link string) error {
	subject := "Reset your TypstPad password"
	text := fmt.Sprintf(
		"Hi %s,\n\nWe received a request to reset your TypstPad password. Use this link:\n\n%s\n\n"+
			"This link expires in 1 hour. If you didn't request this, you can ignore this message.\n",
		toName, link)
	html := fmt.Sprintf(
		`<p>Hi %s,</p><p>We received a request to reset your TypstPad password.</p>`+
			`<p><a href="%s">Reset my password</a></p>`+
			`<p style="color:#666;font-size:13px">Or paste this link: %s<br>This link expires in 1 hour. `+
			`If you didn't request this, you can ignore this message.</p>`,
		toName, link, link)
	return m.send(toEmail, subject, text, html)
}

// SendNotification emails a single feed event (a mention or a share).
func (m *Mailer) SendNotification(toEmail, toName, subject, line, link string) error {
	text := fmt.Sprintf("Hi %s,\n\n%s\n\n%s\n\nManage this in TypstPad.\n", toName, line, link)
	html := fmt.Sprintf(
		`<p>Hi %s,</p><p>%s</p><p><a href="%s">Open in TypstPad</a></p>`+
			`<p style="color:#666;font-size:13px">Or paste this link: %s</p>`,
		toName, line, link, link)
	return m.send(toEmail, subject, text, html)
}

func (m *Mailer) send(to, subject, text, html string) error {
	c := m.cfg
	from := c.SMTPFrom()
	fromHeader := from
	if c.SMTPFromName() != "" {
		fromHeader = fmt.Sprintf("%s <%s>", c.SMTPFromName(), from)
	}

	boundary := "tp_boundary_9f3c1e"
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", fromHeader)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", boundary, text)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n", boundary, html)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)

	addr := fmt.Sprintf("%s:%d", c.SMTPHost(), c.SMTPPort())
	var auth smtp.Auth
	if c.SMTPUsername() != "" {
		auth = smtp.PlainAuth("", c.SMTPUsername(), c.SMTPPassword(), c.SMTPHost())
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(b.String()))
}
