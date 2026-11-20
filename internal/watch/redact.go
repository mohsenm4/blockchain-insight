package watch

import "regexp"

var rpcURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

func safeRPCError(err error) string {
	if err == nil {
		return ""
	}

	return rpcURLPattern.ReplaceAllString(
		err.Error(),
		"[RPC URL REDACTED]",
	)
}
