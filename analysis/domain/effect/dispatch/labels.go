package dispatch

import "github.com/wippyai/go-lua/analysis/domain/effect"

var (
	_ effect.Label = ModuleLoad{}
	_ effect.Label = VariadicTransform{}
	_ effect.Label = TypePredicate{}
	_ effect.Label = TypeValueMethod{}
	_ effect.Label = CallableType{}
)

type ModuleLoad struct{}

func (ModuleLoad) EffectLabel()   {}
func (ModuleLoad) String() string { return "module_load" }
func (ModuleLoad) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(ModuleLoad)
	return ok
}

type VariadicTransform struct{}

func (VariadicTransform) EffectLabel()   {}
func (VariadicTransform) String() string { return "variadic_transform" }
func (VariadicTransform) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(VariadicTransform)
	return ok
}

type TypePredicate struct{}

func (TypePredicate) EffectLabel()   {}
func (TypePredicate) String() string { return "type_predicate" }
func (TypePredicate) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(TypePredicate)
	return ok
}

type TypeValueMethod struct{}

func (TypeValueMethod) EffectLabel()   {}
func (TypeValueMethod) String() string { return "type_value_method" }
func (TypeValueMethod) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(TypeValueMethod)
	return ok
}

type CallableType struct{}

func (CallableType) EffectLabel()   {}
func (CallableType) String() string { return "callable_type" }
func (CallableType) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(CallableType)
	return ok
}
