package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// AttachInferredErrorReturnSpec enriches function types with a canonical
// ErrorReturn effect when the function body proves the `(value, err)` pattern.
func AttachInferredErrorReturnSpec(
	fn *typ.Function,
	graph *cfg.Graph,
	solution *flow.Solution,
	synth api.Synth,
) *typ.Function {
	return erreffect.AttachInferredErrorReturnSpec(fn, graph, solution, synth)
}

func HasErrorReturnLabel(fn *typ.Function) bool {
	return erreffect.HasErrorReturnLabel(fn)
}

func HasStrictInverseReturnPattern(
	graph *cfg.Graph,
	solution *flow.Solution,
	synth api.BaseSynth,
	valueIdx int,
	errorIdx int,
) bool {
	return erreffect.HasStrictInverseReturnPattern(graph, solution, synth, valueIdx, errorIdx)
}

func AttachErrorReturnSpec(fn *typ.Function, valueIndex, errorIndex int) *typ.Function {
	return erreffect.AttachErrorReturnSpec(fn, valueIndex, errorIndex)
}
