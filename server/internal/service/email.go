package service

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/resend/resend-go/v2"
)

// maxSubjectFieldRunes bounds how much user-controlled text (workspace name,
// inviter name) can land in an email Subject. Prevents attackers from stuffing
// a full phishing pitch into a workspace name that gets sent from our domain.
const maxSubjectFieldRunes = 60
const (
	smtpTLSModeStartTLS   = "starttls"
	smtpTLSModeImplicit   = "ssl"
	smtpTLSModeNone       = "none"
	defaultSMTPPortSSL    = "465"
	defaultSMTPPortTLS    = "587"
	defaultFallbackSender = "noreply@multica.ai"
)

type EmailService struct {
	resendClient *resend.Client
	smtpSender   *smtpSender
	fromEmail    string
}

type smtpSender struct {
	host               string
	port               string
	username           string
	password           string
	tlsMode            string
	insecureSkipVerify bool
}

func NewEmailService() *EmailService {
	apiKey := os.Getenv("RESEND_API_KEY")
	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	smtpPort := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	smtpTLSMode := parseSMTPTLSMode(os.Getenv("SMTP_TLS_MODE"), smtpPort)

	resendFrom := strings.TrimSpace(os.Getenv("RESEND_FROM_EMAIL"))
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM_EMAIL"))
	from := firstNonEmpty(smtpFrom, resendFrom, defaultFallbackSender)

	svc := &EmailService{
		fromEmail: from,
	}

	// Priority: SMTP > Resend > stdout fallback.
	if smtpHost != "" {
		if smtpPort == "" {
			if smtpTLSMode == smtpTLSModeImplicit {
				smtpPort = defaultSMTPPortSSL
			} else {
				smtpPort = defaultSMTPPortTLS
			}
		}
		svc.smtpSender = &smtpSender{
			host:               smtpHost,
			port:               smtpPort,
			username:           strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
			password:           os.Getenv("SMTP_PASSWORD"),
			tlsMode:            smtpTLSMode,
			insecureSkipVerify: parseBoolEnv(os.Getenv("SMTP_INSECURE_SKIP_VERIFY")),
		}
		return svc
	}

	if strings.TrimSpace(apiKey) != "" {
		svc.resendClient = resend.NewClient(apiKey)
	}

	return svc
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBoolEnv(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseSMTPTLSMode(rawMode, rawPort string) string {
	mode := strings.ToLower(strings.TrimSpace(rawMode))
	port := strings.TrimSpace(rawPort)

	switch mode {
	case "", "auto":
		if port == defaultSMTPPortSSL {
			return smtpTLSModeImplicit
		}
		return smtpTLSModeStartTLS
	case "starttls":
		return smtpTLSModeStartTLS
	case "ssl", "tls", "smtps", "implicit":
		return smtpTLSModeImplicit
	case "none", "plain":
		return smtpTLSModeNone
	default:
		return smtpTLSModeStartTLS
	}
}

func sanitizeSMTPHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\n", "")
}

func buildSMTPMessage(from string, to []string, subject, htmlBody string) ([]byte, error) {
	if len(to) == 0 {
		return nil, errors.New("smtp message must include at least one recipient")
	}

	var encodedBody bytes.Buffer
	qp := quotedprintable.NewWriter(&encodedBody)
	if _, err := qp.Write([]byte(htmlBody)); err != nil {
		return nil, fmt.Errorf("encode html body: %w", err)
	}
	if err := qp.Close(); err != nil {
		return nil, fmt.Errorf("close body encoder: %w", err)
	}

	var msg bytes.Buffer
	headers := []string{
		"From: " + sanitizeSMTPHeader(from),
		"To: " + sanitizeSMTPHeader(strings.Join(to, ", ")),
		"Subject: " + sanitizeSMTPHeader(mime.QEncoding.Encode("UTF-8", subject)),
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
	}
	for _, header := range headers {
		msg.WriteString(header + "\r\n")
	}
	msg.WriteString("\r\n")
	msg.Write(encodedBody.Bytes())

	return msg.Bytes(), nil
}

func parseAddressList(values []string) ([]*mail.Address, []string, error) {
	if len(values) == 0 {
		return nil, nil, errors.New("missing recipient")
	}

	addrs := make([]*mail.Address, 0, len(values))
	envelopes := make([]string, 0, len(values))
	for _, raw := range values {
		addr, err := mail.ParseAddress(strings.TrimSpace(raw))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid recipient address %q: %w", raw, err)
		}
		addrs = append(addrs, addr)
		envelopes = append(envelopes, addr.Address)
	}
	return addrs, envelopes, nil
}

func (s *smtpSender) newClient() (*smtp.Client, error) {
	addr := net.JoinHostPort(s.host, s.port)
	tlsConfig := &tls.Config{
		ServerName:         s.host,
		InsecureSkipVerify: s.insecureSkipVerify,
	}

	switch s.tlsMode {
	case smtpTLSModeImplicit:
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial %s: %w", addr, err)
		}
		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("smtp create client: %w", err)
		}
		return client, nil
	case smtpTLSModeStartTLS, smtpTLSModeNone:
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("smtp dial %s: %w", addr, err)
		}
		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("smtp create client: %w", err)
		}

		if s.tlsMode == smtpTLSModeStartTLS {
			if ok, _ := client.Extension("STARTTLS"); !ok {
				client.Close()
				return nil, errors.New("smtp server does not support STARTTLS")
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				client.Close()
				return nil, fmt.Errorf("smtp STARTTLS: %w", err)
			}
		}

		return client, nil
	default:
		return nil, fmt.Errorf("unsupported SMTP_TLS_MODE %q", s.tlsMode)
	}
}

func (s *smtpSender) Send(params *resend.SendEmailRequest) error {
	if params == nil {
		return errors.New("email params are required")
	}

	from, err := mail.ParseAddress(strings.TrimSpace(params.From))
	if err != nil {
		return fmt.Errorf("invalid from address %q: %w", params.From, err)
	}
	toAddrs, envelopeRecipients, err := parseAddressList(params.To)
	if err != nil {
		return err
	}

	headerRecipients := make([]string, len(toAddrs))
	for i, addr := range toAddrs {
		headerRecipients[i] = addr.String()
	}

	msg, err := buildSMTPMessage(from.String(), headerRecipients, params.Subject, params.Html)
	if err != nil {
		return err
	}

	client, err := s.newClient()
	if err != nil {
		return err
	}
	defer client.Close()

	if s.username != "" || s.password != "" {
		if s.username == "" || s.password == "" {
			return errors.New("SMTP_USERNAME and SMTP_PASSWORD must both be set when SMTP auth is enabled")
		}
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("smtp MAIL FROM failed: %w", err)
	}
	for _, rcpt := range envelopeRecipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO failed for %s: %w", rcpt, err)
		}
	}

	dataWriter, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA failed: %w", err)
	}
	if _, err := dataWriter.Write(msg); err != nil {
		_ = dataWriter.Close()
		return fmt.Errorf("smtp write message failed: %w", err)
	}
	if err := dataWriter.Close(); err != nil {
		return fmt.Errorf("smtp close DATA failed: %w", err)
	}

	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp QUIT failed: %w", err)
	}

	return nil
}

func (s *EmailService) send(params *resend.SendEmailRequest) error {
	if s.smtpSender != nil {
		return s.smtpSender.Send(params)
	}
	if s.resendClient != nil {
		_, err := s.resendClient.Emails.Send(params)
		return err
	}
	return nil
}

// SendVerificationCode sends a one-time login code. The code is server-generated
// (6-digit numeric) so no user-controlled text reaches the email body here.
// If that ever changes, escape the user-controlled fields the same way
// SendInvitationEmail does.
func (s *EmailService) SendVerificationCode(to, code string) error {
	params := &resend.SendEmailRequest{
		From:    s.fromEmail,
		To:      []string{to},
		Subject: "Your Multica verification code",
		Html: fmt.Sprintf(
			`<div style="font-family: sans-serif; max-width: 400px; margin: 0 auto;">
				<h2>Your verification code</h2>
				<p style="font-size: 32px; font-weight: bold; letter-spacing: 8px; margin: 24px 0;">%s</p>
				<p>This code expires in 10 minutes.</p>
				<p style="color: #666; font-size: 14px;">If you didn't request this code, you can safely ignore this email.</p>
			</div>`, code),
	}

	if s.smtpSender == nil && s.resendClient == nil {
		fmt.Printf("[DEV] Verification code for %s: %s\n", to, code)
		return nil
	}

	return s.send(params)
}

// SendInvitationEmail notifies the invitee that they have been invited to a workspace.
// invitationID is included in the URL so the email deep-links to /invite/{id}.
func (s *EmailService) SendInvitationEmail(to, inviterName, workspaceName, invitationID string) error {
	appURL := strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN"))
	if appURL == "" {
		appURL = "https://app.multica.ai"
	}
	inviteURL := fmt.Sprintf("%s/invite/%s", appURL, invitationID)

	if s.smtpSender == nil && s.resendClient == nil {
		fmt.Printf("[DEV] Invitation email to %s: %s invited you to %s — %s\n", to, inviterName, workspaceName, inviteURL)
		return nil
	}

	params := buildInvitationParams(s.fromEmail, to, inviterName, workspaceName, inviteURL)
	return s.send(params)
}

// buildInvitationParams assembles the Resend request for an invitation email.
// Separated from SendInvitationEmail so the sanitization behavior is unit-testable
// without needing to mock the Resend SDK.
func buildInvitationParams(from, to, inviterName, workspaceName, inviteURL string) *resend.SendEmailRequest {
	safeWorkspace := html.EscapeString(workspaceName)
	safeInviter := html.EscapeString(inviterName)
	subjectInviter := sanitizeSubjectField(inviterName)
	subjectWorkspace := sanitizeSubjectField(workspaceName)

	return &resend.SendEmailRequest{
		From:    from,
		To:      []string{to},
		Subject: fmt.Sprintf("%s invited you to %s on Multica", subjectInviter, subjectWorkspace),
		Html: fmt.Sprintf(
			`<div style="font-family: sans-serif; max-width: 480px; margin: 0 auto;">
				<h2>You're invited to join %s</h2>
				<p><strong>%s</strong> invited you to collaborate in the <strong>%s</strong> workspace on Multica.</p>
				<p style="margin: 24px 0;">
					<a href="%s" style="display: inline-block; padding: 12px 24px; background: #000; color: #fff; text-decoration: none; border-radius: 6px; font-weight: 500;">Accept invitation</a>
				</p>
				<p style="color: #666; font-size: 14px;">You'll need to log in to accept or decline the invitation.</p>
			</div>`, safeWorkspace, safeInviter, safeWorkspace, inviteURL),
	}
}

// sanitizeSubjectField prepares user-controlled text for the email Subject line.
// Subject is not HTML-rendered, so HTML-escaping would leak literal entities
// (e.g. &lt;script&gt;) into the recipient's inbox. Instead strip control
// characters (defense in depth against header-injection-adjacent abuse even
// though Resend also filters CR/LF) and cap length so attackers can't stuff
// a full phishing subject into a workspace name.
func sanitizeSubjectField(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := b.String()
	if utf8.RuneCountInString(cleaned) <= maxSubjectFieldRunes {
		return cleaned
	}
	runes := []rune(cleaned)
	return string(runes[:maxSubjectFieldRunes-1]) + "…"
}
