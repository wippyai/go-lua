package target

import (
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// validateProjectedResult is the complete Lua Values transport law. Preserve
// preserves the whole relation. Exact selects a finite prefix from P·alpha·S:
// a missing closed position is nil; an open tail can be short and therefore
// contributes its class, every reachable suffix position, and nil after that
// suffix is exhausted. It validates candidates independently instead of
// allocating a synthetic union or a second Values carrier.
func (d *operationDraft) validateProjectedResult(source, result valuesDraft, adjustment vocabulary.Adjustment) error {
	switch adjustment {
	case vocabulary.AdjustmentPreserve:
		if compareValues(source, result) != 0 {
			return errors.New("preserve adjustment changes Values")
		}
		return nil
	case vocabulary.AdjustmentExact:
		return d.validateExactProjection(source, result)
	default:
		return errors.New("invalid adjustment")
	}
}

func (d *operationDraft) validateExactProjection(source, result valuesDraft) error {
	if result.tail != vocabulary.ValuesClosed {
		return errors.New("exact adjustment result is not closed")
	}
	for index, destination := range result.types {
		if index < len(source.types) {
			sourceType, sourceOK := d.declarations[source.types[index]]
			destinationType, destinationOK := d.declarations[destination]
			if !sourceOK || !destinationOK {
				return fmt.Errorf("exact adjustment fixed source type relation: type declaration is not admitted")
			}
			assignable, relationErr := d.semantics.Assignable(sourceType, destinationType, d.formalConstraints)
			if relationErr != nil {
				return fmt.Errorf("exact adjustment fixed source type relation: %w", relationErr)
			}
			if !assignable {
				return errors.New("exact adjustment fixed source is type-incompatible")
			}
			continue
		}
		position := index - len(source.types)
		switch source.tail {
		case vocabulary.ValuesClosed:
			nilType, ok := schematype.NewPrimitive(schematype.PrimitiveNil)
			if !ok {
				return errors.New("exact adjustment nil primitive unavailable")
			}
			destinationType, destinationOK := d.declarations[destination]
			if !destinationOK {
				return fmt.Errorf("exact adjustment nil fill type relation: type declaration is not admitted")
			}
			accepted, relationErr := d.semantics.Assignable(nilType, destinationType, d.formalConstraints)
			if relationErr != nil {
				return fmt.Errorf("exact adjustment nil fill type relation: %w", relationErr)
			}
			if !accepted {
				return errors.New("exact adjustment nil fill is type-incompatible")
			}
		case vocabulary.ValuesVariable:
			sourceType, sourceOK := d.declarations[source.tailType]
			destinationType, destinationOK := d.declarations[destination]
			if !sourceOK || !destinationOK {
				return fmt.Errorf("exact adjustment tail source type relation: type declaration is not admitted")
			}
			assignable, relationErr := d.semantics.Assignable(sourceType, destinationType, d.formalConstraints)
			if relationErr != nil {
				return fmt.Errorf("exact adjustment tail source type relation: %w", relationErr)
			}
			if !assignable {
				return errors.New("exact adjustment tail source has unavailable type")
			}
			for suffix := 0; suffix <= position && suffix < len(source.suffix); suffix++ {
				sourceType, sourceOK := d.declarations[source.suffix[suffix]]
				destinationType, destinationOK := d.declarations[destination]
				if !sourceOK || !destinationOK {
					return fmt.Errorf("exact adjustment suffix type relation: type declaration is not admitted")
				}
				assignable, relationErr := d.semantics.Assignable(sourceType, destinationType, d.formalConstraints)
				if relationErr != nil {
					return fmt.Errorf("exact adjustment suffix type relation: %w", relationErr)
				}
				if !assignable {
					return errors.New("exact adjustment suffix source is type-incompatible")
				}
			}
			if position >= len(source.suffix) {
				nilType, ok := schematype.NewPrimitive(schematype.PrimitiveNil)
				if !ok {
					return errors.New("exact adjustment nil primitive unavailable")
				}
				destinationType, destinationOK := d.declarations[destination]
				if !destinationOK {
					return fmt.Errorf("exact adjustment tail nil fill type relation: type declaration is not admitted")
				}
				accepted, relationErr := d.semantics.Assignable(nilType, destinationType, d.formalConstraints)
				if relationErr != nil {
					return fmt.Errorf("exact adjustment tail nil fill type relation: %w", relationErr)
				}
				if !accepted {
					return errors.New("exact adjustment tail nil fill is type-incompatible")
				}
			}
		case vocabulary.ValuesUnknown:
			anyType, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
			if !ok {
				return errors.New("exact adjustment any primitive unavailable")
			}
			destinationType, destinationOK := d.declarations[destination]
			if !destinationOK {
				return fmt.Errorf("exact adjustment unknown source type relation: type declaration is not admitted")
			}
			accepted, relationErr := d.semantics.Assignable(anyType, destinationType, d.formalConstraints)
			if relationErr != nil {
				return fmt.Errorf("exact adjustment unknown source type relation: %w", relationErr)
			}
			if !accepted {
				return errors.New("exact adjustment unknown source is type-incompatible")
			}
			for suffix := 0; suffix <= position && suffix < len(source.suffix); suffix++ {
				sourceType, sourceOK := d.declarations[source.suffix[suffix]]
				destinationType, destinationOK := d.declarations[destination]
				if !sourceOK || !destinationOK {
					return fmt.Errorf("exact adjustment unknown suffix type relation: type declaration is not admitted")
				}
				assignable, relationErr := d.semantics.Assignable(sourceType, destinationType, d.formalConstraints)
				if relationErr != nil {
					return fmt.Errorf("exact adjustment unknown suffix type relation: %w", relationErr)
				}
				if !assignable {
					return errors.New("exact adjustment unknown suffix is type-incompatible")
				}
			}
			if position >= len(source.suffix) {
				nilType, ok := schematype.NewPrimitive(schematype.PrimitiveNil)
				if !ok {
					return errors.New("exact adjustment nil primitive unavailable")
				}
				destinationType, destinationOK := d.declarations[destination]
				if !destinationOK {
					return fmt.Errorf("exact adjustment unknown nil fill type relation: type declaration is not admitted")
				}
				accepted, relationErr := d.semantics.Assignable(nilType, destinationType, d.formalConstraints)
				if relationErr != nil {
					return fmt.Errorf("exact adjustment unknown nil fill type relation: %w", relationErr)
				}
				if !accepted {
					return errors.New("exact adjustment unknown nil fill is type-incompatible")
				}
			}
		default:
			return errors.New("exact adjustment source has invalid tail")
		}
	}
	return nil
}
