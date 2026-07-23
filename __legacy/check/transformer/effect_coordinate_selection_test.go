package transformer

import (
	"sort"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestWidePathStoreSelectsExactRegisteredCoordinateClosure(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	const targetSymbol, sourceSymbol = symbol.ID(8801), symbol.ID(8802)
	shape := Shape{Params: 2}
	terms := NewArena(reg)
	target := terms.Path(Root{Kind: RootParam, Index: 0}, segment.Segment{Kind: segment.SegmentField, Name: "value"})
	source := terms.Path(Root{Kind: RootParam, Index: 1})
	effects := NewEffectArena(terms)
	effect, err := effects.PathStore(PathStoreConfig{
		Target: target, Value: terms.Root(Root{Kind: RootParam, Index: 1}), SourcePath: source,
		Site: EffectSite{Owner: 8800, Ordinal: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{targetSymbol, sourceSymbol}).
		WithBoundaryCaptures(nil).WithBoundaryGlobals(nil)
	roots, err := sealRelationRootCarrier(plan, keys, shape)
	if err != nil {
		t.Fatal(err)
	}
	body := &relationProgramBody{
		keys: keys, roots: roots, productDomain: domain,
		relation: Relation{shape: shape, arena: terms, effects: effects},
	}
	access, err := freezeEffectCoordinateAccess(body, effect)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := access.selection(body)
	if err != nil || selection == nil {
		t.Fatalf("path-store coordinate contract = %T/%v", selection, err)
	}

	input := state.Reachable(domain.Lattice().Bottom())
	write := func(path pathdom.Path) {
		input = input.WriteLocalPathKey(reg, keys.FromPath(path), product.Top())
	}
	write(pathdom.NewPath(targetSymbol, "").Field("value"))
	write(pathdom.NewPath(sourceSymbol, ""))
	for index := 0; index < 181; index++ {
		write(pathdom.NewPath(symbol.ID(9000+index), "").Field("unrelated"))
	}
	lane, ok := domain.ProductLane(state.LanePathEvidence)
	if !ok {
		t.Fatal("registered product has no path-evidence lane")
	}
	family, ok := selection.Family(domain)
	if !ok {
		t.Fatal("path-store selection has no registered family")
	}
	factors, err := domain.DecomposeLanes(input, []state.ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("wide path-store factor inventory = %d/%v", len(factors), err)
	}
	_, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	slots := make([]state.CoordinateSlot, len(scalars))
	for index, scalar := range scalars {
		slots[index] = scalar.Slot()
	}
	selected, err := selection.Select(domain, slots)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) < 181 || len(selected) > 4 {
		t.Fatalf("path-store selected %d/%d path coordinates, want bounded target/source closure", len(selected), len(slots))
	}
}

func TestEffectCoordinateSelectionIncludesDynamicValueTermClosure(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	const targetSymbol, tableSymbol = symbol.ID(9801), symbol.ID(9802)
	shape := Shape{Params: 2}
	terms := NewArena(reg)
	target := terms.Path(Root{Kind: RootParam, Index: 0}, segment.Segment{Kind: segment.SegmentField, Name: "value"})
	tablePath := terms.Path(Root{Kind: RootParam, Index: 1})
	table := terms.Root(Root{Kind: RootParam, Index: 1})
	value := terms.DynamicReadValueAtPaths(1, table, tablePath, terms.Constant(typevalue.LiteralString(reg, "key")), 0)
	effects := NewEffectArena(terms)
	effect, err := effects.PathStore(PathStoreConfig{
		Target: target, Value: value, Site: EffectSite{Owner: 9800, Ordinal: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{targetSymbol, tableSymbol}).
		WithBoundaryCaptures(nil).WithBoundaryGlobals(nil)
	roots, err := sealRelationRootCarrier(plan, keys, shape)
	if err != nil {
		t.Fatal(err)
	}
	body := &relationProgramBody{
		keys: keys, roots: roots, productDomain: domain,
		relation: Relation{shape: shape, arena: terms, effects: effects},
	}
	access, err := freezeEffectCoordinateAccess(body, effect)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := access.selection(body)
	if err != nil || selection == nil {
		t.Fatalf("dynamic effect coordinate contract = %T/%v", selection, err)
	}

	pathSelection, ok := selection.(pathCoordinateSelection)
	if !ok {
		t.Fatalf("dynamic effect selection = %T, want registered path-family certificate", selection)
	}
	want := []keyspace.Key{
		keys.FromPath(pathdom.NewPath(targetSymbol, "").Field("value")),
		keys.FromPath(pathdom.NewPath(tableSymbol, "")),
	}
	for _, key := range want {
		index := sort.Search(len(pathSelection.seeds), func(index int) bool {
			return !keys.Less(pathSelection.seeds[index], key)
		})
		if index == len(pathSelection.seeds) || pathSelection.seeds[index] != key {
			t.Fatalf("dynamic effect coordinate seeds %v omit %v", pathSelection.seeds, key)
		}
	}
}
