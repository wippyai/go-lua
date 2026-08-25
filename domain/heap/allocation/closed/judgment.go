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
// The selector projection is derived here rather than passed in because it IS
// a function of those two schemas: minting it beside the judgment would be a
// second authority over which atoms select which slots.
type Judgment struct {
	heaps      heapdomain.Schema
	values     *valuedomain.Schema
	projection *keymatch.SelectorProjection
}

// NewJudgment seals this rule's state from the axis schemas it is declared
// over. The two must be one owner's pair - the Value schema retains the Heap
// authority it was sealed against - because a constructor's coordinates are
// local handles and two schemas sealed from one Link are not interchangeable.
func NewJudgment(heaps heapdomain.Schema, values *valuedomain.Schema) (Judgment, bool) {
	if values == nil || !values.OwnsHeapSchema(heaps) {
		return Judgment{}, false
	}
	projection, projectionOK := keymatch.NewSelectorProjection(heaps, values)
	if !projectionOK || projection == nil {
		return Judgment{}, false
	}
	return Judgment{heaps: heaps, values: values, projection: projection}, true
}

// Valid reports whether this judgment was sealed.
func (judgment Judgment) Valid() bool {
	return judgment.values != nil && judgment.projection != nil && judgment.heaps.Valid()
}
