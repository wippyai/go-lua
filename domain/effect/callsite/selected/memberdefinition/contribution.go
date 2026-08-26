// Package memberdefinition is the generator-only owner source for the exact
// Call-to-Effect rule's own fold. It is imported by the member definition
// roster and by nothing at runtime, so the callsite package keeps its judgment
// and none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	effectbase "github.com/wippyai/go-lua/domain/effect/memberdefinition"
)

// Contribution is the exact reading's fold: every seed target of the call
// resolves to the effect bindings its operation declares.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis:     "effect",
		Rule:     "effect-selected",
		Carriers: []definition.Carrier{effectbase.CallFactCarrier()},
		Reducers: []definition.Reducer{
			effectbase.CallsiteReducer("SelectedCallEffectReducer", "effect/callsite-selected/reducer", "DeriveSelected"),
		},
	}
}
