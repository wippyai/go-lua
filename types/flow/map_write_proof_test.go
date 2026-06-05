package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyMapWriteProofDecouplesKeyArrayFromReadbackAdmission(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(1), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(2), "edges")
	keyPath := constraint.NewPath(cfg.SymbolID(3), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	keyKey := StablePathKey(keyPath)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithPendingKeyArray(arrayKey, tableKey, keyKey)

	proof, ok := MapWriteProofFromPaths(
		tablePath,
		keyPath,
		product.FromType(typ.Any),
		constraint.Path{},
		product.FromType(typ.String),
		false,
	)
	if !ok {
		t.Fatal("MapWriteProofFromPaths did not resolve")
	}
	if changed := ApplyMapWriteProof(&state, proof); !changed {
		t.Fatal("ApplyMapWriteProof reported no change")
	}
	if !state.KeyPresence.Has(tableKey, keyKey) {
		t.Fatalf("key presence missing in %s", state.KeyPresence.Format())
	}
	if tables := state.KeyPresence.KeyArrayTables(arrayKey); len(tables) != 1 || tables[0] != tableKey {
		t.Fatalf("key-array tables = %v, want %s", tables, tableKey)
	}
	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	if len(values) != 1 || !product.Domain.Equal(values[0], product.FromType(typ.String)) {
		t.Fatalf("key-array values = %v, want string", values)
	}
	if len(state.IndexWrites.Entries()) != 0 {
		t.Fatalf("readback fact was published despite inadmissible key: %s", state.IndexWrites.Format())
	}
}

func TestApplyMapWriteProofPublishesReadbackWhenAdmissible(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(2), "edges")
	keyPath := constraint.NewPath(cfg.SymbolID(3), "node_id")

	state := PointStateDomain.Top()
	proof, ok := MapWriteProofFromPaths(
		tablePath,
		keyPath,
		product.FromType(typ.String),
		constraint.Path{},
		product.FromType(typ.Number),
		false,
	)
	if !ok {
		t.Fatal("MapWriteProofFromPaths did not resolve")
	}
	ApplyMapWriteProof(&state, proof)

	value, ok := state.IndexWrites.AdmissionAtAddress(testIndexWriteAddressQuery(t, tablePath, keyPath, typ.String, constraint.Path{}))
	if !ok || !product.Domain.Equal(value, product.FromType(typ.Number)) {
		t.Fatalf("readback = %v/%v, want number", value, ok)
	}
}

func TestApplyMapWriteProofWidensExistingKeyArrayValueForPresentWrite(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(11), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(12), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(13), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	oldValue := product.FromType(typ.String)
	writtenValue := product.FromType(typ.Number)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.WithKeyArrayValue(arrayKey, tableKey, oldValue)

	proof, ok := MapWriteProofFromPaths(
		tablePath,
		keyPath,
		product.FromType(typ.String),
		constraint.Path{},
		writtenValue,
		false,
	)
	if !ok {
		t.Fatal("MapWriteProofFromPaths did not resolve")
	}
	if changed := ApplyMapWriteProof(&state, proof); !changed {
		t.Fatal("ApplyMapWriteProof reported no change")
	}

	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	want := product.Domain.Join(oldValue, writtenValue)
	if len(values) != 1 || !product.Domain.Equal(values[0], want) {
		t.Fatalf("key-array values = %v, want %v", values, want.ProjectValue())
	}
}

func TestApplyMapWriteProofCoversTrackedAppendEvent(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(21), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(22), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(23), "node_id")
	arrayKey := StablePathKey(arrayPath)
	tableKey := StablePathKey(tablePath)
	keyKey := StablePathKey(keyPath)
	nodeValue := product.FromType(typ.String)

	state := PointStateDomain.Top()
	state.KeyPresence = state.KeyPresence.
		WithAppendHistoryBase(arrayKey).
		WithAppendHistoryEvent(arrayKey, keyKey)

	proof, ok := MapWriteProofFromPaths(
		tablePath,
		keyPath,
		product.FromType(typ.String),
		constraint.Path{},
		nodeValue,
		false,
	)
	if !ok {
		t.Fatal("MapWriteProofFromPaths did not resolve")
	}
	if changed := ApplyMapWriteProof(&state, proof); !changed {
		t.Fatal("ApplyMapWriteProof reported no change")
	}

	values := state.KeyPresence.KeyArrayValues(arrayKey, tableKey)
	if len(values) != 1 || !product.Domain.Equal(values[0], nodeValue) {
		t.Fatalf("tracked append coverage values = %v, want string; facts=%s", values, state.KeyPresence.Format())
	}
}

func TestIndexedIteratorKeyArrayReadbackDerivesValueFromStableFacts(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(1), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(2), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(3), "current_node_id")

	keyPresence := KeyPresenceFacts{}.
		WithKeyArrayValueAddresses(testStableAddressPath(t, arrayPath), testStableAddressPath(t, tablePath), product.FromType(typ.String))
	origins := ValueOriginFacts{}.
		WithAddresses(testStableAddressPath(t, keyPath), testStableAddressPath(t, arrayPath), ValueOriginIndexedIterator, 1)

	got, ok := IndexedIteratorKeyArrayReadback(keyPresence, origins, tablePath, keyPath)
	if !ok || !product.Domain.Equal(got, product.FromType(typ.String)) {
		t.Fatalf("readback = %v/%v, want string", got, ok)
	}
}

func TestIndexedIteratorKeyArrayReadbackFollowsAssignmentAlias(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(1), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(2), "edges")
	iterKeyPath := constraint.NewPath(cfg.SymbolID(3), "node_id")
	aliasPath := constraint.NewPath(cfg.SymbolID(4), "current_node_id")

	keyPresence := KeyPresenceFacts{}.
		WithKeyArrayValueAddresses(testStableAddressPath(t, arrayPath), testStableAddressPath(t, tablePath), product.FromType(typ.Number))
	origins := ValueOriginFacts{}.
		WithAddresses(testStableAddressPath(t, iterKeyPath), testStableAddressPath(t, arrayPath), ValueOriginIndexedIterator, 1).
		WithAddresses(testStableAddressPath(t, aliasPath), testStableAddressPath(t, iterKeyPath), ValueOriginAssignmentAlias, 0)

	got, ok := IndexedIteratorKeyArrayReadback(keyPresence, origins, tablePath, aliasPath)
	if !ok || !product.Domain.Equal(got, product.FromType(typ.Number)) {
		t.Fatalf("alias readback = %v/%v, want number", got, ok)
	}
}
