package compiler_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/domain/composite"
)

// TestExactScalarProductionCounterDriftFixtureTerminates drives the canonical
// lowering and artifact compiler with the real soundness fixture. The
// recurrence law is intentionally checked through production row geometry,
// not only through a synthetic exactscalar Input.
func TestExactScalarProductionCounterDriftFixtureTerminates(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve compiler fixture location")
	}
	repository := filepath.Dir(thisFile)
	for {
		if info, err := os.Stat(filepath.Join(repository, "go.mod")); err == nil && !info.IsDir() {
			break
		}
		parent := filepath.Dir(repository)
		if parent == repository {
			t.Fatal("cannot locate repository root")
		}
		repository = parent
	}
	name := "soundness/integer-loop-counter-drift"
	source, err := os.ReadFile(filepath.Join(repository, "testdata", "fixtures", filepath.FromSlash(name), "main.lua"))
	if err != nil {
		t.Fatalf("read recurrence fixture: %v", err)
	}
	published, err := lower.Lower(lower.Source{Name: name + "/main.lua", Text: source})
	if err != nil || published == nil || !published.Available() {
		t.Fatalf("lower recurrence fixture: %v", err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !grammar.Available() || !issuanceOK {
		t.Fatal("Program artifact issuance unavailable")
	}

	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile recurrence fixture: artifact=%v failure=%s", artifact != nil, failure.Error())
	}
}
