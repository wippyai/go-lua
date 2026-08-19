package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/internal/framing"
)

func encodeValues(w *framing.Writer, c *Contract, values vocabulary.Values) error {
	if err := w.Record(recordValues); err != nil {
		return err
	}
	fixed := c.Operations.ValuesCount(values)
	if err := w.Count(uint64(fixed)); err != nil {
		return err
	}
	for index := 0; index < fixed; index++ {
		value, ok := c.Operations.ValuesAt(values, index)
		if !ok {
			return errors.New("target: malformed fixed Values type")
		}
		if err := encodeType(w, c, value); err != nil {
			return err
		}
	}
	tail, variable, ok := c.Operations.ValuesTail(values)
	if !ok {
		return errors.New("target: malformed Values tail")
	}
	if err := w.Uint(uint64(tail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(variable)); err != nil {
		return err
	}
	suffix := c.Operations.ValuesSuffixCount(values)
	if err := w.Count(uint64(suffix)); err != nil {
		return err
	}
	for index := 0; index < suffix; index++ {
		value, found := c.Operations.ValuesSuffixAt(values, index)
		if !found {
			return errors.New("target: malformed Values suffix type")
		}
		if err := encodeType(w, c, value); err != nil {
			return err
		}
	}
	return nil
}

func encodeType(w *framing.Writer, c *Contract, value vocabulary.Type) error {
	declaration, ok := c.Operations.TypeDeclaration(value)
	if !ok {
		return errors.New("target: malformed frozen type")
	}
	if !declaration.Available() {
		return errors.New("target: unavailable neutral type declaration")
	}
	primitive, primitiveOK := declaration.Primitive()
	if err := w.Bool(primitiveOK); err != nil {
		return err
	}
	if primitiveOK {
		if err := w.Uint(uint64(primitive)); err != nil {
			return err
		}
	} else if err := w.Uint(0); err != nil {
		return err
	}
	if err := w.Uint(uint64(declaration.ExternalFormals())); err != nil {
		return err
	}
	return w.Bytes(declaration.Bytes())
}

func encodeOutcome(w *framing.Writer, c *Contract, op vocabulary.Operation, outcome int) error {
	if err := w.Record(recordOutcome); err != nil {
		return err
	}
	kind, values, ok := c.Operations.OutcomeAt(op, outcome)
	if !ok {
		return errors.New("target: malformed outcome")
	}
	if err := w.Uint(uint64(kind)); err != nil {
		return err
	}
	if err := encodeValues(w, c, values); err != nil {
		return err
	}

	produced := c.producedCount(op, outcome)
	if err := w.Count(uint64(produced)); err != nil {
		return err
	}
	for index := 0; index < produced; index++ {
		result, target, found := c.producedAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed produced operation")
		}
		if err := w.Record(recordProduced); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(target)); err != nil {
			return err
		}
		captures := c.producedCaptureCount(op, outcome, index)
		if err := w.Count(uint64(captures)); err != nil {
			return err
		}
		for capture := 0; capture < captures; capture++ {
			kind, ordinal, found := c.producedCaptureAt(op, outcome, index, capture)
			if !found {
				return errors.New("target: malformed produced capture")
			}
			if err := w.Record(recordCapture); err != nil {
				return err
			}
			if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
				return err
			}
		}
	}

	callbackResults := c.callbackResultCount(op, outcome)
	if err := w.Count(uint64(callbackResults)); err != nil {
		return err
	}
	for index := 0; index < callbackResults; index++ {
		result, callback, found := c.callbackResultAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed callback result")
		}
		if err := w.Record(recordCallbackResult); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(callback)); err != nil {
			return err
		}
	}
	aliases := c.resultAliasCount(op, outcome)
	if err := w.Count(uint64(aliases)); err != nil {
		return err
	}
	for index := 0; index < aliases; index++ {
		result, kind, ordinal, found := c.resultAliasAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed result alias")
		}
		if err := w.Record(recordResultAlias); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := encodeCoordinate(w, uint64(kind), uint64(ordinal)); err != nil {
			return err
		}
	}
	fresh := c.FreshResultCount(op, outcome)
	if err := w.Count(uint64(fresh)); err != nil {
		return err
	}
	for index := 0; index < fresh; index++ {
		result, ordinal, kind, found := c.FreshResultAt(op, outcome, index)
		if !found {
			return errors.New("target: malformed fresh result")
		}
		if err := w.Record(recordFreshResult); err != nil {
			return err
		}
		if err := w.Uint(uint64(result)); err != nil {
			return err
		}
		if err := w.Uint(uint64(ordinal)); err != nil {
			return err
		}
		if err := w.Uint(uint64(kind)); err != nil {
			return err
		}
	}
	return nil
}

func encodeEffect(w *framing.Writer, c *Contract, op vocabulary.Operation, effect int) error {
	row, ok := c.effect(op, effect)
	if !ok {
		return errors.New("target: malformed effect")
	}
	return encodeEffectRow(w, c, row)
}

func encodeEffectRow(w *framing.Writer, c *Contract, effect effectRow) error {
	if err := w.Record(recordEffect); err != nil {
		return err
	}
	if err := w.Uint(uint64(effect.target)); err != nil {
		return err
	}
	valueArgs := effect.values.len()
	if err := w.Count(uint64(valueArgs)); err != nil {
		return err
	}
	for index := 0; index < valueArgs; index++ {
		value := c.effectVals[effect.values.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	typeArgs := effect.types.len()
	if err := w.Count(uint64(typeArgs)); err != nil {
		return err
	}
	for index := 0; index < typeArgs; index++ {
		value := c.effectType[effect.types.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	valuesArgs := effect.valuesVar.len()
	if err := w.Count(uint64(valuesArgs)); err != nil {
		return err
	}
	for index := 0; index < valuesArgs; index++ {
		value := c.effectVars[effect.valuesVar.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	rowArgs := effect.rows.len()
	if err := w.Count(uint64(rowArgs)); err != nil {
		return err
	}
	for index := 0; index < rowArgs; index++ {
		value := c.effectRows[effect.rows.start+uint32(index)]
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	if err := w.Bool(effect.hasPublication); err != nil {
		return err
	}
	if effect.hasPublication {
		if !c.validPublicationEffectRow(effect) {
			return errors.New("target: malformed publication effect selector")
		}
		if err := encodePublicationEffectDescriptor(w, effect.publication); err != nil {
			return err
		}
	}
	return nil
}

func encodePublicationEffectDescriptor(w *framing.Writer, descriptor PublicationEffectDescriptor) error {
	if !descriptor.validConsequences() {
		return errors.New("target: malformed publication effect descriptor")
	}
	if err := w.Uint(uint64(descriptor.kind)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.subject)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.destination)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.context)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.escape)); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.mutability)); err != nil {
		return err
	}
	return w.Uint(uint64(descriptor.lifetime))
}
