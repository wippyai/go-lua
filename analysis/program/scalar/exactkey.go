// Package scalar owns Program's canonical table-key equality and ordering.
package scalar

import (
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Ref is an allocation-free literal view. Text and Bytes are alternative
// representations of the same string payload; Bytes is used while validating
// an artifact before allocating an owned Go string.
type Ref struct {
	Kind      keyspace.LiteralKind
	Bool      bool
	Integer   int64
	FloatBits uint64
	Text      string
	Bytes     []byte
}

// FromLiteral returns a reference to an ordinary literal value.
func FromLiteral(value keyspace.LiteralValue) Ref {
	return Ref{
		Kind:      value.Kind,
		Bool:      value.Bool,
		Integer:   value.Integer,
		FloatBits: value.FloatBits,
		Text:      value.String,
	}
}

// Literal materializes a literal. When copyString is false, string payload is
// omitted so artifact preflight can validate without allocating.
func (ref Ref) Literal(copyString bool) keyspace.LiteralValue {
	value := keyspace.LiteralValue{Kind: ref.Kind, Bool: ref.Bool, Integer: ref.Integer, FloatBits: ref.FloatBits}
	if copyString {
		if ref.Bytes != nil {
			value.String = string(ref.Bytes)
		} else {
			value.String = ref.Text
		}
	}
	return value
}

func (ref Ref) stringLen() int {
	if ref.Bytes != nil {
		return len(ref.Bytes)
	}
	return len(ref.Text)
}

func (ref Ref) stringByte(index int) byte {
	if ref.Bytes != nil {
		return ref.Bytes[index]
	}
	return ref.Text[index]
}

// Normalize returns the unique Lua equality representative for one literal
// table-key atom. Integral floats, including both signed zeroes, are integer
// keys. NaN has no exact-key identity.
func Normalize(value keyspace.LiteralValue) (keyspace.LiteralValue, bool) {
	normalized, ok := NormalizeRef(FromLiteral(value))
	if !ok {
		return keyspace.LiteralValue{}, false
	}
	return normalized.Literal(true), true
}

// NormalizeRef is the allocation-free form of Normalize.
func NormalizeRef(value Ref) (Ref, bool) {
	switch value.Kind {
	case keyspace.LiteralBool:
		return Ref{Kind: keyspace.LiteralBool, Bool: value.Bool}, true
	case keyspace.LiteralInteger:
		return Ref{Kind: keyspace.LiteralInteger, Integer: value.Integer}, true
	case keyspace.LiteralFloat:
		floating := math.Float64frombits(value.FloatBits)
		if math.IsNaN(floating) {
			return Ref{}, false
		}
		if !math.IsInf(floating, 0) && math.Trunc(floating) == floating &&
			floating >= -9223372036854775808.0 && floating < 9223372036854775808.0 {
			return Ref{Kind: keyspace.LiteralInteger, Integer: int64(floating)}, true
		}
		return Ref{Kind: keyspace.LiteralFloat, FloatBits: value.FloatBits}, true
	case keyspace.LiteralString:
		return Ref{Kind: keyspace.LiteralString, Text: value.Text, Bytes: value.Bytes}, true
	default:
		return Ref{}, false
	}
}

// Compare orders two Lua exact-key atoms by their canonical equality
// representatives. It rejects NaN, nil, and unknown literals.
func Compare(left, right keyspace.LiteralValue) (int, bool) {
	leftRef, leftOK := NormalizeRef(FromLiteral(left))
	rightRef, rightOK := NormalizeRef(FromLiteral(right))
	if !leftOK || !rightOK {
		return 0, false
	}
	return CompareCanonical(leftRef, rightRef), true
}

// CompareCanonical orders already-normalized exact-key references.
func CompareCanonical(left, right Ref) int {
	if left.Kind < right.Kind {
		return -1
	}
	if left.Kind > right.Kind {
		return 1
	}
	switch left.Kind {
	case keyspace.LiteralBool:
		if left.Bool == right.Bool {
			return 0
		}
		if !left.Bool {
			return -1
		}
		return 1
	case keyspace.LiteralInteger:
		if left.Integer < right.Integer {
			return -1
		}
		if left.Integer > right.Integer {
			return 1
		}
	case keyspace.LiteralFloat:
		leftValue := math.Float64frombits(left.FloatBits)
		rightValue := math.Float64frombits(right.FloatBits)
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	case keyspace.LiteralString:
		limit := left.stringLen()
		if right.stringLen() < limit {
			limit = right.stringLen()
		}
		for index := 0; index < limit; index++ {
			if left.stringByte(index) < right.stringByte(index) {
				return -1
			}
			if left.stringByte(index) > right.stringByte(index) {
				return 1
			}
		}
		if left.stringLen() < right.stringLen() {
			return -1
		}
		if left.stringLen() > right.stringLen() {
			return 1
		}
	}
	return 0
}
