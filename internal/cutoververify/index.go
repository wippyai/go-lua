package cutoververify

import (
	"fmt"
	"os/exec"
	"strings"
)

// CheckIndexClean runs `git diff --cached --name-only` in repoRoot. A
// nonempty index means files are staged in the shared tree that may belong
// to another lane, so the caller must refuse to proceed rather than clone
// and build around them.
func CheckIndexClean(repoRoot string) (Result, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return Result{Name: "INDEX", Status: StatusFail}, fmt.Errorf("git diff --cached --name-only: %w", err)
	}
	staged := strings.TrimSpace(string(out))
	if staged == "" {
		return Result{Name: "INDEX", Status: StatusPass, Note: "no staged files"}, nil
	}
	files := strings.Split(staged, "\n")
	return Result{Name: "INDEX", Status: StatusFail}, fmt.Errorf(
		"%d file(s) staged in %s, refusing to proceed (they may belong to another lane):\n%s",
		len(files), repoRoot, staged)
}
