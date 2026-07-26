package engine

import (
	"encoding/json"
	"math"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const (
	// residueWindowPrefix carries the integer interval a computed index term
	// occupies, keyed by that term. It is a value fact like any other: the
	// term's own epoch revokes it when the body recomputes the term.
	residueWindowPrefix = "residue-window/"
	// lengthTermPrefix names the container whose length a term holds. It is the
	// only way a later operator can tell `#xs` apart from an unrelated number.
	lengthTermPrefix = "length-term/"
)

// residueWindowWire is the closed encoding of a residue window.
type residueWindowWire struct {
	Low       int64  `json:"low"`
	High      int64  `json:"high"`
	Container string `json:"container,omitempty"`
}

// residueExpressionFacts publishes the residue window an arithmetic result
// occupies, when the operator and its operands describe one. A modulo of an
// integer dividend opens a window; a constant addition or subtraction shifts an
// open one. Every other shape publishes nothing, so a term with no window is
// simply a term this domain says nothing about.
func residueExpressionFacts(operator wir.Operator, operands map[string][]byte, result, operation string, partition equation.Partition) []equation.Fact {
	window, ok := residueExpressionWindow(operator, operands, partition)
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(residueWindowWire{Low: window.Low, High: window.High, Container: window.Container})
	if err != nil {
		return nil
	}
	return []equation.Fact{{Key: residueWindowPrefix + result + "/" + operation, Value: encoded}}
}

func residueExpressionWindow(operator wir.Operator, operands map[string][]byte, partition equation.Partition) (residueWindow, bool) {
	left, right := operands["left"], operands["right"]
	switch operator {
	case wir.BinAdd, wir.BinSub:
		offset, ok := scalarIntegerConstant(right)
		if !ok {
			return residueWindow{}, false
		}
		if operator == wir.BinSub {
			if offset == math.MinInt64 {
				return residueWindow{}, false
			}
			offset = -offset
		}
		window, ok := publishedResidueWindow(left, partition)
		if !ok {
			return residueWindow{}, false
		}
		return window.shift(offset)
	case wir.BinMod:
		if !integerTypedTerm(left, partition) {
			return residueWindow{}, false
		}
		if modulus, ok := scalarIntegerConstant(right); ok {
			return constantModulusWindow(modulus)
		}
		if container, ok := publishedLengthTerm(right, partition); ok {
			return selfLengthWindow(container), true
		}
		return residueWindow{}, false
	default:
		return residueWindow{}, false
	}
}

// residueIndexPresenceProven reports that a computed index term's residue
// window lands inside the container's proven sequence prefix. It is the
// residue-domain sibling of the relational in-range proof: the window supplies
// both bounds, and the container's length floor is what they are measured
// against.
func residueIndexPresenceProven(container, key []byte, partition equation.Partition) bool {
	window, ok := publishedResidueWindow(key, partition)
	if !ok || window.Low < 1 {
		return false
	}
	floor := lengthFloorProven(container, partition)
	if floor < 1 {
		return false
	}
	if window.Container == "" {
		return window.High >= 1 && window.High <= floor
	}
	// A wrap by the read container's own length needs no relation between the
	// index and the length: `x % #c` is below `#c`, so an offset of at most one
	// keeps `(x % #c) + 1` inside the sequence.
	return window.Container == string(container) && window.High <= 0
}

// publishedResidueWindow reads the current window for a term. A window
// established before the term's latest epoch describes an earlier value of the
// same term and is not current.
func publishedResidueWindow(term []byte, partition equation.Partition) (residueWindow, bool) {
	prefix := residueWindowPrefix + string(term) + "/"
	latest, found := partition.LatestValuePrefix(prefix)
	if !found {
		return residueWindow{}, false
	}
	if epoch, versioned := currentEpoch(term, partition); versioned && epoch > factOperation(latest.Key) {
		return residueWindow{}, false
	}
	var wire residueWindowWire
	if json.Unmarshal(latest.Value, &wire) != nil {
		return residueWindow{}, false
	}
	return residueWindow{Low: wire.Low, High: wire.High, Container: wire.Container}, true
}

// publishedLengthTerm names the container a term's length was taken from.
func publishedLengthTerm(term []byte, partition equation.Partition) (string, bool) {
	container, _, found := publishedLengthTermTaken(term, partition)
	return container, found
}

// publishedLengthTermTaken names that container together with the operation the
// length was read at. A length is a snapshot of a border, so a consumer that
// measures the container against the remembered value needs the point the
// snapshot was taken as well as the container it describes.
func publishedLengthTermTaken(term []byte, partition equation.Partition) (string, string, bool) {
	prefix := lengthTermPrefix + string(term) + "/"
	latest, found := partition.LatestValuePrefix(prefix)
	if !found {
		return "", "", false
	}
	if epoch, versioned := currentEpoch(term, partition); versioned && epoch > factOperation(latest.Key) {
		return "", "", false
	}
	return string(latest.Value), factOperation(latest.Key), true
}

// scalarIntegerConstant reads the exact integer an operand term denotes.
func scalarIntegerConstant(term []byte) (int64, bool) {
	scalar, found := shapefact.DecodeScalarKind(term, shapefact.ScalarNumber)
	if !found || !numericLiteralIsInteger(string(scalar.Data)) {
		return 0, false
	}
	value, err := strconv.ParseInt(string(scalar.Data), 10, 64)
	return value, err == nil
}

// integerTypedTerm reports a term whose type is exactly Lua's integer subtype.
// Only an integer dividend has an integer residue, and only an integer index
// addresses an array slot.
func integerTypedTerm(term []byte, partition equation.Partition) bool {
	for _, candidate := range []func() (typ.Type, bool){
		func() (typ.Type, bool) {
			value, err := resolveCurrentValue(term, partition)
			if err != nil {
				return nil, false
			}
			return shapefact.DecodeExactWitnessType(value)
		},
		func() (typ.Type, bool) { return typedPathType(term, partition) },
		func() (typ.Type, bool) { return declaredTypeForTerm(term, partition) },
	} {
		if value, known := candidate(); known && value != nil {
			return typ.TypeEquals(unwrap.Alias(value), typ.Integer)
		}
	}
	return false
}

// luaFloorModulo is Lua's `%` operator: a - floor(a/b)*b. For a positive
// modulus the result lies in [0, b-1] whatever the sign of a, and for a
// negative modulus it lies in [b+1, 0]: the residue always carries the sign of
// the modulus. Every residue rule in this package reads that window from here
// rather than restating the operator's semantics.
func luaFloorModulo(a, b float64) float64 { return a - math.Floor(a/b)*b }

// residueWindow is the closed integer interval [Low, High] a residue-derived
// index term occupies. Container names the array whose length the interval is
// relative to: when it is empty the interval is absolute, and otherwise the
// real bounds are Low and `#Container + High`. The self-length form is the one
// wrap-by-own-length produces, where the upper bound is a length rather than a
// constant and no numeric relation between the two is available.
type residueWindow struct {
	Low       int64
	High      int64
	Container string
}

// shift moves the whole window by an integer offset. It refuses an offset that
// would overflow, because a wrapped bound proves nothing.
func (w residueWindow) shift(offset int64) (residueWindow, bool) {
	low, lowOK := checkedAdd(w.Low, offset)
	high, highOK := checkedAdd(w.High, offset)
	if !lowOK || !highOK {
		return residueWindow{}, false
	}
	return residueWindow{Low: low, High: high, Container: w.Container}, true
}

func checkedAdd(left, right int64) (int64, bool) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, false
	}
	return left + right, true
}

// constantModulusWindow is the interval `dividend % modulus` occupies for a
// constant modulus and an integer dividend. Lua floors, so the residue carries
// the modulus's sign: a positive modulus gives [0, modulus-1] and a negative
// one gives [modulus+1, 0]. Both are exact, so a negative modulus is described
// rather than refused; the index proof that consumes the window rejects it on
// its own lower bound.
func constantModulusWindow(modulus int64) (residueWindow, bool) {
	switch {
	case modulus > 0:
		return residueWindow{Low: 0, High: modulus - 1}, true
	case modulus < 0:
		if modulus == math.MinInt64 {
			return residueWindow{}, false
		}
		return residueWindow{Low: modulus + 1, High: 0}, true
	default:
		return residueWindow{}, false
	}
}

// selfLengthWindow is the interval `dividend % #container` occupies for an
// integer dividend. A proven positive length floor is the caller's obligation:
// `x % 0` raises rather than producing a value, so the window only describes
// runs the guard already admits.
func selfLengthWindow(container string) residueWindow {
	return residueWindow{Low: 0, High: -1, Container: container}
}

// indexResidueClass is the residue class a guard states for an index term. An
// unstated class leaves that index's ceiling untightened.
type indexResidueClass struct {
	stated  bool
	modulus int64
	residue int64
}

// indexCeilingWithinLengthFloor reports that a numeric ceiling on an index puts
// that index at or below a container's length. The proven length floor is what
// the ceiling is measured against: 1 <= i <= ceiling <= floor <= #c places i
// inside c's sequence. A stated residue class first tightens the ceiling to the
// largest member of the class the range still admits. It is the single decision
// the native element lane and the branch relation closure both ask.
func indexCeilingWithinLengthFloor(ceiling, lengthFloor int64, residue indexResidueClass) bool {
	if lengthFloor < 1 {
		return false
	}
	if residue.stated {
		tightened, ok := residueClassCeiling(ceiling, residue.modulus, residue.residue)
		if !ok {
			return false
		}
		ceiling = tightened
	}
	return ceiling >= 1 && ceiling <= lengthFloor
}

// residueClassCeiling is the largest value at or below ceiling that lies in the
// residue class `residue (mod modulus)`. It is the tightening a residue guard
// applies to an already-proven upper bound: within the range, no member of the
// class exceeds it. modulus must be positive and residue must already be
// reduced into [0, modulus-1], which is what the normalized check carries.
func residueClassCeiling(ceiling, modulus, residue int64) (int64, bool) {
	if modulus <= 0 || residue < 0 || residue >= modulus {
		return 0, false
	}
	offset, ok := checkedSub(ceiling, residue)
	if !ok {
		return 0, false
	}
	steps := offset / modulus
	if offset%modulus != 0 && offset < 0 {
		steps--
	}
	product, ok := checkedMul(steps, modulus)
	if !ok {
		return 0, false
	}
	return checkedAdd(residue, product)
}

func checkedSub(left, right int64) (int64, bool) {
	if (right < 0 && left > math.MaxInt64+right) || (right > 0 && left < math.MinInt64+right) {
		return 0, false
	}
	return left - right, true
}

func checkedMul(left, right int64) (int64, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	if left == -1 && right == math.MinInt64 || right == -1 && left == math.MinInt64 {
		return 0, false
	}
	product := left * right
	return product, product/right == left
}
