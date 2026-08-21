package functionboundary_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func lowerFunctionBoundary(t *testing.T, text string) *program.Program {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: "function-boundary-law.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestFunctionBoundaryMatchesRequiresForeignQuartetAndPublishedOwner(t *testing.T) {
	program := lowerFunctionBoundary(t, "local function f(a) return a end return f(1)")
	view := program.Flow()
	result := view.FunctionBoundaries()
	if result == nil {
		t.Fatal("Flow did not publish FunctionBoundary")
	}
	provenance := view.Provenance()
	if !functionboundary.Matches(result, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) {
		t.Fatal("published FunctionBoundary did not match its owner quartet")
	}
	foreign := provenance.Source
	foreign[0]++
	if functionboundary.Matches(result, foreign, provenance.Flow, provenance.Static, provenance.Module) {
		t.Fatal("foreign Source identity crossed FunctionBoundary fence")
	}
	if functionboundary.Matches(nil, provenance.Source, provenance.Flow, provenance.Static, provenance.Module) {
		t.Fatal("nil FunctionBoundary matched an owner quartet")
	}
	if _, ok := result.ForBody(keyspace.MakeTerm(keyspace.FamilyFunction, 1)); ok {
		t.Fatal("ForBody accepted a non-Body term")
	}
}
