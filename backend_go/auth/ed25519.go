package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// VerifyEdgeSignature checks Ed25519 signature over payload
// Payload is typically: node_id|ts|lat|lon|metrics_json
func VerifyEdgeSignature(payload string, signatureB64 string, pubKey []byte) bool {
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pubKey, []byte(payload), sig)
}

// HashForSolana returns SHA256 hex of decision content
func HashForSolana(summary string) string {
	h := sha256.Sum256([]byte(summary))
	return hex.EncodeToString(h[:])
}

// BuildSignablePayload creates canonical string for signing
func BuildSignablePayload(nodeID string, ts int64, lat, lon float64, metricsJSON string) string {
	return fmt.Sprintf("%s|%d|%.6f|%.6f|%s", nodeID, ts, lat, lon, metricsJSON)
}
