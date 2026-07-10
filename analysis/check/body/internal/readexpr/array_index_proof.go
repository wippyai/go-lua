package readexpr

import (
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ArrayIndexTerm is the affine integer index coeff*value(path)+offset.
type ArrayIndexTerm struct {
	Path   pathdom.Path
	Coeff  int64
	Offset int64
}

// ArrayIndexProof supplies one caller's resolved evidence for an array index.
// ProveArrayIndexInBounds owns the proof order and arithmetic while callers
// retain ownership of their visibility and state-read policies.
type ArrayIndexProof struct {
	IsArrayLength       bool
	IsModuloArrayLength bool
	HasConstant         bool
	Constant            int64
	HasTerm             bool
	Term                ArrayIndexTerm

	ArrayLengthAtLeastOne   func() bool
	LengthKnownAtLeastOne   func() bool
	LengthAtLeast           func(int64) bool
	UpperBoundLengthAtLeast func(int64) bool
	NumericFloor            func(pathdom.Path) (int64, bool)
	NumericCeil             func(pathdom.Path) (int64, bool)
	DiffProvesLELength      func(ArrayIndexTerm) bool
	IndexInRange            func(pathdom.Path) bool
}

// ProveArrayIndexInBounds reports whether the supplied evidence proves a Lua
// array index is positive and at most the container length. It recognizes
// #array, (integer % #array) + 1, integer constants, affine terms, and exact
// index-in-range facts.
func ProveArrayIndexInBounds(proof ArrayIndexProof) bool {
	if proof.LengthAtLeast == nil {
		return false
	}
	if proof.IsArrayLength {
		return proof.ArrayLengthAtLeastOne != nil && proof.ArrayLengthAtLeastOne()
	}
	if proof.IsModuloArrayLength {
		return proof.LengthKnownAtLeastOne != nil && proof.LengthKnownAtLeastOne()
	}
	if proof.HasConstant {
		return proof.Constant >= 1 && proof.LengthAtLeast(proof.Constant)
	}
	if !proof.HasTerm || proof.Term.Coeff <= 0 || proof.Term.Path.IsEmpty() || proof.NumericFloor == nil {
		return false
	}
	lower, ok := proof.NumericFloor(proof.Term.Path)
	if !ok {
		return false
	}
	minimum, ok := CheckedAffineInt64(proof.Term.Coeff, lower, proof.Term.Offset)
	if !ok || minimum < 1 {
		return false
	}
	if proof.DiffProvesLELength != nil && proof.DiffProvesLELength(proof.Term) {
		return true
	}
	upperLengthAtLeast := proof.UpperBoundLengthAtLeast
	if upperLengthAtLeast == nil {
		upperLengthAtLeast = proof.LengthAtLeast
	}
	if proof.NumericCeil != nil {
		if upperBound, ok := proof.NumericCeil(proof.Term.Path); ok {
			if maximum, ok := CheckedAffineInt64(proof.Term.Coeff, upperBound, proof.Term.Offset); ok &&
				maximum >= 1 && upperLengthAtLeast(maximum) {
				return true
			}
		}
	}
	return proof.Term.Coeff == 1 && proof.Term.Offset == 0 &&
		proof.IndexInRange != nil && proof.IndexInRange(proof.Term.Path)
}

// CheckedAffineInt64 evaluates coeff*value+offset without overflow.
func CheckedAffineInt64(coeff, value, offset int64) (int64, bool) {
	if coeff != 0 && (value > math.MaxInt64/coeff || value < math.MinInt64/coeff) {
		return 0, false
	}
	product := coeff * value
	if (offset > 0 && product > math.MaxInt64-offset) || (offset < 0 && product < math.MinInt64-offset) {
		return 0, false
	}
	return product + offset, true
}

// SequenceKnownNonEmpty reports whether every possible sequence arm has at
// least one required element.
func SequenceKnownNonEmpty(t typ.Type) bool {
	return SequenceLengthKnownAtLeast(t, 1)
}

// SequenceLengthKnownAtLeast reports whether every possible sequence arm has
// at least floor required elements.
func SequenceLengthKnownAtLeast(t typ.Type, floor int64) bool {
	return sequenceLengthKnownAtLeast(t, floor, 0)
}

func sequenceLengthKnownAtLeast(t typ.Type, floor int64, depth int) bool {
	if floor <= 0 {
		return true
	}
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Tuple:
		return int64(len(tt.Elements)) >= floor
	case *typ.Record:
		for i := int64(1); i <= floor; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return false
			}
		}
		return true
	case *typ.Optional:
		return sequenceLengthKnownAtLeast(tt.Inner, floor, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !sequenceLengthKnownAtLeast(member, floor, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// ArrayIndexElementsNonNil reports whether every reachable array element that
// can be selected by an in-range read excludes nil.
func ArrayIndexElementsNonNil(t typ.Type) bool {
	return arrayIndexElementsNonNil(t, 0)
}

func arrayIndexElementsNonNil(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		elem := tt.Element
		if elem == nil {
			elem = typ.Unknown
		}
		return !typevalue.TypeIncludesNil(elem)
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return false
		}
		for _, elem := range tt.Elements {
			if elem == nil {
				elem = typ.Unknown
			}
			if typevalue.TypeIncludesNil(elem) {
				return false
			}
		}
		return true
	case *typ.Record:
		var found bool
		for i := int64(1); ; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return found
			}
			found = true
			if typevalue.TypeIncludesNil(member.Type) {
				return false
			}
		}
	case *typ.Optional:
		return arrayIndexElementsNonNil(tt.Inner, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		reachable := false
		for _, member := range tt.Members {
			if !arrayIndexCanHaveElement(member, depth+1) {
				continue
			}
			reachable = true
			if !arrayIndexElementsNonNil(member, depth+1) {
				return false
			}
		}
		return reachable
	default:
		return false
	}
}

func arrayIndexCanHaveElement(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Tuple:
		return len(tt.Elements) > 0
	case *typ.Record:
		member := tt.GetStaticIntIndex(1)
		return member != nil && !member.Optional
	case *typ.Optional:
		return arrayIndexCanHaveElement(tt.Inner, depth+1)
	case *typ.Union:
		for _, member := range tt.Members {
			if arrayIndexCanHaveElement(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
