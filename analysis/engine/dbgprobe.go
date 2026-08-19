package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"
)

// dbgprobe.go carries temporary structural counters for the solve loop. The
// executor is single threaded, so the counters are plain fields.

// DbgEngineCounters records fold shape: how many terms each canonical Point
// RHS transaction admits and how wide the widest one is.
type DbgEngineCounters struct {
	Folds        uint64
	FoldTerms    uint64
	FoldMaxTerms uint64
	ReuseAdmit   uint64
	ReuseRefuse  uint64
	ReuseTerms   uint64
	RebuildTerms uint64

	RefuseNotAscent      uint64
	RefuseNoAccumulator  uint64
	RefusePendingUnknown uint64
	RefusePendingDescend uint64
	RefuseNotOwned       uint64
	RefuseChangedRow     uint64
	RefuseReasons        [8]uint64
	RefuseDirection      [8]uint64
}

var dbgEngine DbgEngineCounters

// DbgEngine returns the accumulated solve loop counters.
func DbgEngine() DbgEngineCounters { return dbgEngine }

// DbgEngineReset clears the accumulated solve loop counters.
func DbgEngineReset() { dbgEngine = DbgEngineCounters{} }

// DbgMerge re-exports the many-way merge counters so a corpus lane outside
// the internal tree can read the FDD product shape.
func DbgMerge() (mergeMany, cells, cellPairs, cellWidth, maxOperand uint64) {
	counters := semantic.DbgSemantic()
	return counters.MergeMany, counters.Cells, counters.CellPairs, counters.CellWidth, counters.MaxOperand
}

// DbgMergeReset clears the many-way merge counters.
func DbgMergeReset() { semantic.DbgSemanticReset() }

// dbgRegionReuseRefusal attributes one accumulator reuse refusal to the exact
// clause that refused it.
func dbgRegionReuseRefusal(epoch *executorEpoch, episode *regionEpoch) {
	if episode == nil {
		return
	}
	switch {
	case episode.phase != phaseAscent:
		dbgEngine.RefuseNotAscent++
	case !episode.hasAccumulator:
		dbgEngine.RefuseNoAccumulator++
	case episode.pending.Unknown():
		dbgEngine.RefusePendingUnknown++
	case !episode.pending.Admits():
		dbgEngine.RefusePendingDescend++
	case !epoch.work.OwnsPointRHS(episode.accumulator):
		dbgEngine.RefuseNotOwned++
	default:
		return
	}
	for position := 0; position < change.ReasonWidth && position < len(dbgEngine.RefuseReasons); position++ {
		reason, ok := change.ReasonAt(position)
		if ok && episode.pending.Reasons&reason != 0 {
			dbgEngine.RefuseReasons[position]++
		}
	}
	for position, direction := range []change.Direction{change.Known, change.Ascends, change.Descends} {
		if episode.pending.Direction&direction != 0 {
			dbgEngine.RefuseDirection[position]++
		}
	}
}
