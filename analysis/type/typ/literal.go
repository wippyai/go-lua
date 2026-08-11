package typ

import (
	"fmt"
	"math"
	"strconv"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Literal represents a singleton type containing exactly one value.
//
// Literal types enable precise type checking for constant values:
//   - LiteralBool(true) is a subtype of boolean
//   - LiteralString("foo") is a subtype of string
//   - LiteralInt(42) is a subtype of integer (and number)
//
// Base indicates the underlying primitive kind (Boolean, Number, Integer, String).
// Value holds the actual value with the appropriate Go type.
type Literal struct {
	Base  kind.Kind // Boolean, Number, Integer, or String
	Value any       // bool, float64, int64, or string
	hash  uint64
	str   string
}

// True and False are singleton boolean literals.
var (
	trueHash  = hash.MixHash(hash.MixHash(uint64(kind.Literal), uint64(kind.Boolean)), 1)
	falseHash = hash.MixHash(uint64(kind.Literal), uint64(kind.Boolean))

	True  = &Literal{Base: kind.Boolean, Value: true, hash: trueHash, str: "true"}
	False = &Literal{Base: kind.Boolean, Value: false, hash: falseHash, str: "false"}
)

// LiteralBool returns the canonical boolean literal type.
func LiteralBool(v bool) *Literal {
	if v {
		return True
	}

	return False
}

// LiteralInt creates an integer literal type.
func LiteralInt(v int64) *Literal {
	h := hash.MixHash(uint64(kind.Literal), uint64(kind.Integer))
	h = hash.MixHash(h, uint64(v))

	return &Literal{Base: kind.Integer, Value: v, hash: h, str: strconv.FormatInt(v, 10)}
}

// LiteralNumber creates a number literal type.
func LiteralNumber(v float64) *Literal {
	h := hash.MixHash(uint64(kind.Literal), uint64(kind.Number))
	// A numeric literal is a type-level singleton, so its equality must be
	// reflexive even when its represented runtime value is NaN. IEEE bits are
	// the only total identity for float64: they retain NaN payloads and signed
	// zero without borrowing Go's non-reflexive floating equality.
	h = hash.MixHash(h, math.Float64bits(v))

	return &Literal{Base: kind.Number, Value: v, hash: h, str: strconv.FormatFloat(v, 'g', -1, 64)}
}

// LiteralString creates a string literal type for v.
func LiteralString(v string) *Literal {
	h := hash.MixHash(uint64(kind.Literal), uint64(kind.String))
	h = hash.MixHash(h, hash.FnvString(v))

	return &Literal{Base: kind.String, Value: v, hash: h, str: strconv.Quote(v)}
}

func (l *Literal) Kind() kind.Kind { return kind.Literal }

func (l *Literal) String() string {
	if l.str != "" {
		return l.str
	}
	switch l.Base {
	case kind.Boolean:
		if l.Value.(bool) {
			return "true"
		}

		return "false"
	case kind.Integer:
		return strconv.FormatInt(l.Value.(int64), 10)
	case kind.Number:
		return strconv.FormatFloat(l.Value.(float64), 'g', -1, 64)
	case kind.String:
		return fmt.Sprintf("%q", l.Value.(string))
	}

	return fmt.Sprintf("%v", l.Value)
}

func (l *Literal) Hash() uint64 { return l.hash }

func (l *Literal) Equals(other Type) bool {
	if other.Kind() != kind.Literal {
		return false
	}

	ol := other.(*Literal)
	if l.Base != ol.Base {
		return false
	}
	if l.Base == kind.Number {
		left, leftOK := l.Value.(float64)
		right, rightOK := ol.Value.(float64)
		return leftOK && rightOK && math.Float64bits(left) == math.Float64bits(right)
	}
	return l.Value == ol.Value
}
