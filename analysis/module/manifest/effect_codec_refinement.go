package manifest

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
)

func encodeEffectRefinement(refinement postcondition.Refinement) (*effectRefinementWire, error) {
	if refinement == nil {
		return nil, fmt.Errorf("manifest: missing effect refinement")
	}
	switch r := refinement.(type) {
	case postcondition.Present:
		return &effectRefinementWire{Kind: r.Kind()}, nil
	case *postcondition.Present:
		if r == nil {
			return nil, fmt.Errorf("manifest: missing effect refinement")
		}
		return encodeEffectRefinement(*r)
	case postcondition.Absent:
		return &effectRefinementWire{Kind: r.Kind()}, nil
	case *postcondition.Absent:
		if r == nil {
			return nil, fmt.Errorf("manifest: missing effect refinement")
		}
		return encodeEffectRefinement(*r)
	default:
		return nil, fmt.Errorf("manifest: unsupported effect refinement %T", refinement)
	}
}

func decodeEffectRefinement(w *effectRefinementWire) (postcondition.Refinement, error) {
	if w == nil {
		return nil, fmt.Errorf("manifest: missing effect refinement")
	}
	switch w.Kind {
	case postcondition.PresentKind:
		return postcondition.Present{}, nil
	case postcondition.AbsentKind:
		return postcondition.Absent{}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown effect refinement kind %q", w.Kind)
	}
}

func encodeEffectTransform(transform mutation.TypeTransform) (*effectTransformWire, error) {
	if transform == nil {
		return nil, nil
	}
	switch t := transform.(type) {
	case mutation.Unchanged:
		return &effectTransformWire{Kind: "mutation.unchanged"}, nil
	case *mutation.Unchanged:
		return encodeEffectTransform(*t)
	case mutation.ElementUnion:
		return &effectTransformWire{Kind: "mutation.elementUnion", Source: encodeParamRef(t.Source)}, nil
	case *mutation.ElementUnion:
		return encodeEffectTransform(*t)
	case mutation.ContainerElementUnion:
		return &effectTransformWire{
			Kind:      "mutation.containerElementUnion",
			Container: encodeParamRef(t.Container),
			Value:     encodeParamRef(t.Value),
		}, nil
	case *mutation.ContainerElementUnion:
		return encodeEffectTransform(*t)
	case mutation.ToArray:
		return &effectTransformWire{Kind: "mutation.toArray", Element: encodeParamRef(t.Element)}, nil
	case *mutation.ToArray:
		return encodeEffectTransform(*t)
	default:
		return nil, fmt.Errorf("manifest: unsupported effect transform %T", transform)
	}
}

func decodeEffectTransform(w *effectTransformWire) (mutation.TypeTransform, error) {
	if w == nil {
		return nil, nil
	}
	switch w.Kind {
	case "mutation.unchanged":
		return mutation.Unchanged{}, nil
	case "mutation.elementUnion":
		source, err := decodeRequiredParamRef(w.Source, "mutation.elementUnion source missing param ref")
		if err != nil {
			return nil, err
		}
		return mutation.ElementUnion{Source: source}, nil
	case "mutation.containerElementUnion":
		container, err := decodeRequiredParamRef(w.Container, "mutation.containerElementUnion container missing param ref")
		if err != nil {
			return nil, err
		}
		value, err := decodeRequiredParamRef(w.Value, "mutation.containerElementUnion value missing param ref")
		if err != nil {
			return nil, err
		}
		return mutation.ContainerElementUnion{Container: container, Value: value}, nil
	case "mutation.toArray":
		element, err := decodeRequiredParamRef(w.Element, "mutation.toArray element missing param ref")
		if err != nil {
			return nil, err
		}
		return mutation.ToArray{Element: element}, nil
	default:
		return nil, fmt.Errorf("manifest: unknown effect transform kind %q", w.Kind)
	}
}
