package program

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestInactiveRelationSnapshotFreezesDeterministicAcyclicChain(t *testing.T) {
	var runs [][]transformer.CellRef
	for i := 0; i < 2; i++ {
		catalog := syntheticAcyclicChainCatalog(t)
		snapshot, err := catalog.Freeze(context.Background())
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		entries := snapshot.Entries()
		if len(entries) != 3 {
			t.Fatalf("snapshot entries = %d, want 3", len(entries))
		}
		refs := make([]transformer.CellRef, len(entries))
		for j, identity := range entries {
			refs[j] = identity.Cell
			relation, ok := snapshot.Lookup(identity)
			if !ok || relation.ContextualReason() != "" || relation.Widened() || relation.Rows() == 0 {
				t.Fatalf("entry %d was not frozen exact", j)
			}
		}
		runs = append(runs, refs)
	}
	if fmt.Sprint(runs[0]) != fmt.Sprint(runs[1]) {
		t.Fatalf("snapshot order drifted: %#v vs %#v", runs[0], runs[1])
	}
}

func TestInactiveRelationSnapshotAdmitsExactCyclicProducer(t *testing.T) {
	catalog := syntheticExactCyclicCatalog(t)
	snapshot, err := catalog.Freeze(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entries := snapshot.Entries()
	if len(entries) != 1 {
		t.Fatalf("frozen cyclic producers = %d, want 1", len(entries))
	}
	relation, ok := snapshot.Lookup(entries[0])
	if !ok || relation.Rows() == 0 || relation.ContextualReason() != "" || relation.Widened() {
		t.Fatal("exact cyclic producer was not frozen")
	}
}

func TestInactiveRelationSnapshotExcludesRecursionAndEffects(t *testing.T) {
	stmts := parseChunk(t, `
local function exact(x: any): any return x end
local function recursive(x: any): any return recursive(x) end
local function effectful(): any return table.create(1) end
return exact("ok")
`)
	config := Config{Check: body.Config{Registry: standard.Registry()}}
	config.relationSnapshotAudit = func(snapshot relationRunSnapshot) error {
		entries := snapshot.Entries()
		if len(entries) != 1 {
			return fmt.Errorf("frozen producers = %d, want exact only: %#v", len(entries), entries)
		}
		relation, ok := snapshot.Lookup(entries[0])
		if !ok || relation.Rows() == 0 || relation.ContextualReason() != "" || relation.Widened() {
			return fmt.Errorf("exact producer was not frozen")
		}
		return nil
	}
	if _, err := RunChunk(stmts, config); err != nil {
		t.Fatal(err)
	}
}

func TestInactiveRelationSnapshotCancellationPublishesZero(t *testing.T) {
	catalog := captureRelationCatalog(t, `local function leaf(x: any): any return x end return leaf("ok")`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot, err := catalog.Freeze(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Freeze error = %v, want context.Canceled", err)
	}
	if len(snapshot.Entries()) != 0 {
		t.Fatalf("canceled freeze published %#v", snapshot.Entries())
	}
}

func TestInactiveRelationSnapshotRejectsContextualCellWithoutPartialPublication(t *testing.T) {
	catalog := syntheticAcyclicChainCatalog(t)
	graph := cfg.New()
	head := graph.AddNode(cfg.NodeBranch)
	graph.AddEdge(graph.Entry(), head, false)
	graph.AddEdge(head, head, true)
	graph.AddEdge(head, graph.Exit(), false)
	plan := operationplan.New(graph, factflow.FactsInput{})
	compiler, err := transformer.NewPlanCompiler().Prepare(standard.Registry(), graph, plan, transformer.Shape{})
	if err != nil {
		t.Fatal(err)
	}
	ref := transformer.CellRef{Function: 4}
	direct, _ := transformer.NewDirectCallCatalog(plan.PointCount(), nil)
	catalog.entries = append(catalog.entries, relationCatalogEntry{
		identity: relationCellIdentity{Cell: ref, BodyDigest: 4, Generation: catalog.generation},
		compiler: compiler, direct: direct,
	})
	snapshot, err := catalog.Freeze(context.Background())
	var rejected relationFreezeError
	if !errors.As(err, &rejected) || rejected.Category != relationFreezeContextual {
		t.Fatalf("Freeze error = %v, want contextual category", err)
	}
	if len(snapshot.Entries()) != 0 {
		t.Fatalf("failed transaction published %#v", snapshot.Entries())
	}
}

func TestInactiveRelationSnapshotRejectsProducerAndGenerationMismatch(t *testing.T) {
	catalog := captureRelationCatalog(t, `local function leaf(x: any): any return x end return leaf("ok")`)
	snapshot, err := catalog.Freeze(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identity := snapshot.Entries()[0]
	for name, mutate := range map[string]func(*relationCellIdentity){
		"cell":       func(id *relationCellIdentity) { id.Cell.Function++ },
		"summary":    func(id *relationCellIdentity) { id.Summary.Entry.Values++ },
		"digest":     func(id *relationCellIdentity) { id.BodyDigest++ },
		"prepared":   func(id *relationCellIdentity) { id.Prepared = nil },
		"generation": func(id *relationCellIdentity) { id.Generation = &relationCatalogGeneration{} },
	} {
		mismatch := identity
		mutate(&mismatch)
		if _, ok := snapshot.Lookup(mismatch); ok {
			t.Fatalf("%s mismatch read frozen relation", name)
		}
	}
	owner := catalog.ConsumerPolicy().Owners()[0]
	if _, ok := snapshot.DirectCalls(owner); !ok {
		t.Fatal("same-generation consumer was rejected")
	}
	owner.Generation = &relationCatalogGeneration{}
	if _, ok := snapshot.DirectCalls(owner); ok {
		t.Fatal("foreign-generation consumer read frozen routes")
	}
}

func TestInactiveRelationSnapshotConcurrentFreezeIsRaceSafe(t *testing.T) {
	catalog := captureRelationCatalog(t, `local function leaf(x: any): any return x end return leaf("ok")`)
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	counts := make(chan int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot, err := catalog.Freeze(context.Background())
			errs <- err
			counts <- len(snapshot.Entries())
		}()
	}
	wg.Wait()
	close(errs)
	close(counts)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for count := range counts {
		if count != 1 {
			t.Fatalf("snapshot entries = %d, want 1", count)
		}
	}
}

func captureRelationCatalog(t *testing.T, source string) relationRunCatalog {
	t.Helper()
	stmts := parseChunk(t, source)
	config := Config{Check: body.Config{Registry: standard.Registry()}}
	var catalog relationRunCatalog
	config.relationCatalogAudit = func(got relationRunCatalog) error {
		catalog = got
		return nil
	}
	if _, err := RunChunk(stmts, config); err != nil {
		t.Fatal(err)
	}
	if catalog.generation == nil {
		t.Fatal("catalog was not captured")
	}
	return catalog
}

func syntheticAcyclicChainCatalog(t *testing.T) relationRunCatalog {
	t.Helper()
	reg := standard.Registry()
	shape := transformer.Shape{Params: 1}
	param := symbol.ID(1001)
	result := symbol.ID(1002)
	scalar, _ := factflow.NewValueSourceShape(true, false, false, false)
	paramSource, _ := factflow.NewPathValueSource(pathdom.NewPath(param, "value").Key(), 0, 0, 0, scalar)

	leafGraph := cfg.New()
	leafReturn := leafGraph.AddNode(cfg.NodeReturn)
	leafGraph.AddEdge(leafGraph.Entry(), leafReturn, false)
	leafGraph.AddEdge(leafReturn, leafGraph.Exit(), false)
	leafPlan := operationplan.New(leafGraph, factflow.FactsInput{
		Returns: map[cfg.Point]factflow.Return{leafReturn: factflow.NewReturn([]factflow.ValueSource{paramSource})},
	}).WithBoundaryParams([]symbol.ID{param})
	leaf, err := transformer.NewPlanCompiler().Prepare(reg, leafGraph, leafPlan, shape)
	if err != nil {
		t.Fatal(err)
	}

	callerGraph := cfg.New()
	call := callerGraph.AddNode(cfg.NodeCall)
	ret := callerGraph.AddNode(cfg.NodeReturn)
	callerGraph.AddEdge(callerGraph.Entry(), call, false)
	callerGraph.AddEdge(call, ret, false)
	callerGraph.AddEdge(ret, callerGraph.Exit(), false)
	resultSource, _ := factflow.NewPathValueSource(pathdom.NewPath(result, "result").Key(), 0, 0, 0, scalar)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: call, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{paramSource},
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(
			factflow.CallResultTargetLocalAssignment, 0, 0, result, pathdom.NewPath(result, "result"),
		)},
	})
	callerPlan := operationplan.New(callerGraph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{call: site},
		Returns:   map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{resultSource})},
	}).WithBoundaryParams([]symbol.ID{param})
	caller, err := transformer.NewPlanCompiler().Prepare(reg, callerGraph, callerPlan, shape)
	if err != nil {
		t.Fatal(err)
	}

	generation := &relationCatalogGeneration{}
	refs := []transformer.CellRef{{Function: 1}, {Function: 2}, {Function: 3}}
	leafDirect, _ := transformer.NewDirectCallCatalog(leafPlan.PointCount(), nil)
	middleDirect, err := transformer.NewDirectCallCatalog(callerPlan.PointCount(), map[cfg.Point]transformer.DirectCallTarget{
		call: {Cell: refs[0], Shape: shape},
	})
	if err != nil {
		t.Fatal(err)
	}
	outerDirect, err := transformer.NewDirectCallCatalog(callerPlan.PointCount(), map[cfg.Point]transformer.DirectCallTarget{
		call: {Cell: refs[1], Shape: shape},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := []relationCatalogEntry{
		{identity: relationCellIdentity{Cell: refs[0], BodyDigest: 1, Generation: generation}, compiler: leaf, direct: leafDirect},
		{identity: relationCellIdentity{Cell: refs[1], BodyDigest: 2, Generation: generation}, compiler: caller, direct: middleDirect},
		{identity: relationCellIdentity{Cell: refs[2], BodyDigest: 3, Generation: generation}, compiler: caller, direct: outerDirect},
	}
	return relationRunCatalog{
		entries: entries, generation: generation,
		consumers: relationConsumerPolicy{generation: generation},
	}
}

func syntheticExactCyclicCatalog(t *testing.T) relationRunCatalog {
	t.Helper()
	reg := standard.Registry()
	graph := cfg.New()
	iteratorCall := graph.AddNode(cfg.NodeCall)
	head := graph.AddNode(cfg.NodeBranch)
	genericPoint := graph.AddNode(cfg.NodeAssign)
	returnPoint := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), iteratorCall, false)
	graph.AddEdge(iteratorCall, head, false)
	graph.AddEdge(head, genericPoint, true)
	graph.AddEdge(head, returnPoint, false)
	graph.AddEdge(genericPoint, head, false)
	graph.AddEdge(returnPoint, graph.Exit(), false)

	container := symbol.ID(2001)
	projection := symbol.ID(2002)
	scalar, _ := factflow.NewValueSourceShape(true, false, false, false)
	containerSource, _ := factflow.NewPathValueSource(pathdom.NewPath(container, "items").Key(), 0, 0, 0, scalar)
	iteratorSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: iteratorCall, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{containerSource},
	})
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	generic, ok := operationplan.NewGenericForOperation(1, projection, projection-1, operationplan.GenericForSource{
		Kind: operationplan.GenericForSourceCall, CallPoint: iteratorCall, HasCallPoint: true,
	}, []typ.Type{typ.NewArray(typ.String)})
	if !ok {
		t.Fatal("generic-for operation rejected")
	}
	generic = generic.WithIterator(iter)
	sigOp, ok := operationplan.NewSignatureCallOperation(signature.Function{
		Type:   typ.Func().Param("source", typ.Any).Returns(typ.Any).Build(),
		Effect: effect.Row{Labels: []effect.Label{iter}},
	})
	if !ok {
		t.Fatal("iterator signature rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{iteratorCall: iteratorSite},
		Returns:   map[cfg.Point]factflow.Return{returnPoint: factflow.NewReturn([]factflow.ValueSource{containerSource})},
	}).WithBoundaryParams([]symbol.ID{container}).
		WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{iteratorCall: sigOp}).
		WithExtensions([]operationplan.ExtensionInput{{Point: genericPoint, Kind: operationplan.BodyGenericFor, GenericFor: generic}})
	compiler, err := transformer.NewPlanCompiler().Prepare(reg, graph, plan, transformer.Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	generation := &relationCatalogGeneration{}
	ref := transformer.CellRef{Function: 1}
	direct, _ := transformer.NewDirectCallCatalog(plan.PointCount(), nil)
	return relationRunCatalog{
		entries: []relationCatalogEntry{{
			identity: relationCellIdentity{Cell: ref, BodyDigest: 1, Generation: generation},
			compiler: compiler, direct: direct,
		}},
		generation: generation, consumers: relationConsumerPolicy{generation: generation},
	}
}
