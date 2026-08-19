package typ

import (
	"fmt"
	"math"
	"strconv"

	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
)

// Literal represents a singleton type containing exactly one value.
//
// Literal types enable precise type checking for constant values:
//   - LiteralBool(true) is a subtype of boolean
//   - LiteralString("foo") is a subtype of string
//   - LiteralInt(42) is a subtype of integer (and number)
//
// Base reports the underlying primitive kind (Boolean, Number, Integer,
// or String). Value reports the represented Go value.
type Literal struct {
	base  kind.Kind // Boolean, Number, Integer, or String
	value any       // bool, float64, int64, or string
	hash  uint64
	str   string
}

var (
	trueHash  = hash.MixHash(hash.MixHash(uint64(kind.Literal), uint64(kind.Boolean)), 1)
	falseHash = hash.MixHash(uint64(kind.Literal), uint64(kind.Boolean))

	trueLiteral  = &Literal{base: kind.Boolean, value: true, hash: trueHash, str: "true"}
	falseLiteral = &Literal{base: kind.Boolean, value: false, hash: falseHash, str: "false"}
)

// LiteralBool returns the canonical boolean literal type.
func LiteralBool(v bool) *Literal {
	if v {
		zzProbeConstruct(uint64(kind.Literal), trueHash) // ZZPROBE
		return trueLiteral
	}

	zzProbeConstruct(uint64(kind.Literal), falseHash) // ZZPROBE
	return falseLiteral
}

// LiteralInt creates an integer literal type.
func LiteralInt(v int64) *Literal {
	h := hash.MixHash(uint64(kind.Literal), uint64(kind.Integer))
	h = hash.MixHash(h, uint64(v))

	zzProbeConstruct(uint64(kind.Literal), h) // ZZPROBE
	return &Literal{base: kind.Integer, value: v, hash: h, str: strconv.FormatInt(v, 10)}
}

// LiteralNumber creates a number literal type.
func LiteralNumber(v float64) *Literal {
	h := hash.MixHash(uint64(kind.Literal), uint64(kind.Number))
	// A numeric literal is a type-level singleton, so its equality must be
	// reflexive even when its represented runtime value is NaN. IEEE bits are
	// the only total identity for float64: they retain NaN payloads and signed
	// zero without borrowing Go's non-reflexive floating equality.
	h = hash.MixHash(h, math.Float64bits(v))

	zzProbeConstruct(uint64(kind.Literal), h) // ZZPROBE
	return &Literal{base: kind.Number, value: v, hash: h, str: strconv.FormatFloat(v, 'g', -1, 64)}
}

// LiteralString creates a string literal type for v.
func LiteralString(v string) *Literal {
	h := hash.MixHash(uint64(kind.Literal), uint64(kind.String))
	h = hash.MixHash(h, hash.FnvString(v))

	zzProbeConstruct(uint64(kind.Literal), h) // ZZPROBE
	return &Literal{base: kind.String, value: v, hash: h, str: strconv.Quote(v)}
}

func (l *Literal) Kind() kind.Kind { return kind.Literal }

func (l *Literal) Base() kind.Kind { return l.base }

func (l *Literal) Value() any { return l.value }

func (l *Literal) String() string {
	if l.str != "" {
		return l.str
	}
	switch l.base {
	case kind.Boolean:
		if l.value.(bool) {
			return "true"
		}

		return "false"
	case kind.Integer:
		return strconv.FormatInt(l.value.(int64), 10)
	case kind.Number:
		return strconv.FormatFloat(l.value.(float64), 'g', -1, 64)
	case kind.String:
		return fmt.Sprintf("%q", l.value.(string))
	}

	return fmt.Sprintf("%v", l.value)
}

func (l *Literal) Hash() uint64 { return l.hash }

func (l *Literal) Equals(other Type) bool {
	if other.Kind() != kind.Literal {
		return false
	}

	ol := other.(*Literal)
	if l.base != ol.base {
		return false
	}
	if l.base == kind.Number {
		left, leftOK := l.value.(float64)
		right, rightOK := ol.value.(float64)
		return leftOK && rightOK && math.Float64bits(left) == math.Float64bits(right)
	}
	return l.value == ol.value
}
