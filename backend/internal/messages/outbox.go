package messages

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// PublishPendingMessageEvents forwards committed outbox rows to tenant-scoped
// Redis Streams. Rows are marked published only after Redis accepts the event;
// a crash between XADD and the SQL commit can produce a duplicate, never loss.
func (s *Service) PublishPendingMessageEvents(ctx context.Context, limit int) (int, error) {
	if s.redis == nil {
		return 0, errors.New("realtime message stream is unavailable")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id,tenant_id::text,payload::text
		FROM message_event_outbox
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, err
	}
	type pendingEvent struct {
		id       int64
		tenantID string
		payload  string
	}
	pending := make([]pendingEvent, 0, limit)
	for rows.Next() {
		var event pendingEvent
		if err := rows.Scan(&event.id, &event.tenantID, &event.payload); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	published := 0
	for _, event := range pending {
		if strings.TrimSpace(event.tenantID) == "" || event.payload == "" {
			return published, fmt.Errorf("invalid message event outbox row %d", event.id)
		}
		publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := s.redis.XAdd(publishCtx, &redis.XAddArgs{
			Stream: "tgqf:tenant:" + event.tenantID + ":message-events",
			MaxLen: 10000,
			Approx: true,
			Values: map[string]any{"payload": event.payload},
		}).Err()
		cancel()
		if err != nil {
			return published, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE message_event_outbox
			SET published_at=NOW()
			WHERE id=$1 AND published_at IS NULL`, event.id); err != nil {
			return published, err
		}
		published++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return published, nil
}

// PrunePublishedMessageEvents bounds the PostgreSQL delivery outbox while
// retaining recent delivery history. Pending rows are never eligible.
func (s *Service) PrunePublishedMessageEvents(ctx context.Context, retention time.Duration, limit int) (int64, error) {
	if retention < 24*time.Hour {
		return 0, errors.New("message event outbox retention must be at least 24 hours")
	}
	if limit < 1 || limit > 5000 {
		limit = 1000
	}
	result, err := s.db.ExecContext(ctx, `WITH expired AS (
		SELECT id FROM message_event_outbox
		WHERE published_at IS NOT NULL
		  AND published_at < NOW() - ($1 * INTERVAL '1 second')
		ORDER BY id
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	)
	DELETE FROM message_event_outbox o USING expired e
	WHERE o.id=e.id`, int64(retention/time.Second), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
