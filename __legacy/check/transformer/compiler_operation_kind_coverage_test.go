package transformer

import (
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
)

// TestPlanCompilerSemanticCoverageComplete is the migration tripwire
// between operationplan's closed semantic catalog and the replacement
// compiler. A nil handler is an intentional missing symbolic transaction, not
// an omitted map entry. When either catalog grows, this test requires the new
// family to be classified before it can disappear into contextual fallback.
func TestPlanCompilerSemanticCoverageComplete(t *testing.T) {
	registered := map[operationplan.Kind]bool{
		operationplan.RootAssignment:             true,
		operationplan.PathAssignment:             true,
		operationplan.PathStaticMemberWrite:      true,
		operationplan.DynamicIndexWrite:          true,
		operationplan.PathDescendantInvalidation: true,
		operationplan.BranchEdgeReachability:     true,
		operationplan.BranchConditionSource:      true,
		operationplan.BranchRefinement:           true,
		operationplan.BranchPathEvidence:         true,
		operationplan.Return:                     true,
		operationplan.CallSite:                   true,
		operationplan.ExpressionValue:            true,
		operationplan.ExpressionOperation:        true,
		operationplan.ExpressionFunction:         true,
		operationplan.ExpressionRefinement:       true,
		operationplan.ExpressionPath:             true,
		operationplan.DynamicIndexExpression:     true,
		operationplan.ExpressionCondition:        true,

		operationplan.CovariantExposure:            true,
		operationplan.NoNormalReturn:               true,
		operationplan.BranchPresenceRelation:       true,
		operationplan.BranchPathRelation:           true,
		operationplan.BranchSufficientLiteralCase:  true,
		operationplan.PathValuePresenceImplication: true,
		operationplan.ChannelSelect:                true,
		operationplan.PostconditionRefinement:      true,
		operationplan.PostconditionPathRelation:    true,
		operationplan.CallResultValue:              true,
		operationplan.ReturnPresenceRelation:       true,
		operationplan.ObjectLiteral:                true,
	}
	// Totality is deliberately independent from handler registration. Every
	// family below is backed by a typed transaction in the sole tuple-coordinate
	// program; composite ownership alone never satisfies this ratchet.
	total := make(map[operationplan.Kind]bool, len(registered))
	for kind, present := range registered {
		total[kind] = present
	}

	compiler := NewPlanCompiler()
	kinds := operationplan.Kinds()
	if len(registered) != len(kinds) || len(total) != len(kinds) {
		t.Fatalf("classified operation kinds = registered %d total %d, catalog kinds = %d", len(registered), len(total), len(kinds))
	}
	if len(compiler.facts) != len(kinds) {
		t.Fatalf("compiler operation registrations = %d, catalog kinds = %d", len(compiler.facts), len(kinds))
	}
	var missing []string
	for _, kind := range kinds {
		want, classified := registered[kind]
		if !classified {
			t.Errorf("operation kind %s has no replacement coverage classification", kind)
			continue
		}
		handler, registered := compiler.facts[kind]
		if !registered {
			t.Errorf("operation kind %s is absent from compiler registry", kind)
			continue
		}
		if got := handler != nil; got != want {
			t.Errorf("operation kind %s represented = %t, want %t", kind, got, want)
		}
		if !total[kind] {
			missing = append(missing, kind.String())
		}
		if handler != nil && handler.Kind() != kind {
			t.Errorf("operation kind %s is owned by handler for %s", kind, handler.Kind())
		}
	}

	extensions := operationplan.ExtensionKinds()
	if len(extensions) != 1 || extensions[0] != operationplan.BodyGenericFor {
		t.Fatalf("extension catalog = %v; classify every extension explicitly", extensions)
	}
	handler, extensionRegistered := compiler.extensions[operationplan.BodyGenericFor]
	if !extensionRegistered || handler == nil || handler.Kind() != operationplan.BodyGenericFor {
		t.Fatalf("BodyGenericFor replacement handler = %#v, registered = %t", handler, extensionRegistered)
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		t.Fatalf("replacement PlanCompiler semantic coverage is incomplete (%d families): %s", len(missing), strings.Join(missing, ", "))
	}
}
