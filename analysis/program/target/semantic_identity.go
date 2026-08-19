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
	return c.operationCore.Anchor(op)
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
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operations) || callback == 0 || int(callback) > len(c.callbacks) || int(callback) > len(c.callbackContentIDs) {
		return identity.ContentID{}, false
	}
	row := c.callbacks[callback-1]
	owner, ok := c.operation(op)
	if !ok || row.owner != op || uint64(callback-1) < uint64(owner.callbacks.start) || uint64(callback-1) >= uint64(owner.callbacks.end) {
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
func (c *Contract) ResumeContentID(op vocabulary.Operation, resume vocabulary.ResumeID) (identity.ContentID, bool) {
	if c == nil || !c.sealed || op == 0 || int(op) > len(c.operations) || resume == 0 || int(resume) > len(c.resumes) || int(resume) > len(c.resumeContentIDs) {
		return identity.ContentID{}, false
	}
	row := c.resumes[resume-1]
	owner, ok := c.operation(op)
	if !ok || row.owner != op || uint64(resume-1) < uint64(owner.resumes.start) || uint64(resume-1) >= uint64(owner.resumes.end) {
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
	got, ok := c.ResumeContentID(row.op, row.resume)
	if !ok || got != id {
		return 0, 0, false
	}
	return row.op, row.resume, true
}

func (c *Contract) outcomeIndex(op vocabulary.Operation, index int) (int, bool) {
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
	c.callbackSelectors = make([]identity.ContentID, len(c.callbacks))
	c.outcomeSelectors = make([]identity.ContentID, len(c.outcomes))
	outcomeOwners := make([]vocabulary.Operation, len(c.outcomes))
	outcomeOrdinals := make([]uint32, len(c.outcomes))
	c.operationContentIDs = make([]identity.ContentID, len(c.operations))
	c.outcomeContentIDs = make([]identity.ContentID, len(c.outcomes))
	c.transferContentIDs = make([]identity.ContentID, len(c.transfers))
	c.transferOutcomeIDs = make([]identity.ContentID, len(c.transferOutcomes))
	c.callbackContentIDs = make([]identity.ContentID, len(c.callbacks))
	c.callbackContentIndex = make([]callbackContentIDRow, 0, len(c.callbacks))
	c.resumeContentIDs = make([]identity.ContentID, len(c.resumes))
	c.resumeContentIndex = make([]resumeContentIDRow, 0, len(c.resumes))

	// Dense outcome owner/ordinal columns are formed once in table order.  They
	// make the remaining identity pass strictly linear in the sealed tables.
	for operationIndex, operation := range c.operations {
		owner := vocabulary.Operation(operationIndex + 1)
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
		id, err := c.semanticID(semanticOutcomeSelector, func(w *framing.Writer) error {
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

	for i, row := range c.callbacks {
		owner, ok := c.anchor(row.owner)
		if !ok {
			return errors.New("target: malformed callback owner")
		}
		id, err := c.semanticID(semanticCallbackSelector, func(w *framing.Writer) error {
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
		selector, ok := c.callbackSelector(vocabulary.CallbackID(i + 1))
		if !ok {
			return errors.New("target: missing callback selector")
		}
		ownerRow, ok := c.operation(row.owner)
		if !ok || uint64(i) < uint64(ownerRow.callbacks.start) || uint64(i) >= uint64(ownerRow.callbacks.end) {
			return errors.New("target: callback content owner fence")
		}
		id, err := c.semanticID(semanticCallbackContent, func(w *framing.Writer) error {
			if err := w.Bytes(owner[:]); err != nil {
				return err
			}
			return w.Bytes(selector[:])
		})
		if err != nil {
			return err
		}
		c.callbackContentIDs[i] = id
		c.callbackContentIndex = append(c.callbackContentIndex, callbackContentIDRow{id: id, op: row.owner, callback: vocabulary.CallbackID(i + 1)})
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
		id, err := c.semanticID(semanticResumeContent, func(w *framing.Writer) error {
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
		c.resumeContentIndex = append(c.resumeContentIndex, resumeContentIDRow{id: id, op: row.owner, resume: vocabulary.ResumeID(i + 1)})
	}

	for i := range c.operations {
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
	for oi := range c.outcomes {
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
			return c.encodePortableOutcome(w, vocabulary.Operation(owner), oi)
		})
		if err != nil {
			return err
		}
		c.outcomeContentIDs[oi] = id
	}
	for i, row := range c.transfers {
		ownerID := c.operationContentIDs[row.owner-1]
		id, err := c.semanticID(semanticTransfer, func(w *framing.Writer) error {
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
	row, ok := c.transferID(transfer)
	if !ok || row.owner != owner || !c.sealed {
		return identity.ContentID{}, false
	}
	return c.transferContentIDs[transfer-1], true
}

// transferOutcomeContentID has the same receiver-local scalar limitation as
// TransferContentID; the returned ContentID is portable, not the input handle.
func (c *Contract) transferOutcomeContentID(owner vocabulary.Operation, transfer vocabulary.TransferID, outcome int) (identity.ContentID, vocabulary.TransferPossibility, bool) {
	row, ok := c.transferID(transfer)
	if !ok || row.owner != owner || outcome < 0 || outcome >= row.outcomes.len() || !c.sealed {
		return identity.ContentID{}, vocabulary.TransferPossibility(0), false
	}
	i := int(row.outcomes.start) + outcome
	return c.transferOutcomeIDs[i], c.transferOutcomes[i], true
}

// EffectOperationID is the narrow operation ABI visible to effect
// substitution.  It excludes all operation outcomes, effects, callbacks,
// transfers, and other dynamic relations.
