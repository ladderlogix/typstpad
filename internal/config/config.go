package config

import (
	"fmt"
	"os"
	"strconv"
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
