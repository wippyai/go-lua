package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// careActivatedPresenceStorage is a read-only predicate overlay for one
// selected edge. It reuses the canonical implication evaluator while exposing
// exact truthy/falsy trigger observations; target writes still commit only to
// the underlying coordinate carrier, so transient predicate activation never
// becomes a parallel State transfer or retained axis value.
type careActivatedPresenceStorage struct {
	presenceImplicationStorage
	reg         *axis.Registry
	keys        *keyspace.KeySpace
	activations []pathPredicateActivation
}

func (a pathPredicateActivation) refine(reg *axis.Registry, value product.Value) (product.Value, bool) {
	switch a.kind {
	case pathPredicateActivationTruthiness:
		return valuerefine.PartitionTruthiness(reg, value, a.truthy)
	default:
		return value, false
	}
}

func (s *careActivatedPresenceStorage) Fork() presenceImplicationStorage {
	if s == nil || s.presenceImplicationStorage == nil {
		return nil
	}
	return &careActivatedPresenceStorage{
		presenceImplicationStorage: s.presenceImplicationStorage.Fork(), reg: s.reg,
		keys:        s.keys,
		activations: append([]pathPredicateActivation(nil), s.activations...),
	}
}

func (s *careActivatedPresenceStorage) Commit(staged presenceImplicationStorage) bool {
	next, ok := staged.(*careActivatedPresenceStorage)
	return ok && s != nil && next != nil && s.reg == next.reg && s.keys == next.keys &&
		s.presenceImplicationStorage.Commit(next.presenceImplicationStorage)
}

func (s *careActivatedPresenceStorage) ReadRoot(slot statekey.ValueDependency) (product.Value, bool) {
	if s == nil || s.presenceImplicationStorage == nil {
		return product.Value{}, false
	}
	value, ok := s.presenceImplicationStorage.ReadRoot(slot)
	if !ok {
		return value, false
	}
	for _, activation := range s.activations {
		candidate, root := rootValueDependencyForKey(s.keys, activation.path)
		if !root || candidate != slot {
			continue
		}
		if narrowed, exact := activation.refine(s.reg, value); exact {
			value = narrowed
		}
	}
	return value, true
}

func (s *careActivatedPresenceStorage) ReadPath(path keyspace.Key) (product.Value, bool) {
	if s == nil || s.presenceImplicationStorage == nil {
		return product.Value{}, false
	}
	value, ok := s.presenceImplicationStorage.ReadPath(path)
	if !ok {
		return value, false
	}
	for _, activation := range s.activations {
		if activation.path != path {
			continue
		}
		if narrowed, exact := activation.refine(s.reg, value); exact {
			value = narrowed
		}
	}
	return value, true
}

// presenceImplicationStorage is the storage-only protocol for the canonical
// consequence equation kernel. Root-vs-local routing, visibility, triggers,
// target algebra, barriers, and scheduling remain exclusively in factapply.
type presenceImplicationStorage interface {
	Fork() presenceImplicationStorage
	Commit(presenceImplicationStorage) bool
	MakeUnreachable() bool
	Reachable() bool
	SnapshotImplications() ([]pathevidence.PathPresenceImplication, bool)
	HasImplication(pathevidence.PathPresenceImplication) (bool, bool)
	AddImplication(pathevidence.PathPresenceImplication) (changed, ok bool)
	ReadRoot(statekey.ValueDependency) (product.Value, bool)
	WriteRoot(statekey.ValueDependency, product.Value) (changed, ok bool)
	ReadPath(keyspace.Key) (product.Value, bool)
	WritePath(keyspace.Key, product.Value) (changed, ok bool)
	HasProof(pathevidence.BranchProof) (bool, bool)
	EquivalentKeys(keyspace.Key) ([]keyspace.Key, bool)
	HasEquivalentKey(keyspace.Key, keyspace.Key) (bool, bool)
	InvalidateDescendants(pathdom.PathKey) (changed, ok bool)
}

type concretePresenceStorage struct {
	reg       *axis.Registry
	resolver  *visibility.Resolver
	value     state.State
	reachable bool
	valuesTop bool
}

func (s *concretePresenceStorage) Fork() presenceImplicationStorage {
	if s == nil {
		return nil
	}
	clone := *s
	return &clone
}
func (s *concretePresenceStorage) Commit(staged presenceImplicationStorage) bool {
	next, ok := staged.(*concretePresenceStorage)
	if !ok || s == nil || next == nil || s.reg != next.reg || s.resolver != next.resolver {
		return false
	}
	*s = *next
	return true
}
func (s *concretePresenceStorage) MakeUnreachable() bool {
	if s == nil || s.reg == nil {
		return false
	}
	s.value = state.Domain(s.reg).Bottom()
	s.reachable = false
	s.valuesTop = false
	return true
}

func (s *concretePresenceStorage) Reachable() bool {
	return s != nil && s.reg != nil && s.reachable
}
func (s *concretePresenceStorage) SnapshotImplications() ([]pathevidence.PathPresenceImplication, bool) {
	if s == nil || s.resolver == nil {
		return nil, false
	}
	snapshot := s.value.PathPresenceImplicationsSnapshot(s.resolver.KeySpace())
	if snapshot.Bottom {
		return nil, true
	}
	return snapshot.Implications, true
}
func (s *concretePresenceStorage) HasImplication(value pathevidence.PathPresenceImplication) (bool, bool) {
	if s == nil || s.reg == nil || s.resolver == nil {
		return false, false
	}
	return s.value.HasPathPresenceImplication(value), true
}
func (s *concretePresenceStorage) AddImplication(value pathevidence.PathPresenceImplication) (bool, bool) {
	if s == nil || s.reg == nil || s.resolver == nil {
		return false, false
	}
	next, changed, ok := s.value.AddPathPresenceImplicationChanged(s.reg, s.resolver.KeySpace(), value)
	if !ok {
		return false, false
	}
	s.value = next
	s.reachable = s.reachable || changed
	return changed, true
}
func (s *concretePresenceStorage) ReadRoot(dependency statekey.ValueDependency) (product.Value, bool) {
	slot, concrete := dependency.Concrete()
	if s == nil || s.reg == nil || !concrete {
		return product.Value{}, false
	}
	return s.value.ReadValue(s.reg, slot), true
}
func (s *concretePresenceStorage) WriteRoot(dependency statekey.ValueDependency, value product.Value) (bool, bool) {
	slot, concrete := dependency.Concrete()
	if s == nil || s.reg == nil || !concrete || !product.BelongsToRegistry(s.reg, value) {
		return false, false
	}
	if s.valuesTop {
		return false, true
	}
	current := s.value.ReadValue(s.reg, slot)
	changed := !product.Equal(s.reg, current, value)
	next := s.value.WriteValue(s.reg, slot, value)
	s.value = next
	s.reachable = s.reachable || changed
	return changed, true
}
func (s *concretePresenceStorage) ReadPath(path keyspace.Key) (product.Value, bool) {
	if s == nil || s.reg == nil || s.resolver == nil || !s.ownsPath(path) {
		return product.Value{}, false
	}
	return s.value.ReadLocalPathKey(s.reg, path), true
}
func (s *concretePresenceStorage) WritePath(path keyspace.Key, value product.Value) (bool, bool) {
	if s == nil || s.reg == nil || s.resolver == nil || !s.ownsPath(path) || !product.BelongsToRegistry(s.reg, value) {
		return false, false
	}
	current := s.value.ReadLocalPathKey(s.reg, path)
	changed := !product.Equal(s.reg, current, value)
	next := s.value.WriteLocalPathKey(s.reg, path, value)
	s.value = next
	s.reachable = s.reachable || changed
	return changed, true
}
func (s *concretePresenceStorage) ownsPath(path keyspace.Key) bool {
	if s == nil || s.resolver == nil || path.Kind == keyspace.KindInvalid {
		return false
	}
	_, ok := s.resolver.KeySpace().SegmentsView(path)
	return ok
}
func (s *concretePresenceStorage) HasProof(proof pathevidence.BranchProof) (bool, bool) {
	return s != nil && s.value.HasBranchProof(proof), s != nil
}
func (s *concretePresenceStorage) EquivalentKeys(path keyspace.Key) ([]keyspace.Key, bool) {
	if s == nil || s.resolver == nil {
		return nil, false
	}
	return s.value.EquivalentKeyspaceKeys(s.resolver.KeySpace(), path), true
}
func (s *concretePresenceStorage) HasEquivalentKey(left, right keyspace.Key) (bool, bool) {
	if s == nil || s.resolver == nil {
		return false, false
	}
	return s.value.HasEquivalentKeyspaceKey(s.resolver.KeySpace(), left, right), true
}
func (s *concretePresenceStorage) InvalidateDescendants(path pathdom.PathKey) (bool, bool) {
	if s == nil || s.reg == nil || s.resolver == nil {
		return false, false
	}
	next, changed, ok := s.value.InvalidatePathKeyDescendantsChanged(s.resolver.KeySpace(), path)
	if !ok {
		return false, false
	}
	s.value = next
	return changed, true
}

type coordinatePresenceStorage[K comparable] struct {
	value    *state.CoordinatePathEvidenceCarrier[K]
	roots    map[statekey.ValueDependency]K
	feasible bool
}

func (s *coordinatePresenceStorage[K]) Fork() presenceImplicationStorage {
	if s == nil || s.value == nil {
		return nil
	}
	return &coordinatePresenceStorage[K]{value: s.value.Clone(), roots: s.roots, feasible: s.feasible}
}
func (s *coordinatePresenceStorage[K]) Commit(staged presenceImplicationStorage) bool {
	next, ok := staged.(*coordinatePresenceStorage[K])
	if !ok || s == nil || s.value == nil || next == nil || next.value == nil || !s.value.Commit(next.value) {
		return false
	}
	s.feasible = next.feasible
	return true
}
func (s *coordinatePresenceStorage[K]) MakeUnreachable() bool {
	if s == nil || s.value == nil {
		return false
	}
	s.feasible = false
	return true
}

func (s *coordinatePresenceStorage[K]) Reachable() bool {
	return s != nil && s.value != nil && s.feasible && s.value.Reachable()
}
func (s *coordinatePresenceStorage[K]) SnapshotImplications() ([]pathevidence.PathPresenceImplication, bool) {
	if s == nil || s.value == nil {
		return nil, false
	}
	snapshot, ok := s.value.SnapshotImplications()
	if !ok || snapshot.Bottom {
		return nil, ok
	}
	return snapshot.Implications, true
}
func (s *coordinatePresenceStorage[K]) HasImplication(value pathevidence.PathPresenceImplication) (bool, bool) {
	if s == nil || s.value == nil {
		return false, false
	}
	return s.value.HasImplication(value)
}
func (s *coordinatePresenceStorage[K]) AddImplication(value pathevidence.PathPresenceImplication) (bool, bool) {
	if s == nil || s.value == nil {
		return false, false
	}
	return s.value.AddImplication(value)
}
func (s *coordinatePresenceStorage[K]) ReadRoot(dependency statekey.ValueDependency) (product.Value, bool) {
	if s == nil || s.value == nil {
		return product.Value{}, false
	}
	root, bound := s.roots[dependency]
	if !bound {
		return product.Value{}, false
	}
	return s.value.ReadValue(root)
}
func (s *coordinatePresenceStorage[K]) WriteRoot(dependency statekey.ValueDependency, value product.Value) (bool, bool) {
	if s == nil || s.value == nil {
		return false, false
	}
	root, bound := s.roots[dependency]
	if !bound {
		return false, false
	}
	return s.value.WriteValue(root, value)
}
func (s *coordinatePresenceStorage[K]) ReadPath(path keyspace.Key) (product.Value, bool) {
	if s == nil || s.value == nil {
		return product.Value{}, false
	}
	return s.value.ReadPath(path)
}
func (s *coordinatePresenceStorage[K]) WritePath(path keyspace.Key, value product.Value) (bool, bool) {
	if s == nil || s.value == nil {
		return false, false
	}
	return s.value.WritePath(path, value)
}
func (s *coordinatePresenceStorage[K]) HasProof(proof pathevidence.BranchProof) (bool, bool) {
	if s == nil || s.value == nil {
		return false, false
	}
	return s.value.HasProof(proof), true
}
func (s *coordinatePresenceStorage[K]) EquivalentKeys(path keyspace.Key) ([]keyspace.Key, bool) {
	if s == nil || s.value == nil {
		return nil, false
	}
	return s.value.EquivalentKeys(path)
}
func (s *coordinatePresenceStorage[K]) HasEquivalentKey(left, right keyspace.Key) (bool, bool) {
	if s == nil || s.value == nil {
		return false, false
	}
	return s.value.HasEquivalentKey(left, right)
}
func (s *coordinatePresenceStorage[K]) InvalidateDescendants(path pathdom.PathKey) (bool, bool) {
	if s == nil || s.value == nil {
		return false, false
	}
	return s.value.InvalidateDescendants(path)
}
