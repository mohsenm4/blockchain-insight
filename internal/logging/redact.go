package logging

import "regexp"

var rpcURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

func SafeError(err error) string {
	if err == nil {
		return ""
	}
	return rpcURLPattern.ReplaceAllString(err.Error(), "[RPC URL REDACTED]")
}
