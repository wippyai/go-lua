package definition

import "github.com/wippyai/go-lua/analysis/schema"

// DerivationShape is the direct-call shape one authored relation derivation
// must have. It is derived from the declaration and from nothing else, which
// is fence one of the authored-Build ruling: a Build is admitted as domain
// logic behind a SEALED contract, and a contract nobody derives is a contract.
//
// Build answers the derivation's State from the schemas of its ordered static
// axes followed by the relation's declared inputs. Count and At consume that
// State to expose the relation's Subject rows in canonical order. Nothing else
// is a parameter: which candidate a row hangs off, which projection addresses
// it, and which coordinates it resolves to are the sealed knowledge of the
// family that calls this, exactly as they are for a reducer.
type DerivationShape struct {
	BuildParams  []DerivedParam
	BuildResults []DerivedParam
	CountParams  []DerivedParam
	CountResults []DerivedParam
	AtParams     []DerivedParam
	AtResults    []DerivedParam
}

// DerivedParam is one position of a derived derivation call. Element and Slice
// are set only where the declaration says the position is many-valued: such an
// input is an execution view instantiated at that input's own carrier, and a
// selection delivers a slice of tagged cells while a whole-vector read
// delivers one vector.
type DerivedParam struct {
	Type    GoType
	Element GoType
	Slice   bool
}

// AxisSchemaType is the Go type one axis's own schema is spelled as: the
// receiver its declared key normalizer is a method on. An axis states that
// type once, where it says how a key becomes a dense coordinate, so a
// derivation naming the axis - and a family emitted for a rule that reaches
// it - names the same type its owner does.
func AxisSchemaType(source Definition) (GoType, bool) {
	receiver := source.Binding.Key.Normalizer.Receiver
	if !receiver.Available() {
		return GoType{}, false
	}
	if source.Binding.Key.Normalizer.ReceiverPointer {
		receiver.Pointer = true
	}
	return receiver, true
}

// DerivationSignature derives the call shape of one relation's authored Build,
// Count and At. The relation is resolved in the axis it belongs to, and each
// static axis is resolved through the roster, because a derivation reaching
// another axis's schema is naming that axis's own published type rather than
// one its consumer chose.
func (roster Roster) DerivationSignature(axis schema.Key, relation Relation, cell, vector GoType) (DerivationShape, bool) {
	if !relation.Derivation.complete() {
		return DerivationShape{}, false
	}
	owner, ownerOK := roster.definitionForAxis(axis)
	if !ownerOK {
		return DerivationShape{}, false
	}
	carriers, _, carriersOK := owner.carrierIndex()
	if !carriersOK {
		return DerivationShape{}, false
	}
	subject, subjectOK := carriers[relation.Subject]
	if !subjectOK {
		return DerivationShape{}, false
	}
	params := make([]DerivedParam, 0, len(relation.Derivation.StaticAxes)+len(relation.Inputs))
	for _, static := range relation.Derivation.StaticAxes {
		staticSource, staticOK := roster.definitionForAxis(static.Key)
		if !staticOK {
			return DerivationShape{}, false
		}
		schemaType, schemaTypeOK := AxisSchemaType(staticSource)
		if !schemaTypeOK {
			return DerivationShape{}, false
		}
		params = append(params, DerivedParam{Type: schemaType})
	}
	for _, input := range relation.Inputs {
		carrier, carrierOK := carriers[input.Carrier]
		if !carrierOK {
			return DerivationShape{}, false
		}
		if !input.Many {
			params = append(params, DerivedParam{Type: carrier.Type})
			continue
		}
		// A many-valued input arrives as the whole delivery its own read
		// establishes, and which view that is comes from ManyValuedView - the
		// same answer the fold over this input gets, so a derivation and the
		// reducer beside it can never be handed different views of one read.
		view, slice, viewOK := ManyValuedView(input.Form, cell, vector)
		if !viewOK {
			return DerivationShape{}, false
		}
		params = append(params, DerivedParam{Type: view, Element: carrier.Type, Slice: slice})
	}
	state := relation.Derivation.State
	return DerivationShape{
		BuildParams:  params,
		BuildResults: []DerivedParam{{Type: state}, {Type: GoType{Name: "bool"}}},
		CountParams:  []DerivedParam{{Type: state}},
		CountResults: []DerivedParam{{Type: GoType{Name: "int"}}},
		AtParams:     []DerivedParam{{Type: state}, {Type: GoType{Name: "int"}}},
		AtResults:    []DerivedParam{{Type: subject.Type}, {Type: GoType{Name: "bool"}}},
	}, true
}

// definitionForAxis composes the one source that owns an axis. Two sources
// naming one axis are refused when the roster is admitted, so this resolves at
// most one.
func (roster Roster) definitionForAxis(axis schema.Key) (Definition, bool) {
	for _, source := range roster.sources {
		composed, composedOK := source.Compose()
		if !composedOK {
			return Definition{}, false
		}
		if composed.Axis == axis {
			return composed, true
		}
	}
	return Definition{}, false
}

// ReducerDerivationSignature derives the call shape of one reducer's authored
// state Build. It is the relation derivation's shape narrowed to what a fold
// needs: the schemas of the ordered static axes in, the sealed state and its
// validity out.
//
// Each static axis is resolved through the roster for the same reason a
// relation's is - a fold reaching another axis's schema is naming that axis's
// own published type rather than one its consumer chose.
func (roster Roster) ReducerDerivationSignature(derivation ReducerDerivation) ([]DerivedParam, []DerivedParam, bool) {
	if !derivation.complete() {
		return nil, nil, false
	}
	params := make([]DerivedParam, 0, len(derivation.StaticAxes))
	for _, static := range derivation.StaticAxes {
		staticSource, staticOK := roster.definitionForAxis(static.Key)
		if !staticOK {
			return nil, nil, false
		}
		schemaType, schemaTypeOK := AxisSchemaType(staticSource)
		if !schemaTypeOK {
			return nil, nil, false
		}
		params = append(params, DerivedParam{Type: schemaType})
	}
	return params, []DerivedParam{{Type: derivation.State}, {Type: GoType{Name: "bool"}}}, true
}

// DeclaredDerivationShape is the direct-call shape one DECLARED derivation's
// authored parts must have. Everything else about the construction is
// generated, so these three are the whole of what an owner still writes.
type DeclaredDerivationShape struct {
	// CountParams/CountResults and AtParams/AtResults belong to the source
	// enumerations, in the order the derivation composes them.
	Sources []EnumerationShape
	// ResolveParams is the static axis schemas, then the candidate carrier,
	// then the innermost item. ResolveResults is the row, whether the item
	// contributes one at all, and the owner's validity.
	ResolveParams  []DerivedParam
	ResolveResults []DerivedParam
	// WidenParams/WidenResults are the endpoint's, and are empty when the
	// derivation declares no endpoint. The endpoint is a judgment over what
	// Resolve is a judgment over, minus the item: static axis schemas, the
	// candidate, and the value the outer source reads. It answers twice -
	// whether the set is beyond enumeration, and its own validity - because it
	// is the only judgment of a declared derivation that runs whether or not
	// the source yields anything.
	WidenParams  []DerivedParam
	WidenResults []DerivedParam
	// Widened is the shape of the sources the widened answer is read out of.
	Widened []EnumerationShape
	// WidenResolveParams is the widen chain's own judgment, present exactly
	// when the endpoint states one. It is the derivation's judgment with the
	// innermost item taken from the widen chain instead of the source chain.
	WidenResolveParams  []DerivedParam
	WidenResolveResults []DerivedParam
}

// EnumerationShape is one declared enumeration's own call shape.
type EnumerationShape struct {
	CountParams  []DerivedParam
	CountResults []DerivedParam
	AtParams     []DerivedParam
	AtResults    []DerivedParam
}

// DeclaredDerivationSignature derives the call shape of one relation's
// declared derivation.
//
// The composition reads outermost-first: the first source is enumerated over
// the relation's own declared input, and each further source is enumerated
// over the item the one before it yields. That is what makes a two-level
// derivation - members, then what each member projects to - flat: it is two
// declared enumerations, not an authored nested step.
//
// Resolve is handed the innermost item and answers one row, whether that item
// contributes a row at all, and its own validity. The middle result is what
// absorbs filtering: an item the judgment declines is absent rather than
// refused, so a derivation needs no separate predicate to skip one.
func (roster Roster) DeclaredDerivationSignature(axis schema.Key, relation Relation, candidate GoType) (DeclaredDerivationShape, bool) {
	derivation := relation.Derivation
	if !derivation.DeclaredDerivation() || !derivation.declaredComplete() {
		return DeclaredDerivationShape{}, false
	}
	owner, ownerOK := roster.definitionForAxis(axis)
	if !ownerOK {
		return DeclaredDerivationShape{}, false
	}
	carriers, _, carriersOK := owner.carrierIndex()
	if !carriersOK {
		return DeclaredDerivationShape{}, false
	}
	subject, subjectOK := carriers[relation.Subject]
	if !subjectOK {
		return DeclaredDerivationShape{}, false
	}
	shape := DeclaredDerivationShape{}
	sources, item, sourcesOK := roster.enumerationChain(derivation.Source, subject.Type)
	if !sourcesOK {
		return DeclaredDerivationShape{}, false
	}
	shape.Sources = sources
	params := make([]DerivedParam, 0, len(derivation.StaticAxes)+2)
	for _, static := range derivation.StaticAxes {
		staticSource, staticOK := roster.definitionForAxis(static.Key)
		if !staticOK {
			return DeclaredDerivationShape{}, false
		}
		schemaType, schemaTypeOK := AxisSchemaType(staticSource)
		if !schemaTypeOK {
			return DeclaredDerivationShape{}, false
		}
		params = append(params, DerivedParam{Type: schemaType})
	}
	if candidate.Available() {
		params = append(params, DerivedParam{Type: candidate})
	}
	shape.ResolveParams = append(params, DerivedParam{Type: item})
	shape.ResolveResults = []DerivedParam{
		{Type: subject.Type}, {Type: GoType{Name: "bool"}}, {Type: GoType{Name: "bool"}},
	}
	if derivation.Widen.Declared() {
		first, firstOK := roster.enumerationOver(derivation.Source[0], owner)
		if !firstOK {
			return DeclaredDerivationShape{}, false
		}
		shape.WidenParams = append(append([]DerivedParam{}, params...), DerivedParam{Type: first})
		shape.WidenResults = []DerivedParam{{Type: GoType{Name: "bool"}}, {Type: GoType{Name: "bool"}}}
		// The widened answer is read out of the owner's own directory, so its
		// sources chain from the axis schema rather than from the fact.
		widened, widenedItem, widenedOK := roster.enumerationChain(derivation.Widen.Source, subject.Type)
		if !widenedOK {
			return DeclaredDerivationShape{}, false
		}
		shape.Widened = widened
		// One judgment answers both chains only when both chains yield the
		// same item. Where they do not, the endpoint owes its own, and a
		// second one where they agree would be two answers to what a member is.
		sameItem := sameType(widenedItem, item)
		if sameItem == derivation.Widen.Resolve.Available() {
			return DeclaredDerivationShape{}, false
		}
		if !sameItem {
			shape.WidenResolveParams = append(append([]DerivedParam{}, params...), DerivedParam{Type: widenedItem})
			shape.WidenResolveResults = shape.ResolveResults
		}
	}
	return shape, true
}

// enumerationOver answers the type one declared enumeration reads its sequence
// out of: a carrier of its axis, or that axis's own schema when the
// enumeration is the owner's directory. The widen endpoint is asked of the
// same value the outer source reads, because what is beyond enumeration is a
// property of the thing being enumerated.
func (roster Roster) enumerationOver(reference EnumerationRef, consumer Definition) (GoType, bool) {
	owner, ownerOK := roster.definitionForAxis(reference.Axis.Key)
	if !ownerOK {
		return GoType{}, false
	}
	enumeration, enumerationOK := findEnumeration(owner, reference.Name)
	if !enumerationOK {
		return GoType{}, false
	}
	if enumeration.OverSchema() {
		return AxisSchemaType(owner)
	}
	carriers, _, carriersOK := owner.carrierIndex()
	if !carriersOK {
		return GoType{}, false
	}
	over, overOK := carriers[enumeration.Over]
	_ = consumer
	return over.Type, overOK
}

// enumerationChain resolves one composed source list into its per-level call
// shapes, holding each level to reading what the level before it yielded. The
// last level's item is what a resolve is finally handed.
func (roster Roster) enumerationChain(sources []EnumerationRef, subject GoType) ([]EnumerationShape, GoType, bool) {
	shapes := make([]EnumerationShape, 0, len(sources))
	var item GoType
	if len(sources) == 0 {
		return nil, GoType{}, false
	}
	for _, source := range sources {
		owner, ownerOK := roster.definitionForAxis(source.Axis.Key)
		if !ownerOK {
			return nil, GoType{}, false
		}
		enumeration, enumerationOK := findEnumeration(owner, source.Name)
		if !enumerationOK {
			return nil, GoType{}, false
		}
		carriers, _, carriersOK := owner.carrierIndex()
		if !carriersOK {
			return nil, GoType{}, false
		}
		over, overOK := roster.enumerationOver(source, owner)
		element, elementOK := carriers[enumeration.Item]
		if !overOK || !elementOK {
			return nil, GoType{}, false
		}
		if item.Available() && !sameType(item, over) {
			return nil, GoType{}, false
		}
		shapes = append(shapes, EnumerationShape{
			CountParams:  []DerivedParam{{Type: over}},
			CountResults: []DerivedParam{{Type: GoType{Name: "int"}}},
			AtParams:     []DerivedParam{{Type: over}, {Type: GoType{Name: "int"}}},
			AtResults:    []DerivedParam{{Type: element.Type}, {Type: GoType{Name: "bool"}}},
		})
		item = element.Type
	}
	_ = subject
	return shapes, item, true
}

func findEnumeration(source Definition, name string) (Enumeration, bool) {
	for _, enumeration := range source.Enumerations {
		if enumeration.Name == name {
			return enumeration, true
		}
	}
	return Enumeration{}, false
}
