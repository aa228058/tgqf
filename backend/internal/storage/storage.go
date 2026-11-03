package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	DB    *sql.DB
	Redis *redis.Client
}

func Open(ctx context.Context, databaseURL, redisURL string) (*Store, error) {
	pgConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL URL: %w", err)
	}
	db := stdlib.OpenDB(*pgConfig)
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	cache := redis.NewClient(redisOptions)

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = cache.Close()
		_ = db.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	if err := cache.Ping(pingCtx).Err(); err != nil {
		_ = cache.Close()
		_ = db.Close()
		return nil, fmt.Errorf("connect Redis: %w", err)
	}
	store := &Store{DB: db, Redis: cache}
	if err := store.Migrate(pingCtx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("PostgreSQL: %w", err)
	}
	if err := s.Redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	redisErr := s.Redis.Close()
	dbErr := s.DB.Close()
	if redisErr != nil {
		return redisErr
	}
	return dbErr
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	entries, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		var exists bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}
		script, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(script)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, version)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}
	return nil
}
