package causal

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/recurrence"
	"github.com/wippyai/go-lua/analysis/program/flow/routeplan"
)

func TestRoutePlanDeclarationCannotBeSwappedBeforeBinding(t *testing.T) {
	f := openCausalFixture(t, directCallSpec("causal-plan-route-swap.lua"))
	state, err := newSealState(f.sourceView, f.flow, f.bodies, f.forest, f.outcomes, f.control, f.recurrence, f.ports, f.executable, f.entries, f.staticView.ContentID(), f.flow.ModuleID())
	if err != nil {
		t.Fatalf("newSealState: %v", err)
	}
	builder, err := routeplan.New(f.control.Owner())
	if err != nil {
		t.Fatalf("routeplan.New: %v", err)
	}
	state.plan = &planState{builder: builder, arcOrdinal: make([]int, f.control.ArcCount())}
	for index := range state.plan.arcOrdinal {
		state.plan.arcOrdinal[index] = -1
	}
	state.reset.planState, state.boundary.planState = state.plan, state.plan
	for _, phase := range []func() error{state.eval.emitEvaluation, state.structure.emitStructure, state.outcomes.emitOutcomes, state.boundary.emitBoundaries} {
		if err := phase(); err != nil {
			t.Fatalf("causal phase: %v", err)
		}
	}
	plan, err := builder.Seal()
	if err != nil {
		t.Fatalf("route plan seal: %v", err)
	}
	state.plan.plan = plan
	first, second := -1, -1
	for left := range state.edges.edgeRows {
		for right := left + 1; right < len(state.edges.edgeRows); right++ {
			if state.edges.edgeRows[left].From != state.edges.edgeRows[right].From || state.edges.edgeRows[left].To != state.edges.edgeRows[right].To ||
				state.edges.edgeRows[left].Decision != state.edges.edgeRows[right].Decision || state.edges.edgeRows[left].Truth != state.edges.edgeRows[right].Truth {
				first, second = left, right
				break
			}
		}
		if first >= 0 {
			break
		}
	}
	if first < 0 {
		t.Skip("fixture has no distinct local final route declarations")
	}
	recur, binding, err := recurrence.SealWithPlan(f.sourceView, f.flow, f.bodies, f.forest, f.control, plan, f.outcomePhases, f.staticView.ContentID(), f.flow.ModuleID())
	if err != nil {
		t.Fatalf("recurrence.SealWithPlan: %v", err)
	}
	defer binding.Abort(plan)
	state.proof.recur = recur
	state.edges.edgeRows[first].Edge = state.edges.edgeRows[second].Edge
	err = state.finalizeBinding(plan, binding)
	if err == nil || !strings.Contains(err.Error(), "declaration disagrees with final row") {
		t.Fatalf("swapped final route declaration error = %v, want plan-row mismatch", err)
	}
}
