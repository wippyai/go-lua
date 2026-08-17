package target

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// descriptor while retaining distinct occurrence evidence.
func (c *Contract) sealEffectIdentities() error {
	if c == nil || len(c.operationAnchors) != len(c.operations) {
		return errors.New("target: missing effect operation anchors")
	}
	c.effectOperationIDs = make([]identity.ContentID, len(c.operations))
	for index := range c.operations {
		op := Operation(index + 1)
		anchor := c.operationAnchors[index]
		if !anchor.Available() {
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
		op := Operation(index + 1)
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
		callback := CallbackID(index + 1)
		if err := c.sealEffectRow(row.owner, callback, row.effects); err != nil {
			return err
		}
		id, err := c.effectFamilyID(semanticCallbackEffectFamily, row.owner, callback, row.effectTail, row.effectVar, row.effects)
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
func (c *Contract) encodeEffectOperationABI(w *framing.Writer, op Operation) error {
	row, ok := c.operation(op)
	if !ok {
		return errors.New("target: malformed effect operation")
	}
	if err := encodeValues(w, c, row.input); err != nil {
		return err
	}
	if err := w.Count(uint64(row.typeFormals.len())); err != nil {
		return err
	}
	for index := 0; index < row.typeFormals.len(); index++ {
		value := c.formals[row.typeFormals.start+uint32(index)]
		if err := w.Bool(value != 0); err != nil {
			return err
		}
		if value != 0 {
			if err := encodeType(w, c, value); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.valuesVars)); err != nil {
		return err
	}
	if row.valuesTypes.len() != int(row.valuesVars) || !validIdentityRange(row.valuesTypes, len(c.valuesVarTypes)) {
		return errors.New("target: malformed effect Values ABI")
	}
	for index := row.valuesTypes.start; index < row.valuesTypes.end; index++ {
		if err := encodeType(w, c, c.valuesVarTypes[index]); err != nil {
			return err
		}
	}
	return w.Count(uint64(row.rowFormals))
}

func (c *Contract) sealEffectRow(owner Operation, callback CallbackID, effects indexRange) error {
	if owner == 0 || uint64(owner) > uint64(len(c.operations)) || !validIdentityRange(effects, len(c.effects)) {
		return errors.New("target: malformed effect row range")
	}
	if callback != 0 {
		if uint64(callback) > uint64(len(c.callbacks)) || c.callbacks[callback-1].owner != owner ||
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
		occurrence, err := c.effectOccurrenceID(owner, callback, uint32(local), descriptor)
		if err != nil {
			return err
		}
		c.effectDescriptorIDs[position] = descriptor
		c.effectOccurrenceIDs[position] = occurrence
	}
	return nil
}

func (c *Contract) effectDescriptorID(owner Operation, row effectRow) (identity.ContentID, error) {
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

func (c *Contract) effectOccurrenceID(owner Operation, callback CallbackID, local uint32, descriptor identity.ContentID) (identity.ContentID, error) {
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

func (c *Contract) effectFamilyID(kind uint64, owner Operation, callback CallbackID, tail RowTail, variable RowVar, effects indexRange) (identity.ContentID, error) {
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

func (c *Contract) EffectOperationID(op Operation) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.effectOperationIDs) {
		return identity.ContentID{}, false
	}
	return c.effectOperationIDs[op-1], true
}

// EffectDescriptorID returns the semantic quotient for one ordinary
// operation effect occurrence.  Equal descriptors may intentionally occur at
// several local positions; use EffectOccurrenceID when occurrence evidence is
// required.
func (c *Contract) EffectDescriptorID(op Operation, index int) (identity.ContentID, bool) {
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

// PublicationEffectDescriptor returns the immutable Target-owned publication
// semantics for one exact ordinary effect occurrence. Generic effects remain
// absent unless their author explicitly supplied PublicationEffectSpec.
func (c *Contract) PublicationEffectDescriptor(op Operation, index int) (PublicationEffectDescriptor, bool) {
	row, ok := c.effect(op, index)
	if !ok || !c.sealed || !c.validPublicationEffectRow(row) {
		return PublicationEffectDescriptor{}, false
	}
	return row.publication, true
}

// PublicationEffectDescriptorID returns the existing canonical effect
// descriptor identity only when that exact occurrence carries an explicit
// publication descriptor. Reusing the effect descriptor ID prevents a second
// parallel semantic identity for the same sealed occurrence.
func (c *Contract) PublicationEffectDescriptorID(op Operation, index int) (identity.ContentID, bool) {
	if _, ok := c.PublicationEffectDescriptor(op, index); !ok {
		return identity.ContentID{}, false
	}
	return c.EffectDescriptorID(op, index)
}

// PublicationEffectOccurrenceID returns the existing canonical exact effect
// occurrence identity only when that occurrence carries explicit publication
// semantics.
func (c *Contract) PublicationEffectOccurrenceID(op Operation, index int) (identity.ContentID, bool) {
	if _, ok := c.PublicationEffectDescriptor(op, index); !ok {
		return identity.ContentID{}, false
	}
	return c.EffectOccurrenceID(op, index)
}

// CallbackEffectDescriptorID returns the semantic quotient for one callback
// effect occurrence.  It has no inverse because duplicate descriptors are a
// deliberate quotient of distinct retained occurrences.
func (c *Contract) CallbackEffectDescriptorID(callback CallbackID, index int) (identity.ContentID, bool) {
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

// CallbackPublicationEffectDescriptor returns the immutable Target-owned
// publication semantics for one exact callback effect occurrence.
func (c *Contract) CallbackPublicationEffectDescriptor(callback CallbackID, index int) (PublicationEffectDescriptor, bool) {
	row, ok := c.callback(callback)
	if !ok || !c.sealed || index < 0 || index >= row.effects.len() {
		return PublicationEffectDescriptor{}, false
	}
	effect := c.effects[row.effects.start+uint32(index)]
	if !c.validPublicationEffectRow(effect) {
		return PublicationEffectDescriptor{}, false
	}
	return effect.publication, true
}

// CallbackPublicationEffectDescriptorID returns the canonical generic effect
// descriptor identity for one callback occurrence with explicit publication
// semantics.
func (c *Contract) CallbackPublicationEffectDescriptorID(callback CallbackID, index int) (identity.ContentID, bool) {
	if _, ok := c.CallbackPublicationEffectDescriptor(callback, index); !ok {
		return identity.ContentID{}, false
	}
	return c.CallbackEffectDescriptorID(callback, index)
}

// CallbackPublicationEffectOccurrenceID returns the canonical exact callback
// effect occurrence identity for an explicitly declared publication effect.
func (c *Contract) CallbackPublicationEffectOccurrenceID(callback CallbackID, index int) (identity.ContentID, bool) {
	if _, ok := c.CallbackPublicationEffectDescriptor(callback, index); !ok {
		return identity.ContentID{}, false
	}
	return c.CallbackEffectOccurrenceID(callback, index)
}

// EffectOccurrenceID returns the exact ordinary effect occurrence identity,
// including its canonical local row position.
func (c *Contract) EffectOccurrenceID(op Operation, index int) (identity.ContentID, bool) {
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

// CallbackEffectOccurrenceID returns the exact callback effect occurrence
// identity, including callback correspondence and canonical local position.
func (c *Contract) CallbackEffectOccurrenceID(callback CallbackID, index int) (identity.ContentID, bool) {
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

// EffectRowFamilyID identifies an operation's complete effect row, including
// its tail/variable schema and ordered occurrence identities.  Empty and
// opaque rows therefore receive a real family identity too.
func (c *Contract) EffectRowFamilyID(op Operation) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operationEffectFamilies) {
		return identity.ContentID{}, false
	}
	id := c.operationEffectFamilies[op-1]
	return id, id.Available()
}

// CallbackEffectRowFamilyID identifies a callback's complete expected effect
// row, including its tail/variable schema and ordered occurrence identities.
func (c *Contract) CallbackEffectRowFamilyID(callback CallbackID) (identity.ContentID, bool) {
	if c == nil || !c.sealed || callback == 0 || int(callback) > len(c.callbackEffectFamilies) {
		return identity.ContentID{}, false
	}
	id := c.callbackEffectFamilies[callback-1]
	return id, id.Available()
}
