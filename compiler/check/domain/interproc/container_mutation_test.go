package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeContainerMutationSlices_DedupAndSorted(t *testing.T) {
	existing := []api.ContainerMutation{
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
			ValueType: product.FromType(typ.Number),
		},
	}
	next := []api.ContainerMutation{
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}},
			ValueType: product.FromType(typ.String),
		},
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
			ValueType: product.FromType(typ.Integer),
		},
	}

	got := MergeContainerMutationSlices(existing, next, func(prev *api.ContainerMutation, n api.ContainerMutation) api.ContainerMutation {
		if prev != nil {
			n.ValueType = product.FromType(typ.JoinPreferNonSoft(prev.ValueType.ProjectValue(), n.ValueType.ProjectValue()))
		}
		return n
	})

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if k := api.ContainerMutationKey(got[0]); k != "container:.a" {
		t.Fatalf("first key = %q, want container:.a", k)
	}
	if k := api.ContainerMutationKey(got[1]); k != "container:.b" {
		t.Fatalf("second key = %q, want container:.b", k)
	}
	if !typ.TypeEquals(got[1].ValueType.ProjectValue(), typ.Number) {
		t.Fatalf(".b merged type = %v, want number", got[1].ValueType)
	}
}

func TestMergeCapturedContainerMutationMaps_MergeBySymbol(t *testing.T) {
	existing := map[cfg.SymbolID][]api.ContainerMutation{
		1: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}},
				ValueType: product.FromType(typ.String),
			},
		},
	}
	next := map[cfg.SymbolID][]api.ContainerMutation{
		1: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}},
				ValueType: product.FromType(typ.String),
			},
		},
		2: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "y"}},
				ValueType: product.FromType(typ.Boolean),
			},
		},
	}

	got := MergeCapturedContainerMutationMaps(existing, next, nil)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 symbols", len(got))
	}
	if len(got[1]) != 1 || len(got[2]) != 1 {
		t.Fatalf("unexpected per-symbol merge sizes: sym1=%d sym2=%d", len(got[1]), len(got[2]))
	}
	if key := api.ContainerMutationKey(got[2][0]); key != "container:.y" {
		t.Fatalf("sym2 key = %q, want container:.y", key)
	}
}

func TestMergeContainerMutationSlices_KeepsOperatorKindsDistinct(t *testing.T) {
	existing := []api.ContainerMutation{
		{
			Kind:      api.ContainerMutationContainerElement,
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}},
			ValueType: product.FromType(typ.Number),
		},
	}
	next := []api.ContainerMutation{
		{
			Kind:      api.ContainerMutationTableElement,
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}},
			ValueType: product.FromType(typ.String),
		},
	}

	got := MergeContainerMutationSlices(existing, next, nil)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 distinct operator facts", len(got))
	}
	if got[0].Kind == got[1].Kind {
		t.Fatalf("expected separate facts for same path with different operators, got %#v", got)
	}
}

func TestWidenCapturedContainerMutations_JoinsSameContainerElement(t *testing.T) {
	prevRecord := typ.NewRecord().Field("name", typ.Any).Build()
	nextRecord := typ.NewRecord().Field("error", typ.String).Build()

	prev := api.CapturedContainerMutations{
		10: {
			20: {
				{Kind: api.ContainerMutationContainerElement, ValueType: product.FromType(prevRecord)},
			},
		},
	}
	next := api.CapturedContainerMutations{
		10: {
			20: {
				{Kind: api.ContainerMutationContainerElement, ValueType: product.FromType(nextRecord)},
			},
		},
	}

	got := WidenCapturedContainerMutations(prev, next)
	muts := got[10][20]
	if len(muts) != 1 {
		t.Fatalf("len(muts) = %d, want 1", len(muts))
	}
	if typ.TypeEquals(muts[0].ValueType.ProjectValue(), prevRecord) || typ.TypeEquals(muts[0].ValueType.ProjectValue(), nextRecord) {
		t.Fatalf("expected joined container element type, got %v", muts[0].ValueType.ProjectValue())
	}
	if !typ.TypeEquals(got[10][20][0].ValueType.ProjectValue(), WidenCapturedContainerMutations(got, next)[10][20][0].ValueType.ProjectValue()) {
		t.Fatalf("widened captured container mutation must be idempotent, got %v then %v", got[10][20][0].ValueType.ProjectValue(), WidenCapturedContainerMutations(got, next)[10][20][0].ValueType.ProjectValue())
	}
}

func TestWidenCapturedContainerMutations_DedupesSameIterationMutations(t *testing.T) {
	firstRecord := typ.NewRecord().Field("name", typ.Any).Build()
	secondRecord := typ.NewRecord().Field("error", typ.String).Build()

	next := api.CapturedContainerMutations{
		10: {
			20: {
				{Kind: api.ContainerMutationContainerElement, ValueType: product.FromType(firstRecord)},
				{Kind: api.ContainerMutationContainerElement, ValueType: product.FromType(secondRecord)},
			},
		},
	}

	got := WidenCapturedContainerMutations(nil, next)
	muts := got[10][20]
	if len(muts) != 1 {
		t.Fatalf("len(muts) = %d, want 1 canonical mutation per path", len(muts))
	}
	if typ.TypeEquals(muts[0].ValueType.ProjectValue(), firstRecord) || typ.TypeEquals(muts[0].ValueType.ProjectValue(), secondRecord) {
		t.Fatalf("expected same-iteration container writes to join, got %v", muts[0].ValueType.ProjectValue())
	}
	if !CapturedContainerMutationsEqual(got, WidenCapturedContainerMutations(got, next)) {
		t.Fatalf("widened captured container mutations must be idempotent")
	}
}

func TestWidenCapturedContainerMutations_ConcreteValueRefinesUnknownEvidence(t *testing.T) {
	const (
		fnSym     cfg.SymbolID = 10
		targetSym cfg.SymbolID = 20
	)
	suite := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(typ.Unknown)).
		Build()
	prev := api.CapturedContainerMutations{
		fnSym: {
			targetSym: {
				{Kind: api.ContainerMutationTableElement, ValueType: product.FromType(typ.Unknown)},
			},
		},
	}
	next := api.CapturedContainerMutations{
		fnSym: {
			targetSym: {
				{Kind: api.ContainerMutationTableElement, ValueType: product.FromType(suite)},
			},
		},
	}

	got := WidenCapturedContainerMutations(prev, next)
	muts := got[fnSym][targetSym]
	if len(muts) != 1 {
		t.Fatalf("len(muts) = %d, want 1", len(muts))
	}
	if !typ.TypeEquals(muts[0].ValueType.ProjectValue(), suite) {
		t.Fatalf("captured mutation value = %v, want concrete suite evidence", muts[0].ValueType.ProjectValue())
	}
	if !CapturedContainerMutationsEqual(got, WidenCapturedContainerMutations(got, next)) {
		t.Fatalf("widened captured mutation refinement must be idempotent")
	}
}

func TestJoinCapturedContainerMutations_ConcreteValueRefinesUnknownEvidence(t *testing.T) {
	const (
		fnSym     cfg.SymbolID = 10
		targetSym cfg.SymbolID = 20
	)
	suite := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(typ.Unknown)).
		Build()
	prev := api.CapturedContainerMutations{
		fnSym: {
			targetSym: {
				{Kind: api.ContainerMutationTableElement, ValueType: product.FromType(typ.Unknown)},
			},
		},
	}
	next := api.CapturedContainerMutations{
		fnSym: {
			targetSym: {
				{Kind: api.ContainerMutationTableElement, ValueType: product.FromType(suite)},
			},
		},
	}

	got := JoinCapturedContainerMutations(prev, next)
	muts := got[fnSym][targetSym]
	if len(muts) != 1 {
		t.Fatalf("len(muts) = %d, want 1", len(muts))
	}
	if !typ.TypeEquals(muts[0].ValueType.ProjectValue(), suite) {
		t.Fatalf("captured mutation value = %v, want concrete suite evidence", muts[0].ValueType.ProjectValue())
	}
}
