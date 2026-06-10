package mutation

import "github.com/wippyai/go-lua/analysis/domain/effect"

func HasMutate(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(Mutate); return ok })
}

func GetMutate(r effect.Row, paramIdx int) *Mutate {
	for _, l := range r.Labels {
		if m, ok := l.(Mutate); ok && m.Target.Index == paramIdx {
			return &m
		}
	}
	return nil
}

func HasTableMutator(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(TableMutator); return ok })
}

func GetTableMutator(r effect.Row) *TableMutator {
	for _, l := range r.Labels {
		if mut, ok := l.(TableMutator); ok {
			return &mut
		}
	}
	return nil
}

func Mutates(paramIdx int, transform TypeTransform) effect.Row {
	return effect.Row{Labels: []effect.Label{Mutate{
		Target:    effect.ParamRef{Index: paramIdx},
		Transform: transform,
	}}}
}
