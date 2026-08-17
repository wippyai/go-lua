package wire

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
)

// A refinement's wire kind is the spelling the refinement declares for itself,
// so both directions of this codec go through the postcondition package's own
// vocabulary: the write side asks a refinement for its kind and the read side
// asks the catalog which refinement declares that kind. A refinement added to
// the domain is therefore readable the moment it is writable.

func encodeEffectRefinement(refinement postcondition.Refinement) (*effectRefinementWire, error) {
	normalized, ok := postcondition.NormalizeRefinement(refinement)
	if !ok {
		if postcondition.RefinementIsNil(refinement) {
			return nil, fmt.Errorf("signature/wire: missing effect refinement")
		}
		return nil, fmt.Errorf("signature/wire: unsupported effect refinement %T", refinement)
	}
	return &effectRefinementWire{Kind: normalized.Kind()}, nil
}

func decodeEffectRefinement(w *effectRefinementWire) (postcondition.Refinement, error) {
	if w == nil {
		return nil, fmt.Errorf("signature/wire: missing effect refinement")
	}
	refinement, known := postcondition.RefinementForKind(w.Kind)
	if !known {
		return nil, fmt.Errorf("signature/wire: unknown effect refinement kind %q", w.Kind)
	}
	return refinement, nil
}

// The type-transform vocabulary is the mutation package's, and its members are
// answered by that package's own classification. What this package owns is the
// boundary spelling of those members: the kind string written for each variant
// and which effectTransformWire fields that kind carries. That spelling lives
// in the one table below, so the write side and the read side consult a single
// statement instead of two hand-kept switches.

// transformWirePayload names the effectTransformWire fields a kind carries. It
// is the field applicability rule of the wire struct, stated once beside the
// vocabulary: encode writes exactly the named fields and decode reads exactly
// them, so neither side can quietly carry a field the other ignores.
type transformWirePayload uint8

const (
	transformPayloadNone transformWirePayload = iota
	transformPayloadSource
	transformPayloadContainerValue
	transformPayloadElement
)

// transformTermFields is a transform flattened to the fields the wire carries.
// It is the one shape both directions move through: the write side fills it
// from the vocabulary's own accessors, the read side fills it from the wire.
type transformTermFields struct {
	source    effect.ParamRef
	container effect.ParamRef
	value     effect.ParamRef
	element   effect.ParamRef
}

// transformWireVariant is one vocabulary member's boundary spelling: the kind
// written for it, the wire fields that kind carries, the reading of a transform
// into those fields, and the transform rebuilt from them.
type transformWireVariant struct {
	kind    string
	payload transformWirePayload
	read    func(mutation.TypeTransform) (transformTermFields, bool)
	build   func(transformTermFields) mutation.TypeTransform
}

// transformWireVariants is the boundary vocabulary, one row per type-transform
// variant, indexed by the variant's own ordinal.
var transformWireVariants = [mutation.TransformKindCount + 1]transformWireVariant{
	mutation.TransformUnchanged: {
		kind:    "mutation.unchanged",
		payload: transformPayloadNone,
		read: func(t mutation.TypeTransform) (transformTermFields, bool) {
			_, ok := mutation.AsUnchanged(t)
			return transformTermFields{}, ok
		},
		build: func(transformTermFields) mutation.TypeTransform { return mutation.Unchanged{} },
	},
	mutation.TransformElementUnion: {
		kind:    "mutation.elementUnion",
		payload: transformPayloadSource,
		read: func(t mutation.TypeTransform) (transformTermFields, bool) {
			v, ok := mutation.AsElementUnion(t)
			return transformTermFields{source: v.Source}, ok
		},
		build: func(f transformTermFields) mutation.TypeTransform {
			return mutation.ElementUnion{Source: f.source}
		},
	},
	mutation.TransformContainerElementUnion: {
		kind:    "mutation.containerElementUnion",
		payload: transformPayloadContainerValue,
		read: func(t mutation.TypeTransform) (transformTermFields, bool) {
			v, ok := mutation.AsContainerElementUnion(t)
			return transformTermFields{container: v.Container, value: v.Value}, ok
		},
		build: func(f transformTermFields) mutation.TypeTransform {
			return mutation.ContainerElementUnion{Container: f.container, Value: f.value}
		},
	},
	mutation.TransformToArray: {
		kind:    "mutation.toArray",
		payload: transformPayloadElement,
		read: func(t mutation.TypeTransform) (transformTermFields, bool) {
			v, ok := mutation.AsToArray(t)
			return transformTermFields{element: v.Element}, ok
		},
		build: func(f transformTermFields) mutation.TypeTransform {
			return mutation.ToArray{Element: f.element}
		},
	},
}

// transformWireVariantsByKind is the read side's index into the same rows, so a
// kind the vocabulary does not spell is unknown to the boundary by construction.
var transformWireVariantsByKind = func() map[string]mutation.TransformKind {
	byKind := make(map[string]mutation.TransformKind, mutation.TransformKindCount)
	for _, kind := range mutation.TransformKinds() {
		row := transformWireVariants[kind]
		if row.kind == "" {
			continue
		}
		byKind[row.kind] = kind
	}
	return byKind
}()

func encodeEffectTransform(transform mutation.TypeTransform) (*effectTransformWire, error) {
	if transform == nil {
		return nil, nil
	}
	kind := mutation.KindOfTransform(transform)
	if !kind.Valid() {
		return nil, fmt.Errorf("signature/wire: unsupported effect transform %T", transform)
	}
	row := transformWireVariants[kind]
	if row.kind == "" {
		return nil, fmt.Errorf("signature/wire: unsupported effect transform %T", transform)
	}
	// A transform handed to the boundary as a typed nil pointer carries a kind
	// but no payload to write, so the accessor refuses it here rather than the
	// codec dereferencing it.
	fields, ok := row.read(transform)
	if !ok {
		return nil, fmt.Errorf("signature/wire: nil effect transform %T", transform)
	}
	wire := &effectTransformWire{Kind: row.kind}
	switch row.payload {
	case transformPayloadNone:
	case transformPayloadSource:
		wire.Source = encodeParamRef(fields.source)
	case transformPayloadContainerValue:
		wire.Container = encodeParamRef(fields.container)
		wire.Value = encodeParamRef(fields.value)
	case transformPayloadElement:
		wire.Element = encodeParamRef(fields.element)
	default:
		return nil, fmt.Errorf("signature/wire: effect transform kind %q carries no stated wire payload", row.kind)
	}
	return wire, nil
}

func decodeEffectTransform(w *effectTransformWire) (mutation.TypeTransform, error) {
	if w == nil {
		return nil, nil
	}
	kind, known := transformWireVariantsByKind[w.Kind]
	if !known {
		return nil, fmt.Errorf("signature/wire: unknown effect transform kind %q", w.Kind)
	}
	row := transformWireVariants[kind]
	var fields transformTermFields
	switch row.payload {
	case transformPayloadNone:
	case transformPayloadSource:
		source, err := decodeRequiredParamRef(w.Source, row.kind+" source missing param ref")
		if err != nil {
			return nil, err
		}
		fields.source = source
	case transformPayloadContainerValue:
		container, err := decodeRequiredParamRef(w.Container, row.kind+" container missing param ref")
		if err != nil {
			return nil, err
		}
		value, err := decodeRequiredParamRef(w.Value, row.kind+" value missing param ref")
		if err != nil {
			return nil, err
		}
		fields.container, fields.value = container, value
	case transformPayloadElement:
		element, err := decodeRequiredParamRef(w.Element, row.kind+" element missing param ref")
		if err != nil {
			return nil, err
		}
		fields.element = element
	default:
		return nil, fmt.Errorf("signature/wire: effect transform kind %q carries no stated wire payload", row.kind)
	}
	return row.build(fields), nil
}
