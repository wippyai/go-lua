package value

import (
	"math"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Not applies Lua logical-not to the complete correlated Value relation.
// Capabilities and rooted identity deliberately do not travel through a
// Boolean result. The result contains precisely true for each falsy input
// alternative and false for each truthy alternative.
func (schema *Schema) Not(input Value) (Value, bool) {
	if schema == nil || !schema.owns(input) {
		return Value{}, false
	}
	if schema.Equal(input, schema.Bottom()) {
		return input, true
	}
	includeFalse, includeTrue := false, false
	if input.top {
		includeFalse, includeTrue = true, true
	} else {
		stride := schema.stride()
		for offset := 0; offset < len(input.image); offset += stride {
			truth := schema.atomTruth(uint32(input.image[offset]))
			if truth.MayBeTrue() {
				includeFalse = true
			}
			if truth.MayBeFalse() {
				includeTrue = true
			}
		}
	}
	return schema.booleanResult(includeFalse, includeTrue)
}

// CompareEquality applies Lua ==/~= to two complete Value relations. The
// first reusable slice intentionally proves only the owner-complete nil
// partition: nil equals nil and cannot equal a non-nil value. Other present
// pairs remain may-equal/may-differ until their owning scalar/reference
// domains publish stronger generic relations. Returning both booleans is a
// conservative semantic fact, never a diagnostic fallback.
func (schema *Schema) CompareEquality(left, right Value, notEqual bool) (Value, bool) {
	if schema == nil || !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	leftPresence, rightPresence := schema.Presence(left), schema.Presence(right)
	if leftPresence == PresenceNone || rightPresence == PresenceNone {
		return schema.Bottom(), true
	}
	mayEqual := leftPresence.HasAbsent() && rightPresence.HasAbsent() ||
		leftPresence.HasPresent() && rightPresence.HasPresent()
	mayDiffer := leftPresence.HasAbsent() && rightPresence.HasPresent() ||
		leftPresence.HasPresent() && rightPresence.HasAbsent() ||
		leftPresence.HasPresent() && rightPresence.HasPresent()
	includeFalse, includeTrue := mayDiffer, mayEqual
	if notEqual {
		includeFalse, includeTrue = mayEqual, mayDiffer
	}
	return schema.booleanResult(includeFalse, includeTrue)
}

// CompareOrder applies Lua </<=/>/>= to two complete correlated Value
// relations. Exact numeric and string literals produce an exact Boolean.
// Compatible opaque scalar or metamethod-capable pairs conservatively produce
// both Booleans; incompatible pairs produce no candidate. Capabilities never
// travel into the Boolean result.
func (schema *Schema) CompareOrder(left, right Value, op flowkind.BinaryOp) (Value, bool) {
	if schema == nil || !schema.owns(left) || !schema.owns(right) || !binaryOrderOperator(op) {
		return Value{}, false
	}
	includeFalse, includeTrue := false, false
	leftOK := schema.VisitSupport(left, func(leftAtom Atom) {
		if includeFalse && includeTrue {
			return
		}
		_ = schema.VisitSupport(right, func(rightAtom Atom) {
			mayFalse, mayTrue := schema.compareOrderAtoms(leftAtom, rightAtom, op)
			includeFalse = includeFalse || mayFalse
			includeTrue = includeTrue || mayTrue
		})
	})
	if !leftOK {
		return Value{}, false
	}
	return schema.booleanResult(includeFalse, includeTrue)
}

func (schema *Schema) compareOrderAtoms(left, right Atom, op flowkind.BinaryOp) (mayFalse, mayTrue bool) {
	if schema == nil || !schema.OwnsAtom(left) || !schema.OwnsAtom(right) || !binaryOrderOperator(op) {
		return false, false
	}
	leftRow, rightRow := schema.atoms[left.id-1], schema.atoms[right.id-1]
	// NaN is the only exact non-key numeric atom. Every relational comparison
	// involving it is false, including <= and >=.
	if leftRow.kind == atomNaN || rightRow.kind == atomNaN {
		if left.RuntimeKinds().Contains(runtimekind.Number) && right.RuntimeKinds().Contains(runtimekind.Number) {
			return true, false
		}
		return false, false
	}
	leftKey, leftExact := left.ExactKey()
	rightKey, rightExact := right.ExactKey()
	if leftExact && rightExact {
		comparison, comparable := compareOrderLiterals(leftKey, rightKey)
		if !comparable {
			return false, false
		}
		truth := orderComparisonTruth(comparison, op)
		return !truth, truth
	}
	leftKinds, rightKinds := left.RuntimeKinds(), right.RuntimeKinds()
	if leftKinds.Contains(runtimekind.Number) && rightKinds.Contains(runtimekind.Number) ||
		leftKinds.Contains(runtimekind.String) && rightKinds.Contains(runtimekind.String) {
		return true, true
	}
	// Lua may delegate same-family reference order to __lt/__le. Value does
	// not own that method result, so retain both outcomes rather than inventing
	// either an error-only or definite ordering fact.
	for kind := runtimekind.Table; kind < runtimekind.Count; kind++ {
		if leftKinds.Contains(kind) && rightKinds.Contains(kind) {
			return true, true
		}
	}
	return false, false
}

func compareOrderLiterals(left, right keyspace.LiteralValue) (int, bool) {
	leftNumeric := left.Kind == keyspace.LiteralInteger || left.Kind == keyspace.LiteralFloat
	rightNumeric := right.Kind == keyspace.LiteralInteger || right.Kind == keyspace.LiteralFloat
	if leftNumeric && rightNumeric {
		leftValue, rightValue := literalNumber(left), literalNumber(right)
		if math.IsNaN(leftValue) || math.IsNaN(rightValue) {
			return 0, false
		}
		switch {
		case leftValue < rightValue:
			return -1, true
		case leftValue > rightValue:
			return 1, true
		default:
			return 0, true
		}
	}
	if left.Kind == keyspace.LiteralString && right.Kind == keyspace.LiteralString {
		switch {
		case left.String < right.String:
			return -1, true
		case left.String > right.String:
			return 1, true
		default:
			return 0, true
		}
	}
	return 0, false
}

func literalNumber(value keyspace.LiteralValue) float64 {
	if value.Kind == keyspace.LiteralInteger {
		return float64(value.Integer)
	}
	return math.Float64frombits(value.FloatBits)
}

func orderComparisonTruth(comparison int, op flowkind.BinaryOp) bool {
	switch op {
	case flowkind.BinaryLess:
		return comparison < 0
	case flowkind.BinaryLessEqual:
		return comparison <= 0
	case flowkind.BinaryGreater:
		return comparison > 0
	case flowkind.BinaryGreaterEqual:
		return comparison >= 0
	default:
		return false
	}
}

func (schema *Schema) booleanResult(includeFalse, includeTrue bool) (Value, bool) {
	if schema == nil {
		return Value{}, false
	}
	falseAtom := schema.atomByRow[atomRow{kind: atomFalse}]
	trueAtom := schema.atomByRow[atomRow{kind: atomTrue}]
	if falseAtom == 0 || trueAtom == 0 {
		return Value{}, false
	}
	atoms := make([]Atom, 0, 2)
	if includeFalse {
		atoms = append(atoms, Atom{schema: schema, id: falseAtom})
	}
	if includeTrue {
		atoms = append(atoms, Atom{schema: schema, id: trueAtom})
	}
	return schema.Alternatives(atoms...)
}

// FilterTruth retains exactly the input alternatives observable on one Lua
// truth edge. It preserves each retained atom's capability correlation; no
// projection/rebuild may smear a capability between alternatives.
func (schema *Schema) FilterTruth(input Value, truthy bool) (Value, bool) {
	if schema == nil || !schema.owns(input) {
		return Value{}, false
	}
	if schema.Equal(input, schema.Bottom()) {
		return input, true
	}
	if input.top {
		image := make([]uint64, 0, len(schema.atoms)*schema.stride())
		for index := range schema.atoms {
			truth := schema.atomTruth(uint32(index + 1))
			if (truthy && !truth.MayBeTrue()) || (!truthy && !truth.MayBeFalse()) {
				continue
			}
			row := make([]uint64, schema.stride())
			row[0] = uint64(index + 1)
			// Top denotes every capability attachment admissible for every
			// atom. Filtering by truth may remove atoms, never capability
			// possibilities of those retained atoms. A zero capability tail
			// here would be a false must-not-have conclusion after `and`/`or`.
			for word := 0; word < schema.capWords; word++ {
				row[1+word] = schema.fullCapabilityWord(word)
			}
			image = append(image, row...)
		}
		return schema.canonical(image), true
	}
	stride := schema.stride()
	image := make([]uint64, 0, len(input.image))
	for offset := 0; offset < len(input.image); offset += stride {
		truth := schema.atomTruth(uint32(input.image[offset]))
		if (truthy && !truth.MayBeTrue()) || (!truthy && !truth.MayBeFalse()) {
			continue
		}
		image = append(image, input.image[offset:offset+stride]...)
	}
	return schema.canonical(image), true
}

func (schema *Schema) fullCapabilityWord(word int) uint64 {
	if schema == nil || word < 0 || word >= schema.capWords {
		return 0
	}
	if word+1 < schema.capWords || len(schema.capabilities)%64 == 0 {
		return ^uint64(0)
	}
	return uint64(1)<<uint(len(schema.capabilities)%64) - 1
}
