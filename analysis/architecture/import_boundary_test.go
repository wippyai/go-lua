package architecture

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const modulePath = "github.com/wippyai/go-lua"

type listedPackage struct {
	ImportPath string
}

func TestLowerLayerImportBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		banned   []string
	}{
		{
			name:     "ir stays below lua check and lua parser ast",
			patterns: []string{modulePath + "/analysis/ir/..."},
			banned: []string{
				modulePath + "/analysis/lua",
				modulePath + "/analysis/check",
				modulePath + "/compiler/ast",
				modulePath + "/compiler/parse",
			},
		},
		{
			name:     "cfgbuild stays before semantics transferfacts and check",
			patterns: []string{modulePath + "/analysis/lua/cfgbuild"},
			banned: []string{
				modulePath + "/analysis/lua/semantics",
				modulePath + "/analysis/lua/transferfacts",
				modulePath + "/analysis/check",
			},
		},
		{
			name:     "semantics stays before transferfacts and check",
			patterns: []string{modulePath + "/analysis/lua/semantics"},
			banned: []string{
				modulePath + "/analysis/lua/transferfacts",
				modulePath + "/analysis/check",
			},
		},
		{
			name:     "transferfacts stays before check fixpoint",
			patterns: []string{modulePath + "/analysis/lua/transferfacts"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/check/fixpoint",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dep := range productionDeps(t, tt.patterns...) {
				for _, banned := range tt.banned {
					if dep == banned || strings.HasPrefix(dep, banned+"/") {
						t.Fatalf("%s imports forbidden dependency %q", strings.Join(tt.patterns, " "), dep)
					}
				}
			}
		})
	}
}

func productionDeps(t *testing.T, patterns ...string) []string {
	t.Helper()

	args := append([]string{"list", "-deps", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var deps []string
	for {
		var pkg listedPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		deps = append(deps, pkg.ImportPath)
	}
	return deps
}
