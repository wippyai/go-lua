package closed

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Judgment is the cold state this rule's fold rests on: the two axis schemas
// the constructor is fenced to, and the atom-to-selector projection derived
// from them.
//
// It is sealed once, from the schemas the declaration names as its static
// axes, and holds no runtime capability - no catalog, no binding, no owner
// callback. The fold takes carriers as arguments and reaches cold knowledge
// only through this, so what the judgment can see is exactly what the
// declaration said it rests on.
//
// The selector projection is RECEIVED rather than derived. It reads sealed
// authorities from Heap and Value at once, so no axis is answerable for it and
// the mount phase constructs it exactly once; minting a second one here from
// the same two schemas would be a second authority over which atoms select
// which slots, and that the two would agree today is not the point.
type Judgment struct {
	heaps      heapdomain.Schema
	values     *valuedomain.Schema
	projection *keymatch.SelectorProjection
}

// NewJudgment seals this rule's state from the axis schemas it is declared
// over and the selector projection its composition sealed. The two schemas
// must be one owner's pair - the Value schema retains the Heap authority it
// was sealed against - because a constructor's coordinates are local handles
// and two schemas sealed from one Link are not interchangeable, and the
// projection is held to that same pair rather than taken on trust.
func NewJudgment(heaps heapdomain.Schema, values *valuedomain.Schema, selectors *keymatch.SelectorProjection) (Judgment, bool) {
	if values == nil || !values.OwnsHeapSchema(heaps) || selectors == nil || !selectors.FencedTo(heaps, values) {
		return Judgment{}, false
	}
	return Judgment{heaps: heaps, values: values, projection: selectors}, true
}

// Valid reports whether this judgment was sealed.
func (judgment Judgment) Valid() bool {
	return judgment.values != nil && judgment.projection != nil && judgment.heaps.Valid()
}
