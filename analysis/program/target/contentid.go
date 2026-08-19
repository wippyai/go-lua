package target

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// Version 23 carries the complete neutral type-contract declaration for each
// frozen type row, including primitive identity and external formal scope.
// It also carries the opaque operation behavior result/predicate rows.
// Version 21 adds explicit publication-effect presence and typed descriptor
// bytes to each effect row. A target identity from any preceding layout must
// never be reused for a contract with publication semantics.
// Version 20 adds the exact initial whole-object immutable header to every
// boot shape. A target identity from any preceding layout must never be reused
// for a different bootstrap Heap header.
// Version 19 adds the closed operation-subedge relation branch.
// Version 18 adds retained callback-holder protocol rows and the mandatory
// zero-holder branch of a callback release. A target identity from any
// preceding layout must never be reused as this schema.
const contentIDCodecVersion = 23

// ContentID derives the SHA-256 identity of the complete observable sealed
// contract. It encodes no authoring references, Go object identities, lookup
// indices, capacities, or other derived implementation caches.
func (c *Contract) ContentID() (id identity.ContentID) {
	// Contract is intentionally unconstructable outside this package, but this
	// boundary must still fail closed for a partially assembled internal value.
	// The recovery also keeps a future decoder bug from publishing a digest for
	// a panicking malformed table.
	defer func() {
		if recover() != nil {
			id = identity.ContentID{}
		}
	}()
	opaque, opaqueOK := c.Operations.Opaque()
	if c == nil || !c.sealed || !opaqueOK || uint64(opaque) != uint64(c.Operations.OperationCount()) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	if err := encodeContractCanonical(hash, c); err != nil {
		return identity.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// Record kinds are part of target-contract/v1. They are intentionally local
// to this semantic codec; framing.Writer does not know target's schema.
const (
	recordContract uint64 = iota + 1
	recordOperation
	recordBinding
	recordValues
	recordOutcome
	recordCallback
	recordSubedge
	recordCallbackRelease
	recordSuspension
	recordSpawn
	recordResume
	recordTransfer
	recordEffect
	recordProduced
	recordCapture
	recordCallbackResult
	recordResultAlias
	recordProtocol
	recordState
	recordAcquisition
	recordTransition
	recordTransitionOutcome
	recordEscape
	recordProtocolCallbackHolder
	recordInitialRoot
	recordBootShape
	recordInitialEntry
	recordInitialBinding
	recordInitialValue
	recordFreshResult
	recordInitialMetatableAttachment
	recordOperationSubedgeRelation
)

func encodeCoordinate(w *framing.Writer, kind, ordinal uint64) error {
	if err := w.Uint(kind); err != nil {
		return err
	}
	return w.Uint(ordinal)
}

func encodeContractCanonical(dst interface{ Write([]byte) (int, error) }, c *Contract) error {
	var w framing.Writer
	if err := w.Reset(dst, "program/target-contract", contentIDCodecVersion); err != nil {
		return err
	}
	if err := encodeContract(&w, c); err != nil {
		return err
	}
	return w.Finish()
}

func encodeContract(w *framing.Writer, c *Contract) error {
	if c == nil {
		return errors.New("target: unavailable contract")
	}
	if err := w.Record(recordContract); err != nil {
		return err
	}
	operations := c.Operations.OperationCount()
	if err := w.Count(uint64(operations)); err != nil {
		return err
	}
	for index := 0; index < operations; index++ {
		op, ok := c.Operations.OperationAt(index)
		if !ok {
			return errors.New("target: malformed operation table")
		}
		if err := encodeOperation(w, c, op); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(c.protocols.ProtocolCount())); err != nil {
		return err
	}
	if err := c.protocols.Encode(w); err != nil {
		return err
	}
	if err := c.Table.Encode(w, c.exactKeys); err != nil {
		return err
	}
	return nil
}
