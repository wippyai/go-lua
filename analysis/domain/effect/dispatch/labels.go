package dispatch

import "github.com/wippyai/go-lua/analysis/domain/effect"

var (
	_ effect.Label = ModuleLoad{}
)

type ModuleLoad struct{}

func (ModuleLoad) EffectLabel()   {}
func (ModuleLoad) String() string { return "module_load" }
func (ModuleLoad) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(ModuleLoad)
	return ok
}
