package target

import (
	"errors"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
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

	produced := c.Operations.ProducedCount(op, outcome)
	if err := w.Count(uint64(produced)); err != nil {
		return err
	}
	for index := 0; index < produced; index++ {
		result, target, found := c.Operations.ProducedAt(op, outcome, index)
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
		captures := c.Operations.ProducedCaptureCount(op, outcome, index)
		if err := w.Count(uint64(captures)); err != nil {
			return err
		}
		for capture := 0; capture < captures; capture++ {
			kind, ordinal, found := c.Operations.ProducedCaptureAt(op, outcome, index, capture)
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

	callbackResults := c.Operations.CallbackResultCount(op, outcome)
	if err := w.Count(uint64(callbackResults)); err != nil {
		return err
	}
	for index := 0; index < callbackResults; index++ {
		result, callback, found := c.Operations.CallbackResultAt(op, outcome, index)
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
	aliases := c.Operations.ResultAliasCount(op, outcome)
	if err := w.Count(uint64(aliases)); err != nil {
		return err
	}
	for index := 0; index < aliases; index++ {
		result, kind, ordinal, found := c.Operations.ResultAliasAt(op, outcome, index)
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
	fresh := c.Operations.FreshResultCount(op, outcome)
	if err := w.Count(uint64(fresh)); err != nil {
		return err
	}
	for index := 0; index < fresh; index++ {
		result, ordinal, kind, found := c.Operations.FreshResultAt(op, outcome, index)
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
	target, ok := c.Operations.EffectTarget(op, effect)
	if !ok {
		return errors.New("target: malformed effect")
	}
	return encodeEffectArguments(w, c, target,
		func(index int) (vocabulary.ValueFormal, bool) {
			return c.Operations.EffectValueArgumentAt(op, effect, index)
		},
		func(index int) (vocabulary.TypeFormal, bool) {
			return c.Operations.EffectTypeArgumentAt(op, effect, index)
		},
		func(index int) (vocabulary.ValuesVar, bool) {
			return c.Operations.EffectValuesArgumentAt(op, effect, index)
		},
		func(index int) (vocabulary.RowVar, bool) { return c.Operations.EffectRowArgumentAt(op, effect, index) },
		func() (operationvalue.PublicationEffectDescriptor, bool) {
			return c.Operations.EffectPublication(op, effect)
		})
}

func encodeCallbackEffect(w *framing.Writer, c *Contract, callback vocabulary.CallbackID, effect int) error {
	target, ok := c.Operations.CallbackEffectTarget(callback, effect)
	if !ok {
		return errors.New("target: malformed callback effect")
	}
	return encodeEffectArguments(w, c, target,
		func(index int) (vocabulary.ValueFormal, bool) {
			return c.Operations.CallbackEffectValueArgumentAt(callback, effect, index)
		},
		func(index int) (vocabulary.TypeFormal, bool) {
			return c.Operations.CallbackEffectTypeArgumentAt(callback, effect, index)
		},
		func(index int) (vocabulary.ValuesVar, bool) {
			return c.Operations.CallbackEffectValuesArgumentAt(callback, effect, index)
		},
		func(index int) (vocabulary.RowVar, bool) {
			return c.Operations.CallbackEffectRowArgumentAt(callback, effect, index)
		},
		func() (operationvalue.PublicationEffectDescriptor, bool) {
			return c.Operations.CallbackEffectPublication(callback, effect)
		})
}

func encodeEffectArguments(w *framing.Writer, c *Contract, target vocabulary.Operation,
	valueAt func(int) (vocabulary.ValueFormal, bool), typeAt func(int) (vocabulary.TypeFormal, bool),
	valuesVarAt func(int) (vocabulary.ValuesVar, bool), rowAt func(int) (vocabulary.RowVar, bool),
	publication func() (operationvalue.PublicationEffectDescriptor, bool)) error {
	if err := w.Record(recordEffect); err != nil {
		return err
	}
	if err := w.Uint(uint64(target)); err != nil {
		return err
	}
	valueArgs := 0
	if targetValues, ok := c.Operations.Input(target); ok {
		valueArgs = c.Operations.ValuesCount(targetValues)
	}
	if err := w.Count(uint64(valueArgs)); err != nil {
		return err
	}
	for index := 0; index < valueArgs; index++ {
		value, ok := valueAt(index)
		if !ok {
			return errors.New("target: malformed effect value argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	typeArgs := c.Operations.TypeFormalCount(target)
	if err := w.Count(uint64(typeArgs)); err != nil {
		return err
	}
	for index := 0; index < typeArgs; index++ {
		value, ok := typeAt(index)
		if !ok {
			return errors.New("target: malformed effect type argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	valuesArgs := c.Operations.ValuesVarCount(target)
	if err := w.Count(uint64(valuesArgs)); err != nil {
		return err
	}
	for index := 0; index < valuesArgs; index++ {
		value, ok := valuesVarAt(index)
		if !ok {
			return errors.New("target: malformed effect Values argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	rowArgs := c.Operations.RowFormalCount(target)
	if err := w.Count(uint64(rowArgs)); err != nil {
		return err
	}
	for index := 0; index < rowArgs; index++ {
		value, ok := rowAt(index)
		if !ok {
			return errors.New("target: malformed effect row argument")
		}
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	descriptor, hasPublication := publication()
	if err := w.Bool(hasPublication); err != nil {
		return err
	}
	if hasPublication {
		if !descriptor.Valid() {
			return errors.New("target: malformed publication effect selector")
		}
		if err := encodePublicationEffectDescriptor(w, descriptor); err != nil {
			return err
		}
	}
	return nil
}

func encodePublicationEffectDescriptor(w *framing.Writer, descriptor operationvalue.PublicationEffectDescriptor) error {
	if !descriptor.Valid() {
		return errors.New("target: malformed publication effect descriptor")
	}
	if err := w.Uint(uint64(descriptor.Kind())); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.Subject())); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.DestinationRole())); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.Context())); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.Escape())); err != nil {
		return err
	}
	if err := w.Uint(uint64(descriptor.Mutability())); err != nil {
		return err
	}
	return w.Uint(uint64(descriptor.Lifetime()))
}
