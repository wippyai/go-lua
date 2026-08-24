package cutoververify

import (
	"fmt"
	"os/exec"
	"strings"
)

// firstLines returns the first n lines of s, joined back with newlines.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// RunBuild runs `go build ./...` in clonePath with the module's normal
// build settings: no -p=1, no GOMAXPROCS override.
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
	return Result{Name: "BUILD", Status: StatusPass, Note: "go build ./..."}
}

// RunVet runs `go vet <pkg>/...` in clonePath.
func RunVet(clonePath, pkg string) Result {
	target := pkg + "/..."
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
