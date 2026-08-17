package dispatch

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
	"github.com/wippyai/go-lua/domain/effect/returns"
)

func TestLabels(t *testing.T) {
	assertDispatchLabel(t, "module load", ModuleLoad{}, "module_load", ModuleLoad{}, capability.DispatchModuleLoad)
}

func assertDispatchLabel(t *testing.T, name string, label effect.Label, want string, other effect.Label, id string) {
	t.Helper()

	t.Run(name, func(t *testing.T) {
		if got := label.CapabilityID(); got != id {
			t.Errorf("CapabilityID() = %q, want %q", got, id)
		}

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
