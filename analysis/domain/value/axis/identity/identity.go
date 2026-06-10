package identity

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

type state uint8

const (
	bottom state = iota
	singleton
	top
)

// ID is a stable runtime identity token for allocation, object, closure, or
// other value-producing sites. It intentionally carries no type-domain node.
type ID struct {
	Kind  string
	Site  string
	Index uint64
}

func (id ID) String() string {
	return id.Kind + ":" + id.Site + "#" + strconv.FormatUint(id.Index, 10)
}

func (id ID) hash() uint64 {
	h := internal.FnvString("identity.id")
	h = internal.HashCombine(h, internal.FnvString(id.Kind))
	h = internal.HashCombine(h, internal.FnvString(id.Site))
	return internal.HashCombine(h, id.Index)
}

// Value is the runtime identity axis. Its lattice is flat:
//
//	Bottom < Singleton(ID) < Top
//
// Distinct singleton identities are incomparable and join to Top.
type Value struct {
	state state
	id    ID
}

func Bottom() Value {
	return Value{state: bottom}
}

func Top() Value {
	return Value{state: top}
}

func Singleton(id ID) Value {
	return Value{state: singleton, id: id}
}

func (v Value) IsBottom() bool {
	return v.state == bottom
}

func (v Value) IsTop() bool {
	return v.state == top
}

func (v Value) ID() (ID, bool) {
	if v.state != singleton {
		return ID{}, false
	}
	return v.id, true
}

func Equal(a, b Value) bool {
	if a.state != b.state {
		return false
	}
	if a.state != singleton {
		return true
	}
	return a.id == b.id
}

func LessOrEq(a, b Value) bool {
	if Equal(a, b) {
		return true
	}
	return a.state == bottom || b.state == top
}

func (v Value) Covers(other Value) bool {
	return LessOrEq(other, v)
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
	if a.id == b.id {
		return a
	}
	return Top()
}

func Widen(prev, next Value) Value {
	return Join(prev, next)
}

func Hash(v Value) uint64 {
	return v.Hash()
}

func (v Value) Hash() uint64 {
	h := internal.HashCombine(internal.FnvString("identity"), uint64(v.state))
	if v.state == singleton {
		h = internal.HashCombine(h, v.id.hash())
	}
	return h
}

func String(v Value) string {
	return v.String()
}

func (v Value) String() string {
	switch v.state {
	case bottom:
		return "bottom"
	case singleton:
		return "singleton(" + v.id.String() + ")"
	case top:
		return "top"
	default:
		return "identity(invalid)"
	}
}

var Key = axis.NewKey[Value]("identity")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:      Key,
		Bottom:   Bottom,
		Top:      Top,
		Equal:    Equal,
		LessOrEq: LessOrEq,
		Join:     Join,
		Widen:    Widen,
		Hash:     Hash,
	}
}
