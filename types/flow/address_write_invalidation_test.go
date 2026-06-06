package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyAddressWriteInvalidationKillsAddressSideFacts(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(1), "table")
	key := constraint.NewPath(cfg.SymbolID(2), "key")
	value := constraint.NewPath(cfg.SymbolID(3), "value")
	source := constraint.NewPath(cfg.SymbolID(4), "source")
	tableAddr := testStableAddressPath(t, table)
	keyAddr := testStableAddressPath(t, key)
	valueAddr := testStableAddressPath(t, value)
	sourceAddr := testStableAddressPath(t, source)
	ps := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(tableAddr, keyAddr).
			WithValueAddresses(tableAddr, keyAddr, valueAddr),
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:       tableAddr,
			KeyPath:      keyAddr,
			HasKeyPath:   true,
			Key:          product.FromType(typ.String),
			ValuePath:    valueAddr,
			HasValuePath: true,
			Value:        product.FromType(typ.Number),
		}),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(tableAddr, sourceAddr, ValueOriginAssignmentAlias, 0),
		PathAliases:  PathAliasFacts{}.WithAddresses(tableAddr, sourceAddr),
	}

	if !ApplyAddressWriteInvalidation(&ps, AddressWriteInvalidation{Write: tableAddr}) {
		t.Fatal("ApplyAddressWriteInvalidation reported no change")
	}
	if ps.KeyPresence.HasAddresses(tableAddr, keyAddr) {
		t.Fatalf("key presence survived table write: %s", ps.KeyPresence.Format())
	}
	if _, ok := ps.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:       tableAddr,
		KeyPath:      keyAddr,
		HasKeyPath:   true,
		KeyValue:     product.FromType(typ.String),
		ValuePath:    valueAddr,
		HasValuePath: true,
	}); ok {
		t.Fatalf("index-write admission survived table write: %s", ps.IndexWrites.Format())
	}
	if uses := ps.ValueOrigins.OriginsOfAddress(tableAddr); len(uses) != 0 {
		t.Fatalf("value origins survived table write: %s", ps.ValueOrigins.Format())
	}
	if aliases := ps.PathAliases.AliasesOfAddress(tableAddr); len(aliases) != 0 {
		t.Fatalf("path aliases survived table write: %s", ps.PathAliases.Format())
	}
}

func TestApplyAddressWritePathInvalidationNormalizesWriteFootprint(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(21), "table")
	key := constraint.NewPath(cfg.SymbolID(22), "key")
	value := constraint.NewPath(cfg.SymbolID(23), "value")
	source := constraint.NewPath(cfg.SymbolID(24), "source")
	tableAddr := testStableAddressPath(t, table)
	keyAddr := testStableAddressPath(t, key)
	valueAddr := testStableAddressPath(t, value)
	sourceAddr := testStableAddressPath(t, source)
	ps := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(tableAddr, keyAddr).
			WithValueAddresses(tableAddr, keyAddr, valueAddr),
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:       tableAddr,
			KeyPath:      keyAddr,
			HasKeyPath:   true,
			Key:          product.FromType(typ.String),
			ValuePath:    valueAddr,
			HasValuePath: true,
			Value:        product.FromType(typ.Number),
		}),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(tableAddr, sourceAddr, ValueOriginAssignmentAlias, 0),
		PathAliases:  PathAliasFacts{}.WithAddresses(tableAddr, sourceAddr),
	}

	if !ApplyAddressWritePathInvalidation(&ps, AddressWritePathInvalidation{WritePath: table}) {
		t.Fatal("ApplyAddressWritePathInvalidation reported no change")
	}
	if ps.KeyPresence.HasAddresses(tableAddr, keyAddr) {
		t.Fatalf("key presence survived table write: %s", ps.KeyPresence.Format())
	}
	if _, ok := ps.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
		Target:       tableAddr,
		KeyPath:      keyAddr,
		HasKeyPath:   true,
		KeyValue:     product.FromType(typ.String),
		ValuePath:    valueAddr,
		HasValuePath: true,
	}); ok {
		t.Fatalf("index-write admission survived table write: %s", ps.IndexWrites.Format())
	}
	if uses := ps.ValueOrigins.OriginsOfAddress(tableAddr); len(uses) != 0 {
		t.Fatalf("value origins survived table write: %s", ps.ValueOrigins.Format())
	}
	if aliases := ps.PathAliases.AliasesOfAddress(tableAddr); len(aliases) != 0 {
		t.Fatalf("path aliases survived table write: %s", ps.PathAliases.Format())
	}
}

func TestApplyKeyPresenceProofPublishesValuePath(t *testing.T) {
	tableAddr := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(11), "table"))
	keyAddr := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(12), "key"))
	valueAddr := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(13), "value"))
	ps := PointState{}

	if !ApplyKeyPresenceProof(&ps, KeyPresenceProof{
		Table:        tableAddr,
		Key:          keyAddr,
		ValuePath:    valueAddr,
		HasValuePath: true,
	}) {
		t.Fatal("ApplyKeyPresenceProof reported no change")
	}
	if !ps.KeyPresence.HasAddresses(tableAddr, keyAddr) {
		t.Fatalf("key presence missing: %s", ps.KeyPresence.Format())
	}
	if !ps.KeyPresence.HasValueAddresses(tableAddr, keyAddr, valueAddr) {
		t.Fatalf("value-path presence missing: %s", ps.KeyPresence.Format())
	}
}
