package messages

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

type simulatedNetworkTimeout struct{}

func (simulatedNetworkTimeout) Error() string { return "network operation timed out" }
func (simulatedNetworkTimeout) Timeout() bool { return true }
func (simulatedNetworkTimeout) Temporary() bool { return true }

func TestIsAmbiguousTelegramSendError(t *testing.T) {
	cases := []struct {
		name string
		err error
		want bool
	}{
		{"nil", nil, false},
		{"server rejected request", errors.New("USERNAME_INVALID"), false},
		{"FloodWait response", errors.New("FLOOD_WAIT_30"), false},
		{"deadline exceeded", fmt.Errorf("send Telegram message: %w", context.DeadlineExceeded), true},
		{"cancelled during shutdown", context.Canceled, true},
		{"connection EOF", io.EOF, true},
		{"network timeout", simulatedNetworkTimeout{}, true},
		{"connection reset", errors.New("write tcp: connection reset by peer"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAmbiguousTelegramSendError(tc.err); got != tc.want {
				t.Fatalf("isAmbiguousTelegramSendError(%v)=%v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
