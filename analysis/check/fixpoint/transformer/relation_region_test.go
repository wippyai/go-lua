package transformer

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRelationRegionDifferentialRecursiveCellsAndCanonicalOrder(t *testing.T) {
	cells := relationRegionRecursiveFixture(t)
	want, err := SolveRelationCells(context.Background(), []RelationCell{cells[1], cells[0]}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := solveRelationCellsRegion(context.Background(), cells, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertRelationSnapshotsEqual(t, want, got)
	prepared, err := prepareRelationCellsRegion(cells)
	if err != nil {
		t.Fatal(err)
	}
	preparedGot, err := prepared.solve(context.Background(), RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertRelationSnapshotsEqual(t, want, preparedGot)
	reversed, err := solveRelationCellsRegion(context.Background(), []RelationCell{cells[1], cells[0]}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertRelationSnapshotsEqual(t, got, reversed)
	for _, entry := range got.Entries() {
		if entry.Relation.ContextualReason() != "" || entry.Relation.Widened() || entry.Relation.Rows() != 2 {
			t.Fatalf("recursive relation %v = reason %q widened=%v rows=%d", entry.Ref, entry.Relation.ContextualReason(), entry.Relation.Widened(), entry.Relation.Rows())
		}
	}
}

func TestRelationRegionAcyclicCellsEvaluateOnceDependencyFirst(t *testing.T) {
	reg := standard.Registry()
	calleeBuilder, calleeCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	callerBuilder, callerCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	callee := mustTestRelation(t, calleeBuilder, calleeCertificate, "callee")
	caller := mustTestRelation(t, callerBuilder, callerCertificate, "caller")
	calleeRef, callerRef := CellRef{Function: 100}, CellRef{Function: 101}
	var order []CellRef
	cells := []RelationCell{
		{Ref: callerRef, Arena: callerBuilder.Arena(), Shape: caller.Shape(), Dependencies: []CellRef{calleeRef}, Equation: func(_ context.Context, view RelationView) (Relation, error) {
			order = append(order, callerRef)
			got, ok := view.Lookup(calleeRef)
			if !ok || !EqualRelation(got, callee) {
				t.Fatalf("caller observed unstable callee: ok=%v relation=%#v", ok, got)
			}
			return caller, nil
		}},
		{Ref: calleeRef, Arena: calleeBuilder.Arena(), Shape: callee.Shape(), Equation: func(context.Context, RelationView) (Relation, error) {
			order = append(order, calleeRef)
			return callee, nil
		}},
	}
	if _, err := solveRelationCellsRegion(context.Background(), cells, RelationSolveOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != calleeRef || order[1] != callerRef {
		t.Fatalf("acyclic evaluation order=%v, want exactly [%v %v]", order, calleeRef, callerRef)
	}
}

func TestPreparedRelationRegionIsReusableConcurrently(t *testing.T) {
	cells := relationRegionRecursiveFixture(t)
	want, err := SolveRelationCells(context.Background(), cells, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareRelationCellsRegion(cells)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		snapshot RelationSnapshot
		err      error
	}
	results := make(chan outcome, 8)
	for worker := 0; worker < cap(results); worker++ {
		go func() {
			snapshot, solveErr := prepared.solve(context.Background(), RelationSolveOptions{})
			results <- outcome{snapshot: snapshot, err: solveErr}
		}()
	}
	for worker := 0; worker < cap(results); worker++ {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		assertRelationSnapshotsEqual(t, want, result.snapshot)
	}
}

func TestRelationRegionDifferentialContextualWholeRecursiveComponent(t *testing.T) {
	reg := standard.Registry()
	leftBuilder, leftCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	rightBuilder, rightCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	foreignBuilder, foreignCertificate := emptyBuilder(t, reg, Shape{Params: 3}, nil)
	left := mustTestRelation(t, leftBuilder, leftCertificate, "left")
	right := mustTestRelation(t, rightBuilder, rightCertificate, "right")
	foreign := mustTestRelation(t, foreignBuilder, foreignCertificate, "foreign")
	leftRef, rightRef := CellRef{Function: 90}, CellRef{Function: 91}
	cells := []RelationCell{
		{Ref: leftRef, Arena: leftBuilder.Arena(), Shape: left.Shape(), Dependencies: []CellRef{rightRef}, Equation: func(context.Context, RelationView) (Relation, error) { return foreign, nil }},
		{Ref: rightRef, Arena: rightBuilder.Arena(), Shape: right.Shape(), Dependencies: []CellRef{leftRef}, Equation: func(context.Context, RelationView) (Relation, error) { return right, nil }},
	}
	want, err := SolveRelationCells(context.Background(), cells, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := solveRelationCellsRegion(context.Background(), cells, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertRelationSnapshotsEqual(t, want, got)
	for _, entry := range got.Entries() {
		if entry.Relation.ContextualReason() != "foreign relation identity" || !entry.Relation.Widened() || entry.Relation.Rows() != 0 {
			t.Fatalf("whole-SCC contextual flags for %v = reason %q widened=%v rows=%d", entry.Ref, entry.Relation.ContextualReason(), entry.Relation.Widened(), entry.Relation.Rows())
		}
	}
}

func TestRelationRegionDifferentialGuardedCorrelatedRows(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	arena := builder.Arena()
	param := arena.Root(Root{Kind: RootParam, Index: 0})
	truthy := arena.Constant(typevalue.LiteralString(reg, "truthy-result"))
	falsy := arena.Constant(typevalue.LiteralString(reg, "falsy-result"))
	relation, err := builder.Build(certificate, []Row{
		{Guard: arena.Truthy(param), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: truthy}}},
		{Guard: arena.Falsy(param), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: falsy}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := CellRef{Function: 50}
	cells := []RelationCell{{
		Ref: ref, Arena: arena, Shape: relation.Shape(),
		Equation: func(context.Context, RelationView) (Relation, error) { return relation, nil },
	}}
	want, err := SolveRelationCells(context.Background(), cells, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := solveRelationCellsRegion(context.Background(), cells, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertRelationSnapshotsEqual(t, want, got)
	actual, ok := got.Lookup(ref)
	if !ok || actual.Rows() != 2 || actual.ContextualReason() != "" {
		t.Fatalf("guarded correlated rows lost: ok=%v rows=%d reason=%q", ok, actual.Rows(), actual.ContextualReason())
	}
}

func TestRelationRegionDifferentialWidening(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	one := mustTestRelation(t, builder, certificate, "one")
	two := JoinRelation(one, mustTestRelation(t, builder, certificate, "two"))
	three := JoinRelation(two, mustTestRelation(t, builder, certificate, "three"))
	ref := CellRef{Function: 60}
	cells := []RelationCell{{
		Ref: ref, Arena: builder.Arena(), Shape: one.Shape(), Dependencies: []CellRef{ref},
		Equation: func(_ context.Context, view RelationView) (Relation, error) {
			current, _ := view.Lookup(ref)
			switch current.Rows() {
			case 0:
				return one, nil
			case 1:
				return two, nil
			default:
				return three, nil
			}
		},
	}}
	options := RelationSolveOptions{MaxIterations: 16}
	want, err := SolveRelationCells(context.Background(), cells, options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := solveRelationCellsRegion(context.Background(), cells, options)
	if err != nil {
		t.Fatal(err)
	}
	assertRelationSnapshotsEqual(t, want, got)
	widened, ok := got.Lookup(ref)
	if !ok || widened.ContextualReason() != "" || widened.Widened() || widened.Rows() != 3 || widened.arena != builder.Arena() || widened.Shape() != one.Shape() {
		t.Fatalf("exact relation identity/flags differ: ok=%v relation=%#v", ok, widened)
	}
}

func TestRelationRegionDifferentialCancellationPublishesNothing(t *testing.T) {
	run := func(solve func(context.Context, []RelationCell, RelationSolveOptions) (RelationSnapshot, error)) (RelationSnapshot, error) {
		reg := standard.Registry()
		firstBuilder, firstCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
		secondBuilder, secondCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
		first := mustTestRelation(t, firstBuilder, firstCertificate, "first")
		second := mustTestRelation(t, secondBuilder, secondCertificate, "second")
		firstRef, secondRef := CellRef{Function: 1}, CellRef{Function: 2}
		ctx, cancel := context.WithCancel(context.Background())
		return solve(ctx, []RelationCell{
			{Ref: firstRef, Arena: firstBuilder.Arena(), Shape: first.Shape(), Equation: func(context.Context, RelationView) (Relation, error) { return first, nil }},
			{Ref: secondRef, Arena: secondBuilder.Arena(), Shape: second.Shape(), Dependencies: []CellRef{firstRef}, Equation: func(context.Context, RelationView) (Relation, error) {
				cancel()
				return second, nil
			}},
		}, RelationSolveOptions{})
	}
	want, wantErr := run(SolveRelationCells)
	got, gotErr := run(solveRelationCellsRegion)
	if !errors.Is(wantErr, context.Canceled) || !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("cancellation errors old/new=%v/%v", wantErr, gotErr)
	}
	if len(want.Entries()) != 0 || len(got.Entries()) != 0 {
		t.Fatalf("cancellation published old/new=%#v/%#v", want.Entries(), got.Entries())
	}
}

func TestRelationRegionDifferentialMalformedDependencies(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	relation := mustTestRelation(t, builder, certificate, "value")
	ref := CellRef{Function: 70}
	tests := []struct {
		name  string
		cells []RelationCell
	}{
		{
			name: "unknown declaration",
			cells: []RelationCell{{Ref: ref, Arena: builder.Arena(), Shape: relation.Shape(), Dependencies: []CellRef{{Function: 999}}, Equation: func(context.Context, RelationView) (Relation, error) {
				return relation, nil
			}}},
		},
		{
			name: "undeclared dynamic read",
			cells: []RelationCell{{Ref: ref, Arena: builder.Arena(), Shape: relation.Shape(), Equation: func(_ context.Context, view RelationView) (Relation, error) {
				_, _ = view.Lookup(CellRef{})
				return relation, nil
			}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want, wantErr := SolveRelationCells(context.Background(), test.cells, RelationSolveOptions{})
			got, gotErr := solveRelationCellsRegion(context.Background(), test.cells, RelationSolveOptions{})
			if wantErr == nil || gotErr == nil || wantErr.Error() != gotErr.Error() {
				t.Fatalf("malformed errors old/new=%v/%v", wantErr, gotErr)
			}
			if len(want.Entries()) != 0 || len(got.Entries()) != 0 {
				t.Fatalf("malformed solve published old/new=%#v/%#v", want.Entries(), got.Entries())
			}
		})
	}
}

func TestRelationRegionDifferentialEmptyEquationSet(t *testing.T) {
	want, err := SolveRelationCells(context.Background(), nil, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := solveRelationCellsRegion(context.Background(), nil, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertRelationSnapshotsEqual(t, want, got)
}

func relationRegionRecursiveFixture(t testing.TB) []RelationCell {
	t.Helper()
	reg := standard.Registry()
	leftBuilder, leftCertificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	rightBuilder, rightCertificate := emptyBuilder(t, reg, Shape{Params: 2}, nil)
	leftOne := mustTestRelation(t, leftBuilder, leftCertificate, "left-one")
	leftTwo := JoinRelation(leftOne, mustTestRelation(t, leftBuilder, leftCertificate, "left-two"))
	rightOne := mustTestRelation(t, rightBuilder, rightCertificate, "right-one")
	rightTwo := JoinRelation(rightOne, mustTestRelation(t, rightBuilder, rightCertificate, "right-two"))
	leftRef, rightRef := CellRef{Function: 80}, CellRef{Function: 81}
	return []RelationCell{
		{
			Ref: leftRef, Arena: leftBuilder.Arena(), Shape: leftOne.Shape(), Dependencies: []CellRef{rightRef},
			Equation: func(_ context.Context, view RelationView) (Relation, error) {
				right, _ := view.Lookup(rightRef)
				if right.Rows() == 0 {
					return leftOne, nil
				}
				return leftTwo, nil
			},
		},
		{
			Ref: rightRef, Arena: rightBuilder.Arena(), Shape: rightOne.Shape(), Dependencies: []CellRef{leftRef},
			Equation: func(_ context.Context, view RelationView) (Relation, error) {
				left, _ := view.Lookup(leftRef)
				if left.Rows() == 0 {
					return rightOne, nil
				}
				return rightTwo, nil
			},
		},
	}
}

func assertRelationSnapshotsEqual(t testing.TB, want, got RelationSnapshot) {
	t.Helper()
	wantEntries, gotEntries := want.Entries(), got.Entries()
	if len(wantEntries) != len(gotEntries) {
		t.Fatalf("snapshot cardinality old/new=%d/%d", len(wantEntries), len(gotEntries))
	}
	for index := range wantEntries {
		wantRelation, gotRelation := wantEntries[index].Relation, gotEntries[index].Relation
		if wantEntries[index].Ref != gotEntries[index].Ref ||
			wantRelation.arena != gotRelation.arena || wantRelation.effects != gotRelation.effects ||
			wantRelation.descriptors != gotRelation.descriptors || wantRelation.Shape() != gotRelation.Shape() ||
			wantRelation.ContextualReason() != gotRelation.ContextualReason() ||
			wantRelation.Widened() != gotRelation.Widened() ||
			wantRelation.ObservationCoverageComplete() != gotRelation.ObservationCoverageComplete() ||
			!EqualRelation(wantRelation, gotRelation) {
			t.Fatalf("snapshot entry %d differs: old=%#v new=%#v", index, wantEntries[index], gotEntries[index])
		}
	}
}

func BenchmarkRelationRegionTwoCellRecursive(b *testing.B) {
	cells := relationRegionRecursiveFixture(b)
	benchmarkRelationSolvers(b, cells, RelationSolveOptions{})
}

func BenchmarkRelationRegionAcyclic64(b *testing.B) {
	reg := standard.Registry()
	cells := make([]RelationCell, 64)
	for index := range cells {
		builder, certificate := emptyBuilder(b, reg, Shape{Params: 1}, nil)
		relation := mustTestRelation(b, builder, certificate, "acyclic")
		ref := CellRef{Function: uint64(index + 1)}
		var dependencies []CellRef
		if index != 0 {
			dependencies = []CellRef{{Function: uint64(index)}}
		}
		cellDependencies := append([]CellRef(nil), dependencies...)
		cells[index] = RelationCell{
			Ref: ref, Arena: builder.Arena(), Shape: relation.Shape(), Dependencies: cellDependencies,
			Equation: func(_ context.Context, view RelationView) (Relation, error) {
				if len(cellDependencies) != 0 {
					if _, ok := view.Lookup(cellDependencies[0]); !ok {
						return Relation{}, errors.New("missing acyclic dependency")
					}
				}
				return relation, nil
			},
		}
	}
	benchmarkRelationSolvers(b, cells, RelationSolveOptions{})
}

func benchmarkRelationSolvers(b *testing.B, cells []RelationCell, options RelationSolveOptions) {
	b.Helper()
	prepared, err := prepareRelationCellsRegion(cells)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("existing", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := SolveRelationCells(context.Background(), cells, options); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("region", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := solveRelationCellsRegion(context.Background(), cells, options); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("region-prepared", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := prepared.solve(context.Background(), options); err != nil {
				b.Fatal(err)
			}
		}
	})
}
