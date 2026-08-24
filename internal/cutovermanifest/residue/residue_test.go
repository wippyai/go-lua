package residue

import (
	"path/filepath"
	"sort"
	"testing"
)

const fixtureDir = "testdata/fixture"

func TestCensusFindsLegacyTokens(t *testing.T) {
	hits, err := Census(fixtureDir)
	if err != nil {
		t.Fatalf("Census: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one legacy-token hit in the fixture")
	}
	var files []string
	for _, h := range hits {
		files = append(files, filepath.Base(h.File))
	}
	sort.Strings(files)
	want := map[string]bool{
		"legacy_hot_rule.go":   false,
		"mixed.go":             false,
		"legacy_unexported.go": false,
	}
	for _, f := range files {
		want[f] = true
	}
	for f, seen := range want {
		if !seen {
			t.Errorf("expected a hit in %s", f)
		}
	}
}

func TestLegacyFilesClassifiesPureResidue(t *testing.T) {
	files, err := LegacyFiles(fixtureDir)
	if err != nil {
		t.Fatalf("LegacyFiles: %v", err)
	}
	byName := map[string]File{}
	for _, f := range files {
		byName[filepath.Base(f.Path)] = f
	}

	if _, ok := byName["legacy_hot_rule.go"]; !ok {
		t.Errorf("expected legacy_hot_rule.go (every exported decl is residue) to be classified as legacy")
	}
	if f, ok := byName["legacy_hot_rule.go"]; ok && f.NoExported {
		t.Errorf("legacy_hot_rule.go has exported declarations, NoExported should be false")
	}

	if _, ok := byName["legacy_unexported.go"]; !ok {
		t.Errorf("expected legacy_unexported.go (no exported surface, residue-only) to be classified as legacy")
	}
	if f, ok := byName["legacy_unexported.go"]; ok && !f.NoExported {
		t.Errorf("legacy_unexported.go declares no exported symbol, NoExported should be true")
	}

	if _, ok := byName["mixed.go"]; ok {
		t.Errorf("mixed.go declares real exported surface (Current) untouched by residue and must not be classified as legacy")
	}
}

func TestLegacyFilesEmptyWhenNoHits(t *testing.T) {
	files, err := LegacyFiles("testdata/clean")
	if err != nil {
		t.Fatalf("LegacyFiles: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no legacy files in a clean directory, got %d", len(files))
	}
}
