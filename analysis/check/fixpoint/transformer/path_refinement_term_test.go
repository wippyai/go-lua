package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func pathRefinementCapabilities(t testing.TB) *OutputCapabilityRegistry {
	t.Helper()
	caps := DefaultOutputCapabilityRegistry()
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		if err := caps.SetSummary("NormalReturnFacts", lane, CapabilitySupported); err != nil {
			t.Fatal(err)
		}
	}
	return caps
}

func TestPreservedParamRootRefinementRemainsSpecializationMetadata(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	builder, certificate := emptyBuilder(t, reg, shape, pathRefinementCapabilities(t))
	arena := builder.Arena()
	root := Root{Kind: RootParam, Index: 0}
	value := arena.Root(root)
	relation, err := builder.Build(certificate, []Row{{
		Guard:           arena.True(),
		Ops:             []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: value}},
		PathRefinements: []PathRefinementTerm{{Path: arena.Path(root), Value: value}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	argument := typevalue.LiteralString(reg, "caller-value")
	cursor, err := NewBindingCursor(shape, []product.Value{argument}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := relation.Specialize(cursor, nil, nil)
	if !ok {
		t.Fatal("preserved identity refinement fell back")
	}
	want := summary.Normalize(reg, summary.Summary{
		Returns: []product.Value{argument},
	})
	if !summary.Equal(reg, got, want) {
		t.Fatalf("generic specialized identity = %#v, want canonical base summary %#v", got, want)
	}
	detailed, ok := relation.SpecializeDetailed(cursor, nil, SpecializationContext{})
	if !ok || !summary.Equal(reg, detailed.Summary, want) || len(detailed.PreservedParams) != 1 || detailed.PreservedParams[0] != 0 {
		t.Fatalf("detailed identity = %#v/%v, want canonical Summary plus preserved param 0", detailed, ok)
	}
}

func TestPreservedParamRootsIntersectAcrossFeasibleRows(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	builder, certificate := emptyBuilder(t, reg, shape, pathRefinementCapabilities(t))
	arena := builder.Arena()
	root := Root{Kind: RootParam, Index: 0}
	value := arena.Root(root)
	refinement := PathRefinementTerm{Path: arena.Path(root), Value: value}
	relation, err := builder.Build(certificate, []Row{
		{Guard: arena.Truthy(value), PathRefinements: []PathRefinementTerm{refinement}},
		{Guard: arena.Falsy(value)},
	})
	if err != nil {
		t.Fatal(err)
	}
	unknown := typevalue.FromType(reg, typ.Boolean)
	cursor, _ := NewBindingCursor(shape, []product.Value{unknown}, nil)
	detailed, ok := relation.SpecializeDetailed(cursor, nil, SpecializationContext{})
	if !ok || len(detailed.PreservedParams) != 0 {
		t.Fatalf("multi-row preservation = %#v/%v, want empty must-intersection", detailed.PreservedParams, ok)
	}
	truthy, _ := NewBindingCursor(shape, []product.Value{typevalue.LiteralBool(reg, true)}, nil)
	detailed, ok = relation.SpecializeDetailed(truthy, nil, SpecializationContext{})
	if !ok || len(detailed.PreservedParams) != 1 || detailed.PreservedParams[0] != 0 {
		t.Fatalf("single feasible row preservation = %#v/%v, want [0]", detailed.PreservedParams, ok)
	}
}

func TestPreservedParamRootRefinementRejectsUnprovedPathShapesAndAliases(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 2}
	tests := []struct {
		name  string
		build func(*Arena) PathRefinementTerm
	}{
		{name: "descendant", build: func(a *Arena) PathRefinementTerm {
			root := Root{Kind: RootParam, Index: 0}
			return PathRefinementTerm{Path: a.Path(root, segment.Segment{Kind: segment.SegmentField, Name: "x"}), Value: a.Root(root)}
		}},
		{name: "different-param-value", build: func(a *Arena) PathRefinementTerm {
			return PathRefinementTerm{Path: a.Path(Root{Kind: RootParam, Index: 0}), Value: a.Root(Root{Kind: RootParam, Index: 1})}
		}},
		{name: "computed-value", build: func(a *Arena) PathRefinementTerm {
			root := Root{Kind: RootParam, Index: 0}
			return PathRefinementTerm{Path: a.Path(root), Value: a.JoinValue(a.Root(root), a.Constant(product.Top()))}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder, certificate := emptyBuilder(t, reg, shape, pathRefinementCapabilities(t))
			_, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), PathRefinements: []PathRefinementTerm{test.build(builder.Arena())}}})
			if err == nil || !strings.Contains(err.Error(), "not an unchanged parameter root") {
				t.Fatalf("Build error = %v, want unchanged-root rejection", err)
			}
		})
	}
}

func TestPreservedParamRootRefinementRejectsMutationAndEscape(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	makeBuilder := func(t *testing.T) (*Builder, SemanticCertificate, PathRefinementTerm) {
		builder, certificate := emptyBuilder(t, reg, shape, pathRefinementCapabilities(t))
		root := Root{Kind: RootParam, Index: 0}
		return builder, certificate, PathRefinementTerm{Path: builder.Arena().Path(root), Value: builder.Arena().Root(root)}
	}
	t.Run("invalidation-effect", func(t *testing.T) {
		builder, certificate, refinement := makeBuilder(t)
		effect, err := builder.EffectArena().InvalidatePath(InvalidatePathConfig{
			Target: PathEffectTarget(refinement.Path), Scope: InvalidationScopeDescendants,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Effects: []EffectTerm{effect}, PathRefinements: []PathRefinementTerm{refinement}}})
		if err == nil || !strings.Contains(err.Error(), "non-interference") {
			t.Fatalf("Build error = %v, want mutation rejection", err)
		}
	})
	t.Run("escape-fact", func(t *testing.T) {
		builder, certificate, refinement := makeBuilder(t)
		_, err := builder.Build(certificate, []Row{{
			Guard: builder.Arena().True(), PathRefinements: []PathRefinementTerm{refinement},
			Output: summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{EscapeEvents: []callboundary.EscapeEventFact{{
				Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventStore,
			}}}},
		}})
		if err == nil || !strings.Contains(err.Error(), "non-interference") {
			t.Fatalf("Build error = %v, want escape rejection", err)
		}
	})
	t.Run("outer-alias-family", func(t *testing.T) {
		caps := pathRefinementCapabilities(t)
		for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
			if err := caps.SetSummary("ReturnParamPathAliases", lane, CapabilitySupported); err != nil {
				t.Fatal(err)
			}
		}
		builder, certificate := emptyBuilder(t, reg, shape, caps)
		root := Root{Kind: RootParam, Index: 0}
		refinement := PathRefinementTerm{Path: builder.Arena().Path(root), Value: builder.Arena().Root(root)}
		placeholder, ok := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(0))
		if !ok {
			t.Fatal("placeholder address")
		}
		_, err := builder.Build(certificate, []Row{{
			Guard: builder.Arena().True(), PathRefinements: []PathRefinementTerm{refinement},
			Output: summary.Summary{ReturnParamPathAliases: []summary.ReturnParamPathAlias{{ReturnIndex: 0, Source: placeholder}}},
		}})
		if err == nil || !strings.Contains(err.Error(), "non-interference") {
			t.Fatalf("Build error = %v, want alias-family rejection", err)
		}
	})
}
