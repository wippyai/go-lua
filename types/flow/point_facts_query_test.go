package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPointFactsPathKeyPresenceQueries(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(101), "table")
	key := constraint.NewPath(cfg.SymbolID(102), "key")
	value := constraint.NewPath(cfg.SymbolID(103), "value")
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key)).
			WithValueAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key), testStableAddressPath(t, value)),
	}
	facts := PointFactsOf(state)

	if !facts.HasKeyPresence(table, key) {
		t.Fatal("HasKeyPresence missed table/key fact")
	}
	if !facts.HasKeyValuePresence(table, key, value) {
		t.Fatal("HasKeyValuePresence missed table/key/value fact")
	}
	if facts.HasKeyPresence(table, constraint.NewPath(cfg.SymbolID(104), "other_key")) {
		t.Fatal("HasKeyPresence accepted unrelated key")
	}
}

func TestPointFactsIndexWriteAdmissionPathQuery(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(111), "target")
	key := constraint.NewPath(cfg.SymbolID(112), "key")
	value := product.FromType(typ.String)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, target),
			KeyPath:    testStableAddressPath(t, key),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		}),
	}
	facts := PointFactsOf(state)

	got, ok := facts.IndexWriteAdmission(IndexWritePathQuery{
		Target:     target,
		KeyPath:    key,
		HasKeyPath: true,
		KeyValue:   product.FromType(typ.String),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("IndexWriteAdmission = %v/%v, want string", got, ok)
	}
}

func TestPointFactsIdentityAliasClosurePaths(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(121), "root")
	source := constraint.NewPath(cfg.SymbolID(122), "source")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, root),
			testStableAddressPath(t, source),
		),
	}
	facts := PointFactsOf(state)

	got := facts.IdentityAliasClosurePaths(root)
	if len(got) != 2 {
		t.Fatalf("IdentityAliasClosurePaths got %d paths, want root + source", len(got))
	}
	if !got[0].Equal(root) || !got[1].Equal(source) {
		t.Fatalf("IdentityAliasClosurePaths = %v, want %s then %s", got, root.String(), source.String())
	}
}

func TestPointFactsIdentityAliasSourcePathsWithPolicy(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(131), "target")
	source := constraint.NewPath(cfg.SymbolID(132), "source")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, target),
			testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(133), "path_source")),
		),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, target),
			testStableAddressPath(t, source),
			ValueOriginAssignmentAlias,
			0,
		),
	}
	facts := PointFactsOf(state)

	exact := facts.IdentityAliasSourcePaths(target, IdentityAliasDescendantOriginPolicy)
	if len(exact) != 0 {
		t.Fatalf("exact alias source paths = %d, want none for descendant-only policy", len(exact))
	}

	got := facts.IdentityAliasSourcePaths(target.Field("id"), IdentityAliasDescendantOriginPolicy)
	if len(got) != 1 || !got[0].Equal(source.Field("id")) {
		t.Fatalf("descendant alias source paths = %v, want %s", got, source.Field("id").String())
	}
}
