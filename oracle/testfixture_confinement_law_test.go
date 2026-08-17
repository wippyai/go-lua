package oracle

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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
//
// This kit is the corpus' second importer, and needs no exemption to be one:
// the law scans production sources, and the kit publishes none. That is the
// condition the second law below states, so the kit cannot become production
// code by accident and quietly satisfy the first law by being skipped.

const (
	testfixtureConfinementPackage = "github.com/wippyai/go-lua/internal/testfixture"
	testfixtureConfinementOwner   = "internal/testfixture"
	testfixtureConfinementLegacy  = "__legacy"
	// testfixtureConfinementKit is this package, module-relative.
	testfixtureConfinementKit = "oracle"
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

// TestGroundingKitPublishesNoProductionSource fails when this package grows a
// non-test Go file. The kit exists to measure the analyzer from outside it; a
// production file here would be shipped code that carries the corpus, and would
// also drop out of the scan above by being its own exemption.
func TestGroundingKitPublishesNoProductionSource(t *testing.T) {
	kit := filepath.Join(architectureBatteryRepositoryRoot(t), filepath.FromSlash(testfixtureConfinementKit))
	entries, err := os.ReadDir(kit)
	if err != nil {
		t.Fatal(err)
	}
	published := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		published = append(published, entry.Name())
	}
	if len(published) == 0 {
		return
	}
	sort.Strings(published)
	t.Fatalf("%s publishes production sources:\n  %s", testfixtureConfinementKit, strings.Join(published, "\n  "))
}
