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
	RefuseColdEpisode    uint64
	RefuseDroppedAccum   uint64
	DropNarrowPhase      uint64
	DropRestart          uint64
	DropNarrowFold       uint64
	CarryRetained        uint64
	CarryExtended        uint64
	CarryRebuilt         uint64
	CarryOpened          uint64
	CarryInstalls        uint64
	RefuseReasons        [8]uint64
	RefuseDirection      [8]uint64

	// RegionsTotal through RegionInteriorPointsTotal are the Newton prototype
	// Step 0 measurement: is the pure-transport back-composition class
	// (regions with zero back Group producers) non-empty on the corpus, and
	// how many of those regions also carry an empty widen selection.
	RegionsTotal              uint64
	RegionsWidenFactorFree    uint64
	BackEnvTerms              uint64
	BackFactorTerms           uint64
	BackGroupTerms            uint64
	RegionsPureTransport      uint64
	RegionsLinearCandidate    uint64
	RegionInteriorPointsMax   uint64
	RegionInteriorPointsTotal uint64
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
		if episode.hasExact {
			dbgEngine.RefuseDroppedAccum++
		} else {
			dbgEngine.RefuseColdEpisode++
		}
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

// dbgRegionBackComposition attributes one bound Region's back composition and
// interior size to the Newton prototype Step 0 counters. widenTargets is the
// authored Target count handed to SealWidening: carrier.MergeScope keeps its
// selected slots package-private, so the pre-seal target count is the
// observable proxy for an empty widen selection.
func dbgRegionBackComposition(back, environmentBack, factorBack, points []int, widenTargets int) {
	dbgEngine.RegionsTotal++
	dbgEngine.BackGroupTerms += uint64(len(back))
	dbgEngine.BackEnvTerms += uint64(len(environmentBack))
	dbgEngine.BackFactorTerms += uint64(len(factorBack))
	widenFactorFree := widenTargets == 0
	if widenFactorFree {
		dbgEngine.RegionsWidenFactorFree++
	}
	pureTransport := len(back) == 0
	if pureTransport {
		dbgEngine.RegionsPureTransport++
		if widenFactorFree {
			dbgEngine.RegionsLinearCandidate++
		}
	}
	interior := len(points) - 1
	if interior < 0 {
		interior = 0
	}
	dbgEngine.RegionInteriorPointsTotal += uint64(interior)
	if uint64(interior) > dbgEngine.RegionInteriorPointsMax {
		dbgEngine.RegionInteriorPointsMax = uint64(interior)
	}
}
