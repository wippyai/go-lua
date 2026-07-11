package program

import (
	"fmt"
	"hash/fnv"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// semanticEntryKey is the content address of the entry a callee can observe.
// It deliberately has no caller or expression identity: those belong to the
// call-site routing map in contextIndex.  Any digest collision is checked with
// state.Domain(reg).Equal before a variant is reused.
func semanticEntryKey(reg *axis.Registry, entry state.State, keys *keyspace.KeySpace) summary.EntryKey {
	if reg == nil {
		return summary.EntryKey{}
	}
	return summary.EntryKey{
		Values:     digestEntryValues(reg, entry),
		Facts:      digestEntryFacts(reg, entry, keys),
		References: digestEntryHeap(reg, entry, keys),
	}
}

func digestEntryValues(reg *axis.Registry, entry state.State) summary.Digest {
	h := fnv.New64a()
	fmt.Fprint(h, "entry-values-v1;")
	snapshot := entry.ValuesSnapshot()
	fmt.Fprintf(h, "top:%t;", snapshot.Top)
	slots := make([]uint64, 0, len(snapshot.Values))
	for slot := range snapshot.Values {
		slots = append(slots, uint64(slot))
	}
	slices.Sort(slots)
	for _, slot := range slots {
		fmt.Fprintf(h, "%d=%d;", slot, product.Hash(reg, snapshot.Values[statekey.Value(slot)]))
	}
	return summary.Digest(h.Sum64())
}

func digestEntryFacts(reg *axis.Registry, entry state.State, keys *keyspace.KeySpace) summary.Digest {
	h := fnv.New64a()
	fmt.Fprint(h, "entry-facts-v1;")
	write := func(kind string, bottom, top bool, values map[string]product.Value) {
		fmt.Fprintf(h, "%s:bottom:%t;top:%t;", kind, bottom, top)
		paths := make([]string, 0, len(values))
		for path := range values {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			fmt.Fprintf(h, "%s=%d;", path, product.Hash(reg, values[path]))
		}
	}
	refinements := entry.PathRefinementsSnapshot(keys)
	refinementValues := make(map[string]product.Value, len(refinements.Refinements))
	for path, value := range refinements.Refinements {
		refinementValues[string(path)] = value
	}
	write("refinement", refinements.Bottom, refinements.Top, refinementValues)
	members := entry.PathStaticMembersSnapshot(keys)
	memberValues := make(map[string]product.Value, len(members.Members))
	for path, value := range members.Members {
		memberValues[string(path)] = value
	}
	write("member", members.Bottom, members.Top, memberValues)
	return summary.Digest(h.Sum64())
}

func digestEntryHeap(reg *axis.Registry, entry state.State, keys *keyspace.KeySpace) summary.Digest {
	snapshot := entry.HeapTableObjectsSnapshot()
	if snapshot.Top {
		return summary.Digest(1)
	}
	return summary.NormalizedPayloadDigest(reg, summary.Summary{
		HeapTableObjects: snapshot.Objects,
		HeapKeySpace:     keys,
	})
}
