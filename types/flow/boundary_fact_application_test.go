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
	facts := BoundaryFactsOf(
		nil,
		nil,
		nil,
		[]BoundaryAppendKeyFact{{
			Array:    param(0, arrayPath),
			Key:      param(2, keyPath),
			Table:    param(1, tablePath),
			HasTable: true,
		}},
		nil,
		[]BoundaryIndexWriteFact{{
			Table: param(1, tablePath),
			Key:   param(2, keyPath),
			Value: product.FromType(nodeType),
		}},
	).WithAppendElementFieldOrigins([]BoundaryAppendElementFieldOriginFact{{
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
	if len(app.LengthLowerBounds) != 0 {
		t.Fatalf("ApplyBoundaryFacts returned already-applied length lower replay events: %#v", app.LengthLowerBounds)
	}
	if lower, _, ok := state.Num.LenBoundsFor(SymbolPathKey(targetPath.Symbol, nil)); !ok || lower != 3 {
		t.Fatalf("target length lower = %d/%v, want 3", lower, ok)
	}
}

func TestBoundaryFactPrestateApplicationCapturesLengthLowerReplay(t *testing.T) {
	sourcePath := constraint.NewPath(cfg.SymbolID(421), "source")
	targetPath := constraint.NewPath(cfg.SymbolID(422), "target")
	state := PointState{Num: numeric.NewState()}
	state.Num.ApplyLenGeConst(SymbolPathKey(sourcePath.Symbol, nil), 5)
	roots := NewBoundaryLocalRoots(
		map[int]constraint.Path{0: sourcePath},
		map[int]constraint.Path{0: targetPath},
	)
	facts := BoundaryFactsDomain.Top().WithLengthRelations([]BoundaryLengthRelationFact{{
		Target: BoundaryPath{Kind: BoundaryPathReturn, Index: 0},
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: 0},
	}})

	app := BoundaryFactPrestateApplication(state, facts, roots.Rebase)
	if len(app.LengthLowerBounds) != 1 ||
		!app.LengthLowerBounds[0].Target.Equal(targetPath) ||
		app.LengthLowerBounds[0].Lower != 5 {
		t.Fatalf("prestate length lower replay = %#v, want target >= 5", app.LengthLowerBounds)
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
