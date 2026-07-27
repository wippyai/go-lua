package factapply

import (
	"context"
	"reflect"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestNormalReturnFactorEncoderMatchesStateBoundaryProjection(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := state.RegisteredProductDomain(reg)
	root := pathdom.Path{Root: "arg", Symbol: symbol.ID(7101), Version: 1}
	rootKey, childKey := keys.FromPath(root), keys.FromPath(root.Field("child"))
	id := identity.ID{Kind: "lua.table", Site: t.Name(), Index: 1}
	idValue := identityvalue.Present(reg, id)
	childValue := typevalue.LiteralString(reg, "child")
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: idValue})
	world := domain.Lattice().Bottom().
		WriteValue(reg, statekey.SymbolValue(root.Symbol), idValue).
		WriteLocalPathKey(reg, childKey, childValue).
		WriteHeapTableObject(reg, id, object).
		WritePlacement(id, placement.OwnedHeap)
	roots := state.BoundaryRoots{{Slot: statekey.SymbolValue(root.Symbol), Path: rootKey, Value: idValue}}

	wantWorld, wantRoots := projectedBoundaryWorld(t, reg, keys, world, roots)
	wantFacts, err := callboundary.NormalReturnFactsFromProjectedState(reg, keys, wantWorld, wantRoots, 1)
	if err != nil {
		t.Fatal(err)
	}
	selection := closedFactorSelection(t, domain, keys, world, roots)
	encoder, err := PrepareNormalReturnFactorEncoder[statekey.Value](domain, selection, 1)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(world, encoder.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := encoder.Encode(context.Background(), nil, factors, []NormalReturnFactorOperand[statekey.Value]{{
		Slot: roots[0].Slot, Value: roots[0].Value,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.NormalReturnFacts, wantFacts) {
		t.Fatalf("normal-return facts differ:\nfactor=%#v\nstate=%#v", got.NormalReturnFacts, wantFacts)
	}
	wantHeap := wantWorld.HeapTableObjectsSnapshot()
	if wantHeap.Top || !heapidentity.MapDomain(reg).Equal(got.HeapTableObjects, wantHeap.Objects) {
		t.Fatalf("heap projection differs: factor=%#v state=%#v", got.HeapTableObjects, wantHeap)
	}
	wantPlacement := wantWorld.PlacementsSnapshot()
	if wantPlacement.Top || !reflect.DeepEqual(got.Placements, wantPlacement.Placements) {
		t.Fatalf("placement projection differs: factor=%#v state=%#v", got.Placements, wantPlacement)
	}
}

func TestNormalReturnFactorEncoderOmitsDisabledOptionalLanes(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain, err := state.TryRegisteredProductDomainWithLanes(reg, []state.LaneID{state.LaneValues, state.LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	root := pathdom.Path{Root: "arg", Symbol: symbol.ID(7102), Version: 1}
	rootKey, childKey := keys.FromPath(root), keys.FromPath(root.Field("child"))
	value := typevalue.LiteralString(reg, "reduced")
	world := domain.Lattice().Bottom().WriteLocalPathKey(reg, childKey, value)
	roots := state.BoundaryRoots{{Slot: statekey.SymbolValue(root.Symbol), Path: rootKey, Value: product.Top()}}
	wantWorld, wantRoots := projectedBoundaryWorld(t, reg, keys, world, roots)
	wantFacts, err := callboundary.NormalReturnFactsFromProjectedState(reg, keys, wantWorld, wantRoots, 1)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := PrepareNormalReturnFactorEncoder[statekey.Value](domain, closedFactorSelection(t, domain, keys, world, roots), 1)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(world, encoder.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := encoder.Encode(context.Background(), nil, factors, []NormalReturnFactorOperand[statekey.Value]{{Slot: roots[0].Slot, Value: roots[0].Value}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.NormalReturnFacts, wantFacts) || len(got.HeapTableObjects) != 0 || len(got.Placements) != 0 {
		t.Fatalf("reduced projection differs: factor=%#v state=%#v", got, wantFacts)
	}
}

func TestNormalReturnFactorEncoderPublishesPersistentFormalInputsByBoundaryIdentity(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := state.RegisteredProductDomain(reg)
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	const paramCount = 1
	type persistentRoot struct {
		name string
		sym  symbol.ID
	}
	persistent := []persistentRoot{
		{name: "capture", sym: 7202},
		{name: "global", sym: 7203},
		{name: "ambient", sym: 7204},
	}
	roots := make(state.BoundaryRoots, 1, 1+len(persistent))
	paramKey, ok := keys.InternFormalRoot(formal.NewRoot(owner, 1, formal.Input))
	if !ok {
		t.Fatal("formal parameter root")
	}
	roots[0] = state.BoundaryRoot{Path: paramKey, Value: product.Top()}
	world := domain.Lattice().Bottom()
	for index, item := range persistent {
		root, rootOK := keys.InternFormalRoot(formal.NewRoot(owner, uint64(index+2), formal.Input))
		child, childOK := keys.AppendSegment(root, segment.Segment{Kind: segment.SegmentField, Name: "member"})
		if !rootOK || !childOK {
			t.Fatalf("%s formal root", item.name)
		}
		value := typevalue.LiteralString(reg, item.name)
		world = world.WriteLocalPathKey(reg, child, value)
		roots = append(roots, state.BoundaryRoot{Path: root, Value: product.Top()})
	}
	selection := closedFactorSelection(t, domain, keys, world, roots)
	encoder, err := PrepareNormalReturnFactorEncoder[int](domain, selection, paramCount)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(world, encoder.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	operands := make([]NormalReturnFactorOperand[int], len(roots))
	operands[0] = NormalReturnFactorOperand[int]{Slot: 1, BoundarySlot: statekey.SymbolValue(7201), Value: product.Top()}
	for index, item := range persistent {
		operands[index+1] = NormalReturnFactorOperand[int]{
			Slot: index + 2, BoundarySlot: statekey.SymbolValue(item.sym), Value: product.Top(),
		}
	}
	got, err := encoder.Encode(context.Background(), nil, factors, operands)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NormalReturnFacts.PathRefinements) != len(persistent) {
		t.Fatalf("persistent formal refinements = %#v, want capture/global/ambient", got.NormalReturnFacts.PathRefinements)
	}
	for _, item := range persistent {
		wantPath := pathdom.NewPath(item.sym, "").Field("member")
		found := false
		for _, fact := range got.NormalReturnFacts.PathRefinements {
			if fact.Path.Equal(wantPath) && product.Equal(reg, fact.Value, typevalue.LiteralString(reg, item.name)) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s refinement %s missing from %#v", item.name, wantPath, got.NormalReturnFacts.PathRefinements)
		}
	}
	missingIdentity := append([]NormalReturnFactorOperand[int](nil), operands...)
	missingIdentity[1].BoundarySlot = 0
	if _, err := encoder.Encode(context.Background(), nil, factors, missingIdentity); err == nil {
		t.Fatal("non-parameter formal input without persistent boundary identity did not fail closed")
	}
}

func TestNormalReturnFactorEncoderRejectsNonFiniteCompanionsWithoutPanic(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain, err := state.TryRegisteredProductDomainWithLanes(reg, []state.LaneID{
		state.LaneValues, state.LaneHeapTableIdentity, state.LanePlacement,
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := state.SealBoundaryFactorSelection(keys, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := PrepareNormalReturnFactorEncoder[statekey.Value](domain, selection, 0)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(domain.Lattice().Top(), encoder.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Encode(context.Background(), nil, factors, nil); err == nil {
		t.Fatal("top heap/placement companions crossed the concrete outcome boundary")
	}
}

func projectedBoundaryWorld(t *testing.T, reg *axis.Registry, keys *keyspace.KeySpace, world state.State, roots state.BoundaryRoots) (state.State, state.BoundaryRoots) {
	t.Helper()
	artifact, err := state.ProjectBoundary(reg, keys, world, roots)
	if err != nil {
		t.Fatal(err)
	}
	projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	return projected, projectedRoots
}

func closedFactorSelection(t *testing.T, domain state.ProductDomain, keys *keyspace.KeySpace, world state.State, roots state.BoundaryRoots) state.BoundaryFactorSelection {
	t.Helper()
	factors, err := domain.Decompose(world)
	if err != nil {
		t.Fatal(err)
	}
	programs := make([]state.BoundaryReachabilityProgram, len(factors))
	for index := range factors {
		programs[index], err = domain.PrepareBoundaryFactorReachability(keys, factors[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	program, err := state.SealBoundaryReachabilityProgramSet(programs...)
	if err != nil {
		t.Fatal(err)
	}
	schemas := make([]state.BoundaryFactorRoot, len(roots))
	values := make([]product.Value, len(roots))
	for index, root := range roots {
		schemas[index] = state.BoundaryFactorRoot{Slot: root.Slot, Path: root.Path}
		values[index] = root.Value
	}
	selection, err := state.SealBoundaryFactorSelection(keys, schemas, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	selection, err = program.Close(selection, values)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}
