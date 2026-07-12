package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type freshHeapProjectionResult struct {
	reg   *axis.Registry
	graph cfg.Graph
	entry state.State
	ks    *keyspace.KeySpace
}

func (r freshHeapProjectionResult) Registry() *axis.Registry        { return r.reg }
func (r freshHeapProjectionResult) Graph() cfg.Graph                { return r.graph }
func (r freshHeapProjectionResult) ExitState() (state.State, bool)  { return state.State{}, true }
func (r freshHeapProjectionResult) ReturnPoints() []cfg.Point       { return nil }
func (r freshHeapProjectionResult) KeySpace() *keyspace.KeySpace    { return r.ks }
func (r freshHeapProjectionResult) EntryState() (state.State, bool) { return r.entry, true }

func TestProjectFreshHeapAllocationsUsesEntryProvenanceAcrossGraph(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	rootID := identity.ID{Kind: "table", Site: "local", Index: 1}
	childID := identity.ID{Kind: "table", Site: "local", Index: 2}
	boundID := identity.ID{Kind: "table", Site: "parameter", Index: 3}
	value := func(id identity.ID) product.Value {
		return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	}
	rootValue, childValue, boundValue := value(rootID), value(childID), value(boundID)
	childKey, _ := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	parentKey, _ := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "parent"}})
	boundKey, _ := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "bound"}})
	objects := map[identity.ID]heapidentity.TableObject{
		rootID:  heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue, StaticMembers: map[keyspace.Key]product.Value{childKey: childValue, boundKey: boundValue}}),
		childID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: childValue, StaticMembers: map[keyspace.Key]product.Value{parentKey: rootValue}}),
		boundID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: boundValue}),
	}
	entry := state.State{}.WriteHeapTableObject(reg, boundID, objects[boundID])
	got := projectFreshHeapAllocations(reg, freshHeapProjectionResult{reg: reg, graph: cfg.New(), entry: entry, ks: ks}, objects, []product.Value{rootValue})
	seen := make(map[identity.ID]bool, len(got))
	for _, id := range got {
		seen[id] = true
	}
	if !seen[rootID] || !seen[childID] || len(seen) != 2 {
		t.Fatalf("fresh allocations = %#v, want root+child only", got)
	}
	if seen[boundID] {
		t.Fatal("entry-bound parameter/capture/global identity was marked fresh")
	}
}
