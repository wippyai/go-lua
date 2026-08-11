package source

import (
	"math"

	"github.com/wippyai/go-lua/program/keyspace"
)

// exactKeyRef is the allocation-free representation shared by the ordinary
// Source key plane and artifact preflight. Public literals carry text in text;
// artifact readers carry the same bytes in bytes so validation never has to
// allocate a Go string before the complete payload has passed preflight.
type exactKeyRef struct {
	kind    keyspace.LiteralKind
	boolean bool
	integer int64
	float   uint64
	text    string
	bytes   []byte
}

func exactKeyRefFromLiteral(value keyspace.LiteralValue) exactKeyRef {
	return exactKeyRef{
		kind:    value.Kind,
		boolean: value.Bool,
		integer: value.Integer,
		float:   value.FloatBits,
		text:    value.String,
	}
}

func exactKeyRefValue(ref exactKeyRef, copyString bool) keyspace.LiteralValue {
	value := keyspace.LiteralValue{Kind: ref.kind, Bool: ref.boolean, Integer: ref.integer, FloatBits: ref.float}
	if copyString {
		if ref.bytes != nil {
			value.String = string(ref.bytes)
		} else {
			value.String = ref.text
		}
	}
	return value
}

func exactKeyRefStringLen(ref exactKeyRef) int {
	if ref.bytes != nil {
		return len(ref.bytes)
	}
	return len(ref.text)
}

func exactKeyRefStringByte(ref exactKeyRef, index int) byte {
	if ref.bytes != nil {
		return ref.bytes[index]
	}
	return ref.text[index]
}

// NormalizeExactKey returns the unique Lua equality representative for one
// literal table-key atom. Integral floats, including both signed zeroes, are
// integer keys. NaN has no exact-key identity. Irrelevant payload fields are
// discarded from every successful result.
func NormalizeExactKey(value keyspace.LiteralValue) (keyspace.LiteralValue, bool) {
	normalized, ok := normalizeExactKeyRef(exactKeyRefFromLiteral(value))
	if !ok {
		return keyspace.LiteralValue{}, false
	}
	return exactKeyRefValue(normalized, true), true
}

func normalizeExactKeyRef(value exactKeyRef) (exactKeyRef, bool) {
	switch value.kind {
	case keyspace.LiteralBool:
		return exactKeyRef{kind: keyspace.LiteralBool, boolean: value.boolean}, true
	case keyspace.LiteralInteger:
		return exactKeyRef{kind: keyspace.LiteralInteger, integer: value.integer}, true
	case keyspace.LiteralFloat:
		floating := math.Float64frombits(value.float)
		if math.IsNaN(floating) {
			return exactKeyRef{}, false
		}
		if !math.IsInf(floating, 0) && math.Trunc(floating) == floating &&
			floating >= -9223372036854775808.0 && floating < 9223372036854775808.0 {
			return exactKeyRef{kind: keyspace.LiteralInteger, integer: int64(floating)}, true
		}
		return exactKeyRef{kind: keyspace.LiteralFloat, float: value.float}, true
	case keyspace.LiteralString:
		return exactKeyRef{kind: keyspace.LiteralString, text: value.text, bytes: value.bytes}, true
	default:
		return exactKeyRef{}, false
	}
}

// CompareExactKey orders two Lua exact-key atoms by their canonical equality
// representatives. It rejects NaN, nil, and unknown literals instead of
// inventing an order.
func CompareExactKey(left, right keyspace.LiteralValue) (int, bool) {
	leftRef, leftOK := normalizeExactKeyRef(exactKeyRefFromLiteral(left))
	rightRef, rightOK := normalizeExactKeyRef(exactKeyRefFromLiteral(right))
	if !leftOK || !rightOK {
		return 0, false
	}
	return compareCanonicalExactKey(leftRef, rightRef), true
}

// compareCanonicalExactKey orders already-normalized exact-key atoms. It is
// the sealing-only fast path; callers establish canonical ingress first.
func compareCanonicalExactKey(left, right exactKeyRef) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	switch left.kind {
	case keyspace.LiteralBool:
		if left.boolean == right.boolean {
			return 0
		}
		if !left.boolean {
			return -1
		}
		return 1
	case keyspace.LiteralInteger:
		if left.integer < right.integer {
			return -1
		}
		if left.integer > right.integer {
			return 1
		}
	case keyspace.LiteralFloat:
		leftValue := math.Float64frombits(left.float)
		rightValue := math.Float64frombits(right.float)
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	case keyspace.LiteralString:
		leftLength := exactKeyRefStringLen(left)
		rightLength := exactKeyRefStringLen(right)
		limit := leftLength
		if rightLength < limit {
			limit = rightLength
		}
		for index := 0; index < limit; index++ {
			leftByte := exactKeyRefStringByte(left, index)
			rightByte := exactKeyRefStringByte(right, index)
			if leftByte < rightByte {
				return -1
			}
			if leftByte > rightByte {
				return 1
			}
		}
		if leftLength < rightLength {
			return -1
		}
		if leftLength > rightLength {
			return 1
		}
	}
	return 0
}

func exactKeyRefEqual(left, right exactKeyRef) bool {
	if left.kind != right.kind {
		return false
	}
	switch left.kind {
	case keyspace.LiteralBool:
		return left.boolean == right.boolean
	case keyspace.LiteralInteger:
		return left.integer == right.integer
	case keyspace.LiteralFloat:
		return left.float == right.float
	case keyspace.LiteralString:
		if exactKeyRefStringLen(left) != exactKeyRefStringLen(right) {
			return false
		}
		for index := 0; index < exactKeyRefStringLen(left); index++ {
			if exactKeyRefStringByte(left, index) != exactKeyRefStringByte(right, index) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
