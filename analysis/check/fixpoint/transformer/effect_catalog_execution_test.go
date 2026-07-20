package transformer

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func assertEffectChangedLanesDeclared(t *testing.T, reg *axis.Registry, kind EffectKind, before, after state.State) {
	t.Helper()
	descriptor, ok := DefaultEffectCatalog().Descriptor(kind)
	if !ok {
		t.Fatalf("effect %d has no descriptor", kind)
	}
	domain := state.RegisteredProductDomain(reg)
	left, err := domain.Decompose(before)
	if err != nil {
		t.Fatal(err)
	}
	right, err := domain.Decompose(after)
	if err != nil {
		t.Fatal(err)
	}
	for index := range left {
		equal, err := domain.LaneEqual(left[index], right[index])
		if err != nil {
			t.Fatal(err)
		}
		if equal {
			continue
		}
		lane := left[index].Lane().ID()
		use := descriptor.LaneUse(lane)
		if use != LaneUseWrite && use != LaneUseReadWrite {
			t.Errorf("effect %d changed lane %q declared as %d", kind, lane, use)
		}
	}
}

func TestEffectCatalogAllocationMatchesConcreteLaneExecution(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	id := identity.ID{Kind: "effect-catalog", Site: "allocation", Index: 1}
	root := product.Set(reg, product.Top(), identity.Key, identity.Singleton(id))
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root})
	transaction, err := factapply.NewAllocationTemplateTransaction(reg, factapply.AllocationTemplateMaterialization{
		Result: root, Objects: map[identity.ID]heapidentity.TableObject{id: object},
		Placements: map[identity.ID]placement.Value{id: placement.Stack}, KeySpace: keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	marker := key.SymbolValue(symbol.ID(91001))
	before := state.Reachable(state.State{}).
		WriteValue(reg, marker, product.Top()).
		WritePlacement(id, placement.OwnedHeap)
	after, err := factapply.ApplyAllocationTemplateTransaction(context.Background(), reg, transaction, before)
	if err != nil {
		t.Fatal(err)
	}
	assertEffectChangedLanesDeclared(t, reg, EffectAllocationTemplate, before, after)
	if !product.Equal(reg, before.ReadValue(reg, marker), after.ReadValue(reg, marker)) {
		t.Fatal("allocation heap/fresh effect mutated the Values lane")
	}
	if after.ReadPlacement(id) != placement.OwnedHeap {
		t.Fatal("allocation effect failed to join the pre-existing placement lane")
	}
}

func TestEffectCatalogObjectMaterializationMatchesConcreteLaneExecution(t *testing.T) {
	reg := standard.Registry()
	resolver := visibility.NewResolver(visibility.NewBuilder().Build())
	authority := factapply.NewPathSemanticAuthority(resolver, nil, nil)
	id := identity.ID{Kind: "effect-catalog", Site: "object", Index: 1}
	root := product.Set(reg, product.Top(), identity.Key, identity.Singleton(id))
	object := factapply.ResolvedPathStoreObject{Heaps: []factapply.ResolvedPathStoreHeapObject{{Root: root}}}
	marker := key.SymbolValue(symbol.ID(91002))
	before := state.Reachable(state.State{}).
		WriteValue(reg, marker, product.Top()).
		WritePlacement(id, placement.SharedHeap)
	after, err := authority.ApplyResolvedObjectMaterialization(context.Background(), reg, object, before)
	if err != nil {
		t.Fatal(err)
	}
	assertEffectChangedLanesDeclared(t, reg, EffectObjectMaterialization, before, after)
	if !product.Equal(reg, before.ReadValue(reg, marker), after.ReadValue(reg, marker)) {
		t.Fatal("object materialization mutated its term-source Values lane")
	}
	if after.ReadPlacement(id) != placement.SharedHeap {
		t.Fatal("object materialization failed to join the pre-existing placement lane")
	}
}

func TestEffectCatalogBoundaryIndexIncludesEffectDeltaExecution(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	table := symbol.ID(91003)
	tablePath := pathdom.NewPath(table, "boundary-table")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 91004, HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 91005, HasExpr: true}
	write := factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, keySource, nil), valueSource,
		dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue)
	invalidation := factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(write.TargetRef())
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexWrites:          map[cfg.Point]factflow.DynamicIndexWrite{point: write},
		PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{point: invalidation},
	})
	resolver := visibility.NewResolver(visibility.NewBuilder().Build())
	root := resolver.KeySpace().FromPath(tablePath.RootOnly())
	address, err := factapply.FreezeBoundaryPathAddress(resolver.KeySpace(), root, tablePath)
	if err != nil {
		t.Fatal(err)
	}
	before := state.Reachable(state.State{})
	after, err := factapply.NewPathSemanticAuthority(resolver, nil, nil).ApplyBoundaryIndexMutation(
		context.Background(), reg, graph, facts, point, product.Top(), product.Top(),
		before, before, address, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEffectChangedLanesDeclared(t, reg, EffectIndexMutation, before, after)
	domain := state.DomainWithLanes(reg, []state.LaneID{state.LaneEffectDeltas})
	if domain.Equal(before, after) {
		t.Fatal("boundary index mutation did not exercise its EffectDeltas lane")
	}
}
