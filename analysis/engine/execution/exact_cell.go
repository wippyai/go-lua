// exact_cell.go owns the delivery half of the exact read boundary: the one
// place a coordinate's whole observation becomes the single cell a fold
// consumes. It sits beside read_cell.go, which owns what one observed cell
// becomes under a read's declared substitutions, so between them a form never
// restates either engine policy. Both forms that deliver a single cell - the
// direct exact read here and the selected read's per-member observation - fold
// their blocks through the one accumulator below.

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

// exactCellFold accumulates the blocks of one coordinate's delivery into the
// single cell they name. It is the whole of the law below, held apart from any
// one cursor because both read forms that deliver a single cell - the direct
// exact read and the selected read's per-member observation - step their own
// cursor and must reach the same answer from it.
type exactCellFold[V any] struct {
	cell    ExactCell[V]
	written int
	// unwritten is the Factor's own reading of an unwritten block. An absence
	// still delivers a value - the Factor's, not the language's zero - and
	// that reading is the coordinate's, so which block it was taken from does
	// not change what the delivered absence means.
	unwritten V
	absent    bool
}

// admit takes one block of the delivery under the axis's own join.
//
// An unwritten block names no alternative and cannot be the coordinate's
// answer, but its reading is kept so an absence is still delivered as the
// Factor read it. Written blocks disagree by construction - the cursor
// coalesces equal ones - so over a region that spans more than one of them the
// coordinate holds their join and nothing smaller.
func (fold *exactCellFold[V]) admit(value V, present bool, region support.Mask, join func(V, V) (V, bool)) bool {
	if !present {
		if !fold.absent {
			fold.unwritten, fold.absent = value, true
		}
		return true
	}
	if fold.written == 0 {
		fold.cell, fold.written = ExactCell[V]{Value: value, Present: true, Region: region}, 1
		return true
	}
	joined, joinOK := join(fold.cell.Value, value)
	if !joinOK {
		return false
	}
	fold.cell.Value, fold.written = joined, fold.written+1
	return true
}

// settle answers the delivered cell once every block has been admitted.
//
// One written block is the exact answer, over exactly the region it was proved
// on. A coordinate no block wrote is absent over the whole read region,
// carrying the Factor's reading of it. Blocks that disagree have no single
// exact value, so what the coordinate holds over the read region is their
// join, proved over the whole of it - which is the sound answer a single cell
// can give, and weaker on each block than an invocation run once per block
// would be. The read's declared substitutions are applied once, here: a sparse
// default stands for a coordinate nothing wrote, not for a block that happened
// to be enumerated before the one that did.
func (fold *exactCellFold[V]) settle(within support.Mask, policy ReadCellPolicy[V]) ExactCell[V] {
	cell := fold.cell
	switch {
	case fold.written == 0:
		cell.Value, cell.Region = fold.unwritten, within
	case fold.written > 1:
		cell.Region = within
	}
	cell.Value, cell.Present = policy.Cell(cell.Value, cell.Present)
	return cell
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
	var fold exactCellFold[V]
	observed := false
	for {
		switch read.Read(ticket, scratch) {
		case ReadAvailable:
			observed = true
			value, available := scratch.Value()
			region, regionOK := scratch.Region()
			if !available || !regionOK || !fold.admit(value, scratch.Present(), region, read.join) {
				_ = scratch.Discard(ticket)
				return ExactCell[V]{}, ReadRefuse
			}
		case ReadExhausted:
			if !read.Close(ticket, scratch) || !scratch.Reuse(ticket) {
				_ = scratch.Discard(ticket)
				return ExactCell[V]{}, ReadRefuse
			}
			if !observed {
				return ExactCell[V]{}, ReadExhausted
			}
			return fold.settle(within, policy), ReadAvailable
		default:
			_ = scratch.Discard(ticket)
			return ExactCell[V]{}, ReadRefuse
		}
	}
}
