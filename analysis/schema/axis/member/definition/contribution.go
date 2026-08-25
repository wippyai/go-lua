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
	// Relations are the owner-issued relations this rule's fold reads and the
	// base does not declare. A rule's relations are that rule's declaration for
	// the same reason its reducer is: the rows it folds over are part of how it
	// decides. Pushing them into the axis base would make the base the file
	// every new rule has to edit, which is the choke point contributions exist
	// to remove. A relation the base already declares may be repeated verbatim;
	// a repeat that disagrees is two declarations of one name and is refused.
	Relations []Relation
	// Projections are the projections over those relations, declared with them
	// under the same law. A projection names a relation, so declaring the two
	// apart would let a rule contribute a projection over rows it never
	// declared.
	Projections []Projection
	// Reducers are this rule's reducer definitions, in declaration order.
	Reducers []Reducer
}

// relationsAgree reports whether two declarations of one relation name state
// the same relation. Relation carries slices, so identity is stated here
// rather than left to comparison.
func relationsAgree(left, right Relation) bool {
	if left.Name != right.Name || left.Key != right.Key || left.Subject != right.Subject || left.Axis != right.Axis ||
		left.CandidateProvider != right.CandidateProvider || left.CandidateResolver != right.CandidateResolver ||
		left.CandidateOrdinal != right.CandidateOrdinal || left.CandidateAt != right.CandidateAt ||
		left.CandidateCount != right.CandidateCount || left.Materialize != right.Materialize ||
		left.CandidateIdentityAt != right.CandidateIdentityAt {
		return false
	}
	if len(left.Inputs) != len(right.Inputs) {
		return false
	}
	for index, input := range left.Inputs {
		if input != right.Inputs[index] {
			return false
		}
	}
	if !correspondencesAgree(left.Correspondences, right.Correspondences) {
		return false
	}
	return derivationsAgree(left.Derivation, right.Derivation)
}

// derivationsAgree reports whether two relation derivations are the same
// construction, static axis order included.
func derivationsAgree(left, right RelationDerivation) bool {
	if left.State != right.State || left.Build != right.Build || left.Count != right.Count || left.At != right.At {
		return false
	}
	if len(left.StaticAxes) != len(right.StaticAxes) {
		return false
	}
	for index, axis := range left.StaticAxes {
		if axis != right.StaticAxes[index] {
			return false
		}
	}
	return true
}

// Available reports whether this contribution identifies one rule of one axis
// and declares at least one reducer. A contribution that declares none is not
// an empty contribution: it is a rule claiming a fold it did not state.
//
// The reducer clause is the AUTHORED law and the roster states it over what a
// package wrote. It is deliberately not Compose's gate: the roster folds a
// contribution's rows into the source of the axis each row names, so a rule
// that declares rows of a foreign axis reaches that axis's composition
// carrying rows and no fold, which is the correct shape rather than an empty
// claim.
func (contribution Contribution) Available() bool {
	return len(contribution.Reducers) != 0 && contribution.rowsAvailable()
}

// rowsAvailable reports whether every row this contribution declares is a
// complete declaration of one rule of one axis.
func (contribution Contribution) rowsAvailable() bool {
	if !contribution.Axis.Available() || !contribution.Rule.Available() {
		return false
	}
	for _, carrier := range contribution.Carriers {
		if !identifierAvailable(carrier.Name) || !carrier.Key.Available() || !carrier.Type.Available() {
			return false
		}
	}
	for _, relation := range contribution.Relations {
		if !identifierAvailable(relation.Name) || !relation.Key.Available() || !identifierAvailable(relation.Subject) {
			return false
		}
	}
	for _, projection := range contribution.Projections {
		if !identifierAvailable(projection.Name) || !projection.Key.Available() || !identifierAvailable(projection.Relation) || !identifierAvailable(projection.Result) {
			return false
		}
	}
	for _, reducer := range contribution.Reducers {
		if !identifierAvailable(reducer.Name) || !reducer.Key.Available() || !reducer.Implementation.Available() {
			return false
		}
		// A fold's declared outputs are the facts it publishes. A structural
		// fold publishes none - its whole result is the disposition of the
		// branch it was invoked for - and declares no output carrier. Every
		// other fold still owes one, so the exception is the marker's and not
		// an empty list's.
		if (len(reducer.Outputs) == 0) != reducer.Structural {
			return false
		}
	}
	return true
}

// Clone returns an independent contribution.
func (contribution Contribution) Clone() Contribution {
	clone := contribution
	clone.Carriers = append([]Carrier(nil), contribution.Carriers...)
	clone.Projections = append([]Projection(nil), contribution.Projections...)
	clone.Relations = make([]Relation, len(contribution.Relations))
	for index, relation := range contribution.Relations {
		clone.Relations[index] = relation
		clone.Relations[index].Inputs = append([]RelationInput(nil), relation.Inputs...)
		clone.Relations[index].Derivation.StaticAxes = append([]schema.EntryReference(nil), relation.Derivation.StaticAxes...)
	}
	clone.Reducers = make([]Reducer, len(contribution.Reducers))
	for index, reducer := range contribution.Reducers {
		clone.Reducers[index] = reducer
		clone.Reducers[index].Inputs = append([]ReducerInput(nil), reducer.Inputs...)
		clone.Reducers[index].Outputs = append([]ReducerOutput(nil), reducer.Outputs...)
		clone.Reducers[index].Implementation = cloneSymbol(reducer.Implementation)
		clone.Reducers[index].Derivation.Build = cloneSymbol(reducer.Derivation.Build)
		clone.Reducers[index].Derivation.StaticAxes = append([]schema.EntryReference(nil), reducer.Derivation.StaticAxes...)
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
	relations := make(map[string]Relation, len(base.Relations))
	for _, relation := range base.Relations {
		relations[relation.Name] = relation
	}
	projections := make(map[string]Projection, len(base.Projections))
	for _, projection := range base.Projections {
		projections[projection.Name] = projection
	}
	rules := make(map[schema.Key]struct{}, len(source.Contributions))
	reducerNames := make(map[string]struct{}, len(source.Contributions))
	reducerKeys := make(map[schema.Key]struct{}, len(source.Contributions))
	composed := make([]Reducer, 0, len(source.Contributions))
	for _, contribution := range source.Contributions {
		if !contribution.rowsAvailable() || contribution.Axis != base.Axis {
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
		for _, relation := range contribution.Relations {
			existing, declared := relations[relation.Name]
			if declared {
				if !relationsAgree(existing, relation) {
					return Definition{}, false
				}
				continue
			}
			row := relation
			row.Inputs = append([]RelationInput(nil), relation.Inputs...)
			row.Derivation.StaticAxes = append([]schema.EntryReference(nil), relation.Derivation.StaticAxes...)
			relations[relation.Name] = row
			base.Relations = append(base.Relations, row)
		}
		for _, projection := range contribution.Projections {
			existing, declared := projections[projection.Name]
			if declared {
				if existing != projection {
					return Definition{}, false
				}
				continue
			}
			projections[projection.Name] = projection
			base.Projections = append(base.Projections, projection)
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
			row.Derivation.StaticAxes = append([]schema.EntryReference(nil), reducer.Derivation.StaticAxes...)
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

// NewRoster admits an ordered set of axis sources and folds every declared row
// into the source of the axis that row names. Two sources naming one axis or
// one generator name are two owners of one vocabulary and are refused.
//
// A relation over call coordinates is call-axis data whichever rule declares
// it, so the axis is a property of the row and not of the contribution that
// authored it. A rule that reads a foreign axis states its join rows on that
// axis - where the key projection's accessor is a method the owner actually
// has - and the roster places them there. The alternative was for the reading
// domain's candidate to carry the foreign coordinate, which is a schema-level
// dependency between two domains that have none.
//
// The fold happens once, here, so composition stays a per-source operation and
// there is exactly one statement of where a row lives.
func NewRoster(sources ...Source) (Roster, bool) {
	names := make(map[string]struct{}, len(sources))
	axes := make(map[schema.Key]int, len(sources))
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
		for _, contribution := range source.Contributions {
			if !contribution.Available() || contribution.Axis != source.Base.Axis {
				return Roster{}, false
			}
		}
		names[source.Name] = struct{}{}
		axes[source.Base.Axis] = len(admitted)
		admitted = append(admitted, source)
	}
	if len(admitted) == 0 {
		return Roster{}, false
	}
	folded, foldOK := foldForeignRows(admitted, axes)
	if !foldOK {
		return Roster{}, false
	}
	return Roster{sources: folded}, true
}

// foldForeignRows moves every relation and projection to the source of the
// axis it names, carrying the carrier rows it is typed in. A row naming an axis
// no source registers has no home and refuses the roster where it is written
// rather than where a plan later fails to resolve it.
func foldForeignRows(sources []Source, axes map[schema.Key]int) ([]Source, bool) {
	retained := make([][]Contribution, len(sources))
	received := make([][]Contribution, len(sources))
	for index, source := range sources {
		retained[index] = make([]Contribution, 0, len(source.Contributions))
		for _, contribution := range source.Contributions {
			home := contribution
			home.Relations = nil
			home.Projections = nil
			foreign := make(map[schema.Key]Contribution, 1)
			order := make([]schema.Key, 0, 1)
			for _, relation := range contribution.Relations {
				axis := relation.Axis
				if !axis.Available() {
					axis = contribution.Axis
				}
				if axis == contribution.Axis {
					home.Relations = append(home.Relations, relation)
					continue
				}
				target, targetOK := axes[axis]
				if !targetOK || target == index {
					return nil, false
				}
				row, seen := foreign[axis]
				if !seen {
					row = Contribution{Axis: axis, Rule: contribution.Rule}
					order = append(order, axis)
				}
				row.Relations = append(row.Relations, relation)
				names := append([]string(nil), relation.Subject)
				for _, input := range relation.Inputs {
					names = append(names, input.Carrier)
				}
				carriers, carriersOK := namedCarriers(contribution.Carriers, row.Carriers, names)
				if !carriersOK {
					return nil, false
				}
				row.Carriers = carriers
				foreign[axis] = row
			}
			for _, projection := range contribution.Projections {
				axis := projection.Axis
				if !axis.Available() {
					axis = contribution.Axis
				}
				if axis == contribution.Axis {
					home.Projections = append(home.Projections, projection)
					continue
				}
				target, targetOK := axes[axis]
				if !targetOK || target == index {
					return nil, false
				}
				row, seen := foreign[axis]
				if !seen {
					row = Contribution{Axis: axis, Rule: contribution.Rule}
					order = append(order, axis)
				}
				row.Projections = append(row.Projections, projection)
				carriers, carriersOK := namedCarriers(contribution.Carriers, row.Carriers, []string{projection.Result})
				if !carriersOK {
					return nil, false
				}
				row.Carriers = carriers
				foreign[axis] = row
			}
			retained[index] = append(retained[index], home)
			for _, axis := range order {
				target := axes[axis]
				received[target] = append(received[target], foreign[axis])
			}
		}
	}
	folded := make([]Source, len(sources))
	for index, source := range sources {
		folded[index] = source
		folded[index].Contributions = append(retained[index], received[index]...)
	}
	return folded, true
}

// namedCarriers appends the declared carriers a routed row is typed in, taking
// them from the contribution that authored the row. A name the authoring
// contribution does not declare is the receiving axis's own, and composition
// refuses it there if it is neither.
func namedCarriers(declared, carried []Carrier, names []string) ([]Carrier, bool) {
	for _, name := range names {
		if !identifierAvailable(name) {
			return nil, false
		}
		held := false
		for _, carrier := range carried {
			if carrier.Name == name {
				held = true
				break
			}
		}
		if held {
			continue
		}
		for _, carrier := range declared {
			if carrier.Name == name {
				carried = append(carried, carrier)
				break
			}
		}
	}
	return carried, true
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
