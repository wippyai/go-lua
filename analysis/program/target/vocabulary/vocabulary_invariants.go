package vocabulary

import (
	"fmt"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

// CrossActivationOutcomeIndex maps the only outcomes that can leave an
// activation boundary to compact canonical row coordinates.
func CrossActivationOutcomeIndex(kind flowkind.OutcomeKind) (int, bool) {
	switch kind {
	case flowkind.OutcomeNormal:
		return 0, true
	case flowkind.OutcomeReturn:
		return 1, true
	case flowkind.OutcomeThrow:
		return 2, true
	case flowkind.OutcomeYield:
		return 3, true
	case flowkind.OutcomeCancel:
		return 4, true
	default:
		return 0, false
	}
}

func ValidCallbackReleaseZeroBehavior(behavior CallbackReleaseZeroBehavior) bool {
	switch behavior {
	case CallbackReleaseZeroSuppress, CallbackReleaseZeroThrow, CallbackReleaseZeroIdempotent:
		return true
	default:
		return false
	}
}

func ValidSubedgeFamily(family SubedgeFamily) bool {
	return family >= SubedgeFamilyCall && family <= SubedgeFamilyLess
}

func ValidBinding(binding BindingSpec) bool {
	if len(binding.Member) == 0 || !validSegments(binding.Member) || !bindingLengthsFit(binding) {
		return false
	}
	switch binding.Namespace {
	case BindingBuiltin:
		return len(binding.Owner) == 0
	case BindingModule, BindingProvider:
		return len(binding.Owner) != 0 && validSegments(binding.Owner)
	default:
		return false
	}
}

func validSegments(parts []string) bool {
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := CheckedStoredLength("binding segment bytes", len(part)); err != nil {
			return false
		}
	}
	return true
}

func bindingLengthsFit(binding BindingSpec) bool {
	if _, err := CheckedStoredLength("binding owner length", len(binding.Owner)); err != nil {
		return false
	}
	if _, err := CheckedStoredLength("binding member length", len(binding.Member)); err != nil {
		return false
	}
	return true
}

func CompareSegments(left, right []string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

const MaxStoredCount = uint64(^uint32(0))

// CheckedStoredLength verifies a Go length can be represented by every
// persisted target table/range that consumes it. This is a representation law,
// not a resource budget: Seal rejects an unrepresentable contract before it
// can publish a truncated handle or range.
func CheckedStoredLength(what string, length int) (uint32, error) {
	if length < 0 || uint64(length) > MaxStoredCount {
		return 0, fmt.Errorf("target: %s representation overflow", what)
	}
	return uint32(length), nil
}

func CheckedCoordinateCount(what string, count uint32) error {
	if uint64(count) > uint64(MaxIndex()) {
		return fmt.Errorf("target: %s representation overflow", what)
	}
	return nil
}

func CheckedStoredTotal(what string, lengths ...int) (int, error) {
	var total uint64
	for _, length := range lengths {
		value, err := CheckedStoredLength(what+" component", length)
		if err != nil {
			return 0, err
		}
		total += uint64(value)
		if total > MaxStoredCount || total > uint64(MaxIndex()) {
			return 0, fmt.Errorf("target: %s representation overflow", what)
		}
	}
	return int(total), nil
}

func MaxIndex() int { return int(^uint(0) >> 1) }
