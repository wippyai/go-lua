package relation_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
)

const bridgeModule = "github.com/wippyai/go-lua/"

// TestBridgeReachesNoEngineOrLegacyPlacementRelation keeps concrete Placement
// values on the owner side of the opaque token boundary. The generic engine
// receives only semantic binding.ValueToken values through relbindgen.
func TestBridgeReachesNoEngineOrLegacyPlacementRelation(t *testing.T) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve bridge source")
	}
	directory := filepath.Dir(thisFile)
	set := token.NewFileSet()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, parseErr := parser.ParseFile(set, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), parseErr)
		}
		for _, specification := range file.Imports {
			imported, unquoteErr := strconv.Unquote(specification.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote %s: %v", entry.Name(), unquoteErr)
			}
			if strings.HasPrefix(imported, bridgeModule+"analysis/engine/") {
				t.Errorf("%s imports engine protocol %s", entry.Name(), imported)
			}
			if imported == bridgeModule+"domain/placement/relation" || strings.HasPrefix(imported, bridgeModule+"domain/placement/relation/") {
				t.Errorf("%s resurrects legacy Placement relation package %s", entry.Name(), imported)
			}
		}
	}
}

// TestGeneratedPlacementArtifactsUseTheCanonicalAnalysisLayout ensures this
// bridge cannot grow a hand-maintained zz_ file or leave behind a former
// family after the corpus marks it pending. The corpus is the only source of
// generated layout; domain/placement/relation is deliberately not one.
func TestGeneratedPlacementArtifactsUseTheCanonicalAnalysisLayout(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve bridge source")
	}
	directory := filepath.Dir(thisFile)
	artifacts, err := relbind.Emit(relbind.Declared())
	if err != nil {
		t.Fatalf("emit canonical bridge layout: %v", err)
	}
	const bridgeDir = "analysis/domain/placement/relation"
	want := make([]string, 0, 2)
	for _, artifact := range artifacts {
		if artifact.Dir == bridgeDir {
			want = append(want, artifact.Name)
		}
	}
	sort.Strings(want)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(want))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "zz_") || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("generated Placement bridge layout = %v, want %v", got, want)
	}
}
