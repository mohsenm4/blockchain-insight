package logging

import (
	"errors"
	"strings"
	"testing"
)

func TestSafeErrorRedactsRPCURLs(t *testing.T) {
	err := errors.Join(
		errors.New(`Post "https://mainnet.infura.io/v3/PRIMARY_SECRET": connection refused`),
		errors.New(`Post "https://archive.infura.io/v3/ARCHIVE_SECRET": timeout`),
	)

	got := SafeError(err)

	for _, secret := range []string{"PRIMARY_SECRET", "ARCHIVE_SECRET"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret leaked: %s", got)
		}
	}

	if strings.Count(got, "[RPC URL REDACTED]") != 2 {
		t.Fatalf("expected 2 redactions: %s", got)
	}
}
