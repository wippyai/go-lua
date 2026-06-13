package manifest

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/type/projection"
)

func encodeEffectReturn(ret returns.ReturnType) (*effectReturnWire, error) {
	if ret == nil {
		return nil, nil
	}
	switch r := ret.(type) {
	case returns.SelectCaseOfParam:
		return &effectReturnWire{Kind: "returns.selectCaseOfParam", Source: encodeParamRef(r.Source)}, nil
	case *returns.SelectCaseOfParam:
		return encodeEffectReturn(*r)
	case returns.SelectResultOfCases:
		return &effectReturnWire{
			Kind:    "returns.selectResultOfCases",
			Cases:   encodeParamRef(r.Cases),
			Default: encodeParamRef(r.Default),
		}, nil
	case *returns.SelectResultOfCases:
		return encodeEffectReturn(*r)
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
	case returns.DeepElementOf:
		return &effectReturnWire{Kind: "returns.deepElementOf", Source: encodeParamRef(r.Source)}, nil
	case *returns.DeepElementOf:
		return encodeEffectReturn(*r)
	case returns.StringUnpackValue:
		return &effectReturnWire{Kind: "returns.stringUnpackValue", Format: encodeParamRef(r.Format)}, nil
	case *returns.StringUnpackValue:
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
		return returns.SelectCaseOfParam{Source: decodeParamRef(w.Source)}, nil
	case "returns.selectResultOfCases":
		return returns.SelectResultOfCases{Cases: decodeParamRef(w.Cases), Default: decodeParamRef(w.Default)}, nil
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
		return returns.DeepElementOf{Source: decodeParamRef(w.Source)}, nil
	case "returns.stringUnpackValue":
		return returns.StringUnpackValue{Format: decodeParamRef(w.Format)}, nil
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
