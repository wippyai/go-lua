package factapply

import (
	"fmt"
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestNarrowedRootDescendantFactsRestoreMatchesSequentialWrites(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	values := []product.Value{product.Bottom(reg), presentValue(reg), absentValue(reg), product.Top()}
	keys := make([]keyspace.Key, 24)
	for i := range keys {
		var ok bool
		keys[i], ok = ks.FromPathKey(pathdom.PathKey(fmt.Sprintf("sym71@1.member%d", i)))
		if !ok {
			t.Fatalf("intern test path %d", i)
		}
	}

	rng := rand.New(rand.NewSource(71))
	for trial := 0; trial < 300; trial++ {
		base := state.State{}
		if trial%3 == 0 {
			base = state.Domain(reg).Bottom()
		}
		for i := 0; i < rng.Intn(16); i++ {
			base = base.WriteLocalPathKey(reg, keys[rng.Intn(len(keys))], values[rng.Intn(len(values))])
			base = base.WriteLocalPathStaticMember(keys[rng.Intn(len(keys))], values[rng.Intn(len(values))])
		}

		facts := narrowedRootDescendantFacts{reg: reg}
		for i := 0; i < rng.Intn(48); i++ {
			fact := narrowedRootDescendantFact{key: keys[rng.Intn(len(keys))], value: values[rng.Intn(len(values))]}
			if rng.Intn(2) == 0 {
				facts.pathFacts = append(facts.pathFacts, fact)
			} else {
				facts.staticMembers = append(facts.staticMembers, fact)
			}
		}

		want := restoreNarrowedRootDescendantFactsSequential(facts, base)
		got := facts.Restore(base)
		if !state.Domain(reg).Equal(got, want) {
			t.Fatalf("trial %d: batched restore differs from sequential restore", trial)
		}
	}
}

func TestNarrowedRootDescendantFactsRestoreBatchesMapClone(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	const baseCount = 64
	const restoreCount = 64
	base := state.State{}
	for i := 0; i < baseCount; i++ {
		key, ok := ks.FromPathKey(pathdom.PathKey(fmt.Sprintf("sym72@1.base%d", i)))
		if !ok {
			t.Fatalf("intern base path %d", i)
		}
		base = base.WriteLocalPathStaticMember(key, presentValue(reg))
	}
	facts := narrowedRootDescendantFacts{reg: reg}
	for i := 0; i < restoreCount; i++ {
		key, ok := ks.FromPathKey(pathdom.PathKey(fmt.Sprintf("sym72@1.restore%d", i)))
		if !ok {
			t.Fatalf("intern restore path %d", i)
		}
		facts.staticMembers = append(facts.staticMembers, narrowedRootDescendantFact{key: key, value: absentValue(reg)})
	}

	batched := testing.AllocsPerRun(20, func() { _ = facts.Restore(base) })
	sequential := testing.AllocsPerRun(20, func() { _ = restoreNarrowedRootDescendantFactsSequential(facts, base) })
	t.Logf("allocations/run: batched %.1f, sequential %.1f", batched, sequential)
	if batched*8 >= sequential {
		t.Fatalf("batched restore allocations/run = %.1f, sequential = %.1f; want at least 8x fewer", batched, sequential)
	}
}

func restoreNarrowedRootDescendantFactsSequential(facts narrowedRootDescendantFacts, out state.State) state.State {
	for _, fact := range facts.pathFacts {
		out = out.WriteLocalPathKey(facts.reg, fact.key, fact.value)
	}
	for _, fact := range facts.staticMembers {
		out = out.WriteLocalPathStaticMember(fact.key, fact.value)
	}
	return out
}
