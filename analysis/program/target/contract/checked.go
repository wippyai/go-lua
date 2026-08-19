package contract

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// checkedStoredRange returns the exact range produced by appending length
// elements to a persisted pool of start elements. It also rejects a native
// index overflow before append can run.
func checkedStoredRange(what string, start, length int) (indexRange, error) {
	start32, err := vocabulary.CheckedStoredLength(what+" start", start)
	if err != nil {
		return indexRange{}, err
	}
	length32, err := vocabulary.CheckedStoredLength(what+" length", length)
	if err != nil {
		return indexRange{}, err
	}
	end := uint64(start32) + uint64(length32)
	if end > vocabulary.MaxStoredCount || end > uint64(vocabulary.MaxIndex()) {
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

func appendStoredRange[T any](dst *[]T, input []T, what string) (indexRange, error) {
	rangeAfter, err := checkedStoredRange(what, len(*dst), len(input))
	if err != nil {
		return indexRange{}, err
	}
	*dst = append(*dst, input...)
	return rangeAfter, nil
}
