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

	one := PathAliasFacts{}.WithPaths(alias, source)
	two := one.WithPaths(other, source)

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
	facts := PathAliasFacts{}.WithPaths(alias, source)

	uses := facts.AliasesCoveringPath(alias.Field("id"))
	if len(uses) != 1 {
		t.Fatalf("AliasesCoveringPath(alias.id) got %d uses, want 1: %s", len(uses), facts.Format())
	}
	if uses[0].Alias.Source != KeyPresencePathKey(source) {
		t.Fatalf("source = %s, want %s", uses[0].Alias.Source, KeyPresencePathKey(source))
	}
	if len(uses[0].Remainder) != 1 || uses[0].Remainder[0].Name != "id" {
		t.Fatalf("remainder = %#v, want [.id]", uses[0].Remainder)
	}
}

func TestPathAliasFactsKillAffectedByWrite(t *testing.T) {
	alias := constraint.NewPath(cfg.SymbolID(21), "alias")
	source := constraint.NewPath(cfg.SymbolID(22), "source")
	other := constraint.NewPath(cfg.SymbolID(23), "other")
	facts := PathAliasFacts{}.
		WithPaths(alias, source).
		WithPaths(other, source)

	got := facts.KillAffectedByWrite(KeyPresencePathKey(alias))
	if len(got.AliasesOfPath(alias)) != 0 {
		t.Fatalf("alias write kept stale alias fact: %s", got.Format())
	}
	if len(got.AliasesOfPath(other)) != 1 {
		t.Fatalf("unrelated alias fact was dropped: %s", got.Format())
	}
}
