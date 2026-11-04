package messages

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
)

// isAmbiguousTelegramSendError identifies failures where the client may have
// lost the response after Telegram accepted the send. Replaying those actions
// automatically can create duplicate messages, so callers must quarantine them
// for manual verification instead of consuming the retry budget.
func isAmbiguousTelegramSendError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
		return true
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}
	upper := strings.ToUpper(err.Error())
	for _, marker := range []string{
		"UNEXPECTED EOF",
		"CONNECTION RESET",
		"CONNECTION ABORTED",
		"BROKEN PIPE",
		"I/O TIMEOUT",
		"TLS HANDSHAKE TIMEOUT",
		"TRANSPORT IS CLOSING",
		"CLIENT IS CLOSED",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
