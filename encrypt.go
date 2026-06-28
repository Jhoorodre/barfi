package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

// encPrefix marks ciphertext stored in config so we can distinguish it from
// plain-text tokens written by older versions (backward compat).
const encPrefix = "enc1:"

// deriveKey builds a 32-byte AES key from machine + user identifiers.
// Deterministic on the same machine/user pair; never written to disk.
func deriveKey() []byte {
	home, _ := os.UserHomeDir()
	raw := "barfi-token-v1\x00" + machineID() + "\x00" + home
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

func encryptToken(plaintext string) (string, error) {
	block, err := aes.NewCipher(deriveKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// decryptToken returns the plain-text token. If stored does not start with
// encPrefix it is treated as plain text (tokens from older versions).
func decryptToken(stored string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	data, err := base64.StdEncoding.DecodeString(stored[len(encPrefix):])
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}
	block, err := aes.NewCipher(deriveKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	pt, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}
