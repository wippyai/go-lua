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
	// Carriers are axis-local carrier authorities this contribution declares.
	// A carrier the base already declares may be repeated verbatim; a repeat
	// that disagrees is two authority declarations of one name and is refused.
	Carriers []Carrier
	// CarrierRefs are imported authorities used by this contribution's rows.
	// They stay imports if rows move to another axis; moving a row never turns
	// one into a local authority.
	CarrierRefs []CarrierReference
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
	// Selections are the operations that publish this rule's produced rows.
	// A rule whose read is selected rather than enumerated declares the
	// operation that computes those rows beside the relation they land in,
	// for the same reason its reducer is declared beside its Program.
	Selections []Selection
	// Reducers are this rule's reducer definitions, in declaration order.
	Reducers []Reducer
}

// relationsAgree reports whether two declarations of one relation name state
// the same relation. Relation carries slices, so identity is stated here
// rather than left to comparison.
func relationsAgree(left, right Relation) bool {
	if left.Name != right.Name || left.Key != right.Key || left.Subject != right.Subject || left.Axis != right.Axis ||
		left.Addressing != right.Addressing ||
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
	declarations := newCarrierDeclarations(axisCarrierOwner(contribution.Axis), len(contribution.Carriers)+len(contribution.CarrierRefs))
	for _, carrier := range contribution.Carriers {
		if _, ok := declarations.addAuthority(carrier); !ok {
			return false
		}
	}
	for _, reference := range contribution.CarrierRefs {
		if _, ok := declarations.addReference(reference); !ok {
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
	for _, selection := range contribution.Selections {
		if !identifierAvailable(selection.Name) || !selection.Key.Available() ||
			!identifierAvailable(selection.Relation) || !identifierAvailable(selection.Tag) ||
			!selection.Implementation.Available() {
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
	clone.CarrierRefs = append([]CarrierReference(nil), contribution.CarrierRefs...)
	clone.Projections = append([]Projection(nil), contribution.Projections...)
	clone.Selections = append([]Selection(nil), contribution.Selections...)
	clone.Relations = make([]Relation, len(contribution.Relations))
	for index, relation := range contribution.Relations {
		clone.Relations[index] = relation
		clone.Relations[index].Inputs = append([]RelationInput(nil), relation.Inputs...)
		clone.Relations[index].Correspondences = cloneCorrespondences(relation.Correspondences)
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
	// The base itself cannot contain a duplicate declaration, even a verbatim
	// one. Contributions may repeat a base row so independently authored rules
	// do not depend on registration order; an axis base is one owner statement.
	if _, _, carriersOK := base.carrierIndex(); !carriersOK {
		return Definition{}, false
	}
	carriers := newCarrierDeclarations(axisCarrierOwner(base.Axis), len(base.Carriers)+len(base.CarrierRefs))
	for _, carrier := range base.Carriers {
		if added, ok := carriers.addAuthority(carrier); !ok || !added {
			return Definition{}, false
		}
	}
	for _, reference := range base.CarrierRefs {
		if added, ok := carriers.addReference(reference); !ok || !added {
			return Definition{}, false
		}
	}
	relations := make(map[string]Relation, len(base.Relations))
	for _, relation := range base.Relations {
		relations[relation.Name] = relation
	}
	selections := make(map[string]Selection, len(base.Selections))
	for _, selection := range base.Selections {
		selections[selection.Name] = selection
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
			added, carriersOK := carriers.addAuthority(carrier)
			if !carriersOK {
				return Definition{}, false
			}
			if added {
				base.Carriers = append(base.Carriers, carrier)
			}
		}
		for _, reference := range contribution.CarrierRefs {
			added, carriersOK := carriers.addReference(reference)
			if !carriersOK {
				return Definition{}, false
			}
			if added {
				base.CarrierRefs = append(base.CarrierRefs, reference)
			}
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
		for _, selection := range contribution.Selections {
			existing, declared := selections[selection.Name]
			if declared {
				if existing != selection {
					return Definition{}, false
				}
				continue
			}
			selections[selection.Name] = selection
			base.Selections = append(base.Selections, selection)
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
	// The composed rows are returned even when they are not complete, so a
	// caller can ask them why. Admission is still the bool: an incomplete
	// definition is refused by every caller that checks it, and CompleteRefusal
	// reads the same rows rather than admitting any of them.
	return base, base.Complete()
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

// sourceCarrierDeclarations preindexes every authority and import a source
// can contribute before rows are folded. The table is complete rather than
// sequential: a carrier introduced by one contribution can be referenced by a
// row introduced by another without turning either contribution into a second
// authority owner.
func sourceCarrierDeclarations(source Source) (carrierDeclarations, bool) {
	if _, _, baseOK := source.Base.carrierIndex(); !baseOK {
		return carrierDeclarations{}, false
	}
	declarations := newCarrierDeclarations(axisCarrierOwner(source.Base.Axis), len(source.Base.Carriers)+len(source.Base.CarrierRefs))
	for _, carrier := range source.Base.Carriers {
		if added, ok := declarations.addAuthority(carrier); !ok || !added {
			return carrierDeclarations{}, false
		}
	}
	for _, reference := range source.Base.CarrierRefs {
		if added, ok := declarations.addReference(reference); !ok || !added {
			return carrierDeclarations{}, false
		}
	}
	for _, contribution := range source.Contributions {
		if contribution.Axis != source.Base.Axis || !contribution.rowsAvailable() {
			return carrierDeclarations{}, false
		}
		for _, carrier := range contribution.Carriers {
			if _, ok := declarations.addAuthority(carrier); !ok {
				return carrierDeclarations{}, false
			}
		}
		for _, reference := range contribution.CarrierRefs {
			if _, ok := declarations.addReference(reference); !ok {
				return carrierDeclarations{}, false
			}
		}
	}
	return declarations, true
}

// appendForeignCarrierReference adds a binding to one received contribution.
// The target never receives a Carrier here: a moved row may import an
// authority, but it cannot recreate the authority in a different axis.
func appendForeignCarrierReference(contribution *Contribution, reference CarrierReference) bool {
	for _, held := range contribution.CarrierRefs {
		if held.Name == reference.Name {
			return held == reference
		}
		if held.Key == reference.Key {
			return false
		}
	}
	contribution.CarrierRefs = append(contribution.CarrierRefs, reference)
	return true
}

// targetAuthorityName finds the target's local source alias for one authority
// it owns. A moved row can then use that existing authority rather than
// importing it back into the owner under the alias it had in its source axis.
func targetAuthorityName(target carrierDeclarations, source carrierUse) (string, bool) {
	for name, candidate := range target.byName {
		if candidate.Local && candidate.Ref == source.Ref && candidate.Type == source.Type {
			return name, true
		}
	}
	return "", false
}

// carryForeignCarrierNames preserves the semantic owner of every source name
// a moved row uses. A local authority becomes an import to its original axis;
// an existing import remains exactly that import. If the receiving axis owns
// the referenced authority already, it resolves its own local declaration and
// returns that declaration's source alias. A name that resolves differently,
// or nowhere, is refused rather than guessed from Go type or raw key text.
func carryForeignCarrierNames(contribution *Contribution, names []string, source, target carrierDeclarations) (map[string]string, bool) {
	resolved := make(map[string]string, len(names))
	for _, name := range names {
		if !identifierAvailable(name) {
			return nil, false
		}
		sourceCarrier, sourceOK := source.byName[name]
		targetCarrier, targetOK := target.byName[name]
		if !sourceOK {
			// A row can name an authority its target base already owns. This is
			// the target-owner case for a name the contributing source never
			// declared; no carrier row crosses the axis boundary.
			if !targetOK {
				return nil, false
			}
			resolved[name] = name
			continue
		}
		if targetOK {
			if targetCarrier.Ref != sourceCarrier.Ref || targetCarrier.Type != sourceCarrier.Type {
				return nil, false
			}
			resolved[name] = name
			continue
		}
		// The target is the referenced owner, but it has no source alias for
		// its authority. Resolve the target's local alias rather than importing
		// an authority back into its owner under the source axis's alias.
		if sourceCarrier.Ref.Owner == target.owner {
			targetName, ownerOK := targetAuthorityName(target, sourceCarrier)
			if !ownerOK {
				return nil, false
			}
			resolved[name] = targetName
			continue
		}
		reference := CarrierReference{
			Name: name,
			Key:  sourceCarrier.Key,
			Ref:  sourceCarrier.Ref,
			Type: sourceCarrier.Type,
		}
		if !carrierReferenceAvailable(reference, target.owner) || !appendForeignCarrierReference(contribution, reference) {
			return nil, false
		}
		resolved[name] = name
	}
	return resolved, true
}

func relationCarrierNames(relation Relation) []string {
	names := make([]string, 0, len(relation.Inputs)+2)
	names = append(names, relation.Subject)
	for _, input := range relation.Inputs {
		names = append(names, input.Carrier)
	}
	if relation.MemberOrdinal != "" {
		names = append(names, relation.MemberOrdinal)
	}
	return names
}

// foldForeignRows moves every relation and projection to the source of the
// axis it names, carrying only the imports its source names resolve through.
// A row naming an axis no source registers has no home and refuses the roster
// where it is written rather than where a plan later fails to resolve it.
func foldForeignRows(sources []Source, axes map[schema.Key]int) ([]Source, bool) {
	inventories := make([]carrierDeclarations, len(sources))
	for index, source := range sources {
		inventory, inventoryOK := sourceCarrierDeclarations(source)
		if !inventoryOK {
			return nil, false
		}
		inventories[index] = inventory
	}
	retained := make([][]Contribution, len(sources))
	received := make([][]Contribution, len(sources))
	for index, source := range sources {
		retained[index] = make([]Contribution, 0, len(source.Contributions))
		for _, contribution := range source.Contributions {
			home := contribution.Clone()
			home.Relations = nil
			home.Projections = nil
			home.Selections = nil
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
				aliases, aliasesOK := carryForeignCarrierNames(&row, relationCarrierNames(relation), inventories[index], inventories[target])
				if !aliasesOK {
					return nil, false
				}
				relation.Subject = aliases[relation.Subject]
				for inputIndex := range relation.Inputs {
					relation.Inputs[inputIndex].Carrier = aliases[relation.Inputs[inputIndex].Carrier]
				}
				if relation.MemberOrdinal != "" {
					relation.MemberOrdinal = aliases[relation.MemberOrdinal]
				}
				row.Relations = append(row.Relations, relation)
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
				aliases, aliasesOK := carryForeignCarrierNames(&row, []string{projection.Result}, inventories[index], inventories[target])
				if !aliasesOK {
					return nil, false
				}
				projection.Result = aliases[projection.Result]
				row.Projections = append(row.Projections, projection)
				foreign[axis] = row
			}
			// A selection publishes into one relation and stamps one
			// projection of it, so it belongs to the axis those rows belong
			// to. Moving the relation and leaving the operation behind would
			// leave the operation naming rows its own axis never declares,
			// which is the refusal this fold exists to prevent.
			for _, selection := range contribution.Selections {
				axis := selectionAxis(contribution, selection)
				if axis == contribution.Axis {
					home.Selections = append(home.Selections, selection)
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
				row.Selections = append(row.Selections, selection)
				foreign[axis] = row
			}
			// A selection publishes into one relation and stamps one
			// projection of it, so it belongs to the axis those rows belong
			// to. Moving the relation and leaving the operation behind would
			// leave the operation naming rows its own axis never declares,
			// which is the refusal this fold exists to prevent.
			for _, selection := range contribution.Selections {
				axis := selectionAxis(contribution, selection)
				if axis == contribution.Axis {
					home.Selections = append(home.Selections, selection)
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
				row.Selections = append(row.Selections, selection)
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

// selectionAxis is the axis one selection belongs to: the axis of the
// relation it publishes into. A relation the authoring contribution does not
// declare is one its own axis already carries, so the selection stays home and
// composition refuses it there if that axis declares neither.
func selectionAxis(contribution Contribution, selection Selection) schema.Key {
	for _, relation := range contribution.Relations {
		if relation.Name != selection.Relation {
			continue
		}
		if relation.Axis.Available() {
			return relation.Axis
		}
		return contribution.Axis
	}
	return contribution.Axis
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

// ComposeRefusal names why one source does not compose, for the refusals whose
// symptom is otherwise a bare false. It reports nothing for a source that
// composes.
func (roster Roster) ComposeRefusal(name string) string {
	source, sourceOK := roster.Source(name)
	if !sourceOK {
		return ""
	}
	composed, composedOK := source.Compose()
	if composedOK {
		return ""
	}
	return composed.CompleteRefusal()
}

// packagePathAvailable states the roster's own fence on a source's Go package
// name: it is the identifier a generated file declares, not a path.
func packagePathAvailable(name string) bool { return identifierAvailable(name) }
