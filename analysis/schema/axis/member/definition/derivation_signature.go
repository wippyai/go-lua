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
// are set only where the declaration says the position is many-valued: an
// input delivered as the ordered cells of a selected join is a slice of the
// execution cell view instantiated at that input's own carrier.
type DerivedParam struct {
	Type    GoType
	Element GoType
	Slice   bool
}

// axisSchemaType is the Go type one axis's own schema is spelled as: the
// receiver its declared key normalizer is a method on. An axis states that
// type once, where it says how a key becomes a dense coordinate, so a
// derivation naming the axis names the same type its owner does.
func axisSchemaType(source Definition) (GoType, bool) {
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
func (roster Roster) DerivationSignature(axis schema.Key, relation Relation, cell GoType) (DerivationShape, bool) {
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
		schemaType, schemaTypeOK := axisSchemaType(staticSource)
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
		// A many-valued input is the ordered cells of a selected join. The cell
		// view is the execution layer's, named by the caller for the same
		// reason the reducer's is: this package states the shape without
		// naming that package.
		if !cell.Available() {
			return DerivationShape{}, false
		}
		params = append(params, DerivedParam{Type: cell, Element: carrier.Type, Slice: true})
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
