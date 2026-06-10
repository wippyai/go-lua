package control

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
		{"throw", Throw{}, "throw", Throw{}},
		{"diverge", Diverge{}, "diverge", Diverge{}},
		{"io", IO{}, "io", IO{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.label.EffectLabel()

			if got := tt.label.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}

			if !tt.label.Equals(tt.other) {
				t.Error("label should equal same control label")
			}

			if tt.label.Equals(returns.Return{}) {
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
		{"throw", Throws(), HasThrow},
		{"diverge", MayDiverge(), HasDiverge},
		{"io", WithIO(), HasIO},
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

func TestRowFormatting(t *testing.T) {
	tests := []struct {
		row  effect.Row
		want string
	}{
		{Throws(), "{throw}"},
		{effect.Row{Labels: []effect.Label{Throw{}, IO{}}}, "{throw, io}"},
		{effect.Open("rho", Throw{}), "{throw | rho}"},
	}

	for _, tt := range tests {
		if got := tt.row.String(); got != tt.want {
			t.Errorf("Row.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestRowFiltering(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{Throw{}, IO{}, Diverge{}}}
	filtered := r.Without(func(l effect.Label) bool {
		_, ok := l.(IO)
		return ok
	})

	if HasIO(filtered) {
		t.Error("Without should remove IO")
	}

	if !HasThrow(filtered) {
		t.Error("Without should keep Throw")
	}

	if !HasDiverge(filtered) {
		t.Error("Without should keep Diverge")
	}
}
