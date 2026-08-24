package cutoververify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CloneDirName is the fixed subdirectory name of the cached shared clone
// inside the scratch root, so repeated invocations reuse rather than
// re-clone it.
const CloneDirName = "cutover-verify-clone"

// ScratchRoot resolves the directory the cached clone lives under: the
// CUTOVER_SCRATCH environment variable when set, otherwise the OS temp
// directory.
func ScratchRoot() string {
	if dir := os.Getenv("CUTOVER_SCRATCH"); dir != "" {
		return dir
	}
	return os.TempDir()
}

// EnsureClone returns the path to a shared clone of repoRoot under
// scratchRoot, cloning it once if it does not already exist. `--shared`
// keeps the clone's object database as an alternate onto repoRoot, so
// commits made in repoRoot after the clone was created are still reachable
// by SHA without a fetch.
func EnsureClone(scratchRoot, repoRoot string) (string, error) {
	clonePath := filepath.Join(scratchRoot, CloneDirName)
	if info, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil && info.IsDir() {
		return clonePath, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", clonePath, err)
	}

	if err := os.MkdirAll(scratchRoot, 0o755); err != nil {
		return "", fmt.Errorf("create scratch root %s: %w", scratchRoot, err)
	}
	cmd := exec.Command("git", "clone", "--shared", repoRoot, clonePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone --shared %s %s: %w\n%s", repoRoot, clonePath, err, out)
	}
	return clonePath, nil
}

// ResetClone hard-resets the clone to commit.
func ResetClone(clonePath, commit string) error {
	cmd := exec.Command("git", "reset", "--hard", commit)
	cmd.Dir = clonePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git reset --hard %s in %s: %w\n%s", commit, clonePath, err, out)
	}
	cmd = exec.Command("git", "clean", "-fd")
	cmd.Dir = clonePath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clean -fd in %s: %w\n%s", clonePath, err, out)
	}
	return nil
}

// ResolveCommit resolves a revision to its full commit SHA inside the clone.
func ResolveCommit(clonePath, revision string) (string, error) {
	cmd := exec.Command("git", "rev-parse", revision)
	cmd.Dir = clonePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s in %s: %w", revision, clonePath, err)
	}
	return trimNewline(string(out)), nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
