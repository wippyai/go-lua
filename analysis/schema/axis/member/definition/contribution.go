package definition

import "github.com/wippyai/go-lua/analysis/schema"

// Contribution is one rule's own share of its axis's member vocabulary: the
// reducer definition that rule folds with, and the carrier rows that reducer's
// signature is typed in when the axis base does not already declare them.
//
// A reducer belongs to the rule that folds with it. The owner-qualified
// symbol, the input roles it consumes, and the fact it produces are that
// rule's statement of how it decides, so they are declared beside its Program
// rather than in a list the axis keeps of every rule that ever wrote it. The
// axis member definition is the sealed fold of the contributions the
// composition registers, which is the same composition law rule.Spec is
// admitted under: nothing registers itself, declarations are values handed to
// the fold.
//
// The consequence is the property this shape exists for: adding a rule adds a
// contribution to that rule's package and a row to the roster, and edits no
// third place.
type Contribution struct {
	// Axis is the axis whose member vocabulary this contribution extends. It
	// must be the axis the base declares: a contribution cannot place a reducer
	// in a foreign owner's catalog.
	Axis schema.Key
	// Rule is the rule that declares this contribution. It is the provenance
	// every composed reducer row carries, so a generated row traces back to
	// exactly one declaration and a diff is attributable to one rule.
	Rule schema.Key
	// Carriers are the carrier rows this contribution's reducer signature needs
	// and the base does not declare. A carrier the base already declares may be
	// repeated verbatim; a repeat that disagrees is two declarations of one
	// name and is refused.
	Carriers []Carrier
	// Reducers are this rule's reducer definitions, in declaration order.
	Reducers []Reducer
}

// Available reports whether this contribution identifies one rule of one axis
// and declares at least one reducer. A contribution that declares none is not
// an empty contribution: it is a rule claiming a fold it did not state.
func (contribution Contribution) Available() bool {
	if !contribution.Axis.Available() || !contribution.Rule.Available() || len(contribution.Reducers) == 0 {
		return false
	}
	for _, carrier := range contribution.Carriers {
		if !identifierAvailable(carrier.Name) || !carrier.Key.Available() || !carrier.Type.Available() {
			return false
		}
	}
	for _, reducer := range contribution.Reducers {
		if !identifierAvailable(reducer.Name) || !reducer.Key.Available() || !reducer.Implementation.Available() || len(reducer.Outputs) == 0 {
			return false
		}
	}
	return true
}

// Clone returns an independent contribution.
func (contribution Contribution) Clone() Contribution {
	clone := contribution
	clone.Carriers = append([]Carrier(nil), contribution.Carriers...)
	clone.Reducers = make([]Reducer, len(contribution.Reducers))
	for index, reducer := range contribution.Reducers {
		clone.Reducers[index] = reducer
		clone.Reducers[index].Inputs = append([]ReducerInput(nil), reducer.Inputs...)
		clone.Reducers[index].Outputs = append([]ReducerOutput(nil), reducer.Outputs...)
		clone.Reducers[index].Implementation = cloneSymbol(reducer.Implementation)
	}
	return clone
}

// Source is one axis's whole authored member vocabulary: the base its owner
// declares - carriers, relations, projections, carry transforms, and the key
// binding - and the reducer contributions its rules declare.
//
// Package is the Go package the generated artifacts belong to; Name is the
// identity the generator selects this source by.
type Source struct {
	Package       string
	Name          string
	Base          Definition
	Contributions []Contribution
}

// Compose seals the base and its contributions into the one axis member
// definition the generator renders.
//
// The base may not declare a reducer of its own. A reducer authored beside the
// carriers is a reducer no rule is on record as folding with, and it is the
// central per-axis list this composition replaces, so it is refused here
// rather than merged.
func (source Source) Compose() (Definition, bool) {
	base := source.Base.Clone()
	if len(base.Reducers) != 0 || !base.Axis.Available() {
		return Definition{}, false
	}
	carriers := make(map[string]Carrier, len(base.Carriers))
	keys := make(map[Carrier]struct{}, len(base.Carriers))
	for _, carrier := range base.Carriers {
		carriers[carrier.Name] = carrier
		keys[carrier] = struct{}{}
	}
	rules := make(map[schema.Key]struct{}, len(source.Contributions))
	reducerNames := make(map[string]struct{}, len(source.Contributions))
	reducerKeys := make(map[schema.Key]struct{}, len(source.Contributions))
	composed := make([]Reducer, 0, len(source.Contributions))
	for _, contribution := range source.Contributions {
		if !contribution.Available() || contribution.Axis != base.Axis {
			return Definition{}, false
		}
		if _, duplicate := rules[contribution.Rule]; duplicate {
			return Definition{}, false
		}
		rules[contribution.Rule] = struct{}{}
		for _, carrier := range contribution.Carriers {
			existing, declared := carriers[carrier.Name]
			if declared {
				if existing != carrier {
					return Definition{}, false
				}
				continue
			}
			if _, taken := keys[carrier]; taken {
				return Definition{}, false
			}
			carriers[carrier.Name] = carrier
			keys[carrier] = struct{}{}
			base.Carriers = append(base.Carriers, carrier)
		}
		for _, reducer := range contribution.Reducers {
			if _, duplicate := reducerNames[reducer.Name]; duplicate {
				return Definition{}, false
			}
			if _, duplicate := reducerKeys[reducer.Key]; duplicate {
				return Definition{}, false
			}
			reducerNames[reducer.Name] = struct{}{}
			reducerKeys[reducer.Key] = struct{}{}
			row := reducer
			row.Rule = contribution.Rule
			row.Inputs = append([]ReducerInput(nil), reducer.Inputs...)
			row.Outputs = append([]ReducerOutput(nil), reducer.Outputs...)
			composed = append(composed, row)
		}
	}
	base.Reducers = composed
	if !base.Complete() {
		return Definition{}, false
	}
	return base, true
}

// Roster is the composition's ordered registry of axis member sources. It is
// the one list the generator selects a source from, and the one place a new
// axis or a new rule contribution is registered.
type Roster struct {
	sources []Source
}

// NewRoster admits an ordered set of axis sources. Two sources naming one axis
// or one generator name are two owners of one vocabulary and are refused.
func NewRoster(sources ...Source) (Roster, bool) {
	names := make(map[string]struct{}, len(sources))
	axes := make(map[schema.Key]struct{}, len(sources))
	admitted := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source.Name == "" || source.Package == "" || !packagePathAvailable(source.Package) || !source.Base.Axis.Available() {
			return Roster{}, false
		}
		if _, duplicate := names[source.Name]; duplicate {
			return Roster{}, false
		}
		if _, duplicate := axes[source.Base.Axis]; duplicate {
			return Roster{}, false
		}
		names[source.Name] = struct{}{}
		axes[source.Base.Axis] = struct{}{}
		admitted = append(admitted, source)
	}
	return Roster{sources: admitted}, len(admitted) > 0
}

// Count is the number of registered axis sources.
func (roster Roster) Count() int { return len(roster.sources) }

// At returns one registered source in roster order.
func (roster Roster) At(index int) (Source, bool) {
	if index < 0 || index >= len(roster.sources) {
		return Source{}, false
	}
	return roster.sources[index], true
}

// Source resolves one registered axis source by its generator name.
func (roster Roster) Source(name string) (Source, bool) {
	for _, source := range roster.sources {
		if source.Name == name {
			return source, true
		}
	}
	return Source{}, false
}

// Definition resolves and composes one registered axis source.
func (roster Roster) Definition(name string) (string, Definition, bool) {
	source, sourceOK := roster.Source(name)
	if !sourceOK {
		return "", Definition{}, false
	}
	composed, composedOK := source.Compose()
	if !composedOK {
		return "", Definition{}, false
	}
	return source.Package, composed, true
}

// packagePathAvailable states the roster's own fence on a source's Go package
// name: it is the identifier a generated file declares, not a path.
func packagePathAvailable(name string) bool { return identifierAvailable(name) }
