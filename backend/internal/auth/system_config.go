package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SystemConfig struct {
	Key string `json:"key"`
	Value any `json:"value"`
	Description string `json:"description"`
	UpdatedAt time.Time `json:"updated_at"`
}

type configDefinition struct {
	description string
	kind string
	min int
	max int
}

var systemConfigDefinitions = map[string]configDefinition{
	"registration_enabled": {"是否允许新租户注册", "bool", 0, 0},
	"default_account_limit": {"新租户默认 Telegram 账号额度", "int", 1, 10000},
	"auth_session_ttl_hours": {"登录会话有效期（小时）", "int", 1, 720},
}

func validateSystemConfig(key string, raw json.RawMessage) (any, error) {
	def, ok := systemConfigDefinitions[key]
	if !ok { return nil, fmt.Errorf("unsupported system config key: %s", key) }
	if def.kind == "bool" {
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil { return nil, errors.New("value must be a boolean") }
		return value, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil { return nil, errors.New("value must be an integer") }
	if value < def.min || value > def.max { return nil, fmt.Errorf("value must be between %d and %d", def.min, def.max) }
	return value, nil
}

func (s *Service) ListSystemConfigs(ctx context.Context) ([]SystemConfig, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value,updated_at FROM system_configs ORDER BY key`)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]SystemConfig, 0, len(systemConfigDefinitions))
	for rows.Next() {
		var item SystemConfig
		var raw []byte
		if err := rows.Scan(&item.Key, &raw, &item.UpdatedAt); err != nil { return nil, err }
		def, ok := systemConfigDefinitions[item.Key]
		if !ok { continue }
		if err := json.Unmarshal(raw, &item.Value); err != nil { return nil, fmt.Errorf("decode system config %s: %w", item.Key, err) }
		item.Description = def.description
		items = append(items, item)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return items, nil
}

func (s *Service) UpdateSystemConfig(ctx context.Context, actorID, key string, raw json.RawMessage) (SystemConfig, error) {
	key = strings.TrimSpace(key)
	value, err := validateSystemConfig(key, raw)
	if err != nil { return SystemConfig{}, err }
	encoded, err := json.Marshal(value)
	if err != nil { return SystemConfig{}, err }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return SystemConfig{}, err }
	defer tx.Rollback()
	var previous []byte
	err = tx.QueryRowContext(ctx, `SELECT value FROM system_configs WHERE key=$1 FOR UPDATE`, key).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) { return SystemConfig{}, err }
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO system_configs(key,value,updated_by,updated_at)
		VALUES($1,$2::jsonb,$3,NOW())
		ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_by=EXCLUDED.updated_by,updated_at=NOW()
	`, key, string(encoded), actorID); err != nil { return SystemConfig{}, err }
	details := map[string]any{"key": key, "new_value": value}
	if len(previous) > 0 {
		var oldValue any
		if json.Unmarshal(previous, &oldValue) == nil { details["previous_value"] = oldValue }
	}
	audit, err := json.Marshal(details)
	if err != nil { return SystemConfig{}, err }
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs(tenant_id,actor_user_id,action,resource_type,resource_id,details)
		VALUES(NULL,$1,'admin.system_config.update','system_config',$2,$3::jsonb)
	`, actorID, key, string(audit)); err != nil { return SystemConfig{}, err }
	if err = tx.Commit(); err != nil { return SystemConfig{}, err }
	return SystemConfig{Key:key, Value:value, Description:systemConfigDefinitions[key].description, UpdatedAt:time.Now().UTC()}, nil
}

func (s *Service) SystemConfigBool(ctx context.Context, key string, fallback bool) bool {
	if def, ok := systemConfigDefinitions[key]; !ok || def.kind != "bool" { return fallback }
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM system_configs WHERE key=$1`, key).Scan(&raw); err != nil { return fallback }
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil { return fallback }
	return value
}

func (s *Service) SystemConfigInt(ctx context.Context, key string, fallback int) int {
	def, ok := systemConfigDefinitions[key]
	if !ok || def.kind != "int" { return fallback }
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM system_configs WHERE key=$1`, key).Scan(&raw); err != nil { return fallback }
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value < def.min || value > def.max { return fallback }
	return value
}
