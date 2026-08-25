package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestDispatchDeclaresValueCalleeAndCallOwnedRoutes(t *testing.T) {
	declaration := Dispatch()
	if problem, ok := declaration.Check(); !ok {
		t.Fatalf("dispatch declaration refused: %+v", problem)
	}
	if len(declaration.Joins) != 2 || declaration.Joins[0].Relation.Member != MountedCallParents || declaration.Joins[1].Relation.Member != DispatchRoutes {
		t.Fatalf("dispatch joins = %+v", declaration.Joins)
	}
	route := declaration.Joins[1]
	if route.Read.Form != ruleprogram.Selected || route.Read.Contract.Order != ruleprogram.OrderByTag || route.Predicate.Member != DispatchRouteTag || len(route.Sources) != 2 || route.Sources[0] != ruleprogram.CandidateSource() || route.Sources[1] != ruleprogram.PriorSource(0) {
		t.Fatalf("dispatch route join = %+v", route)
	}
	if declaration.Fold.Reducer != (member.ReducerRef{Axis: route.Relation.Axis, Member: DispatchReducer}) || len(declaration.Fold.Inputs) != 1 || declaration.Fold.Inputs[0] != 1 || len(declaration.Fold.Outputs) != 1 || declaration.Fold.Outputs[0].Destination.Member != DispatchRouteDestination {
		t.Fatalf("dispatch fold = %+v", declaration.Fold)
	}
}
