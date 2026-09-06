package cryptoutil

import (
	"crypto/rand"
	"testing"

	"github.com/emmansun/gmsm/sm2"
)

func TestSM3KnownVector(t *testing.T) {
	const want = "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"
	if got := SM3Hex("abc"); got != want {
		t.Fatalf("SM3(abc)=%s, want %s", got, want)
	}
}

func TestSM4GCMRoundTripAndTamperFailure(t *testing.T) {
	key := []byte("0123456789abcdef")
	envelope, err := SealSM4GCM(key, []byte("nursing-record"), []byte("patient:001"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := OpenSM4GCM(key, envelope, []byte("patient:001"))
	if err != nil || string(plaintext) != "nursing-record" {
		t.Fatalf("round trip failed: %q %v", plaintext, err)
	}
	if _, err := OpenSM4GCM(key, envelope, []byte("patient:002")); err == nil {
		t.Fatal("tampered AAD was accepted")
	}
	if _, err := SealSM4GCM([]byte("short"), []byte("x"), nil); err == nil {
		t.Fatal("invalid key was accepted")
	}
}

func TestSM2SM3Signature(t *testing.T) {
	privateKey, err := sm2.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("experiment-manifest")
	signature, err := SignSM2SM3(privateKey, message)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifySM2SM3(&privateKey.PublicKey, message, signature) {
		t.Fatal("signature did not verify")
	}
	if VerifySM2SM3(&privateKey.PublicKey, []byte("modified"), signature) {
		t.Fatal("modified message verified")
	}
}
