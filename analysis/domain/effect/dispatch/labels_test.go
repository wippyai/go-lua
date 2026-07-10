package dispatch

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
)

func TestLabels(t *testing.T) {
	assertDispatchLabel(t, "module load", ModuleLoad{}, "module_load", ModuleLoad{})
}

func assertDispatchLabel(t *testing.T, name string, label effect.Label, want string, other effect.Label) {
	t.Helper()

	t.Run(name, func(t *testing.T) {
		label.EffectLabel()

		if got := label.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}

		if !label.Equals(other) {
			t.Error("label should equal same dispatch label")
		}

		if label.Equals(returns.Return{}) {
			t.Error("label should not equal unrelated effect label")
		}
	})
}
