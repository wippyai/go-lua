package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// helperIdentity binds the executable that rendered and will apply a lock.
// Semantic Go environment and module evidence are deliberately produced by
// semantic.Loader, the sole resolver authority.
func helperIdentity() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve flashrefactor executable: %w", err)
	}
	helper, err := hashFile(executable)
	if err != nil {
		return "", fmt.Errorf("hash flashrefactor executable: %w", err)
	}
	return helper, nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalGoVersion converts the Go driver's authoritative human-readable
// identity into the single version token the lock vocabulary permits. It does
// not invent a version: malformed or non-Go output remains a hard failure.
func canonicalGoVersion(value string) (string, error) {
	parts := strings.Fields(value)
	if len(parts) < 3 || parts[0] != "go" || parts[1] != "version" {
		return "", fmt.Errorf("invalid Go executable version identity: %q", value)
	}
	version := parts[2]
	if version == "go" || !strings.HasPrefix(version, "go") || strings.ContainsAny(version, " \\;|&`$\n\r\t") {
		return "", fmt.Errorf("invalid Go version token: %q", version)
	}
	return version, nil
}
