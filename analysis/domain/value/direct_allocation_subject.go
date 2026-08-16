package value

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/analysis/identity"
)

const directAllocationSubjectDomain = "wippy.analysis.value.direct-allocation-subject.v1\x00"

// DirectAllocationSubject is a Value-owned, owner-fenced proof that one exact
// mounted semantic source is the allocation root itself. Its public identity
// is detached; the private owner fences remain only until the next live Pack
// join. It is deliberately a direct identity proof, not an alias, escape,
// uniqueness, frozen, lifetime, or placement conclusion.
//
// The coordinate ordinal is issued and checked only by Value's mounted
// directory at construction. It is committed to the scalar seal but never
// exposed as a caller-supplied authority.
type DirectAllocationSubject struct {
	owner      *Schema
	packs      *pack.Schema
	allocation *AllocationResult
	key        heap.Key
	module     identity.ContentID
	semantic   identity.ContentID
	coordinate uint32
	id         identity.ContentID
}

func directAllocationSubjectID(module, semantic, key identity.ContentID, coordinate uint32) identity.ContentID {
	if !module.Available() || !semantic.Available() || !key.Available() || coordinate == 0 {
		return identity.ContentID{}
	}
	var payload [32*3 + 4]byte
	copy(payload[0:32], module[:])
	copy(payload[32:64], semantic[:])
	copy(payload[64:96], key[:])
	payload[96] = byte(coordinate >> 24)
	payload[97] = byte(coordinate >> 16)
	payload[98] = byte(coordinate >> 8)
	payload[99] = byte(coordinate)
	hash := sha256.New()
	_, _ = hash.Write([]byte(directAllocationSubjectDomain))
	_, _ = hash.Write(payload[:])
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// directAllocationSubjectKeyIDMatches is the private anti-splice boundary
// between Value's AllocationResult cache row and Heap's owner-issued Key.
// Both scalar IDs must be independently available and exactly equal before a
// direct receipt can be sealed or remain valid.
func directAllocationSubjectKeyIDMatches(result, issued identity.ContentID) bool {
	return result.Available() && issued.Available() && result == issued
}

// directAllocationSubjectSealMatches is the one private recomputation check
// for a stored direct-identity scalar seal. Keeping it separate makes every
// mutation path—including a changed key, coordinate, or stored ID—fail at the
// same boundary that receipt validation uses.
func directAllocationSubjectSealMatches(id, module, semantic, key identity.ContentID, coordinate uint32) bool {
	return id.Available() && id == directAllocationSubjectID(module, semantic, key, coordinate)
}

func (receipt DirectAllocationSubject) valid() bool {
	if receipt.owner == nil || receipt.packs == nil || !receipt.owner.Valid() || !receipt.owner.LinkOwner().Matches(receipt.packs.LinkOwner()) ||
		!receipt.allocation.Owns(receipt.owner) || !receipt.owner.heap.OwnsKey(receipt.key) || !receipt.key.Valid() {
		return false
	}
	if _, sourceOwned := receipt.packs.EndpointForMountedSemantic(receipt.module, receipt.semantic); !sourceOwned {
		return false
	}
	// The source itself is scalar-only by design, but resolving it through this
	// exact Value directory re-establishes the owner fence before comparison.
	source, sourceOK := receipt.owner.CoordinateForMountedSemantic(receipt.module, receipt.semantic)
	allocation, allocationOK := receipt.allocation.Coordinate()
	keyID, keyOK := receipt.allocation.KeyID()
	sourceIndex, sourceIndexOK := receipt.owner.CoordinateIndex(source)
	allocationIndex, allocationIndexOK := receipt.owner.CoordinateIndex(allocation)
	issuedKey, issuedKeyOK := receipt.allocation.Key()
	issuedKeyID, issuedKeyIDOK := receipt.key.ContentID()
	if !sourceOK || !allocationOK || !keyOK || !sourceIndexOK || !allocationIndexOK || !issuedKeyOK || !issuedKeyIDOK ||
		issuedKey != receipt.key || !directAllocationSubjectKeyIDMatches(keyID, issuedKeyID) || sourceIndex != allocationIndex || receipt.coordinate != sourceIndex+1 {
		return false
	}
	return directAllocationSubjectSealMatches(receipt.id, receipt.module, receipt.semantic, issuedKeyID, receipt.coordinate)
}

// Valid reports only the receipt's detached scalar seal. It does not turn
// direct identity into any later placement proof.
func (receipt DirectAllocationSubject) Valid() bool { return receipt.valid() }

// DirectAllocationSubjectFor issues the direct identity receipt only when the
// exact Pack semantic source and the exact owned allocation result resolve to
// the same Value coordinate. A matching ContentID alone is never admitted:
// Value's mounted directory and AllocationResult owner fence both participate.
func (schema *Schema) DirectAllocationSubjectFor(packs *pack.Schema, source pack.SemanticSource, allocation *AllocationResult) (DirectAllocationSubject, bool) {
	if schema == nil || packs == nil || !schema.Valid() || !schema.LinkOwner().Matches(packs.LinkOwner()) || !packs.OwnsSemanticSource(source) || !allocation.Owns(schema) {
		return DirectAllocationSubject{}, false
	}
	sourceCoordinate, sourceOK := schema.CoordinateForMountedSemantic(source.Module(), source.ID())
	allocationCoordinate, allocationOK := allocation.Coordinate()
	sourceIndex, sourceIndexOK := schema.CoordinateIndex(sourceCoordinate)
	allocationIndex, allocationIndexOK := schema.CoordinateIndex(allocationCoordinate)
	if !sourceOK || !allocationOK || !sourceIndexOK || !allocationIndexOK || sourceIndex != allocationIndex {
		return DirectAllocationSubject{}, false
	}
	key, keyOK := allocation.Key()
	if !keyOK {
		return DirectAllocationSubject{}, false
	}
	resultKeyID, resultKeyIDOK := allocation.KeyID()
	issuedKeyID, issuedKeyIDOK := key.ContentID()
	if !resultKeyIDOK || !issuedKeyIDOK || !directAllocationSubjectKeyIDMatches(resultKeyID, issuedKeyID) {
		return DirectAllocationSubject{}, false
	}
	receipt := DirectAllocationSubject{
		owner: schema, packs: packs, allocation: allocation, key: key,
		module: source.Module(), semantic: source.ID(), coordinate: sourceIndex + 1,
	}
	receipt.id = directAllocationSubjectID(receipt.module, receipt.semantic, issuedKeyID, receipt.coordinate)
	return receipt, receipt.valid()
}

func (receipt DirectAllocationSubject) ContentID() (identity.ContentID, bool) {
	return receipt.id, receipt.valid()
}

// MatchesSource proves that source is the exact mounted semantic identity
// admitted by Value at issuance. It accepts no raw module or semantic ID.
func (receipt DirectAllocationSubject) MatchesSource(source pack.SemanticSource) bool {
	return receipt.valid() && source.Available() && receipt.module == source.Module() && receipt.semantic == source.ID()
}

// MatchesAllocationKeyID proves that this receipt is bound to the same exact
// Heap allocation key identity carried by a mounted requirement.
func (receipt DirectAllocationSubject) MatchesAllocationKeyID(key identity.ContentID) bool {
	if !receipt.valid() || !key.Available() {
		return false
	}
	keyID, keyOK := receipt.key.ContentID()
	return keyOK && keyID == key
}

// MatchesRuntimeBinding reauthenticates the live Pack+Heap binding against
// the exact schemas and allocation key retained by this Value-issued receipt.
// It rejects a foreign/equal-content Pack or Heap before any scalar ID is
// compared by the callsite projection.
func (receipt DirectAllocationSubject) MatchesRuntimeBinding(binding pack.RuntimeAllocationContextBinding) bool {
	source, sourceOK := binding.Source()
	return receipt.valid() && sourceOK && binding.IssuedByPack(receipt.packs) && binding.MatchesAllocationKey(receipt.key) && receipt.MatchesSource(source)
}

// ClassifySummaryCell classifies one cell only when index is this direct
// subject's privately issued Value coordinate. The caller never supplies a
// coordinate authority: a mismatched index, foreign Value, or stale receipt
// fails closed. This remains a pure membership query, not a summary proof or
// uniqueness/placement claim.
func (receipt DirectAllocationSubject) ClassifySummaryCell(index int, fact Value) (AllocationMembership, bool) {
	if !receipt.valid() || index < 0 || uint64(index) >= uint64(receipt.owner.CoordinateCount()) || uint32(index+1) != receipt.coordinate {
		return AllocationMembershipInvalid, false
	}
	return receipt.allocation.ClassifyMembership(fact)
}
