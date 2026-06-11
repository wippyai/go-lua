package ownership

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
)

func BorrowsOnly() effect.Row {
	return effect.Row{Labels: []effect.Label{BorrowAll{}}}
}

func WithBorrow(paramIdx int) effect.Row {
	return effect.Row{Labels: []effect.Label{Borrow{
		Param: effect.ParamRef{Index: paramIdx},
	}}}
}

func WithStore(paramIdx int, intoIdx int) effect.Row {
	return effect.Row{Labels: []effect.Label{Store{
		Param: effect.ParamRef{Index: paramIdx},
		Into:  effect.ParamRef{Index: intoIdx},
	}}}
}

func WithSend(fromParam int) effect.Row {
	return effect.Row{Labels: []effect.Label{Send{FromParam: fromParam}}}
}

func WithFreeze(paramIdx int) effect.Row {
	return effect.Row{Labels: []effect.Label{Freeze{
		Param: effect.ParamRef{Index: paramIdx},
	}}}
}

func HasBorrow(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool {
		if _, ok := l.(Borrow); ok {
			return true
		}
		_, ok := l.(BorrowAll)
		return ok
	})
}

func HasStore(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(Store); return ok })
}

// OnlyBorrows reports whether r contains an ownership borrow label without an
// ownership store or mutation.Mutate label.
func OnlyBorrows(r effect.Row) bool {
	return HasBorrow(r) && !HasStore(r) && !mutation.HasMutate(r)
}

func HasSend(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(Send); return ok })
}

func HasFreeze(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(Freeze); return ok })
}

func GetBorrow(r effect.Row, paramIdx int) *Borrow {
	for _, l := range r.Labels {
		if b, ok := l.(Borrow); ok && b.Param.Index == paramIdx {
			return &b
		}
	}
	return nil
}

func GetStore(r effect.Row, paramIdx int) *Store {
	for _, l := range r.Labels {
		if s, ok := l.(Store); ok && s.Param.Index == paramIdx {
			return &s
		}
	}
	return nil
}

func BorrowsAllParams(r effect.Row) bool {
	return r.Has(func(l effect.Label) bool { _, ok := l.(BorrowAll); return ok })
}
