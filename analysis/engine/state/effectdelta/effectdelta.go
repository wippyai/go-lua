package effectdelta

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type Site string

type Kind uint8

const (
	Mutation Kind = iota + 1
	Escape
	Call
)

type Change uint8

const (
	ChangeBottom Change = iota
	ChangeNone
	ChangeChanged
	ChangeUnknown
)

type Key struct {
	Target pathdom.PathKey
	Site   Site
	Kind   Kind
}

type Value struct {
	Before product.Value
	After  product.Value
	Change Change
}

func Domain(reg *axis.Registry) lattice.Lattice[Value] {
	valueDomain := product.Domain(reg)
	return lattice.Lattice[Value]{
		Bottom: func() Value { return Bottom(reg) },
		Top:    Top,
		Equal: func(a, b Value) bool {
			if a.Change == ChangeBottom || b.Change == ChangeBottom {
				return a.Change == ChangeBottom && b.Change == ChangeBottom
			}
			return valueDomain.Equal(a.Before, b.Before) &&
				valueDomain.Equal(a.After, b.After) &&
				a.Change == b.Change
		},
		LessOrEq: func(a, b Value) bool {
			if a.Change == ChangeBottom {
				return true
			}
			if b.Change == ChangeBottom {
				return false
			}
			return valueDomain.LessOrEq(a.Before, b.Before) &&
				valueDomain.LessOrEq(a.After, b.After) &&
				changeLessOrEq(a.Change, b.Change)
		},
		Join: func(a, b Value) Value {
			if a.Change == ChangeBottom {
				return b
			}
			if b.Change == ChangeBottom {
				return a
			}
			return Value{
				Before: valueDomain.Join(a.Before, b.Before),
				After:  valueDomain.Join(a.After, b.After),
				Change: changeJoin(a.Change, b.Change),
			}
		},
		Widen: func(prev, next Value) Value {
			if prev.Change == ChangeBottom {
				return next
			}
			if next.Change == ChangeBottom {
				return prev
			}
			return Value{
				Before: valueDomain.Widen(prev.Before, next.Before),
				After:  valueDomain.Widen(prev.After, next.After),
				Change: changeJoin(prev.Change, next.Change),
			}
		},
	}
}

func MapDomain(reg *axis.Registry) lattice.Lattice[map[Key]Value] {
	return lift.Map[Key, Value](Domain(reg))
}

func Bottom(reg *axis.Registry) Value {
	if reg == nil {
		return Value{Change: ChangeBottom}
	}
	return Value{
		Before: product.Bottom(reg),
		After:  product.Bottom(reg),
		Change: ChangeBottom,
	}
}

func Top() Value {
	return Value{
		Before: product.Top(),
		After:  product.Top(),
		Change: ChangeUnknown,
	}
}

func CloneMap(in map[Key]Value) map[Key]Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[Key]Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func DeleteEntry(in map[Key]Value, key Key) (map[Key]Value, bool) {
	if _, ok := in[key]; !ok {
		return in, false
	}
	out := make(map[Key]Value, len(in)-1)
	for k, v := range in {
		if k != key {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func changeLessOrEq(a, b Change) bool {
	return a == b || a == ChangeBottom || b == ChangeUnknown
}

func changeJoin(a, b Change) Change {
	if a == b {
		return a
	}
	if a == ChangeBottom {
		return b
	}
	if b == ChangeBottom {
		return a
	}
	return ChangeUnknown
}
