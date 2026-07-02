package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr            string
	DatabaseURL     string
	SessionSecret   string
	CollabURL       string
	CollabSecret    string
	DevHTTP         bool
	DataDir         string
	TypstBin        string
	PublicURL       string
	OIDCIssuer      string
	OIDCClientID    string
	OIDCClientSecret string
	OIDCScopes      string
	CompileTimeout  int // seconds

	// Abuse quotas (0 = unlimited). Admins are always exempt.
	MaxProjectsPerUser  int
	MaxAssetBytesPerUser int64

	// SignupAllowlist restricts which emails may register (comma/space
	// separated; each entry is a domain like "ics.red" or an exact address
	// like "me@ics.red"). Empty allows any email.
	SignupAllowlist string

	// SMTP (e.g. Amazon SES SMTP): when configured, local signups must verify
	// their email before they can log in.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	// RequireEmailVerification overrides the default (verify when SMTP set).
	requireEmailVerification string
}

func FromEnv() (*Config, error) {
	c := &Config{
		Addr:            getenv("APP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		SessionSecret:   os.Getenv("SESSION_SECRET"),
		CollabURL:       getenv("COLLAB_URL", "http://localhost:8090"),
		CollabSecret:    os.Getenv("COLLAB_SECRET"),
		DevHTTP:         boolenv("APP_DEV_HTTP", false),
		DataDir:         getenv("DATA_DIR", "./data"),
		TypstBin:        getenv("TYPST_BIN", "typst"),
		PublicURL:       getenv("PUBLIC_URL", "http://localhost:8080"),
		OIDCIssuer:      os.Getenv("OIDC_ISSUER"),
		OIDCClientID:    os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCScopes:      getenv("OIDC_SCOPES", "openid profile email"),
		CompileTimeout:  intenv("COMPILE_TIMEOUT_SECONDS", 30),
		MaxProjectsPerUser:   intenv("MAX_PROJECTS_PER_USER", 500),
		MaxAssetBytesPerUser: int64(intenv("MAX_ASSET_MB_PER_USER", 1024)) << 20,
		SignupAllowlist: os.Getenv("SIGNUP_ALLOWLIST"),
		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        intenv("SMTP_PORT", 587),
		SMTPUsername:    os.Getenv("SMTP_USERNAME"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		SMTPFromName:    getenv("SMTP_FROM_NAME", "TypstPad"),
		requireEmailVerification: os.Getenv("REQUIRE_EMAIL_VERIFICATION"),
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if c.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required")
	}
	if c.CollabSecret == "" {
		return nil, fmt.Errorf("COLLAB_SECRET is required")
	}
	return c, nil
}

func (c *Config) OIDCEnabled() bool {
	return c.OIDCIssuer != "" && c.OIDCClientID != "" && c.OIDCClientSecret != ""
}

func (c *Config) SMTPEnabled() bool {
	return c.SMTPHost != "" && c.SMTPFrom != ""
}

// EmailVerificationRequired reports whether local signups must verify their
// email. Defaults to true when SMTP is configured; REQUIRE_EMAIL_VERIFICATION
// (true/false) overrides.
func (c *Config) EmailVerificationRequired() bool {
	if c.requireEmailVerification != "" {
		if b, err := strconv.ParseBool(c.requireEmailVerification); err == nil {
			return b && c.SMTPEnabled()
		}
	}
	return c.SMTPEnabled()
}

// EmailAllowed checks an email against SignupAllowlist (case-insensitive).
func (c *Config) EmailAllowed(email string) bool {
	return EmailMatchesAllowlist(c.SignupAllowlist, email)
}

// EmailMatchesAllowlist reports whether email is permitted by a comma/space
// separated allowlist of domains ("ics.red") and/or exact addresses
// ("me@ics.red"). An empty allowlist permits everything.
func EmailMatchesAllowlist(allowlist, email string) bool {
	if strings.TrimSpace(allowlist) == "" {
		return true
	}
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, entry := range strings.FieldsFunc(allowlist, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		entry = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(entry, "@")))
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "@") {
			if entry == email {
				return true
			}
		} else if entry == domain {
			return true
		}
	}
	return false
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func boolenv(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func intenv(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
