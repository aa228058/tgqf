package safelog

import (
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
)

var (
	sensitiveKey    = regexp.MustCompile(`(?i)(password|passwd|secret|token|api.?hash|session|authorization|cookie|credential|private.?key|database.?url|redis.?url|dsn)`)
	urlCredential   = regexp.MustCompile(`(?i)((?:postgres(?:ql)?|rediss?|https?|socks5h?)://[^:/@\s]*:[^@/\s]+@`)
	parameterSecret = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?hash|session|authorization|cookie)=([^&\s,;]+)`)
	jsonSecret      = regexp.MustCompile(`(?i)("(?:password|passwd|secret|token|api[_-]?hash|session|authorization|cookie)"\s*:\s*")([^"]+)(")`)
	bearerSecret    = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
)

const redacted = "[REDACTED]"

// NewJSONHandler creates a JSON slog handler that removes sensitive fields and
// scrubs common credentials embedded in error messages and URLs.
func NewJSONHandler(w io.Writer, options *slog.HandlerOptions) slog.Handler {
	var opts slog.HandlerOptions
	if options != nil {
		opts = *options
	}
	previous := opts.ReplaceAttr
	opts.ReplaceAttr = func(groups []string, attr slog.Attr) slog.Attr {
		if sensitiveKey.MatchString(attr.Key) {
			attr.Value = slog.StringValue(redacted)
		} else if attr.Value.Kind() == slog.KindString {
			attr.Value = slog.StringValue(RedactText(attr.Value.String()))
		} else if attr.Key == "error" && attr.Value.Kind() == slog.KindAny {
			if value := attr.Value.Any(); value != nil {
				attr.Value = slog.StringValue(RedactText(fmt.Sprint(value)))
			}
		}
		if previous != nil {
			return previous(groups, attr)
		}
		return attr
	}
	return slog.NewJSONHandler(w, &opts)
}

// RedactText scrubs credentials that may be embedded in dependency errors.
func RedactText(value string) string {
	urlCredential   = regexp.MustCompile(`(?i)((?:postgres(?:ql)?|rediss?|https?|socks5h?)://[^:/@\s]*:[^@/\s]+@`)
		scheme := strings.SplitN(match, "://", 2)[0]
		return scheme + "://[REDACTED]@"
	})
	value = parameterSecret.ReplaceAllString(value, "$1="+redacted)
	jsonSecret      = regexp.MustCompile(`(?i)("(?:password|passwd|secret|token|api[_-]?hash|session|authorization|cookie)"\s*:\s*")([^"]+)(")`)
	value = jsonSecret.ReplaceAllString(value, "$1"+redacted+"$3")
	value = bearerSecret.ReplaceAllString(value, "$1"+redacted)
	return value
}
