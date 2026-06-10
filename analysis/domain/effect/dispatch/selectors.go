package dispatch

import "github.com/wippyai/go-lua/analysis/domain/effect"

func HasModuleLoad(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(ModuleLoad); return ok })
}

func HasVariadicTransform(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(VariadicTransform); return ok })
}

func HasTypePredicate(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(TypePredicate); return ok })
}

func HasTypeValueMethod(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(TypeValueMethod); return ok })
}

func HasCallableType(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(CallableType); return ok })
}

func WithModuleLoad() effect.Row {
	return effect.Row{Labels: []effect.Label{ModuleLoad{}}}
}

func WithVariadicTransform() effect.Row {
	return effect.Row{Labels: []effect.Label{VariadicTransform{}}}
}

func WithTypePredicate() effect.Row {
	return effect.Row{Labels: []effect.Label{TypePredicate{}}}
}

func WithTypeValueMethod() effect.Row {
	return effect.Row{Labels: []effect.Label{TypeValueMethod{}}}
}

func WithCallableType() effect.Row {
	return effect.Row{Labels: []effect.Label{CallableType{}}}
}
