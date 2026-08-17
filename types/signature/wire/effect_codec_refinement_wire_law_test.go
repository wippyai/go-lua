package wire

import (
	"encoding/json"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
)

// The refinement and type-transform codecs write a format other builds already
// read, so the bytes they produce are a commitment, not an implementation
// detail. These laws hold both codecs to that commitment from both ends: the
// exact bytes each representative value serializes to, and the value that comes
// back when those bytes are read again.
//
// Coverage is derived from the domain catalogs, so a member added to either
// package without a corpus value is a verdict here rather than an untested
// spelling.

func transformParam(index int) effect.ParamRef { return effect.ParamRef{Index: index} }

// transformWireCase is one representative transform and the exact bytes the
// codec writes for it.
type transformWireCase struct {
	name      string
	kind      mutation.TransformKind
	transform mutation.TypeTransform
	pointer   mutation.TypeTransform
	wire      string
}

func transformWireCorpus() []transformWireCase {
	return []transformWireCase{
		{
			name:      "unchanged",
			kind:      mutation.TransformUnchanged,
			transform: mutation.Unchanged{},
			pointer:   &mutation.Unchanged{},
			wire:      `{"kind":"mutation.unchanged"}`,
		},
		{
			name:      "elementUnion",
			kind:      mutation.TransformElementUnion,
			transform: mutation.ElementUnion{Source: transformParam(0)},
			pointer:   &mutation.ElementUnion{Source: transformParam(0)},
			wire:      `{"kind":"mutation.elementUnion","source":{"index":0}}`,
		},
		{
			name:      "containerElementUnion",
			kind:      mutation.TransformContainerElementUnion,
			transform: mutation.ContainerElementUnion{Container: transformParam(1), Value: transformParam(2)},
			pointer:   &mutation.ContainerElementUnion{Container: transformParam(1), Value: transformParam(2)},
			wire:      `{"kind":"mutation.containerElementUnion","container":{"index":1},"value":{"index":2}}`,
		},
		{
			name:      "toArray",
			kind:      mutation.TransformToArray,
			transform: mutation.ToArray{Element: transformParam(3)},
			pointer:   &mutation.ToArray{Element: transformParam(3)},
			wire:      `{"kind":"mutation.toArray","element":{"index":3}}`,
		},
	}
}

// TestEffectTransformWireBytesAreStable states the written commitment: each
// transform serializes to exactly the recorded bytes, kind spelling, field
// placement and field omission included. The pointer spelling of a transform is
// the same transform, so it writes the same bytes.
func TestEffectTransformWireBytesAreStable(t *testing.T) {
	for _, tc := range transformWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			for label, transform := range map[string]mutation.TypeTransform{"value": tc.transform, "pointer": tc.pointer} {
				wire, err := encodeEffectTransform(transform)
				if err != nil {
					t.Fatalf("encodeEffectTransform(%s): %v", label, err)
				}
				if wire == nil {
					t.Fatalf("encodeEffectTransform(%s) wrote nothing", label)
				}
				data, err := json.Marshal(wire)
				if err != nil {
					t.Fatalf("marshal(%s): %v", label, err)
				}
				if string(data) != tc.wire {
					t.Fatalf("%s wire bytes = %s, want %s", label, data, tc.wire)
				}
			}
		})
	}
}

// TestEffectTransformRoundTripsThroughItsOwnBytes states the read commitment:
// the recorded bytes parse back into the transform they were written from,
// compared by the vocabulary's own structural equality rather than by a
// spelling of it here.
func TestEffectTransformRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, tc := range transformWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			var read effectTransformWire
			if err := json.Unmarshal([]byte(tc.wire), &read); err != nil {
				t.Fatalf("unmarshal recorded bytes: %v", err)
			}
			decoded, err := decodeEffectTransform(&read)
			if err != nil {
				t.Fatalf("decodeEffectTransform: %v", err)
			}
			if got := mutation.KindOfTransform(decoded); got != tc.kind {
				t.Fatalf("decoded transform is kind %d, want %d", got, tc.kind)
			}
			want := mutation.Mutate{Target: transformParam(0), Transform: tc.transform}
			if !(mutation.Mutate{Target: transformParam(0), Transform: decoded}).Equals(want) {
				t.Fatalf("decoded transform = %s, want %s", decoded, tc.transform)
			}
			rewritten, err := encodeEffectTransform(decoded)
			if err != nil {
				t.Fatalf("encodeEffectTransform(decoded): %v", err)
			}
			data, err := json.Marshal(rewritten)
			if err != nil {
				t.Fatalf("marshal rewritten: %v", err)
			}
			if string(data) != tc.wire {
				t.Fatalf("rewritten bytes = %s, want %s", data, tc.wire)
			}
		})
	}
}

// TestEffectTransformCorpusReachesEveryDeclaredKind derives coverage from the
// vocabulary catalog, so a variant the codec serializes without a corpus
// transform is unproven and says so here.
func TestEffectTransformCorpusReachesEveryDeclaredKind(t *testing.T) {
	reached := make(map[mutation.TransformKind]bool, mutation.TransformKindCount)
	for _, tc := range transformWireCorpus() {
		if got := mutation.KindOfTransform(tc.transform); got != tc.kind {
			t.Fatalf("corpus case %q stands for kind %d but its transform is kind %d", tc.name, tc.kind, got)
		}
		reached[tc.kind] = true
	}
	for _, kind := range mutation.TransformKinds() {
		if !reached[kind] {
			t.Fatalf("vocabulary kind %d is serialized by the codec but no corpus transform exercises it", kind)
		}
	}
}

// TestEffectTransformCodecSpellsEveryDeclaredKindOnce states that the boundary
// vocabulary is total over the domain catalog and injective into wire kinds: no
// declared variant is unspelled, and no two share a spelling.
func TestEffectTransformCodecSpellsEveryDeclaredKindOnce(t *testing.T) {
	spelled := make(map[string]mutation.TransformKind, mutation.TransformKindCount)
	for _, kind := range mutation.TransformKinds() {
		row := transformWireVariants[kind]
		if row.kind == "" {
			t.Fatalf("declared transform kind %d has no wire spelling", kind)
		}
		if prior, duplicate := spelled[row.kind]; duplicate {
			t.Fatalf("declared kinds %d and %d are both spelled %q", prior, kind, row.kind)
		}
		spelled[row.kind] = kind
		if row.read == nil || row.build == nil {
			t.Fatalf("wire kind %q states no reading or no rebuilding", row.kind)
		}
		read, known := transformWireVariantsByKind[row.kind]
		if !known || read != kind {
			t.Fatalf("wire kind %q is written for kind %d but read back as kind %d", row.kind, kind, read)
		}
	}
}

// TestEffectTransformCodecRejectsAbsentAndUnknown states the boundary's closing
// half: an absent transform writes nothing, a transform spelled as a typed nil
// pointer is refused rather than dereferenced, and a kind the vocabulary does
// not declare is refused on read.
func TestEffectTransformCodecRejectsAbsentAndUnknown(t *testing.T) {
	wire, err := encodeEffectTransform(nil)
	if err != nil || wire != nil {
		t.Fatalf("encodeEffectTransform(nil) = %v/%v, want nothing written", wire, err)
	}
	for _, absent := range []mutation.TypeTransform{
		(*mutation.Unchanged)(nil),
		(*mutation.ElementUnion)(nil),
		(*mutation.ContainerElementUnion)(nil),
		(*mutation.ToArray)(nil),
	} {
		if _, err := encodeEffectTransform(absent); err == nil {
			t.Fatalf("codec wrote the absent %T transform", absent)
		}
	}
	if _, err := decodeEffectTransform(&effectTransformWire{Kind: "mutation.unsealed"}); err == nil {
		t.Fatal("codec admitted a wire kind the vocabulary does not declare")
	}
	decoded, err := decodeEffectTransform(nil)
	if err != nil || decoded != nil {
		t.Fatalf("decodeEffectTransform(nil) = %v/%v, want no transform", decoded, err)
	}
}

// refinementWireCase is one representative refinement and the exact bytes the
// codec writes for it.
type refinementWireCase struct {
	name       string
	refinement postcondition.Refinement
	pointer    postcondition.Refinement
	wire       string
}

func refinementWireCorpus() []refinementWireCase {
	return []refinementWireCase{
		{
			name:       "present",
			refinement: postcondition.Present{},
			pointer:    &postcondition.Present{},
			wire:       `{"kind":"present"}`,
		},
		{
			name:       "absent",
			refinement: postcondition.Absent{},
			pointer:    &postcondition.Absent{},
			wire:       `{"kind":"absent"}`,
		},
	}
}

// TestEffectRefinementWireBytesAreStable states the written commitment for the
// refinement codec, pointer spellings included.
func TestEffectRefinementWireBytesAreStable(t *testing.T) {
	for _, tc := range refinementWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			for label, refinement := range map[string]postcondition.Refinement{"value": tc.refinement, "pointer": tc.pointer} {
				wire, err := encodeEffectRefinement(refinement)
				if err != nil {
					t.Fatalf("encodeEffectRefinement(%s): %v", label, err)
				}
				data, err := json.Marshal(wire)
				if err != nil {
					t.Fatalf("marshal(%s): %v", label, err)
				}
				if string(data) != tc.wire {
					t.Fatalf("%s wire bytes = %s, want %s", label, data, tc.wire)
				}
			}
		})
	}
}

// TestEffectRefinementRoundTripsThroughItsOwnBytes states the read commitment
// for the refinement codec.
func TestEffectRefinementRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, tc := range refinementWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			var read effectRefinementWire
			if err := json.Unmarshal([]byte(tc.wire), &read); err != nil {
				t.Fatalf("unmarshal recorded bytes: %v", err)
			}
			decoded, err := decodeEffectRefinement(&read)
			if err != nil {
				t.Fatalf("decodeEffectRefinement: %v", err)
			}
			if !decoded.Equals(tc.refinement) {
				t.Fatalf("decoded refinement = %s, want %s", decoded, tc.refinement)
			}
			rewritten, err := encodeEffectRefinement(decoded)
			if err != nil {
				t.Fatalf("encodeEffectRefinement(decoded): %v", err)
			}
			data, err := json.Marshal(rewritten)
			if err != nil {
				t.Fatalf("marshal rewritten: %v", err)
			}
			if string(data) != tc.wire {
				t.Fatalf("rewritten bytes = %s, want %s", data, tc.wire)
			}
		})
	}
}

// TestEveryDeclaredRefinementIsReadableAtTheBoundary is the law the decode side
// now rests on: every member of the domain catalog is written by the codec and
// read back by it. A refinement added to the domain is readable by
// construction, so the corpus above cannot be the only thing standing between a
// new member and a manifest that will not parse.
func TestEveryDeclaredRefinementIsReadableAtTheBoundary(t *testing.T) {
	corpus := make(map[string]bool, postcondition.RefinementCount)
	for _, tc := range refinementWireCorpus() {
		corpus[tc.refinement.Kind()] = true
	}
	for _, refinement := range postcondition.Refinements() {
		wire, err := encodeEffectRefinement(refinement)
		if err != nil {
			t.Fatalf("codec does not write declared refinement %T: %v", refinement, err)
		}
		decoded, err := decodeEffectRefinement(wire)
		if err != nil {
			t.Fatalf("codec writes declared refinement %T as kind %q but does not read it: %v", refinement, wire.Kind, err)
		}
		if !decoded.Equals(refinement) {
			t.Fatalf("declared refinement %T reads back as %T", refinement, decoded)
		}
		if !corpus[refinement.Kind()] {
			t.Fatalf("declared refinement %T is serialized by the codec but no corpus case pins its bytes", refinement)
		}
	}
}

// TestEffectRefinementCodecRejectsAbsentAndUnknown states the boundary's
// closing half for the refinement codec.
func TestEffectRefinementCodecRejectsAbsentAndUnknown(t *testing.T) {
	if _, err := encodeEffectRefinement(nil); err == nil {
		t.Fatal("codec wrote an absent refinement")
	}
	if _, err := encodeEffectRefinement((*postcondition.Present)(nil)); err == nil {
		t.Fatal("codec wrote a refinement spelled as a typed nil pointer")
	}
	if _, err := decodeEffectRefinement(nil); err == nil {
		t.Fatal("codec read a refinement from nothing")
	}
	if _, err := decodeEffectRefinement(&effectRefinementWire{Kind: "unsealed"}); err == nil {
		t.Fatal("codec admitted a wire kind the vocabulary does not declare")
	}
	if _, err := decodeEffectRefinement(&effectRefinementWire{}); err == nil {
		t.Fatal("codec admitted an unspelled wire kind")
	}
}
