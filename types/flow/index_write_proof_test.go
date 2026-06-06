package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyIndexWriteAdmissionProofPublishesFact(t *testing.T) {
	table := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(1), "table"))
	key := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(2), "key"))
	value := product.FromType(typ.String)
	state := PointState{}

	if !ApplyIndexWriteAdmissionProof(&state, IndexWriteAdmissionProof{
		Fact: IndexWriteAdmissionAddressFact{
			Target:     table,
			KeyPath:    key,
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		},
	}) {
		t.Fatal("ApplyIndexWriteAdmissionProof reported no change")
	}

	got, ok := state.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:   table,
		KeyPath:  key,
		KeyValue: product.FromType(typ.String),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("admission = %v/%v, want string", got, ok)
	}
}

func TestApplyIndexWriteKeyAliasProofCopiesKeyPath(t *testing.T) {
	table := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(3), "table"))
	source := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(4), "source_key"))
	target := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(5), "target_key"))
	value := product.FromType(typ.Number)
	state := PointState{}
	ApplyIndexWriteAdmissionProof(&state, IndexWriteAdmissionProof{
		Fact: IndexWriteAdmissionAddressFact{
			Target:     table,
			KeyPath:    source,
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		},
	})

	if !ApplyIndexWriteKeyAliasProof(&state, IndexWriteKeyAliasProof{
		SourceKey: source,
		TargetKey: target,
	}) {
		t.Fatal("ApplyIndexWriteKeyAliasProof reported no change")
	}

	got, ok := state.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:   table,
		KeyPath:  target,
		KeyValue: product.FromType(typ.String),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("alias admission = %v/%v, want number", got, ok)
	}
}
