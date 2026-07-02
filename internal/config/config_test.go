package config

import "testing"

func TestEmailAllowed(t *testing.T) {
	cases := []struct {
		allowlist string
		email     string
		want      bool
	}{
		{"", "anyone@example.com", true},           // empty = allow all
		{"ics.red", "me@ics.red", true},            // domain match
		{"ics.red", "me@ICS.RED", true},            // case-insensitive
		{"ics.red", "me@other.com", false},         // domain mismatch
		{"ics.red, purdue.edu", "x@purdue.edu", true}, // multi
		{"@ics.red", "me@ics.red", true},           // leading @ tolerated
		{"boss@ics.red", "boss@ics.red", true},     // exact address
		{"boss@ics.red", "other@ics.red", false},   // exact address, no domain fallthrough
		{"ics.red", "notanemail", false},           // malformed
	}
	for _, c := range cases {
		cfg := &Config{SignupAllowlist: c.allowlist}
		if got := cfg.EmailAllowed(c.email); got != c.want {
			t.Errorf("allowlist=%q email=%q: got %v want %v", c.allowlist, c.email, got, c.want)
		}
	}
}

func TestEmailVerificationRequired(t *testing.T) {
	smtp := &Config{SMTPHost: "smtp.example.com", SMTPFrom: "no-reply@example.com"}
	if !smtp.EmailVerificationRequired() {
		t.Error("verification should default on when SMTP configured")
	}
	off := &Config{}
	if off.EmailVerificationRequired() {
		t.Error("verification should be off with no SMTP")
	}
	forcedOff := &Config{SMTPHost: "smtp.example.com", SMTPFrom: "x@y.z", requireEmailVerification: "false"}
	if forcedOff.EmailVerificationRequired() {
		t.Error("REQUIRE_EMAIL_VERIFICATION=false should disable")
	}
}
