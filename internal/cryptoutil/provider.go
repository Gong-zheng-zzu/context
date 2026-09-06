// Package cryptoutil provides the project's small, auditable Guomi primitive boundary.
package cryptoutil

import (
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/emmansun/gmsm/sm2"
	"github.com/emmansun/gmsm/sm3"
	"github.com/emmansun/gmsm/sm4"
)

const (
	SM3Algorithm    = "SM3"
	SM4GCMAlgorithm = "SM4-GCM"
	SM2SM3Algorithm = "SM2-SM3"
	EnvelopeVersion = "gm-v1"
)

// SM3Hex returns the lowercase SM3 digest for a string.
func SM3Hex(value string) string {
	return SM3HexBytes([]byte(value))
}

// SM3HexBytes returns the lowercase SM3 digest for bytes.
func SM3HexBytes(value []byte) string {
	digest := sm3.Sum(value)
	return hex.EncodeToString(digest[:])
}

// SM4GCMEnvelope is a self-describing encrypted value. The authentication tag
// is part of Ciphertext as required by cipher.AEAD.Seal.
type SM4GCMEnvelope struct {
	Version    string `json:"version"`
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func sm4GCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("SM4-GCM requires a 16-byte key, got %d", len(key))
	}
	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create SM4 cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// SealSM4GCM encrypts plaintext and authenticates optional additional data.
func SealSM4GCM(key, plaintext, additionalData []byte) (SM4GCMEnvelope, error) {
	gcm, err := sm4GCM(key)
	if err != nil {
		return SM4GCMEnvelope{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SM4GCMEnvelope{}, fmt.Errorf("generate SM4-GCM nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)
	return SM4GCMEnvelope{
		Version: EnvelopeVersion, Algorithm: SM4GCMAlgorithm,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

// OpenSM4GCM fails closed for malformed, unsupported, or tampered envelopes.
func OpenSM4GCM(key []byte, envelope SM4GCMEnvelope, additionalData []byte) ([]byte, error) {
	if envelope.Version != EnvelopeVersion || envelope.Algorithm != SM4GCMAlgorithm {
		return nil, errors.New("unsupported SM4-GCM envelope")
	}
	gcm, err := sm4GCM(key)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid SM4-GCM nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, errors.New("invalid SM4-GCM ciphertext")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, errors.New("SM4-GCM authentication failed")
	}
	return plaintext, nil
}

// SignSM2SM3 signs the raw message with the standard SM2 message preprocessing.
func SignSM2SM3(privateKey *sm2.PrivateKey, message []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, errors.New("SM2 private key is required")
	}
	return sm2.SignASN1(rand.Reader, privateKey, message, sm2.DefaultSM2SignerOpts)
}

// VerifySM2SM3 validates a signature produced by SignSM2SM3.
func VerifySM2SM3(publicKey *ecdsa.PublicKey, message, signature []byte) bool {
	if publicKey == nil {
		return false
	}
	return sm2.VerifyASN1WithSM2(publicKey, nil, message, signature)
}
