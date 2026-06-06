package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/access"
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

func TestApplyAccessMutationAppliesSelectedWriteConsequences(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(31), "table")
	child := table.Field("field")
	other := constraint.NewPath(cfg.SymbolID(32), "other")
	key := constraint.NewPath(cfg.SymbolID(33), "key")
	tableAddr := testStableAddressPath(t, table)
	keyAddr := testStableAddressPath(t, key)
	ps := PointState{
		Cond: constraint.FromConstraints(
			constraint.Truthy{Path: child},
			constraint.Truthy{Path: other},
		),
		KeyPresence: KeyPresenceFacts{}.WithAddresses(tableAddr, keyAddr),
	}
	SetStaticMemberPath(&ps, child, product.FromType(typ.String))

	if !ApplyAccessMutation(&ps, AccessMutation{
		Footprint: access.WriteFootprint{
			WritePath:         table,
			ExactWritePath:    table,
			HasExactWritePath: true,
		},
		StaticMembers: true,
		Conditions:    true,
		AddressFacts:  true,
	}) {
		t.Fatal("ApplyAccessMutation reported no change")
	}
	if _, ok := PointFactsOf(ps).StaticMemberValue(child); ok {
		t.Fatalf("static member survived access mutation: %s", ps.StaticMembers.Format())
	}
	if accessMutationConditionMentions(ps.Cond, child) {
		t.Fatalf("condition still mentions written subtree: %v", ps.Cond)
	}
	if !accessMutationConditionMentions(ps.Cond, other) {
		t.Fatalf("condition lost unrelated path: %v", ps.Cond)
	}
	if ps.KeyPresence.HasAddresses(tableAddr, keyAddr) {
		t.Fatalf("key presence survived access mutation: %s", ps.KeyPresence.Format())
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

func accessMutationConditionMentions(cond constraint.Condition, path constraint.Path) bool {
	found := false
	for i := 0; i < cond.NumDisjuncts(); i++ {
		for _, c := range cond.DisjunctConstraints(i) {
			constraint.VisitPaths(c, func(candidate constraint.Path) bool {
				if candidate.Equal(path) {
					found = true
					return true
				}
				return false
			})
			if found {
				return true
			}
		}
	}
	return false
}
