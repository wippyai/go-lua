package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type lifecycleFactKey struct {
	target   pathdom.PathKey
	protocol typestate.Protocol
	kind     callboundary.LifecycleKind
	from     typestate.State
	to       typestate.State
	final    typestate.State
}

// lifecycleLane is a must-fact lane for cross-boundary typestate effects.
// Facts survive summary joins only when every normal-return path publishes the
// same lifecycle update for the same placeholder or captured concrete target.
var lifecycleLane = factset.Set[lifecycleFactKey, callboundary.LifecycleFact]{
	Key:       lifecycleKeyOf,
	EqualFact: func(a, b callboundary.LifecycleFact) bool { return lifecycleKeyOf(a) == lifecycleKeyOf(b) },
	Less:      lifecycleFactLess,
	Valid: func(f callboundary.LifecycleFact) bool {
		return !f.Target.IsEmpty() && f.Protocol != "" && f.Kind != callboundary.LifecycleNone
	},
	CloneFact: func(f callboundary.LifecycleFact) callboundary.LifecycleFact {
		f.Target = f.Target.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.LifecycleFact) bool { return true },
	Intersect: true,
}

func lifecycleKeyOf(f callboundary.LifecycleFact) lifecycleFactKey {
	return lifecycleFactKey{
		target:   f.Target.Key(),
		protocol: f.Protocol,
		kind:     f.Kind,
		from:     f.From,
		to:       f.To,
		final:    f.Obligation.Final,
	}
}

func lifecycleFactLess(a, b callboundary.LifecycleFact) bool {
	if !a.Target.Equal(b.Target) {
		return a.Target.Less(b.Target)
	}
	if a.Protocol != b.Protocol {
		return a.Protocol < b.Protocol
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.From != b.From {
		return a.From < b.From
	}
	if a.To != b.To {
		return a.To < b.To
	}
	return a.Obligation.Final < b.Obligation.Final
}
