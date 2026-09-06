package security

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/contextkeeper/service/internal/cryptoutil"
	"github.com/emmansun/gmsm/sm2"
	"github.com/emmansun/gmsm/smx509"
)

const (
	auditSM2KeyFileEnv = "AUDIT_SM2_PRIVATE_KEY_FILE"
	auditSM2KeyIDEnv   = "AUDIT_SM2_PUBLIC_KEY_ID"
	auditRequireEnv    = "AUDIT_REQUIRE_SM2_SIGNATURE"
)

func auditSignatureRequired() bool {
	value, _ := os.LookupEnv(auditRequireEnv)
	return strings.EqualFold(strings.TrimSpace(value), "true") || strings.TrimSpace(value) == "1"
}

func loadAuditSM2PrivateKey() (*sm2.PrivateKey, error) {
	path := strings.TrimSpace(os.Getenv(auditSM2KeyFileEnv))
	if path == "" {
		return nil, errors.New("SM2 audit key file is not configured")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SM2 audit key: %w", err)
	}
	block, _ := pem.Decode(contents)
	if block == nil {
		return nil, errors.New("SM2 audit key is not PEM")
	}
	parsed, err := smx509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse SM2 audit key: %w", err)
	}
	privateKey, ok := parsed.(*sm2.PrivateKey)
	if !ok {
		return nil, errors.New("audit key is not an SM2 PKCS#8 private key")
	}
	return privateKey, nil
}

// attachAuditSignature adds verifiable metadata without serializing a private key.
// When signing is required, an unavailable or invalid key prevents audit loss.
func attachAuditSignature(event *AuditEvent) error {
	if event.Metadata == nil {
		event.Metadata = make(map[string]interface{})
	}
	metadata := make(map[string]interface{}, len(event.Metadata))
	for key, value := range event.Metadata {
		if key != "audit_signature" {
			metadata[key] = value
		}
	}
	unsigned := *event
	unsigned.Metadata = metadata
	payload, err := json.Marshal(unsigned)
	if err != nil {
		return fmt.Errorf("marshal audit signing payload: %w", err)
	}

	privateKey, err := loadAuditSM2PrivateKey()
	if err != nil {
		if auditSignatureRequired() {
			return err
		}
		event.Metadata["audit_signature"] = map[string]interface{}{
			"status":      "unsigned_not_eligible_for_signed_claim",
			"algorithm":   cryptoutil.SM2SM3Algorithm,
			"payload_sm3": cryptoutil.SM3HexBytes(payload),
		}
		return nil
	}

	signature, err := cryptoutil.SignSM2SM3(privateKey, payload)
	if err != nil {
		return fmt.Errorf("sign audit event: %w", err)
	}
	if !cryptoutil.VerifySM2SM3(&privateKey.PublicKey, payload, signature) {
		return errors.New("SM2 audit signature verification failed")
	}
	event.Metadata["audit_signature"] = map[string]interface{}{
		"status":              "signed",
		"algorithm":           cryptoutil.SM2SM3Algorithm,
		"payload_sm3":         cryptoutil.SM3HexBytes(payload),
		"signature_base64":    base64.StdEncoding.EncodeToString(signature),
		"public_key_id":       strings.TrimSpace(os.Getenv(auditSM2KeyIDEnv)),
		"verification_status": "verified",
	}
	return nil
}
