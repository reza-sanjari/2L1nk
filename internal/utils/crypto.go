package utils

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Auth error taxonomy shared by HTTP middleware, WS first-message auth,
// and the gate proof-of-possession check. Callers use errors.Is to map to
// their own HTTP status codes or WS close semantics.
var (
	ErrAuthMissing   = errors.New("missing auth fields")
	ErrAuthTimestamp = errors.New("invalid timestamp")
	ErrAuthWindow    = errors.New("timestamp out of window")
	ErrAuthReplay    = errors.New("replayed request")
	ErrAuthSignature = errors.New("invalid signature")
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

// MessageCanonical builds the canonical string for a per-message Ed25519
// signature on a live chat message. Version-prefixed so we can evolve the
// format without ambiguity. See V-02 in docs/audit-report.md.
//
//	MSG_V1\nROOM_ID\nEPOCH_DECIMAL\nSENDER_FP\nTIMESTAMP\nNONCE\nCIPHERTEXT_SHA256_HEX
//
//   - epochDecimal must be strconv.FormatInt(int64(epoch), 10) on both sides
//     (client JS must emit String(epoch) on its Number-typed epoch).
//   - timestamp is unix seconds as a decimal string, matching HTTP/WS/GATE.
//   - ciphertextHashHex is lowercase-hex SHA-256 of the ciphertext bytes (same
//     convention as HashBodyHex and the frontend's hashBody helper).
func MessageCanonical(roomID, epochDecimal, senderFP, timestamp, nonce, ciphertextHashHex string) string {
	return "MSG_V1\n" + roomID + "\n" + epochDecimal + "\n" + senderFP + "\n" + timestamp + "\n" + nonce + "\n" + ciphertextHashHex
}

// GateCanonical builds the canonical string for the /api/auth/gate
// proof-of-possession signature. All identity-relevant fields are bound so a
// MITM cannot rewrite the username/mode or swap X25519 keys within the
// replay window:
//
//	GATE\nTIMESTAMP\nNONCE\nMODE\nUSERNAME\nED25519_PUBKEY_B64\nX25519_PUBKEY_B64\nBODY_SHA256_HEX
func GateCanonical(timestampRaw, nonce, mode, username, edPubB64, x25519PubB64, bodyHash string) string {
	return "GATE\n" + timestampRaw + "\n" + nonce + "\n" + mode + "\n" + username + "\n" + edPubB64 + "\n" + x25519PubB64 + "\n" + bodyHash
}

// HashBodyHex returns hex(sha256(body)). A nil or empty body hashes the
// empty byte slice, matching the frontend's SHA-256 of an empty string.
func HashBodyHex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// VerifyTimestamp parses a unix-seconds string and ensures it falls within
// ±window of the server's current time. Returns the parsed timestamp on
// success.
func VerifyTimestamp(timestampRaw string, window time.Duration) (int64, error) {
	ts, err := strconv.ParseInt(timestampRaw, 10, 64)
	if err != nil {
		return 0, ErrAuthTimestamp
	}
	now := time.Now().Unix()
	w := int64(window.Seconds())
	if ts < now-w || ts > now+w {
		return ts, ErrAuthWindow
	}
	return ts, nil
}

// AuthErrorStatus translates the typed auth errors into HTTP statuses:
// missing/bad-format fields are 400 (client bug), everything else is 401.
// Middleware applies this mapping inline; handlers that want the split call
// this helper directly.
func AuthErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrAuthMissing), errors.Is(err, ErrAuthTimestamp):
		return 400
	default:
		return 401
	}
}

// VerifySignedRequest is the single unified verifier used by the HTTP
// signature middleware, the WS first-message auth, and the gate PoP check.
// It validates the timestamp window, rejects replayed requests via the
// nonce store, and verifies the Ed25519 signature over the canonical.
//
// replayKey lets the caller choose what identifier goes into the nonce store.
// HTTP and WS both pass the signature bytes; callers are free to pass any
// high-entropy request-unique string.
func VerifySignedRequest(
	pubKey ed25519.PublicKey,
	canonical string,
	timestampRaw string,
	signatureB64 string,
	replayKey string,
	window time.Duration,
	ns *NonceStore,
) error {
	if timestampRaw == "" || signatureB64 == "" || replayKey == "" {
		return ErrAuthMissing
	}
	if _, err := VerifyTimestamp(timestampRaw, window); err != nil {
		return err
	}
	if !ns.Add(replayKey) {
		return ErrAuthReplay
	}
	if err := VerifySignature(pubKey, canonical, signatureB64); err != nil {
		return ErrAuthSignature
	}
	return nil
}
