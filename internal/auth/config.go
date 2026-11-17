// Package auth owns Google login, account access, and database-backed sessions.
package auth

import (
	"errors"
	"net/mail"
	"net/url"
	"strings"
)

type Config struct {
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	AllowedEmails []string
}

// LoadConfig reads server-only settings. An empty allowlist denies startup.
func LoadConfig(getenv func(string) string) (Config, error) {
	c := Config{ClientID: strings.TrimSpace(getenv("GOOGLE_CLIENT_ID")), ClientSecret: strings.TrimSpace(getenv("GOOGLE_CLIENT_SECRET")), RedirectURI: strings.TrimSpace(getenv("GOOGLE_REDIRECT_URI"))}
	for _, email := range strings.Split(getenv("AUTH_ALLOWED_EMAILS"), ",") {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			c.AllowedEmails = append(c.AllowedEmails, email)
		}
	}
	return c, c.validate()
}

func (c Config) validate() error {
	if c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("Google client ID and secret are required")
	}
	u, err := url.Parse(c.RedirectURI)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/auth/google/callback" {
		return errors.New("GOOGLE_REDIRECT_URI must be an absolute /auth/google/callback URL")
	}
	loopback := u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return errors.New("login requires HTTPS except on loopback")
	}
	if len(c.AllowedEmails) == 0 {
		return errors.New("AUTH_ALLOWED_EMAILS must contain at least one account")
	}
	for _, email := range c.AllowedEmails {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email || email != strings.ToLower(strings.TrimSpace(email)) {
			return errors.New("AUTH_ALLOWED_EMAILS must contain plain, lowercase email addresses")
		}
	}
	return nil
}
