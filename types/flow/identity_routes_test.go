package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestIdentityAliasSourcesFollowPathAndValueAliases(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(1), "target").Field("id")
	pathAliasSource := constraint.NewPath(cfg.SymbolID(2), "path_source").Field("id")
	valueAliasSource := constraint.NewPath(cfg.SymbolID(3), "value_source").Field("id")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(1), "target")),
			testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(2), "path_source")),
		),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(1), "target")),
			testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(3), "value_source")),
			ValueOriginAssignmentAlias,
			0,
		),
	}

	got := IdentityAliasSources(state, testStableAddressPath(t, target))
	if len(got) != 2 {
		t.Fatalf("alias sources got %d, want 2", len(got))
	}
	assertStableAddressPath(t, got[0], pathAliasSource)
	assertStableAddressPath(t, got[1], valueAliasSource)
}

func TestIdentityAliasClosureDeduplicatesReachableAliases(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(11), "root")
	mid := constraint.NewPath(cfg.SymbolID(12), "mid")
	source := constraint.NewPath(cfg.SymbolID(13), "source")
	state := PointState{
		PathAliases: PathAliasFacts{}.
			WithAddresses(testStableAddressPath(t, root), testStableAddressPath(t, mid)).
			WithAddresses(testStableAddressPath(t, root), testStableAddressPath(t, mid)),
		ValueOrigins: ValueOriginFacts{}.
			WithAddresses(testStableAddressPath(t, mid), testStableAddressPath(t, source), ValueOriginAssignmentAlias, 0),
	}

	got := IdentityAliasClosure(state, testStableAddressPath(t, root))
	if len(got) != 3 {
		t.Fatalf("alias closure got %d, want root + mid + source", len(got))
	}
	assertStableAddressPath(t, got[0], root)
	assertStableAddressPath(t, got[1], mid)
	assertStableAddressPath(t, got[2], source)
}

func TestIdentityAliasSourcesPolicyCanRequireAssignmentOriginRemainder(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(21), "target")
	source := constraint.NewPath(cfg.SymbolID(22), "source")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, target),
			testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(23), "path_source")),
		),
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, target),
			testStableAddressPath(t, source),
			ValueOriginAssignmentAlias,
			0,
		),
	}

	exact := IdentityAliasSourcesWithPolicy(state, testStableAddressPath(t, target), IdentityAliasDescendantOriginPolicy)
	if len(exact) != 0 {
		t.Fatalf("exact alias sources = %d, want none for descendant-only policy", len(exact))
	}
	descendant := IdentityAliasSourcesWithPolicy(state, testStableAddressPath(t, target.Field("id")), IdentityAliasDescendantOriginPolicy)
	if len(descendant) != 1 {
		t.Fatalf("descendant alias sources = %d, want one assignment-origin source", len(descendant))
	}
	assertStableAddressPath(t, descendant[0], source.Field("id"))
}

func assertStableAddressPath(t *testing.T, got StableAddress, want constraint.Path) {
	t.Helper()
	wantAddr := testStableAddressPath(t, want)
	if !got.Equal(wantAddr) {
		t.Fatalf("address = %s, want %s", got.Key(), wantAddr.Key())
	}
}
