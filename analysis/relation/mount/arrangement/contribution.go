package arrangement

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// ContributionCell is the mounted descriptor for one exact contribution
// output cell.  The operation identity and owner-issued column are both part
// of the key; neither an output ordinal nor a column shape can redeem it.
// The descriptor is issued only by Derive and remains tied to that mount's
// address fence.
type ContributionCell struct {
	fence     address.Fence
	operation signature.Identity
	column    model.ColumnID
	spec      output.ContributionSpec
	sealed    bool
}

func newContributionCell(fence address.Fence, spec output.ContributionSpec) (ContributionCell, bool) {
	if !fence.Available() || !spec.Available() || !spec.Port().Available() {
		return ContributionCell{}, false
	}
	port := spec.Port()
	value := ContributionCell{
		fence:     fence,
		operation: port.Operation,
		column:    port.Column,
		spec:      spec,
		sealed:    true,
	}
	return value, value.Available()
}

// Available reports whether the complete immutable output-cell descriptor
// was sealed.  This is deliberately constant-time; all structural checks
// happen before the descriptor enters the mounted directory.
func (cell ContributionCell) Available() bool {
	return cell.sealed && cell.fence.Available() && cell.operation.Available() && cell.column.Available() && cell.spec.Available() && cell.spec.Port().Operation == cell.operation && cell.spec.Column() == cell.column
}

// Fence returns the exact address fence captured by mount.
func (cell ContributionCell) Fence() address.Fence {
	if !cell.Available() {
		return address.Fence{}
	}
	return cell.fence
}

// Operation returns the exact signed operation identity declared by the
// contribution. Version is retained; a sibling version cannot alias it.
func (cell ContributionCell) Operation() signature.Identity {
	if !cell.Available() {
		return signature.Identity{}
	}
	return cell.operation
}

// Column returns the exact owner-issued output column identity.
func (cell ContributionCell) Column() model.ColumnID {
	if !cell.Available() {
		return model.ColumnID{}
	}
	return cell.column
}

// Spec returns the exact schema-authored contribution declaration carried by
// this descriptor.
func (cell ContributionCell) Spec() output.ContributionSpec {
	if !cell.Available() {
		return output.ContributionSpec{}
	}
	return cell.spec
}

// ValidFor redeems the descriptor against one exact mounted address fence.
func (cell ContributionCell) ValidFor(fence address.Fence) bool {
	return cell.Available() && cell.fence.Same(fence)
}

// contributionCellKey is the complete O(1) classifier key. Both fields are
// nominal owner-issued identities, so equal content from a foreign owner or
// a sibling operation cannot collide.
type contributionCellKey struct {
	operation signature.Identity
	column    model.ColumnID
}

type contributionDirectory struct {
	fence   address.Fence
	entries []ContributionCell
	byKey   map[contributionCellKey]ContributionCell
	sealed  bool
}

func newContributionDirectory(fence address.Fence, specs []output.ContributionSpec) (contributionDirectory, bool) {
	if !fence.Available() || specs == nil {
		// A nil declaration vector is a valid empty contribution catalogue, but
		// normalize it to a non-nil sealed vector so directory availability does
		// not depend on whether the certificate happened to allocate the slice.
		specs = []output.ContributionSpec{}
	}
	entries := make([]ContributionCell, 0, len(specs))
	byKey := make(map[contributionCellKey]ContributionCell, len(specs))
	for _, spec := range specs {
		cell, ok := newContributionCell(fence, spec)
		if !ok {
			return contributionDirectory{}, false
		}
		key := contributionCellKey{operation: cell.operation, column: cell.column}
		if _, duplicate := byKey[key]; duplicate {
			return contributionDirectory{}, false
		}
		byKey[key] = cell
		entries = append(entries, cell)
	}
	sort.SliceStable(entries, func(left, right int) bool {
		return contributionCellLess(entries[left], entries[right])
	})
	result := contributionDirectory{fence: fence, entries: entries, byKey: byKey, sealed: true}
	return result, result.Available()
}

func (directory contributionDirectory) Available() bool {
	return directory.sealed && directory.fence.Available() && directory.entries != nil && directory.byKey != nil
}

func (directory contributionDirectory) ValidFor(fence address.Fence) bool {
	return directory.Available() && directory.fence.Same(fence)
}

func (directory contributionDirectory) Lookup(operation signature.Identity, cell binding.CellToken) (ContributionCell, bool) {
	if !directory.Available() || !operation.Available() || !cell.Available() || !cellMatchesFence(cell, directory.fence) {
		return ContributionCell{}, false
	}
	value, ok := directory.byKey[contributionCellKey{operation: operation, column: cell.Column()}]
	if !ok || !value.ValidFor(directory.fence) || value.operation != operation || value.column != cell.Column() {
		return ContributionCell{}, false
	}
	return value, true
}

// LookupPort is the direct directory lookup retained for mounted state
// consumers that already carry the sealed structural port. It does not scan
// the descriptor vector; runtime cell classification should use Lookup so a
// CellToken's mounted fence is redeemed at the same boundary.
func (directory contributionDirectory) LookupPort(port output.OutputPort) (ContributionCell, bool) {
	if !directory.Available() || !port.Available() {
		return ContributionCell{}, false
	}
	value, ok := directory.byKey[contributionCellKey{operation: port.Operation, column: port.Column}]
	if !ok || !value.ValidFor(directory.fence) || value.operation != port.Operation || value.column != port.Column {
		return ContributionCell{}, false
	}
	return value, true
}

func contributionCellLess(left, right ContributionCell) bool {
	leftOperation, rightOperation := left.operation, right.operation
	if compared := compareOperation(leftOperation, rightOperation); compared != 0 {
		return compared < 0
	}
	return compareColumn(left.column, right.column) < 0
}

// CellToken carries a semantic binding fence while arrangement carries the
// richer address fence (including StoreID). CellToken intentionally cannot
// expose StoreID; these three exact runtime coordinates are therefore the
// complete cross-layer fence relation available at this boundary.
func cellMatchesFence(cell binding.CellToken, fence address.Fence) bool {
	if !cell.Available() || !fence.Available() {
		return false
	}
	runtime := cell.Fence()
	return runtime.Available() && runtime.Schema() == fence.SchemaID() && runtime.Mount() == fence.MountID() && runtime.Generation() == fence.Generation()
}
