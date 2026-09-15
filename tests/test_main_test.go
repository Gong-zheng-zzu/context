package tests

import (
	"os"
	"testing"
)

// TestMain supplies an ephemeral signing key for package-level integration
// tests when the caller did not provide one. Production code still requires
// JWT_SECRET and has no fallback; this only makes `go test ./tests` reproducible
// outside CI.
func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("JWT_SECRET"); !ok {
		_ = os.Setenv("JWT_SECRET", "ci-only-test-secret")
	}
	os.Exit(m.Run())
}
