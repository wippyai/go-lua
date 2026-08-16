package bootstrap_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/target/profile"
)

// Link bootstrap is validated only after the complete value (header,
// mutability, and raw absence/presence) has crossed receipt assembly and the
// solver has published a detached result.
func TestBootstrapReceiptStagesCompleteValueThroughMountedSolver(t *testing.T) {
	linked := bootstrapSuccessorLink(t, `local missing = nil; local number = 1; return number`)
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
		t.Fatalf("bootstrap body=%t values=%d", bodyOK, body.ValueCount())
	}
	present := false
	for index := 0; index < body.ValueCount(); index++ {
		_, valuePresent, valueOK := body.ValueAt(index)
		if !valueOK {
			t.Fatalf("bootstrap value[%d] unreadable", index)
		}
		present = present || valuePresent
	}
	if !present {
		t.Fatal("bootstrap did not publish any present detached value")
	}
}

func bootstrapSuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "bootstrap_successor.lua", Text: []byte(text)})
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
