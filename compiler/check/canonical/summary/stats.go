package summary

// Stats is a run-scoped counter set for canonical solve, snapshot, diagnostic,
// and call-entry cache observability. It deliberately stores counts only; the
// canonical key set remains owned by Queries.demanded.
type Stats struct {
	uniqueSummaryKeyDemands       int
	summaryKeyDemandsByRef        map[FuncRef]int
	summaryKeyDemandFamiliesByRef map[FuncRef]SummaryKeyFamilyCounts
	summarizeWithKeyCalls         int
	intraObserverCalls            int
	observeIntraWithKeyCalls      int
	snapshotExactKeyHits          int
	snapshotExactKeyMisses        int
	diagnosticObservedStates      int
	callEntryProjectionRuns       int
	nestedCallProductCacheHits    int
	nestedCallProductCacheMisses  int
}

type SummaryKeyFamilyCounts struct {
	Default          int
	WithValues       int
	WithReferences   int
	WithFacts        int
	WithMultipleAxes int
}

type StatsSnapshot struct {
	UniqueSummaryKeyDemands       int
	SummaryKeyDemandsByRef        map[FuncRef]int
	SummaryKeyDemandFamiliesByRef map[FuncRef]SummaryKeyFamilyCounts
	SummarizeWithKeyCalls         int
	IntraObserverCalls            int
	ObserveIntraWithKeyCalls      int
	SnapshotExactKeyHits          int
	SnapshotExactKeyMisses        int
	DiagnosticObservedStates      int
	CallEntryProjectionRuns       int
	NestedCallProductCacheHits    int
	NestedCallProductCacheMisses  int
}

func NewStats() *Stats {
	return &Stats{}
}

func (s *Stats) Snapshot() StatsSnapshot {
	if s == nil {
		return StatsSnapshot{}
	}
	return StatsSnapshot{
		UniqueSummaryKeyDemands:       s.uniqueSummaryKeyDemands,
		SummaryKeyDemandsByRef:        cloneIntByRef(s.summaryKeyDemandsByRef),
		SummaryKeyDemandFamiliesByRef: cloneFamilyCountsByRef(s.summaryKeyDemandFamiliesByRef),
		SummarizeWithKeyCalls:         s.summarizeWithKeyCalls,
		IntraObserverCalls:            s.intraObserverCalls,
		ObserveIntraWithKeyCalls:      s.observeIntraWithKeyCalls,
		SnapshotExactKeyHits:          s.snapshotExactKeyHits,
		SnapshotExactKeyMisses:        s.snapshotExactKeyMisses,
		DiagnosticObservedStates:      s.diagnosticObservedStates,
		CallEntryProjectionRuns:       s.callEntryProjectionRuns,
		NestedCallProductCacheHits:    s.nestedCallProductCacheHits,
		NestedCallProductCacheMisses:  s.nestedCallProductCacheMisses,
	}
}

func (s *Stats) RecordSummaryKeyDemand(key Key, newlyDemanded bool) {
	if s == nil || !newlyDemanded {
		return
	}
	s.uniqueSummaryKeyDemands++
	if s.summaryKeyDemandsByRef == nil {
		s.summaryKeyDemandsByRef = make(map[FuncRef]int)
	}
	s.summaryKeyDemandsByRef[key.Ref]++
	if s.summaryKeyDemandFamiliesByRef == nil {
		s.summaryKeyDemandFamiliesByRef = make(map[FuncRef]SummaryKeyFamilyCounts)
	}
	counts := s.summaryKeyDemandFamiliesByRef[key.Ref]
	counts = incrementFamilyCounts(counts, key)
	s.summaryKeyDemandFamiliesByRef[key.Ref] = counts
}

func (s *Stats) RecordSummarizeWithKeyCall() {
	if s != nil {
		s.summarizeWithKeyCalls++
	}
}

func (s *Stats) RecordIntraObserverCall() {
	if s != nil {
		s.intraObserverCalls++
	}
}

func (s *Stats) RecordObserveIntraWithKeyCall() {
	if s != nil {
		s.observeIntraWithKeyCalls++
	}
}

func (s *Stats) RecordSnapshotExactKeyRead(hit bool) {
	if s == nil {
		return
	}
	if hit {
		s.snapshotExactKeyHits++
	} else {
		s.snapshotExactKeyMisses++
	}
}

func (s *Stats) RecordDiagnosticObservedState() {
	if s != nil {
		s.diagnosticObservedStates++
	}
}

func (s *Stats) RecordCallEntryProjectionRun() {
	if s != nil {
		s.callEntryProjectionRuns++
	}
}

func (s *Stats) RecordNestedCallProductCacheHit() {
	if s != nil {
		s.nestedCallProductCacheHits++
	}
}

func (s *Stats) RecordNestedCallProductCacheMiss() {
	if s != nil {
		s.nestedCallProductCacheMisses++
	}
}

func incrementFamilyCounts(counts SummaryKeyFamilyCounts, key Key) SummaryKeyFamilyCounts {
	axes := 0
	if key.Values.n != nil {
		axes++
	}
	defaultKey := NewDefaultKey(key.Ref, nil)
	if key.References != defaultKey.References {
		axes++
	}
	if key.Facts.n != nil {
		axes++
	}
	switch axes {
	case 0:
		counts.Default++
	case 1:
		if key.Values.n != nil {
			counts.WithValues++
		} else if key.References != defaultKey.References {
			counts.WithReferences++
		} else {
			counts.WithFacts++
		}
	default:
		counts.WithMultipleAxes++
	}
	return counts
}

func cloneIntByRef(in map[FuncRef]int) map[FuncRef]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[FuncRef]int, len(in))
	for ref, count := range in {
		out[ref] = count
	}
	return out
}

func cloneFamilyCountsByRef(in map[FuncRef]SummaryKeyFamilyCounts) map[FuncRef]SummaryKeyFamilyCounts {
	if len(in) == 0 {
		return nil
	}
	out := make(map[FuncRef]SummaryKeyFamilyCounts, len(in))
	for ref, counts := range in {
		out[ref] = counts
	}
	return out
}
