package utils

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// FingerprintEd25519 returns SHA-256(pubKey) as a hex string.
func FingerprintEd25519(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	return hex.EncodeToString(hash[:])
}

// VerifySignature decodes a standard base64 Ed25519 signature and verifies it
// over the given canonical string. Returns a non-nil error if decoding fails,
// the signature length is wrong, or the signature does not verify.
func VerifySignature(pubKey ed25519.PublicKey, canonical string, sigBase64 string) error {
	sigBytes, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length")
	}
	if !ed25519.Verify(pubKey, []byte(canonical), sigBytes) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

// HTTPCanonical builds the canonical string for HTTP request signing:
//
//	METHOD\nPATH\nTIMESTAMP\nBODY_SHA256_HEX\nNONCE
func HTTPCanonical(method, path, timestampRaw, bodyHash, nonce string) string {
	return method + "\n" + path + "\n" + timestampRaw + "\n" + bodyHash + "\n" + nonce
}

// WSCanonical builds the canonical string for WebSocket auth signing:
//
//	WS\nSESSION_ID\nTIMESTAMP
func WSCanonical(sessionID, timestampRaw string) string {
	return "WS\n" + sessionID + "\n" + timestampRaw
}
