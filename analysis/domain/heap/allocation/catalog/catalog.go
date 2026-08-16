// Package catalog owns the Link-local substitution from reusable Program
// allocation occurrence IDs to exact mounted Heap allocation receipts.
package catalog

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Catalog is the immutable allocation-occurrence denominator for one exact
// Heap/Value Link seal. Duplicate mounts have distinct Mount issuers even
// when their reusable occurrence IDs are identical.
type Catalog struct {
	heap         heapdomain.Schema
	values       *valuedomain.Schema
	summaryOwner *valueowner.HotOwner
	mounts       map[identity.ContentID]*mountRows
	mountOrder   []identity.ContentID
	summaryState summaryState
}

type mountRows struct {
	owner      *Catalog
	module     identity.ContentID
	occurrence heapdomain.OccurrenceMount
	closed     []source.Closed
	closedSet  []bool
}

type summaryState uint8

const (
	summaryPending summaryState = iota + 1
	summarySealed
	summaryFailed
)

// SealFailure is the closed cold-binding stage that rejected an allocation
// catalog. It deliberately exposes no Program proof, Heap key, coordinate,
// mount handle, or row identity.
type SealFailure uint8

const (
	SealFailureNone SealFailure = iota
	SealFailureInput
	SealFailureMount
	SealFailureDuplicateMount
	SealFailureAllocation
	SealFailureKey
	SealFailureRoot
	SealFailureClosed
	SealFailureCoordinate
	SealFailureSummary
	SealFailureSummaryAttachment
	SealFailureDuplicateAllocation
	SealFailureAllocationCoverage
	SealFailureMountCoverage
)

func (failure SealFailure) String() string {
	switch failure {
	case SealFailureInput:
		return "input"
	case SealFailureMount:
		return "mount"
	case SealFailureDuplicateMount:
		return "duplicate-mount"
	case SealFailureAllocation:
		return "allocation"
	case SealFailureKey:
		return "key"
	case SealFailureRoot:
		return "root"
	case SealFailureClosed:
		return "closed"
	case SealFailureCoordinate:
		return "coordinate"
	case SealFailureSummary:
		return "summary"
	case SealFailureSummaryAttachment:
		return "summary-attachment"
	case SealFailureDuplicateAllocation:
		return "duplicate-allocation"
	case SealFailureAllocationCoverage:
		return "allocation-coverage"
	case SealFailureMountCoverage:
		return "mount-coverage"
	default:
		return "none"
	}
}

// Mount is an opaque exact mounted substitution issuer.
type Mount struct{ rows *mountRows }

// FencedTo authenticates this immutable catalog against the exact Link-local
// Heap and Value owners used at seal.
func (catalog *Catalog) FencedTo(heap heapdomain.Schema, values *valuedomain.Schema) bool {
	return catalog != nil && catalog.heap == heap && catalog.values == values && catalog.summaryOwner != nil && catalog.summaryOwner.Schema() == values && catalog.summaryState != summaryFailed && heap.Valid() && values != nil && values.Valid() && values.OwnsHeapSchema(heap) && len(catalog.mounts) == values.MountCount()
}

// FencedToSummaryOwner additionally authenticates the exact Value hot owner
// that issued every closed-constructor summary receipt at seal.
func (catalog *Catalog) FencedToSummaryOwner(heap heapdomain.Schema, values *valuedomain.Schema, owner *valueowner.HotOwner) bool {
	return catalog != nil && owner != nil && catalog.summaryOwner == owner && catalog.FencedTo(heap, values)
}

func (catalog *Catalog) FencedToHeap(heap heapdomain.Schema) bool {
	return catalog != nil && catalog.heap == heap && heap.Valid() && catalog.values != nil && catalog.values.Valid() && len(catalog.mounts) == catalog.values.MountCount()
}

// Seal joins each exact mounted Program Allocation proof to Heap's already
// issued key once. No raw term, ordinal, or engine authority is retained.
func Seal(heap heapdomain.Schema, values *valuedomain.Schema, summaryOwner *valueowner.HotOwner, mounts []heapdomain.ArtifactMount) (*Catalog, bool) {
	catalog, failure := SealWithFailure(heap, values, summaryOwner, mounts)
	return catalog, failure == SealFailureNone
}

// SealWithFailure is the single-call form for callers whose Value binding is
// already sealed. Production binding uses BeginWithFailure, binds every Rule,
// seals the shared SchemaBinding, then calls SealSummaryReceiptsWithFailure.
func SealWithFailure(heap heapdomain.Schema, values *valuedomain.Schema, summaryOwner *valueowner.HotOwner, mounts []heapdomain.ArtifactMount) (*Catalog, SealFailure) {
	result, failure := BeginWithFailure(heap, values, summaryOwner, mounts)
	if failure != SealFailureNone {
		return nil, failure
	}
	if failure = result.SealSummaryReceiptsWithFailure(); failure != SealFailureNone {
		return nil, failure
	}
	return result, SealFailureNone
}

// BeginWithFailure seals the mount owner fence while the shared
// SchemaBinding remains open. Heap already owns the allocation occurrence
// inverse; this catalog retains no key/artifact-occurrence reconstruction.
func BeginWithFailure(heap heapdomain.Schema, values *valuedomain.Schema, summaryOwner *valueowner.HotOwner, mounts []heapdomain.ArtifactMount) (*Catalog, SealFailure) {
	if !heap.Valid() || values == nil || !values.Valid() || summaryOwner == nil || summaryOwner.Schema() != values || !values.OwnsHeapSchema(heap) || !values.LinkOwner().Matches(heap.LinkOwner()) || len(mounts) != values.MountCount() {
		return nil, SealFailureInput
	}
	result := &Catalog{heap: heap, values: values, summaryOwner: summaryOwner, mounts: make(map[identity.ContentID]*mountRows), summaryState: summaryPending}
	for _, mounted := range mounts {
		if !mounted.Available() {
			return nil, SealFailureMount
		}
		module := mounted.Module()
		if _, duplicate := result.mounts[module]; duplicate {
			return nil, SealFailureDuplicateMount
		}
		canonical, canonicalOK := heap.ArtifactMountForModule(module)
		if !canonicalOK || canonical.ProgramID() != mounted.ProgramID() || canonical.Artifact() != mounted.Artifact() {
			return nil, SealFailureMount
		}
		occurrence, occurrenceOK := heap.OccurrenceMountForModule(module)
		if !occurrenceOK || occurrence.ProgramID() != mounted.ProgramID() {
			return nil, SealFailureMount
		}
		// Heap has already admitted every Program allocation and sealed the
		// occurrence inverse. Keep only the mount-scoped issuer here; no key or
		// artifact-occurrence directory is reconstructed by this catalog.
		allocationCount := occurrence.AllocationCount()
		if allocationCount < 0 {
			return nil, SealFailureAllocation
		}
		rows := &mountRows{owner: result, module: module, occurrence: occurrence, closed: make([]source.Closed, allocationCount), closedSet: make([]bool, allocationCount)}
		result.mounts[module] = rows
		result.mountOrder = append(result.mountOrder, module)
	}
	if len(result.mounts) != len(mounts) {
		return nil, SealFailureMountCoverage
	}
	return result, SealFailureNone
}

// SealSummaryReceiptsWithFailure completes the catalog exactly once after
// SchemaBinding publication. Closed source summaries are preissued into the
// binding-specific source vector; Heap remains the sole occurrence identity
// authority and this catalog stores no occurrence-keyed directory.
func (catalog *Catalog) SealSummaryReceiptsWithFailure() SealFailure {
	if catalog == nil || catalog.summaryState != summaryPending || !catalog.FencedTo(catalog.heap, catalog.values) {
		return SealFailureInput
	}
	for _, module := range catalog.mountOrder {
		rows := catalog.mounts[module]
		if rows == nil || rows.owner != catalog || rows.module != module || !rows.occurrence.Module().Available() {
			catalog.summaryState = summaryFailed
			return SealFailureMount
		}
		for index := 0; index < rows.occurrence.AllocationCount(); index++ {
			id, key, allocationOK := rows.occurrence.AllocationAt(index)
			if !allocationOK || !id.Available() {
				catalog.summaryState = summaryFailed
				return SealFailureAllocation
			}
			root, rootOK := source.New(catalog.heap, key)
			if !rootOK || !root.FencedTo(catalog.heap) {
				catalog.summaryState = summaryFailed
				return SealFailureRoot
			}
			if root.Form() != source.FormClosed {
				continue
			}
			closed, closedOK := source.NewClosed(catalog.heap, catalog.values, key)
			if !closedOK || !closed.FencedTo(catalog.heap, catalog.values) {
				catalog.summaryState = summaryFailed
				return SealFailureClosed
			}
			coordinates := make([]valuedomain.Coordinate, closed.CoordinateCount())
			for coordinateIndex := range coordinates {
				coordinate, coordinateOK := closed.CoordinateAt(coordinateIndex)
				if !coordinateOK {
					catalog.summaryState = summaryFailed
					return SealFailureCoordinate
				}
				coordinates[coordinateIndex] = coordinate
			}
			summary, summaryOK := catalog.summaryOwner.IssueSummaryReceipt(coordinates)
			if !summaryOK || !summary.IssuedBy(catalog.summaryOwner) || summary.Width() != len(coordinates) {
				catalog.summaryState = summaryFailed
				return SealFailureSummary
			}
			closed, closedOK = closed.WithSummaryReceipt(summary)
			if !closedOK || !closed.SummaryReceipt().IssuedBy(catalog.summaryOwner) {
				catalog.summaryState = summaryFailed
				return SealFailureSummaryAttachment
			}
			rows.closed[index] = closed
			rows.closedSet[index] = true
		}
	}
	// This state transition seals the catalog's owner fence; it does not build
	// an occurrence-keyed directory.
	catalog.summaryState = summarySealed
	return SealFailureNone
}

func (catalog *Catalog) ForMount(module identity.ContentID) (Mount, bool) {
	if catalog == nil || catalog.summaryState != summarySealed || !module.Available() {
		return Mount{}, false
	}
	rows := catalog.mounts[module]
	return Mount{rows: rows}, rows != nil && rows.owner == catalog && rows.module == module
}

func (mount Mount) ownedBy(catalog *Catalog) bool {
	return catalog != nil && mount.rows != nil && mount.rows.owner == catalog && mount.rows.module.Available() && catalog.mounts[mount.rows.module] == mount.rows
}

// ModuleID returns the exact mount substitution identity.
func (mount Mount) ModuleID() identity.ContentID {
	if mount.rows == nil {
		return identity.ContentID{}
	}
	return mount.rows.module
}

func (mount Mount) KeyForOccurrence(id identity.ContentID) (heapdomain.Key, bool) {
	if mount.rows == nil || !id.Available() || !mount.ownedBy(mount.rows.owner) {
		return heapdomain.Key{}, false
	}
	return mount.rows.occurrence.AllocationRootForOccurrence(id)
}

func (mount Mount) RootForOccurrence(id identity.ContentID) (source.Root, bool) {
	if mount.rows == nil || !id.Available() || !mount.ownedBy(mount.rows.owner) {
		return source.Root{}, false
	}
	key, ok := mount.KeyForOccurrence(id)
	if !ok {
		return source.Root{}, false
	}
	root, rootOK := source.New(mount.rows.owner.heap, key)
	return root, rootOK && root.FencedTo(mount.rows.owner.heap)
}

func (mount Mount) ClosedForOccurrence(id identity.ContentID) (source.Closed, bool) {
	if mount.rows == nil || !id.Available() || !mount.ownedBy(mount.rows.owner) {
		return source.Closed{}, false
	}
	ordinal, ordinalOK := mount.rows.occurrence.AllocationOrdinal(id)
	if !ordinalOK || ordinal < 0 || ordinal >= len(mount.rows.closed) || !mount.rows.closedSet[ordinal] {
		return source.Closed{}, false
	}
	closed := mount.rows.closed[ordinal]
	return closed, closed.FencedTo(mount.rows.owner.heap, mount.rows.owner.values) && closed.SummaryReceipt().IssuedBy(mount.rows.owner.summaryOwner)
}

// OwnedBy is the exact catalog fence used by package-owned hot rules.
func (mount Mount) OwnedBy(catalog *Catalog) bool { return mount.ownedBy(catalog) }
