package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func certifySummaryKinds(t testing.TB, caps *OutputCapabilityRegistry, kinds ...callboundary.BoundaryFactKind) {
	t.Helper()
	for _, kind := range kinds {
		for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
			if err := caps.SetSummary(kind, lane, CapabilityUnaffected); err != nil {
				t.Fatal(err)
			}
		}
		if err := caps.SetSummary(kind, state.LaneValues, CapabilitySupported); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStructuredSummaryOutputRepresentsValidateGraphCensus(t *testing.T) {
	reg := standard.Registry()
	caps := DefaultOutputCapabilityRegistry()
	certifySummaryKinds(t, caps,
		"Returns",
		"ParamObligations",
		"NormalReturnParams",
		"NormalReturnFacts",
		"HeapTableObjects",
		"ReturnConditionSlotRefinements",
		"ReturnPresenceRelations",
		"MaySuspend",
	)
	b, certificate := emptyBuilder(t, reg, Shape{}, caps)
	a := b.Arena()
	value := typevalue.LiteralString(reg, "value")
	other := typevalue.LiteralString(reg, "other")
	heapID := identity.ID{Kind: "test", Site: "structured-output", Index: 1}
	ks := keyspace.New()
	want := summary.Summary{
		Returns:          []product.Value{value, other},
		ParamObligations: []product.Value{value},
		NormalReturnParams: []product.Value{
			other,
		},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathInvalidations: []callboundary.PathInvalidationFact{{Path: pathdom.Path{Root: "$0"}}},
			PathRefinements:   []callboundary.PathValueFact{{Path: pathdom.Path{Root: "$0"}, Value: value}},
			PathStaticMembers: []callboundary.PathStaticMemberFact{{Path: pathdom.Path{Root: "$0.member"}, Value: other}},
		},
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			heapID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: value}),
		},
		HeapKeySpace: ks,
		ReturnConditionSlotRefinements: []summary.ReturnConditionSlotRefinement{{
			ReturnIndex: 0,
			ReturnValue: true,
			TargetIndex: 1,
			Value:       other,
		}},
		ReturnPresenceRelations: []summary.ReturnPresenceRelation{{
			TriggerIndex:    0,
			TriggerPresence: presence.Present(),
			TargetIndex:     1,
			TargetPresence:  presence.Present(),
		}},
		MaySuspend: true,
	}
	expected := summary.Normalize(reg, want)
	relation, err := b.Build(certificate, []Row{{Guard: a.True(), Output: want}})
	if err != nil {
		t.Fatal(err)
	}
	// Build owns a canonical snapshot; later caller mutations cannot alter it.
	want.Returns[0] = other
	delete(want.HeapTableObjects, heapID)
	cursor, err := NewBindingCursor(relation.Shape(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := relation.Specialize(cursor, nil, nil)
	if !ok || !summary.Equal(reg, got, expected) || got.HeapKeySpace != ks {
		t.Fatalf("structured output mismatch: ok=%v\ngot=%#v\nwant=%#v", ok, got, expected)
	}
}

func TestStructuredSummaryOutputsArePartOfRowIdentity(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{}, nil)
	a := b.Arena()
	yes := typevalue.LiteralString(reg, "yes")
	no := typevalue.LiteralString(reg, "no")
	relation, err := b.Build(certificate, []Row{
		{Guard: a.True(), Output: summary.Summary{Returns: []product.Value{yes}}},
		{Guard: a.True(), Output: summary.Summary{Returns: []product.Value{no}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if relation.Rows() != 2 {
		t.Fatalf("distinct structured outputs collapsed to %d row(s)", relation.Rows())
	}
	cursor, _ := NewBindingCursor(relation.Shape(), nil, nil)
	got, ok := relation.Specialize(cursor, nil, nil)
	want := product.Join(reg, yes, no)
	if !ok || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], want) {
		t.Fatalf("joined structured outputs = %#v/%v", got, ok)
	}
}

func TestStructuredSummaryOutputPreservesAtomicRowCorrelation(t *testing.T) {
	reg := standard.Registry()
	caps := DefaultOutputCapabilityRegistry()
	certifySummaryKinds(t, caps, "MaySuspend")
	b, certificate := emptyBuilder(t, reg, Shape{Params: 1}, caps)
	a := b.Arena()
	p := a.Root(Root{Kind: RootParam})
	yes := typevalue.LiteralString(reg, "yes")
	no := typevalue.LiteralString(reg, "no")
	relation, err := b.Build(certificate, []Row{
		{Guard: a.Truthy(p), Output: summary.Summary{Returns: []product.Value{yes}, MaySuspend: true}},
		{Guard: a.Falsy(p), Output: summary.Summary{Returns: []product.Value{no}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	truthy, _ := NewBindingCursor(relation.Shape(), []product.Value{yes}, nil)
	got, ok := relation.Specialize(truthy, nil, nil)
	if !ok || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], yes) || !got.MaySuspend {
		t.Fatalf("truthy correlated output = %#v/%v", got, ok)
	}
	falsy, _ := NewBindingCursor(relation.Shape(), []product.Value{typevalue.Nil(reg)}, nil)
	got, ok = relation.Specialize(falsy, nil, nil)
	if !ok || len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], no) || got.MaySuspend {
		t.Fatalf("falsy correlated output = %#v/%v", got, ok)
	}
}

func TestStructuredSummaryOutputFailsClosedForUnsupportedField(t *testing.T) {
	reg := standard.Registry()
	b, certificate := emptyBuilder(t, reg, Shape{}, nil)
	a := b.Arena()
	relation, err := b.Build(certificate, []Row{{Guard: a.True(), Output: summary.Summary{MaySuspend: true}}})
	if err == nil || relation.arena != nil {
		t.Fatalf("unsupported structured output published: %#v, %v", relation, err)
	}
}

func TestJoinAndWidenPreserveDeclaredReturnDescriptor(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := CertifyPlan(plan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	declared := typevalue.FromType(reg, typ.String)
	descriptors, err := NewDescriptorRegistry(returnHandler{declared: []product.Value{declared}}, obligationHandler{})
	if err != nil {
		t.Fatal(err)
	}
	builder := NewBuilderWithDescriptors(reg, shape, nil, descriptors, plan)
	actual := typevalue.LiteralInt(reg, 42)
	relation, err := builder.Build(certificate, []Row{{
		Guard: builder.Arena().True(),
		Ops:   []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: builder.Arena().Constant(actual)}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	bottom := Relation{shape: shape, arena: builder.Arena()}
	cursor, _ := NewBindingCursor(shape, nil, nil)
	want, exact := relation.Specialize(cursor, nil, nil)
	if !exact || len(want.Returns) != 1 {
		t.Fatalf("direct declared specialization = %#v/%v", want, exact)
	}
	for name, composed := range map[string]Relation{
		"join":  JoinRelation(bottom, relation),
		"widen": WidenRelation(bottom, relation, 8),
	} {
		got, exact := composed.Specialize(cursor, nil, nil)
		if !exact || len(got.Returns) != 1 {
			t.Fatalf("%s specialization = %#v/%v", name, got, exact)
		}
		if !product.Equal(reg, got.Returns[0], want.Returns[0]) {
			t.Fatalf("%s lost declared return contract\n got=%#v\nwant=%#v", name, got.Returns[0], want.Returns[0])
		}
	}
}
