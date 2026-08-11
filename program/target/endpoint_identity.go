package target

// This file owns the *open* Target relation identities.  Contract.ContentID
// is deliberately a closed whole-contract digest; the identities below are
// declaration identities, so an unrelated record cannot renumber them.  The
// seal has already proved the only recursive-looking relation (produced
// operations) is a finite acyclic forest.  Everything else points at a
// precomputed structural anchor, never at another descriptor hash.

import (
	"crypto/sha256"
	"errors"
	"sort"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

const endpointIdentityCodecVersion uint64 = 6

const (
	semanticOperationAnchor uint64 = iota + 80
	semanticProducedAnchor
	semanticOpaqueAnchor
	semanticCallbackSelector
	semanticOutcomeSelector
	semanticOperation
	semanticOutcome
	semanticTransfer
	semanticTransferOutcome
	semanticInputFormal
	semanticOutcomeResult
	semanticInitialValue
	semanticBootRelation
	semanticCallbackContent
	semanticResumeContent
	semanticEffectOperation
	semanticEffectDescriptor
	semanticEffectOccurrence
	semanticOperationEffectFamily
	semanticCallbackEffectFamily
)

func (c *Contract) semanticID(kind uint64, encode func(*canonical.Writer) error) (id keyspace.ContentID, err error) {
	if c == nil || encode == nil {
		return id, errors.New("target: missing semantic identity")
	}
	h := sha256.New()
	var w canonical.Writer
	if err = w.Reset(h, "program/target-semantic", endpointIdentityCodecVersion); err != nil {
		return id, err
	}
	if err = w.Record(kind); err != nil {
		return id, err
	}
	if err = encode(&w); err != nil {
		return id, err
	}
	if err = w.Finish(); err != nil {
		return id, err
	}
	if got := h.Sum(id[:0]); len(got) != len(id) {
		return keyspace.ContentID{}, errors.New("target: semantic digest failure")
	}
	return id, nil
}

func (c *Contract) anchor(op Operation) (keyspace.ContentID, bool) {
	if c == nil || op == 0 || int(op) > len(c.operationAnchors) {
		return keyspace.ContentID{}, false
	}
	return c.operationAnchors[op-1], true
}

func (c *Contract) callbackSelector(id CallbackID) (keyspace.ContentID, bool) {
	if c == nil || id == 0 || int(id) > len(c.callbackSelectors) {
		return keyspace.ContentID{}, false
	}
	return c.callbackSelectors[id-1], true
}

// CallbackContentID identifies one exact callback correspondence under its
// owning operation. The operation and callback handles are receiver-local;
// the explicit range fence prevents a numerically coincident callback from a
// different operation from being accepted.
func (c *Contract) CallbackContentID(op Operation, callback CallbackID) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operations) || callback == 0 || int(callback) > len(c.callbacks) || int(callback) > len(c.callbackContentIDs) {
		return keyspace.ContentID{}, false
	}
	row := c.callbacks[callback-1]
	owner, ok := c.operation(op)
	if !ok || row.owner != op || uint64(callback-1) < uint64(owner.callbacks.start) || uint64(callback-1) >= uint64(owner.callbacks.end) {
		return keyspace.ContentID{}, false
	}
	id := c.callbackContentIDs[callback-1]
	if !id.Available() {
		return keyspace.ContentID{}, false
	}
	return id, true
}

// FindCallbackContentID is the allocation-free O(log n) inverse over the
// immutable sorted callback-content column.
func (c *Contract) FindCallbackContentID(id keyspace.ContentID) (Operation, CallbackID, bool) {
	if c == nil || !c.sealed || !id.Available() {
		return 0, 0, false
	}
	i := sort.Search(len(c.callbackContentIndex), func(i int) bool {
		return compareSemanticID(c.callbackContentIndex[i].id, id) >= 0
	})
	if i >= len(c.callbackContentIndex) || compareSemanticID(c.callbackContentIndex[i].id, id) != 0 {
		return 0, 0, false
	}
	row := c.callbackContentIndex[i]
	got, ok := c.CallbackContentID(row.op, row.callback)
	if !ok || got != id {
		return 0, 0, false
	}
	return row.op, row.callback, true
}

// ResumeContentID identifies one exact operation-owned resumption
// correspondence. Its canonical record is derived during Seal from the
// operation anchor, source/carrier, argument Values, and all five sealed
// outcome selectors.
func (c *Contract) ResumeContentID(op Operation, resume ResumeID) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operations) || resume == 0 || int(resume) > len(c.resumes) || int(resume) > len(c.resumeContentIDs) {
		return keyspace.ContentID{}, false
	}
	row := c.resumes[resume-1]
	owner, ok := c.operation(op)
	if !ok || row.owner != op || uint64(resume-1) < uint64(owner.resumes.start) || uint64(resume-1) >= uint64(owner.resumes.end) {
		return keyspace.ContentID{}, false
	}
	id := c.resumeContentIDs[resume-1]
	if !id.Available() {
		return keyspace.ContentID{}, false
	}
	return id, true
}

// FindResumeContentID is the allocation-free O(log n) inverse over the
// immutable sorted resume-content column.
func (c *Contract) FindResumeContentID(id keyspace.ContentID) (Operation, ResumeID, bool) {
	if c == nil || !c.sealed || !id.Available() {
		return 0, 0, false
	}
	i := sort.Search(len(c.resumeContentIndex), func(i int) bool {
		return compareSemanticID(c.resumeContentIndex[i].id, id) >= 0
	})
	if i >= len(c.resumeContentIndex) || compareSemanticID(c.resumeContentIndex[i].id, id) != 0 {
		return 0, 0, false
	}
	row := c.resumeContentIndex[i]
	got, ok := c.ResumeContentID(row.op, row.resume)
	if !ok || got != id {
		return 0, 0, false
	}
	return row.op, row.resume, true
}

func (c *Contract) outcomeIndex(op Operation, index int) (int, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.outcomes.len() {
		return 0, false
	}
	return int(row.outcomes.start) + index, true
}

// sealSemanticIdentities is one finite finalization phase.  It retains only
// dense immutable result columns; all sorting/graph validation belongs to the
// normal Target Seal phase above.
func (c *Contract) sealSemanticIdentities() error {
	c.operationAnchors = make([]keyspace.ContentID, len(c.operations))
	c.callbackSelectors = make([]keyspace.ContentID, len(c.callbacks))
	c.outcomeSelectors = make([]keyspace.ContentID, len(c.outcomes))
	outcomeOwners := make([]Operation, len(c.outcomes))
	outcomeOrdinals := make([]uint32, len(c.outcomes))
	c.operationContentIDs = make([]keyspace.ContentID, len(c.operations))
	c.outcomeContentIDs = make([]keyspace.ContentID, len(c.outcomes))
	c.transferContentIDs = make([]keyspace.ContentID, len(c.transfers))
	c.transferOutcomeIDs = make([]keyspace.ContentID, len(c.transferOutcomes))
	c.callbackContentIDs = make([]keyspace.ContentID, len(c.callbacks))
	c.callbackContentIndex = make([]callbackContentIDRow, 0, len(c.callbacks))
	c.resumeContentIDs = make([]keyspace.ContentID, len(c.resumes))
	c.resumeContentIndex = make([]resumeContentIDRow, 0, len(c.resumes))

	// Dense outcome owner/ordinal columns are formed once in table order.  They
	// make the remaining identity pass strictly linear in the sealed tables.
	for operationIndex, operation := range c.operations {
		owner := Operation(operationIndex + 1)
		for outcome := operation.outcomes.start; outcome < operation.outcomes.end; outcome++ {
			outcomeOwners[outcome] = owner
			outcomeOrdinals[outcome] = outcome - operation.outcomes.start
		}
	}

	// Outcome selectors are deliberately owner-free: they are the fixed local
	// discriminator needed while deriving a produced child's parent anchor.
	for i := range c.outcomes {
		row := c.outcomes[i]
		local := outcomeOrdinals[i]
		id, err := c.semanticID(semanticOutcomeSelector, func(w *canonical.Writer) error {
			if err := w.Uint(uint64(local)); err != nil {
				return err
			}
			if err := w.Uint(uint64(row.kind)); err != nil {
				return err
			}
			if err := encodeValues(w, c, row.values); err != nil {
				return err
			}
			if err := w.Count(uint64(row.fresh.len())); err != nil {
				return err
			}
			for j := row.fresh.start; j < row.fresh.end; j++ {
				x := c.fresh[j]
				if err := w.Uint(uint64(x.result)); err != nil {
					return err
				}
				if err := w.Uint(uint64(x.ordinal)); err != nil {
					return err
				}
				if err := w.Uint(uint64(x.kind)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		c.outcomeSelectors[i] = id
	}

	// The normal produced-anchor validation has already given this Contract a
	// topological canonical table order: every parent precedes its produced
	// child.  Build one temporary reverse edge table, then consume it once in
	// that order.  There is no retry cap, graph search, or repeated full scan.
	type producedParent struct {
		parent  int
		outcome int
		result  uint32
		found   bool
	}
	parents := make([]producedParent, len(c.operations))
	for parentIndex, parent := range c.operations {
		for outcome := parent.outcomes.start; outcome < parent.outcomes.end; outcome++ {
			for produced := c.outcomes[outcome].produced.start; produced < c.outcomes[outcome].produced.end; produced++ {
				child := int(c.produced[produced].target) - 1
				if child < 0 || child >= len(c.operations) {
					return errors.New("target: malformed produced anchor")
				}
				if c.operations[child].bindings.len() != 0 {
					continue
				}
				if parents[child].found {
					return errors.New("target: duplicate semantic produced parent")
				}
				parents[child] = producedParent{parent: parentIndex, outcome: int(outcome), result: c.produced[produced].result, found: true}
			}
		}
	}
	for i, row := range c.operations {
		op := Operation(i + 1)
		if op == c.opaque {
			id, err := c.semanticID(semanticOpaqueAnchor, func(w *canonical.Writer) error { return w.Uint(1) })
			if err != nil {
				return err
			}
			c.operationAnchors[i] = id
			continue
		}
		if row.bindings.len() != 0 {
			id, err := c.semanticID(semanticOperationAnchor, func(w *canonical.Writer) error { return c.encodeBindings(w, op) })
			if err != nil {
				return err
			}
			c.operationAnchors[i] = id
			continue
		}
		parent := parents[i]
		if !parent.found || parent.parent >= i || parent.outcome < 0 || parent.outcome >= len(c.outcomeSelectors) {
			return errors.New("target: malformed produced anchor order")
		}
		parentAnchor := c.operationAnchors[parent.parent]
		if parentAnchor == (keyspace.ContentID{}) {
			return errors.New("target: unresolved semantic produced parent")
		}
		selector := c.outcomeSelectors[parent.outcome]
		id, err := c.semanticID(semanticProducedAnchor, func(w *canonical.Writer) error {
			if err := w.Bytes(parentAnchor[:]); err != nil {
				return err
			}
			if err := w.Bytes(selector[:]); err != nil {
				return err
			}
			return w.Uint(uint64(parent.result))
		})
		if err != nil {
			return err
		}
		c.operationAnchors[i] = id
	}
	for i := range c.operationAnchors {
		if c.operationAnchors[i] == (keyspace.ContentID{}) {
			return errors.New("target: unresolved semantic operation anchor")
		}
	}

	for i, row := range c.callbacks {
		owner, ok := c.anchor(row.owner)
		if !ok {
			return errors.New("target: malformed callback owner")
		}
		id, err := c.semanticID(semanticCallbackSelector, func(w *canonical.Writer) error {
			if err := w.Bytes(owner[:]); err != nil {
				return err
			}
			ownerRow, ok := c.operation(row.owner)
			if !ok {
				return errors.New("target: malformed callback owner")
			}
			if err := w.Uint(uint64(uint32(i) - ownerRow.callbacks.start)); err != nil {
				return err
			}
			if err := encodeInput(w, row.function); err != nil {
				return err
			}
			if err := encodeValues(w, c, row.arguments); err != nil {
				return err
			}
			for _, values := range row.outcomes {
				if err := encodeValues(w, c, values); err != nil {
					return err
				}
			}
			if err := w.Uint(uint64(row.admission)); err != nil {
				return err
			}
			return w.Uint(uint64(row.lifecycle))
		})
		if err != nil {
			return err
		}
		c.callbackSelectors[i] = id
	}
	for i, row := range c.callbacks {
		owner, ok := c.anchor(row.owner)
		if !ok {
			return errors.New("target: malformed callback content owner")
		}
		selector, ok := c.callbackSelector(CallbackID(i + 1))
		if !ok {
			return errors.New("target: missing callback selector")
		}
		ownerRow, ok := c.operation(row.owner)
		if !ok || uint64(i) < uint64(ownerRow.callbacks.start) || uint64(i) >= uint64(ownerRow.callbacks.end) {
			return errors.New("target: callback content owner fence")
		}
		id, err := c.semanticID(semanticCallbackContent, func(w *canonical.Writer) error {
			if err := w.Bytes(owner[:]); err != nil {
				return err
			}
			return w.Bytes(selector[:])
		})
		if err != nil {
			return err
		}
		c.callbackContentIDs[i] = id
		c.callbackContentIndex = append(c.callbackContentIndex, callbackContentIDRow{id: id, op: row.owner, callback: CallbackID(i + 1)})
	}
	if err := c.sealEffectIdentities(); err != nil {
		return err
	}

	for i, row := range c.resumes {
		owner, ok := c.anchor(row.owner)
		if !ok {
			return errors.New("target: malformed resume content owner")
		}
		ownerRow, ok := c.operation(row.owner)
		if !ok || uint64(i) < uint64(ownerRow.resumes.start) || uint64(i) >= uint64(ownerRow.resumes.end) {
			return errors.New("target: resume content owner fence")
		}
		id, err := c.semanticID(semanticResumeContent, func(w *canonical.Writer) error {
			if err := w.Bytes(owner[:]); err != nil {
				return err
			}
			if err := w.Uint(uint64(row.source)); err != nil {
				return err
			}
			if err := w.Uint(uint64(row.carrier)); err != nil {
				return err
			}
			if err := encodeValues(w, c, row.arguments); err != nil {
				return err
			}
			for _, outcome := range row.outcomes {
				index, ok := c.outcomeIndex(row.owner, int(outcome))
				if !ok || index < 0 || index >= len(c.outcomeSelectors) {
					return errors.New("target: malformed resume content outcome")
				}
				selector := c.outcomeSelectors[index]
				if err := w.Bytes(selector[:]); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		c.resumeContentIDs[i] = id
		c.resumeContentIndex = append(c.resumeContentIndex, resumeContentIDRow{id: id, op: row.owner, resume: ResumeID(i + 1)})
	}

	for i := range c.operations {
		op := Operation(i + 1)
		id, err := c.semanticID(semanticOperation, func(w *canonical.Writer) error {
			a := c.operationAnchors[i]
			if err := w.Bytes(a[:]); err != nil {
				return err
			}
			return c.encodePortableOperation(w, op)
		})
		if err != nil {
			return err
		}
		c.operationContentIDs[i] = id
	}
	for oi := range c.outcomes {
		owner := outcomeOwners[oi]
		if owner == 0 {
			return errors.New("target: malformed outcome owner")
		}
		ownerID := c.operationContentIDs[owner-1]
		selector := c.outcomeSelectors[oi]
		id, err := c.semanticID(semanticOutcome, func(w *canonical.Writer) error {
			if err := w.Bytes(ownerID[:]); err != nil {
				return err
			}
			if err := w.Bytes(selector[:]); err != nil {
				return err
			}
			return c.encodePortableOutcome(w, Operation(owner), oi)
		})
		if err != nil {
			return err
		}
		c.outcomeContentIDs[oi] = id
	}
	for i, row := range c.transfers {
		ownerID := c.operationContentIDs[row.owner-1]
		id, err := c.semanticID(semanticTransfer, func(w *canonical.Writer) error {
			if err := w.Bytes(ownerID[:]); err != nil {
				return err
			}
			return c.encodeTransferRow(w, row)
		})
		if err != nil {
			return err
		}
		c.transferContentIDs[i] = id
		for j := row.outcomes.start; j < row.outcomes.end; j++ {
			outcome := int(j - row.outcomes.start)
			oi, ok := c.outcomeIndex(row.owner, outcome)
			if !ok {
				return errors.New("target: malformed transfer outcome")
			}
			outcomeID := c.outcomeContentIDs[oi]
			transferID := id
			possibility := c.transferOutcomes[j]
			outID, err := c.semanticID(semanticTransferOutcome, func(w *canonical.Writer) error {
				if err := w.Bytes(transferID[:]); err != nil {
					return err
				}
				if err := w.Bytes(outcomeID[:]); err != nil {
					return err
				}
				return w.Uint(uint64(possibility))
			})
			if err != nil {
				return err
			}
			c.transferOutcomeIDs[j] = outID
		}
	}
	return c.sealHostIdentityRelations(outcomeOwners, outcomeOrdinals)
}

// sealEffectIdentities derives the complete Target-owned identity surface for
// authored effect rows.  The only retained state is one dense column per
// existing Target row; all relation payload remains in the ordinary effect,
// operation, and callback pools above.  In particular, there is deliberately
// no descriptor index: duplicate occurrences are valid and share a semantic
// descriptor while retaining distinct occurrence evidence.
func (c *Contract) sealEffectIdentities() error {
	if c == nil || len(c.operationAnchors) != len(c.operations) {
		return errors.New("target: missing effect operation anchors")
	}
	c.effectOperationIDs = make([]keyspace.ContentID, len(c.operations))
	for index := range c.operations {
		op := Operation(index + 1)
		anchor := c.operationAnchors[index]
		if !anchor.Available() {
			return errors.New("target: missing effect operation anchor")
		}
		id, err := c.semanticID(semanticEffectOperation, func(w *canonical.Writer) error {
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

	c.effectDescriptorIDs = make([]keyspace.ContentID, len(c.effects))
	c.effectOccurrenceIDs = make([]keyspace.ContentID, len(c.effects))
	c.operationEffectFamilies = make([]keyspace.ContentID, len(c.operations))
	c.callbackEffectFamilies = make([]keyspace.ContentID, len(c.callbacks))

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
func (c *Contract) encodeEffectOperationABI(w *canonical.Writer, op Operation) error {
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

func (c *Contract) effectDescriptorID(owner Operation, row effectRow) (keyspace.ContentID, error) {
	if owner == 0 || uint64(owner) > uint64(len(c.effectOperationIDs)) || row.target == 0 || uint64(row.target) > uint64(len(c.effectOperationIDs)) {
		return keyspace.ContentID{}, errors.New("target: malformed effect descriptor owner")
	}
	ownerID := c.effectOperationIDs[owner-1]
	targetID := c.effectOperationIDs[row.target-1]
	if !ownerID.Available() || !targetID.Available() {
		return keyspace.ContentID{}, errors.New("target: missing effect descriptor operation identity")
	}
	id, err := c.semanticID(semanticEffectDescriptor, func(w *canonical.Writer) error {
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

func (c *Contract) encodeEffectArguments(w *canonical.Writer, row effectRow) error {
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
	return nil
}

func (c *Contract) effectOccurrenceID(owner Operation, callback CallbackID, local uint32, descriptor keyspace.ContentID) (keyspace.ContentID, error) {
	if owner == 0 || uint64(owner) > uint64(len(c.effectOperationIDs)) || !descriptor.Available() {
		return keyspace.ContentID{}, errors.New("target: malformed effect occurrence")
	}
	ownerID := c.effectOperationIDs[owner-1]
	if !ownerID.Available() {
		return keyspace.ContentID{}, errors.New("target: missing effect occurrence owner identity")
	}
	id, err := c.semanticID(semanticEffectOccurrence, func(w *canonical.Writer) error {
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

func (c *Contract) effectFamilyID(kind uint64, owner Operation, callback CallbackID, tail RowTail, variable RowVar, effects indexRange) (keyspace.ContentID, error) {
	if owner == 0 || uint64(owner) > uint64(len(c.effectOperationIDs)) || !validIdentityRange(effects, len(c.effects)) {
		return keyspace.ContentID{}, errors.New("target: malformed effect family")
	}
	ownerID := c.effectOperationIDs[owner-1]
	if !ownerID.Available() {
		return keyspace.ContentID{}, errors.New("target: missing effect family owner identity")
	}
	id, err := c.semanticID(kind, func(w *canonical.Writer) error {
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

func validIdentityRange(value indexRange, length int) bool {
	return uint64(value.start) <= uint64(value.end) && uint64(value.end) <= uint64(length)
}

// sealHostIdentityRelations is deliberately after operation/outcome identity
// finalization. Its indexes are sorted immutable value tables, while the
// direct paths remain dense operation/outcome ranges.
func (c *Contract) sealHostIdentityRelations(outcomeOwners []Operation, outcomeOrdinals []uint32) error {
	c.inputFormalRanges = make([]indexRange, len(c.operations))
	for operationIndex, row := range c.operations {
		start, err := checkedStoredRange("semantic input formal table", len(c.inputFormalIDs), c.ValuesCount(row.input))
		if err != nil {
			return err
		}
		op := Operation(operationIndex + 1)
		operationID := c.operationContentIDs[operationIndex]
		for formal := 0; formal < c.ValuesCount(row.input); formal++ {
			selector := ValueFormal(formal)
			id, err := c.semanticID(semanticInputFormal, func(w *canonical.Writer) error {
				if err := w.Bytes(operationID[:]); err != nil {
					return err
				}
				return w.Uint(uint64(selector))
			})
			if err != nil {
				return err
			}
			c.inputFormalIDs = append(c.inputFormalIDs, id)
			c.inputFormalIndex = append(c.inputFormalIndex, inputFormalIDRow{id: id, op: op, formal: selector})
		}
		c.inputFormalRanges[operationIndex] = start
	}

	c.outcomeResultRanges = make([]indexRange, len(c.outcomes))
	for outcomeIndex, row := range c.outcomes {
		count := c.ValuesCount(row.values)
		start, err := checkedStoredRange("semantic outcome result table", len(c.outcomeResultIDs), count)
		if err != nil {
			return err
		}
		owner := outcomeOwners[outcomeIndex]
		ordinal := outcomeOrdinals[outcomeIndex]
		if owner == 0 {
			return errors.New("target: malformed semantic outcome owner")
		}
		outcomeID := c.outcomeContentIDs[outcomeIndex]
		for result := 0; result < count; result++ {
			selector := uint32(result)
			id, err := c.semanticID(semanticOutcomeResult, func(w *canonical.Writer) error {
				if err := w.Bytes(outcomeID[:]); err != nil {
					return err
				}
				return w.Uint(uint64(selector))
			})
			if err != nil {
				return err
			}
			c.outcomeResultIDs = append(c.outcomeResultIDs, id)
			c.outcomeResultIndex = append(c.outcomeResultIndex, outcomeResultIDRow{id: id, op: owner, outcome: ordinal, result: selector})
		}
		c.outcomeResultRanges[outcomeIndex] = start
	}
	if err := sortInputFormalIDs(c.inputFormalIndex); err != nil {
		return err
	}
	if err := sortOutcomeResultIDs(c.outcomeResultIndex); err != nil {
		return err
	}
	c.initialValueContentIDs = make([]keyspace.ContentID, len(c.initialValues))
	for index := range c.initialValues {
		value := InitialValue(index + 1)
		id, err := c.semanticID(semanticInitialValue, func(w *canonical.Writer) error { return c.encodeInitialValueContent(w, value) })
		if err != nil {
			return err
		}
		c.initialValueContentIDs[index] = id
	}
	boot, err := c.semanticID(semanticBootRelation, func(w *canonical.Writer) error { return c.encodeBootRelation(w) })
	if err != nil {
		return err
	}
	c.bootRelationID = boot
	if err := sortCallbackContentIDs(c.callbackContentIndex); err != nil {
		return err
	}
	if err := sortResumeContentIDs(c.resumeContentIndex); err != nil {
		return err
	}
	return nil
}

func compareSemanticID(left, right keyspace.ContentID) int {
	for i := range left {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	return 0
}
func sortInputFormalIDs(rows []inputFormalIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
			return errors.New("target: duplicate semantic input formal identity")
		}
	}
	return nil
}
func sortOutcomeResultIDs(rows []outcomeResultIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
			return errors.New("target: duplicate semantic outcome result identity")
		}
	}
	return nil
}

func sortCallbackContentIDs(rows []callbackContentIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
			return errors.New("target: duplicate semantic callback content identity")
		}
	}
	return nil
}

func sortResumeContentIDs(rows []resumeContentIDRow) error {
	sort.Slice(rows, func(i, j int) bool { return compareSemanticID(rows[i].id, rows[j].id) < 0 })
	for i := 1; i < len(rows); i++ {
		if compareSemanticID(rows[i-1].id, rows[i].id) == 0 {
			return errors.New("target: duplicate semantic resume content identity")
		}
	}
	return nil
}

// encodeInitialValueContent is a value relation, not an environment digest.
// Operation values use their binding/path anchor, so unrelated operation body
// edits do not churn boot cells that merely name the operation.
func (c *Contract) encodeInitialValueContent(w *canonical.Writer, value InitialValue) error {
	row, ok := c.initialValue(value)
	if !ok {
		return errors.New("target: malformed initial value")
	}
	if err := w.Uint(uint64(row.kind)); err != nil {
		return err
	}
	switch row.kind {
	case InitialValueNil, InitialValueAbsent:
		return nil
	case InitialValueBoolean:
		return w.Bool(row.boolean)
	case InitialValueInteger:
		return w.Uint(uint64(row.integer))
	case InitialValueFloat:
		return w.Uint(row.floatBits)
	case InitialValueString:
		return w.String(row.string)
	case InitialValueRoot:
		if row.root == 0 || int(row.root) > len(c.initialRoots) {
			return errors.New("target: malformed initial value root")
		}
		return w.String(c.initialRoots[row.root-1].identity)
	case InitialValueOperation:
		anchor, ok := c.anchor(row.operation)
		if !ok {
			return errors.New("target: malformed initial value operation")
		}
		return w.Bytes(anchor[:])
	case InitialValueDeniedOperation:
		binding, ok := c.initialValueBinding(value)
		if !ok {
			return errors.New("target: malformed denied initial value")
		}
		if err := w.Uint(uint64(binding.namespace)); err != nil {
			return err
		}
		if err := w.Count(uint64(binding.ownerKeys.len())); err != nil {
			return err
		}
		for i := binding.ownerKeys.start; i < binding.ownerKeys.end; i++ {
			if err := encodeExactKey(w, c, c.bindingKeys[i]); err != nil {
				return err
			}
		}
		if err := w.Count(uint64(binding.memberKeys.len())); err != nil {
			return err
		}
		for i := binding.memberKeys.start; i < binding.memberKeys.end; i++ {
			if err := encodeExactKey(w, c, c.bindingKeys[i]); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("target: invalid initial value kind")
	}
}

// encodeBootRelation commits only the root topology Host must establish:
// root identities/shapes and metatable attachments. Global rows are composed
// later by Host from selected values and InitialValueContentID.
func (c *Contract) encodeBootRelation(w *canonical.Writer) error {
	if err := w.Count(uint64(len(c.initialRoots))); err != nil {
		return err
	}
	for index, root := range c.initialRoots {
		if err := w.String(root.identity); err != nil {
			return err
		}
		if root.shape == 0 || int(root.shape) > len(c.bootShapes) {
			return errors.New("target: malformed boot root shape")
		}
		shape := c.bootShapes[root.shape-1]
		if shape.root != InitialRoot(index+1) {
			return errors.New("target: malformed boot root relation")
		}
		if err := w.Uint(uint64(shape.aggregate)); err != nil {
			return err
		}
		if err := w.Bool(shape.immutable); err != nil {
			return err
		}
		if shape.value == 0 || int(shape.value) > len(c.initialValueContentIDs) {
			return errors.New("target: malformed boot root value")
		}
		value := c.initialValueContentIDs[shape.value-1]
		if err := w.Bytes(value[:]); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(len(c.initialMetatables))); err != nil {
		return err
	}
	for _, attachment := range c.initialMetatables {
		if attachment.metatable == 0 || int(attachment.metatable) > len(c.initialRoots) {
			return errors.New("target: malformed initial metatable")
		}
		if err := w.Uint(uint64(attachment.base)); err != nil {
			return err
		}
		if err := w.String(c.initialRoots[attachment.metatable-1].identity); err != nil {
			return err
		}
	}
	return nil
}

// InputFormalID identifies one exact fixed input ABI slot.
func (c *Contract) InputFormalID(op Operation, formal ValueFormal) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.inputFormalRanges) {
		return keyspace.ContentID{}, false
	}
	r := c.inputFormalRanges[op-1]
	if uint64(formal) >= uint64(r.len()) {
		return keyspace.ContentID{}, false
	}
	return c.inputFormalIDs[r.start+uint32(formal)], true
}

// FindInputFormalID is the allocation-free O(log n) inverse over this Contract.
func (c *Contract) FindInputFormalID(id keyspace.ContentID) (Operation, ValueFormal, bool) {
	if c == nil || !c.sealed || !id.Available() {
		return 0, 0, false
	}
	i := sort.Search(len(c.inputFormalIndex), func(i int) bool { return compareSemanticID(c.inputFormalIndex[i].id, id) >= 0 })
	if i >= len(c.inputFormalIndex) || compareSemanticID(c.inputFormalIndex[i].id, id) != 0 {
		return 0, 0, false
	}
	x := c.inputFormalIndex[i]
	if _, ok := c.InputFormalID(x.op, x.formal); !ok {
		return 0, 0, false
	}
	return x.op, x.formal, true
}

// OutcomeResultID identifies one fixed result slot of an exact Outcome case.
func (c *Contract) OutcomeResultID(op Operation, outcome, result int) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed {
		return keyspace.ContentID{}, false
	}
	i, ok := c.outcomeIndex(op, outcome)
	if !ok || i >= len(c.outcomeResultRanges) {
		return keyspace.ContentID{}, false
	}
	r := c.outcomeResultRanges[i]
	if result < 0 || result >= r.len() {
		return keyspace.ContentID{}, false
	}
	return c.outcomeResultIDs[r.start+uint32(result)], true
}

// FindOutcomeResultID is the allocation-free O(log n) inverse over this Contract.
func (c *Contract) FindOutcomeResultID(id keyspace.ContentID) (Operation, int, int, bool) {
	if c == nil || !c.sealed || !id.Available() {
		return 0, 0, 0, false
	}
	i := sort.Search(len(c.outcomeResultIndex), func(i int) bool { return compareSemanticID(c.outcomeResultIndex[i].id, id) >= 0 })
	if i >= len(c.outcomeResultIndex) || compareSemanticID(c.outcomeResultIndex[i].id, id) != 0 {
		return 0, 0, 0, false
	}
	x := c.outcomeResultIndex[i]
	if _, ok := c.OutcomeResultID(x.op, int(x.outcome), int(x.result)); !ok {
		return 0, 0, 0, false
	}
	return x.op, int(x.outcome), int(x.result), true
}

// InitialValueContentID is a portable identity for one exact sealed boot
// value; it deliberately says nothing about which global row selected it.
func (c *Contract) InitialValueContentID(value InitialValue) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || value == 0 || int(value) > len(c.initialValueContentIDs) {
		return keyspace.ContentID{}, false
	}
	return c.initialValueContentIDs[value-1], true
}

// BootRelationID commits roots, identities, shapes, and metatable attachments.
func (c *Contract) BootRelationID() (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || !c.bootRelationID.Available() {
		return keyspace.ContentID{}, false
	}
	return c.bootRelationID, true
}

func encodeInput(w *canonical.Writer, value InputSource) error {
	if err := w.Uint(uint64(value.Kind)); err != nil {
		return err
	}
	return w.Uint(uint64(value.Ordinal))
}

func (c *Contract) encodeBindings(w *canonical.Writer, op Operation) error {
	count := c.BindingCount(op)
	if err := w.Count(uint64(count)); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		ns, ok := c.BindingNamespaceAt(op, i)
		if !ok {
			return errors.New("target: malformed binding")
		}
		if err := w.Uint(uint64(ns)); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, i, true); err != nil {
			return err
		}
		if err := encodeBindingSegments(w, c, op, i, false); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableOperation(w *canonical.Writer, op Operation) error {
	row, ok := c.operation(op)
	if !ok {
		return errors.New("target: malformed operation")
	}
	if err := c.encodeBindings(w, op); err != nil {
		return err
	}
	if err := w.Count(uint64(row.typeFormals.len())); err != nil {
		return err
	}
	for i := 0; i < row.typeFormals.len(); i++ {
		value := c.formals[row.typeFormals.start+uint32(i)]
		if err := w.Bool(value != 0); err != nil {
			return err
		}
		if value != 0 {
			if err := encodeType(w, c, value); err != nil {
				return err
			}
		}
	}
	if err := w.Uint(uint64(row.valuesVars)); err != nil {
		return err
	}
	for i := 0; i < int(row.valuesVars); i++ {
		if err := encodeType(w, c, c.valuesVarTypes[row.valuesTypes.start+uint32(i)]); err != nil {
			return err
		}
	}
	if err := w.Uint(uint64(row.rowFormals)); err != nil {
		return err
	}
	if err := encodeValues(w, c, row.input); err != nil {
		return err
	}
	if err := w.Count(uint64(row.callbacks.len())); err != nil {
		return err
	}
	for i := row.callbacks.start; i < row.callbacks.end; i++ {
		if err := c.encodePortableCallback(w, CallbackID(i+1)); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.subedges.len())); err != nil {
		return err
	}
	for i := row.subedges.start; i < row.subedges.end; i++ {
		if err := c.encodePortableSubedge(w, op, c.subedges[i]); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.outcomes.len())); err != nil {
		return err
	}
	for i := row.outcomes.start; i < row.outcomes.end; i++ {
		if err := c.encodePortableOutcome(w, op, int(i)); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(c.SuspensionCount(op))); err != nil {
		return err
	}
	for i := 0; i < c.SuspensionCount(op); i++ {
		y, r, s, m, ok := c.SuspensionAt(op, i)
		if !ok {
			return errors.New("target: malformed suspension")
		}
		for _, v := range []uint64{uint64(y), uint64(r), uint64(s), uint64(m)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.spawns.len())); err != nil {
		return err
	}
	for i := row.spawns.start; i < row.spawns.end; i++ {
		x := c.spawns[i]
		child, ok := c.callbackSelector(x.child)
		if !ok {
			return errors.New("target: malformed spawn callback")
		}
		if err := encodeInput(w, x.function); err != nil {
			return err
		}
		if err := w.Bytes(child[:]); err != nil {
			return err
		}
		for _, v := range []uint64{uint64(x.yield), uint64(x.parentResume)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeValues(w, c, x.childEntry); err != nil {
			return err
		}
		if err := encodeValues(w, c, x.resumeValues); err != nil {
			return err
		}
		for _, v := range x.alternatives {
			if err := w.Uint(uint64(v)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.resumes.len())); err != nil {
		return err
	}
	for i := row.resumes.start; i < row.resumes.end; i++ {
		x := c.resumes[i]
		for _, v := range []uint64{uint64(x.source), uint64(x.carrier)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeValues(w, c, x.arguments); err != nil {
			return err
		}
		for _, v := range x.outcomes {
			if err := w.Uint(uint64(v)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.transfers.len())); err != nil {
		return err
	}
	for i := row.transfers.start; i < row.transfers.end; i++ {
		if err := c.encodeTransferRow(w, c.transfers[i]); err != nil {
			return err
		}
	}
	if err := w.Uint(uint64(row.effectTail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.effectVar)); err != nil {
		return err
	}
	if err := w.Count(uint64(row.effects.len())); err != nil {
		return err
	}
	for i := row.effects.start; i < row.effects.end; i++ {
		if err := c.encodePortableEffect(w, c.effects[i]); err != nil {
			return err
		}
	}
	if row.gsubTable == 0 {
		return w.Bool(false)
	}
	if err := w.Bool(true); err != nil {
		return err
	}
	g := c.gsubTables[row.gsubTable-1]
	for _, v := range []uint64{uint64(g.replacement), uint64(GsubTableKeyFirstCaptureOrWholeMatch), uint64(g.resultOutcome), uint64(g.result)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	role := c.subedges[g.access-1].role
	if err := w.Uint(uint64(role)); err != nil {
		return err
	}
	if err := w.Count(uint64(g.effects.len())); err != nil {
		return err
	}
	for i := g.effects.start; i < g.effects.end; i++ {
		if err := w.Uint(uint64(c.gsubEffects[i])); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableCallback(w *canonical.Writer, id CallbackID) error {
	row, ok := c.callback(id)
	if !ok {
		return errors.New("target: malformed callback")
	}
	selector, ok := c.callbackSelector(id)
	if !ok {
		return errors.New("target: missing callback selector")
	}
	if err := w.Bytes(selector[:]); err != nil {
		return err
	}
	if err := encodeInput(w, row.function); err != nil {
		return err
	}
	if err := encodeValues(w, c, row.arguments); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.admission)); err != nil {
		return err
	}
	for _, v := range row.outcomes {
		if err := encodeValues(w, c, v); err != nil {
			return err
		}
	}
	if err := w.Uint(uint64(row.lifecycle)); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.effectTail)); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.effectVar)); err != nil {
		return err
	}
	if err := w.Count(uint64(row.effects.len())); err != nil {
		return err
	}
	for i := row.effects.start; i < row.effects.end; i++ {
		if err := c.encodePortableEffect(w, c.effects[i]); err != nil {
			return err
		}
	}
	if row.release == 0 {
		return w.Bool(false)
	}
	r := c.callbackReleases[row.release-1]
	target, ok := c.anchor(r.operation)
	if !ok {
		return errors.New("target: malformed release target")
	}
	if err := w.Bool(true); err != nil {
		return err
	}
	if err := w.Bytes(target[:]); err != nil {
		return err
	}
	if err := w.Uint(uint64(r.input)); err != nil {
		return err
	}
	for _, outcome := range []uint32{r.outcome, r.zeroOutcome} {
		if r.zeroBehavior == CallbackReleaseZeroSuppress && outcome == r.zeroOutcome {
			if err := w.Bool(false); err != nil {
				return err
			}
			continue
		}
		i, ok := c.outcomeIndex(r.operation, int(outcome))
		if !ok {
			return errors.New("target: malformed release outcome")
		}
		if err := w.Bool(true); err != nil {
			return err
		}
		selector := c.outcomeSelectors[i]
		if err := w.Bytes(selector[:]); err != nil {
			return err
		}
	}
	for _, v := range []uint64{uint64(r.mode), uint64(r.zeroBehavior)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableSubedge(w *canonical.Writer, owner Operation, row subedgeRow) error {
	for _, v := range []uint64{uint64(row.role), uint64(row.family), uint64(row.callee), uint64(row.admission)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	switch row.callee {
	case SubedgeCalleeCallback:
		s, ok := c.callbackSelector(row.callback)
		if !ok {
			return errors.New("target: malformed subedge callback")
		}
		if err := w.Bytes(s[:]); err != nil {
			return err
		}
	case SubedgeCalleeCapturedInitialRead:
		if row.readRoot == 0 || int(row.readRoot) > len(c.initialRoots) {
			return errors.New("target: malformed subedge root")
		}
		if err := w.String(c.initialRoots[row.readRoot-1].identity); err != nil {
			return err
		}
		if err := encodeExactKey(w, c, row.readKey); err != nil {
			return err
		}
	case SubedgeCalleeMetaKey:
		if err := encodeExactKey(w, c, row.metaKey); err != nil {
			return err
		}
	case SubedgeCalleeInvalid:
	default:
		return errors.New("target: malformed subedge callee")
	}
	if err := encodeValues(w, c, row.arguments); err != nil {
		return err
	}
	if err := w.Bool(row.ruleEntry); err != nil {
		return err
	}
	if err := w.Count(uint64(row.argumentOrigins.len())); err != nil {
		return err
	}
	for i := row.argumentOrigins.start; i < row.argumentOrigins.end; i++ {
		x := c.subedgeOrigins[i]
		for _, v := range []uint64{uint64(x.segment), uint64(x.index), uint64(x.kind)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
		if err := encodeInput(w, x.source); err != nil {
			return err
		}
	}
	for _, v := range row.outcomes {
		if err := encodeValues(w, c, v); err != nil {
			return err
		}
	}
	if err := encodeValues(w, c, row.admissionFailure); err != nil {
		return err
	}
	if err := c.encodePortableRoute(w, owner, row.admissionRoute); err != nil {
		return err
	}
	for _, r := range row.routes {
		if err := c.encodePortableRoute(w, owner, r); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) encodePortableRoute(w *canonical.Writer, owner Operation, row subedgeRouteRow) error {
	for _, v := range []uint64{uint64(row.route), uint64(row.adjustment)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if err := encodeValues(w, c, row.result); err != nil {
		return err
	}
	for _, v := range []uint64{uint64(row.placement), uint64(row.offset), uint64(row.outcome)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if row.subedge != 0 {
		edge, ok := c.subedge(row.subedge)
		if !ok || edge.owner != owner {
			return errors.New("target: malformed route sibling")
		}
		if err := w.Uint(uint64(edge.role)); err != nil {
			return err
		}
	} else if err := w.Uint(0); err != nil {
		return err
	}
	return encodeOptionalValues(w, c, row.destination)
}
func encodeOptionalValues(w *canonical.Writer, c *Contract, v Values) error {
	if err := w.Bool(v != 0); err != nil {
		return err
	}
	if v == 0 {
		return nil
	}
	return encodeValues(w, c, v)
}

func (c *Contract) encodePortableOutcome(w *canonical.Writer, owner Operation, flat int) error {
	if flat < 0 || flat >= len(c.outcomes) {
		return errors.New("target: malformed outcome")
	}
	row := c.outcomes[flat]
	if err := w.Uint(uint64(row.kind)); err != nil {
		return err
	}
	if err := encodeValues(w, c, row.values); err != nil {
		return err
	}
	if err := w.Count(uint64(row.produced.len())); err != nil {
		return err
	}
	for i := row.produced.start; i < row.produced.end; i++ {
		p := c.produced[i]
		a, ok := c.anchor(p.target)
		if !ok {
			return errors.New("target: malformed produced anchor")
		}
		if err := w.Uint(uint64(p.result)); err != nil {
			return err
		}
		if err := w.Bytes(a[:]); err != nil {
			return err
		}
		if err := w.Count(uint64(p.captures.len())); err != nil {
			return err
		}
		for j := p.captures.start; j < p.captures.end; j++ {
			x := c.captures[j]
			if err := w.Uint(uint64(x.kind)); err != nil {
				return err
			}
			if x.kind == CaptureCallback {
				s, ok := c.callbackSelector(CallbackID(x.ordinal))
				if !ok {
					return errors.New("target: malformed produced callback")
				}
				if err := w.Bytes(s[:]); err != nil {
					return err
				}
			} else if err := w.Uint(uint64(x.ordinal)); err != nil {
				return err
			}
		}
	}
	if err := w.Count(uint64(row.callbackResults.len())); err != nil {
		return err
	}
	for i := row.callbackResults.start; i < row.callbackResults.end; i++ {
		x := c.callbackResults[i]
		s, ok := c.callbackSelector(x.callback)
		if !ok {
			return errors.New("target: malformed callback result")
		}
		if err := w.Uint(uint64(x.result)); err != nil {
			return err
		}
		if err := w.Bytes(s[:]); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.resultAliases.len())); err != nil {
		return err
	}
	for i := row.resultAliases.start; i < row.resultAliases.end; i++ {
		x := c.resultAliases[i]
		if err := w.Uint(uint64(x.result)); err != nil {
			return err
		}
		if err := encodeInput(w, x.source); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.fresh.len())); err != nil {
		return err
	}
	for i := row.fresh.start; i < row.fresh.end; i++ {
		x := c.fresh[i]
		for _, v := range []uint64{uint64(x.result), uint64(x.ordinal), uint64(x.kind)} {
			if err := w.Uint(v); err != nil {
				return err
			}
		}
	}
	_ = owner
	return nil
}

func (c *Contract) encodeTransferRow(w *canonical.Writer, row transferRow) error {
	if err := w.Uint(uint64(row.endpoint.Kind)); err != nil {
		return err
	}
	if err := w.Uint(uint64(row.endpoint.Input)); err != nil {
		return err
	}
	if err := encodeInput(w, row.payload); err != nil {
		return err
	}
	if err := encodeInput(w, row.alias); err != nil {
		return err
	}
	for _, v := range []uint64{uint64(row.identity), uint64(row.capabilities)} {
		if err := w.Uint(v); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(row.outcomes.len())); err != nil {
		return err
	}
	for i := row.outcomes.start; i < row.outcomes.end; i++ {
		if err := w.Uint(uint64(i - row.outcomes.start)); err != nil {
			return err
		}
		if err := w.Uint(uint64(c.transferOutcomes[i])); err != nil {
			return err
		}
	}
	return nil
}
func (c *Contract) encodePortableEffect(w *canonical.Writer, row effectRow) error {
	a, ok := c.anchor(row.target)
	if !ok {
		return errors.New("target: malformed effect target")
	}
	if err := w.Bytes(a[:]); err != nil {
		return err
	}
	for _, xs := range []indexRange{row.values, row.types, row.valuesVar, row.rows} {
		if err := w.Count(uint64(xs.len())); err != nil {
			return err
		}
	}
	for i := row.values.start; i < row.values.end; i++ {
		if err := w.Uint(uint64(c.effectVals[i])); err != nil {
			return err
		}
	}
	for i := row.types.start; i < row.types.end; i++ {
		if err := w.Uint(uint64(c.effectType[i])); err != nil {
			return err
		}
	}
	for i := row.valuesVar.start; i < row.valuesVar.end; i++ {
		if err := w.Uint(uint64(c.effectVars[i])); err != nil {
			return err
		}
	}
	for i := row.rows.start; i < row.rows.end; i++ {
		if err := w.Uint(uint64(c.effectRows[i])); err != nil {
			return err
		}
	}
	return nil
}

// OperationContentID is an O(1) portable declaration identity. Operation is a
// receiver-local scalar: a numerically coincident foreign handle is
// indistinguishable from this receiver's same coordinate, so callers must
// retain the owning Contract rather than treating this query as an ownership
// proof.
func (c *Contract) OperationContentID(op Operation) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operationContentIDs) {
		return keyspace.ContentID{}, false
	}
	return c.operationContentIDs[op-1], true
}

// OutcomeContentID is the selected operation's exact local outcome relation.
// Both arguments are receiver-local scalar coordinates; see OperationContentID
// for the deliberate foreign-handle limitation.
func (c *Contract) OutcomeContentID(op Operation, index int) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed {
		return keyspace.ContentID{}, false
	}
	i, ok := c.outcomeIndex(op, index)
	if !ok || i >= len(c.outcomeContentIDs) {
		return keyspace.ContentID{}, false
	}
	return c.outcomeContentIDs[i], true
}

// TransferContentID checks that two receiver-local handles agree on their
// owner. It cannot authenticate a numerically coincident foreign scalar.
func (c *Contract) TransferContentID(owner Operation, transfer TransferID) (keyspace.ContentID, bool) {
	row, ok := c.transferID(transfer)
	if !ok || row.owner != owner || !c.sealed {
		return keyspace.ContentID{}, false
	}
	return c.transferContentIDs[transfer-1], true
}

// TransferOutcomeContentID has the same receiver-local scalar limitation as
// TransferContentID; the returned ContentID is portable, not the input handle.
func (c *Contract) TransferOutcomeContentID(owner Operation, transfer TransferID, outcome int) (keyspace.ContentID, TransferPossibility, bool) {
	row, ok := c.transferID(transfer)
	if !ok || row.owner != owner || outcome < 0 || outcome >= row.outcomes.len() || !c.sealed {
		return keyspace.ContentID{}, TransferPossibility(0), false
	}
	i := int(row.outcomes.start) + outcome
	return c.transferOutcomeIDs[i], c.transferOutcomes[i], true
}

// EffectOperationID is the narrow operation ABI visible to effect
// substitution.  It excludes all operation outcomes, effects, callbacks,
// transfers, and other dynamic relations.
func (c *Contract) EffectOperationID(op Operation) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.effectOperationIDs) {
		return keyspace.ContentID{}, false
	}
	return c.effectOperationIDs[op-1], true
}

// EffectDescriptorID returns the semantic quotient for one ordinary
// operation effect occurrence.  Equal descriptors may intentionally occur at
// several local positions; use EffectOccurrenceID when occurrence evidence is
// required.
func (c *Contract) EffectDescriptorID(op Operation, index int) (keyspace.ContentID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return keyspace.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectDescriptorIDs) {
		return keyspace.ContentID{}, false
	}
	id := c.effectDescriptorIDs[position]
	return id, id.Available()
}

// CallbackEffectDescriptorID returns the semantic quotient for one callback
// effect occurrence.  It has no inverse because duplicate descriptors are a
// deliberate quotient of distinct retained occurrences.
func (c *Contract) CallbackEffectDescriptorID(callback CallbackID, index int) (keyspace.ContentID, bool) {
	row, ok := c.callback(callback)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return keyspace.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectDescriptorIDs) {
		return keyspace.ContentID{}, false
	}
	id := c.effectDescriptorIDs[position]
	return id, id.Available()
}

// EffectOccurrenceID returns the exact ordinary effect occurrence identity,
// including its canonical local row position.
func (c *Contract) EffectOccurrenceID(op Operation, index int) (keyspace.ContentID, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return keyspace.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectOccurrenceIDs) {
		return keyspace.ContentID{}, false
	}
	id := c.effectOccurrenceIDs[position]
	return id, id.Available()
}

// CallbackEffectOccurrenceID returns the exact callback effect occurrence
// identity, including callback correspondence and canonical local position.
func (c *Contract) CallbackEffectOccurrenceID(callback CallbackID, index int) (keyspace.ContentID, bool) {
	row, ok := c.callback(callback)
	if !ok || index < 0 || index >= row.effects.len() || !c.sealed {
		return keyspace.ContentID{}, false
	}
	position := int(row.effects.start) + index
	if position < 0 || position >= len(c.effectOccurrenceIDs) {
		return keyspace.ContentID{}, false
	}
	id := c.effectOccurrenceIDs[position]
	return id, id.Available()
}

// EffectRowFamilyID identifies an operation's complete effect row, including
// its tail/variable schema and ordered occurrence identities.  Empty and
// opaque rows therefore receive a real family identity too.
func (c *Contract) EffectRowFamilyID(op Operation) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operationEffectFamilies) {
		return keyspace.ContentID{}, false
	}
	id := c.operationEffectFamilies[op-1]
	return id, id.Available()
}

// CallbackEffectRowFamilyID identifies a callback's complete expected effect
// row, including its tail/variable schema and ordered occurrence identities.
func (c *Contract) CallbackEffectRowFamilyID(callback CallbackID) (keyspace.ContentID, bool) {
	if c == nil || !c.sealed || callback == 0 || int(callback) > len(c.callbackEffectFamilies) {
		return keyspace.ContentID{}, false
	}
	id := c.callbackEffectFamilies[callback-1]
	return id, id.Available()
}

var _ = flowkind.OutcomeNormal
