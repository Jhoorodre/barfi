package main

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	tokens := []string{"mytoken123", "Bearer abc-def", "", "unicode-tëst-ñ"}
	for _, want := range tokens {
		enc, err := encryptToken(want)
		if err != nil {
			t.Fatalf("encryptToken(%q): %v", want, err)
		}
		if want != "" && !strings.HasPrefix(enc, encPrefix) {
			t.Errorf("encrypted %q missing prefix %q", want, encPrefix)
		}
		got, err := decryptToken(enc)
		if err != nil {
			t.Fatalf("decryptToken(%q): %v", enc, err)
		}
		if got != want {
			t.Errorf("round-trip: got %q, want %q", got, want)
		}
	}
}

func TestDecryptPlainTextBackcompat(t *testing.T) {
	// tokens stored without the prefix (older versions) must pass through as-is
	plain := "legacy-plain-token"
	got, err := decryptToken(plain)
	if err != nil {
		t.Fatalf("unexpected error for plain-text token: %v", err)
	}
	if got != plain {
		t.Errorf("got %q, want %q", got, plain)
	}
}

func TestDecryptCorruptedCiphertext(t *testing.T) {
	_, err := decryptToken(encPrefix + "bm90YmFzZTY0ISE=")
	if err == nil {
		t.Error("expected error for corrupted ciphertext, got nil")
	}
}

func TestEncryptProducesUniqueCiphertexts(t *testing.T) {
	// Same plaintext must produce different ciphertext each call (random nonce).
	a, _ := encryptToken("sametoken")
	b, _ := encryptToken("sametoken")
	if a == b {
		t.Error("two encryptions of the same token produced identical ciphertext")
	}
}
