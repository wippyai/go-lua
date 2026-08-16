package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// validateProjectedResult is the complete Lua Values transport law. Preserve
// preserves the whole relation. Exact selects a finite prefix from P·alpha·S:
// a missing closed position is nil; an open tail can be short and therefore
// contributes its class, every reachable suffix position, and nil after that
// suffix is exhausted. It validates candidates independently instead of
// allocating a synthetic union or a second Values carrier.
func (d *operationDraft) validateProjectedResult(source, result valuesDraft, adjustment Adjustment) error {
	switch adjustment {
	case AdjustmentPreserve:
		if compareValues(source, result) != 0 {
			return errors.New("preserve adjustment changes Values")
		}
		return nil
	case AdjustmentExact:
		return d.validateExactProjection(source, result)
	default:
		return errors.New("invalid adjustment")
	}
}

func (d *operationDraft) validateExactProjection(source, result valuesDraft) error {
	if result.tail != ValuesClosed {
		return errors.New("exact adjustment result is not closed")
	}
	for index, destination := range result.types {
		if index < len(source.types) {
			if !d.typeAssignable(source.types[index], destination) {
				return errors.New("exact adjustment fixed source is type-incompatible")
			}
			continue
		}
		position := index - len(source.types)
		switch source.tail {
		case ValuesClosed:
			if !d.typeAccepts(typ.Nil, destination) {
				return errors.New("exact adjustment nil fill is type-incompatible")
			}
		case ValuesVariable:
			if !d.typeAssignable(source.tailType, destination) {
				return errors.New("exact adjustment tail source is type-incompatible")
			}
			for suffix := 0; suffix <= position && suffix < len(source.suffix); suffix++ {
				if !d.typeAssignable(source.suffix[suffix], destination) {
					return errors.New("exact adjustment suffix source is type-incompatible")
				}
			}
			if position >= len(source.suffix) && !d.typeAccepts(typ.Nil, destination) {
				return errors.New("exact adjustment tail nil fill is type-incompatible")
			}
		case ValuesUnknown:
			if !d.typeAccepts(typ.Any, destination) {
				return errors.New("exact adjustment unknown source is type-incompatible")
			}
			for suffix := 0; suffix <= position && suffix < len(source.suffix); suffix++ {
				if !d.typeAssignable(source.suffix[suffix], destination) {
					return errors.New("exact adjustment unknown suffix is type-incompatible")
				}
			}
			if position >= len(source.suffix) && !d.typeAccepts(typ.Nil, destination) {
				return errors.New("exact adjustment unknown nil fill is type-incompatible")
			}
		default:
			return errors.New("exact adjustment source has invalid tail")
		}
	}
	return nil
}
