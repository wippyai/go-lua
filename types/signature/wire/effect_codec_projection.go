package wire

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/type/projection"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// The projection-step vocabulary is the projection package's. What this package
// owns is the boundary spelling of its members: the kind string written for
// each step and which projectionStepWire fields that kind carries. That
// spelling lives in the one table below, so the write side and the read side
// consult a single statement instead of two hand-kept switches.
//
// The tokens here are the codec's serialization commitment and are deliberately
// not the tokens Step.String writes: the display spelling of a step is free to
// change, the wire spelling is not.

// projectionWirePayload names the projectionStepWire fields a kind carries. It
// is the field applicability rule of the wire struct, stated once beside the
// vocabulary: encode writes exactly the named fields and decode reads exactly
// them, so neither side can quietly carry a field the other ignores.
type projectionWirePayload uint8

const (
	projectionPayloadNone projectionWirePayload = iota
	projectionPayloadField
	projectionPayloadIndex
	projectionPayloadType
)

// projectionStepWireVariant is one vocabulary member's boundary spelling: the
// kind written for it, the wire fields that kind carries, and the step rebuilt
// from them.
type projectionStepWireVariant struct {
	kind    string
	payload projectionWirePayload
	build   func(field string, index int, generic typ.Type) projection.Step
}

// projectionStepWireVariants is the boundary vocabulary, one row per step kind,
// indexed by the kind's own ordinal.
var projectionStepWireVariants = [projection.StepKindCount + 1]projectionStepWireVariant{
	projection.StepField: {
		kind:    "field",
		payload: projectionPayloadField,
		build: func(field string, _ int, _ typ.Type) projection.Step {
			return projection.Field(field)
		},
	},
	projection.StepCallableReturn: {
		kind:    "callableReturn",
		payload: projectionPayloadNone,
		build: func(string, int, typ.Type) projection.Step {
			return projection.CallableReturn()
		},
	},
	projection.StepGenericArg: {
		kind:    "genericArg",
		payload: projectionPayloadIndex,
		build: func(_ string, index int, _ typ.Type) projection.Step {
			return projection.GenericArg(index)
		},
	},
	projection.StepInstantiateGeneric: {
		kind:    "instantiateGeneric",
		payload: projectionPayloadType,
		build: func(_ string, _ int, generic typ.Type) projection.Step {
			return projection.InstantiateGeneric(generic)
		},
	},
}

// projectionStepWireVariantsByKind is the read side's index into the same rows,
// so a kind the vocabulary does not spell is unknown to the boundary by
// construction.
var projectionStepWireVariantsByKind = func() map[string]projection.StepKind {
	byKind := make(map[string]projection.StepKind, projection.StepKindCount)
	for _, kind := range projection.StepKinds() {
		row := projectionStepWireVariants[kind]
		if row.kind == "" {
			continue
		}
		byKind[row.kind] = kind
	}
	return byKind
}()

func encodeProjectionSteps(steps []projection.Step) ([]projectionStepWire, error) {
	out := make([]projectionStepWire, 0, len(steps))
	for _, step := range steps {
		if !step.Kind.Valid() {
			return nil, fmt.Errorf("signature/wire: unknown projection step kind %d", step.Kind)
		}
		row := projectionStepWireVariants[step.Kind]
		if row.kind == "" {
			return nil, fmt.Errorf("signature/wire: unknown projection step kind %d", step.Kind)
		}
		encoded := projectionStepWire{Kind: row.kind}
		switch row.payload {
		case projectionPayloadNone:
		case projectionPayloadField:
			encoded.Field = step.Field
		case projectionPayloadIndex:
			encoded.Index = encodeInt(step.Index)
		case projectionPayloadType:
			t, err := EncodeType(step.Type)
			if err != nil {
				return nil, err
			}
			encoded.Type = t
		default:
			return nil, fmt.Errorf("signature/wire: projection step kind %q carries no stated wire payload", row.kind)
		}
		out = append(out, encoded)
	}
	return out, nil
}

func decodeProjectionSteps(w []projectionStepWire) ([]projection.Step, error) {
	steps := make([]projection.Step, 0, len(w))
	for _, step := range w {
		kind, known := projectionStepWireVariantsByKind[step.Kind]
		if !known {
			return nil, fmt.Errorf("signature/wire: unknown projection step kind %q", step.Kind)
		}
		row := projectionStepWireVariants[kind]
		var (
			field   string
			index   int
			generic typ.Type
		)
		switch row.payload {
		case projectionPayloadNone:
		case projectionPayloadField:
			field = step.Field
		case projectionPayloadIndex:
			decoded, err := decodeRequiredInt(step.Index, "projection "+row.kind+" index missing")
			if err != nil {
				return nil, err
			}
			index = decoded
		case projectionPayloadType:
			decoded, err := DecodeType(step.Type)
			if err != nil {
				return nil, err
			}
			generic = decoded
		default:
			return nil, fmt.Errorf("signature/wire: projection step kind %q carries no stated wire payload", row.kind)
		}
		steps = append(steps, row.build(field, index, generic))
	}
	return steps, nil
}
