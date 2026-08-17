package ingress_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// The zero-input ingress must seed the exact WorldZero root in a real
// assembled solve.  The returned table makes the ingress value observable at
// the public result boundary without exposing private Heap coordinates.
func TestIngressReceiptWorldZeroRunsThroughMountedSolver(t *testing.T) {
	linked := ingressSuccessorLink(t, `return {}`)
	plan, compileStatus := analysis.Compile(linked)
	if compileStatus != analysis.CompileComplete || plan == nil {
		t.Fatalf("receipt compile=%v plan=%t", compileStatus, plan != nil)
	}
	defer plan.Close()
	result, solveStatus := plan.Solve(context.Background())
	if solveStatus != analysis.AnalyzeComplete || result == nil {
		t.Fatalf("mounted solver status=%v result=%t", solveStatus, result != nil)
	}
	body, bodyOK := result.BodyAt(0)
	if !bodyOK || body.ValueCount() == 0 {
		t.Fatalf("ingress body=%t values=%d", bodyOK, body.ValueCount())
	}
	present := false
	for index := 0; index < body.ValueCount(); index++ {
		_, valuePresent, valueOK := body.ValueAt(index)
		if !valueOK {
			t.Fatalf("ingress value[%d] unreadable", index)
		}
		present = present || valuePresent
	}
	if !present {
		t.Fatal("WorldZero ingress did not publish a present root")
	}
}

func ingressSuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "ingress_successor.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	return linked
}
