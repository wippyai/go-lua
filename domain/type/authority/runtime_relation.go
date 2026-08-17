package typeauthority

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/domain/type/subtype"
)

// sealSubtypeRelation materializes the finite subtype relation of the sealed
// closed universe once, from the canonical checker, while the construction
// graphs are still owned by the builder. Runtime therefore holds no second
// structural proof engine: after this pass every judgment is one bit read out
// of a packed row.
//
// The relation is a pure function of the sealed rows, so it is deliberately
// not part of Runtime's content identity: two Runtimes with equal rows
// materialize the same relation.
func (b *runtimeBuilder) sealSubtypeRelation() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime relation source")
	}
	runtime := b.runtime
	runtime.closedPositions = make([]int32, len(runtime.rows))
	runtime.closedRows = make([]uint32, 0, len(runtime.rows))
	for index := range runtime.rows {
		runtime.closedPositions[index] = -1
		if !runtime.rows[index].closed {
			continue
		}
		if b.construction[index] == nil {
			return errors.New("typeauthority: closed Runtime row lacks a relation source graph")
		}
		if uint64(len(runtime.closedRows)) >= uint64(math.MaxInt32) {
			return errors.New("typeauthority: Runtime relation universe overflow")
		}
		runtime.closedPositions[index] = int32(len(runtime.closedRows))
		runtime.closedRows = append(runtime.closedRows, uint32(index+1))
	}
	count := len(runtime.closedRows)
	if count == 0 {
		return errors.New("typeauthority: empty Runtime relation universe")
	}
	runtime.subtypeStride = (count + 63) / 64
	runtime.subtypeBits = make([]uint64, count*runtime.subtypeStride)
	// One prover serves the whole matrix: each ordered pair still starts from
	// an empty memo, so no coinductive assumption crosses a judgment.
	var prover subtype.Batch
	for leftPosition, leftRow := range runtime.closedRows {
		left := b.construction[leftRow-1]
		base := leftPosition * runtime.subtypeStride
		for rightPosition, rightRow := range runtime.closedRows {
			// The canonical prover memoizes under active coinductive
			// assumptions, so its table belongs to exactly one top-level
			// judgment and is cleared between pairs.
			if !prover.IsSubtype(left, b.construction[rightRow-1]) {
				continue
			}
			runtime.subtypeBits[base+rightPosition>>6] |= 1 << (uint(rightPosition) & 63)
		}
	}
	return nil
}
