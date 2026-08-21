//go:build zzsolveprobe

package oracle

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	engineprobe "github.com/wippyai/go-lua/analysis/engine"
)

// ZZPROBE: solver-ladder stage 0 measurement lane. It runs the frozen corpus
// serially through the existing diagnostic spine and attributes every global
// solve counter to the fixture that produced it, which a concurrent walk
// cannot do. Compiled only with -tags zzsolveprobe; the default oracle test
// binary never links this file.
//
// The classes it reports are the operations an apply-cache or a region memo
// would have to serve:
//
//	evaluates  -- one Rule Group fold over its transported input vector
//	             (executorEpoch.evaluate, runtime_point_fold.go:71)
//	folds      -- one canonical Point RHS transaction
//	             (foldPointTermSetsWithBoundary, runtime_point_fold.go:360)
//	rhs        -- one Region head E+B refold
//	             (regionRHS, runtime_region_interface.go:20)
//	refreshes  -- one Point candidate replacement pass
//	             (refreshPoint, runtime_point_refresh.go:202)
//	cellPairs  -- one pairwise lattice join inside a many-way merge
//	             (semantic.JoinContributionsMany)
//
// Distinct-key counts for the first four classes need call-site hooks that do
// not exist yet; only cellPairs is key-instrumented today (semantic's
// zzProbeCellPair hook, readable with -tags typprobe).

type zzProbeSolverSample struct {
	name  string
	class string
	err   bool

	seal, compile, solve time.Duration

	epochs, passes, refreshes, evaluates, folds, rhs, restarts uint64
	publications, semanticPubs, rawPubs, rawOnly, bumps        uint64
	ifaceRefresh, ifaceDone, ifaceFallback                     uint64
	maxQueue, maxEpisode                                       uint64

	dbgFolds, dbgFoldTerms, dbgFoldMax               uint64
	reuseAdmit, reuseRefuse, reuseTerms, rebuildTerm uint64
	dropRestart, dropNarrowPhase, dropNarrowFold     uint64

	refuseNotAscent, refuseNoAccumulator, refuseColdEpisode, refuseDroppedAccum  uint64
	refusePendingUnknown, refusePendingDescend, refuseNotOwned, refuseChangedRow uint64
	carryRetained, carryExtended, carryRebuilt, carryOpened                      uint64

	mergeMany, cells, cellPairs, cellWidth, maxOperand uint64

	regionsTotal, regionsWidenFactorFree               uint64
	backEnvTerms, backFactorTerms, backGroupTerms      uint64
	regionsPureTransport, regionsLinearCandidate       uint64
	regionInteriorPointsMax, regionInteriorPointsTotal uint64
}

func zzProbeSolverSampleOne(t *testing.T, project corpusHarnessProject) zzProbeSolverSample {
	sample := zzProbeSolverSample{name: project.name}
	engineprobe.DbgEngineReset()
	engineprobe.DbgMergeReset()
	run, class, err := corpusHarnessExecuteDetached(t, project, corpusHarnessDiagnosticMode())
	sample.class, sample.err = class, err != nil
	if run == nil {
		return sample
	}
	sample.seal, sample.compile, sample.solve = run.cost.seal, run.cost.compile, run.cost.solve
	engine := run.solveDiagnostics.Engine
	sample.epochs, sample.passes = engine.Epochs, engine.EpochPasses
	sample.refreshes, sample.evaluates = engine.Refreshes, engine.Evaluates
	sample.folds, sample.rhs, sample.restarts = engine.Folds, engine.RegionRHS, engine.Restarts
	sample.publications, sample.semanticPubs = engine.Publications, engine.SemanticPublications
	sample.rawPubs, sample.rawOnly, sample.bumps = engine.RawPublications, engine.RawOnlyPublications, engine.VersionBumps
	sample.ifaceRefresh, sample.ifaceDone, sample.ifaceFallback = engine.InterfaceRefreshes, engine.InterfaceRefreshCompleted, engine.InterfaceRefreshFallbacks
	sample.maxQueue, sample.maxEpisode = engine.MaxQueue, engine.MaxEpisode

	counters := engineprobe.DbgEngine()
	sample.dbgFolds, sample.dbgFoldTerms, sample.dbgFoldMax = counters.Folds, counters.FoldTerms, counters.FoldMaxTerms
	sample.reuseAdmit, sample.reuseRefuse = counters.ReuseAdmit, counters.ReuseRefuse
	sample.reuseTerms, sample.rebuildTerm = counters.ReuseTerms, counters.RebuildTerms
	sample.dropRestart, sample.dropNarrowPhase, sample.dropNarrowFold = counters.DropRestart, counters.DropNarrowPhase, counters.DropNarrowFold
	sample.refuseNotAscent, sample.refuseNoAccumulator = counters.RefuseNotAscent, counters.RefuseNoAccumulator
	sample.refuseColdEpisode, sample.refuseDroppedAccum = counters.RefuseColdEpisode, counters.RefuseDroppedAccum
	sample.refusePendingUnknown, sample.refusePendingDescend = counters.RefusePendingUnknown, counters.RefusePendingDescend
	sample.refuseNotOwned, sample.refuseChangedRow = counters.RefuseNotOwned, counters.RefuseChangedRow
	sample.carryRetained, sample.carryExtended = counters.CarryRetained, counters.CarryExtended
	sample.carryRebuilt, sample.carryOpened = counters.CarryRebuilt, counters.CarryOpened

	sample.mergeMany, sample.cells, sample.cellPairs, sample.cellWidth, sample.maxOperand = engineprobe.DbgMerge()

	sample.regionsTotal, sample.regionsWidenFactorFree = counters.RegionsTotal, counters.RegionsWidenFactorFree
	sample.backEnvTerms, sample.backFactorTerms, sample.backGroupTerms = counters.BackEnvTerms, counters.BackFactorTerms, counters.BackGroupTerms
	sample.regionsPureTransport, sample.regionsLinearCandidate = counters.RegionsPureTransport, counters.RegionsLinearCandidate
	sample.regionInteriorPointsMax, sample.regionInteriorPointsTotal = counters.RegionInteriorPointsMax, counters.RegionInteriorPointsTotal
	return sample
}

func zzProbeSolverLine(sample zzProbeSolverSample) string {
	return fmt.Sprintf(
		"%-52s solve=%-10s compile=%-10s class=%-14s err=%t | epochs=%d passes=%d refreshes=%d evaluates=%d folds=%d rhs=%d restarts=%d "+
			"pubs=%d sem=%d raw=%d rawOnly=%d bumps=%d iface=%d/%d/%d maxQueue=%d maxEpisode=%d | "+
			"dbgFolds=%d foldTerms=%d foldMax=%d reuse=%d/%d reuseTerms=%d rebuildTerms=%d drops=%d/%d/%d | "+
			"refuse{notAscent=%d noAccum=%d cold=%d dropped=%d pendUnknown=%d pendDescend=%d notOwned=%d changedRow=%d} carry{%d/%d/%d/%d} | "+
			"mergeMany=%d cells=%d cellPairs=%d cellWidth=%d maxOperand=%d | "+
			"regions=%d widenFree=%d pureTransport=%d linearCandidate=%d back{env=%d factor=%d group=%d} interior{max=%d total=%d}",
		sample.name, sample.solve.Round(time.Microsecond), sample.compile.Round(time.Microsecond), sample.class, sample.err,
		sample.epochs, sample.passes, sample.refreshes, sample.evaluates, sample.folds, sample.rhs, sample.restarts,
		sample.publications, sample.semanticPubs, sample.rawPubs, sample.rawOnly, sample.bumps,
		sample.ifaceRefresh, sample.ifaceDone, sample.ifaceFallback, sample.maxQueue, sample.maxEpisode,
		sample.dbgFolds, sample.dbgFoldTerms, sample.dbgFoldMax, sample.reuseAdmit, sample.reuseRefuse,
		sample.reuseTerms, sample.rebuildTerm, sample.dropRestart, sample.dropNarrowPhase, sample.dropNarrowFold,
		sample.refuseNotAscent, sample.refuseNoAccumulator, sample.refuseColdEpisode, sample.refuseDroppedAccum,
		sample.refusePendingUnknown, sample.refusePendingDescend, sample.refuseNotOwned, sample.refuseChangedRow,
		sample.carryRetained, sample.carryExtended, sample.carryRebuilt, sample.carryOpened,
		sample.mergeMany, sample.cells, sample.cellPairs, sample.cellWidth, sample.maxOperand,
		sample.regionsTotal, sample.regionsWidenFactorFree, sample.regionsPureTransport, sample.regionsLinearCandidate,
		sample.backEnvTerms, sample.backFactorTerms, sample.backGroupTerms,
		sample.regionInteriorPointsMax, sample.regionInteriorPointsTotal)
}

// TestZZProbeSolverLadderWalk walks the corpus serially and reports the
// fixtures whose solve costs the most, with the per-class operation volumes
// that cost is made of. ZZPROBE_SHARD selects a fixture-name prefix;
// ZZPROBE_TOP sets how many rows are printed (default 30).
func TestZZProbeSolverLadderWalk(t *testing.T) {
	prefix := os.Getenv("ZZPROBE_SHARD")
	projects := corpusHarnessProjects(t)
	selected := make([]corpusHarnessProject, 0, len(projects))
	for _, project := range projects {
		if prefix == "" || strings.HasPrefix(project.name, prefix) {
			selected = append(selected, project)
		}
	}
	if len(selected) == 0 {
		t.Fatalf("ZZPROBE_SHARD=%q selects no fixture", prefix)
	}
	top := 30
	if value, err := strconv.Atoi(os.Getenv("ZZPROBE_TOP")); err == nil && value > 0 {
		top = value
	}

	samples := make([]zzProbeSolverSample, 0, len(selected))
	for _, project := range selected {
		samples = append(samples, zzProbeSolverSampleOne(t, project))
	}

	var totals zzProbeSolverSample
	failures := 0
	for _, sample := range samples {
		if sample.err {
			failures++
			continue
		}
		totals.solve += sample.solve
		totals.compile += sample.compile
		totals.passes += sample.passes
		totals.refreshes += sample.refreshes
		totals.evaluates += sample.evaluates
		totals.folds += sample.folds
		totals.rhs += sample.rhs
		totals.restarts += sample.restarts
		totals.publications += sample.publications
		totals.bumps += sample.bumps
		totals.ifaceRefresh += sample.ifaceRefresh
		totals.dbgFoldTerms += sample.dbgFoldTerms
		totals.reuseAdmit += sample.reuseAdmit
		totals.reuseRefuse += sample.reuseRefuse
		totals.reuseTerms += sample.reuseTerms
		totals.rebuildTerm += sample.rebuildTerm
		totals.mergeMany += sample.mergeMany
		totals.cells += sample.cells
		totals.cellPairs += sample.cellPairs
		totals.regionsTotal += sample.regionsTotal
		totals.regionsWidenFactorFree += sample.regionsWidenFactorFree
		totals.regionsPureTransport += sample.regionsPureTransport
		totals.regionsLinearCandidate += sample.regionsLinearCandidate
		totals.backEnvTerms += sample.backEnvTerms
		totals.backFactorTerms += sample.backFactorTerms
		totals.backGroupTerms += sample.backGroupTerms
		totals.regionInteriorPointsTotal += sample.regionInteriorPointsTotal
		if sample.regionInteriorPointsMax > totals.regionInteriorPointsMax {
			totals.regionInteriorPointsMax = sample.regionInteriorPointsMax
		}
	}

	sort.SliceStable(samples, func(left, right int) bool { return samples[left].solve > samples[right].solve })
	t.Logf("ZZPROBE walk shard=%q fixtures=%d solved=%d failed=%d", prefix, len(samples), len(samples)-failures, failures)
	for index, sample := range samples {
		if index >= top {
			break
		}
		t.Logf("ZZPROBE row %s", zzProbeSolverLine(sample))
	}
	t.Logf("ZZPROBE totals solve=%s compile=%s passes=%d refreshes=%d evaluates=%d folds=%d rhs=%d restarts=%d pubs=%d bumps=%d iface=%d foldTerms=%d reuse=%d/%d reuseTerms=%d rebuildTerms=%d mergeMany=%d cells=%d cellPairs=%d "+
		"regions=%d widenFree=%d pureTransport=%d linearCandidate=%d back{env=%d factor=%d group=%d} interior{max=%d total=%d}",
		totals.solve.Round(time.Millisecond), totals.compile.Round(time.Millisecond), totals.passes, totals.refreshes, totals.evaluates,
		totals.folds, totals.rhs, totals.restarts, totals.publications, totals.bumps, totals.ifaceRefresh, totals.dbgFoldTerms,
		totals.reuseAdmit, totals.reuseRefuse, totals.reuseTerms, totals.rebuildTerm, totals.mergeMany, totals.cells, totals.cellPairs,
		totals.regionsTotal, totals.regionsWidenFactorFree, totals.regionsPureTransport, totals.regionsLinearCandidate,
		totals.backEnvTerms, totals.backFactorTerms, totals.backGroupTerms,
		totals.regionInteriorPointsMax, totals.regionInteriorPointsTotal)
}

// TestZZProbeSolverLadderFixture runs exactly one named fixture so the global
// counters, including the typprobe cell-pair distinctness map printed by the
// oracle TestMain, are attributable to it. Set ZZPROBE_FIXTURE.
func TestZZProbeSolverLadderFixture(t *testing.T) {
	name := os.Getenv("ZZPROBE_FIXTURE")
	if name == "" {
		t.Skip("set ZZPROBE_FIXTURE to a canonical fixture name")
	}
	sample := zzProbeSolverSampleOne(t, corpusHarnessFixture(t, name))
	t.Logf("ZZPROBE fixture %s", zzProbeSolverLine(sample))
}
