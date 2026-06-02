package observation

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDeclaredPathTypeProjectsThroughDeclaredCarrier(t *testing.T) {
	const sym cfg.SymbolID = 12

	declared := flow.DeclaredTypes{
		sym: typ.NewRecord().
			Field("event", typ.NewRecord().
				Field("kind", typ.LiteralString("message")).
				StaticStringIndex("channel", typ.String).
				StaticIntIndex(1, typ.Number).
				Build()).
			Build(),
	}

	cases := []struct {
		name string
		path constraint.Path
		want typ.Type
	}{
		{
			name: "field",
			path: constraint.NewPath(sym, "root").Field("event").Field("kind"),
			want: typ.LiteralString("message"),
		},
		{
			name: "static string index",
			path: constraint.NewPath(sym, "root").Field("event").IndexStr("channel"),
			want: typ.String,
		},
		{
			name: "static int index",
			path: constraint.NewPath(sym, "root").Field("event").IndexInt(1),
			want: typ.Number,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeclaredPathType(declared, tc.path); !typ.TypeEquals(got, tc.want) {
				t.Fatalf("DeclaredPathType(%v) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
