package factflow

import (
	"os/exec"
	"strings"
	"testing"
)

const modulePath = "github.com/wippyai/go-lua"

func TestFactflowPackageDoesNotImportLuaCompilerCheckASTOrQuarantinedPackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		modulePath + "/__old",
		modulePath + "/analysis/check",
		modulePath + "/analysis/diagnostic",
		modulePath + "/analysis/domain/effect",
		modulePath + "/analysis/lua",
		modulePath + "/analysis/type",
		modulePath + "/compiler",
		modulePath + "/compiler/ast",
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
		modulePath + "/analysis/domain/state/key":  true,
		modulePath + "/analysis/engine/state":      true,
		modulePath + "/analysis/engine/transfer":   true,
		modulePath + "/analysis/engine/visibility": true,
		modulePath + "/analysis/engine/factapply":  true,
	}
	for _, dep := range strings.Fields(string(out)) {
		if banned[dep] {
			t.Fatalf("factflow package imports forbidden dependency %q", dep)
		}
	}
}
