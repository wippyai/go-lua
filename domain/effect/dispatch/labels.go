package dispatch

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
)

var (
	_ effect.Label = ModuleLoad{}
)

type ModuleLoad struct{}

func (ModuleLoad) CapabilityID() string { return capability.DispatchModuleLoad }
func (ModuleLoad) String() string       { return "module_load" }
func (ModuleLoad) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(ModuleLoad)
	return ok
}
