package unwrap

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestAlias(t *testing.T) {
	t.Run("preserves optional", func(t *testing.T) {
		opt := typ.NewOptional(typ.String)
		alias := typ.NewAlias("OptString", opt)
		result := Alias(alias)
		if _, ok := result.(*typ.Optional); !ok {
			t.Error("Alias should preserve Optional")
		}
	})
}

func TestOptional(t *testing.T) {
	t.Run("nested", func(t *testing.T) {
		inner := typ.NewOptional(typ.String)
		outer := typ.NewOptional(inner)
		result := Optional(outer)
		if result != typ.String {
			t.Error("Optional should fully unwrap nested optionals")
		}
	})
}

func TestIsOptionalLike(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil type", nil, true},
		{"Nil", typ.Nil, true},
		{"Optional", typ.NewOptional(typ.String), true},
		{"Union with nil", typ.NewUnion(typ.String, typ.Nil), true},
		{"Any", typ.Any, true},
		{"Unknown", typ.Unknown, true},
		{"String", typ.String, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOptionalLike(tt.t); got != tt.want {
				t.Errorf("IsOptionalLike() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBuiltinTableTop(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	nonTableIface := typ.NewInterface("Reader", nil)
	aliasedTable := typ.NewAlias("TTable", tableTop)

	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"builtin table marker", tableTop, true},
		{"aliased builtin table marker", aliasedTable, true},
		{"non-table interface", nonTableIface, false},
		{"string", typ.String, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBuiltinTableTop(tt.t); got != tt.want {
				t.Errorf("IsBuiltinTableTop() = %v, want %v", got, tt.want)
			}
		})
	}
}
