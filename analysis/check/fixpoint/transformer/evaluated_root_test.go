package transformer

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEvaluateSparseRootUsesOneConsistentSpecializedWorld(t *testing.T) {
	relation, request, cursor, input := evaluatedRootFixture(t)
	root, err := relation.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{})
	if err != nil {
		t.Fatal(err)
	}
	if root.Authoritative() {
		t.Fatal("shadow root with unavailable canonical identities became authoritative")
	}
	feasiblePoints := 0
	for _, point := range root.Points() {
		if point.Worlds.Root == evaluated.DecisionFalse {
			continue
		}
		feasiblePoints++
	}
	if feasiblePoints < 4 {
		t.Fatalf("only %d point selectors shared the concrete valuation", feasiblePoints)
	}
	boundaries := root.Boundaries()
	if len(boundaries) != 1 || len(boundaries[0].Fragments) != 1 || boundaries[0].Fragments[0].Worlds.Root == evaluated.DecisionFalse {
		t.Fatalf("specialized Return boundary = %#v", boundaries)
	}
	gotSummary := root.Summary()
	if len(gotSummary.Returns) != 1 || !product.Equal(standard.Registry(), gotSummary.Returns[0], input) {
		t.Fatalf("owner summary Returns = %#v, want specialized input", gotSummary.Returns)
	}
	coverage := root.Coverage()
	if !coverage.Complete() || coverage.Required != uint32(len(request.Requirements.Entries(false))) {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestEvaluateSparseRootTopKeepsExclusiveBranchesAsCompactProof(t *testing.T) {
	relation, request, _, _ := evaluatedRootFixture(t)
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{product.Top()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := relation.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Proof().Decisions) == 0 {
		t.Fatal("Top lost compact guard correlation")
	}
	edges := root.Edges()
	if len(edges) != 2 || edges[0].Worlds.Root < 2 || edges[1].Worlds.Root < 2 || edges[0].Worlds.Root == edges[1].Worlds.Root {
		t.Fatalf("exclusive edge world sets = %#v", edges)
	}
}

func TestEvaluateSparseRootIsDeterministicAcrossRowPermutation(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	left, err := relation.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{})
	if err != nil {
		t.Fatal(err)
	}
	permuted := relation
	permuted.rows = append([]Row(nil), relation.rows...)
	for low, high := 0, len(permuted.rows)-1; low < high; low, high = low+1, high-1 {
		permuted.rows[low], permuted.rows[high] = permuted.rows[high], permuted.rows[low]
	}
	right, err := permuted.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left.Proof(), right.Proof()) || !reflect.DeepEqual(left.Points(), right.Points()) ||
		!reflect.DeepEqual(left.Boundaries(), right.Boundaries()) || !reflect.DeepEqual(left.Edges(), right.Edges()) ||
		!reflect.DeepEqual(left.Observations(), right.Observations()) || !reflect.DeepEqual(left.Routes(), right.Routes()) ||
		!summary.EqualNormalized(standard.Registry(), left.Summary(), right.Summary()) {
		t.Fatal("evaluated root changed under row permutation")
	}
}

func TestEvaluateGuardWorldProofFailsClosedOnCanonicalAtomCollision(t *testing.T) {
	key := axis.NewKey[evaluatedCollisionValue]("test.evaluated-collision")
	reg := axis.NewRegistry()
	axis.Register(reg, axis.Spec[evaluatedCollisionValue]{
		Key:      key,
		Bottom:   func() evaluatedCollisionValue { return evaluatedCollisionValue{} },
		Top:      func() evaluatedCollisionValue { return evaluatedCollisionValue{rank: 3} },
		Equal:    func(a, b evaluatedCollisionValue) bool { return a == b },
		LessOrEq: func(a, b evaluatedCollisionValue) bool { return a.rank <= b.rank },
		Join: func(a, b evaluatedCollisionValue) evaluatedCollisionValue {
			if a.rank > b.rank {
				return a
			}
			return b
		},
		Meet: func(a, b evaluatedCollisionValue) evaluatedCollisionValue {
			if a.rank < b.rank {
				return a
			}
			return b
		},
		Hash: func(evaluatedCollisionValue) uint64 { return 0 }, Boundary: axis.PortableIdentity,
		Retention: axis.ImmutableRetention[evaluatedCollisionValue](),
		Canonical: axis.PendingCanonical[evaluatedCollisionValue]("test-only axis"),
	})
	reg.Freeze()
	first := product.Set(reg, product.Top(), key, evaluatedCollisionValue{rank: 1})
	second := product.Set(reg, product.Top(), key, evaluatedCollisionValue{rank: 2})
	if product.Equal(reg, first, second) || product.Hash(reg, first) != product.Hash(reg, second) {
		t.Fatal("test did not force an unequal product hash collision")
	}
	arena := NewArena(reg)
	firstTerm, secondTerm := arena.Constant(first), arena.Constant(second)
	relation := Relation{arena: arena, rows: []Row{{Guard: arena.Truthy(firstTerm)}, {Guard: arena.Truthy(secondTerm)}}, projectionTrace: &sparseProjectionTrace{}}
	cursor, err := NewBindingCursor(Shape{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := newEvaluatedTermEvaluator(context.Background(), arena, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := relation.evaluateGuardWorldProof(context.Background(), evaluator); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("canonical atom collision did not fail closed: %v", err)
	}
}

type evaluatedCollisionValue struct{ rank int }

func TestEvaluateSparseRootRejectsEveryIdentityFenceTransactionally(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	tests := []struct {
		name   string
		mutate func(*EvaluatedRootRequest)
	}{
		{name: "relation", mutate: func(request *EvaluatedRootRequest) {
			request.ExpectedIdentity.Relation = availableEvaluatedRootAuthority(1)
		}},
		{name: "entry", mutate: func(request *EvaluatedRootRequest) {
			request.ExpectedIdentity.Entry = availableEvaluatedRootAuthority(2)
		}},
		{name: "lineage", mutate: func(request *EvaluatedRootRequest) {
			request.ExpectedIdentity.Lineage = availableEvaluatedRootAuthority(3)
		}},
		{name: "call-surface", mutate: func(request *EvaluatedRootRequest) {
			request.ExpectedIdentity.CallSurface[0] ^= 1
		}},
		{name: "schema", mutate: func(request *EvaluatedRootRequest) {
			request.ExpectedIdentity.Schema[0] ^= 1
		}},
		{name: "inventory", mutate: func(request *EvaluatedRootRequest) {
			request.ExpectedIdentity.Inventory[0] ^= 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := request
			test.mutate(&changed)
			root, err := relation.EvaluateSparseRoot(context.Background(), changed, cursor, SpecializationContext{})
			if err == nil || root.Coverage() != (evaluated.Coverage{}) {
				t.Fatalf("mismatched authority published root %#v, %v", root.Coverage(), err)
			}
		})
	}
}

func TestEvaluateSparseRootRejectsSelfConsistentForgedAuthority(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	forged := availableEvaluatedRootAuthority(91)
	request.Identity.Relation = forged
	request.ExpectedIdentity.Relation = forged
	root, err := relation.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{})
	if err == nil || root.Coverage() != (evaluated.Coverage{}) {
		t.Fatalf("forged equal authority published root %#v, %v", root.Coverage(), err)
	}
}

func TestEvaluateSparseRootRejectsStatefulResolverWithoutInvokingIt(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	calls := 0
	root, err := relation.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{
		CellResult: func(CellRef, []product.Value) (product.Value, bool) {
			calls++
			return product.Top(), true
		},
	})
	if err == nil || calls != 0 || root.Coverage() != (evaluated.Coverage{}) {
		t.Fatalf("stateful resolver admission = calls %d, root %#v, %v", calls, root.Coverage(), err)
	}
}

func TestEvaluateSparseRootRejectsForeignBindingRegistryWithoutPanic(t *testing.T) {
	relation, request, _, _ := evaluatedRootFixture(t)
	key := axis.NewKey[evaluatedCollisionValue]("test.evaluated.foreign-binding")
	foreign, err := standard.RegistryWithAxes(axis.Spec[evaluatedCollisionValue]{
		Key: key, Bottom: func() evaluatedCollisionValue { return evaluatedCollisionValue{} },
		Top:      func() evaluatedCollisionValue { return evaluatedCollisionValue{rank: 2} },
		Equal:    func(a, b evaluatedCollisionValue) bool { return a == b },
		LessOrEq: func(a, b evaluatedCollisionValue) bool { return a.rank <= b.rank },
		Join: func(a, b evaluatedCollisionValue) evaluatedCollisionValue {
			if a.rank > b.rank {
				return a
			}
			return b
		},
		Meet: func(a, b evaluatedCollisionValue) evaluatedCollisionValue {
			if a.rank < b.rank {
				return a
			}
			return b
		},
		Hash:     func(value evaluatedCollisionValue) uint64 { return uint64(value.rank) },
		Boundary: axis.PortableIdentity, Retention: axis.ImmutableRetention[evaluatedCollisionValue](),
		Canonical: axis.PendingCanonical[evaluatedCollisionValue]("test-only axis"),
	}.Erase())
	if err != nil {
		t.Fatal(err)
	}
	value := product.Set(foreign, product.Top(), key, evaluatedCollisionValue{rank: 1})
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{value}, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := relation.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{})
	if err == nil || root.Coverage() != (evaluated.Coverage{}) {
		t.Fatalf("foreign binding published root %#v, %v", root.Coverage(), err)
	}
}

func TestEvaluateSparseRootRejectsUnsealedDescriptorRegistry(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	custom, err := NewDescriptorRegistry(returnHandler{}, obligationHandler{})
	if err != nil {
		t.Fatal(err)
	}
	relation.descriptors = custom
	root, err := relation.EvaluateSparseRoot(context.Background(), request, cursor, SpecializationContext{})
	if err == nil || root.Coverage() != (evaluated.Coverage{}) {
		t.Fatalf("unsealed descriptors published root %#v, %v", root.Coverage(), err)
	}
}

func TestEvaluateSparseRootRejectsSlotAndCursorMismatchTransactionally(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	tests := []struct {
		name   string
		mutate func(*Relation) BindingCursor
	}{
		{name: "missing-slot", mutate: func(relation *Relation) BindingCursor {
			relation.projectionTrace = cloneSparseProjectionTrace(relation.projectionTrace)
			relation.projectionTrace.slots = relation.projectionTrace.slots[:len(relation.projectionTrace.slots)-1]
			return cursor
		}},
		{name: "duplicate-slot", mutate: func(relation *Relation) BindingCursor {
			relation.projectionTrace = cloneSparseProjectionTrace(relation.projectionTrace)
			relation.projectionTrace.slots = append(relation.projectionTrace.slots, relation.projectionTrace.slots[len(relation.projectionTrace.slots)-1])
			return cursor
		}},
		{name: "wrong-slot", mutate: func(relation *Relation) BindingCursor {
			relation.projectionTrace = cloneSparseProjectionTrace(relation.projectionTrace)
			relation.projectionTrace.slots[0].requirement = relation.projectionTrace.slots[1].requirement
			return cursor
		}},
		{name: "cursor", mutate: func(_ *Relation) BindingCursor {
			wrong, _ := NewBindingCursor(Shape{}, nil, nil)
			return wrong
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := relation
			changedCursor := test.mutate(&changed)
			root, err := changed.EvaluateSparseRoot(context.Background(), request, changedCursor, SpecializationContext{})
			if err == nil || root.Coverage() != (evaluated.Coverage{}) {
				t.Fatalf("mismatched trace published root %#v, %v", root.Coverage(), err)
			}
		})
	}
}

func TestEvaluateSparseRootCancellationPublishesNothing(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root, err := relation.EvaluateSparseRoot(ctx, request, cursor, SpecializationContext{})
	if err == nil || !strings.Contains(err.Error(), "canceled") || root.Coverage() != (evaluated.Coverage{}) {
		t.Fatalf("canceled conversion = %#v, %v", root.Coverage(), err)
	}
}

func TestEvaluateSparseRootCancelsDuringLargeTermDAG(t *testing.T) {
	relation, request, cursor, _ := evaluatedRootFixture(t)
	terms := make([]ValueTerm, 4096)
	for index := range terms {
		terms[index] = relation.arena.Constant(typevalue.LiteralNumber(relation.arena.reg, float64(index)))
	}
	large := relation.arena.JoinValue(terms...)
	for rowIndex := range relation.rows {
		for operationIndex := range relation.rows[rowIndex].Ops {
			relation.rows[rowIndex].Ops[operationIndex].Value = large
		}
	}
	ctx := &cancelAfterChecksContext{remaining: 80}
	root, err := relation.EvaluateSparseRoot(ctx, request, cursor, SpecializationContext{})
	if err == nil || !strings.Contains(err.Error(), "canceled") || root.Coverage() != (evaluated.Coverage{}) {
		t.Fatalf("mid-DAG cancellation = %#v, %v", root.Coverage(), err)
	}
}

func TestEvaluatedExpressionOwnsScalarLiteralDTOs(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	relation := Relation{arena: arena}
	tests := []struct {
		name  string
		value product.Value
		want  evaluated.Scalar
	}{
		{name: "boolean", value: typevalue.LiteralBool(reg, false), want: evaluated.Scalar{Kind: evaluated.ScalarBoolean}},
		{name: "integer", value: typevalue.LiteralInt(reg, -7), want: evaluated.Scalar{Kind: evaluated.ScalarInteger, Integer: -7}},
		{name: "number", value: typevalue.LiteralNumber(reg, 2.5), want: evaluated.Scalar{Kind: evaluated.ScalarNumber, Number: 2.5}},
		{name: "string", value: typevalue.LiteralString(reg, "string"), want: evaluated.Scalar{Kind: evaluated.ScalarString, String: "string"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var expressions []evaluated.Expression
			_, err := relation.appendEvaluatedExpression(context.Background(), arena.Constant(test.value), make(map[ValueTerm]evaluated.ExpressionID), &expressions)
			if err != nil {
				t.Fatal(err)
			}
			if len(expressions) != 1 || expressions[0].Scalar != test.want || !product.Equal(reg, expressions[0].Constant, product.Top()) {
				t.Fatalf("scalar expression = %#v, want %#v", expressions, test.want)
			}
		})
	}
}

type cancelAfterChecksContext struct{ remaining int }

func (c *cancelAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterChecksContext) Done() <-chan struct{}       { return nil }
func (c *cancelAfterChecksContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}
func (c *cancelAfterChecksContext) Value(any) any { return nil }

func evaluatedRootFixture(tb testing.TB) (Relation, EvaluatedRootRequest, BindingCursor, product.Value) {
	tb.Helper()
	graph, branch, _, _, ret, requirements := sparseProjectionReturnFixture(tb)
	reg := standard.Registry()
	arena := NewArena(reg)
	param := arena.Root(Root{Kind: RootParam})
	builder, err := newSparseProjectionTraceBuilder(arena, requirements)
	if err != nil {
		tb.Fatal(err)
	}
	tape, err := compileSymbolicWTOTape(graph)
	if err != nil {
		tb.Fatal(err)
	}
	exit, err := solveExactWTOCFGExpandedExitRowsWithTrace(context.Background(), graph, tape, arena,
		SymbolicCFGRow{Guard: arena.True()},
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			if point == ret {
				row.Operations = append(row.Operations, Operation{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: param})
			}
			return []SymbolicCFGRow{row}, nil
		},
		func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
			if point != branch {
				tb.Fatalf("unexpected branch point %d", point)
			}
			if cond {
				return row, arena.Truthy(param), nil
			}
			return row, arena.Falsy(param), nil
		}, SymbolicExactWTOOptions{SymbolicCFGOptions: SymbolicCFGOptions{Shape: Shape{Params: 1}}}, builder)
	if err != nil {
		tb.Fatal(err)
	}
	trace, err := builder.freeze()
	if err != nil {
		tb.Fatal(err)
	}
	rows := make([]Row, len(exit))
	for index, row := range exit {
		rows[index] = Row{Guard: row.Guard, Output: row.Output, Ops: row.Operations}
	}
	relation := Relation{
		shape: Shape{Params: 1}, arena: arena, descriptors: DefaultDescriptorRegistry(), rows: rows,
		observationComplete: true, projectionTrace: trace,
	}
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 37
	surface, err := operationplan.SealCallSurface(owner, graph.Size(), nil, nil)
	if err != nil {
		tb.Fatal(err)
	}
	identity := evaluated.Identity{
		Body: owner, Relation: evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		Entry:       evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		Lineage:     evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		Registry:    evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable},
		CallSurface: surface.Digest(), Schema: requirements.SchemaID(),
		Inventory: requirements.ConsumerInventoryID(), PointCount: uint32(graph.Size()),
	}
	identity.View, err = evaluated.SealProjectionView(requirements, false)
	if err != nil {
		tb.Fatal(err)
	}
	request := EvaluatedRootRequest{
		Identity: identity, ExpectedIdentity: identity, Requirements: requirements, CallSurface: surface,
	}
	input := typevalue.String(reg)
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{input}, nil)
	if err != nil {
		tb.Fatal(err)
	}
	return relation, request, cursor, input
}

func availableEvaluatedRootAuthority(seed byte) evaluated.AuthorityDigest {
	value := evaluated.Digest{}
	value[0] = seed
	return evaluated.AuthorityDigest{Status: evaluated.AuthorityAvailable, Value: value}
}

var evaluatedRootBenchmarkSink evaluated.Root

func BenchmarkEvaluateSparseRoot(b *testing.B) {
	relation, request, cursor, _ := evaluatedRootFixture(b)
	ctx := context.Background()
	b.Run("transient", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			root, err := relation.EvaluateSparseRoot(ctx, request, cursor, SpecializationContext{})
			if err != nil {
				b.Fatal(err)
			}
			evaluatedRootBenchmarkSink = root
		}
	})
	b.Run("retained-128", func(b *testing.B) {
		b.ReportAllocs()
		ring := make([]evaluated.Root, 128)
		index := 0
		for i := 0; i < b.N; i++ {
			root, err := relation.EvaluateSparseRoot(ctx, request, cursor, SpecializationContext{})
			if err != nil {
				b.Fatal(err)
			}
			ring[index&127] = root
			index++
		}
		evaluatedRootBenchmarkSink = ring[(index-1)&127]
	})
}
