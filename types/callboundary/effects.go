package callboundary

import (
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
)

// Effects groups caller-visible side effects selected at one call boundary.
// It is below canonical/transfer so call-boundary projection can construct the
// carrier without knowing how transfer later applies each fact lane.
type Effects struct {
	CellEffects     flow.CaptureEffects
	ReceiverEffects flow.ReceiverEffects
	BoundaryFacts   flow.BoundaryFacts
	ElementUnions   []effect.ContainerElementUnion
}

func EmptyEffects() Effects {
	return Effects{
		CellEffects:     flow.CaptureEffectsDomain.Bottom(),
		ReceiverEffects: flow.ReceiverEffectsDomain.Bottom(),
		BoundaryFacts:   flow.BoundaryFactsDomain.Top(),
	}
}

func EffectsOf(
	cellEffects flow.CaptureEffects,
	receiverEffects flow.ReceiverEffects,
	boundaryFacts flow.BoundaryFacts,
	elementUnions []effect.ContainerElementUnion,
) Effects {
	return Effects{
		CellEffects:     cellEffects,
		ReceiverEffects: receiverEffects,
		BoundaryFacts:   boundaryFacts,
		ElementUnions:   append([]effect.ContainerElementUnion(nil), elementUnions...),
	}
}
