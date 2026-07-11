package program

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
)

// SolvePhase identifies the fixed-point phase which solved a body.
type SolvePhase string

const (
	SolvePhasePrepass     SolvePhase = "prepass"
	SolvePhaseSummary     SolvePhase = "summary"
	SolvePhaseMaterialize SolvePhase = "materialize"
)

// BodySolveAttribution reports work for one body/function/context/phase tuple.
// BodyID is the stable function-body identity. Context is false for the base
// function summary and true for a call-context specialization.
type BodySolveAttribution struct {
	BodyID                         uint64
	Function                       summary.SummaryKey
	Phase                          SolvePhase
	Context                        bool
	BodySolves                     int
	PointTransfers                 int
	DependencyChangeResolves       int
	DependencyChangePointTransfers int
}

type bodySolveAttributionKey struct {
	bodyID   uint64
	function summary.SummaryKey
	phase    SolvePhase
	context  bool
}

type solveAttribution struct {
	stats            *Stats
	key              bodySolveAttributionKey
	dependencyChange bool
}

func newSolveAttribution(stats *Stats, bodyID uint64, function summary.SummaryKey, phase SolvePhase, context bool) *solveAttribution {
	if stats == nil {
		return nil
	}
	return &solveAttribution{stats: stats, key: bodySolveAttributionKey{bodyID: bodyID, function: function, phase: phase, context: context}}
}

func solveAttributionFor(stats *Stats, prepared *body.Static, function summary.SummaryKey, phase SolvePhase, context bool) *solveAttribution {
	if stats == nil || prepared == nil {
		return nil
	}
	return newSolveAttribution(stats, prepared.IdentityDigest(), function, phase, context)
}

func (a *solveAttribution) afterDependencyChange() *solveAttribution {
	if a == nil {
		return nil
	}
	copy := *a
	copy.dependencyChange = true
	return &copy
}

func (s *Stats) recordBodySolve(a *solveAttribution, pointTransfers int) {
	if s == nil || a == nil {
		return
	}
	if s.bodySolveAttribution == nil {
		s.bodySolveAttribution = make(map[bodySolveAttributionKey]BodySolveAttribution)
	}
	entry := s.bodySolveAttribution[a.key]
	if entry.BodyID == 0 {
		entry.BodyID = a.key.bodyID
		entry.Function = a.key.function
		entry.Phase = a.key.phase
		entry.Context = a.key.context
	}
	entry.BodySolves++
	entry.PointTransfers += pointTransfers
	if a.dependencyChange {
		entry.DependencyChangeResolves++
		entry.DependencyChangePointTransfers += pointTransfers
	}
	s.bodySolveAttribution[a.key] = entry
}

// BodySolveAttribution returns a stable snapshot of per-body solve work.
func (s *Stats) BodySolveAttribution() []BodySolveAttribution {
	if s == nil || len(s.bodySolveAttribution) == 0 {
		return nil
	}
	out := make([]BodySolveAttribution, 0, len(s.bodySolveAttribution))
	for _, entry := range s.bodySolveAttribution {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Function != out[j].Function {
			return out[i].Function.Less(out[j].Function)
		}
		if out[i].Context != out[j].Context {
			return !out[i].Context
		}
		if out[i].Phase != out[j].Phase {
			return out[i].Phase < out[j].Phase
		}
		return out[i].BodyID < out[j].BodyID
	})
	return out
}

func queryStats(stats *Stats) *query.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Query
}

func prepassCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.PrepassBodySolves
}

func summaryCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.SummaryBodySolves
}

func summaryPointTransferCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.SummaryPointTransfers
}

func summaryDependencyChangeCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.SummaryBodySolvesAfterDependencyChange
}

func summaryDependencyChangePointTransferCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.SummaryPointTransfersAfterDependencyChange
}

func materializeCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.MaterializeBodySolves
}

func summaryCacheHitCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.SummaryCacheHits
}

func summaryCacheMissCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.SummaryCacheMisses
}

func recordProgramShape(stats *Stats, keys programKeys) {
	if stats == nil {
		return
	}
	recordMaxInt(&stats.MaxFunctionCount, len(keys.functions))
	recordMaxInt(&stats.MaxContextCount, keys.contexts.Len())
	recordMaxInt(&stats.MaxCallContextRefCount, keys.contexts.CallRefCount())
	recordMaxInt(&stats.MaxSemanticCallContextCount, keys.contexts.SemanticCallContextCount())
	for sites, variants := range keys.contexts.CallSiteHistogram() {
		recordMaxInt(&stats.MaxSitesPerSemanticEntry, sites)
		if stats.CallSitesPerSemanticEntry == nil {
			stats.CallSitesPerSemanticEntry = make(map[int]int)
		}
		if variants > stats.CallSitesPerSemanticEntry[sites] {
			stats.CallSitesPerSemanticEntry[sites] = variants
		}
	}
}

func recordMaxInt(dst *int, value int) {
	if dst != nil && value > *dst {
		*dst = value
	}
}
