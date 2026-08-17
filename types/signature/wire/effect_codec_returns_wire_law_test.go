package wire

import (
	"encoding/json"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/projection"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// The return-transform codec writes a format other builds already read, so the
// bytes it produces for a transform are a commitment, not an implementation
// detail. These laws hold the codec to that commitment from both ends: the
// exact bytes each representative transform serializes to, and the transform
// that comes back when those bytes are read again.
//
// Coverage is derived from the returns package's own vocabulary catalog, so a
// variant added there without a corpus transform is a verdict here rather than
// an untested spelling.

func returnParam(index int) effect.ParamRef { return effect.ParamRef{Index: index} }

// returnCorpusProjection reaches every projection step form, so the return
// transforms that carry a projection carry a full one.
func returnCorpusProjection() projection.Projection {
	return projection.Projection{Steps: []projection.Step{
		projection.Field("inner"),
		projection.CallableReturn(),
		projection.GenericArg(2),
		projection.InstantiateGeneric(typ.String),
	}}
}

// returnWireCase is one representative transform and the exact bytes the codec
// writes for it.
type returnWireCase struct {
	name      string
	kind      returns.ReturnTypeKind
	transform returns.ReturnType
	pointer   returns.ReturnType
	wire      string
}

func returnWireCorpus() []returnWireCase {
	proj := returnCorpusProjection()
	projWire := `[{"kind":"field","field":"inner"},{"kind":"callableReturn"},{"kind":"genericArg","index":2},{"kind":"instantiateGeneric","type":{"kind":"string"}}]`
	return []returnWireCase{
		{
			name:      "elementOf",
			kind:      returns.ReturnTypeElementOf,
			transform: returns.ElementOf{Source: returnParam(0)},
			pointer:   &returns.ElementOf{Source: returnParam(0)},
			wire:      `{"kind":"returns.elementOf","source":{"index":0}}`,
		},
		{
			name:      "optionalElementOf",
			kind:      returns.ReturnTypeOptionalElementOf,
			transform: returns.OptionalElementOf{Source: returnParam(1)},
			pointer:   &returns.OptionalElementOf{Source: returnParam(1)},
			wire:      `{"kind":"returns.optionalElementOf","source":{"index":1}}`,
		},
		{
			name:      "callbackReturn",
			kind:      returns.ReturnTypeCallbackReturn,
			transform: returns.CallbackReturn{CallbackParam: returnParam(2)},
			pointer:   &returns.CallbackReturn{CallbackParam: returnParam(2)},
			wire:      `{"kind":"returns.callbackReturn","callbackParam":{"index":2}}`,
		},
		{
			name:      "arrayOfCallbackReturn",
			kind:      returns.ReturnTypeArrayOfCallbackReturn,
			transform: returns.ArrayOfCallbackReturn{CallbackParam: returnParam(3)},
			pointer:   &returns.ArrayOfCallbackReturn{CallbackParam: returnParam(3)},
			wire:      `{"kind":"returns.arrayOfCallbackReturn","callbackParam":{"index":3}}`,
		},
		{
			name:      "sameAs",
			kind:      returns.ReturnTypeSameAs,
			transform: returns.SameAs{Source: returnParam(4)},
			pointer:   &returns.SameAs{Source: returnParam(4)},
			wire:      `{"kind":"returns.sameAs","source":{"index":4}}`,
		},
		{
			name:      "typeProjection",
			kind:      returns.ReturnTypeTypeProjection,
			transform: returns.TypeProjection{Source: returnParam(5), Projection: proj},
			pointer:   &returns.TypeProjection{Source: returnParam(5), Projection: proj},
			wire:      `{"kind":"returns.typeProjection","source":{"index":5},"projection":` + projWire + `}`,
		},
		{
			name:      "typeProjection/empty path",
			kind:      returns.ReturnTypeTypeProjection,
			transform: returns.TypeProjection{Source: returnParam(5)},
			pointer:   &returns.TypeProjection{Source: returnParam(5)},
			wire:      `{"kind":"returns.typeProjection","source":{"index":5}}`,
		},
		{
			name: "conditionalType",
			kind: returns.ReturnTypeConditionalType,
			transform: returns.ConditionalType{
				Source: returnParam(6), Projection: proj, When: typ.String, Then: typ.Number,
			},
			pointer: &returns.ConditionalType{
				Source: returnParam(6), Projection: proj, When: typ.String, Then: typ.Number,
			},
			wire: `{"kind":"returns.conditionalType","source":{"index":6},"projection":` + projWire + `,"when":{"kind":"string"},"then":{"kind":"number"}}`,
		},
	}
}

// TestEffectReturnWireBytesAreStable states the written commitment: each
// transform serializes to exactly the recorded bytes, kind spelling, field
// placement and field omission included. The pointer spelling of a transform is
// the same transform, so it writes the same bytes.
func TestEffectReturnWireBytesAreStable(t *testing.T) {
	for _, tc := range returnWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			for label, transform := range map[string]returns.ReturnType{"value": tc.transform, "pointer": tc.pointer} {
				wire, err := encodeEffectReturn(transform)
				if err != nil {
					t.Fatalf("encodeEffectReturn(%s): %v", label, err)
				}
				if wire == nil {
					t.Fatalf("encodeEffectReturn(%s) wrote nothing", label)
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

// TestEffectReturnRoundTripsThroughItsOwnBytes states the read commitment: the
// recorded bytes parse back into the transform they were written from, compared
// by the vocabulary's own structural equality rather than by a spelling of it
// here.
func TestEffectReturnRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, tc := range returnWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			var read effectReturnWire
			if err := json.Unmarshal([]byte(tc.wire), &read); err != nil {
				t.Fatalf("unmarshal recorded bytes: %v", err)
			}
			decoded, err := decodeEffectReturn(&read)
			if err != nil {
				t.Fatalf("decodeEffectReturn: %v", err)
			}
			if got := returns.KindOfReturnType(decoded); got != tc.kind {
				t.Fatalf("decoded transform is kind %d, want %d", got, tc.kind)
			}
			want := returns.Return{ReturnIndex: 0, Transform: tc.transform}
			if !(returns.Return{ReturnIndex: 0, Transform: decoded}).Equals(want) {
				t.Fatalf("decoded transform = %s, want %s", decoded, tc.transform)
			}
			rewritten, err := encodeEffectReturn(decoded)
			if err != nil {
				t.Fatalf("encodeEffectReturn(decoded): %v", err)
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

// TestEffectReturnCorpusReachesEveryDeclaredKind derives coverage from the
// vocabulary catalog, so a variant the codec serializes without a corpus
// transform is unproven and says so here.
func TestEffectReturnCorpusReachesEveryDeclaredKind(t *testing.T) {
	reached := make(map[returns.ReturnTypeKind]bool, returns.ReturnTypeKindCount)
	for _, tc := range returnWireCorpus() {
		if got := returns.KindOfReturnType(tc.transform); got != tc.kind {
			t.Fatalf("corpus case %q stands for kind %d but its transform is kind %d", tc.name, tc.kind, got)
		}
		reached[tc.kind] = true
	}
	for _, kind := range returns.ReturnTypeKinds() {
		if !reached[kind] {
			t.Fatalf("vocabulary kind %d is serialized by the codec but no corpus transform exercises it", kind)
		}
	}
}

// TestEffectReturnCodecSpellsEveryDeclaredKindOnce states that the boundary
// vocabulary is total over the domain catalog and injective into wire kinds: no
// declared variant is unspelled, and no two share a spelling.
func TestEffectReturnCodecSpellsEveryDeclaredKindOnce(t *testing.T) {
	spelled := make(map[string]returns.ReturnTypeKind, returns.ReturnTypeKindCount)
	for _, kind := range returns.ReturnTypeKinds() {
		row := returnWireVariants[kind]
		if row.kind == "" {
			t.Fatalf("declared return transform kind %d has no wire spelling", kind)
		}
		if prior, duplicate := spelled[row.kind]; duplicate {
			t.Fatalf("declared kinds %d and %d are both spelled %q", prior, kind, row.kind)
		}
		spelled[row.kind] = kind
		if row.read == nil || row.build == nil {
			t.Fatalf("wire kind %q states no reading or no rebuilding", row.kind)
		}
		read, known := returnWireVariantsByKind[row.kind]
		if !known || read != kind {
			t.Fatalf("wire kind %q is written for kind %d but read back as kind %d", row.kind, kind, read)
		}
	}
}

// typedNilReturnTransforms is one typed nil pointer per declared variant, keyed
// by the variant it names. The keys derive the coverage of the refusal law from
// the vocabulary catalog; the terms themselves classify as absent, so they carry
// no kind the law could read back off them.
func typedNilReturnTransforms(t *testing.T) []returns.ReturnType {
	t.Helper()
	byKind := map[returns.ReturnTypeKind]returns.ReturnType{
		returns.ReturnTypeSameAs:                (*returns.SameAs)(nil),
		returns.ReturnTypeElementOf:             (*returns.ElementOf)(nil),
		returns.ReturnTypeOptionalElementOf:     (*returns.OptionalElementOf)(nil),
		returns.ReturnTypeCallbackReturn:        (*returns.CallbackReturn)(nil),
		returns.ReturnTypeArrayOfCallbackReturn: (*returns.ArrayOfCallbackReturn)(nil),
		returns.ReturnTypeTypeProjection:        (*returns.TypeProjection)(nil),
		returns.ReturnTypeConditionalType:       (*returns.ConditionalType)(nil),
	}
	absent := make([]returns.ReturnType, 0, returns.ReturnTypeKindCount)
	for _, kind := range returns.ReturnTypeKinds() {
		term, named := byKind[kind]
		if !named {
			t.Fatalf("declared return transform kind %d has no typed nil term at the boundary", kind)
		}
		absent = append(absent, term)
	}
	return absent
}

// TestEffectReturnCodecRejectsAbsentAndUnknown states the boundary's closing
// half: an absent transform writes nothing, a transform spelled as a typed nil
// pointer is refused rather than dereferenced, and a kind the vocabulary does
// not declare is refused on read. The refusal rests on the vocabulary's own
// classification of a typed nil as absent, so the boundary never reaches a
// payload row for one.
func TestEffectReturnCodecRejectsAbsentAndUnknown(t *testing.T) {
	wire, err := encodeEffectReturn(nil)
	if err != nil || wire != nil {
		t.Fatalf("encodeEffectReturn(nil) = %v/%v, want nothing written", wire, err)
	}
	for _, absent := range typedNilReturnTransforms(t) {
		if got := returns.KindOfReturnType(absent); got.Valid() {
			t.Fatalf("the vocabulary classifies the absent %T as kind %d, so the boundary would route it to a payload", absent, got)
		}
		if _, err := encodeEffectReturn(absent); err == nil {
			t.Fatalf("codec wrote the absent %T transform", absent)
		}
		if _, err := encodeEffectLabel(returns.Return{ReturnIndex: 0, Transform: absent}); err == nil {
			t.Fatalf("codec wrote the return label carrying the absent %T transform", absent)
		}
	}
	if _, err := decodeEffectReturn(&effectReturnWire{Kind: "returns.unsealed"}); err == nil {
		t.Fatal("codec admitted a wire kind the vocabulary does not declare")
	}
	decoded, err := decodeEffectReturn(nil)
	if err != nil || decoded != nil {
		t.Fatalf("decodeEffectReturn(nil) = %v/%v, want no transform", decoded, err)
	}
}
