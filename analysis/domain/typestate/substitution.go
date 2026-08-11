package typestate

import "github.com/wippyai/go-lua/analysis/domain/materialization"

// Substitution is one exact finite resource/holder boundary rewrite inside a
// homogeneous Typestate family. It introduces no alternate origin namespace.
type Substitution struct {
	resources map[ResourceOrigin]ResourceOrigin
	holders   map[HolderOrigin]HolderOrigin
}

func NewSubstitution(resources [][2]ResourceOrigin, holders [][2]HolderOrigin) (Substitution, bool) {
	out := Substitution{
		resources: make(map[ResourceOrigin]ResourceOrigin, len(resources)),
		holders:   make(map[HolderOrigin]HolderOrigin, len(holders)),
	}
	for _, pair := range resources {
		// Ordinary boundary transport changes only the Link-proved raw origin.
		// Materialization is deliberately not a substitution: preserving role
		// here prevents a hidden Recent→Summary rekey in call transport.
		if !pair[0].valid() || !pair[1].valid() || pair[0].source != pair[1].source || pair[0].role != pair[1].role {
			return Substitution{}, false
		}
		if _, duplicate := out.resources[pair[0]]; duplicate {
			return Substitution{}, false
		}
		out.resources[pair[0]] = pair[1]
	}
	for _, pair := range holders {
		if !pair[0].valid() || !pair[1].valid() || pair[0].source != pair[1].source {
			return Substitution{}, false
		}
		if _, duplicate := out.holders[pair[0]]; duplicate {
			return Substitution{}, false
		}
		out.holders[pair[0]] = pair[1]
	}
	return out, true
}

// Materialize performs Typestate's only materialization rekey.  It advances a
// Recent resource to the Summary key for the same raw Link origin and joins
// its relation with the already-present destination fact.  Ordinary
// Substitution cannot perform this rewrite.
func (a Algebra) Materialize(recent Fact, destination Fact) (Fact, bool) {
	if !a.validFact(recent) || !a.validFact(destination) || recent.Key.Resource.role != materialization.Recent {
		return Fact{}, false
	}
	summary, ok := materializeResource(recent.Key.Resource.raw, materialization.Summary)
	if !ok {
		return Fact{}, false
	}
	key, ok := a.schema.Admit(summary)
	if !ok || destination.Key != key {
		return Fact{}, false
	}
	var moved Relation
	if recent.Value.IsTop() {
		moved = a.Top()
	} else {
		moved, ok = a.Of(key, recent.Value.Entries()...)
		if !ok {
			return Fact{}, false
		}
	}
	joined := a.Join(destination.Value, moved)
	if !a.validFact(Fact{Key: key, Value: joined}) {
		return Fact{}, false
	}
	return Fact{Key: key, Value: joined}, true
}

func (s Substitution) Resource(origin ResourceOrigin) ResourceOrigin {
	if replacement, ok := s.resources[origin]; ok {
		return replacement
	}
	return origin
}

func (s Substitution) Holder(origin HolderOrigin) HolderOrigin {
	if replacement, ok := s.holders[origin]; ok {
		return replacement
	}
	return origin
}

// Fact pairs a key with its homogeneous carrier value only at a boundary.
type Fact struct {
	Key   Key
	Value Relation
}

// Substitute applies key and holder transport atomically, then validates the
// result against the exact destination row in the same family schema.
func (a Algebra) Substitute(fact Fact, substitution Substitution) (Fact, bool) {
	if !a.validFact(fact) {
		return Fact{}, false
	}
	fact.Key.Resource = substitution.Resource(fact.Key.Resource)
	if _, ok := a.schema.Admit(fact.Key.Resource); !ok {
		return Fact{}, false
	}
	if fact.Value.IsTop() || fact.Value.IsBottom() {
		return fact, true
	}
	entries := fact.Value.Entries()
	for i := range entries {
		entries[i].Holder = substitution.Holder(entries[i].Holder)
	}
	value, ok := a.Of(fact.Key, entries...)
	if !ok {
		return Fact{}, false
	}
	fact.Value = value
	return fact, true
}
