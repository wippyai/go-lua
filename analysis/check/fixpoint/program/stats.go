package program

import "github.com/wippyai/go-lua/analysis/check/fixpoint/query"

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
