package flow

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Flow's judgments live in public sibling packages, and the shell assembles
// them. That is a direction, and a direction only holds if naming it the wrong
// way round fails to compile. A sibling that named the shell would close a
// cycle; the two laws below state the direction and keep it from going vacuous
// once every sibling is a package in its own right.

const flowPackage = "github.com/wippyai/go-lua/analysis/program/flow"

// TestFlowSiblingsNameNoShell states the direction over every sibling: no
// owner package names the assembling shell, so no Flow judgment can be written
// in terms of how Flow is assembled or published. External test packages sit
// at the consumer altitude and are free to name both.
func TestFlowSiblingsNameNoShell(t *testing.T) {
	fileset := token.NewFileSet()
	root := flowTreeRoot(t)
	scanned := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Dir(path) == root || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		scanned++
		for _, spec := range parsed.Imports {
			if strings.Trim(spec.Path.Value, `"`) == flowPackage {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s imports the Flow shell; a Flow owner never names its assembly", filepath.ToSlash(relative))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("Flow tree has no sibling sources")
	}
}

// TestFlowShellNamesItsSiblings keeps the direction law honest: the shell names
// the owners it assembles, so the closure above constrains packages that
// actually meet rather than unrelated ones. Every sibling is reachable from the
// shell -- directly, or through the sibling that drives its seal.
func TestFlowShellNamesItsSiblings(t *testing.T) {
	root := flowTreeRoot(t)
	shellNames := flowImportedSiblings(t, filepath.Join(root, "*.go"))
	if len(shellNames) == 0 {
		t.Fatal("the Flow shell names no sibling; the direction law would be vacuous")
	}
	reachable := map[string]bool{}
	for name := range shellNames {
		reachable[name] = true
	}
	for changed := true; changed; {
		changed = false
		for name := range reachable {
			for named := range flowImportedSiblings(t, filepath.Join(root, name, "*.go")) {
				if !reachable[named] {
					reachable[named] = true
					changed = true
				}
			}
		}
	}
	for _, sibling := range flowSiblings(t) {
		if sibling == "internal" || reachable[sibling] {
			continue
		}
		t.Errorf("sibling %q is unreachable from the Flow shell; either nothing assembles it or it does not belong under Flow", sibling)
	}
}

func flowImportedSiblings(t *testing.T, pattern string) map[string]bool {
	t.Helper()
	fileset := token.NewFileSet()
	named := map[string]bool{}
	sources, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), parseErr)
		}
		for _, spec := range parsed.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(imported, flowPackage+"/") {
				named[strings.TrimPrefix(imported, flowPackage+"/")] = true
			}
		}
	}
	return named
}

// TestFlowTestSupportStaysUnpublished keeps the seal fixtures out of the
// published surface: flowtest exists for sibling seal tests and must remain
// unreachable from any consumer outside the Flow tree.
func TestFlowTestSupportStaysUnpublished(t *testing.T) {
	root := flowTreeRoot(t)
	if _, err := os.Stat(filepath.Join(root, "internal", "flowtest")); err != nil {
		t.Fatalf("Flow test support is not under internal/: %v", err)
	}
}

func flowSiblings(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(flowTreeRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

func flowTreeRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Flow source location unavailable")
	}
	return filepath.Dir(current)
}
