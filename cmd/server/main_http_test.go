//go:build http

package main

import (
	"testing"
	"time"
)

func TestToolCallErrorKeepsPercentSignsLiteral(t *testing.T) {
	err := toolCallError("invalid value: 100% complete")
	if err.Error() != "invalid value: 100% complete" {
		t.Fatalf("toolCallError() = %q", err.Error())
	}
}

func TestNeo4jEngineConfigReadsConnectionSettings(t *testing.T) {
	t.Setenv("NEO4J_URI", "bolt://graph:7687")
	t.Setenv("NEO4J_USERNAME", "graph-user")
	t.Setenv("NEO4J_PASSWORD", "graph-password")
	t.Setenv("NEO4J_DATABASE", "graph-db")
	t.Setenv("NEO4J_MAX_CONNECTION_POOL_SIZE", "17")
	t.Setenv("NEO4J_CONNECTION_TIMEOUT", "7s")
	t.Setenv("NEO4J_MAX_TRANSACTION_RETRY_TIME", "11s")

	config := neo4jEngineConfigFromEnv()
	if config.URI != "bolt://graph:7687" || config.Username != "graph-user" || config.Password != "graph-password" || config.Database != "graph-db" {
		t.Fatalf("identity settings = %#v", config)
	}
	if config.MaxConnectionPoolSize != 17 || config.ConnectionTimeout != 7*time.Second || config.MaxTransactionRetryTime != 11*time.Second {
		t.Fatalf("connection settings = %#v", config)
	}
}

func TestDurationEnvFallsBackForInvalidValues(t *testing.T) {
	t.Setenv("NEO4J_CONNECTION_TIMEOUT", "invalid")
	if got := getDurationEnv("NEO4J_CONNECTION_TIMEOUT", 9*time.Second); got != 9*time.Second {
		t.Fatalf("fallback duration = %s, want 9s", got)
	}
}
