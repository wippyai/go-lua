package control

import "github.com/wippyai/go-lua/analysis/domain/effect"

func HasThrow(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(Throw); return ok })
}

func HasIO(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(IO); return ok })
}

func HasDiverge(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(Diverge); return ok })
}

func Throws() effect.Row {
	return effect.Row{Labels: []effect.Label{Throw{}}}
}

func WithIO() effect.Row {
	return effect.Row{Labels: []effect.Label{IO{}}}
}

func MayDiverge() effect.Row {
	return effect.Row{Labels: []effect.Label{Diverge{}}}
}
