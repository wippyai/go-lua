package program

import (
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

// A graph ID identifies one in-memory CFG instance. Rebuilding identical
// source must not let that process-order token leak into summaries that can be
// reused as artifacts by another analysis or compiler invocation.
func TestColdBuildTableIdentitiesAndSummariesAreStable(t *testing.T) {
	const source = `
local function make_pair()
	return { left = 1 }, { right = 2 }
end

local left, right = make_pair()
return left, right
`

	reg := standard.Registry()
	first := runArtifactIdentityChunk(t, reg, source)
	firstGraphID := first.RootResult().Graph().ID()

	// Make reliance on the package-global CFG allocation order observable even
	// if the body construction count later happens to change.
	for range 7 {
		_ = cfg.New()
	}

	second := runArtifactIdentityChunk(t, reg, source)
	secondGraphID := second.RootResult().Graph().ID()
	if firstGraphID == secondGraphID {
		t.Fatalf("independent runs reused graph ID %d; test cannot detect graph-order leakage", firstGraphID)
	}

	firstIDs := literalTableIdentities(first.Snapshot(), reg)
	secondIDs := literalTableIdentities(second.Snapshot(), reg)
	if len(firstIDs) != 2 || len(secondIDs) != 2 {
		t.Fatalf("literal table identity census = %v and %v, want two distinct lexical sites per run", firstIDs, secondIDs)
	}
	if firstIDs[0] == firstIDs[1] || secondIDs[0] == secondIDs[1] {
		t.Fatalf("distinct table literal sites collapsed: first=%v second=%v", firstIDs, secondIDs)
	}
	if !equalIdentitySlices(firstIDs, secondIDs) {
		t.Errorf("same lexical table sites changed identity across cold builds: first=%v second=%v (graph IDs %d and %d)", firstIDs, secondIDs, firstGraphID, secondGraphID)
	}

	firstEntries := first.Snapshot().EntriesOwnedNormalized()
	secondEntries := second.Snapshot().EntriesOwnedNormalized()
	if len(firstEntries) != len(secondEntries) {
		t.Fatalf("normalized summary entry count changed across cold builds: first=%d second=%d", len(firstEntries), len(secondEntries))
	}
	for i := range firstEntries {
		left, right := firstEntries[i], secondEntries[i]
		if left.Key != right.Key {
			t.Errorf("normalized summary key %d changed across cold builds: first=%v second=%v", i, left.Key, right.Key)
			continue
		}
		if !summary.EqualNormalized(reg, left.Summary, right.Summary) {
			t.Errorf("raw normalized summary %v changed across cold builds: first digest=%d second digest=%d", left.Key,
				summary.NormalizedPayloadDigest(reg, left.Summary), summary.NormalizedPayloadDigest(reg, right.Summary))
		}
	}
}

func runArtifactIdentityChunk(t *testing.T, reg *axis.Registry, source string) Result {
	t.Helper()
	stmts, err := parse.ParseString(source, "artifact_identity_stability.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := RunChunk(stmts, Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	return result
}

func literalTableIdentities(snapshot summary.Snapshot, reg *axis.Registry) []identity.ID {
	seen := make(map[identity.ID]struct{})
	for _, entry := range snapshot.EntriesOwnedNormalized() {
		for id := range entry.Summary.HeapTableObjects {
			if isLexicalTableIdentity(id) {
				seen[id] = struct{}{}
			}
		}
		for _, allocation := range entry.Summary.FreshHeapAllocations {
			id := allocation.ID
			if isLexicalTableIdentity(id) {
				seen[id] = struct{}{}
			}
		}
		for _, value := range entry.Summary.Returns {
			if id, ok := product.Get(reg, value, identity.Key).ID(); ok && isLexicalTableIdentity(id) {
				seen[id] = struct{}{}
			}
		}
	}
	out := make([]identity.ID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		return out[i].Index < out[j].Index
	})
	return out
}

func isLexicalTableIdentity(id identity.ID) bool {
	return id.Kind == "lua.table" && strings.HasPrefix(id.Site, "lexical-body-expr-v2:")
}

func equalIdentitySlices(left, right []identity.ID) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
