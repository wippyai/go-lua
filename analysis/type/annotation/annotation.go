package annotation

import (
	"math"

	"github.com/wippyai/go-lua/analysis/internal/hash"
)

type payloadKind uint8

const (
	payloadNone payloadKind = iota
	payloadString
	payloadBool
	payloadInt
	payloadInt64
	payloadFloat64
)

// Payload is the closed set of scalar annotation arguments.
type Payload struct {
	kind payloadKind
	s    string
	b    bool
	i    int
	i64  int64
	f64  float64
}

// Annotation describes a runtime validation payload attached to a type wrapper.
type Annotation struct {
	Name string
	Arg  Payload
}

// StringArg returns a string annotation payload.
func StringArg(v string) Payload {
	return Payload{kind: payloadString, s: v}
}

// BoolArg returns a bool annotation payload.
func BoolArg(v bool) Payload {
	return Payload{kind: payloadBool, b: v}
}

// IntArg returns an int annotation payload.
func IntArg(v int) Payload {
	return Payload{kind: payloadInt, i: v}
}

// Int64Arg returns an int64 annotation payload.
func Int64Arg(v int64) Payload {
	return Payload{kind: payloadInt64, i64: v}
}

// Float64Arg returns a float64 annotation payload.
func Float64Arg(v float64) Payload {
	return Payload{kind: payloadFloat64, f64: v}
}

// IsNone reports whether the payload is absent.
func (p Payload) IsNone() bool {
	return p.kind == payloadNone
}

// AsString returns the string payload, if present.
func (p Payload) AsString() (string, bool) {
	if p.kind != payloadString {
		return "", false
	}
	return p.s, true
}

// AsBool returns the bool payload, if present.
func (p Payload) AsBool() (bool, bool) {
	if p.kind != payloadBool {
		return false, false
	}
	return p.b, true
}

// AsInt returns the int payload, if present.
func (p Payload) AsInt() (int, bool) {
	if p.kind != payloadInt {
		return 0, false
	}
	return p.i, true
}

// AsInt64 returns the int64 payload, if present.
func (p Payload) AsInt64() (int64, bool) {
	if p.kind != payloadInt64 {
		return 0, false
	}
	return p.i64, true
}

// AsFloat64 returns the float64 payload, if present.
func (p Payload) AsFloat64() (float64, bool) {
	if p.kind != payloadFloat64 {
		return 0, false
	}
	return p.f64, true
}

// Equal reports whether two payloads are structurally equal.
func (p Payload) Equal(other Payload) bool {
	if p.kind != other.kind {
		return false
	}
	switch p.kind {
	case payloadNone:
		return true
	case payloadString:
		return p.s == other.s
	case payloadBool:
		return p.b == other.b
	case payloadInt:
		return p.i == other.i
	case payloadInt64:
		return p.i64 == other.i64
	case payloadFloat64:
		return math.Float64bits(p.f64) == math.Float64bits(other.f64)
	default:
		return false
	}
}

// Hash returns a stable identity hash aligned with Equal.
func (p Payload) Hash() uint64 {
	switch p.kind {
	case payloadNone:
		return hash.FnvString("<nil>")
	case payloadString:
		h := hash.FnvString("string")
		return hash.MixHash(h, hash.FnvString(p.s))
	case payloadBool:
		h := hash.FnvString("bool")
		if p.b {
			return hash.MixHash(h, 1)
		}
		return hash.MixHash(h, 0)
	case payloadInt:
		h := hash.FnvString("int")
		return hash.MixHash(h, uint64(int64(p.i)))
	case payloadInt64:
		h := hash.FnvString("int64")
		return hash.MixHash(h, uint64(p.i64))
	case payloadFloat64:
		h := hash.FnvString("float64")
		return hash.MixHash(h, math.Float64bits(p.f64))
	default:
		return hash.FnvString("<invalid>")
	}
}

// Equal reports whether two annotations carry the same validation payload.
func (a Annotation) Equal(other Annotation) bool {
	return a.Name == other.Name && a.Arg.Equal(other.Arg)
}

// Hash returns a stable identity hash for the annotation payload.
func (a Annotation) Hash() uint64 {
	h := hash.FnvString(a.Name)
	return hash.MixHash(h, a.Arg.Hash())
}
