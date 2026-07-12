package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func emptyBuilder(t testing.TB, reg *axis.Registry, shape Shape, caps *OutputCapabilityRegistry) (*Builder, SemanticCertificate) {
	t.Helper()
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := CertifyPlan(plan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return NewBuilder(reg, shape, caps, plan), certificate
}

func TestArenaHashConsesValuePathAndGuardDAGs(t *testing.T) {
	reg := standard.Registry()
	a := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	x, y := a.Root(root), a.Root(root)
	if x != y {
		t.Fatal("root was not hash-consed")
	}
	c := a.Constant(typevalue.LiteralString(reg, "x"))
	if got := a.JoinValue(x, c, x); got != a.JoinValue(c, x) {
		t.Fatal("join is not canonical")
	}
	g := a.And(a.Truthy(x), a.Falsy(c), a.Truthy(x))
	if g != a.And(a.Falsy(c), a.Truthy(x)) {
		t.Fatal("guard DAG is not canonical")
	}
	pathA := a.Path(root, segment.Segment{Kind: segment.SegmentField, Name: "field"})
	pathB := a.Path(root, segment.Segment{Kind: segment.SegmentField, Name: "field"})
	if pathA != pathB {
		t.Fatal("path was not hash-consed")
	}
	values := []product.Value{typevalue.LiteralString(reg, "value")}
	paths := []pathdom.Path{{Root: "p"}}
	cursor, _ := NewBindingCursor(Shape{Params: 1}, values, paths)
	bound, ok := a.evalPath(pathA, cursor)
	if !ok || bound.Root != "p" || len(bound.Segments) != 1 || bound.Segments[0].Name != "field" {
		t.Fatalf("path specialization failed: %#v", bound)
	}
}

func TestCapabilityMatrixCoversEveryDefaultStateLane(t *testing.T) {
	r := DefaultOutputCapabilityRegistry()
	if err := r.Complete(state.DefaultLaneCatalog()); err != nil {
		t.Fatal(err)
	}
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("test assumption: got %d State lanes", len(lanes))
	}
	for _, lane := range lanes {
		if got := r.Capability(OutputReturn, lane); got == CapabilityUnsupported {
			t.Fatalf("return lane %q has no explicit verdict", lane)
		}
		if got := r.Capability(OutputEffect, lane); got != CapabilityUnsupported {
			t.Fatalf("effect lane %q unexpectedly enabled", lane)
		}
	}
}

func TestDefaultCapabilityRegistriesKeepMutableMatricesIndependent(t *testing.T) {
	outputA, outputB := DefaultOutputCapabilityRegistry(), DefaultOutputCapabilityRegistry()
	if err := outputA.Set(OutputEffect, state.LaneValues, CapabilitySupported); err != nil {
		t.Fatal(err)
	}
	if got := outputB.Capability(OutputEffect, state.LaneValues); got != CapabilityUnsupported {
		t.Fatalf("output default mutation leaked: got capability %d", got)
	}

	semanticA, semanticB := DefaultSemanticCapabilityRegistry(), DefaultSemanticCapabilityRegistry()
	kind := operationplan.Kinds()[0]
	if err := semanticA.SetFact(kind, state.LaneValues, CapabilitySupported); err != nil {
		t.Fatal(err)
	}
	if got := semanticB.Fact(kind, state.LaneValues); got != CapabilityUnsupported {
		t.Fatalf("semantic default mutation leaked: got capability %d", got)
	}
}

func TestOutputCapabilityMatrixCoversEverySummaryFieldAndStateLane(t *testing.T) {
	r := DefaultOutputCapabilityRegistry()
	if err := r.Complete(state.DefaultLaneCatalog()); err != nil {
		t.Fatal(err)
	}
	descriptors := summary.SummaryFactDescriptors()
	wantKinds := make([]callboundary.BoundaryFactKind, len(descriptors))
	for i, descriptor := range descriptors {
		wantKinds[i] = descriptor.Kind
	}
	if got := r.SummaryKinds(); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("Summary output catalog = %v, want %v", got, wantKinds)
	}
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("test assumption: got %d State lanes", len(lanes))
	}
	for _, kind := range wantKinds {
		for _, lane := range lanes {
			got := r.SummaryCapability(kind, lane)
			switch kind {
			case callboundary.BoundaryFactKind(DescriptorReturn), callboundary.BoundaryFactKind(DescriptorObligation):
				if got == CapabilityUnsupported {
					t.Fatalf("enabled Summary output %q lane %q has no verdict", kind, lane)
				}
			default:
				if got != CapabilityUnsupported {
					t.Fatalf("unimplemented Summary output %q lane %q unexpectedly enabled", kind, lane)
				}
			}
		}
	}
}

func TestSemanticCapabilityMatrixCoversOperationPlanAndFailsClosed(t *testing.T) {
	r := DefaultSemanticCapabilityRegistry()
	if err := r.Complete(state.DefaultLaneCatalog()); err != nil {
		t.Fatal(err)
	}
	for _, kind := range operationplan.Kinds() {
		for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
			if got := r.Fact(kind, lane); got != CapabilityUnsupported {
				t.Fatalf("fact %d lane %q unexpectedly enabled", kind, lane)
			}
		}
	}
	for _, extension := range operationplan.ExtensionKinds() {
		for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
			if got := r.Extension(extension, lane); got != CapabilityUnsupported {
				t.Fatalf("extension %d lane %q unexpectedly enabled", extension, lane)
			}
		}
	}
	if got := r.Fact(operationplan.ExpressionCondition+1, state.LaneValues); got != CapabilityUnsupported {
		t.Fatalf("future operation-plan kind got capability %d", got)
	}
	if got := r.Fact(operationplan.Return, state.LaneID("future-lane")); got != CapabilityUnsupported {
		t.Fatalf("future State lane got capability %d", got)
	}
}

func TestCellResultRequiresExplicitAllLaneCertificate(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam})
	call := a.CellResultValue(CellRef{Function: 1}, p)
	if relation, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputCellResult, Descriptor: DescriptorReturn, Value: call}}}}); err == nil || relation.arena != nil {
		t.Fatalf("uncertified scalar cell result published: %#v, %v", relation, err)
	}
}

func TestCellResultCannotBypassCapabilityThroughReturnOrGuard(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam})
	call := a.CellResultValue(CellRef{Function: 1}, p)
	if relation, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: call}}}}); err == nil || relation.arena != nil {
		t.Fatalf("cell result escaped through return capability: %#v, %v", relation, err)
	}
	if relation, err := b.Build(certificate, []Row{{Guard: a.Truthy(call), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: p}}}}); err == nil || relation.arena != nil {
		t.Fatalf("cell result escaped through guard capability: %#v, %v", relation, err)
	}
}

func TestCertificateCoversPresentCellsAndDependencies(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	plan := operationplan.New(graph, factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{point: {}}, ExpressionValues: map[factflow.ExprRef]product.Value{1: product.Top()}})
	registry := DefaultSemanticCapabilityRegistry()
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		if err := registry.SetFact(operationplan.RootAssignment, lane, CapabilityUnaffected); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CertifyPlan(plan, registry); err == nil {
		t.Fatal("present expression dependency escaped certification")
	}
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		if err := registry.SetFact(operationplan.ExpressionValue, lane, CapabilityUnaffected); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CertifyPlan(plan, registry); err != nil {
		t.Fatalf("fully certified plan rejected: %v", err)
	}
	b := NewBuilder(standard.Registry(), Shape{}, nil, plan)
	if relation, err := b.Build(SemanticCertificate{}, nil); err == nil || relation.arena != nil {
		t.Fatalf("missing certificate published relation: %#v, %v", relation, err)
	}
}

func TestBuilderRejectsCertificateFromDifferentOperationPlan(t *testing.T) {
	planA := operationplan.New(cfg.New(), factflow.FactsInput{})
	planB := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificateA, err := CertifyPlan(planA, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	certificateB, err := CertifyPlan(planB, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(standard.Registry(), Shape{}, nil, planB)
	a := b.Arena()
	rows := []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: a.Constant(product.Top())}}}}
	if relation, err := b.Build(certificateA, rows); err == nil || relation.arena != nil {
		t.Fatalf("foreign operation-plan certificate published relation: %#v, %v", relation, err)
	}
	if relation, err := b.Build(certificateB, rows); err != nil || relation.arena == nil {
		t.Fatalf("matching operation-plan certificate rejected: %#v, %v", relation, err)
	}
}

func TestBuilderWithoutOperationPlanProvenanceFailsClosed(t *testing.T) {
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := CertifyPlan(plan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(standard.Registry(), Shape{}, nil, nil)
	a := b.Arena()
	rows := []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: a.Constant(product.Top())}}}}
	if relation, err := b.Build(certificate, rows); err == nil || relation.arena != nil {
		t.Fatalf("provenance-free builder published relation: %#v, %v", relation, err)
	}
}

func TestCertificateCoversPresentHigherLayerExtensions(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	plan := operationplan.New(graph, factflow.FactsInput{}).WithExtensions([]operationplan.ExtensionInput{{Point: point, Kind: operationplan.BodyGenericFor}})
	registry := DefaultSemanticCapabilityRegistry()
	if _, err := CertifyPlan(plan, registry); err == nil {
		t.Fatal("present higher-layer extension escaped certification")
	}
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		if err := registry.SetExtension(operationplan.BodyGenericFor, lane, CapabilityUnaffected); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CertifyPlan(plan, registry); err != nil {
		t.Fatalf("fully certified extension rejected: %v", err)
	}
}

func TestBuildFailsAtomicallyForConditionalObligation(t *testing.T) {
	b, certificate := emptyBuilder(t, standard.Registry(), Shape{Params: 1}, nil)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	got, err := b.Build(certificate, []Row{{Guard: a.Truthy(p), Ops: []Operation{{Kind: OutputObligation, Descriptor: DescriptorObligation, Value: p}}}})
	if err == nil || got.arena != nil {
		t.Fatalf("conditional obligation partially published: %#v, %v", got, err)
	}
}

func TestBuildFailsClosedForUnsupportedOperationLane(t *testing.T) {
	b, certificate := emptyBuilder(t, standard.Registry(), Shape{Params: 1}, nil)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	if got, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputEffect, Descriptor: "effect", Value: p}}}}); err == nil || got.arena != nil {
		t.Fatalf("unsupported effect partially published: %#v, %v", got, err)
	}
}

func TestBuildFailsAtomicallyForUnknownSummaryOutput(t *testing.T) {
	caps := DefaultOutputCapabilityRegistry()
	for _, lane := range caps.Lanes() {
		if err := caps.Set(OutputEffect, lane, CapabilityUnaffected); err != nil {
			t.Fatal(err)
		}
	}
	b, certificate := emptyBuilder(t, standard.Registry(), Shape{Params: 1}, caps)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	got, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputEffect, Descriptor: "future-summary-field", Value: p}}}})
	if err == nil || got.arena != nil {
		t.Fatalf("unknown Summary output partially published: %#v, %v", got, err)
	}
}

func TestBuildFailsAtomicallyWhenCertifiedSummaryOutputHasNoHandler(t *testing.T) {
	caps := DefaultOutputCapabilityRegistry()
	kind := callboundary.BoundaryFactKind("MaySuspend")
	for _, lane := range caps.Lanes() {
		if err := caps.SetSummary(kind, lane, CapabilityUnaffected); err != nil {
			t.Fatal(err)
		}
	}
	b, certificate := emptyBuilder(t, standard.Registry(), Shape{Params: 1}, caps)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	got, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorKind(kind), Value: p}}}})
	if err == nil || got.arena != nil {
		t.Fatalf("handlerless Summary output partially published: %#v, %v", got, err)
	}
}

func TestSpecializePreservesCorrelatedRowsAndExistingSummary(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1, Results: 2}, nil)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	yesValue := typevalue.LiteralString(reg, "yes")
	noValue := typevalue.LiteralString(reg, "no")
	yes := a.Constant(yesValue)
	no := a.Constant(noValue)
	relation, err := b.Build(certificate, []Row{
		{Guard: a.Truthy(p), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: yes}, {Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: p}}},
		{Guard: a.Falsy(p), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: no}, {Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: p}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings := []product.Value{typevalue.LiteralString(reg, "input"), product.Bottom(reg), product.Bottom(reg)}
	cursor, err := NewBindingCursor(relation.Shape(), bindings, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := relation.Specialize(cursor, nil, nil)
	if !ok {
		t.Fatal("specialization fell back")
	}
	if len(got.Returns) != 2 || !product.Equal(reg, got.Returns[0], yesValue) || !product.Equal(reg, got.Returns[1], bindings[0]) {
		t.Fatalf("unexpected summary: %#v", got)
	}
	want := summary.Summary{Returns: []product.Value{yesValue, bindings[0]}}
	if !summary.Equal(reg, got, want) {
		t.Fatal("specialization did not emit existing Summary semantics")
	}
}

func TestScalarCellResultReferenceUsesResolver(t *testing.T) {
	reg := standard.Registry()
	caps := DefaultOutputCapabilityRegistry()
	for _, lane := range caps.Lanes() {
		if err := caps.Set(OutputCellResult, lane, CapabilityUnaffected); err != nil {
			t.Fatal(err)
		}
	}
	if err := caps.Set(OutputCellResult, state.LaneValues, CapabilitySupported); err != nil {
		t.Fatal(err)
	}
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, caps)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	cell := CellRef{Function: 9, Slot: 2}
	call := a.CellResultValue(cell, p)
	relation, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputCellResult, Descriptor: DescriptorReturn, Value: call}}}})
	if err != nil {
		t.Fatal(err)
	}
	input := typevalue.LiteralString(reg, "arg")
	cursor, _ := NewBindingCursor(relation.Shape(), []product.Value{input}, nil)
	seen := false
	got, ok := relation.Specialize(cursor, nil, func(ref CellRef, args []product.Value) (product.Value, bool) {
		seen = ref == cell && len(args) == 1 && product.Equal(reg, args[0], input)
		return args[0], seen
	})
	if !ok || !seen || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], input) {
		t.Fatalf("scalar cell result failed: ok=%v seen=%v summary=%#v", ok, seen, got)
	}
}

func TestRelationJoinAndWidenLaws(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	c1 := a.Constant(typevalue.LiteralString(reg, "a"))
	c2 := a.Constant(typevalue.LiteralString(reg, "b"))
	makeRelation := func(v ValueTerm) Relation {
		r, err := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: v}}}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	x, y, z := makeRelation(p), makeRelation(c1), makeRelation(c2)
	if !EqualRelation(JoinRelation(x, x), x) {
		t.Fatal("join idempotence")
	}
	if !EqualRelation(JoinRelation(x, y), JoinRelation(y, x)) {
		t.Fatal("join commutativity")
	}
	if !EqualRelation(JoinRelation(JoinRelation(x, y), z), JoinRelation(x, JoinRelation(y, z))) {
		t.Fatal("join associativity")
	}
	if !LessOrEqRelation(x, JoinRelation(x, y)) {
		t.Fatal("join upper bound")
	}
	top := WidenRelation(JoinRelation(x, y), z, 2)
	if top.ContextualReason() == "" || !top.Widened() || !LessOrEqRelation(x, top) {
		t.Fatal("widen must fail closed to top")
	}
}

func TestDeterministicRowNormalization(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, nil)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam, Index: 0})
	x := a.Constant(typevalue.LiteralString(reg, "x"))
	left, _ := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: p}, {Kind: OutputReturn, Descriptor: DescriptorReturn, Value: x}}}})
	right, _ := b.Build(certificate, []Row{{Guard: a.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: x}, {Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: p}}}})
	if !EqualRelation(left, right) {
		t.Fatal("operation insertion order changed relation")
	}
}

func TestBindingCursorRejectsBadWidthAndReadsDenseNamespaces(t *testing.T) {
	shape := Shape{Params: 1, Captures: 1, Globals: 1, Results: 1, HeapTemplates: 1}
	values := make([]product.Value, shape.ValueCount())
	cursor, err := NewBindingCursor(shape, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	for kind := RootParam; kind < rootKindCount; kind++ {
		if _, ok := cursor.Value(Root{Kind: kind}); !ok {
			t.Fatalf("root kind %d unreadable", kind)
		}
	}
	if _, err := NewBindingCursor(shape, values[:4], nil); err == nil {
		t.Fatal("bad packed width accepted")
	}
}

func BenchmarkBindingCursorValue(b *testing.B) {
	shape := Shape{Params: 4, Captures: 2, Globals: 2, Results: 2, HeapTemplates: 2}
	values := make([]product.Value, shape.ValueCount())
	cursor, _ := NewBindingCursor(shape, values, nil)
	root := Root{Kind: RootGlobal, Index: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = cursor.Value(root)
	}
}

func BenchmarkSpecializeSimple(b *testing.B) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(b, reg, Shape{Params: 1}, nil)
	arena := builder.Arena()
	p := arena.Root(Root{Kind: RootParam})
	relation, err := builder.Build(certificate, []Row{{Guard: arena.True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: p}}}})
	if err != nil {
		b.Fatal(err)
	}
	cursor, _ := NewBindingCursor(relation.Shape(), []product.Value{typevalue.LiteralString(reg, "x")}, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, ok := relation.Specialize(cursor, nil, nil); !ok {
			b.Fatal("fallback")
		}
	}
}
