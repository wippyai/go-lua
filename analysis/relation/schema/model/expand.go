package model

import "github.com/wippyai/go-lua/analysis/identity"

// ExpandContract is the one model-owned contract for a dependent key-vector
// join. C is the left/source relation, P is the owner relation that publishes
// the finite key vector, and R is the relation whose rows are redeemed by the
// vector. Correlation is the owner-issued relation identity that proves which
// candidate directory P enumerates; it replaces a local direct/corresponded
// enum and therefore cannot become a second correlation vocabulary. The
// publisher's vector is an authored sparse extent; its order is intrinsic to
// the owner evidence. Neither the logical contract nor its digest carries a
// physical ordering mode, completion denominator, coordinate, ordinal,
// callback, or runtime token.
type ExpandContract struct {
	candidate   RelationID
	publisher   RelationID
	reader      RelationID
	key         ColumnID
	correlation RelationID
	scope       ScopeID
}

// DefineExpandContract freezes the logical identities consumed by Expand.
// Malformed or foreign identities remain representable for independent
// checker passes; Available reports only the local closed shape.
func DefineExpandContract(candidate, publisher, reader RelationID, key ColumnID, correlation RelationID) ExpandContract {
	return ExpandContract{
		candidate:   candidate,
		publisher:   publisher,
		reader:      reader,
		key:         key,
		correlation: correlation,
	}
}

// Candidate returns the left/source relation C.
func (contract ExpandContract) Candidate() RelationID { return contract.candidate }

// Publisher returns the owner relation P that publishes the key vector.
func (contract ExpandContract) Publisher() RelationID { return contract.publisher }

// Reader returns the redeemed relation R.
func (contract ExpandContract) Reader() RelationID { return contract.reader }

// Key returns the R-owned column addressed by each published semantic key.
func (contract ExpandContract) Key() ColumnID { return contract.key }

// Correlation returns the owner-issued relation identity proving P's subject
// order corresponds to the reader's candidate order.
func (contract ExpandContract) Correlation() RelationID { return contract.correlation }

// Scope returns the optional decision scope carried by the read. Scope is
// orthogonal to the C/P/R span identities and is absent only for standalone
// logical specimens that have not yet been placed.
func (contract ExpandContract) Scope() ScopeID { return contract.scope }

// WithScope attaches the sealed read scope without reopening any span
// identity. It is used by relcompile when the composition placement is known.
func (contract ExpandContract) WithScope(scope ScopeID) ExpandContract {
	contract.scope = scope
	return contract
}

// Available reports the local shape of a complete dependent-join contract.
// Cross-schema membership, correspondence, and denominator authority remain
// the responsibility of relation/check and the declaration registry.
func (contract ExpandContract) Available() bool {
	if !contract.candidate.Available() || !contract.publisher.Available() ||
		!contract.reader.Available() || !contract.key.Available() ||
		!contract.correlation.Available() {
		return false
	}
	if contract.key.Relation() != contract.reader {
		return false
	}
	return true
}

// Digest returns the canonical logical identity of the dependent-join
// contract. It includes every semantic identity and delivery choice in the
// contract and scope. Mount coordinate contents are deliberately absent: they
// are evidence for this sealed contract, not logical identity.
func (contract ExpandContract) Digest() identity.ContentID {
	if !contract.Available() {
		return identity.ContentID{}
	}
	candidateOwner, candidateContent := contract.candidate.Owner().Content(), contract.candidate.Content()
	publisherOwner, publisherContent := contract.publisher.Owner().Content(), contract.publisher.Content()
	readerOwner, readerContent := contract.reader.Owner().Content(), contract.reader.Content()
	keyRelationOwner, keyRelationContent := contract.key.Relation().Owner().Content(), contract.key.Relation().Content()
	keyContent := contract.key.Content()
	correlationOwner, correlationContent := contract.correlation.Owner().Content(), contract.correlation.Content()
	parts := [][]byte{
		candidateOwner[:], candidateContent[:],
		publisherOwner[:], publisherContent[:],
		readerOwner[:], readerContent[:],
		keyRelationOwner[:], keyRelationContent[:], keyContent[:],
		correlationOwner[:], correlationContent[:],
	}
	if contract.scope.Available() {
		scopeOwner, scopeContent := contract.scope.Owner().Content(), contract.scope.Content()
		parts = append(parts, []byte{1}, scopeOwner[:], scopeContent[:])
	} else {
		parts = append(parts, []byte{0})
	}
	digest, ok := identity.DeriveContentID("analysis/relation/schema/model/expand/v2", parts...)
	if !ok {
		return identity.ContentID{}
	}
	return digest
}
