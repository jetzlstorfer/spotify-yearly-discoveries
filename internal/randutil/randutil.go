// Package randutil provides cryptographically secure random helpers.
package randutil

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
)

// HexString returns a cryptographically random hex-encoded string of n bytes.
// It calls os.Exit(1) if the system random source is unavailable, which should
// never happen on a properly functioning OS.
func HexString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		slog.Error("could not generate random bytes", "err", err)
		os.Exit(1)
	}
	return hex.EncodeToString(b)
}
