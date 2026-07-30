package mutation

import "github.com/wippyai/go-lua/analysis/domain/effect"

// PathInvalidationTarget projects the path authority carried by partial
// mutation labels. Shape transforms and value payloads remain metadata until
// precise mutation semantics are implemented.
func PathInvalidationTarget(label effect.Label) (effect.ParamRef, bool) {
	switch normalized := effect.NormalizeLabel(label).(type) {
	case Mutate:
		return normalized.Target, true
	case TableMutator:
		return normalized.Target, true
	case LengthChange:
		return normalized.Target, true
	default:
		return effect.ParamRef{}, false
	}
}

// PositiveLengthFloor projects the active length proof carried by a positive
// LengthChange. Zero and negative deltas are not active length proofs.
func PositiveLengthFloor(label effect.Label) (effect.ParamRef, int, bool) {
	normalized, ok := effect.NormalizeLabel(label).(LengthChange)
	if !ok || normalized.Delta <= 0 {
		return effect.ParamRef{}, 0, false
	}
	return normalized.Target, normalized.Delta, true
}
