package typevalue

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/lattice"
)

type valueKind uint8

const (
	valueBottom valueKind = iota + 1
	valueSingleton
	valueSparse
	valueTop
)

// Value is an immutable exact powerset value. Singletons stay allocation-free;
// sparse unions own their sorted atom IDs; Top is a constant tag.
type Value struct {
	owner       *Authority
	kind        valueKind
	single      uint64
	atoms       []uint64
	fingerprint uint64
}

func (a *Authority) Bottom() Value {
	if a == nil {
		return Value{}
	}
	return Value{owner: a, kind: valueBottom, fingerprint: valueFingerprint(valueBottom, 0, nil)}
}

func (a *Authority) Top() Value {
	if a == nil {
		return Value{}
	}
	return Value{owner: a, kind: valueTop, fingerprint: valueFingerprint(valueTop, 0, nil)}
}

func (a *Authority) Singleton(atom Atom) (Value, bool) {
	if !a.ownsAtom(atom) {
		return Value{}, false
	}
	return Value{owner: a, kind: valueSingleton, single: atom.id, fingerprint: valueFingerprint(valueSingleton, atom.id, nil)}, true
}

func (a *Authority) FromAtoms(atoms ...Atom) (Value, bool) {
	if a == nil {
		return Value{}, false
	}
	ids := make([]uint64, len(atoms))
	for index, atom := range atoms {
		if !a.ownsAtom(atom) {
			return Value{}, false
		}
		ids[index] = atom.id
	}
	return a.fromIDs(ids), true
}

func (a *Authority) fromIDs(ids []uint64) Value {
	if len(ids) == 0 {
		return a.Bottom()
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	write := 1
	for read := 1; read < len(ids); read++ {
		if ids[read] != ids[write-1] {
			ids[write] = ids[read]
			write++
		}
	}
	ids = ids[:write]
	if uint64(len(ids)) == a.atomEnd {
		return a.Top()
	}
	if len(ids) == 1 {
		return Value{owner: a, kind: valueSingleton, single: ids[0], fingerprint: valueFingerprint(valueSingleton, ids[0], nil)}
	}
	return Value{owner: a, kind: valueSparse, atoms: ids, fingerprint: valueFingerprint(valueSparse, 0, ids)}
}

func (a *Authority) Owns(value Value) bool {
	if a == nil || value.owner != a {
		return false
	}
	switch value.kind {
	case valueBottom, valueTop:
		return value.fingerprint != 0
	case valueSingleton:
		return value.single < a.atomEnd && value.fingerprint != 0
	case valueSparse:
		// Sparse rows can only be constructed by fromIDs after atom validation;
		// fields are private, so admission remains O(1) on the solver hot path.
		return len(value.atoms) >= 2 && uint64(len(value.atoms)) < a.atomEnd && value.fingerprint != 0
	default:
		return false
	}
}

func (a *Authority) Equal(left, right Value) bool {
	if !a.Owns(left) || !a.Owns(right) || left.kind != right.kind {
		return false
	}
	switch left.kind {
	case valueBottom, valueTop:
		return true
	case valueSingleton:
		return left.single == right.single
	case valueSparse:
		if len(left.atoms) != len(right.atoms) {
			return false
		}
		for index := range left.atoms {
			if left.atoms[index] != right.atoms[index] {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (a *Authority) Same(left, right Value) bool { return a.Equal(left, right) }

func (a *Authority) LessOrEq(left, right Value) bool {
	if !a.Owns(left) || !a.Owns(right) {
		return false
	}
	if left.kind == valueBottom || right.kind == valueTop || a.Equal(left, right) {
		return true
	}
	if left.kind == valueTop || right.kind == valueBottom {
		return false
	}
	leftLen, rightLen := valueCardinality(left), valueCardinality(right)
	for i, j := 0, 0; i < leftLen; {
		leftID := valueIDAt(left, i)
		for j < rightLen && valueIDAt(right, j) < leftID {
			j++
		}
		if j == rightLen || valueIDAt(right, j) != leftID {
			return false
		}
		i++
	}
	return true
}

func (a *Authority) Join(left, right Value) Value {
	if !a.Owns(left) || !a.Owns(right) {
		panic("typevalue: foreign value reached sealed lattice")
	}
	if left.kind == valueTop || right.kind == valueTop {
		return a.Top()
	}
	if left.kind == valueBottom {
		return right
	}
	if right.kind == valueBottom {
		return left
	}
	if a.Equal(left, right) {
		return left
	}
	leftLen, rightLen := valueCardinality(left), valueCardinality(right)
	merged := make([]uint64, 0, leftLen+rightLen)
	for i, j := 0, 0; i < leftLen || j < rightLen; {
		switch {
		case j == rightLen || i < leftLen && valueIDAt(left, i) < valueIDAt(right, j):
			merged = append(merged, valueIDAt(left, i))
			i++
		case i == leftLen || valueIDAt(right, j) < valueIDAt(left, i):
			merged = append(merged, valueIDAt(right, j))
			j++
		default:
			merged = append(merged, valueIDAt(left, i))
			i++
			j++
		}
	}
	return a.fromIDs(merged)
}

func (a *Authority) Widen(previous, next Value) Value { return a.Join(previous, next) }

func (a *Authority) WidenRank(value Value) uint64 {
	if !a.Owns(value) {
		return 0
	}
	if value.kind == valueTop {
		return 0
	}
	return a.atomEnd - uint64(valueCardinality(value))
}

func (a *Authority) Fingerprint(value Value) uint64 {
	if !a.Owns(value) {
		return 0
	}
	return value.fingerprint
}

func valueFingerprint(kind valueKind, single uint64, atoms []uint64) uint64 {
	hash := uint64(1469598103934665603)
	mix := func(word uint64) {
		for shift := 0; shift < 64; shift += 8 {
			hash ^= (word >> shift) & 0xff
			hash *= 1099511628211
		}
	}
	mix(uint64(kind))
	if kind == valueSingleton {
		mix(single)
	} else if kind == valueSparse {
		for _, atom := range atoms {
			mix(atom)
		}
	}
	if hash == 0 {
		return 1
	}
	return hash
}

func (a *Authority) AtomCountIn(value Value) (int, bool) {
	if !a.Owns(value) || value.kind == valueTop {
		return 0, false
	}
	return valueCardinality(value), true
}

func (a *Authority) AtomAt(value Value, index int) (Atom, bool) {
	if !a.Owns(value) || index < 0 || value.kind == valueTop {
		return Atom{}, false
	}
	if index >= valueCardinality(value) {
		return Atom{}, false
	}
	return Atom{owner: a, id: valueIDAt(value, index)}, true
}

func (a *Authority) Lattice() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{Bottom: a.Bottom, Top: a.Top, Equal: a.Equal, Same: a.Same, LessOrEq: a.LessOrEq, Join: a.Join, Widen: a.Widen}
}

func valueIDAt(value Value, index int) uint64 {
	switch value.kind {
	case valueSingleton:
		if index == 0 {
			return value.single
		}
	case valueSparse:
		if index >= 0 && index < len(value.atoms) {
			return value.atoms[index]
		}
	}
	return 0
}

func valueCardinality(value Value) int {
	switch value.kind {
	case valueSingleton:
		return 1
	case valueSparse:
		return len(value.atoms)
	default:
		return 0
	}
}
