package typewitness

import (
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type state uint8

const (
	bottom state = iota
	concrete
	top
)

// Value carries exact type evidence proven by runtime type witnesses.
type Value struct {
	state state
	t     typ.Type
}

func Bottom() Value { return Value{state: bottom} }
func Top() Value    { return Value{state: top} }

func Of(t typ.Type) Value {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || inspect.ContainsTypeParam(t) {
		return Top()
	}
	return Value{state: concrete, t: t}
}

func (v Value) IsBottom() bool { return v.state == bottom }
func (v Value) IsTop() bool    { return v.state == top }

func (v Value) Type() (typ.Type, bool) {
	if v.state != concrete || v.t == nil {
		return nil, false
	}
	return v.t, true
}

func Join(a, b Value) Value {
	if a.state == bottom {
		return b
	}
	if b.state == bottom {
		return a
	}
	if a.state == top || b.state == top {
		return Top()
	}
	if typ.SameNodeOrAcyclicEqual(a.t, b.t) {
		return a
	}
	return Top()
}

func Meet(a, b Value) Value {
	if a.state == bottom || b.state == bottom {
		return Bottom()
	}
	if a.state == top {
		return b
	}
	if b.state == top {
		return a
	}
	if typ.SameNodeOrAcyclicEqual(a.t, b.t) {
		return a
	}
	return Bottom()
}

func Equal(a, b Value) bool {
	if a.state != b.state {
		return false
	}
	if a.state != concrete {
		return true
	}
	return typ.SameNodeOrAcyclicEqual(a.t, b.t)
}

func (v Value) Hash() uint64 {
	h := internal.MixHash(internal.FnvString("typewitness"), uint64(v.state))
	if v.state == concrete && v.t != nil {
		h = internal.MixHash(h, typ.EqualityHash(v.t))
	}
	return h
}
