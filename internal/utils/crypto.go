package utils

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
)

// FingerprintEd25519 returns SHA-256(pubKey) as a hex string.
func FingerprintEd25519(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	return hex.EncodeToString(hash[:])
}
