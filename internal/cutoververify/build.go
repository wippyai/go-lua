package cutoververify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// buildVetTimeout bounds the BUILD step's repo-wide `go vet ./...` pass so a
// hang in the clone fails the check instead of blocking the landing ritual
// indefinitely.
const buildVetTimeout = 10 * time.Minute

// packagePattern turns a bare import path like "domain/x/y" into the
// directory-relative pattern "./domain/x/y/..." that the go tool resolves
// against the current directory rather than against GOPATH-style import
// path matching, which a bare path without a leading "./" fails.
func packagePattern(pkg string) string {
	if strings.HasPrefix(pkg, "./") || strings.HasPrefix(pkg, "/") {
		return pkg + "/..."
	}
	return "./" + pkg + "/..."
}

// firstLines returns the first n lines of s, joined back with newlines.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// RunBuild runs `go build ./...` then `go vet ./...` in clonePath with the
// module's normal build settings: no -p=1, no GOMAXPROCS override.
//
// go build never compiles _test.go files, so a test-only compile error is
// invisible to it - the exact shape of the two HIGH build-health misses the
// hostile audit at 6c3bf305 found (a struct field a _test.go still compared
// as a string; an import cycle only a test's own package closes). go vet
// compiles tests too, so the BUILD step now runs both and fails on either.
func RunBuild(clonePath string) Result {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = clonePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Name:   "BUILD",
			Status: StatusFail,
			Note:   "go build ./... failed",
			Detail: firstLines(string(out), 10),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), buildVetTimeout)
	defer cancel()
	vetCmd := exec.CommandContext(ctx, "go", "vet", "./...")
	vetCmd.Dir = clonePath
	vetOut, vetErr := vetCmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return Result{
			Name:   "BUILD",
			Status: StatusFail,
			Note:   fmt.Sprintf("go vet ./... exceeded %s", buildVetTimeout),
			Detail: firstLines(string(vetOut), 10),
		}
	}
	if vetErr != nil {
		return Result{
			Name:   "BUILD",
			Status: StatusFail,
			Note:   "go vet ./... failed (a package's tests do not compile)",
			Detail: firstLines(string(vetOut), 10),
		}
	}
	return Result{Name: "BUILD", Status: StatusPass, Note: "go build ./... && go vet ./..."}
}

// RunVet runs `go vet <pkg>/...` in clonePath.
func RunVet(clonePath, pkg string) Result {
	target := packagePattern(pkg)
	cmd := exec.Command("go", "vet", target)
	cmd.Dir = clonePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Name:   "VET",
			Status: StatusFail,
			Note:   fmt.Sprintf("go vet %s failed", target),
			Detail: firstLines(string(out), 10),
		}
	}
	return Result{Name: "VET", Status: StatusPass, Note: "go vet " + target}
}
