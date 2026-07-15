package transformer

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestSpecializeObserverCallsPreservesGuardedBindingCorrelationAndNonreturningCalls(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	callerShape, targetShape := Shape{Params: 1}, Shape{Params: 1}
	param := arena.Root(Root{Kind: RootParam})
	owner := testObservationBody(91)
	target := DirectCallTarget{Cell: CellRef{Function: 17}, Shape: targetShape}
	makeCall := func(point cfg.Point, ordinal uint32, guard Guard, value ValueTerm) ObserverCallTemplate {
		return ObserverCallTemplate{
			owner: owner, occurrence: engineobservation.Occurrence{Point: wir.DebugPointID{Ordinal: ordinal, Phase: wir.DebugPhaseCall}, Kind: engineobservation.CallInvocation},
			point: point, target: target, guard: guard, values: []ValueTerm{value}, paths: []PathTerm{0},
		}
	}
	truthy := makeCall(3, 4, arena.Truthy(param), arena.Constant(typevalue.LiteralString(reg, "truthy")))
	falsy := makeCall(3, 4, arena.Falsy(param), arena.Constant(typevalue.LiteralString(reg, "falsy")))
	unreachable := makeCall(9, 10, arena.False(), arena.Constant(typevalue.LiteralString(reg, "impossible")))
	relation := Relation{shape: callerShape, arena: arena, annotations: relationAnnotations{
		calls: unionObserverCallTemplates(arena, []ObserverCallTemplate{unreachable, truthy, falsy}),
	}}
	cursor, err := NewBindingCursor(callerShape, []product.Value{product.Top()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := relation.SpecializeObserverCalls(context.Background(), cursor, SpecializationContext{})
	if err != nil {
		t.Fatal(err)
	}
	items := projection.Items()
	if len(items) != 2 || len(projection.Proof().Decisions) == 0 || projection.TermApplications() != 5 {
		t.Fatalf("observer projection = items:%#v proof:%#v applications:%d", items, projection.Proof(), projection.TermApplications())
	}
	if items[0].Worlds.Root == evaluated.DecisionFalse || items[1].Worlds.Root == evaluated.DecisionFalse || items[0].Worlds == items[1].Worlds ||
		items[0].Target != target || items[1].Target != target || items[0].Owner != owner || items[1].Owner != owner ||
		len(items[0].Values) != 1 || len(items[1].Values) != 1 || product.Equal(reg, items[0].Values[0], items[1].Values[0]) {
		t.Fatalf("guarded call correlation collapsed: %#v", items)
	}
	if len(relation.rows) != 0 || len(relation.ObserverCallTemplates()) != 3 {
		t.Fatal("nonreturning relation lost its owner-local call template")
	}
}

func TestObserverCallTemplatesConvergeInRecursiveSCCWithoutPathGrowth(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	shape := Shape{Params: 1}
	owner := testObservationBody(92)
	ref := CellRef{Function: 23}
	template := ObserverCallTemplate{
		owner: owner, occurrence: engineobservation.Occurrence{Point: wir.DebugPointID{Ordinal: 2, Phase: wir.DebugPhaseCall}, Kind: engineobservation.CallInvocation},
		point: 1, target: DirectCallTarget{Cell: ref, Shape: shape}, guard: arena.True(),
		values: []ValueTerm{arena.Root(Root{Kind: RootParam})}, paths: []PathTerm{arena.Path(Root{Kind: RootParam})},
	}
	bottom := Relation{shape: shape, arena: arena}
	contribution := Relation{shape: shape, arena: arena, annotations: relationAnnotations{calls: []ObserverCallTemplate{template}}}
	evaluations := 0
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{{
		Ref: ref, Arena: arena, Shape: shape, Bottom: bottom, Dependencies: []CellRef{ref},
		Equation: func(_ context.Context, view RelationView) (Relation, error) {
			evaluations++
			current, _ := view.Lookup(ref)
			return JoinRelation(current, contribution), nil
		},
	}}, RelationSolveOptions{MaxRows: 8, MaxIterations: 8})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snapshot.Lookup(ref)
	if !ok || evaluations != 2 || len(got.ObserverCallTemplates()) != 1 || len(got.ObserverCallTemplates()[0].PathTerms()) != 1 {
		t.Fatalf("recursive observer convergence = ok:%v evaluations:%d calls:%#v", ok, evaluations, got.ObserverCallTemplates())
	}
}

func TestObserverCallTemplatePermutationKeepsEqualityAndCanonicalIdentity(t *testing.T) {
	relation, _, cursor, _ := evaluatedRootFixture(t)
	owner := testObservationBody(93)
	param := relation.arena.Root(Root{Kind: RootParam})
	target := DirectCallTarget{Cell: CellRef{Function: 31}, Shape: Shape{Params: 1}}
	first := ObserverCallTemplate{owner: owner, occurrence: engineobservation.Occurrence{Point: wir.DebugPointID{Ordinal: 2, Phase: wir.DebugPhaseCall}, Kind: engineobservation.CallInvocation}, point: 1, target: target, guard: relation.arena.Truthy(param), values: []ValueTerm{param}, paths: []PathTerm{0}}
	second := first
	second.guard = relation.arena.Falsy(param)
	left, right := relation, relation
	left.annotations.calls = unionObserverCallTemplates(relation.arena, []ObserverCallTemplate{first, second})
	right.annotations.calls = unionObserverCallTemplates(relation.arena, []ObserverCallTemplate{second, first})
	if !EqualRelation(left, right) || !reflect.DeepEqual(left.ObserverCallTemplates(), right.ObserverCallTemplates()) {
		t.Fatal("observer call permutation changed relation equality")
	}
	leftAuthority := mustDeriveEvaluatedRootAuthority(t, left, cursor, nil)
	rightAuthority := mustDeriveEvaluatedRootAuthority(t, right, cursor, nil)
	if leftAuthority.RelationIdentity() != rightAuthority.RelationIdentity() {
		t.Fatal("observer call permutation changed canonical relation identity")
	}
}
