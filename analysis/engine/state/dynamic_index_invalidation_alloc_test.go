package state

import (
	"runtime"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDynamicIndexUnmatchedSubtreeInvalidationDoesNotCloneLane(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	first := ks.FromPath(pathdom.NewPath(symbol.ID(1), "first"))
	second := ks.FromPath(pathdom.NewPath(symbol.ID(2), "second"))
	unmatched := ks.FromPath(pathdom.NewPath(symbol.ID(3), "unmatched"))
	fact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		Value: presentValue(reg), HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
	})
	input := State{}.
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: first, Site: "first"}, fact).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: second, Site: "second"}, fact)
	unmatchedPath := ks.Format(unmatched)

	if allocs := testing.AllocsPerRun(100, func() {
		got := input.ClearDynamicIndexFactsForPathKeySubtree(ks, unmatchedPath)
		runtime.KeepAlive(got)
	}); allocs != 0 {
		t.Fatalf("unmatched invalidation allocations = %g, want zero", allocs)
	}
}
