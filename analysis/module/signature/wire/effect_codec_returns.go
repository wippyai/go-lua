package wire

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/type/projection"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// The return-transform vocabulary is the returns package's, and its members are
// answered by that package's own classification. What this package owns is the
// boundary spelling of those members: the kind string written for each variant
// and which effectReturnWire fields that kind carries. That spelling lives in
// the one table below, so the write side and the read side consult a single
// statement instead of two hand-kept switches.

// returnWirePayload names the effectReturnWire fields a kind carries. It is the
// field applicability rule of the wire struct, stated once beside the
// vocabulary: encode writes exactly the named fields and decode reads exactly
// them, so neither side can quietly carry a field the other ignores.
type returnWirePayload uint8

const (
	returnPayloadSource returnWirePayload = iota
	returnPayloadCallbackParam
	returnPayloadSourceProjection
	returnPayloadSourceProjectionCondition
)

// returnTermFields is a transform flattened to the fields the wire carries. It
// is the one shape both directions move through: the write side fills it from
// the vocabulary's own accessors, the read side fills it from the wire.
type returnTermFields struct {
	source        effect.ParamRef
	callbackParam effect.ParamRef
	steps         []projection.Step
	when          typ.Type
	then          typ.Type
}

// returnWireVariant is one vocabulary member's boundary spelling: the kind
// written for it, the wire fields that kind carries, the reading of a transform
// into those fields, and the transform rebuilt from them.
type returnWireVariant struct {
	kind    string
	payload returnWirePayload
	read    func(returns.ReturnType) (returnTermFields, bool)
	build   func(returnTermFields) returns.ReturnType
}

// returnWireVariants is the boundary vocabulary, one row per return-transform
// variant, indexed by the variant's own ordinal.
var returnWireVariants = [returns.ReturnTypeKindCount + 1]returnWireVariant{
	returns.ReturnTypeElementOf: {
		kind:    "returns.elementOf",
		payload: returnPayloadSource,
		read: func(r returns.ReturnType) (returnTermFields, bool) {
			t, ok := returns.AsElementOf(r)
			return returnTermFields{source: t.Source}, ok
		},
		build: func(f returnTermFields) returns.ReturnType { return returns.ElementOf{Source: f.source} },
	},
	returns.ReturnTypeOptionalElementOf: {
		kind:    "returns.optionalElementOf",
		payload: returnPayloadSource,
		read: func(r returns.ReturnType) (returnTermFields, bool) {
			t, ok := returns.AsOptionalElementOf(r)
			return returnTermFields{source: t.Source}, ok
		},
		build: func(f returnTermFields) returns.ReturnType {
			return returns.OptionalElementOf{Source: f.source}
		},
	},
	returns.ReturnTypeCallbackReturn: {
		kind:    "returns.callbackReturn",
		payload: returnPayloadCallbackParam,
		read: func(r returns.ReturnType) (returnTermFields, bool) {
			t, ok := returns.AsCallbackReturn(r)
			return returnTermFields{callbackParam: t.CallbackParam}, ok
		},
		build: func(f returnTermFields) returns.ReturnType {
			return returns.CallbackReturn{CallbackParam: f.callbackParam}
		},
	},
	returns.ReturnTypeArrayOfCallbackReturn: {
		kind:    "returns.arrayOfCallbackReturn",
		payload: returnPayloadCallbackParam,
		read: func(r returns.ReturnType) (returnTermFields, bool) {
			t, ok := returns.AsArrayOfCallbackReturn(r)
			return returnTermFields{callbackParam: t.CallbackParam}, ok
		},
		build: func(f returnTermFields) returns.ReturnType {
			return returns.ArrayOfCallbackReturn{CallbackParam: f.callbackParam}
		},
	},
	returns.ReturnTypeSameAs: {
		kind:    "returns.sameAs",
		payload: returnPayloadSource,
		read: func(r returns.ReturnType) (returnTermFields, bool) {
			t, ok := returns.AsSameAs(r)
			return returnTermFields{source: t.Source}, ok
		},
		build: func(f returnTermFields) returns.ReturnType { return returns.SameAs{Source: f.source} },
	},
	returns.ReturnTypeTypeProjection: {
		kind:    "returns.typeProjection",
		payload: returnPayloadSourceProjection,
		read: func(r returns.ReturnType) (returnTermFields, bool) {
			t, ok := returns.AsTypeProjection(r)
			return returnTermFields{source: t.Source, steps: t.Projection.Steps}, ok
		},
		build: func(f returnTermFields) returns.ReturnType {
			return returns.TypeProjection{Source: f.source, Projection: projection.Projection{Steps: f.steps}}
		},
	},
	returns.ReturnTypeConditionalType: {
		kind:    "returns.conditionalType",
		payload: returnPayloadSourceProjectionCondition,
		read: func(r returns.ReturnType) (returnTermFields, bool) {
			t, ok := returns.AsConditionalType(r)
			return returnTermFields{
				source: t.Source,
				steps:  t.Projection.Steps,
				when:   t.When,
				then:   t.Then,
			}, ok
		},
		build: func(f returnTermFields) returns.ReturnType {
			return returns.ConditionalType{
				Source:     f.source,
				Projection: projection.Projection{Steps: f.steps},
				When:       f.when,
				Then:       f.then,
			}
		},
	},
}

// returnWireVariantsByKind is the read side's index into the same rows, so a
// kind the vocabulary does not spell is unknown to the boundary by construction.
var returnWireVariantsByKind = func() map[string]returns.ReturnTypeKind {
	byKind := make(map[string]returns.ReturnTypeKind, returns.ReturnTypeKindCount)
	for _, kind := range returns.ReturnTypeKinds() {
		row := returnWireVariants[kind]
		if row.kind == "" {
			continue
		}
		byKind[row.kind] = kind
	}
	return byKind
}()

func encodeEffectReturn(ret returns.ReturnType) (*effectReturnWire, error) {
	if ret == nil {
		return nil, nil
	}
	// A transform handed to the boundary as a typed nil pointer carries a kind
	// but no payload to write, so it is rejected here rather than read for
	// fields it does not have.
	if returns.IsNilReturnType(ret) {
		return nil, fmt.Errorf("signature/wire: nil return effect transform %T", ret)
	}
	kind := returns.KindOfReturnType(ret)
	if !kind.Valid() {
		return nil, fmt.Errorf("signature/wire: unsupported return effect transform %T", ret)
	}
	row := returnWireVariants[kind]
	if row.kind == "" {
		return nil, fmt.Errorf("signature/wire: unsupported return effect transform %T", ret)
	}
	fields, ok := row.read(ret)
	if !ok {
		return nil, fmt.Errorf("signature/wire: unsupported return effect transform %T", ret)
	}
	wire := &effectReturnWire{Kind: row.kind}
	switch row.payload {
	case returnPayloadSource:
		wire.Source = encodeParamRef(fields.source)
	case returnPayloadCallbackParam:
		wire.CallbackParam = encodeParamRef(fields.callbackParam)
	case returnPayloadSourceProjection:
		steps, err := encodeProjectionSteps(fields.steps)
		if err != nil {
			return nil, err
		}
		wire.Source = encodeParamRef(fields.source)
		wire.Projection = steps
	case returnPayloadSourceProjectionCondition:
		steps, err := encodeProjectionSteps(fields.steps)
		if err != nil {
			return nil, err
		}
		when, err := EncodeType(fields.when)
		if err != nil {
			return nil, err
		}
		then, err := EncodeType(fields.then)
		if err != nil {
			return nil, err
		}
		wire.Source = encodeParamRef(fields.source)
		wire.Projection = steps
		wire.When = when
		wire.Then = then
	default:
		return nil, fmt.Errorf("signature/wire: return effect transform kind %q carries no stated wire payload", row.kind)
	}
	return wire, nil
}

func decodeEffectReturn(w *effectReturnWire) (returns.ReturnType, error) {
	if w == nil {
		return nil, nil
	}
	kind, known := returnWireVariantsByKind[w.Kind]
	if !known {
		return nil, fmt.Errorf("signature/wire: unknown return effect transform kind %q", w.Kind)
	}
	row := returnWireVariants[kind]
	var fields returnTermFields
	switch row.payload {
	case returnPayloadSource:
		source, err := decodeRequiredParamRef(w.Source, row.kind+" source missing param ref")
		if err != nil {
			return nil, err
		}
		fields.source = source
	case returnPayloadCallbackParam:
		callback, err := decodeRequiredParamRef(w.CallbackParam, row.kind+" callback param missing param ref")
		if err != nil {
			return nil, err
		}
		fields.callbackParam = callback
	case returnPayloadSourceProjection:
		steps, err := decodeProjectionSteps(w.Projection)
		if err != nil {
			return nil, err
		}
		source, err := decodeRequiredParamRef(w.Source, row.kind+" source missing param ref")
		if err != nil {
			return nil, err
		}
		fields.steps, fields.source = steps, source
	case returnPayloadSourceProjectionCondition:
		steps, err := decodeProjectionSteps(w.Projection)
		if err != nil {
			return nil, err
		}
		source, err := decodeRequiredParamRef(w.Source, row.kind+" source missing param ref")
		if err != nil {
			return nil, err
		}
		when, err := DecodeType(w.When)
		if err != nil {
			return nil, fmt.Errorf("%s when: %w", row.kind, err)
		}
		then, err := DecodeType(w.Then)
		if err != nil {
			return nil, fmt.Errorf("%s then: %w", row.kind, err)
		}
		fields.steps, fields.source, fields.when, fields.then = steps, source, when, then
	default:
		return nil, fmt.Errorf("signature/wire: return effect transform kind %q carries no stated wire payload", row.kind)
	}
	return row.build(fields), nil
}
