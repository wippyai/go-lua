package context

import (
	"encoding/binary"
	"hash/fnv"

	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/domain/heap"
)

// Bottom is the sparse no-reference cell for this exact contextual authority.
func (schema Schema) Bottom() Value {
	if !schema.Valid() {
		return Value{}
	}
	return Value{owner: schema.owner}
}

// Top is the explicit contextual widening element. It carries no synthetic
// reference, holder, or current-context guess.
func (schema Schema) Top() Value {
	if !schema.Valid() {
		return Value{}
	}
	return Value{owner: schema.owner, top: true}
}

// Same is the persistent-image fast path. A positive result implies Equal.
func Same(left, right Value) bool {
	return left.owner != nil && left.owner == right.owner && left.top == right.top &&
		len(left.rows) == len(right.rows) && (len(left.rows) == 0 || &left.rows[0] == &right.rows[0])
}

// Equal compares two owner-fenced contextual cells extensionally.
func (schema Schema) Equal(left, right Value) bool {
	if !schema.ownsValue(left) || !schema.ownsValue(right) {
		return false
	}
	if Same(left, right) {
		return true
	}
	if left.top != right.top || len(left.rows) != len(right.rows) {
		return false
	}
	for index := range left.rows {
		if !left.rows[index].Equal(right.rows[index]) {
			return false
		}
	}
	return true
}

// LessOrEq is subset inclusion over exact rows, with explicit Top as the
// greatest element. Values over different allocation coordinates are
// incomparable; the Factor admission boundary supplies that coordinate.
func (schema Schema) LessOrEq(left, right Value) bool {
	if !schema.ownsValue(left) || !schema.ownsValue(right) {
		return false
	}
	if Same(left, right) || right.top || left.IsBottom() {
		return true
	}
	if left.top {
		return false
	}
	if right.IsBottom() {
		return false
	}
	if !sameValueKey(schema, left, right) {
		return false
	}
	for _, wanted := range left.rows {
		found := false
		for _, candidate := range right.rows {
			if wanted.Equal(candidate) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Join unions exact rows for one allocation coordinate. The operation is
// commutative, associative, and idempotent; an explicit Top absorbs it.
func (schema Schema) Join(left, right Value) (Value, bool) {
	if !schema.ownsValue(left) || !schema.ownsValue(right) {
		return Value{}, false
	}
	if left.top {
		return left, true
	}
	if right.top {
		return right, true
	}
	if left.IsBottom() {
		return right, true
	}
	if right.IsBottom() {
		return left, true
	}
	if !sameValueKey(schema, left, right) {
		return Value{}, false
	}
	rows := make([]Reference, 0, len(left.rows)+len(right.rows))
	rows = append(rows, left.rows...)
	rows = append(rows, right.rows...)
	return canonicalContextRows(schema.owner, rows)
}

// Widen is exact finite relation union. The sealed directory and materialized
// role catalog make the row universe finite; preserving exact rows is the
// mathematically precise widening until an explicit Top is supplied.
func (schema Schema) Widen(previous, next Value) (Value, bool) {
	return schema.Join(previous, next)
}

// Lattice exposes this exact contextual authority through the generic solver
// algebra. Foreign values are construction defects and are rejected by the
// owner boundary before this adapter is reached.
func (schema Schema) Lattice() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom: schema.Bottom,
		Top:    schema.Top,
		Equal:  func(left, right Value) bool { return schema.Equal(left, right) },
		Same:   Same,
		LessOrEq: func(left, right Value) bool {
			return schema.LessOrEq(left, right)
		},
		Join: func(left, right Value) Value {
			joined, ok := schema.Join(left, right)
			if !ok {
				panic("heap/context: foreign or cross-key Join")
			}
			return joined
		},
		Widen: func(previous, next Value) Value {
			widened, ok := schema.Widen(previous, next)
			if !ok {
				panic("heap/context: foreign or cross-key Widen")
			}
			return widened
		},
	}
}

// Domain is an alias that mirrors the other Link-local domain authorities.
func (schema Schema) Domain() lattice.Lattice[Value] { return schema.Lattice() }

// Admit reports whether a cell belongs at one exact allocation-root
// coordinate. Bottom and Top are coordinate-independent; exact rows must all
// carry the supplied key.
func (schema Schema) Admit(key heap.Key, value Value) bool {
	if !schema.Valid() || !schema.OwnsKey(key) || key.Kind() != heap.RootAllocation || !schema.ownsValue(value) {
		return false
	}
	if value.IsBottom() || value.IsTop() {
		return true
	}
	return sameValueKeyAsHeap(schema, value, key)
}

func sameValueKey(schema Schema, left, right Value) bool {
	leftKey, leftOK := left.Key()
	rightKey, rightOK := right.Key()
	if !leftOK || !rightOK {
		return false
	}
	leftID, leftIDOK := schema.owner.heap.KeyID(leftKey)
	rightID, rightIDOK := schema.owner.heap.KeyID(rightKey)
	return leftIDOK && rightIDOK && leftID == rightID
}

func sameValueKeyAsHeap(schema Schema, value Value, key heap.Key) bool {
	valueKey, valueOK := value.Key()
	if !valueOK {
		return false
	}
	valueID, valueIDOK := schema.owner.heap.KeyID(valueKey)
	keyID, keyIDOK := schema.owner.heap.KeyID(key)
	return valueIDOK && keyIDOK && valueID == keyID
}

func (schema Schema) ownsValue(value Value) bool {
	return schema.Valid() && value.valid() && value.owner == schema.owner
}

// WidenRank is a finite remaining-row witness. Every exact contextual row is
// one member of a finite directory-context × materialization-role universe;
// Top is terminal and has no witness.
func (schema Schema) WidenRank(value Value) (uint64, bool) {
	if !schema.ownsValue(value) || value.IsTop() {
		return 0, false
	}
	potential := schema.referencePotential()
	if potential == 0 {
		return 1, true
	}
	occupied := uint64(value.ReferenceCount())
	if occupied > potential {
		return 0, false
	}
	return potential - occupied + 1, true
}

func (schema Schema) referencePotential() uint64 {
	if !schema.Valid() {
		return 0
	}
	contexts := uint64(schema.owner.directory.ContextCount())
	roles := uint64(3) // materialization.Exact, Recent, Summary
	if contexts == 0 {
		return 0
	}
	if contexts > ^uint64(0)/roles {
		return ^uint64(0)
	}
	// A Value is fixed to one allocation key, but its exact rows may retain
	// any authenticated origin/holder pair admitted by the directory. The
	// finite witness therefore counts the correlated context-pair × role
	// universe, not the unrelated number of Heap coordinates.
	perKey := contexts * roles
	if contexts > ^uint64(0)/perKey {
		return ^uint64(0)
	}
	return contexts * perKey
}

// Fingerprint is a stable compact hash for engine memoization. It includes
// the owner authority, explicit Top state, and every exact row's key/origin/
// holder/role. The schema owner remains the admission fence; this hash is not
// accepted as an identity constructor.
func (schema Schema) Fingerprint(value Value) uint64 {
	if !schema.ownsValue(value) {
		return 0
	}
	hash := fnv.New64a()
	ownerID := schema.ContentID()
	_, _ = hash.Write(ownerID[:])
	if value.top {
		_, _ = hash.Write([]byte{1})
		return hash.Sum64()
	}
	_, _ = hash.Write([]byte{0})
	for _, row := range value.rows {
		keyID, keyOK := schema.owner.heap.KeyID(row.Key())
		if !keyOK {
			return 0
		}
		_, _ = hash.Write(keyID[:])
		originID := row.Origin().Context().ID()
		holderID := row.Holder().ID()
		_, _ = hash.Write(originID[:])
		_, _ = hash.Write(holderID[:])
		_, _ = hash.Write([]byte{byte(row.Role())})
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(value.rows)))
	_, _ = hash.Write(count[:])
	return hash.Sum64()
}
