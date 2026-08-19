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
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// Version 8 adds opaque operation behavior rows to semantic operation
// identities; version 7 added publication-effect descriptor bytes. Earlier
// endpoint identities are deliberately distinct.

const endpointIdentityCodecVersion uint64 = 8

const (
	semanticCallbackSelector uint64 = iota + 83
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

func (c *Contract) semanticID(kind uint64, encode func(*framing.Writer) error) (id identity.ContentID, err error) {
	if c == nil || encode == nil {
		return id, errors.New("target: missing semantic identity")
	}
	h := sha256.New()
	var w framing.Writer
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
		return identity.ContentID{}, errors.New("target: semantic digest failure")
	}
	return id, nil
}

func (c *Contract) anchor(op vocabulary.Operation) (identity.ContentID, bool) {
	if c == nil || op == 0 {
		return identity.ContentID{}, false
	}
	return c.Operations.Anchor(op)
}

func (c *Contract) callbackSelector(id vocabulary.CallbackID) (identity.ContentID, bool) {
	if c == nil || id == 0 || int(id) > len(c.callbackSelectors) {
		return identity.ContentID{}, false
	}
	return c.callbackSelectors[id-1], true
}

// CallbackContentID identifies one exact callback correspondence under its
// owning operation. The operation and callback handles are receiver-local;
// the explicit range fence prevents a numerically coincident callback from a
// different operation from being accepted.
func (c *Contract) CallbackContentID(op vocabulary.Operation, callback vocabulary.CallbackID) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > c.Operations.OperationCount() || callback == 0 || int(callback) > len(c.callbackContentIDs) {
		return identity.ContentID{}, false
	}
	owner, callbackIndex, ownerOK := c.Operations.CallbackIndex(callback)
	if !ownerOK || owner != op || callbackIndex < 0 || callbackIndex >= c.Operations.CallbackCount(op) {
		return identity.ContentID{}, false
	}
	id := c.callbackContentIDs[callback-1]
	if !id.Available() {
		return identity.ContentID{}, false
	}
	return id, true
}

// findCallbackContentID is the allocation-free O(log n) inverse over the
// immutable sorted callback-content column.
func (c *Contract) findCallbackContentID(id identity.ContentID) (vocabulary.Operation, vocabulary.CallbackID, bool) {
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
	owner, ownerOK := c.Operations.CallbackOwner(row.callback)
	if !ownerOK {
		return 0, 0, false
	}
	got, ok := c.CallbackContentID(owner, row.callback)
	if !ok || got != id {
		return 0, 0, false
	}
	return owner, row.callback, true
}

// ResumeContentID identifies one exact operation-owned resumption
// correspondence. Its canonical record is derived during Seal from the
// operation anchor, source/carrier, argument Values, and all five sealed
// outcome selectors.
func (c *Contract) ResumeContentID(op vocabulary.Operation, resume vocabulary.ResumeID) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > c.Operations.OperationCount() || resume == 0 || int(resume) > len(c.resumeContentIDs) {
		return identity.ContentID{}, false
	}
	owner, _, _, _, ok := c.Operations.Resume(resume)
	if !ok || owner != op {
		return identity.ContentID{}, false
	}
	id := c.resumeContentIDs[resume-1]
	if !id.Available() {
		return identity.ContentID{}, false
	}
	return id, true
}

// findResumeContentID is the allocation-free O(log n) inverse over the
// immutable sorted resume-content column.
func (c *Contract) findResumeContentID(id identity.ContentID) (vocabulary.Operation, vocabulary.ResumeID, bool) {
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
	if row.resume == 0 || int(row.resume) > len(c.resumeContentIDs) {
		return 0, 0, false
	}
	owner, _, _, _, ownerOK := c.Operations.Resume(row.resume)
	if !ownerOK {
		return 0, 0, false
	}
	got, ok := c.ResumeContentID(owner, row.resume)
	if !ok || got != id {
		return 0, 0, false
	}
	return owner, row.resume, true
}

func (c *Contract) outcomeIndex(op vocabulary.Operation, index int) (int, bool) {
	return c.Operations.OutcomePositionAt(op, index)
}

// sealSemanticIdentities is one finite finalization phase.  It retains only
// dense immutable result columns; all sorting/graph validation belongs to the
// normal Target Seal phase above.
func (c *Contract) sealSemanticIdentities() error {
	callbackCount := 0
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		operation, ok := c.Operations.OperationAt(operationIndex)
		if !ok {
			return errors.New("target: malformed callback owner")
		}
		callbackCount += c.Operations.CallbackCount(operation)
	}
	c.callbackSelectors = make([]identity.ContentID, callbackCount)
	outcomeCount := 0
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		operation, ok := c.Operations.OperationAt(operationIndex)
		if !ok {
			return errors.New("target: malformed operation outcome owner")
		}
		outcomeCount += c.Operations.OutcomeCount(operation)
	}
	c.outcomeSelectors = make([]identity.ContentID, outcomeCount)
	outcomeOwners := make([]vocabulary.Operation, outcomeCount)
	outcomeOrdinals := make([]uint32, outcomeCount)
	c.operationContentIDs = make([]identity.ContentID, c.Operations.OperationCount())
	c.outcomeContentIDs = make([]identity.ContentID, outcomeCount)
	transferCount, transferOutcomeCount := 0, 0
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		op := vocabulary.Operation(operationIndex + 1)
		transferCount += c.Operations.TransferCount(op)
		for transferIndex := 0; transferIndex < c.Operations.TransferCount(op); transferIndex++ {
			transferOutcomeCount += c.Operations.TransferOutcomeCount(op, transferIndex)
		}
	}
	c.transferContentIDs = make([]identity.ContentID, transferCount)
	c.transferOutcomeIDs = make([]identity.ContentID, transferOutcomeCount)
	c.callbackContentIDs = make([]identity.ContentID, callbackCount)
	c.callbackContentIndex = make([]callbackContentIDRow, 0, callbackCount)
	resumeCount := 0
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		operation, ok := c.Operations.OperationAt(operationIndex)
		if !ok {
			return errors.New("target: malformed resume owner")
		}
		resumeCount += c.Operations.ResumeCount(operation)
	}
	c.resumeContentIDs = make([]identity.ContentID, resumeCount)
	c.resumeContentIndex = make([]resumeContentIDRow, 0, resumeCount)

	// Dense outcome owner/ordinal columns are formed once in table order.  They
	// make the remaining identity pass strictly linear in the sealed tables.
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		owner, ok := c.Operations.OperationAt(operationIndex)
		if !ok {
			return errors.New("target: malformed operation outcome owner")
		}
		for ordinal := 0; ordinal < c.Operations.OutcomeCount(owner); ordinal++ {
			flat, flatOK := c.Operations.OutcomePositionAt(owner, ordinal)
			if !flatOK || flat < 0 || flat >= len(outcomeOwners) {
				return errors.New("target: malformed operation outcome position")
			}
			outcomeOwners[flat] = owner
			outcomeOrdinals[flat] = uint32(ordinal)
		}
	}

	// Outcome selectors are deliberately owner-free: they are the fixed local
	// discriminator needed while deriving a produced child's parent anchor.
	for i := range c.outcomeSelectors {
		owner := outcomeOwners[i]
		local := outcomeOrdinals[i]
		kind, values, outcomeOK := c.Operations.OutcomeAt(owner, int(local))
		if !outcomeOK {
			return errors.New("target: malformed outcome")
		}
		id, err := c.semanticID(semanticOutcomeSelector, func(w *framing.Writer) error {
			if err := w.Uint(uint64(local)); err != nil {
				return err
			}
			if err := w.Uint(uint64(kind)); err != nil {
				return err
			}
			if err := encodeValues(w, c, values); err != nil {
				return err
			}
			if err := w.Count(uint64(c.Operations.FreshResultCount(owner, int(local)))); err != nil {
				return err
			}
			for j := 0; j < c.Operations.FreshResultCount(owner, int(local)); j++ {
				result, ordinal, freshKind, freshOK := c.Operations.FreshResultAt(owner, int(local), j)
				if !freshOK {
					return errors.New("target: malformed fresh result")
				}
				if err := w.Uint(uint64(result)); err != nil {
					return err
				}
				if err := w.Uint(uint64(ordinal)); err != nil {
					return err
				}
				if err := w.Uint(uint64(freshKind)); err != nil {
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

	for i := 0; i < callbackCount; i++ {
		callbackID := vocabulary.CallbackID(i + 1)
		owner, callbackIndex, callbackOK := c.Operations.CallbackIndex(callbackID)
		anchorID, anchorOK := c.anchor(owner)
		if !callbackOK || !anchorOK {
			return errors.New("target: malformed callback owner")
		}
		lifecycle, lifecycleOK := c.Operations.CallbackLifecycle(callbackID)
		if !lifecycleOK {
			return errors.New("target: malformed callback lifecycle")
		}
		source, sourceOK := c.Operations.CallbackSource(callbackID)
		arguments, argumentsOK := c.Operations.CallbackArguments(callbackID)
		admission, admissionOK := c.Operations.CallbackAdmission(callbackID)
		if !sourceOK || !argumentsOK || !admissionOK {
			return errors.New("target: malformed callback values")
		}
		id, err := c.semanticID(semanticCallbackSelector, func(w *framing.Writer) error {
			if err := w.Bytes(anchorID[:]); err != nil {
				return err
			}
			if err := w.Uint(uint64(callbackIndex)); err != nil {
				return err
			}
			if err := encodeInput(w, source); err != nil {
				return err
			}
			if err := encodeValues(w, c, arguments); err != nil {
				return err
			}
			for _, kind := range [...]flowkind.OutcomeKind{
				flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
				flowkind.OutcomeYield, flowkind.OutcomeCancel,
			} {
				values, valuesOK := c.Operations.CallbackOutcome(callbackID, kind)
				if !valuesOK {
					return errors.New("target: malformed callback outcome")
				}
				if err := encodeValues(w, c, values); err != nil {
					return err
				}
			}
			if err := w.Uint(uint64(admission)); err != nil {
				return err
			}
			return w.Uint(uint64(lifecycle))
		})
		if err != nil {
			return err
		}
		c.callbackSelectors[i] = id
	}
	for i := 0; i < callbackCount; i++ {
		callbackID := vocabulary.CallbackID(i + 1)
		owner, _, ownerOK := c.Operations.CallbackIndex(callbackID)
		anchorID, anchorOK := c.anchor(owner)
		if !ownerOK || !anchorOK {
			return errors.New("target: malformed callback content owner")
		}
		selector, ok := c.callbackSelector(vocabulary.CallbackID(i + 1))
		if !ok {
			return errors.New("target: missing callback selector")
		}
		id, err := c.semanticID(semanticCallbackContent, func(w *framing.Writer) error {
			if err := w.Bytes(anchorID[:]); err != nil {
				return err
			}
			return w.Bytes(selector[:])
		})
		if err != nil {
			return err
		}
		c.callbackContentIDs[i] = id
		c.callbackContentIndex = append(c.callbackContentIndex, callbackContentIDRow{id: id, callback: callbackID})
	}
	sealedOperations, err := c.Operations.SealEffectIdentities(operationvalue.EffectIdentityContext{
		SemanticID:   c.semanticID,
		EncodeValues: func(w *framing.Writer, values vocabulary.Values) error { return encodeValues(w, c, values) },
		EncodeType:   func(w *framing.Writer, typ vocabulary.Type) error { return encodeType(w, c, typ) },
		CallbackContentID: func(callback vocabulary.CallbackID) (identity.ContentID, bool) {
			if callback == 0 || int(callback) > len(c.callbackContentIDs) {
				return identity.ContentID{}, false
			}
			id := c.callbackContentIDs[callback-1]
			return id, id.Available()
		},
		EffectOperationKind:       semanticEffectOperation,
		EffectDescriptorKind:      semanticEffectDescriptor,
		EffectOccurrenceKind:      semanticEffectOccurrence,
		OperationEffectFamilyKind: semanticOperationEffectFamily,
		CallbackEffectFamilyKind:  semanticCallbackEffectFamily,
	})
	if err != nil {
		return err
	}
	c.Operations = sealedOperations

	resumeOrdinal := 0
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		operation, operationOK := c.Operations.OperationAt(operationIndex)
		if !operationOK {
			return errors.New("target: malformed resume content owner")
		}
		for resumeIndex := 0; resumeIndex < c.Operations.ResumeCount(operation); resumeIndex++ {
			resume, resumeOK := c.Operations.ResumeIDAt(operation, resumeIndex)
			if !resumeOK {
				return errors.New("target: malformed resume content handle")
			}
			ownerID, ownerOK := c.anchor(operation)
			if !ownerOK {
				return errors.New("target: malformed resume content owner")
			}
			_, source, carrier, arguments, rowOK := c.Operations.Resume(resume)
			if !rowOK {
				return errors.New("target: malformed resume content row")
			}
			id, err := c.semanticID(semanticResumeContent, func(w *framing.Writer) error {
				if err := w.Bytes(ownerID[:]); err != nil {
					return err
				}
				if err := w.Uint(uint64(source)); err != nil {
					return err
				}
				if err := w.Uint(uint64(carrier)); err != nil {
					return err
				}
				if err := encodeValues(w, c, arguments); err != nil {
					return err
				}
				for outcome := 0; outcome < c.Operations.ResumeOutcomeCount(resume); outcome++ {
					_, targetOutcome, outcomeOK := c.Operations.ResumeOutcomeAt(resume, outcome)
					if !outcomeOK {
						return errors.New("target: malformed resume content outcome")
					}
					position, positionOK := c.Operations.OutcomePositionAt(operation, int(targetOutcome))
					if !positionOK || position < 0 || position >= len(c.outcomeSelectors) {
						return errors.New("target: malformed resume content outcome")
					}
					selector := c.outcomeSelectors[position]
					if err := w.Bytes(selector[:]); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			c.resumeContentIDs[resumeOrdinal] = id
			c.resumeContentIndex = append(c.resumeContentIndex, resumeContentIDRow{id: id, resume: resume})
			resumeOrdinal++
		}
	}

	for i := 0; i < c.Operations.OperationCount(); i++ {
		op := vocabulary.Operation(i + 1)
		anchor, anchorOK := c.anchor(op)
		if !anchorOK {
			return errors.New("target: missing operation anchor")
		}
		id, err := c.semanticID(semanticOperation, func(w *framing.Writer) error {
			if err := w.Bytes(anchor[:]); err != nil {
				return err
			}
			return c.encodePortableOperation(w, op)
		})
		if err != nil {
			return err
		}
		c.operationContentIDs[i] = id
	}
	for oi := range c.outcomeSelectors {
		owner := outcomeOwners[oi]
		if owner == 0 {
			return errors.New("target: malformed outcome owner")
		}
		ownerID := c.operationContentIDs[owner-1]
		selector := c.outcomeSelectors[oi]
		id, err := c.semanticID(semanticOutcome, func(w *framing.Writer) error {
			if err := w.Bytes(ownerID[:]); err != nil {
				return err
			}
			if err := w.Bytes(selector[:]); err != nil {
				return err
			}
			return c.encodePortableOutcome(w, owner, int(outcomeOrdinals[oi]))
		})
		if err != nil {
			return err
		}
		c.outcomeContentIDs[oi] = id
	}
	for operationIndex := 0; operationIndex < c.Operations.OperationCount(); operationIndex++ {
		owner := vocabulary.Operation(operationIndex + 1)
		ownerID := c.operationContentIDs[operationIndex]
		for transferIndex := 0; transferIndex < c.Operations.TransferCount(owner); transferIndex++ {
			transfer, transferOK := c.Operations.TransferIDAt(owner, transferIndex)
			if !transferOK {
				return errors.New("target: malformed transfer handle")
			}
			id, err := c.semanticID(semanticTransfer, func(w *framing.Writer) error {
				if err := w.Bytes(ownerID[:]); err != nil {
					return err
				}
				return c.encodeTransfer(w, owner, transferIndex)
			})
			if err != nil {
				return err
			}
			c.transferContentIDs[transfer-1] = id
			for outcome := 0; outcome < c.Operations.TransferOutcomeCount(owner, transferIndex); outcome++ {
				oi, ok := c.outcomeIndex(owner, outcome)
				if !ok {
					return errors.New("target: malformed transfer outcome")
				}
				outcomeID := c.outcomeContentIDs[oi]
				_, possibility, possibilityOK := c.Operations.TransferDeclarationOutcomeAt(transfer, outcome)
				position, positionOK := c.Operations.TransferOutcomePositionAt(transfer, outcome)
				if !possibilityOK || !positionOK || position >= len(c.transferOutcomeIDs) {
					return errors.New("target: malformed transfer outcome")
				}
				transferID := id
				outID, err := c.semanticID(semanticTransferOutcome, func(w *framing.Writer) error {
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
				c.transferOutcomeIDs[position] = outID
			}
		}
	}
	return c.sealHostIdentityRelations(outcomeOwners, outcomeOrdinals)
}

// sealEffectIdentities derives the complete Target-owned identity surface for
// authored effect rows.  The only retained state is one dense column per
// existing Target row; all relation payload remains in the ordinary effect,
// operation, and callback pools above.  In particular, there is deliberately
// no descriptor index: duplicate occurrences are valid and share a semantic
// OperationContentID is an O(1) portable declaration identity. Operation is a
// receiver-local scalar: a numerically coincident foreign handle is
// indistinguishable from this receiver's same coordinate, so callers must
// retain the owning Contract rather than treating this query as an ownership
// proof.
func (c *Contract) OperationContentID(op vocabulary.Operation) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operationContentIDs) {
		return identity.ContentID{}, false
	}
	return c.operationContentIDs[op-1], true
}

// outcomeContentID is the selected operation's exact local outcome relation.
// Both arguments are receiver-local scalar coordinates; see OperationContentID
// for the deliberate foreign-handle limitation.
func (c *Contract) outcomeContentID(op vocabulary.Operation, index int) (identity.ContentID, bool) {
	if c == nil || !c.sealed {
		return identity.ContentID{}, false
	}
	i, ok := c.outcomeIndex(op, index)
	if !ok || i >= len(c.outcomeContentIDs) {
		return identity.ContentID{}, false
	}
	return c.outcomeContentIDs[i], true
}

// transferContentID checks that two receiver-local handles agree on their
// owner. It cannot authenticate a numerically coincident foreign scalar.
func (c *Contract) transferContentID(owner vocabulary.Operation, transfer vocabulary.TransferID) (identity.ContentID, bool) {
	transferOwner, ok := c.Operations.TransferOwner(transfer)
	if !ok || transferOwner != owner || !c.sealed || int(transfer) > len(c.transferContentIDs) {
		return identity.ContentID{}, false
	}
	return c.transferContentIDs[transfer-1], true
}

// transferOutcomeContentID has the same receiver-local scalar limitation as
// TransferContentID; the returned ContentID is portable, not the input handle.
func (c *Contract) transferOutcomeContentID(owner vocabulary.Operation, transfer vocabulary.TransferID, outcome int) (identity.ContentID, vocabulary.TransferPossibility, bool) {
	transferOwner, ok := c.Operations.TransferOwner(transfer)
	if !ok || transferOwner != owner || outcome < 0 || !c.sealed {
		return identity.ContentID{}, vocabulary.TransferPossibility(0), false
	}
	i, positionOK := c.Operations.TransferOutcomePositionAt(transfer, outcome)
	if !positionOK || i >= len(c.transferOutcomeIDs) {
		return identity.ContentID{}, vocabulary.TransferPossibility(0), false
	}
	_, possibility, possibilityOK := c.Operations.TransferDeclarationOutcomeAt(transfer, outcome)
	if !possibilityOK {
		return identity.ContentID{}, vocabulary.TransferPossibility(0), false
	}
	return c.transferOutcomeIDs[i], possibility, true
}

// EffectOperationID is the narrow operation ABI visible to effect
// substitution.  It excludes all operation outcomes, effects, callbacks,
// transfers, and other dynamic relations.
