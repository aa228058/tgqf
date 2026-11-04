package safelog

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestJSONHandlerRedactsSensitiveFieldsAndEmbeddedCredentials(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewJSONHandler(&output, nil))
	logger.Error("connection failed",
		"password", "plain-password-value",
		"api_hash", "telegram-api-hash",
		"error", errors.New("dial postgres://tgqf:db-password@db:5432/app?sslmode=disable with token=access-token"),
		"details", "Authorization: Bearer bearer-secret-value",
	)
	got := output.String()
	for _, secret := range []string{"plain-password-value", "telegram-api-hash", "db-password", "access-token", "bearer-secret-value"} {
		if strings.Contains(got, secret) {
			t.Fatalf("log output leaked %q: %s", secret, got)
		}
	}
	if strings.Count(got, redacted) < 4 {
		t.Fatalf("expected redaction markers in output: %s", got)
	}
	if !strings.Contains(got, "connection failed") {
		t.Fatalf("non-sensitive log context was lost: %s", got)
	}
}

func TestRedactText(t *testing.T) {
	got := RedactText("redis://:redis-password@cache:6379/0 https://proxy-user:proxy-password@proxy password=another-secret Bearer bearer-token")
	for _, secret := range []string{"redis-password", "proxy-password", "another-secret", "bearer-token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("RedactText leaked %q: %s", secret, got)
		}
	}
}
