package dispatch

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
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
		{"type value method", TypeValueMethod{}, "type_value_method", TypeValueMethod{}},
		{"callable type", CallableType{}, "callable_type", CallableType{}},
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

			if tt.label.Equals(effect.Return{}) {
				t.Error("label should not equal unrelated effect label")
			}
		})
	}
}

func TestSelectors(t *testing.T) {
	tests := []struct {
		name string
		row  effect.Row
		has  func(effect.Row) bool
	}{
		{"module load", WithModuleLoad(), HasModuleLoad},
		{"variadic transform", WithVariadicTransform(), HasVariadicTransform},
		{"type predicate", WithTypePredicate(), HasTypePredicate},
		{"type value method", WithTypeValueMethod(), HasTypeValueMethod},
		{"callable type", WithCallableType(), HasCallableType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.has(tt.row) {
				t.Error("selector should find constructor label")
			}

			if tt.has(effect.Empty) {
				t.Error("selector should not match empty row")
			}
		})
	}
}
