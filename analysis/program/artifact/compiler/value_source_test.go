package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestValueSourceCompilationRetainsLiteralOccurrences(t *testing.T) {
	lowered, err := lower.Lower(lower.Source{Name: "value-source.lua", Text: []byte("return 7, true, 'x'")})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, lowered, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("value source compilation failed: %s", failure.Error())
	}
	program := artifact.Program()
	count, held := program.OccurrenceKindCount(programschema.OccurrenceValueSource)
	if !held || count < 3 {
		t.Fatalf("literal value-source count = %d/%v", count, held)
	}
}
