package factflow

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFactflowPackageDoesNotImportLuaCompilerCheckASTOrQuarantinedPackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		"github.com/wippyai/go-lua/__old",
		"github.com/wippyai/go-lua/analysis/lua",
		"github.com/wippyai/go-lua/compiler/ast",
		"github.com/wippyai/go-lua/compiler/check",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("factflow package imports forbidden dependency %q", dep)
			}
		}
	}
}
