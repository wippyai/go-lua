package callboundary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

type lifecycleFactKey struct {
	target   pathdom.PathKey
	protocol typestate.Protocol
	kind     LifecycleKind
	from     typestate.State
	to       typestate.State
	final    typestate.State
	finals   typestate.FinalStates
}

// lifecycleLane is a must-fact lane for cross-boundary typestate effects.
// Facts survive summary joins only when every normal-return path publishes the
// same lifecycle update for the same placeholder or captured concrete target.
var lifecycleLane = factset.Set[lifecycleFactKey, LifecycleFact]{
	Key:       lifecycleKeyOf,
	EqualFact: func(a, b LifecycleFact) bool { return lifecycleKeyOf(a) == lifecycleKeyOf(b) },
	Less:      lifecycleFactLess,
	Valid: func(f LifecycleFact) bool {
		return !f.Target.IsEmpty() && f.Protocol != "" && f.Kind != LifecycleNone
	},
	CloneFact: func(f LifecycleFact) LifecycleFact {
		f.Target = f.Target.Clone()
		return f
	},
	Prefer:    func(kept, incoming LifecycleFact) bool { return true },
	Intersect: true,
}

func lifecycleKeyOf(f LifecycleFact) lifecycleFactKey {
	return lifecycleFactKey{
		target:   f.Target.Key(),
		protocol: f.Protocol,
		kind:     f.Kind,
		from:     f.From,
		to:       f.To,
		final:    f.Obligation.Final,
		finals:   f.Obligation.Finals,
	}
}

func lifecycleFactLess(a, b LifecycleFact) bool {
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
	if a.Obligation.Final != b.Obligation.Final {
		return a.Obligation.Final < b.Obligation.Final
	}
	return a.Obligation.Finals < b.Obligation.Finals
}
