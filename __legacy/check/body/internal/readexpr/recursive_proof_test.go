package readexpr

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/type/indexproof"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestSequenceAndIndexProofsAreDepthIndependent(t *testing.T) {
	leaf := typ.NewTuple(typ.String)
	var deep typ.Type = leaf
	for range 256 {
		deep = &typ.Optional{Inner: deep}
	}
	if !indexproof.SequenceLengthKnownAtLeast(deep, 1) {
		t.Fatal("deep sequence wrapper changed the shallow length proof")
	}
	if !typevalue.InRangeIndexExcludesNil(deep) {
		t.Fatal("deep sequence wrapper changed the shallow non-nil proof")
	}
}

func TestRecursiveProofsUseSemanticFixedPoints(t *testing.T) {
	goodSequence := typ.NewRecursivePlaceholder("GoodSequence")
	goodSequence.SetBody(&typ.Union{Members: []typ.Type{goodSequence, typ.NewTuple(typ.String)}})
	if !indexproof.SequenceLengthKnownAtLeast(goodSequence, 1) || !typevalue.InRangeIndexExcludesNil(goodSequence) {
		t.Fatal("productive recursive sequence must prove its leaf invariant")
	}

	badSequence := typ.NewRecursivePlaceholder("BadSequence")
	badSequence.SetBody(&typ.Union{Members: []typ.Type{badSequence, typ.NewTuple(typ.Nil)}})
	if typevalue.InRangeIndexExcludesNil(badSequence) {
		t.Fatal("recursive sequence with a nil leaf must not prove non-nil elements")
	}

	key := typ.NewRecursivePlaceholder("Key")
	key.SetBody(&typ.Union{Members: []typ.Type{key, typ.LiteralString("field")}})
	if !dynamicIndexKeyDefinitelyMatchesSegment(key, segment.Segment{Kind: segment.SegmentField, Name: "field"}) {
		t.Fatal("productive recursive key must prove the exact segment")
	}
	if dynamicIndexKeyDefinitelyMatchesSegment(key, segment.Segment{Kind: segment.SegmentField, Name: "other"}) {
		t.Fatal("recursive key must not prove a different segment")
	}
}
