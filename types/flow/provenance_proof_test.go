package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyPathAliasProofPublishesAlias(t *testing.T) {
	value := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(1), "value"))
	source := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(2), "source"))
	state := PointState{}

	if !ApplyPathAliasProof(&state, PathAliasProof{Value: value, Source: source}) {
		t.Fatal("ApplyPathAliasProof reported no change")
	}
	aliases := state.PathAliases.AliasesOfAddress(value)
	if len(aliases) != 1 || aliases[0].Source != source.Key() {
		t.Fatalf("aliases = %v, want source %s", aliases, source.Key())
	}
}

func TestApplyValueOriginPathTransactionPublishesOrigin(t *testing.T) {
	valuePath := constraint.NewPath(cfg.SymbolID(13), "value")
	sourcePath := constraint.NewPath(cfg.SymbolID(14), "source")
	value := testStableAddressPath(t, valuePath)
	source := testStableAddressPath(t, sourcePath)
	state := PointState{}

	if !ApplyValueOriginPathTransaction(&state, ValueOriginPathTransaction{
		ValuePath:  valuePath,
		SourcePath: sourcePath,
		Kind:       ValueOriginIndexedIterator,
		VarIndex:   1,
	}) {
		t.Fatal("ApplyValueOriginPathTransaction reported unchanged")
	}

	origins := state.ValueOrigins.OriginsOfAddress(value)
	if len(origins) != 1 || origins[0].Source != source.Key() || origins[0].Kind != ValueOriginIndexedIterator || origins[0].VarIndex != 1 {
		t.Fatalf("origins = %v, want indexed iterator source %s", origins, source.Key())
	}
}

func TestApplyValueOriginProofPublishesOrigin(t *testing.T) {
	value := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(3), "value"))
	source := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(4), "source"))
	state := PointState{}

	if !ApplyValueOriginProof(&state, ValueOriginProof{
		Value:    value,
		Source:   source,
		Kind:     ValueOriginAssignmentAlias,
		VarIndex: 1,
	}) {
		t.Fatal("ApplyValueOriginProof reported no change")
	}
	origins := state.ValueOrigins.OriginsOfAddress(value)
	if len(origins) != 1 || origins[0].Source != source.Key() || origins[0].Kind != ValueOriginAssignmentAlias || origins[0].VarIndex != 1 {
		t.Fatalf("origins = %v, want assignment alias from %s", origins, source.Key())
	}
}

func TestApplyAssignmentAliasPathTransactionPublishesReducedProductFacts(t *testing.T) {
	tablePath := constraint.NewPath(cfg.SymbolID(21), "table")
	sourcePath := constraint.NewPath(cfg.SymbolID(22), "source_key")
	targetPath := constraint.NewPath(cfg.SymbolID(23), "target_key")
	table := testStableAddressPath(t, tablePath)
	source := testStableAddressPath(t, sourcePath)
	target := testStableAddressPath(t, targetPath)
	keyValue := product.FromType(typ.LiteralString("id"))
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(tablePath.Symbol):  product.FromType(typ.NewRecord().Field("id", typ.Number).Build()),
			SymbolValueKey(sourcePath.Symbol): keyValue,
		},
		KeyPresence: KeyPresenceFacts{}.WithAddresses(table, source),
	}

	if !ApplyAssignmentAliasPathTransaction(&state, AssignmentAliasPathTransaction{
		TargetPath:  targetPath,
		SourcePath:  sourcePath,
		SourceValue: keyValue,
	}) {
		t.Fatal("ApplyAssignmentAliasPathTransaction reported unchanged")
	}
	if aliases := state.PathAliases.AliasesOfAddress(target); len(aliases) != 1 || aliases[0].Source != source.Key() {
		t.Fatalf("aliases = %v, want source %s", aliases, source.Key())
	}
	if origins := state.ValueOrigins.OriginsOfAddress(target); len(origins) != 1 || origins[0].Source != source.Key() || origins[0].Kind != ValueOriginAssignmentAlias {
		t.Fatalf("origins = %v, want assignment alias source %s", origins, source.Key())
	}
	if !state.KeyPresence.Has(table.Key(), target.Key()) {
		t.Fatalf("key presence did not alias to target: %s", state.KeyPresence.Format())
	}
	got, ok := state.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:     table,
		KeyPath:    target,
		HasKeyPath: true,
		KeyValue:   keyValue,
	})
	if !ok {
		t.Fatal("assignment alias transaction did not derive readback admission")
	}
	if !product.Domain.Equal(got, product.FromType(typ.Number)) {
		t.Fatalf("readback admission = %v, want number", got.ProjectValue())
	}
}

func TestApplyArrayElementKeyPathTransactionPublishesReducedProductFacts(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(31), "keys")
	tablePath := constraint.NewPath(cfg.SymbolID(32), "table")
	targetPath := constraint.NewPath(cfg.SymbolID(33), "key")
	array := testStableAddressPath(t, arrayPath)
	table := testStableAddressPath(t, tablePath)
	target := testStableAddressPath(t, targetPath)
	keyValue := product.FromType(typ.LiteralString("id"))
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.WithKeyArrayAddresses(array, table),
	}

	if !ApplyArrayElementKeyPathTransaction(&state, ArrayElementKeyPathTransaction{
		TargetPath: targetPath,
		ArrayPath:  arrayPath,
		KeyValue:   keyValue,
	}) {
		t.Fatal("ApplyArrayElementKeyPathTransaction reported unchanged")
	}
	if !state.KeyPresence.Has(table.Key(), target.Key()) {
		t.Fatalf("key-array element did not publish table/key presence: %s", state.KeyPresence.Format())
	}
	if origins := state.ValueOrigins.OriginsOfAddress(target); len(origins) != 1 || origins[0].Source != array.Key() || origins[0].Kind != ValueOriginIndexedIterator || origins[0].VarIndex != 1 {
		t.Fatalf("origins = %v, want indexed iterator source %s", origins, array.Key())
	}
}
