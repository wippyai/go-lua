package manifest

import (
	"fmt"

	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/type/projection"
)

func encodeEffectReturn(ret returns.ReturnType) (*effectReturnWire, error) {
	if ret == nil {
		return nil, nil
	}
	switch r := ret.(type) {
	case returns.ElementOf:
		return &effectReturnWire{Kind: "returns.elementOf", Source: encodeParamRef(r.Source)}, nil
	case *returns.ElementOf:
		return encodeEffectReturn(*r)
	case returns.OptionalElementOf:
		return &effectReturnWire{Kind: "returns.optionalElementOf", Source: encodeParamRef(r.Source)}, nil
	case *returns.OptionalElementOf:
		return encodeEffectReturn(*r)
	case returns.CallbackReturn:
		return &effectReturnWire{Kind: "returns.callbackReturn", CallbackParam: encodeParamRef(r.CallbackParam)}, nil
	case *returns.CallbackReturn:
		return encodeEffectReturn(*r)
	case returns.ArrayOfCallbackReturn:
		return &effectReturnWire{Kind: "returns.arrayOfCallbackReturn", CallbackParam: encodeParamRef(r.CallbackParam)}, nil
	case *returns.ArrayOfCallbackReturn:
		return encodeEffectReturn(*r)
	case returns.SameAs:
		return &effectReturnWire{Kind: "returns.sameAs", Source: encodeParamRef(r.Source)}, nil
	case *returns.SameAs:
		return encodeEffectReturn(*r)
	case returns.TypeProjection:
		steps, err := encodeProjectionSteps(r.Projection.Steps)
		if err != nil {
			return nil, err
		}
		return &effectReturnWire{Kind: "returns.typeProjection", Source: encodeParamRef(r.Source), Projection: steps}, nil
	case *returns.TypeProjection:
		return encodeEffectReturn(*r)
	default:
		return nil, fmt.Errorf("manifest: unsupported return effect transform %T", ret)
	}
}

func decodeEffectReturn(w *effectReturnWire) (returns.ReturnType, error) {
	if w == nil {
		return nil, nil
	}
	switch w.Kind {
	case "returns.selectCaseOfParam":
		return nil, inactiveReturnTransform(returns.SelectCaseOfParam{})
	case "returns.selectResultOfCases":
		return nil, inactiveReturnTransform(returns.SelectResultOfCases{})
	case "returns.elementOf":
		return returns.ElementOf{Source: decodeParamRef(w.Source)}, nil
	case "returns.optionalElementOf":
		return returns.OptionalElementOf{Source: decodeParamRef(w.Source)}, nil
	case "returns.callbackReturn":
		return returns.CallbackReturn{CallbackParam: decodeParamRef(w.CallbackParam)}, nil
	case "returns.arrayOfCallbackReturn":
		return returns.ArrayOfCallbackReturn{CallbackParam: decodeParamRef(w.CallbackParam)}, nil
	case "returns.sameAs":
		return returns.SameAs{Source: decodeParamRef(w.Source)}, nil
	case "returns.deepElementOf":
		return nil, inactiveReturnTransform(returns.DeepElementOf{})
	case "returns.stringUnpackValue":
		return nil, inactiveReturnTransform(returns.StringUnpackValue{})
	case "returns.typeProjection":
		steps, err := decodeProjectionSteps(w.Projection)
		if err != nil {
			return nil, err
		}
		return returns.TypeProjection{Source: decodeParamRef(w.Source), Projection: projection.Projection{Steps: steps}}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown return effect transform kind %q", w.Kind)
	}
}

func inactiveReturnTransform(transform returns.ReturnType) error {
	desc, ok := caplabel.DescriptorForReturnTransform(transform)
	if !ok {
		return fmt.Errorf("manifest: unsupported return effect transform %T", transform)
	}
	return inactiveManifestEffectLabelError(desc)
}
