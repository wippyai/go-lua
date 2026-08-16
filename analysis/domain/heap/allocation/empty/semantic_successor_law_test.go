package empty_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/target/profile"
)

// Empty self-create is exercised through the mounted receipt assembly.  The
// closure and table share the same artifact but must retain their distinct
// ineligible/eligible Heap shapes while the solver carries the result.
func TestEmptyReceiptSelfCreateRunsThroughMountedSolver(t *testing.T) {
	linked := emptySuccessorLink(t, `local function make() return {} end; local closure = function() end; return make(), make(), closure`)
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
	if !bodyOK || body.ValueCount() < 3 {
		t.Fatalf("empty self-create body=%t values=%d", bodyOK, body.ValueCount())
	}
	for index := 0; index < body.ValueCount(); index++ {
		if _, present, valueOK := body.ValueAt(index); !valueOK || !present {
			t.Fatalf("empty self-create value[%d]=%t/%t", index, present, valueOK)
		}
	}
}

func emptySuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "empty_successor.lua", Text: []byte(text)})
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
