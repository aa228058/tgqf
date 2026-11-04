package messages

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const incomingUpdateStream = "tgqf:telegram:incoming-updates"
const incomingUpdateGroup = "tgqf-message-ingestors"

type queuedIncomingUpdate struct {
	TenantID string   `json:"tenant_id"`
	Incoming Incoming `json:"incoming"`
}

type IncomingQueueEvent struct {
	ID        string
	TenantID  string
	Incoming  Incoming
	DecodeErr error
}

// EnqueueIncomingUpdate durably hands an MTGo update to the worker stream before
// any PostgreSQL work is attempted. The stream is intentionally not trimmed:
// unacknowledged updates must remain available for recovery.
func (s *Service) EnqueueIncomingUpdate(ctx context.Context, tenantID string, in Incoming) (string, error) {
	if s.redis == nil {
		return "", errors.New("incoming update queue is unavailable")
	}
	if strings.TrimSpace(tenantID) == "" || !validUUID(in.AccountID) || in.ChatID == 0 || in.MessageID <= 0 {
		return "", errors.New("invalid incoming update identity")
	}
	payload, err := json.Marshal(queuedIncomingUpdate{TenantID: tenantID, Incoming: in})
	if err != nil {
		return "", err
	}
	id, err := s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: incomingUpdateStream,
		Values: map[string]any{"payload": string(payload)},
	}).Result()
	if err != nil {
		return "", err
	}
	return id, nil
}

// ReadIncomingUpdates first reclaims stale pending deliveries, then reads new
// updates. A worker crash after delivery therefore does not strand the event.
func (s *Service) ReadIncomingUpdates(ctx context.Context, consumer string, count int64) ([]IncomingQueueEvent, error) {
	if s.redis == nil {
		return nil, errors.New("incoming update queue is unavailable")
	}
	consumer = strings.TrimSpace(consumer)
	if consumer == "" {
		return nil, errors.New("incoming update consumer is required")
	}
	if count < 1 || count > 500 {
		count = 100
	}
	if err := s.redis.XGroupCreateMkStream(ctx, incomingUpdateStream, incomingUpdateGroup, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, err
	}

	messages, _, err := s.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: incomingUpdateStream,
		Group: incomingUpdateGroup,
		Consumer: consumer,
		MinIdle: 30 * time.Second,
		Start: "0-0",
		Count: count,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	if len(messages) == 0 {
		streams, readErr := s.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: incomingUpdateGroup,
			Consumer: consumer,
			Streams: []string{incomingUpdateStream, ">"},
			Count: count,
			Block: time.Millisecond,
		}).Result()
		if readErr != nil {
			if errors.Is(readErr, redis.Nil) {
				return nil, nil
			}
			return nil, readErr
		}
		for _, stream := range streams {
			messages = append(messages, stream.Messages...)
		}
	}
	out := make([]IncomingQueueEvent, 0, len(messages))
	for _, message := range messages {
		event := IncomingQueueEvent{ID: message.ID}
		payload, ok := message.Values["payload"].(string)
		if !ok {
			event.DecodeErr = errors.New("incoming update payload is missing")
			out = append(out, event)
			continue
		}
		var queued queuedIncomingUpdate
		if err := json.Unmarshal([]byte(payload), &queued); err != nil {
			event.DecodeErr = err
		} else {
			event.TenantID = queued.TenantID
			event.Incoming = queued.Incoming
		}
		out = append(out, event)
	}
	return out, nil
}

// AckIncomingUpdate removes a delivery from the consumer group's pending list
// only after the PostgreSQL ingestion transaction has completed successfully.
func (s *Service) AckIncomingUpdate(ctx context.Context, id string) error {
	if s.redis == nil {
		return errors.New("incoming update queue is unavailable")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("incoming update ID is required")
	}
	// ACK and delete in one Redis transaction. XACK alone removes the
	// pending-delivery reference but leaves the payload in the stream forever;
	// this stream carries every live Telegram update, so that would grow without
	// bound even when PostgreSQL ingestion is healthy.
	_, err := s.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.XAck(ctx, incomingUpdateStream, incomingUpdateGroup, id)
		pipe.XDel(ctx, incomingUpdateStream, id)
		return nil
	})
	return err
}
