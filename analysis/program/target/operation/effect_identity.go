package operation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// EffectIdentityContext supplies the Target-neutral codecs needed to derive
// the existing semantic effect identities. Core performs all traversal and
// retains the resulting columns; Target supplies only its established type
// and Values byte codecs and semantic digest function.
type EffectIdentityContext struct {
	SemanticID                func(uint64, func(*framing.Writer) error) (identity.ContentID, error)
	EncodeValues              func(*framing.Writer, vocabulary.Values) error
	EncodeType                func(*framing.Writer, vocabulary.Type) error
	CallbackContentID         func(vocabulary.CallbackID) (identity.ContentID, bool)
	EffectOperationKind       uint64
	EffectDescriptorKind      uint64
	EffectOccurrenceKind      uint64
	OperationEffectFamilyKind uint64
	CallbackEffectFamilyKind  uint64
}

// SealEffectIdentities returns a Core copy with the effect identity columns
// sealed. The receiver remains unchanged, so the construction capability is
// still one-shot and no mutable identity state leaks from Core.
func (core Core) SealEffectIdentities(context EffectIdentityContext) (Core, error) {
	if context.SemanticID == nil || context.EncodeValues == nil || context.EncodeType == nil || context.CallbackContentID == nil {
		return Core{}, errors.New("target/operation: incomplete effect identity context")
	}
	if context.EffectOperationKind == 0 || context.EffectDescriptorKind == 0 || context.EffectOccurrenceKind == 0 || context.OperationEffectFamilyKind == 0 || context.CallbackEffectFamilyKind == 0 {
		return Core{}, errors.New("target/operation: incomplete effect identity kinds")
	}
	sealed := core
	sealed.effectOperationIDs = make([]identity.ContentID, core.OperationCount())
	for index := range sealed.effectOperationIDs {
		op := vocabulary.Operation(index + 1)
		anchor, ok := core.Anchor(op)
		if !ok {
			return Core{}, errors.New("target/operation: missing effect operation anchor")
		}
		id, err := context.SemanticID(context.EffectOperationKind, func(w *framing.Writer) error {
			if err := w.Bytes(anchor[:]); err != nil {
				return err
			}
			return core.encodeEffectOperationABI(w, op, context)
		})
		if err != nil {
			return Core{}, err
		}
		sealed.effectOperationIDs[index] = id
	}
	sealed.effectDescriptorIDs = make([]identity.ContentID, len(core.query.effects))
	sealed.effectOccurrenceIDs = make([]identity.ContentID, len(core.query.effects))
	sealed.operationEffectFamilies = make([]identity.ContentID, core.OperationCount())
	sealed.callbackEffectFamilies = make([]identity.ContentID, len(core.query.callbacks))

	for index := 0; index < core.OperationCount(); index++ {
		op := vocabulary.Operation(index + 1)
		row, ok := core.queryOperation(op)
		if !ok || row.outcomes.end == 0 {
			return Core{}, errors.New("target/operation: incomplete operation query for effect identity")
		}
		for local, handle := range row.effects {
			if err := sealed.sealEffectOccurrence(op, 0, local, handle, context); err != nil {
				return Core{}, err
			}
		}
		id, err := sealed.effectFamily(context.OperationEffectFamilyKind, op, 0, row.effectTail, row.effectVar, row.effects, context)
		if err != nil {
			return Core{}, err
		}
		sealed.operationEffectFamilies[index] = id
	}
	for index := range core.query.callbacks {
		callback := vocabulary.CallbackID(index + 1)
		row, ok := core.callbackQuery(callback)
		if !ok {
			return Core{}, errors.New("target/operation: incomplete callback effect query")
		}
		owner, ownerOK := core.CallbackOwner(callback)
		if !ownerOK {
			return Core{}, errors.New("target/operation: malformed effect callback owner")
		}
		for local := 0; local < row.effects.len(); local++ {
			handle := int(row.effects.start) + local
			if err := sealed.sealEffectOccurrence(owner, callback, local, handle, context); err != nil {
				return Core{}, err
			}
		}
		id, err := sealed.effectFamily(context.CallbackEffectFamilyKind, owner, callback, row.effectTail, row.effectVar, effectHandles(row.effects), context)
		if err != nil {
			return Core{}, err
		}
		sealed.callbackEffectFamilies[index] = id
	}
	return sealed, nil
}

func effectHandles(span queryRange) []int {
	handles := make([]int, span.len())
	for index := range handles {
		handles[index] = span.start + index
	}
	return handles
}

func (core Core) encodeEffectOperationABI(w *framing.Writer, op vocabulary.Operation, context EffectIdentityContext) error {
	input, ok := core.Input(op)
	if !ok {
		return errors.New("target/operation: malformed effect operation input")
	}
	if err := context.EncodeValues(w, input); err != nil {
		return err
	}
	formals := core.TypeFormalCount(op)
	if err := w.Count(uint64(formals)); err != nil {
		return err
	}
	for index := 0; index < formals; index++ {
		value, found := core.TypeFormalConstraint(op, vocabulary.TypeFormal(index))
		if err := w.Bool(found); err != nil {
			return err
		}
		if found {
			if err := context.EncodeType(w, value); err != nil {
				return err
			}
		}
	}
	vars := core.ValuesVarCount(op)
	if err := w.Count(uint64(vars)); err != nil {
		return err
	}
	for index := 0; index < vars; index++ {
		value, found := core.ValuesVarType(op, vocabulary.ValuesVar(index))
		if !found {
			return errors.New("target/operation: malformed effect Values ABI")
		}
		if err := context.EncodeType(w, value); err != nil {
			return err
		}
	}
	return w.Count(uint64(core.RowFormalCount(op)))
}

func (core Core) sealEffectOccurrence(owner vocabulary.Operation, callback vocabulary.CallbackID, local, handle int, context EffectIdentityContext) error {
	if handle < 0 || handle >= len(core.query.effects) || owner == 0 || int(owner) > len(core.effectOperationIDs) {
		return errors.New("target/operation: malformed effect row")
	}
	descriptor, err := core.effectDescriptorID(owner, core.query.effects[handle], context)
	if err != nil {
		return err
	}
	occurrence, err := core.effectOccurrenceID(owner, callback, uint32(local), descriptor, context)
	if err != nil {
		return err
	}
	// The receiver is a copy with writable slices; callers only observe the
	// returned Core after this pass completes.
	core.effectDescriptorIDs[handle] = descriptor
	core.effectOccurrenceIDs[handle] = occurrence
	return nil
}

func (core Core) effectDescriptorID(owner vocabulary.Operation, row queryEffectRow, context EffectIdentityContext) (identity.ContentID, error) {
	if row.target == 0 || int(row.target) > len(core.effectOperationIDs) || int(owner) > len(core.effectOperationIDs) {
		return identity.ContentID{}, errors.New("target/operation: malformed effect descriptor owner")
	}
	ownerID, targetID := core.effectOperationIDs[owner-1], core.effectOperationIDs[row.target-1]
	if !ownerID.Available() || !targetID.Available() {
		return identity.ContentID{}, errors.New("target/operation: missing effect descriptor operation identity")
	}
	return context.SemanticID(context.EffectDescriptorKind, func(w *framing.Writer) error {
		if err := w.Bytes(ownerID[:]); err != nil {
			return err
		}
		if err := w.Bytes(targetID[:]); err != nil {
			return err
		}
		return core.encodeEffectArguments(w, row, context)
	})
}

func (core Core) encodeEffectArguments(w *framing.Writer, row queryEffectRow, context EffectIdentityContext) error {
	if err := w.Count(uint64(len(row.values))); err != nil {
		return err
	}
	for _, value := range row.values {
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(len(row.types))); err != nil {
		return err
	}
	for _, value := range row.types {
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(len(row.valuesVar))); err != nil {
		return err
	}
	for _, value := range row.valuesVar {
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(len(row.rows))); err != nil {
		return err
	}
	for _, value := range row.rows {
		if err := w.Uint(uint64(value)); err != nil {
			return err
		}
	}
	if err := w.Bool(row.hasPublication); err != nil {
		return err
	}
	if row.hasPublication {
		return encodePublicationEffectDescriptor(w, row.publication)
	}
	return nil
}

func encodePublicationEffectDescriptor(w *framing.Writer, descriptor PublicationEffectDescriptor) error {
	if !descriptor.Valid() {
		return errors.New("target/operation: malformed publication effect descriptor")
	}
	for _, value := range []uint64{
		uint64(descriptor.Kind()), uint64(descriptor.Subject()), uint64(descriptor.DestinationRole()),
		uint64(descriptor.Context()), uint64(descriptor.Escape()), uint64(descriptor.Mutability()), uint64(descriptor.Lifetime()),
	} {
		if err := w.Uint(value); err != nil {
			return err
		}
	}
	return nil
}

func (core Core) effectOccurrenceID(owner vocabulary.Operation, callback vocabulary.CallbackID, local uint32, descriptor identity.ContentID, context EffectIdentityContext) (identity.ContentID, error) {
	if owner == 0 || int(owner) > len(core.effectOperationIDs) || !descriptor.Available() {
		return identity.ContentID{}, errors.New("target/operation: malformed effect occurrence")
	}
	ownerID := core.effectOperationIDs[owner-1]
	return context.SemanticID(context.EffectOccurrenceKind, func(w *framing.Writer) error {
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
			callbackID, ok := context.CallbackContentID(callback)
			if !ok || !callbackID.Available() {
				return errors.New("target/operation: missing effect callback identity")
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
}

func (core Core) effectFamily(kind uint64, owner vocabulary.Operation, callback vocabulary.CallbackID, tail vocabulary.RowTail, variable vocabulary.RowVar, handles []int, context EffectIdentityContext) (identity.ContentID, error) {
	if owner == 0 || int(owner) > len(core.effectOperationIDs) {
		return identity.ContentID{}, errors.New("target/operation: malformed effect family")
	}
	ownerID := core.effectOperationIDs[owner-1]
	return context.SemanticID(kind, func(w *framing.Writer) error {
		if callback != 0 {
			callbackID, ok := context.CallbackContentID(callback)
			if !ok || !callbackID.Available() {
				return errors.New("target/operation: missing callback effect family identity")
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
		if err := w.Count(uint64(len(handles))); err != nil {
			return err
		}
		for _, handle := range handles {
			if handle < 0 || handle >= len(core.effectOccurrenceIDs) || !core.effectOccurrenceIDs[handle].Available() {
				return errors.New("target/operation: missing effect family occurrence identity")
			}
			if err := w.Bytes(core.effectOccurrenceIDs[handle][:]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (core Core) effectPosition(op vocabulary.Operation, index int) (int, bool) {
	row, ok := core.queryOperation(op)
	if !ok || index < 0 || index >= len(row.effects) {
		return 0, false
	}
	position := row.effects[index]
	return position, position >= 0 && position < len(core.query.effects)
}

func (core Core) callbackEffectPosition(callback vocabulary.CallbackID, index int) (int, bool) {
	row, ok := core.callbackQuery(callback)
	if !ok || index < 0 || index >= row.effects.len() {
		return 0, false
	}
	position := row.effects.start + index
	return position, position >= 0 && position < len(core.query.effects)
}

func (core Core) EffectOperationID(op vocabulary.Operation) (identity.ContentID, bool) {
	if op == 0 || int(op) > len(core.effectOperationIDs) {
		return identity.ContentID{}, false
	}
	id := core.effectOperationIDs[op-1]
	return id, id.Available()
}

func (core Core) EffectDescriptorID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	position, ok := core.effectPosition(op, index)
	if !ok || position >= len(core.effectDescriptorIDs) {
		return identity.ContentID{}, false
	}
	id := core.effectDescriptorIDs[position]
	return id, id.Available()
}

func (core Core) EffectOccurrenceID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	position, ok := core.effectPosition(op, index)
	if !ok || position >= len(core.effectOccurrenceIDs) {
		return identity.ContentID{}, false
	}
	id := core.effectOccurrenceIDs[position]
	return id, id.Available()
}

func (core Core) PublicationEffectDescriptorID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	if _, ok := core.EffectPublication(op, index); !ok {
		return identity.ContentID{}, false
	}
	return core.EffectDescriptorID(op, index)
}

func (core Core) PublicationEffectOccurrenceID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	if _, ok := core.EffectPublication(op, index); !ok {
		return identity.ContentID{}, false
	}
	return core.EffectOccurrenceID(op, index)
}

func (core Core) CallbackEffectDescriptorID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	position, ok := core.callbackEffectPosition(callback, index)
	if !ok || position >= len(core.effectDescriptorIDs) {
		return identity.ContentID{}, false
	}
	id := core.effectDescriptorIDs[position]
	return id, id.Available()
}

func (core Core) CallbackEffectOccurrenceID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	position, ok := core.callbackEffectPosition(callback, index)
	if !ok || position >= len(core.effectOccurrenceIDs) {
		return identity.ContentID{}, false
	}
	id := core.effectOccurrenceIDs[position]
	return id, id.Available()
}

func (core Core) CallbackPublicationEffectDescriptorID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	if _, ok := core.CallbackEffectPublication(callback, index); !ok {
		return identity.ContentID{}, false
	}
	return core.CallbackEffectDescriptorID(callback, index)
}

func (core Core) CallbackPublicationEffectOccurrenceID(callback vocabulary.CallbackID, index int) (identity.ContentID, bool) {
	if _, ok := core.CallbackEffectPublication(callback, index); !ok {
		return identity.ContentID{}, false
	}
	return core.CallbackEffectOccurrenceID(callback, index)
}

func (core Core) EffectRowFamilyID(op vocabulary.Operation) (identity.ContentID, bool) {
	if op == 0 || int(op) > len(core.operationEffectFamilies) {
		return identity.ContentID{}, false
	}
	id := core.operationEffectFamilies[op-1]
	return id, id.Available()
}

func (core Core) CallbackEffectRowFamilyID(callback vocabulary.CallbackID) (identity.ContentID, bool) {
	if callback == 0 || int(callback) > len(core.callbackEffectFamilies) {
		return identity.ContentID{}, false
	}
	id := core.callbackEffectFamilies[callback-1]
	return id, id.Available()
}
