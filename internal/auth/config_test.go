package auth

import (
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	values := map[string]string{"GOOGLE_CLIENT_ID": "client", "GOOGLE_CLIENT_SECRET": "secret", "GOOGLE_REDIRECT_URI": "http://127.0.0.1:5173/auth/google/callback", "AUTH_ALLOWED_EMAILS": " Buyer@Example.test, second@example.test "}
	c, err := LoadConfig(func(key string) string { return values[key] })
	if err != nil || len(c.AllowedEmails) != 2 || c.AllowedEmails[0] != "buyer@example.test" {
		t.Fatalf("unexpected configuration: %v", err)
	}
	for _, item := range []struct{ key, value string }{
		{"GOOGLE_CLIENT_ID", ""}, {"GOOGLE_CLIENT_SECRET", ""}, {"AUTH_ALLOWED_EMAILS", ""}, {"AUTH_ALLOWED_EMAILS", "not-an-email"}, {"AUTH_ALLOWED_EMAILS", "Buyer <buyer@example.test>"},
		{"GOOGLE_REDIRECT_URI", "http://example.test/auth/google/callback"}, {"GOOGLE_REDIRECT_URI", "https://example.test/wrong"}, {"GOOGLE_REDIRECT_URI", "https://example.test/auth/google/callback?next=evil"}, {"GOOGLE_REDIRECT_URI", "https://user@example.test/auth/google/callback"},
	} {
		t.Run(item.key+item.value, func(t *testing.T) {
			bad := make(map[string]string)
			for k, v := range values {
				bad[k] = v
			}
			bad[item.key] = item.value
			_, err := LoadConfig(func(key string) string { return bad[key] })
			if err == nil {
				t.Fatal("accepted unsafe configuration")
			}
			if strings.Contains(err.Error(), "secret") && err.Error() != "Google client ID and secret are required" {
				t.Fatal("error may expose credentials")
			}
		})
	}
	values["GOOGLE_REDIRECT_URI"] = "https://app.example.test/auth/google/callback"
	if _, err := LoadConfig(func(key string) string { return values[key] }); err != nil {
		t.Fatal(err)
	}
}
