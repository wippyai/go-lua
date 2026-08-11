package static

import (
	"github.com/wippyai/go-lua/program/keyspace"
	staticrole "github.com/wippyai/go-lua/program/static/role"
)

// containment is construction-only validation state. It is deliberately not a
// Program representation: every concrete relation remains in its typed owner,
// and this transient index merely proves that their combined containment is a
// forest before a Component becomes immutable.
type containment struct {
	counts [keyspace.FamilyCount]uint32

	// Each dense static family owns one ordinal-indexed parent slice. The
	// slices are construction-only rows; the local proof retains only these
	// immutable relation rows after direct-return validation has completed.
	parents [keyspace.FamilyCount][]keyspace.Term

	// Fields have two distinct typed relations: a field owns its value type
	// through parents, while Record/Interface owns the Field through this
	// separate totality ledger.
	fieldOwners []keyspace.Term

	// Direct assertion returns are evidence for the bound-assertion law only.
	// They are never copied into the retained LocalContainment proof.
	directReturns [keyspace.FamilyCount][]keyspace.Term
}

func newContainment(counts [keyspace.FamilyCount]uint32, fields int) containment {
	check := containment{
		counts:      counts,
		fieldOwners: make([]keyspace.Term, fields),
	}
	// StaticTypeFamilies is the one closed denominator used by Static's
	// typed roots. Allocate by the already-validated family counts rather than
	// growing a map keyed by whichever rows happen to emit an edge.
	for _, family := range staticTypeFamilies {
		if count := counts[family]; count != 0 {
			check.parents[family] = make([]keyspace.Term, int(count))
		}
	}
	if count := counts[keyspace.FamilyTypeAsserts]; count != 0 {
		check.directReturns[keyspace.FamilyTypeAsserts] = make([]keyspace.Term, int(count))
	}
	return check
}

// attach accepts an opaque parent handle but only a concrete authored Static
// child. Consequently Flow can contribute parents without Static inspecting
// Flow geometry, while sharing of static syntax remains impossible.
func (check *containment) attach(parent, child keyspace.Term) bool {
	if parent == 0 || parent == child || !staticrole.Node(check.counts, child) {
		return false
	}
	family, ordinal := keyspace.TermFamily(child), keyspace.TermOrdinal(child)
	parents := check.parents[family]
	if ordinal == 0 || uint64(ordinal) > uint64(len(parents)) || parents[ordinal-1] != 0 {
		return false
	}
	parents[ordinal-1] = parent
	return true
}

// claimField retains Field membership as its own typed ownership relation.
// It is intentionally not represented as a generic type edge: a Field owns
// its value while a Record or Interface owns the Field.
func (check *containment) claimField(owner, field keyspace.Term) bool {
	if owner == 0 || !hasFamily(check.counts, field, keyspace.FamilyTypeField) {
		return false
	}
	ordinal := keyspace.TermOrdinal(field) - 1
	if uint64(ordinal) >= uint64(len(check.fieldOwners)) || check.fieldOwners[ordinal] != 0 {
		return false
	}
	check.fieldOwners[ordinal] = owner
	return true
}

// markDirectReturn records only the role needed by the bound-assertion law.
// It is discarded after Build and is not a second semantic edge authority.
func (check *containment) markDirectReturn(parent, child keyspace.Term) bool {
	if keyspace.TermFamily(child) != keyspace.FamilyTypeAsserts {
		return false
	}
	ordinal := keyspace.TermOrdinal(child)
	rows := check.directReturns[keyspace.FamilyTypeAsserts]
	if ordinal == 0 || uint64(ordinal) > uint64(len(rows)) || rows[ordinal-1] != 0 {
		return false
	}
	rows[ordinal-1] = parent
	return true
}

// localForest collects typed containment rows from their owning verticals,
// then validates the one combined concrete relation. No central collector
// knows how a Type, declaration, signature, operator, or sidecar is shaped.
func localForest(component *Component, counts [keyspace.FamilyCount]uint32) (*localContainmentProof, bool) {
	if component == nil {
		return nil, false
	}
	check := newContainment(counts, len(component.types.field))
	if !emitTypesContainment(component, &check) ||
		!emitDeclarationsContainment(component, &check) ||
		!emitSignaturesContainment(component, &check) ||
		!emitContractsContainment(component, &check) ||
		!emitOperatorsContainment(component, &check) ||
		!emitOperandsContainment(component, &check) ||
		!emitPublicationsContainment(component, &check) ||
		!validBoundAssertions(component, &check) {
		return nil, false
	}
	if !check.valid() {
		return nil, false
	}
	return check.proof(), true
}

// proof transfers only the immutable local parent and Field-owner rows. The
// direct-return ledger remains construction evidence and is intentionally
// absent from the returned proof.
func (check *containment) proof() *localContainmentProof {
	if check == nil {
		return nil
	}
	return &localContainmentProof{
		parents:     check.parents,
		fieldOwners: check.fieldOwners,
	}
}

// valid is linear in the emitted edge and Field-membership count. Each node
// receives one permanent color; unlike the old fresh-map-per-root walk, a
// long acyclic chain is visited once rather than once per ancestor.
func (check *containment) valid() bool {
	for _, owner := range check.fieldOwners {
		if owner == 0 {
			return false
		}
	}
	var color [keyspace.FamilyCount][]uint8
	for _, family := range staticTypeFamilies {
		if !staticNodeFamily(family) {
			continue
		}
		if count := len(check.parents[family]); count != 0 {
			color[family] = make([]uint8, count)
		}
	}
	fieldColor := make([]uint8, len(check.fieldOwners))
	var path []keyspace.Term
	visit := func(start keyspace.Term) bool {
		path = path[:0]
		for current := start; current != 0; {
			family, ordinal := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
			var state *uint8
			switch {
			case family == keyspace.FamilyTypeField:
				if ordinal == 0 || uint64(ordinal) > uint64(len(fieldColor)) {
					// A non-static opaque parent is a valid frontier and ends
					// the local walk. A malformed field can only be emitted by
					// an invalid construction row, which fails closed here.
					return false
				}
				state = &fieldColor[ordinal-1]
			case staticNodeFamily(family):
				parents := color[family]
				if ordinal == 0 || uint64(ordinal) > uint64(len(parents)) {
					return false
				}
				state = &parents[ordinal-1]
			default:
				// Parent handles are intentionally opaque to Static. Flow,
				// Source, and other owners close those frontiers later.
				current = 0
				continue
			}
			switch *state {
			case 1:
				return false
			case 2:
				current = 0
				continue
			}
			*state = 1
			path = append(path, current)
			current = check.parentOf(current)
		}
		for _, term := range path {
			family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
			if family == keyspace.FamilyTypeField {
				fieldColor[ordinal-1] = 2
			} else {
				color[family][ordinal-1] = 2
			}
		}
		return true
	}
	for _, family := range staticTypeFamilies {
		if !staticNodeFamily(family) {
			continue
		}
		for ordinal := range check.parents[family] {
			if color[family][ordinal] == 0 && !visit(keyspace.MakeTerm(family, uint32(ordinal+1))) {
				return false
			}
		}
	}
	for index := range check.fieldOwners {
		field := keyspace.MakeTerm(keyspace.FamilyTypeField, uint32(index+1))
		if fieldColor[index] == 0 && !visit(field) {
			return false
		}
	}
	return true
}

func (check *containment) parentOf(term keyspace.Term) keyspace.Term {
	if keyspace.TermFamily(term) == keyspace.FamilyTypeField {
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 || uint64(ordinal) > uint64(len(check.fieldOwners)) {
			return 0
		}
		return check.fieldOwners[ordinal-1]
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if !staticNodeFamily(family) || ordinal == 0 {
		return 0
	}
	parents := check.parents[family]
	if uint64(ordinal) > uint64(len(parents)) {
		return 0
	}
	return parents[ordinal-1]
}
