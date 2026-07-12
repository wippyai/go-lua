package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRelationSCCAcyclicFreezesCalleeBeforeCaller(t *testing.T) {
	reg := standard.Registry()
	calleeBuilder, calleeCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	callerBuilder, callerCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	callee := mustTestRelation(t, calleeBuilder, calleeCertificate, "callee")
	caller := mustTestRelation(t, callerBuilder, callerCertificate, "caller")
	calleeRef := CellRef{Function: 1}
	callerRef := CellRef{Function: 2}
	calleeCalls, callerCalls := 0, 0
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{
		{
			Ref: callerRef, Arena: callerBuilder.Arena(), Shape: caller.Shape(), Dependencies: []CellRef{calleeRef},
			Equation: func(_ context.Context, view RelationView) (Relation, error) {
				callerCalls++
				got, ok := view.Lookup(calleeRef)
				if !ok || !EqualRelation(got, callee) {
					t.Fatalf("caller observed unfrozen callee: ok=%v rows=%d", ok, got.Rows())
				}
				return caller, nil
			},
		},
		{
			Ref: calleeRef, Arena: calleeBuilder.Arena(), Shape: callee.Shape(),
			Equation: func(context.Context, RelationView) (Relation, error) {
				calleeCalls++
				return callee, nil
			},
		},
	}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if calleeCalls != 1 || callerCalls != 1 {
		t.Fatalf("acyclic cells not solved once: callee=%d caller=%d", calleeCalls, callerCalls)
	}
	entries := snapshot.Entries()
	if len(entries) != 2 || entries[0].Ref != calleeRef || entries[1].Ref != callerRef {
		t.Fatalf("unstable snapshot order: %#v", entries)
	}
}

func TestRelationSCCRecursiveGrowthWidensWholeRelation(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	one := mustTestRelation(t, b, certificate, "one")
	two := JoinRelation(one, mustTestRelation(t, b, certificate, "two"))
	three := JoinRelation(two, mustTestRelation(t, b, certificate, "three"))
	ref := CellRef{Function: 7}
	calls := 0
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{{
		Ref: ref, Arena: b.Arena(), Shape: one.Shape(), Dependencies: []CellRef{ref},
		Equation: func(_ context.Context, view RelationView) (Relation, error) {
			calls++
			current, ok := view.Lookup(ref)
			if !ok {
				t.Fatal("recursive dependency unavailable")
			}
			if current.ContextualReason() != "" {
				return current, nil
			}
			switch current.Rows() {
			case 0:
				return one, nil
			case 1:
				return two, nil
			default:
				return three, nil
			}
		},
	}}, RelationSolveOptions{MaxRows: 2, MaxIterations: 16})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snapshot.Lookup(ref)
	if !ok || got.ContextualReason() != "row budget" || !got.Widened() || got.Rows() != 0 {
		t.Fatalf("recursive growth did not widen atomically: ok=%v reason=%q widened=%v rows=%d", ok, got.ContextualReason(), got.Widened(), got.Rows())
	}
	if calls != 3 {
		t.Fatalf("unexpected convergence rounds: %d", calls)
	}
}

func TestRelationSCCForeignArenaFailsClosedInOwner(t *testing.T) {
	reg := standard.Registry()
	owner, ownerCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	foreign, foreignCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	foreignRelation := mustTestRelation(t, foreign, foreignCertificate, "foreign")
	ref := CellRef{Function: 11}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{{
		Ref: ref, Arena: owner.Arena(), Shape: Shape{Params: 1},
		Equation: func(context.Context, RelationView) (Relation, error) { return foreignRelation, nil },
	}}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snapshot.Lookup(ref)
	if !ok || got.arena != owner.Arena() || got.Shape() != (Shape{Params: 1}) || got.ContextualReason() != "foreign relation identity" {
		t.Fatalf("foreign relation escaped or used foreign identity: ok=%v relation=%#v", ok, got)
	}
	_ = ownerCertificate // The owner identity is intentionally allowed to start at bottom.
}

func TestRelationSCCForeignMemberFailsWholeRecursiveComponentClosed(t *testing.T) {
	reg := standard.Registry()
	leftBuilder, leftCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	rightBuilder, rightCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	foreignBuilder, foreignCertificate := emptyBuilder(t, reg, Shape{Params: 3}, nil)
	leftRelation := mustTestRelation(t, leftBuilder, leftCertificate, "left")
	rightRelation := mustTestRelation(t, rightBuilder, rightCertificate, "right")
	foreignRelation := mustTestRelation(t, foreignBuilder, foreignCertificate, "foreign")
	leftRef, rightRef := CellRef{Function: 21}, CellRef{Function: 22}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{
		{
			Ref: leftRef, Arena: leftBuilder.Arena(), Shape: leftRelation.Shape(), Dependencies: []CellRef{rightRef},
			Equation: func(context.Context, RelationView) (Relation, error) { return foreignRelation, nil },
		},
		{
			Ref: rightRef, Arena: rightBuilder.Arena(), Shape: rightRelation.Shape(), Dependencies: []CellRef{leftRef},
			Equation: func(context.Context, RelationView) (Relation, error) { return rightRelation, nil },
		},
	}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	left, leftOK := snapshot.Lookup(leftRef)
	right, rightOK := snapshot.Lookup(rightRef)
	if !leftOK || !rightOK || left.ContextualReason() == "" || right.ContextualReason() == "" {
		t.Fatalf("foreign SCC member did not invalidate the whole component: left=%#v right=%#v", left, right)
	}
	if left.arena != leftBuilder.Arena() || left.Shape() != leftRelation.Shape() || right.arena != rightBuilder.Arena() || right.Shape() != rightRelation.Shape() {
		t.Fatalf("SCC fallback lost per-cell identity: left=%#v right=%#v", left, right)
	}
}

func TestRelationSCCRejectsUndeclaredZeroCellRead(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	relation := mustTestRelation(t, b, certificate, "value")
	ref := CellRef{Function: 30}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{{
		Ref: ref, Arena: b.Arena(), Shape: relation.Shape(),
		Equation: func(_ context.Context, view RelationView) (Relation, error) {
			_, _ = view.Lookup(CellRef{})
			return relation, nil
		},
	}}, RelationSolveOptions{})
	if err == nil {
		t.Fatal("undeclared zero CellRef read was not rejected")
	}
	if len(snapshot.Entries()) != 0 {
		t.Fatalf("invalid dependency transaction published cells: %#v", snapshot.Entries())
	}
}

func TestRelationSCCCancellationPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	firstBuilder, firstCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	secondBuilder, secondCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	first := mustTestRelation(t, firstBuilder, firstCertificate, "first")
	second := mustTestRelation(t, secondBuilder, secondCertificate, "second")
	firstRef, secondRef := CellRef{Function: 1}, CellRef{Function: 2}
	ctx, cancel := context.WithCancel(context.Background())
	snapshot, err := SolveRelationCells(ctx, []RelationCell{
		{Ref: firstRef, Arena: firstBuilder.Arena(), Shape: first.Shape(), Equation: func(context.Context, RelationView) (Relation, error) { return first, nil }},
		{Ref: secondRef, Arena: secondBuilder.Arena(), Shape: second.Shape(), Dependencies: []CellRef{firstRef}, Equation: func(context.Context, RelationView) (Relation, error) {
			cancel()
			return second, nil
		}},
	}, RelationSolveOptions{})
	if err != context.Canceled {
		t.Fatalf("got error %v, want cancellation", err)
	}
	if len(snapshot.Entries()) != 0 {
		t.Fatalf("canceled transaction published partial cells: %#v", snapshot.Entries())
	}
}

func mustTestRelation(t testing.TB, b *Builder, certificate SemanticCertificate, literal string) Relation {
	t.Helper()
	a := b.Arena()
	relation, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{
		Kind: OutputReturn, Descriptor: DescriptorReturn, Value: a.Constant(typevalue.LiteralString(a.reg, literal)),
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	return relation
}
