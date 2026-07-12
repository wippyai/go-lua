package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDynamicBranchProofFailsClosedForNonLiteralKey(t *testing.T) {
	reg := standard.Registry()
	caps := DefaultOutputCapabilityRegistry()
	for _, lane := range state.DefaultLanes() {
		_ = caps.SetSummary(callboundary.BoundaryFactKind("NormalReturnFacts"), lane, CapabilitySupported)
	}
	shape := Shape{Params: 2}
	builder, certificate := emptyBuilder(t, reg, shape, caps)
	a := builder.Arena()
	proof := BranchProofTerm{
		Kind:  pathevidence.BranchProofPathPresence,
		Table: a.Path(Root{Kind: RootParam, Index: 0}),
		Key:   a.Root(Root{Kind: RootParam, Index: 1}), Presence: presence.Present(),
	}
	relation, err := builder.Build(certificate, []Row{{Guard: a.True(), Proofs: []BranchProofTerm{proof}}})
	if err != nil {
		t.Fatal(err)
	}
	abstractString := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	cursor, _ := NewBindingCursor(shape, []product.Value{product.Top(), abstractString}, []pathdom.Path{
		pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1),
	})
	if got, ok := relation.Specialize(cursor, nil, nil); ok {
		t.Fatalf("non-literal dynamic proof key published: %#v", got)
	}
}

func TestDynamicBranchProofRetainsCalleePlaceholderNamespace(t *testing.T) {
	reg := standard.Registry()
	caps := DefaultOutputCapabilityRegistry()
	for _, lane := range state.DefaultLanes() {
		_ = caps.SetSummary(callboundary.BoundaryFactKind("NormalReturnFacts"), lane, CapabilitySupported)
	}
	shape := Shape{Params: 1}
	builder, certificate := emptyBuilder(t, reg, shape, caps)
	a := builder.Arena()
	proof := BranchProofTerm{
		Kind:  pathevidence.BranchProofPathPresence,
		Table: a.Path(Root{Kind: RootParam, Index: 0}, segment.Segment{Kind: segment.SegmentField, Name: "references"}),
		Key:   a.Constant(typevalue.LiteralString(reg, "present")), Presence: presence.Present(),
	}
	relation, err := builder.Build(certificate, []Row{{Guard: a.True(), Proofs: []BranchProofTerm{proof}}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _ := NewBindingCursor(shape, []product.Value{product.Top()}, []pathdom.Path{{Root: "caller.self"}})
	got, ok := relation.Specialize(cursor, nil, nil)
	want := pathdom.NewPlaceholder(0).Field("references").IndexStr("present")
	if !ok || len(got.NormalReturnFacts.BranchProofs) != 1 || !got.NormalReturnFacts.BranchProofs[0].Path.Equal(want) {
		t.Fatalf("placeholder proof = %#v/%v, want %s", got.NormalReturnFacts.BranchProofs, ok, want)
	}
}
