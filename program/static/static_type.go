package static

import "github.com/wippyai/go-lua/program/keyspace"

// StaticTypes is the post-commit hot view over Static's complete authored type
// forest. It retains only the published Component pointer; construction Views
// deliberately do not carry that pointer and therefore expose an expired view.
type StaticTypes struct{ component *Component }

// StaticTypeRef is an owner-bound capability for one published Static type.
// The owner and Term remain private so callers can transport only the checked
// capability and recover its local Term. Term itself is deliberately a local
// (family, ordinal) encoding: it carries no Component provenance, so passing
// the result to another StaticTypes.Ref may bind the same encoding there.
type StaticTypeRef struct {
	component *Component
	term      keyspace.Term
}

// StaticTypes returns the post-commit Static type capability. A claimed
// construction View has no Component pointer and therefore returns a zero
// capability that cannot leak an enduring reference.
func (view View) StaticTypes() StaticTypes { return StaticTypes{component: view.component} }

// Count returns the complete canonical Static type forest cardinality.
func (types StaticTypes) Count() int {
	component := types.component
	if component == nil || !component.contentID.Available() {
		return 0
	}
	return component.StaticTypeTermCount()
}

// At returns one owner-bound capability in the Component's existing canonical
// family order. No second Term list or identity is materialized.
func (types StaticTypes) At(index int) (StaticTypeRef, bool) {
	component := types.component
	if component == nil || !component.contentID.Available() {
		return StaticTypeRef{}, false
	}
	term, ok := component.StaticTypeTermAt(index)
	if !ok || !component.StaticTypeTerm(term) {
		return StaticTypeRef{}, false
	}
	return StaticTypeRef{component: component, term: term}, true
}

// Ref validates and binds one raw Component-local static type Term. Terms are
// local (family, ordinal) encodings rather than owner-provenanced identities:
// the same encoding from another Component may be rebound here. Nil,
// wrong-family, malformed, and out-of-range Terms fail closed.
func (types StaticTypes) Ref(term keyspace.Term) (StaticTypeRef, bool) {
	component := types.component
	if component == nil || !component.StaticTypeTerm(term) {
		return StaticTypeRef{}, false
	}
	return StaticTypeRef{component: component, term: term}, true
}

// Owns authenticates a StaticTypeRef against this exact published Static
// component. It does not rebind the ref's local Term into another owner.
func (types StaticTypes) Owns(ref StaticTypeRef) bool {
	return types.component != nil && ref.component == types.component && types.component.StaticTypeTerm(ref.term)
}

// Term recovers the checked local Term and discards the owner binding. A zero
// ref or a ref whose Component is no longer available returns the zero Term;
// callers that pass the result to another StaticTypes view are requesting a
// fresh local binding there.
func (ref StaticTypeRef) Term() keyspace.Term {
	if ref.component == nil || !ref.component.StaticTypeTerm(ref.term) {
		return 0
	}
	return ref.term
}

const staticTypeFamilyCount = 19

// staticTypeFamilies is the complete authored static-type authority in its
// stable family order. It includes the declaration-owned type roots (aliases,
// interfaces, and type parameters) followed by the existing typed expression
// relations. DeclaredType bindings, fields, annotations, and runtime operands
// are intentionally not static type roots.
//
// Keep this order stable: it is a derived query order and is used to make
// source-to-term enumeration deterministic. It is not part of Static
// ContentID and does not create another semantic identity.
var staticTypeFamilies = [staticTypeFamilyCount]keyspace.Family{
	keyspace.FamilyTypeAlias,
	keyspace.FamilyTypeInterface,
	keyspace.FamilyTypeParam,
	keyspace.FamilyTypePrimitive,
	keyspace.FamilyTypeLiteral,
	keyspace.FamilyTypeOptional,
	keyspace.FamilyTypeUnion,
	keyspace.FamilyTypeIntersection,
	keyspace.FamilyTypeRef,
	keyspace.FamilyTypeGeneric,
	keyspace.FamilyTypeArray,
	keyspace.FamilyTypeMap,
	keyspace.FamilyTypeRecord,
	keyspace.FamilyTypeFunction,
	keyspace.FamilyTypeAsserts,
	keyspace.FamilyTypeOf,
	keyspace.FamilyTypeKeyOf,
	keyspace.FamilyTypeIndexAccess,
	keyspace.FamilyTypeConditional,
}

// staticTypeIndex is the complete sealed cardinality authority. prefix[i]
// starts family i and prefix[i+1] ends it. Fixed-size metadata makes the
// derived order queryable without retaining one duplicate Term per row.
type staticTypeIndex struct {
	prefix [staticTypeFamilyCount + 1]uint64
}

func (index staticTypeIndex) total() uint64 { return index.prefix[staticTypeFamilyCount] }

func (index staticTypeIndex) familyCount(position int) uint64 {
	if position < 0 || position >= staticTypeFamilyCount {
		return 0
	}
	return index.prefix[position+1] - index.prefix[position]
}

// staticTypeCount returns the authored cardinality for one static-type
// family.  The switch is closed deliberately: adding a new static family
// requires an explicit owner decision and cannot silently enter this
// authority.
func staticTypeCount(component *Component, family keyspace.Family) int {
	if component == nil {
		return 0
	}
	switch family {
	case keyspace.FamilyTypeAlias:
		return len(component.declarations.aliases)
	case keyspace.FamilyTypeInterface:
		return len(component.declarations.interfaces)
	case keyspace.FamilyTypeParam:
		return len(component.declarations.params)
	default:
		return staticNodeCount(component, family)
	}
}

// buildStaticTypeIndex seals the complete finite cardinality from the typed
// rows into fixed prefix metadata. It allocates nothing and retains no copied
// Terms; Count, At, and membership all consult this one authority.
func buildStaticTypeIndex(component *Component) staticTypeIndex {
	var index staticTypeIndex
	if component == nil {
		return index
	}
	for position, family := range staticTypeFamilies {
		index.prefix[position+1] = index.prefix[position] + uint64(staticTypeCount(component, family))
	}
	return index
}

// ContentID returns the sealed authored Static identity. A zero Component or
// an unsealed component fails closed with an unavailable identity.
func (component *Component) ContentID() keyspace.ContentID {
	if component == nil {
		return keyspace.ContentID{}
	}
	return component.contentID
}

// ContentID returns the authored Static identity through a lifecycle-bound
// construction View. A claimed View exposes the draft's identity; once its
// Finalizer commits or aborts (including an invalid terminal receipt), the
// same copied View returns an unavailable identity. A published Component
// View remains identity-bearing because it has no construction state.
func (view View) ContentID() keyspace.ContentID {
	component := view.componentOf()
	if component == nil {
		return keyspace.ContentID{}
	}
	return component.contentID
}

// StaticTypeTerm reports whether term is one of the authored static type
// roots. It is a family/ordinal membership check over the sealed typed rows;
// it does not scan or materialize the enumeration.
func (component *Component) StaticTypeTerm(term keyspace.Term) bool {
	if component == nil || !component.contentID.Available() {
		return false
	}
	family := keyspace.TermFamily(term)
	for position, candidate := range staticTypeFamilies {
		if family == candidate {
			ordinal := keyspace.TermOrdinal(term)
			return ordinal != 0 && uint64(ordinal) <= component.staticTypes.familyCount(position)
		}
	}
	return false
}

// StaticTypeTermCount returns the complete finite authored static-type
// authority. The fixed prefix metadata was sealed once by Build; this query
// is O(1).
func (component *Component) StaticTypeTermCount() int {
	if component == nil {
		return 0
	}
	return int(component.staticTypes.total())
}

// StaticTypeTermAt returns the stable derived order of the authored static
// type forest. It returns no bare term outside the sealed enumeration.
func (component *Component) StaticTypeTermAt(index int) (keyspace.Term, bool) {
	if component == nil || index < 0 || uint64(index) >= component.staticTypes.total() {
		return 0, false
	}
	offset := uint64(index)
	for position, family := range staticTypeFamilies {
		if offset < component.staticTypes.prefix[position+1] {
			ordinal := offset - component.staticTypes.prefix[position] + 1
			return keyspace.MakeTerm(family, uint32(ordinal)), true
		}
	}
	return 0, false
}
