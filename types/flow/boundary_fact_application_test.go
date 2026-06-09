package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyBoundaryFactsAppliesCollectionProvenanceTransaction(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(301), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(302), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(303), "node_id")
	sourcePath := constraint.NewPath(cfg.SymbolID(304), "source")
	nodeType := typ.NewRecord().Field("node_id", typ.String).Field("status", typ.String).Build()
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(arrayPath.Symbol): product.FromType(typ.NewFreshArray()),
			SymbolValueKey(keyPath.Symbol):   product.FromType(typ.String),
		},
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, tablePath), testStableAddressPath(t, keyPath)),
	}
	param := func(index int, path constraint.Path) BoundaryPath {
		return BoundaryPath{
			Kind:     BoundaryPathParam,
			Index:    index,
			Segments: append([]constraint.Segment(nil), path.Segments...),
		}
	}
	roots := NewBoundaryLocalRoots(map[int]constraint.Path{
		0: constraint.NewPath(arrayPath.Symbol, arrayPath.Root),
		1: constraint.NewPath(tablePath.Symbol, tablePath.Root),
		2: constraint.NewPath(keyPath.Symbol, keyPath.Root),
		3: constraint.NewPath(sourcePath.Symbol, sourcePath.Root),
	}, nil)
	facts := BoundaryFactsFromParts(BoundaryFactParts{
		AppendKeys: []BoundaryAppendKeyFact{{
			Array:    param(0, arrayPath),
			Key:      param(2, keyPath),
			Table:    param(1, tablePath),
			HasTable: true,
		}},
		IndexWrites: []BoundaryIndexWriteFact{{
			Table:      param(1, tablePath),
			KeyPath:    param(2, keyPath),
			HasKeyPath: true,
			KeyValue:   product.FromType(typ.String),
			Value:      product.FromType(nodeType),
		}},
	}).WithAppendElementFieldOrigins([]BoundaryAppendElementFieldOriginFact{{
		Array:  param(0, arrayPath),
		Field:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "status"}},
		Source: param(3, sourcePath),
	}})

	plans := BoundaryAppendKeyPlans(state, facts, roots.Rebase)
	if len(plans) != 1 {
		t.Fatalf("BoundaryAppendKeyPlans = %d, want one", len(plans))
	}
	if _, changed := ApplyBoundaryFacts(&state, facts, roots.Rebase, plans); !changed {
		t.Fatal("ApplyBoundaryFacts reported no change")
	}
	if tables := state.KeyPresence.KeyArrayTables(StablePathKey(arrayPath)); len(tables) != 1 || tables[0] != StablePathKey(tablePath) {
		t.Fatalf("boundary facts did not seed key-array table: %s", state.KeyPresence.Format())
	}
	values := state.KeyPresence.KeyArrayValues(StablePathKey(arrayPath), StablePathKey(tablePath))
	if len(values) != 1 || !product.Domain.Equal(values[0], product.FromType(nodeType)) {
		t.Fatalf("boundary key-array values = %v, want node record", values)
	}
	sources := state.KeyPresence.AppendElementFieldOriginUses(AppendElementFieldSourceQuery{
		Array: testStableAddressPath(t, arrayPath),
		Field: []constraint.Segment{{Kind: constraint.SegmentField, Name: "status"}},
	})
	if len(sources) != 1 {
		t.Fatalf("boundary append field sources = %v, want one; facts=%s", sources, state.KeyPresence.Format())
	}
}

func TestApplyBoundaryFactsAppliesIndexWriteWithKeyValueOnly(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(511), "graph").Field("nodes")
	state := PointState{}
	roots := NewBoundaryLocalRoots(map[int]constraint.Path{0: constraint.NewPath(tablePath.Symbol, tablePath.Root)}, nil)
	keyValue := product.FromType(typ.LiteralString("n1"))
	value := product.FromType(typ.NewRecord().Field("id", typ.String).Build())
	facts := BoundaryFactsFromParts(BoundaryFactParts{
		IndexWrites: []BoundaryIndexWriteFact{{
			Table:    BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: tablePath.Segments},
			KeyValue: keyValue,
			Value:    value,
		}},
	})

	if _, changed := ApplyBoundaryFacts(&state, facts, roots.Rebase, nil); !changed {
		t.Fatal("ApplyBoundaryFacts reported no change")
	}
	got, ok := PointFactsOf(state).DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:   tablePath,
		KeyValue: keyValue,
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("dynamic readback = %v/%v, want %s", got.ProjectValue(), ok, value.ProjectValue())
	}
}

func TestApplyBoundaryFactsReplaysUnnamedAppendTableCoverageFromEmptyPrestate(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(521), "graph").Field("node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(521), "graph").Field("edges")
	value := product.FromType(typ.NewRecord().
		Field("targets", typ.NewArray(typ.String)).
		Field("error_targets", typ.NewArray(typ.String)).
		Build())
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithEmptyKeyArrayAddress(testStableAddressPath(t, arrayPath)).
			WithAppendHistoryBaseAddress(testStableAddressPath(t, arrayPath)),
	}
	roots := NewBoundaryLocalRoots(map[int]constraint.Path{0: constraint.NewPath(arrayPath.Symbol, arrayPath.Root)}, nil)
	facts := BoundaryFactsDomain.Top().
		WithAppendHistoryBases([]BoundaryAppendHistoryBaseFact{{
			Array: BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: arrayPath.Segments},
		}}).
		WithAppendHistoryTableCoverage([]BoundaryAppendHistoryTableCoverageFact{{
			Array: BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: arrayPath.Segments},
			Table: BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: tablePath.Segments},
			Value: value,
		}})

	plan := PrepareBoundaryFactReplay(state, facts, roots)
	state.KeyPresence = KeyPresenceFacts{}
	if _, changed := ApplyBoundaryFactsWithReplay(&state, facts, roots, plan); !changed {
		t.Fatal("ApplyBoundaryFactsWithReplay reported no change")
	}
	values := state.KeyPresence.KeyArrayValues(StablePathKey(arrayPath), StablePathKey(tablePath))
	if len(values) != 1 || !product.Domain.Equal(values[0], value) {
		t.Fatalf("unnamed append table coverage = %v, want edge record; facts=%s", values, state.KeyPresence.Format())
	}
}

func TestApplyBoundaryFactsStaticMemberReplacesStaleValue(t *testing.T) {
	graphPath := constraint.NewPath(cfg.SymbolID(512), "graph")
	fieldPath := graphPath.Field("last_node_id")
	addr := testStableAddressPath(t, fieldPath)
	state := PointState{
		StaticMembers: StaticMemberFactsDomain.Top().WithAddress(addr, product.FromType(typ.Nil)),
	}
	roots := NewBoundaryLocalRoots(map[int]constraint.Path{0: graphPath}, nil)
	facts := BoundaryFactsDomain.Top().WithStaticMembers([]BoundaryStaticMemberFact{{
		Target: BoundaryPath{
			Kind:     BoundaryPathParam,
			Index:    0,
			Segments: fieldPath.Segments,
		},
		Value: product.FromType(typ.String),
	}})

	if _, changed := ApplyBoundaryFacts(&state, facts, roots.Rebase, nil); !changed {
		t.Fatal("ApplyBoundaryFacts reported no change")
	}
	got, ok := state.StaticMembers.ValueAtAddress(addr)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("static member after boundary replay = %v/%v, want string; facts=%s", got.ProjectValue(), ok, state.StaticMembers.Format())
	}
}

func TestApplyBoundaryFactsAppliesLengthRelation(t *testing.T) {
	sourcePath := constraint.NewPath(cfg.SymbolID(411), "source")
	targetPath := constraint.NewPath(cfg.SymbolID(412), "target")
	state := PointState{Num: numeric.NewState()}
	state.Num.ApplyLenGeConst(SymbolPathKey(sourcePath.Symbol, nil), 3)
	roots := NewBoundaryLocalRoots(
		map[int]constraint.Path{0: sourcePath},
		map[int]constraint.Path{0: targetPath},
	)
	facts := BoundaryFactsDomain.Top().WithLengthRelations([]BoundaryLengthRelationFact{{
		Target: BoundaryPath{Kind: BoundaryPathReturn, Index: 0},
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: 0},
	}})

	app, changed := ApplyBoundaryFacts(&state, facts, roots.Rebase, nil)
	if !changed {
		t.Fatal("ApplyBoundaryFacts reported no change")
	}
	if len(app.LengthRelations) != 1 || !app.LengthRelations[0].Target.Equal(targetPath) || !app.LengthRelations[0].Source.Equal(sourcePath) {
		t.Fatalf("length relation application = %#v, want target/source paths", app.LengthRelations)
	}
	if lower, _, ok := state.Num.LenBoundsFor(SymbolPathKey(targetPath.Symbol, nil)); !ok || lower != 3 {
		t.Fatalf("target length lower = %d/%v, want 3", lower, ok)
	}
}

func TestApplyBoundaryFactsAppliesLengthUpperBound(t *testing.T) {
	targetPath := constraint.NewPath(cfg.SymbolID(421), "target")
	state := PointState{}
	roots := NewBoundaryLocalRoots(
		nil,
		map[int]constraint.Path{0: targetPath},
	)
	facts := BoundaryFactsDomain.Top().WithLengthUpperBounds([]BoundaryLengthUpperBound{{
		Target: BoundaryPath{Kind: BoundaryPathReturn, Index: 0},
		Upper:  0,
	}})

	_, changed := ApplyBoundaryFacts(&state, facts, roots.Rebase, nil)
	if !changed {
		t.Fatal("ApplyBoundaryFacts reported no change")
	}
	if _, upper, ok := state.Num.LenBoundsFor(SymbolPathKey(targetPath.Symbol, nil)); !ok || upper != 0 {
		t.Fatalf("target length upper = %d/%v, want 0", upper, ok)
	}
}

func TestApplyBoundaryFactsWithReplayAppliesPrestateLengthLowerReplay(t *testing.T) {
	sourcePath := constraint.NewPath(cfg.SymbolID(431), "source")
	targetPath := constraint.NewPath(cfg.SymbolID(432), "target")
	state := PointState{Num: numeric.NewState()}
	state.Num.ApplyLenGeConst(SymbolPathKey(sourcePath.Symbol, nil), 7)
	roots := NewBoundaryLocalRoots(
		map[int]constraint.Path{0: sourcePath},
		map[int]constraint.Path{0: targetPath},
	)
	facts := BoundaryFactsDomain.Top().WithLengthRelations([]BoundaryLengthRelationFact{{
		Target: BoundaryPath{Kind: BoundaryPathReturn, Index: 0},
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: 0},
	}})

	plan := PrepareBoundaryFactReplay(state, facts, roots)
	state.Num = numeric.NewState()
	app, changed := ApplyBoundaryFactsWithReplay(&state, facts, roots, plan)
	if !changed {
		t.Fatal("ApplyBoundaryFactsWithReplay reported no change")
	}
	if len(app.LengthRelations) != 1 {
		t.Fatalf("plan application length relations = %#v, want one transfer-local relation application", app.LengthRelations)
	}
	if lower, _, ok := state.Num.LenBoundsFor(SymbolPathKey(targetPath.Symbol, nil)); !ok || lower != 7 {
		t.Fatalf("target prestate length lower = %d/%v, want 7", lower, ok)
	}
}

func TestBoundaryLocalRootsRebaseCopiesRootsAndAppendsSuffix(t *testing.T) {
	paramRoot := constraint.NewPath(cfg.SymbolID(401), "payload")
	returnRoot := constraint.NewPath(cfg.SymbolID(402), "result")
	roots := NewBoundaryLocalRoots(
		map[int]constraint.Path{0: paramRoot},
		map[int]constraint.Path{1: returnRoot},
	)

	paramRoot = paramRoot.Field("mutated")
	local, ok := roots.Rebase(BoundaryPath{
		Kind:     BoundaryPathParam,
		Index:    0,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
	})
	if !ok || !local.path.Equal(constraint.NewPath(cfg.SymbolID(401), "payload").Field("id")) {
		t.Fatalf("param rebase = %v/%v, want copied root plus suffix", local.path, ok)
	}
	local, ok = roots.Rebase(BoundaryPath{
		Kind:     BoundaryPathReturn,
		Index:    1,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "status"}},
	})
	if !ok || !local.path.Equal(constraint.NewPath(cfg.SymbolID(402), "result").Field("status")) {
		t.Fatalf("return rebase = %v/%v, want return root plus suffix", local.path, ok)
	}
	if _, ok := roots.Rebase(BoundaryPath{Kind: BoundaryPathParam, Index: 99}); ok {
		t.Fatal("missing boundary root should not rebase")
	}
}
