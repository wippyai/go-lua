package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPathTypeEvidenceProjectsDeclaredRootToPath(t *testing.T) {
	const sym = cfg.SymbolID(301)
	path := constraint.NewPath(sym, "entry").Field("meta").Field("tag")
	declared := typ.NewRecord().
		Field("meta", typ.NewRecord().Field("tag", typ.LiteralString("ready")).Build()).
		Build()

	got := PointFactsOf(PointState{}).PathTypeEvidence(path, declared)

	if got.Current.State != StateUnknown {
		t.Fatalf("current evidence = %#v, want unknown", got.Current)
	}
	if got.Declared.State != StateResolved || !typ.TypeEquals(got.Declared.Type, typ.LiteralString("ready")) {
		t.Fatalf("declared evidence = %#v, want literal ready", got.Declared)
	}
}

func TestPathTypeEvidenceReturnsCurrentAndDeclaredSources(t *testing.T) {
	const sym = cfg.SymbolID(302)
	path := constraint.NewPath(sym, "entry").Field("kind")
	declared := typ.NewRecord().Field("kind", typ.LiteralString("declared")).Build()
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewRecord().
				Field("kind", typ.String).
				Build()),
		},
	}

	got := PointFactsOf(state).PathTypeEvidence(path, declared)

	if got.Current.State != StateResolved || !typ.TypeEquals(got.Current.Type, typ.String) {
		t.Fatalf("current evidence = %#v, want string", got.Current)
	}
	if got.Declared.State != StateResolved || !typ.TypeEquals(got.Declared.Type, typ.LiteralString("declared")) {
		t.Fatalf("declared evidence = %#v, want literal declared", got.Declared)
	}
}
