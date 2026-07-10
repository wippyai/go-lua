package factflow

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
	Imports    []string
}

func TestFactflowPackageDirectImportsStayDomainOnly(t *testing.T) {
	banned := []string{
		modulePath + "/analysis/check",
		modulePath + "/analysis/engine/state",
		modulePath + "/analysis/lua",
		modulePath + "/analysis/type",
		modulePath + "/compiler",
	}

	for _, dep := range productionImports(t, ".") {
		for _, prefix := range banned {
			if forbiddenImport(dep, prefix, false) {
				t.Fatalf("factflow package imports forbidden dependency %q", dep)
			}
		}
	}
}

func TestFactflowPackageDoesNotReachForbiddenLayersTransitively(t *testing.T) {
	banned := []string{
		modulePath + "/analysis/check",
		modulePath + "/analysis/engine/state",
		modulePath + "/analysis/lua",
		modulePath + "/analysis/type",
	}

	for _, dep := range productionDeps(t, ".") {
		for _, prefix := range banned {
			if forbiddenImport(dep, prefix, false) {
				t.Fatalf("factflow package reaches forbidden dependency %q", dep)
			}
		}
	}
}

func productionPackages(t *testing.T, patterns ...string) []listedPackage {
	t.Helper()

	args := append([]string{"list", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []listedPackage
	for {
		var pkg listedPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
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

func productionImports(t *testing.T, pattern string) []string {
	t.Helper()

	for _, pkg := range productionPackages(t, pattern) {
		return pkg.Imports
	}
	return nil
}

func forbiddenImport(dep, banned string, exactly bool) bool {
	if exactly {
		return dep == banned
	}
	return dep == banned || strings.HasPrefix(dep, banned+"/")
}
