package closed_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// These cases enter the production mounted-artifact receipt assembly and the
// real solver.  The table keeps the semantic witnesses compact while making
// each successor branch observable through the detached result contract.
func TestClosedReceiptSuccessorsRunThroughMountedSolver(t *testing.T) {
	cases := []struct {
		name        string
		source      string
		checkValues bool
	}{
		{
			name:        "source-order-nil-deletion",
			source:      `local child = {}; return { item = child, item = nil }`,
			checkValues: true,
		},
		{
			name:        "diagonal",
			source:      `local a = {}; local x = a; return { [x] = x }`,
			checkValues: true,
		},
		{
			name:        "independent-product",
			source:      `local a = {}; local b = {}; return { [a] = b }`,
			checkValues: true,
		},
		{
			name:   "invalid-key-no-candidate",
			source: `local x = nil; return { [x] = x }`,
		},
		{
			name:        "opaque-containment",
			source:      `local x = {}; return { [x] = x }`,
			checkValues: true,
		},
		{
			name:        "carry-recurrence",
			source:      `local function make() local child = {}; return { item = child } end; return make(), make()`,
			checkValues: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			linked := closedSuccessorLink(t, testCase.source)
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
				t.Fatalf("detached body=%t values=%d", bodyOK, body.ValueCount())
			}
			present := false
			for index := 0; index < body.ValueCount(); index++ {
				_, valuePresent, valueOK := body.ValueAt(index)
				if !valueOK {
					t.Fatalf("detached value %d unreadable", index)
				}
				present = present || valuePresent
			}
			if testCase.checkValues && !present {
				t.Fatal("closed successor published no present value")
			}
		})
	}
}

func closedSuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "closed_successor.lua", Text: []byte(text)})
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
