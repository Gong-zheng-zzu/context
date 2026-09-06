package security

import (
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emmansun/gmsm/sm2"
	"github.com/emmansun/gmsm/smx509"
)

func TestAttachAuditSignatureUsesSM2WhenKeyConfigured(t *testing.T) {
	privateKey, err := sm2.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := smx509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "audit-sm2.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(auditSM2KeyFileEnv, path)
	t.Setenv(auditSM2KeyIDEnv, "test-sm2-key")
	t.Setenv(auditRequireEnv, "true")
	event := AuditEvent{ID: "event-1", Timestamp: time.Unix(0, 0).UTC(), Metadata: map[string]interface{}{"kind": "test"}}
	if err := attachAuditSignature(&event); err != nil {
		t.Fatal(err)
	}
	signature, ok := event.Metadata["audit_signature"].(map[string]interface{})
	if !ok || signature["status"] != "signed" || signature["algorithm"] != "SM2-SM3" {
		t.Fatalf("unexpected signature metadata: %#v", event.Metadata)
	}
}

func TestAttachAuditSignatureFailsClosedWhenRequired(t *testing.T) {
	t.Setenv(auditSM2KeyFileEnv, "")
	t.Setenv(auditRequireEnv, "true")
	if err := attachAuditSignature(&AuditEvent{ID: "event-2", Timestamp: time.Unix(0, 0).UTC()}); err == nil {
		t.Fatal("missing required key was accepted")
	}
}
