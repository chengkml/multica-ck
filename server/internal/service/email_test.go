package service

import (
	"strings"
	"testing"
)

func TestSanitizeSubjectField(t *testing.T) {
	long := strings.Repeat("a", 100)
	longRunes := strings.Repeat("深", 100)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain ascii", "Acme", "Acme"},
		{"strips newline", "Acme\nEvil", "AcmeEvil"},
		{"strips crlf header-style", "Acme\r\nBcc: evil@example.com", "AcmeBcc: evil@example.com"},
		{"strips tab", "Acme\tTeam", "AcmeTeam"},
		{"strips unicode control", "Acme\x07Beep", "AcmeBeep"},
		{"preserves non-ascii", "深度学习工作区", "深度学习工作区"},
		{"preserves emoji", "Team 🚀", "Team 🚀"},
		{"truncates long ascii", long, strings.Repeat("a", maxSubjectFieldRunes-1) + "…"},
		{"truncates rune-aware", longRunes, strings.Repeat("深", maxSubjectFieldRunes-1) + "…"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSubjectField(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeSubjectField(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildInvitationParams_EscapesHTMLInBody(t *testing.T) {
	tests := []struct {
		name          string
		inviter       string
		workspace     string
		wantInBody    []string
		wantNotInBody []string
	}{
		{
			name:      "escapes script tag in inviter",
			inviter:   "<script>alert(1)</script>",
			workspace: "Acme",
			wantInBody: []string{
				"&lt;script&gt;alert(1)&lt;/script&gt;",
			},
			wantNotInBody: []string{
				"<script>alert(1)</script>",
			},
		},
		{
			name:      "escapes attribute-break payload in inviter",
			inviter:   `Alice" onclick="evil()`,
			workspace: "Acme",
			wantNotInBody: []string{
				`Alice" onclick="evil()`,
			},
		},
		{
			name:      "escapes anchor tag in workspace",
			inviter:   "Alice",
			workspace: `<a href="https://evil.example">Click</a>`,
			wantInBody: []string{
				"&lt;a href=",
				"&gt;Click&lt;/a&gt;",
			},
			wantNotInBody: []string{
				`<a href="https://evil.example">Click</a>`,
			},
		},
		{
			name:      "benign text unchanged",
			inviter:   "Alice",
			workspace: "Acme",
			wantInBody: []string{
				"Alice",
				"Acme",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildInvitationParams(
				"noreply@multica.ai",
				"invitee@example.com",
				tt.inviter,
				tt.workspace,
				"https://app.multica.ai/invite/abc-123",
			)
			for _, needle := range tt.wantInBody {
				if !strings.Contains(p.Html, needle) {
					t.Errorf("body missing %q\nbody: %s", needle, p.Html)
				}
			}
			for _, needle := range tt.wantNotInBody {
				if strings.Contains(p.Html, needle) {
					t.Errorf("body should not contain raw %q\nbody: %s", needle, p.Html)
				}
			}
		})
	}
}

func TestBuildInvitationParams_SubjectStripsControls(t *testing.T) {
	p := buildInvitationParams(
		"noreply@multica.ai",
		"invitee@example.com",
		"Alice\r\n",
		"Acme\t",
		"https://app.multica.ai/invite/abc",
	)
	if strings.ContainsAny(p.Subject, "\r\n\t") {
		t.Errorf("subject still contains control characters: %q", p.Subject)
	}
	if p.Subject != "Alice invited you to Acme on Multica" {
		t.Errorf("unexpected subject: %q", p.Subject)
	}
}

func TestBuildInvitationParams_SubjectNotHTMLEscaped(t *testing.T) {
	// Subject is not HTML-rendered; entities would render literally in inboxes.
	p := buildInvitationParams(
		"noreply@multica.ai",
		"invitee@example.com",
		"Alice",
		"Acme & Co.",
		"https://app.multica.ai/invite/abc",
	)
	if strings.Contains(p.Subject, "&amp;") {
		t.Errorf("subject should not be HTML-escaped, got %q", p.Subject)
	}
	if !strings.Contains(p.Subject, "Acme & Co.") {
		t.Errorf("subject missing literal ampersand: %q", p.Subject)
	}
}

func TestBuildInvitationParams_SubjectTruncated(t *testing.T) {
	longWorkspace := strings.Repeat("A", 200)
	p := buildInvitationParams(
		"noreply@multica.ai",
		"invitee@example.com",
		"Alice",
		longWorkspace,
		"https://app.multica.ai/invite/abc",
	)
	// Template: "Alice invited you to <ws> on Multica"
	// ws is capped at maxSubjectFieldRunes; overall subject should also be bounded.
	maxExpected := len("Alice invited you to  on Multica") + maxSubjectFieldRunes
	if runes := len([]rune(p.Subject)); runes > maxExpected {
		t.Errorf("subject not bounded: %d runes, max %d: %q", runes, maxExpected, p.Subject)
	}
	if !strings.Contains(p.Subject, "…") {
		t.Errorf("truncated subject should contain ellipsis marker: %q", p.Subject)
	}
}

func TestBuildInvitationParams_ToAndFromPassedThrough(t *testing.T) {
	p := buildInvitationParams(
		"noreply@multica.ai",
		"invitee@example.com",
		"Alice",
		"Acme",
		"https://app.multica.ai/invite/abc",
	)
	if p.From != "noreply@multica.ai" {
		t.Errorf("From = %q", p.From)
	}
	if len(p.To) != 1 || p.To[0] != "invitee@example.com" {
		t.Errorf("To = %v", p.To)
	}
	if !strings.Contains(p.Html, "https://app.multica.ai/invite/abc") {
		t.Errorf("body missing invite URL: %s", p.Html)
	}
}

func TestParseSMTPTLSMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		port string
		want string
	}{
		{name: "default uses starttls", mode: "", port: "", want: smtpTLSModeStartTLS},
		{name: "default port 465 uses ssl", mode: "", port: "465", want: smtpTLSModeImplicit},
		{name: "explicit starttls", mode: "starttls", port: "25", want: smtpTLSModeStartTLS},
		{name: "explicit ssl alias", mode: "tls", port: "587", want: smtpTLSModeImplicit},
		{name: "explicit none", mode: "none", port: "587", want: smtpTLSModeNone},
		{name: "invalid mode falls back starttls", mode: "bad-mode", port: "587", want: smtpTLSModeStartTLS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSMTPTLSMode(tt.mode, tt.port); got != tt.want {
				t.Fatalf("parseSMTPTLSMode(%q, %q) = %q, want %q", tt.mode, tt.port, got, tt.want)
			}
		})
	}
}

func TestBuildSMTPMessage(t *testing.T) {
	msg, err := buildSMTPMessage(
		"Sender <sender@example.com>",
		[]string{"Alice <alice@example.com>", "bob@example.com"},
		"邀请 Acme & Co.",
		"<h1>Hello SMTP</h1>",
	)
	if err != nil {
		t.Fatalf("buildSMTPMessage returned error: %v", err)
	}

	raw := string(msg)
	mustContain := []string{
		"From: Sender <sender@example.com>\r\n",
		"To: Alice <alice@example.com>, bob@example.com\r\n",
		"Subject: =?UTF-8?",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/html; charset=UTF-8\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
		"\r\n<h1>Hello SMTP</h1>",
	}
	for _, needle := range mustContain {
		if !strings.Contains(raw, needle) {
			t.Fatalf("message missing %q\nfull message:\n%s", needle, raw)
		}
	}
}

func TestBuildSMTPMessage_StripsHeaderInjection(t *testing.T) {
	msg, err := buildSMTPMessage(
		"sender@example.com\r\nBcc:evil@example.com",
		[]string{"victim@example.com"},
		"safe\r\nInjected: bad",
		"ok",
	)
	if err != nil {
		t.Fatalf("buildSMTPMessage returned error: %v", err)
	}

	raw := string(msg)
	if strings.Contains(raw, "\r\nBcc:evil@example.com") {
		t.Fatalf("raw message should not contain injected Bcc header: %s", raw)
	}
	if strings.Contains(raw, "\r\nInjected: bad") {
		t.Fatalf("raw message should not contain injected header from subject: %s", raw)
	}
}

func TestNewEmailServiceProviderSelection(t *testing.T) {
	t.Run("uses smtp when smtp configured", func(t *testing.T) {
		t.Setenv("SMTP_HOST", "smtp.example.com")
		t.Setenv("SMTP_PORT", "587")
		t.Setenv("SMTP_TLS_MODE", "starttls")
		t.Setenv("RESEND_API_KEY", "re_should_be_ignored")

		svc := NewEmailService()
		if svc.smtpSender == nil {
			t.Fatalf("expected smtp sender to be configured")
		}
		if svc.resendClient != nil {
			t.Fatalf("expected resend client to be nil when SMTP is configured")
		}
	})

	t.Run("uses resend when smtp missing", func(t *testing.T) {
		t.Setenv("SMTP_HOST", "")
		t.Setenv("RESEND_API_KEY", "re_configured")

		svc := NewEmailService()
		if svc.smtpSender != nil {
			t.Fatalf("expected smtp sender to be nil")
		}
		if svc.resendClient == nil {
			t.Fatalf("expected resend client to be configured")
		}
	})

	t.Run("uses default from email", func(t *testing.T) {
		t.Setenv("SMTP_HOST", "")
		t.Setenv("SMTP_FROM_EMAIL", "")
		t.Setenv("RESEND_FROM_EMAIL", "")
		t.Setenv("RESEND_API_KEY", "")

		svc := NewEmailService()
		if svc.fromEmail != defaultFallbackSender {
			t.Fatalf("fromEmail = %q, want %q", svc.fromEmail, defaultFallbackSender)
		}
	})
}
