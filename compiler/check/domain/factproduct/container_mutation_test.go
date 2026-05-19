package factproduct

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeContainerMutationSlices_DedupAndSorted(t *testing.T) {
	existing := []api.ContainerMutation{
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
			ValueType: typ.Number,
		},
	}
	next := []api.ContainerMutation{
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}},
			ValueType: typ.String,
		},
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
			ValueType: typ.Integer,
		},
	}

	got := MergeContainerMutationSlices(existing, next, func(prev *api.ContainerMutation, n api.ContainerMutation) api.ContainerMutation {
		if prev != nil {
			n.ValueType = typ.JoinPreferNonSoft(prev.ValueType, n.ValueType)
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
	if !typ.TypeEquals(got[1].ValueType, typ.Number) {
		t.Fatalf(".b merged type = %v, want number", got[1].ValueType)
	}
}

func TestMergeCapturedContainerMutationMaps_MergeBySymbol(t *testing.T) {
	existing := map[cfg.SymbolID][]api.ContainerMutation{
		1: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}},
				ValueType: typ.String,
			},
		},
	}
	next := map[cfg.SymbolID][]api.ContainerMutation{
		1: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}},
				ValueType: typ.String,
			},
		},
		2: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "y"}},
				ValueType: typ.Boolean,
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
			ValueType: typ.Number,
		},
	}
	next := []api.ContainerMutation{
		{
			Kind:      api.ContainerMutationTableElement,
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}},
			ValueType: typ.String,
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
