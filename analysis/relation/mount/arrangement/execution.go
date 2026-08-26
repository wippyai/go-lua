package arrangement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/derivation"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Execution is the mount-owned physical redemption of checked logical
// expressions. It is born only in Derive: runtime receives compiler-issued
// expression identities and exact immutable layouts, never a logical Access
// to resolve or an expression to classify again.
type Execution struct{ data *executionData }

type executionData struct {
	fence   address.Fence
	entries []executionEntry
	byID    map[model.ExpressionID]int
	// byNode is the immutable physical-node directory redeemed by derivation
	// frames. It keeps Later zipper ascent O(1), so a frame never walks an
	// expression root looking for a matching binding.
	byNode map[identity.ContentID]*executionNode
	// byLogical closes derivation's logical expression digest to the same
	// sealed physical node without walking an execution tree. Path/frame
	// witnesses are issued from logical expression digests; runtime must have
	// an exact directory for that boundary as well as the physical directory.
	byLogical     map[identity.ContentID]*executionNode
	dependencies  DependencySchedule
	logicalDigest identity.ContentID
	digest        identity.ContentID
	sealed        bool
}

type executionEntry struct {
	id         model.ExpressionID
	digest     identity.ContentID
	root       *executionNode
	derivation derivation.Plan
}

// Available is deliberately O(1). Derive performs the full DAG/layout proof
// once before setting sealed; runtime must redeem an immutable proof rather
// than walk and revalidate the physical plan on every operator call.
func (execution Execution) Available() bool {
	return execution.data != nil && execution.data.sealed && execution.data.fence.Available() && execution.data.logicalDigest.Available() && execution.data.digest.Available() && execution.data.entries != nil && execution.data.byID != nil && execution.data.byNode != nil && execution.data.byLogical != nil && execution.data.dependencies.Available()
}

func (execution Execution) Fence() address.Fence {
	if !execution.Available() {
		return address.Fence{}
	}
	return execution.data.fence
}

func (execution Execution) LogicalDigest() identity.ContentID {
	if !execution.Available() {
		return identity.ContentID{}
	}
	return execution.data.logicalDigest
}

func (execution Execution) Digest() identity.ContentID {
	if !execution.Available() {
		return identity.ContentID{}
	}
	return execution.data.digest
}

// ExpressionIDs returns compiler-issued root identities in canonical order.
func (execution Execution) ExpressionIDs() []model.ExpressionID {
	if !execution.Available() {
		return nil
	}
	result := make([]model.ExpressionID, len(execution.data.entries))
	for index, entry := range execution.data.entries {
		result[index] = entry.id
	}
	return result
}

// Entry is an O(1) immutable lookup. A structural digest is deliberately not
// accepted as an evaluator selector: only the compiler-issued logical ID is
// stable authority for a top-level plan root.
func (execution Execution) Entry(id model.ExpressionID) (Node, bool) {
	if !execution.Available() || !id.Available() {
		return Node{}, false
	}
	index, ok := execution.data.byID[id]
	if !ok || index < 0 || index >= len(execution.data.entries) {
		return Node{}, false
	}
	entry := execution.data.entries[index]
	if entry.id != id || entry.root == nil || !entry.digest.Available() {
		return Node{}, false
	}
	return Node{value: entry.root}, true
}

// Node redeems one exact sealed physical node digest. The digest is an
// execution identity, not a request to resolve another layout or reopen an
// algebra expression.
func (execution Execution) Node(digest identity.ContentID) (Node, bool) {
	if !execution.Available() || !digest.Available() {
		return Node{}, false
	}
	value, ok := execution.data.byNode[digest]
	if !ok || value == nil || value.digest != digest {
		return Node{}, false
	}
	return Node{value: value}, true
}

// LogicalNode redeems the exact logical expression digest sealed by
// derivation. It is a separate directory from Node because expression and
// physical node digests have different domains; accepting one as the other
// would make path redemption silently fail or alias a node.
func (execution Execution) LogicalNode(digest identity.ContentID) (Node, bool) {
	if !execution.Available() || !digest.Available() {
		return Node{}, false
	}
	value, ok := execution.data.byLogical[digest]
	if !ok || value == nil || value.logical != digest {
		return Node{}, false
	}
	return Node{value: value}, true
}

// Derivation returns the immutable occurrence-path derivative sealed for one
// compiler-issued expression root. Runtime may redeem paths by occurrence
// without reopening an algebra expression, an Access inventory, or a schema
// catalogue.
func (execution Execution) Derivation(id model.ExpressionID) (derivation.Plan, bool) {
	if !execution.Available() || !id.Available() {
		return derivation.Plan{}, false
	}
	index, ok := execution.data.byID[id]
	if !ok || index < 0 || index >= len(execution.data.entries) {
		return derivation.Plan{}, false
	}
	value := execution.data.entries[index].derivation
	if !value.Available() || value.Root() != id {
		return derivation.Plan{}, false
	}
	return value, true
}

// DependencySchedule returns the immutable dependency/SCC schedule sealed
// together with the expression bindings. Runtime consumers use its opaque
// owner-issued lookups; they never reconstruct a dependency graph.
func (execution Execution) DependencySchedule() DependencySchedule {
	if !execution.Available() {
		return DependencySchedule{}
	}
	return execution.data.dependencies
}

// Dependency resolves one compiler-issued work entry in O(1).
func (execution Execution) Dependency(id model.DependencyID) (ScheduleEntry, bool) {
	if !execution.Available() {
		return ScheduleEntry{}, false
	}
	return execution.data.dependencies.Dependency(id)
}

// Schedules returns all sealed work entries in canonical solve order.
func (execution Execution) Schedules() []ScheduleEntry {
	if !execution.Available() {
		return nil
	}
	return execution.data.dependencies.Schedules()
}

// WakeRelation returns only entries with a sealed relation-wide read.
func (execution Execution) WakeRelation(relation model.RelationID) []ScheduleEntry {
	if !execution.Available() {
		return nil
	}
	return execution.data.dependencies.WakeRelation(relation)
}

// WakeColumn returns only entries with a sealed exact column read.
func (execution Execution) WakeColumn(column model.ColumnID) []ScheduleEntry {
	if !execution.Available() {
		return nil
	}
	return execution.data.dependencies.WakeColumn(column)
}
