package manifest

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/type/projection"
)

func encodeProjectionSteps(steps []projection.Step) ([]projectionStepWire, error) {
	out := make([]projectionStepWire, 0, len(steps))
	for _, step := range steps {
		encoded := projectionStepWire{}
		switch step.Kind {
		case projection.StepField:
			encoded.Kind = "field"
			encoded.Field = step.Field
		case projection.StepCallableReturn:
			encoded.Kind = "callableReturn"
		case projection.StepGenericArg:
			encoded.Kind = "genericArg"
			encoded.Index = step.Index
		case projection.StepInstantiateGeneric:
			encoded.Kind = "instantiateGeneric"
			t, err := encodeType(step.Type)
			if err != nil {
				return nil, err
			}
			encoded.Type = t
		default:
			return nil, fmt.Errorf("manifest: unknown projection step kind %d", step.Kind)
		}
		out = append(out, encoded)
	}
	return out, nil
}

func decodeProjectionSteps(w []projectionStepWire) ([]projection.Step, error) {
	steps := make([]projection.Step, 0, len(w))
	for _, step := range w {
		switch step.Kind {
		case "field":
			steps = append(steps, projection.Field(step.Field))
		case "callableReturn":
			steps = append(steps, projection.CallableReturn())
		case "genericArg":
			steps = append(steps, projection.GenericArg(step.Index))
		case "instantiateGeneric":
			t, err := decodeType(step.Type)
			if err != nil {
				return nil, err
			}
			steps = append(steps, projection.InstantiateGeneric(t))
		default:
			return nil, fmt.Errorf("manifest: unknown projection step kind %q", step.Kind)
		}
	}
	return steps, nil
}
