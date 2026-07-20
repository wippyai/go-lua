package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func normalReturnDynamicIndexMutationTables(facts []callboundary.DynamicIndexFact) []pathdom.Path {
	if len(facts) == 0 {
		return nil
	}
	out := make([]pathdom.Path, 0, len(facts))
	for _, fact := range facts {
		if fact.Table.IsEmpty() {
			continue
		}
		out = append(out, fact.Table)
	}
	return out
}

func normalReturnPathMatchesAny(target pathdom.Path, candidates []pathdom.Path) bool {
	for _, candidate := range candidates {
		if target.Equal(candidate) {
			return true
		}
	}
	return false
}
