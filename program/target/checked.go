package target

import "fmt"

const maxUint32Count = uint64(^uint32(0))

// checkedStoredLength verifies a Go length can be represented by every
// persisted target table/range that consumes it. This is a representation law,
// not a resource budget: Seal rejects an unrepresentable contract before it
// can publish a truncated handle or range.
func checkedStoredLength(what string, length int) (uint32, error) {
	if length < 0 || uint64(length) > maxUint32Count {
		return 0, fmt.Errorf("target: %s representation overflow", what)
	}
	return uint32(length), nil
}

// checkedStoredRange returns the exact range produced by appending length
// elements to a persisted pool of start elements. It also rejects a native
// index overflow before append can run.
func checkedStoredRange(what string, start, length int) (indexRange, error) {
	start32, err := checkedStoredLength(what+" start", start)
	if err != nil {
		return indexRange{}, err
	}
	length32, err := checkedStoredLength(what+" length", length)
	if err != nil {
		return indexRange{}, err
	}
	end := uint64(start32) + uint64(length32)
	if end > maxUint32Count || end > uint64(maxInt()) {
		return indexRange{}, fmt.Errorf("target: %s representation overflow", what)
	}
	return indexRange{start: start32, end: uint32(end)}, nil
}

func checkedStoredHandle(what string, current int) (uint32, error) {
	rangeAfter, err := checkedStoredRange(what, current, 1)
	if err != nil {
		return 0, err
	}
	return rangeAfter.end, nil
}

func checkedCoordinateCount(what string, count uint32) error {
	if uint64(count) > uint64(maxInt()) {
		return fmt.Errorf("target: %s representation overflow", what)
	}
	return nil
}

func checkedStoredTotal(what string, lengths ...int) (int, error) {
	var total uint64
	for _, length := range lengths {
		value, err := checkedStoredLength(what+" component", length)
		if err != nil {
			return 0, err
		}
		total += uint64(value)
		if total > maxUint32Count || total > uint64(maxInt()) {
			return 0, fmt.Errorf("target: %s representation overflow", what)
		}
	}
	return int(total), nil
}

func maxInt() int { return int(^uint(0) >> 1) }

func appendStoredRange[T any](dst *[]T, input []T, what string) (indexRange, error) {
	rangeAfter, err := checkedStoredRange(what, len(*dst), len(input))
	if err != nil {
		return indexRange{}, err
	}
	*dst = append(*dst, input...)
	return rangeAfter, nil
}
