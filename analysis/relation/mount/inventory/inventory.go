// Package inventory owns the production certificate-to-mount inventory.
//
// The package is deliberately a small composition boundary. Owner packages
// hand it immutable denominator populations and the two cold evidence
// sources; it issues one store identity and binds those sources to one
// certificate fence. It does not read a snapshot, derive logical identities,
// or retain a physical lookup cache.
package inventory

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Population is one owner-issued denominator membership. The relation is
// intentionally derived from Denominator: the factory accepts no second
// relation label that could disagree with the certified reference.
//
// Rows must be in the owner's canonical logical order. A non-nil empty Rows
// slice is an authenticated empty population, not an unavailable source.
type Population interface {
	Available() bool
	Denominator() model.DenominatorRef
	Rows() []model.RowID
	Evidence() identity.ContentID
}

// ExpandSource is the owner-side cold source for an Expand contract. The
// inventory forwards the contract directly; expand evidence is frozen by
// witness.Specialize under the mount fence.
type ExpandSource interface {
	ResolveExpand(model.ExpandContract) ([]expand.Vector, bool)
}

// PartitionSource is the owner-side cold source for a correlated Apply
// partition. The returned map is an ephemeral ABI result; this inventory
// retains no partition map and performs no posting inference.
type PartitionSource interface {
	ResolvePartition(certificate.CorrelationPartition) (map[model.RowID]witness.DenominatorEvidence, bool)
}

// Factory binds checked certificates to one mount identity and its immutable
// owner evidence. NewFactory issues the StoreID once; each Bind uses that same
// store identity at Generation(1).
type Factory interface {
	Bind(certificate.Certificate) (witness.Inventory, bool)
}

type factory struct {
	mu          sync.Mutex
	used        bool
	mount       identity.MountID
	store       identity.StoreID
	populations []Population
	expand      ExpandSource
	partitions  PartitionSource
}

var _ Factory = (*factory)(nil)
var _ witness.Inventory = (*bound)(nil)
var _ witness.PartitionInventory = (*bound)(nil)

// NewFactory seals the owner capabilities and issues one process-local
// StoreID. It copies only the capability vector: rows and evidence remain
// under their owner Population values. Expand and partition sources remain
// direct authorities and are only called by the bound inventory when
// Specialize requests them.
func NewFactory(mount identity.MountID, populations []Population, expand ExpandSource, partitions PartitionSource) (Factory, bool) {
	if !mount.Available() {
		return nil, false
	}
	owned, ok := validatePopulations(populations)
	if !ok {
		return nil, false
	}
	store, ok := identity.IssueStore()
	if !ok {
		return nil, false
	}
	return &factory{
		mount:       mount,
		store:       store,
		populations: owned,
		expand:      expand,
		partitions:  partitions,
	}, true
}

// Bind adopts one complete checked certificate. Required denominator census
// is shared with witness.Specialize, so no certificate-facing operation can
// request an evidence population that this factory failed to validate.
func (value *factory) Bind(cert certificate.Certificate) (witness.Inventory, bool) {
	if value == nil {
		return nil, false
	}
	// A factory is a one-shot capability. Consume it before doing any
	// certificate work so concurrent callers cannot bind two stores, and a
	// failed attempt cannot be retried against a different certificate.
	value.mu.Lock()
	if value.used {
		value.mu.Unlock()
		return nil, false
	}
	value.used = true
	mount := value.mount
	store := value.store
	populations := append([]Population(nil), value.populations...)
	expandSource := value.expand
	partitionSource := value.partitions
	value.mu.Unlock()

	if !mount.Available() || !store.Available() || !cert.Available() {
		return nil, false
	}
	required, ok := witness.RequiredDenominators(cert)
	if !ok || !sameDenominators(required, populations) {
		return nil, false
	}
	fence, ok := address.NewFence(cert.SchemaID(), cert.Digest(), store, mount, identity.Generation(1))
	if !ok {
		return nil, false
	}
	// Copy only the owner capability vector. Row membership and evidence remain
	// under the owner's sealed Population authority and are redeemed directly
	// by ResolveDenominator; this package is not a second population store.
	return &bound{
		certificate: cert,
		fence:       fence,
		populations: populations,
		expand:      expandSource,
		partitions:  partitionSource,
	}, true
}

// validatePopulations validates owner capabilities once. The list is
// intentionally checked by linear scans: the production population count is
// small and no generic retained map or copied row store is part of the mount
// representation.
func validatePopulations(values []Population) ([]Population, bool) {
	result := make([]Population, len(values))
	for index, source := range values {
		if source == nil || !source.Available() {
			return nil, false
		}
		denominator := source.Denominator()
		if !denominator.Available() {
			return nil, false
		}
		for _, prior := range result[:index] {
			if prior.Denominator() == denominator {
				return nil, false
			}
		}
		result[index] = source
	}
	return result, true
}

func sameDenominators(required []model.DenominatorRef, populations []Population) bool {
	if len(required) != len(populations) {
		return false
	}
	for _, denominator := range required {
		found := false
		for _, population := range populations {
			if population.Denominator() == denominator {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

type bound struct {
	certificate certificate.Certificate
	fence       address.Fence
	populations []Population
	expand      ExpandSource
	partitions  PartitionSource
	next        uint64
}

func (value *bound) Fence() address.Fence {
	if value == nil {
		return address.Fence{}
	}
	return value.fence
}

func (value *bound) ResolveRelation(id model.RelationID) (uint64, bool) {
	if value == nil || !id.Available() {
		return 0, false
	}
	for index, relation := range value.certificate.Relations() {
		if relation.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *bound) ResolveColumn(id model.ColumnID) (uint64, bool) {
	if value == nil || !id.Available() {
		return 0, false
	}
	for index, column := range value.certificate.Columns() {
		if column.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *bound) ResolveKey(id model.KeyID) (uint64, bool) {
	if value == nil || !id.Available() {
		return 0, false
	}
	for index, key := range value.certificate.Keys() {
		if key.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *bound) ResolveScope(id model.ScopeID) (uint64, bool) {
	if value == nil || !id.Available() {
		return 0, false
	}
	for index, scope := range value.certificate.Scopes() {
		if scope.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *bound) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	if value == nil || !id.Available() {
		return 0, false
	}
	for index, expression := range value.certificate.Expressions() {
		if expression.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *bound) ResolveDependency(id model.DependencyID) (uint64, bool) {
	if value == nil || !id.Available() {
		return 0, false
	}
	for index, dependency := range value.certificate.Dependencies() {
		if dependency.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

// Resolve issues a fresh monotonic arrangement coordinate for every valid
// logical access. Equal accesses are intentionally not coalesced: arrangement
// Derive is the exact-once caller and no Access cache is retained here.
func (value *bound) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	if value == nil || !value.fence.Available() || !access.Available() || value.next == ^uint64(0) {
		return arrangement.Handle{}, false
	}
	slot := value.next + 1
	handle, ok := arrangement.NewHandle(value.fence, slot)
	if !ok {
		return arrangement.Handle{}, false
	}
	value.next = slot
	return handle, true
}

func (value *bound) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if value == nil || !ref.Available() {
		return witness.DenominatorEvidence{}, false
	}
	for _, population := range value.populations {
		if population.Denominator() != ref {
			continue
		}
		return witness.NewDenominatorEvidence(population.Rows(), population.Evidence())
	}
	return witness.DenominatorEvidence{}, false
}

// ResolveExpand delegates the complete contract to the owner source. The
// inventory does not infer vectors from certificate expressions or alter the
// owner's response.
func (value *bound) ResolveExpand(contract model.ExpandContract) ([]expand.Vector, bool) {
	if value == nil || value.expand == nil {
		return nil, false
	}
	return value.expand.ResolveExpand(contract)
}

// ResolvePartition delegates the exact checked partition to the owner
// source. The returned posting directory is consumed by witness.Specialize;
// no directory is retained by this package.
func (value *bound) ResolvePartition(partition certificate.CorrelationPartition) (map[model.RowID]witness.DenominatorEvidence, bool) {
	if value == nil || value.partitions == nil {
		return nil, false
	}
	return value.partitions.ResolvePartition(partition)
}
