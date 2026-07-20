package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// boundaryAffectedAtomKind is the closed structural vocabulary from which a
// coordinate family may define destination ownership. Selectors are finite
// disjunctions: when any atom belongs to the boundary closure, applying the
// transported fragment owns that destination coordinate.
type boundaryAffectedAtomKind uint8

const (
	boundaryAffectedAtomInvalid boundaryAffectedAtomKind = iota
	boundaryAffectedAtomPath
	boundaryAffectedAtomIdentity
	boundaryAffectedAtomSlot
	boundaryAffectedAtomHeapSuffix
	boundaryAffectedAtomAlways
)

type boundaryAffectedAtom struct {
	kind       boundaryAffectedAtomKind
	path       keyspace.Key
	identity   identity.Term
	slot       statekey.Value
	heapSuffix boundaryHeapSuffix
}

// boundaryAffectedSelector is the one immutable destination-ownership law
// used by both boundary execution and static footprint scheduling. Its atoms
// are canonical and available package-locally as exact wake incidences; no
// selector can inspect State, a coordinate inventory, or a sibling family.
type boundaryAffectedSelector struct {
	seal  *boundaryAffectedSelectorSeal
	keys  *keyspace.KeySpace
	atoms []boundaryAffectedAtom
}

type boundaryAffectedSelectorSeal struct{ owned byte }

type boundaryAffectedSelectorBuilder struct {
	keys     *keyspace.KeySpace
	atoms    []boundaryAffectedAtom
	declared bool
	terminal bool
	err      error
}

func newBoundaryAffectedSelectorBuilder(keys *keyspace.KeySpace) *boundaryAffectedSelectorBuilder {
	return &boundaryAffectedSelectorBuilder{keys: keys}
}

func (b *boundaryAffectedSelectorBuilder) add(atom boundaryAffectedAtom) {
	if b == nil || b.err != nil {
		return
	}
	if b.terminal {
		b.err = fmt.Errorf("state: boundary affected selector mixes terminal and atom declarations")
		return
	}
	b.declared = true
	b.atoms = append(b.atoms, atom)
}

func (b *boundaryAffectedSelectorBuilder) anyPaths(paths ...keyspace.Key) {
	for _, path := range paths {
		if path.Kind == keyspace.KindInvalid {
			continue
		}
		if b == nil || b.keys == nil || !b.keys.Valid() || b.keys.FormatReadOnly(path) == "" {
			if b != nil {
				b.err = fmt.Errorf("state: boundary affected selector contains a foreign path")
			}
			return
		}
		b.add(boundaryAffectedAtom{kind: boundaryAffectedAtomPath, path: path})
	}
}

func (b *boundaryAffectedSelectorBuilder) anyIdentities(terms ...identity.Term) {
	for _, term := range terms {
		if !term.Valid() {
			if b != nil {
				b.err = fmt.Errorf("state: boundary affected selector contains an invalid identity")
			}
			return
		}
		b.add(boundaryAffectedAtom{kind: boundaryAffectedAtomIdentity, identity: term})
	}
}

func (b *boundaryAffectedSelectorBuilder) anySlots(slots ...statekey.Value) {
	for _, slot := range slots {
		if slot == 0 || !validBoundaryRootSlot(slot) {
			if b != nil {
				b.err = fmt.Errorf("state: boundary affected selector contains an invalid slot")
			}
			return
		}
		b.add(boundaryAffectedAtom{kind: boundaryAffectedAtomSlot, slot: slot})
	}
}

func (b *boundaryAffectedSelectorBuilder) anyHeapSuffixes(suffixes ...boundaryHeapSuffix) {
	for _, suffix := range suffixes {
		if !suffix.owner.Valid() || suffix.suffix.Kind == keyspace.KindInvalid || b == nil || b.keys == nil || b.keys.FormatReadOnly(suffix.suffix) == "" {
			if b != nil {
				b.err = fmt.Errorf("state: boundary affected selector contains an invalid heap suffix")
			}
			return
		}
		b.add(boundaryAffectedAtom{kind: boundaryAffectedAtomHeapSuffix, heapSuffix: suffix})
	}
}

func (b *boundaryAffectedSelectorBuilder) always() {
	if b == nil || b.err != nil {
		return
	}
	if b.declared {
		b.err = fmt.Errorf("state: boundary affected selector Always is not exclusive")
		return
	}
	b.declared, b.terminal = true, true
	b.atoms = append(b.atoms, boundaryAffectedAtom{kind: boundaryAffectedAtomAlways})
}

func (b *boundaryAffectedSelectorBuilder) never() {
	if b == nil || b.err != nil {
		return
	}
	if b.declared {
		b.err = fmt.Errorf("state: boundary affected selector Never is not exclusive")
		return
	}
	b.declared, b.terminal = true, true
}

func (b *boundaryAffectedSelectorBuilder) neverIfEmpty() {
	if b != nil && b.err == nil && !b.declared {
		b.never()
	}
}

func (b *boundaryAffectedSelectorBuilder) seal() (boundaryAffectedSelector, error) {
	if b == nil || b.keys == nil || !b.keys.Valid() || b.err != nil || !b.declared {
		if b != nil && b.err != nil {
			return boundaryAffectedSelector{}, b.err
		}
		return boundaryAffectedSelector{}, fmt.Errorf("state: boundary affected selector is undeclared")
	}
	atoms := append([]boundaryAffectedAtom(nil), b.atoms...)
	sort.Slice(atoms, func(i, j int) bool { return boundaryAffectedAtomLess(b.keys, atoms[i], atoms[j]) })
	unique := atoms[:0]
	for _, atom := range atoms {
		if len(unique) == 0 || unique[len(unique)-1] != atom {
			unique = append(unique, atom)
		}
	}
	return boundaryAffectedSelector{seal: &boundaryAffectedSelectorSeal{}, keys: b.keys, atoms: unique}, nil
}

func boundaryAffectedAtomLess(keys *keyspace.KeySpace, left, right boundaryAffectedAtom) bool {
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	switch left.kind {
	case boundaryAffectedAtomPath:
		return keys.Less(left.path, right.path)
	case boundaryAffectedAtomIdentity:
		return identityTermLess(left.identity, right.identity)
	case boundaryAffectedAtomSlot:
		return left.slot < right.slot
	case boundaryAffectedAtomHeapSuffix:
		if left.heapSuffix.owner != right.heapSuffix.owner {
			return identityTermLess(left.heapSuffix.owner, right.heapSuffix.owner)
		}
		return keys.Less(left.heapSuffix.suffix, right.heapSuffix.suffix)
	default:
		return false
	}
}

func (s boundaryAffectedSelector) validFor(keys *keyspace.KeySpace) bool {
	return s.seal != nil && s.keys == keys && keys != nil && keys.Valid()
}

func (s boundaryAffectedSelector) affected(closure BoundaryClosure) bool {
	if !s.validFor(s.keys) {
		return false
	}
	for _, atom := range s.atoms {
		switch atom.kind {
		case boundaryAffectedAtomPath:
			if closure.ContainsPath(atom.path) {
				return true
			}
		case boundaryAffectedAtomIdentity:
			if closure.ContainsIdentityTerm(atom.identity) {
				return true
			}
		case boundaryAffectedAtomSlot:
			if closure.ContainsSlot(atom.slot) {
				return true
			}
		case boundaryAffectedAtomHeapSuffix:
			if closure.ContainsHeapSuffixTerm(atom.heapSuffix.owner, atom.heapSuffix.suffix) {
				return true
			}
		case boundaryAffectedAtomAlways:
			return true
		}
	}
	return false
}

// incidenceCount/incidence expose the immutable selector to the state-owned
// static scheduler without allocating a detached inventory or exposing family
// key representations outside this package.
func (s boundaryAffectedSelector) incidenceCount() int { return len(s.atoms) }

func (s boundaryAffectedSelector) incidence(index int) (boundaryAffectedAtom, bool) {
	if index < 0 || index >= len(s.atoms) {
		return boundaryAffectedAtom{}, false
	}
	return s.atoms[index], true
}
