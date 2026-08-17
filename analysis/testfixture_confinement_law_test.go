package analysis

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The fixture corpus is test material. A production file that imports it gains
// the checked-in corpus, its filesystem layout, and its denominator as ambient
// inputs of the analyzer itself, which is how a harness becomes a dependency of
// the thing it measures. Confinement is stated here as a law rather than left
// to the import path's convention.

const (
	testfixtureConfinementPackage = "github.com/wippyai/go-lua/analysis/internal/testfixture"
	testfixtureConfinementOwner   = "analysis/internal/testfixture"
	testfixtureConfinementLegacy  = "__legacy"
)

// TestTestfixtureConfinementHoldsForProductionSources fails when any non-test
// Go file outside the owning package imports the fixture corpus.
func TestTestfixtureConfinementHoldsForProductionSources(t *testing.T) {
	repository := architectureBatteryRepositoryRoot(t)
	violations := make([]string, 0)
	err := filepath.WalkDir(repository, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, relativeErr := filepath.Rel(repository, path)
		if relativeErr != nil {
			return relativeErr
		}
		slashed := filepath.ToSlash(relative)
		if entry.IsDir() {
			name := entry.Name()
			if slashed == testfixtureConfinementLegacy || slashed == testfixtureConfinementOwner ||
				(len(name) > 1 && strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imported := range parsed.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if value == testfixtureConfinementPackage {
				violations = append(violations, slashed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		return
	}
	sort.Strings(violations)
	t.Fatalf("production sources import the fixture corpus %s:\n  %s", testfixtureConfinementOwner, strings.Join(violations, "\n  "))
}
