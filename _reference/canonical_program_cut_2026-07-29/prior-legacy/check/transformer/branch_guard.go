package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// BranchValueTerm resolves the canonical concrete condition source into the
// current relation arena. Resolution owns boundary/path identity; guard
// lowering owns only Lua truthy/falsy polarity.
type BranchValueTerm func(factflow.ValueSource) (ValueTerm, bool)

func lowerBranchConditionGuards(arena *Arena, branch factapply.BranchAlgebra, resolve BranchValueTerm, representedPathEvidence, representedStateRelations, representedSufficientCases bool) (truthy, falsy Guard, err error) {
	if arena == nil || resolve == nil {
		return 0, 0, fmt.Errorf("branch: arena and condition resolver are required")
	}
	for _, reason := range branch.GuardOnlyBlockers() {
		if reason != "branch:refinement" &&
			!(representedPathEvidence && reason == "branch:path-evidence") &&
			!(representedStateRelations && (reason == "branch:presence-relation" || reason == "branch:path-relation")) &&
			!(representedSufficientCases && reason == "branch:sufficient-literal-case") {
			return 0, 0, fmt.Errorf("%s", reason)
		}
	}
	condition, ok := branch.Condition()
	if !ok {
		return 0, 0, fmt.Errorf("branch:missing-condition-source")
	}
	source := condition.Source()
	value, ok := resolve(source)
	if !ok || value == 0 {
		return 0, 0, fmt.Errorf("branch: contextual-condition-source")
	}
	if condition.TruthyOnTrueEdge() {
		return arena.Truthy(value), arena.Falsy(value), nil
	}
	return arena.Falsy(value), arena.Truthy(value), nil
}
