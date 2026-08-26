// exact_cell.go owns the delivery half of the exact read boundary: the one
// place a coordinate's whole observation becomes the single cell a fold
// consumes. It sits beside read_cell.go, which owns what one observed cell
// becomes under a read's declared substitutions, so between them a form never
// restates either engine policy.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// ExactCell is one coordinate's whole answer over a read region: the value it
// holds, whether it holds one at all, and the support that answer was proved
// over. It is the value form of the exact cursor, so a caller that needs one
// cell never has to know the cursor emits a partition.
type ExactCell[V any] struct {
	Value   V
	Present bool
	Region  support.Mask
}

// DeliverExactCell consumes one exact coordinate's complete observation and
// answers the single cell it names.
//
// A read region is partitioned by guard, and the cursor emits one block per
// region the coordinate answers differently over. An unwritten block is the
// Factor's absence: it names no alternative at all, so it cannot be the
// coordinate's answer while a written block exists. A caller that took the
// first block would be answering by the partition's canonical guard order
// instead of by what the coordinate holds - and which block that order puts
// first is a property of the branch structure above the read, not of the
// coordinate being read. That is why the whole delivery is consumed here.
//
// So the delivered cell is the written block together with the region it was
// proved over, and the caller's own support conjunction narrows the conclusion
// to exactly that region. A coordinate no block writes is absent over the
// whole read region. Two written blocks disagree by construction - the cursor
// coalesces equal ones - so they name no single cell, and that is refused by
// name rather than settled by whichever arrived first, which is the same law
// ConjoinSupport states for two supports neither of which contains the other.
//
// The status is the cursor disposition of the delivery as a whole: Available
// carries a cell, Exhausted says the coordinate produced no block at all, and
// Refuse is a cursor or contract failure. The scratch is left closed and
// reusable on every non-refusing return, so the next coordinate opens on it.
func DeliverExactCell[K scalar.Key, V any](
	read ExactRead[K, V],
	policy ReadCellPolicy[V],
	ticket Ticket,
	scratch *Scratch[K, V],
) (ExactCell[V], ReadStatus) {
	if !read.Valid() || scratch == nil {
		return ExactCell[V]{}, ReadRefuse
	}
	within, withinOK := ticket.Within()
	if !withinOK || !within.Valid() {
		return ExactCell[V]{}, ReadRefuse
	}
	var delivered ExactCell[V]
	observed := false
	for {
		switch read.Read(ticket, scratch) {
		case ReadAvailable:
			observed = true
			value, available := scratch.Value()
			present := scratch.Present()
			region, regionOK := scratch.Region()
			if !available || !regionOK {
				_ = scratch.Discard(ticket)
				return ExactCell[V]{}, ReadRefuse
			}
			if !present {
				continue
			}
			if delivered.Present {
				_ = scratch.Discard(ticket)
				return ExactCell[V]{}, ReadRefuse
			}
			delivered = ExactCell[V]{Value: value, Present: true, Region: region}
		case ReadExhausted:
			if !read.Close(ticket, scratch) || !scratch.Reuse(ticket) {
				_ = scratch.Discard(ticket)
				return ExactCell[V]{}, ReadRefuse
			}
			if !observed {
				return ExactCell[V]{}, ReadExhausted
			}
			if !delivered.Present {
				delivered.Region = within
			}
			// The declared substitutions are applied to the coordinate's own
			// answer, once, after the whole delivery has been read: a sparse
			// default stands for a coordinate no block wrote, not for a block
			// that happened to be enumerated before the one that did.
			delivered.Value, delivered.Present = policy.Cell(delivered.Value, delivered.Present)
			return delivered, ReadAvailable
		default:
			_ = scratch.Discard(ticket)
			return ExactCell[V]{}, ReadRefuse
		}
	}
}
