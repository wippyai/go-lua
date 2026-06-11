package summary

import (
	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// FromResult projects one completed check result into a canonical summary.
func FromResult(result *check.Result) Summary {
	if result == nil {
		return Summary{}
	}
	reg := result.Registry()
	graph := result.Graph()
	exit, ok := result.ExitState()
	if reg == nil || graph == nil || !ok {
		return Summary{}
	}

	arity := 0
	for _, point := range result.ReturnPoints() {
		pointArity, ok := result.ReturnArity(point)
		if ok && pointArity > arity {
			arity = pointArity
		}
	}
	if arity == 0 {
		return Summary{}
	}

	slots := make([]product.Value, arity)
	for i := range slots {
		slots[i] = exit.ReadValue(reg, key.ReturnSlot(i))
	}
	return Normalize(reg, Summary{Returns: slots})
}
