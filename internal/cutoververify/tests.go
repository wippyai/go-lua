package cutoververify

import (
	"fmt"
	"os"
	"os/exec"
)

// RunTargetedTests runs `go test -count=1 <pkg>/...` in clonePath under a
// bounded memory/CPU envelope, since this is a test process rather than a
// build.
func RunTargetedTests(clonePath, pkg string) Result {
	target := packagePattern(pkg)
	cmd := exec.Command("go", "test", "-count=1", target)
	cmd.Dir = clonePath
	cmd.Env = append(os.Environ(), "GOMEMLIMIT=2GiB", "GOMAXPROCS=4")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Name:   "TESTS",
			Status: StatusFail,
			Note:   fmt.Sprintf("go test -count=1 %s failed", target),
			Detail: string(out),
		}
	}
	return Result{Name: "TESTS", Status: StatusPass, Note: "go test -count=1 " + target}
}
