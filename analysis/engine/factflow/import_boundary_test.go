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
		"github.com/wippyai/go-lua/analysis/check",
		"github.com/wippyai/go-lua/analysis/diagnostic",
		"github.com/wippyai/go-lua/analysis/domain/effect",
		"github.com/wippyai/go-lua/analysis/lua",
		"github.com/wippyai/go-lua/analysis/type",
		"github.com/wippyai/go-lua/compiler",
		"github.com/wippyai/go-lua/compiler/ast",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("factflow package imports forbidden dependency %q", dep)
			}
		}
	}
}

func TestFactflowPackageDoesNotImportApplicationStatePackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list imports . failed: %v", err)
	}
	banned := map[string]bool{
		"github.com/wippyai/go-lua/analysis/domain/state/key":      true,
		"github.com/wippyai/go-lua/analysis/engine/state":          true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":       true,
		"github.com/wippyai/go-lua/analysis/engine/visibility":     true,
		"github.com/wippyai/go-lua/analysis/engine/factflow/apply": true,
	}
	for _, dep := range strings.Fields(string(out)) {
		if banned[dep] {
			t.Fatalf("factflow package imports forbidden dependency %q", dep)
		}
	}
}
