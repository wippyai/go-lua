package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// descriptor while retaining distinct occurrence evidence.
func (c *Contract) sealEffectIdentities() error {
	c.effectOperationIDs = make([]identity.ContentID, len(c.operations))
	for index := range c.operations {
		op := vocabulary.Operation(index + 1)
		anchor, anchorOK := c.anchor(op)
		if !anchorOK {
			return errors.New("target: missing effect operation anchor")
		}
		id, err := c.semanticID(semanticEffectOperation, func(w *framing.Writer) error {
			if err := w.Bytes(anchor[:]); err != nil {
				return err
			}
			return c.encodeEffectOperationABI(w, op)
		})
		if err != nil {
			return err
		}
		c.effectOperationIDs[index] = id
	}

	c.effectDescriptorIDs = make([]identity.ContentID, len(c.effects))
	c.effectOccurrenceIDs = make([]identity.ContentID, len(c.effects))
	c.operationEffectFamilies = make([]identity.ContentID, len(c.operations))
	c.callbackEffectFamilies = make([]identity.ContentID, len(c.callbacks))

	for index, row := range c.operations {
		op := vocabulary.Operation(index + 1)
		if err := c.sealEffectRow(op, 0, row.effects); err != nil {
			return err
		}
		id, err := c.effectFamilyID(semanticOperationEffectFamily, op, 0, row.effectTail, row.effectVar, row.effects)
		if err != nil {
			return err
		}
		c.operationEffectFamilies[index] = id
	}
	for index, row := range c.callbacks {
		callback := vocabulary.CallbackID(index + 1)
		owner, ownerOK := c.Operations.CallbackOwner(callback)
		if !ownerOK {
			return errors.New("target: malformed effect callback owner")
		}
		if err := c.sealEffectRow(owner, callback, row.effects); err != nil {
			return err
		}
		id, err := c.effectFamilyID(semanticCallbackEffectFamily, owner, callback, row.effectTail, row.effectVar, row.effects)
		if err != nil {
			return err
		}
		c.callbackEffectFamilies[index] = id
	}
	return nil
}

// encodeEffectOperationABI writes exactly the operation-scoped ABI that an
// effect substitution can observe.  Outcomes, callbacks, effects,
// transfers, and every other operation relation are intentionally absent.
func (c *Contract) encodeEffectOperationABI(w *framing.Writer, op vocabulary.Operation) error {
	if _, ok := c.operation(op); !ok {
		return errors.New("target: malformed effect operation")
	}
	input, inputOK := c.Operations.Input(op)
	if !inputOK {
		return errors.New("target: malformed effect operation input")
	}
	if err := encodeValues(w, c, input); err != nil {
		return err
	}
	typeFormals := c.Operations.TypeFormalCount(op)
	if err := w.Count(uint64(typeFormals)); err != nil {
		return err
	}
	for index := 0; index < typeFormals; index++ {
		value, valueOK := c.Operations.TypeFormalConstraint(op, vocabulary.TypeFormal(index))
		if !valueOK {
			value = 0
		}
		if err := w.Bool(value != 0); err != nil {
			return err
		}
		if value != 0 {
			if err := encodeType(w, c, value); err != nil {
				return err
			}
		}
	}
	valuesVars := c.Operations.ValuesVarCount(op)
	if err := w.Count(uint64(valuesVars)); err != nil {
		return err
	}
	for index := 0; index < valuesVars; index++ {
		value, valueOK := c.Operations.ValuesVarType(op, vocabulary.ValuesVar(index))
		if !valueOK {
			return errors.New("target: malformed effect Values ABI")
		}
		if err := encodeType(w, c, value); err != nil {
			return err
		}
	}
	return w.Count(uint64(c.Operations.RowFormalCount(op)))
}

func (c *Contract) sealEffectRow(owner vocabulary.Operation, callback vocabulary.CallbackID, effects indexRange) error {
	if owner == 0 || uint64(owner) > uint64(len(c.operations)) || !validIdentityRange(effects, len(c.effects)) {
		return errors.New("target: malformed effect row range")
	}
	if callback != 0 {
		callbackOwner, ownerOK := c.Operations.CallbackOwner(callback)
		if uint64(callback) > uint64(len(c.callbacks)) || !ownerOK || callbackOwner != owner ||
			uint64(callback) > uint64(len(c.callbackContentIDs)) || !c.callbackContentIDs[callback-1].Available() {
			return errors.New("target: malformed effect callback")
		}
	}
	for local, position := 0, effects.start; position < effects.end; local, position = local+1, position+1 {
		row := c.effects[position]
		descriptor, err := c.effectDescriptorID(owner, row)
		if err != nil {
			return err
		}
		occurrence, err := c.sealEffectOccurrenceID(owner, callback, uint32(local), descriptor)
		if err != nil {
			return err
		}
		c.effectDescriptorIDs[position] = descriptor
		c.effectOccurrenceIDs[position] = occurrence
	}
	return nil
}

func (c *Contract) effectDescriptorID(owner vocabulary.Operation, row effectRow) (identity.ContentID, error) {
	if owner == 0 || uint64(owner) > uint64(len(c.effectOperationIDs)) || row.target == 0 || uint64(row.target) > uint64(len(c.effectOperationIDs)) {
		return identity.ContentID{}, errors.New("target: malformed effect descriptor owner")
	}
	ownerID := c.effectOperationIDs[owner-1]
	targetID := c.effectOperationIDs[row.target-1]
	if !ownerID.Available() || !targetID.Available() {
		return identity.ContentID{}, errors.New("target: missing effect descriptor operation identity")
	}
	id, err := c.semanticID(semanticEffectDescriptor, func(w *framing.Writer) error {
		if err := w.Bytes(ownerID[:]); err != nil {
			return err
		}
		if err := w.Bytes(targetID[:]); err != nil {
			return err
		}
		return c.encodeEffectArguments(w, row)
	})
	return id, err
}

func (c *Contract) encodeEffectArguments(w *framing.Writer, row effectRow) error {
	if !validIdentityRange(row.values, len(c.effectVals)) ||
		!validIdentityRange(row.types, len(c.effectType)) ||
		!validIdentityRange(row.valuesVar, len(c.effectVars)) ||
		!validIdentityRange(row.rows, len(c.effectRows)) {
		return errors.New("target: malformed effect descriptor arguments")
	}
	if err := w.Count(uint64(row.values.len())); err != nil {
		return err
	}
	for position := row.values.start; position < row.values.end; position++ {
		if err := w.Uint(uint64(c.effectVals[position])); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.types.len())); err != nil {
		return err
	}
	for position := row.types.start; position < row.types.end; position++ {
		if err := w.Uint(uint64(c.effectType[position])); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.valuesVar.len())); err != nil {
		return err
	}
	for position := row.valuesVar.start; position < row.valuesVar.end; position++ {
		if err := w.Uint(uint64(c.effectVars[position])); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.rows.len())); err != nil {
		return err
	}
	for position := row.rows.start; position < row.rows.end; position++ {
		if err := w.Uint(uint64(c.effectRows[position])); err != nil {
			return err
		}
	}
	if err := w.Bool(row.hasPublication); err != nil {
		return err
	}
	if row.hasPublication {
		if !c.validPublicationEffectRow(row) {
			return errors.New("target: malformed publication effect selector")
		}
		if err := encodePublicationEffectDescriptor(w, row.publication); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) sealEffectOccurrenceID(owner vocabulary.Operation, callback vocabulary.CallbackID, local uint32, descriptor identity.ContentID) (identity.ContentID, error) {
	if owner == 0 || uint64(owner) > uint64(len(c.effectOperationIDs)) || !descriptor.Available() {
		return identity.ContentID{}, errors.New("target: malformed effect occurrence")
	}
	ownerID := c.effectOperationIDs[owner-1]
	if !ownerID.Available() {
		return identity.ContentID{}, errors.New("target: missing effect occurrence owner identity")
	}
	id, err := c.semanticID(semanticEffectOccurrence, func(w *framing.Writer) error {
		kind := uint64(1)
		if callback != 0 {
			kind = 2
		}
		if err := w.Uint(kind); err != nil {
			return err
		}
		if err := w.Bytes(ownerID[:]); err != nil {
			return err
		}
		if callback != 0 {
			if uint64(callback) > uint64(len(c.callbackContentIDs)) {
				return errors.New("target: malformed effect occurrence callback")
			}
			callbackID := c.callbackContentIDs[callback-1]
			if !callbackID.Available() {
				return errors.New("target: missing effect occurrence callback identity")
			}
			if err := w.Bytes(callbackID[:]); err != nil {
				return err
			}
		}
		if err := w.Uint(uint64(local)); err != nil {
			return err
		}
		return w.Bytes(descriptor[:])
	})
	return id, err
}

func (c *Contract) effectFamilyID(kind uint64, owner vocabulary.Operation, callback vocabulary.CallbackID, tail vocabulary.RowTail, variable vocabulary.RowVar, effects indexRange) (identity.ContentID, error) {
	if owner == 0 || uint64(owner) > uint64(len(c.effectOperationIDs)) || !validIdentityRange(effects, len(c.effects)) {
		return identity.ContentID{}, errors.New("target: malformed effect family")
	}
	ownerID := c.effectOperationIDs[owner-1]
	if !ownerID.Available() {
		return identity.ContentID{}, errors.New("target: missing effect family owner identity")
	}
	id, err := c.semanticID(kind, func(w *framing.Writer) error {
		if callback != 0 {
			if uint64(callback) > uint64(len(c.callbackContentIDs)) {
				return errors.New("target: malformed callback effect family")
			}
			callbackID := c.callbackContentIDs[callback-1]
			if !callbackID.Available() {
				return errors.New("target: missing callback effect family identity")
			}
			if err := w.Bytes(callbackID[:]); err != nil {
				return err
			}
		} else if err := w.Bytes(ownerID[:]); err != nil {
			return err
		}
		if err := w.Uint(uint64(tail)); err != nil {
			return err
		}
		if err := w.Uint(uint64(variable)); err != nil {
			return err
		}
		if err := w.Count(uint64(effects.len())); err != nil {
			return err
		}
		for position := effects.start; position < effects.end; position++ {
			occurrence := c.effectOccurrenceIDs[position]
			if !occurrence.Available() {
				return errors.New("target: missing effect family occurrence identity")
			}
			if err := w.Bytes(occurrence[:]); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

func (c *Contract) EffectOperationID(op vocabulary.Operation) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.effectOperationIDs) {
		return identity.ContentID{}, false
	}
	return c.effectOperationIDs[op-1], true
}

// EffectDescriptorID returns the semantic quotient for one ordinary
// operation effect occurrence.  Equal descriptors may intentionally occur at
// several local positions; use EffectOccurrenceID when occurrence evidence is
// required.
func (c *Contract) EffectDescriptorID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return identity.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectDescriptorIDs) {
		return identity.ContentID{}, false
	}
	id := c.effectDescriptorIDs[position]
	return id, id.Available()
}

// PublicationEffectDescriptorID returns the existing canonical effect
// descriptor identity only when that exact occurrence carries an explicit
// publication descriptor. Reusing the effect descriptor ID prevents a second
// parallel semantic identity for the same sealed occurrence.
func (c *Contract) PublicationEffectDescriptorID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	if _, ok := c.PublicationEffectDescriptor(op, index); !ok {
		return identity.ContentID{}, false
	}
	return c.EffectDescriptorID(op, index)
}

// PublicationEffectOccurrenceID returns the existing canonical exact effect
// occurrence identity only when that occurrence carries explicit publication
// semantics.
func (c *Contract) PublicationEffectOccurrenceID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	if _, ok := c.PublicationEffectDescriptor(op, index); !ok {
		return identity.ContentID{}, false
	}
	return c.effectOccurrenceID(op, index)
}

// CallbackEffectDescriptorID returns the semantic quotient for one callback
// effect occurrence.  It has no inverse because duplicate descriptors are a
// deliberate quotient of distinct retained occurrences.
func (c *Contract) CallbackEffectDescriptorID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	row, ok := c.callback(callback)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return identity.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectDescriptorIDs) {
		return identity.ContentID{}, false
	}
	id := c.effectDescriptorIDs[position]
	return id, id.Available()
}

// CallbackPublicationEffectDescriptorID returns the canonical generic effect
// descriptor identity for one callback occurrence with explicit publication
// semantics.
func (c *Contract) CallbackPublicationEffectDescriptorID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	if _, ok := c.CallbackPublicationEffectDescriptor(callback, index); !ok {
		return identity.ContentID{}, false
	}
	return c.CallbackEffectDescriptorID(callback, index)
}

// CallbackPublicationEffectOccurrenceID returns the canonical exact callback
// effect occurrence identity for an explicitly declared publication effect.
func (c *Contract) CallbackPublicationEffectOccurrenceID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	if _, ok := c.CallbackPublicationEffectDescriptor(callback, index); !ok {
		return identity.ContentID{}, false
	}
	return c.callbackEffectOccurrenceID(callback, index)
}

// effectOccurrenceID returns the exact ordinary effect occurrence identity,
// including its canonical local row position.
func (c *Contract) effectOccurrenceID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return identity.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectOccurrenceIDs) {
		return identity.ContentID{}, false
	}
	id := c.effectOccurrenceIDs[position]
	return id, id.Available()
}

// callbackEffectOccurrenceID returns the exact callback effect occurrence
// identity, including callback correspondence and canonical local position.
func (c *Contract) callbackEffectOccurrenceID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	row, ok := c.callback(callback)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return identity.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectOccurrenceIDs) {
		return identity.ContentID{}, false
	}
	id := c.effectOccurrenceIDs[position]
	return id, id.Available()
}

// effectRowFamilyID identifies an operation's complete effect row, including
// its tail/variable schema and ordered occurrence identities.  Empty and
// opaque rows therefore receive a real family identity too.
func (c *Contract) effectRowFamilyID(op vocabulary.Operation) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operationEffectFamilies) {
		return identity.ContentID{}, false
	}
	id := c.operationEffectFamilies[op-1]
	return id, id.Available()
}

// callbackEffectRowFamilyID identifies a callback's complete expected effect
// row, including its tail/variable schema and ordered occurrence identities.
func (c *Contract) callbackEffectRowFamilyID(callback vocabulary.CallbackID) (identity.ContentID, bool) {
	if c == nil || !c.sealed || callback == 0 || int(callback) > len(c.callbackEffectFamilies) {
		return identity.ContentID{}, false
	}
	id := c.callbackEffectFamilies[callback-1]
	return id, id.Available()
}
