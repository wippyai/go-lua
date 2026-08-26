// Package memberdefinition is the generator-only owner source for the opaque
// Call-to-Effect rule's own fold. It is imported by the member definition
// roster and by nothing at runtime, so the callsite package keeps its judgment
// and none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	effectbase "github.com/wippyai/go-lua/domain/effect/memberdefinition"
)

// Contribution is the opaque reading's fold: the same seed targets resolve to
// the one unknown part the Effect algebra publishes per operation, and the
// call value's opaque alternative joins them.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis:        "effect",
		Rule:        "effect-opaque",
		Carriers:    []definition.Carrier{effectbase.CallFactCarrier(), effectbase.MountedCallCarrier()},
		Relations:   []definition.Relation{effectbase.EffectSites()},
		Projections: []definition.Projection{effectbase.EffectSiteKey()},
		Reducers: []definition.Reducer{
			effectbase.CallsiteReducer("OpaqueCallEffectReducer", "effect/callsite-opaque/reducer", "DeriveOpaque"),
		},
	}
}
