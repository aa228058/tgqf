package auth

import (
    "encoding/json"
    "testing"
)

func TestValidateSystemConfig(t *testing.T) {
    tests := []struct {
        name string
        key string
        raw string
        want any
        wantErr bool
    }{
        {"registration enabled", "registration_enabled", "false", false, false},
        {"account limit minimum", "default_account_limit", "1", 1, false},
        {"account limit maximum", "default_account_limit", "10000", 10000, false},
        {"session ttl", "auth_session_ttl_hours", "24", 24, false},
        {"unknown key", "DATABASE_URL", "`secret`", nil, true},
        {"wrong boolean type", "registration_enabled", "1", nil, true},
        {"fractional integer", "default_account_limit", "1.5", nil, true},
        {"too small", "default_account_limit", "0", nil, true},
        {"too large", "auth_session_ttl_hours", "721", nil, true},
        {"string injection", "registration_enabled", "`true`", nil, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := validateSystemConfig(tt.key, json.RawMessage(tt.raw))
            if (err != nil) != tt.wantErr { t.Fatalf("error=%v, wantErr=%v", err, tt.wantErr) }
            if !tt.wantErr && got != tt.want { t.Fatalf("value=%#v, want %#v", got, tt.want) }
        })
    }
}
