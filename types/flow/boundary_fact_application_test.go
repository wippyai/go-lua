package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
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
	rebase := func(path BoundaryPath) (BoundaryLocalPath, bool) {
		if path.Kind != BoundaryPathParam {
			return BoundaryLocalPath{}, false
		}
		var root constraint.Path
		switch path.Index {
		case 0:
			root = constraint.NewPath(arrayPath.Symbol, arrayPath.Root)
		case 1:
			root = constraint.NewPath(tablePath.Symbol, tablePath.Root)
		case 2:
			root = constraint.NewPath(keyPath.Symbol, keyPath.Root)
		case 3:
			root = constraint.NewPath(sourcePath.Symbol, sourcePath.Root)
		default:
			return BoundaryLocalPath{}, false
		}
		for _, seg := range path.Segments {
			root = root.Append(seg)
		}
		return BoundaryLocalPathOfPath(root)
	}
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

	plans := BoundaryAppendKeyPlans(state, facts, rebase)
	if len(plans) != 1 {
		t.Fatalf("BoundaryAppendKeyPlans = %d, want one", len(plans))
	}
	if _, changed := ApplyBoundaryFacts(&state, facts, rebase, plans); !changed {
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
