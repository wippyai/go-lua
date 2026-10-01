package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestContextualSynthesisFinalizesFlowResults(t *testing.T) {
	for _, mode := range []subtype.Assignability{subtype.Gradual, subtype.Strict} {
		t.Run(map[subtype.Assignability]string{subtype.Gradual: "gradual", subtype.Strict: "strict"}[mode], func(t *testing.T) {
			const sym = cfg.SymbolID(1)
			ident := &ast.IdentExpr{Value: "context"}
			bindings := bind.NewBindingTable()
			bindings.Bind(ident, sym)
			pending := typ.NewRecord().Field("node_id", typ.NewUnion(typ.Any, typ.Unresolved)).
				MapComponent(typ.Any, typ.Any).SetComplete(true).Build()
			ctx := db.NewQueryContext(db.New())
			core.WithAssignability(ctx, mode)
			engine := New(Config{
				Ctx: ctx, Types: mockTypeQuerier{}, Scopes: make(api.ScopeMap),
				Flow: mockFlowOps{narrowed: map[cfg.SymbolID]typ.Type{sym: pending}},
				Env: api.NewNarrowEnv(api.NarrowEnvConfig{
					Graph:    mockGraph{symbols: map[string]cfg.SymbolID{"context": sym}},
					Bindings: bindings, DeclaredTypes: flow.DeclaredTypes{sym: pending},
				}), Phase: api.PhaseNarrowing,
			})
			want := typ.Finalize(pending)
			for _, query := range []struct {
				name   string
				typeOf func() typ.Type
			}{
				{"ordinary", func() typ.Type { return engine.TypeOf(ident, 0) }},
				{"contextual", func() typ.Type { return engine.TypeOfWithExpected(ident, 0, typ.NewMap(typ.String, typ.Any)) }},
			} {
				got := query.typeOf()
				if !typ.IsFinal(got) || !typ.TypeEquals(got, want) {
					t.Errorf("%s result = %v, want final %v", query.name, got, want)
				}
			}
		})
	}
}
