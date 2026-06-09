package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

func TestPathAliasFactsDomainLaws(t *testing.T) {
	alias := constraint.NewPath(cfg.SymbolID(1), "alias")
	source := constraint.NewPath(cfg.SymbolID(2), "source")
	other := constraint.NewPath(cfg.SymbolID(3), "other")

	one := PathAliasFacts{}.WithAddresses(testStableAddressPath(t, alias), testStableAddressPath(t, source))
	two := one.WithAddresses(testStableAddressPath(t, other), testStableAddressPath(t, source))

	lattice.LawSuite[PathAliasFacts]{
		Name:   "PathAliasFacts",
		Domain: PathAliasFactsDomain,
		Sample: []PathAliasFacts{
			PathAliasFactsDomain.Bottom(),
			PathAliasFactsDomain.Top(),
			one,
			two,
		},
		Format: func(f PathAliasFacts) string { return f.Format() },
	}.Run(t)
}

func TestPathAliasFactsAliasesCoveringPath(t *testing.T) {
	alias := constraint.NewPath(cfg.SymbolID(11), "alias")
	source := constraint.NewPath(cfg.SymbolID(12), "source")
	facts := PathAliasFacts{}.WithAddresses(testStableAddressPath(t, alias), testStableAddressPath(t, source))

	uses := facts.AliasesCoveringAddress(testStableAddressPath(t, alias.Field("id")))
	if len(uses) != 1 {
		t.Fatalf("AliasesCoveringPath(alias.id) got %d uses, want 1: %s", len(uses), facts.Format())
	}
	if uses[0].Alias.Source != StablePathKey(source) {
		t.Fatalf("source = %s, want %s", uses[0].Alias.Source, StablePathKey(source))
	}
	if len(uses[0].Remainder) != 1 || uses[0].Remainder[0].Name != "id" {
		t.Fatalf("remainder = %#v, want [.id]", uses[0].Remainder)
	}
}

func TestPathAliasFactsAddressCoverageSupportsNamedRoots(t *testing.T) {
	alias, _ := StableAddressOfRoot("$0", nil)
	source, _ := StableAddressOfRoot("$1", nil)
	child, _ := StableAddressOfRoot("$0", []constraint.Segment{{Kind: constraint.SegmentField, Name: "run"}})

	facts := PathAliasFacts{}.WithAddresses(alias, source)
	uses := facts.AliasesCoveringAddress(child)
	if len(uses) != 1 {
		t.Fatalf("AliasesCoveringAddress($0.run) got %d uses, want 1: %s", len(uses), facts.Format())
	}
	if uses[0].Alias.Source != source.Key() {
		t.Fatalf("source = %s, want %s", uses[0].Alias.Source, source.Key())
	}
	if len(uses[0].Remainder) != 1 || uses[0].Remainder[0].Name != "run" {
		t.Fatalf("remainder = %#v, want [.run]", uses[0].Remainder)
	}
}

func TestPathAliasSourceRoutesIgnoreNonCanonicalStoredSource(t *testing.T) {
	aliasPath := constraint.NewPath(cfg.SymbolID(17), "alias")
	sourcePath := constraint.NewPath(cfg.SymbolID(18), "source")
	alias := testStableAddressPath(t, aliasPath)
	facts := PathAliasFacts{}.With(PathAliasFact{
		Value:  alias.Key(),
		Source: sourcePath.Key(),
	})
	raw := facts.AliasesCoveringAddress(alias)
	if len(raw) != 1 || raw[0].Alias.Source != sourcePath.Key() {
		t.Fatalf("test setup did not keep noncanonical stored source key: %s", facts.Format())
	}

	if got := (pointRelationIndex{aliases: facts}).SourceRoutes(relationSourceQuery{
		Target: alias,
		Kind:   relationSourceIdentityAlias,
		IdentityPolicy: IdentityAliasRoutePolicy{
			PathAliasFacts: true,
		},
	}); len(got) != 0 {
		t.Fatalf("canonical path-alias route accepted noncanonical source key: %#v", got)
	}
}

func TestPathAliasFactsAliasesCoveringAddressIgnoresNonCanonicalStoredValue(t *testing.T) {
	aliasPath := constraint.NewPath(cfg.SymbolID(19), "alias")
	sourcePath := constraint.NewPath(cfg.SymbolID(20), "source")
	alias := testStableAddressPath(t, aliasPath)
	facts := PathAliasFacts{}.With(PathAliasFact{
		Value:  aliasPath.Key(),
		Source: testStableAddressPath(t, sourcePath).Key(),
	})
	if entries := facts.Entries(); len(entries) != 1 || entries[0].Value != aliasPath.Key() {
		t.Fatalf("test setup did not keep noncanonical stored value key: %s", facts.Format())
	}

	if got := facts.AliasesCoveringAddress(alias); len(got) != 0 {
		t.Fatalf("noncanonical stored value key produced alias uses: %#v", got)
	}
}

func TestPathAliasFactsKillAffectedByWrite(t *testing.T) {
	alias := constraint.NewPath(cfg.SymbolID(21), "alias")
	source := constraint.NewPath(cfg.SymbolID(22), "source")
	other := constraint.NewPath(cfg.SymbolID(23), "other")
	facts := PathAliasFacts{}.
		WithAddresses(testStableAddressPath(t, alias), testStableAddressPath(t, source)).
		WithAddresses(testStableAddressPath(t, other), testStableAddressPath(t, source))

	got := facts.KillAffectedByWriteAddress(testStableAddressPath(t, alias))
	if len(got.AliasesOfAddress(testStableAddressPath(t, alias))) != 0 {
		t.Fatalf("alias write kept stale alias fact: %s", got.Format())
	}
	if len(got.AliasesOfAddress(testStableAddressPath(t, other))) != 1 {
		t.Fatalf("unrelated alias fact was dropped: %s", got.Format())
	}
}
