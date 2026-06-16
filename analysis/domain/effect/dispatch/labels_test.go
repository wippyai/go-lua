package dispatch

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
)

func TestLabels(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
		want  string
		other effect.Label
	}{
		{"module load", ModuleLoad{}, "module_load", ModuleLoad{}},
		{"variadic transform", VariadicTransform{}, "variadic_transform", VariadicTransform{}},
		{"type predicate", TypePredicate{}, "type_predicate", TypePredicate{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.label.EffectLabel()

			if got := tt.label.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}

			if !tt.label.Equals(tt.other) {
				t.Error("label should equal same dispatch label")
			}

			if tt.label.Equals(returns.Return{}) {
				t.Error("label should not equal unrelated effect label")
			}
		})
	}
}
