// Package carrier owns the Type Factor's reduced terminal value.
//
// A finite value is exactly a result Pack together with the finite set of
// Program value positions from which that Pack was obtained.  Pack retains Lua
// result-position correlation; Origins retains source correlation.  Neither
// component is reconstructed from a string, a global registry, or a parallel
// value plane.  FactorTop is the sole collapsed value.
package carrier

import (
	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/internal/hash"
)

// Value is either FactorTop or the direct product of one sealed, local Pack
// and one finite Origin set.  Its fields stay private so every non-top value
// has passed both component ownership fences.
//
// Bottom is represented uniquely by a Pack bottom and an empty Origin set.
// An origin attached to an uninhabited Pack would describe no execution and is
// therefore rejected rather than silently retained or discarded.
type Value struct {
	data     typedomain.Pack
	ceiling  typedomain.Pack
	origins  origin.Set
	universe *origin.Universe
	top      bool
}

// WidenRank is the exact lexicographic descent witness for carrier widening.
// The first two components are the Pack proof; RemainingOrigins is the finite
// complement in this Link's closed provenance universe.  FactorTop has the
// unique all-zero rank.
type WidenRank struct {
	NotTop           uint64
	ShapeClass       uint64
	ExactLabels      uint64
	RemainingOrigins uint64
}

// Top is the Factor's explicit collapsed value for one sealed Table and one
// closed Link-derived provenance universe. It is distinct from a Pack whose
// labels have widened to TypeTop: that finite Pack still has a shape and exact
// provenance. Retaining both authorities inside Top prevents a top from one
// installed Factor being mixed with a finite value from another.
func Top(table *typedomain.Table, universe *origin.Universe) (Value, bool) {
	if table == nil || !table.Sealed() || universe == nil {
		return Value{}, false
	}
	data := table.Top()
	if !validPack(data) || !universe.Valid(origin.Empty()) {
		return Value{}, false
	}
	return Value{data: data, ceiling: data, universe: universe, top: true}, true
}

// Bottom builds the unique bottom for one sealed Type Table.  The enclosing
// Type Factor validates Origin source coordinates against its exact Link at
// rule declaration; origin.Set deliberately remains a pure finite algebra.
func Bottom(table *typedomain.Table, universe *origin.Universe) (Value, bool) {
	if table == nil || !table.Sealed() || universe == nil {
		return Value{}, false
	}
	return New(table, universe, table.Bottom(), origin.Empty())
}

// New admits one finite carrier.  A Pack validates itself by a lawful
// idempotent join: it succeeds only for a sealed Pack from one Table, without
// exposing the Table's private owner word. The enclosing analysis Domain
// validates Origin source coordinates against its Link before construction;
// carrier itself remains a pure product algebra.
func New(table *typedomain.Table, universe *origin.Universe, data typedomain.Pack, origins origin.Set) (Value, bool) {
	if table == nil || !table.Sealed() || universe == nil || !validPack(data) || !universe.Valid(origins) {
		return Value{}, false
	}
	ceiling := table.Top()
	if !validPack(ceiling) || !typedomain.LessEqual(data, ceiling) {
		return Value{}, false
	}
	if data.IsBottom() && origins.Count() != 0 {
		return Value{}, false
	}
	return Value{data: data, ceiling: ceiling, origins: origins, universe: universe}, true
}

// IsTop reports the distinguished FactorTop case.
func (value Value) IsTop() bool { return value.top }

// IsBottom reports the unique finite bottom.  The zero Value is invalid, not
// a hidden second spelling of bottom.
func (value Value) IsBottom() bool {
	return value.validFinite() && value.data.IsBottom()
}

// Rank projects one component of WidenRank. Components 0, 1, 2, and 3 are
// the finite-vs-FactorTop phase, Pack shape, Pack exact labels, and remaining
// finite Origins respectively.
// An invalid value or an invalid component has no rank.
func (value Value) Rank(component int) (uint64, bool) {
	rank, ok := value.WidenRank()
	if !ok {
		return 0, false
	}
	switch component {
	case 0:
		return rank.NotTop, true
	case 1:
		return rank.ShapeClass, true
	case 2:
		return rank.ExactLabels, true
	case 3:
		return rank.RemainingOrigins, true
	default:
		return 0, false
	}
}

// WidenRank returns the complete finite descent witness. It does not invent a
// rank for a malformed/foreign carrier. A FactorTop is valid and has rank
// (0,0,0), the terminal point of every widening chain.
func (value Value) WidenRank() (WidenRank, bool) {
	if !value.valid() {
		return WidenRank{}, false
	}
	if value.top {
		return WidenRank{}, true
	}
	remaining, ok := value.universe.Remaining(value.origins)
	if !ok {
		return WidenRank{}, false
	}
	pack := value.data.WidenRank()
	return WidenRank{
		NotTop:           1,
		ShapeClass:       pack.ShapeClass,
		ExactLabels:      pack.ExactLabels,
		RemainingOrigins: remaining,
	}, true
}

// Data returns the immutable correlated Pack of a finite Value.  FactorTop
// deliberately has no Pack projection.
func (value Value) Data() (typedomain.Pack, bool) {
	if !value.validFinite() {
		return typedomain.Pack{}, false
	}
	return value.data, true
}

// Origins returns the immutable finite source-position evidence of a finite
// Value. FactorTop deliberately has no finite provenance projection.
func (value Value) Origins() (origin.Set, bool) {
	if !value.validFinite() {
		return origin.Empty(), false
	}
	return value.origins, true
}

// Equal is equality over the explicit top or both product components.
func Equal(left, right Value) bool {
	if !left.valid() || !right.valid() || left.universe != right.universe {
		return false
	}
	if left.top || right.top {
		return left.top && right.top && typedomain.Equal(left.data, right.data)
	}
	return typedomain.Equal(left.data, right.data) && left.origins.Equal(right.origins)
}

// LessEqual is the direct-product information order.  Origins order by
// inclusion because each additional source position is another possible
// provenance, hence less precise.
func LessEqual(left, right Value) bool {
	if !left.valid() || !right.valid() || left.universe != right.universe {
		return false
	}
	if right.top {
		return true
	}
	if left.top {
		return false
	}
	return typedomain.LessEqual(left.data, right.data) && left.origins.LessEqual(right.origins)
}

// Join is the exact least upper bound of finite carriers.  It unions neither
// strings nor type graphs: Pack and Origin each keep their own finite,
// owner-fenced representation. FactorTop absorbs every valid value.
func Join(left, right Value) (Value, bool) {
	if !left.valid() || !right.valid() || left.universe != right.universe {
		return Value{}, false
	}
	if left.top || right.top {
		return topLike(left)
	}
	data, ok := typedomain.Join(left.data, right.data)
	if !ok {
		return Value{}, false
	}
	origins := origin.Union(left.origins, right.origins)
	return NewLike(left, data, origins)
}

// Widen is legal only at a solver-proven Mu recurrence.  Pack supplies its
// terminating shape/label widening; finite Origin evidence has a finite
// carrier and widens exactly by union.  No cardinality budget or evidence
// collapse is introduced here.
func Widen(previous, next Value) (Value, bool) {
	if !previous.valid() || !next.valid() || previous.universe != next.universe {
		return Value{}, false
	}
	if previous.top || next.top {
		return topLike(previous)
	}
	data, ok := typedomain.Widen(previous.data, next.data)
	if !ok {
		return Value{}, false
	}
	origins := origin.Union(previous.origins, next.origins)
	return NewLike(previous, data, origins)
}

// Mu is the exact recurrence closure before any Pack widening is required.
// It is Join, named separately to make the call-site obligation visible: the
// solver may invoke it only for a Link-certified recurrence.  It never grows
// an unbounded auxiliary witness structure.
func Mu(previous, next Value) (Value, bool) { return Join(previous, next) }

// Narrow is intentionally unavailable. typedomain.Pack has no proven
// decreasing operator, so manufacturing a no-op or a componentwise origin
// intersection would falsely advertise a lawful narrowing phase.
func Narrow(_, _ Value) (Value, bool) { return Value{}, false }

// Hash fingerprints the complete finite carrier. Equal values always receive
// the same hash; collisions remain harmless because Equal is authoritative.
func (value Value) Hash() uint64 {
	if !value.valid() {
		return 0
	}
	if value.top {
		return hash.MixHash(0x9e3779b97f4a7c15, value.data.Hash())
	}
	h := hash.MixHash(0x51d7348c2f6a19b3, value.data.Hash())
	for index := 0; index < value.origins.Count(); index++ {
		entry, ok := value.origins.At(index)
		if !ok {
			return 0
		}
		h = hash.MixHash(h, uint64(entry.Source()))
		position := entry.Position()
		h = hash.MixHash(h, uint64(position.Index())<<1)
		if position.IsTail() {
			h = hash.MixHash(h, 1)
		}
	}
	return h
}

func (value Value) valid() bool {
	if value.universe == nil || !validPack(value.data) || !validPack(value.ceiling) ||
		!value.ceiling.IsTop() || !typedomain.LessEqual(value.data, value.ceiling) ||
		!value.universe.Valid(value.origins) {
		return false
	}
	if value.top {
		return typedomain.Equal(value.data, value.ceiling) && value.origins.Count() == 0
	}
	return !value.data.IsBottom() || value.origins.Count() == 0
}

func (value Value) validFinite() bool {
	if value.top || !value.valid() {
		return false
	}
	return true
}

func validPack(value typedomain.Pack) bool {
	_, ok := typedomain.Join(value, value)
	return ok
}

func topLike(value Value) (Value, bool) {
	if !value.valid() {
		return Value{}, false
	}
	if value.top {
		return value, true
	}
	return Value{data: value.ceiling, ceiling: value.ceiling, universe: value.universe, top: true}, true
}

func NewLike(value Value, data typedomain.Pack, origins origin.Set) (Value, bool) {
	if !value.valid() {
		return Value{}, false
	}
	if !typedomain.LessEqual(data, value.ceiling) {
		return Value{}, false
	}
	if data.IsBottom() && origins.Count() != 0 {
		return Value{}, false
	}
	if !value.universe.Valid(origins) {
		return Value{}, false
	}
	return Value{data: data, ceiling: value.ceiling, origins: origins, universe: value.universe}, true
}
