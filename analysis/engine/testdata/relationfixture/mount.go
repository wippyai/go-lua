package testfixture

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// inventory is the sealed mount authority used by the fixture. It only
// resolves issued schema identities and owner evidence; it does not expose
// state columns or provide a compatibility reader.
type inventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
	denominators map[model.DenominatorRef]witness.DenominatorEvidence
	partitions   map[uint32]map[model.RowID]witness.DenominatorEvidence
	accesses     []arrangement.Access
}

func (value *inventory) Fence() address.Fence { return value.fence }

func (value *inventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	slot, ok := value.relations[id]
	return slot, ok
}

func (value *inventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	slot, ok := value.columns[id]
	return slot, ok
}

func (value *inventory) ResolveKey(id model.KeyID) (uint64, bool) {
	slot, ok := value.keys[id]
	return slot, ok
}

func (value *inventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	slot, ok := value.scopes[id]
	return slot, ok
}

func (value *inventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	slot, ok := value.expressions[id]
	return slot, ok
}

func (value *inventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	slot, ok := value.dependencies[id]
	return slot, ok
}

func (value *inventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range value.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(value.fence, uint64(index+1))
		}
	}
	value.accesses = append(value.accesses, access)
	return arrangement.NewHandle(value.fence, uint64(len(value.accesses)))
}

func (value *inventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	evidence, ok := value.denominators[ref]
	return evidence, ok
}

func (value *inventory) ResolvePartition(partition certificate.CorrelationPartition) (map[model.RowID]witness.DenominatorEvidence, bool) {
	if value == nil || !partition.Available() || value.partitions == nil {
		return nil, false
	}
	source, ok := value.partitions[partition.Ordinal()]
	if !ok || source == nil {
		return nil, false
	}
	copyOf := make(map[model.RowID]witness.DenominatorEvidence, len(source))
	for row, evidence := range source {
		copyOf[row] = evidence
	}
	return copyOf, true
}

func (value *inventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

func buildMasks(t TB, manager *guard.Manager) (support.Mask, support.Mask, support.Mask, support.Mask) {
	t.Helper()
	work := support.New(manager)
	if work == nil {
		t.Fatal("support work")
	}
	left, ok := work.Literal(1, true)
	if !ok {
		t.Fatal("left mask")
	}
	rightAtom, ok := work.Literal(2, true)
	if !ok {
		t.Fatal("right atom")
	}
	right, ok := work.Or(left, rightAtom)
	if !ok {
		t.Fatal("right mask")
	}
	disjoint, ok := work.Not(left)
	if !ok {
		t.Fatal("disjoint mask")
	}
	disjointRight, ok := work.Not(right)
	if !ok || !work.Seal() {
		t.Fatal("disjoint mask")
	}
	return left, right, disjoint, disjointRight
}

func mustScopeToken(t TB, mounted witness.Mounted, scope witness.Scope) binding.ScopeToken {
	t.Helper()
	token, ok := mounted.ScopeToken(scope)
	if !ok {
		t.Fatal("scope token")
	}
	return token
}
