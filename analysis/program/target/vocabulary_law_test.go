package target

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// The closed vocabulary is a package of its own because it is the one altitude
// that references nothing: the declared enums, handles and authoring specs stand
// on their own, and the sealed contract is written in them. Making it a package
// puts that under the compiler instead of under review - a vocabulary file cannot
// name the contract, because it cannot import it.
//
// The two laws below state the direction. The first is the one that can regress
// silently, so it is stated over the vocabulary's own imports; the second keeps it
// from being vacuous by requiring the contract to actually be written in it.

const (
	targetPackage     = "github.com/wippyai/go-lua/analysis/program/target"
	vocabularyPackage = targetPackage + "/vocabulary"
)

// TestVocabularyImportsNothingFromTheContract states that the vocabulary is
// closed: no file in it names the contract package or anything under it.
func TestVocabularyImportsNothingFromTheContract(t *testing.T) {
	fileset := token.NewFileSet()
	files := vocabularySources(t)
	if len(files) == 0 {
		t.Fatal("vocabulary package has no sources")
	}
	for _, path := range files {
		parsed, err := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), err)
		}
		for _, spec := range parsed.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if imported == targetPackage || strings.HasPrefix(imported, targetPackage+"/") {
				t.Errorf("%s imports %s; the vocabulary references nothing above it",
					filepath.Base(path), imported)
			}
		}
	}
}

// TestContractIsWrittenInTheVocabulary keeps the direction law honest: the
// contract package names the vocabulary, so the closure above is a real
// constraint rather than a statement about two unrelated packages.
func TestContractIsWrittenInTheVocabulary(t *testing.T) {
	fileset := token.NewFileSet()
	naming := 0
	for _, path := range contractSources(t) {
		parsed, err := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), err)
		}
		for _, spec := range parsed.Imports {
			if strings.Trim(spec.Path.Value, `"`) == vocabularyPackage {
				naming++
			}
		}
	}
	if naming == 0 {
		t.Fatal("no contract source names the vocabulary package")
	}
}

func vocabularySources(t *testing.T) []string {
	t.Helper()
	return sourcesIn(t, filepath.Join(packageDirectory(t), "vocabulary"))
}

func contractSources(t *testing.T) []string {
	t.Helper()
	return sourcesIn(t, packageDirectory(t))
}

func sourcesIn(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	var out []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func packageDirectory(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	return filepath.Dir(self)
}
