package messages

import (
	"errors"
	"strconv"
	"strings"
)

type BulkTaskActionResult struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// normalizeBulkTaskIDs validates the complete request before any queue mutation.
// Queue rows are independent transactions, so execution intentionally reports
// per-item outcomes instead of claiming all-or-nothing semantics.
func normalizeBulkTaskIDs(taskType, action string, input []string) ([]string, error) {
	switch taskType {
	case "message_reply", "keyword_action", "profile_update", "group_join":
	default:
		return nil, errors.New("invalid task type")
	}
	switch action {
	case "pause", "resume", "retry", "cancel":
	default:
		return nil, errors.New("unsupported task action")
	}
	if len(input) == 0 || len(input) > 100 {
		return nil, errors.New("bulk task action requires between 1 and 100 ids")
	}
	ids := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, raw := range input {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, errors.New("task id is required")
		}
		if _, ok := seen[id]; ok {
			return nil, errors.New("duplicate task id")
		}
		if taskType == "group_join" {
			n, err := strconv.ParseInt(id, 10, 64)
			if err != nil || n < 1 {
				return nil, errors.New("invalid group join task id")
			}
		} else if !validUUID(id) {
			return nil, errors.New("invalid task id")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}
