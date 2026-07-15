package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// BoundaryRoot is one caller value/path pair bound to a callee root. Slot is
// optional because transformer roots may be expression values with no State
// value cell. Path is the structural authority used to retain and substitute
// path-keyed facts.
type BoundaryRoot struct {
	Slot  key.Value
	Path  keyspace.Key
	Value product.Value
}

// BoundaryRoots is the complete ordered root tuple of one call frame.
type BoundaryRoots []BoundaryRoot

// BoundaryRootBinding maps one projected root to its caller/callee
// counterpart. From and To belong to the corresponding keyspaces passed to
// RebaseBoundaryPath.
type BoundaryRootBinding struct {
	From keyspace.Key
	To   keyspace.Key
}

// BoundaryRootMap is a structural, prefix-preserving substitution. Bindings
// must be unambiguous; aliases are represented by multiple From roots mapping
// to the same To root.
type BoundaryRootMap []BoundaryRootBinding

// BoundaryAllocationMap rebases lexical allocation templates atomically with
// path roots. Every nonzero identity transported by a lane must have an
// explicit entry; callers add identity mappings for intentionally stable IDs.
type BoundaryAllocationMap map[identity.ID]identity.ID

// BoundaryClosure is the least root-reachable set needed to project State
// lanes. It is state-owned so every lane can share one reachability authority
// rather than inventing its own approximation.
type BoundaryClosure struct {
	paths        map[keyspace.Key]struct{}
	identities   map[identity.ID]struct{}
	heapSuffixes map[boundaryHeapSuffix]struct{}
}

type boundaryHeapSuffix struct {
	owner  identity.ID
	suffix keyspace.Key
}

// BuildBoundaryRootClosure computes the finite root seed closure through path
// equality/implication edges and heap object product identities. Lane boundary
// projectors must extend this seed with their own path/resource operands before
// projection; this function is intentionally not an all-lane projection.
// No iteration budget is involved: every step adds an element from finite
// State maps.
func BuildBoundaryRootClosure(reg *axis.Registry, keys *keyspace.KeySpace, source State, roots BoundaryRoots) (BoundaryClosure, error) {
	if reg == nil || keys == nil || !keys.Valid() {
		return BoundaryClosure{}, fmt.Errorf("state: boundary closure requires registry and valid keyspace")
	}
	closure := BoundaryClosure{
		paths:        make(map[keyspace.Key]struct{}, len(roots)),
		identities:   make(map[identity.ID]struct{}),
		heapSuffixes: make(map[boundaryHeapSuffix]struct{}),
	}
	for _, root := range roots {
		if root.Path.Kind != keyspace.KindInvalid {
			if keys.FormatReadOnly(root.Path) == "" {
				return BoundaryClosure{}, fmt.Errorf("state: boundary root belongs to a foreign keyspace")
			}
			closure.paths[root.Path] = struct{}{}
		}
		closure.addValueIdentity(reg, root.Value)
	}

	changed := true
	for changed {
		changed = false
		addPath := func(path keyspace.Key) {
			if path.Kind == keyspace.KindInvalid || closure.hasPath(path) {
				return
			}
			closure.paths[path] = struct{}{}
			changed = true
		}
		addValue := func(value product.Value) {
			if id, ok := identityvalue.ExactID(reg, value); ok {
				if _, seen := closure.identities[id]; !seen {
					closure.identities[id] = struct{}{}
					changed = true
				}
			}
		}

		source.ForEachPathRefinement(func(path keyspace.Key, value product.Value) bool {
			if closure.pathTouches(keys, path) {
				addPath(path)
				addValue(value)
			}
			return true
		})
		source.ForEachPathStaticMember(func(path keyspace.Key, value product.Value) bool {
			if closure.pathTouches(keys, path) {
				addPath(path)
				addValue(value)
			}
			return true
		})
		source.ForEachBranchProof(func(proof pathevidence.BranchProof) bool {
			if closure.pathTouches(keys, proof.Path) || closure.pathTouches(keys, proof.Other) {
				addPath(proof.Path)
				addPath(proof.Other)
			}
			return true
		})
		source.pathEvidence.ForEachPathPresenceImplication(func(implication pathevidence.PathPresenceImplication) bool {
			if closure.pathTouches(keys, implication.Trigger) || closure.pathTouches(keys, implication.TriggerOther) ||
				closure.pathTouches(keys, implication.Target) {
				addPath(implication.Trigger)
				addPath(implication.TriggerOther)
				addPath(implication.Target)
				if implication.HasTriggerValue {
					addValue(implication.TriggerValue)
				}
				if implication.HasTargetValue {
					addValue(implication.TargetValue)
				}
			}
			return true
		})
		if !source.heapTableIdentity.top {
			for id := range closure.identities {
				object, ok := source.heapTableIdentity.values[id]
				if !ok {
					continue
				}
				addValue(object.Root())
				for path, value := range object.StaticMembers() {
					closure.heapSuffixes[boundaryHeapSuffix{owner: id, suffix: path}] = struct{}{}
					addValue(value)
				}
				for factKey, fact := range object.DynamicIndexFacts() {
					closure.heapSuffixes[boundaryHeapSuffix{owner: id, suffix: factKey.Table}] = struct{}{}
					addValue(fact.KeyValue)
					addValue(fact.Value)
				}
			}
		}
	}
	return closure, nil
}

// ContainsPath reports whether path belongs to the least reachable closure.
func (c BoundaryClosure) ContainsPath(path keyspace.Key) bool {
	_, ok := c.paths[path]
	return ok
}

// ContainsIdentity reports whether id belongs to the least reachable closure.
func (c BoundaryClosure) ContainsIdentity(id identity.ID) bool {
	_, ok := c.identities[id]
	return ok
}

// ContainsHeapSuffix reports whether a rootless static-member suffix is
// reachable through owner. Rootless heap suffixes deliberately never enter the
// absolute path namespace.
func (c BoundaryClosure) ContainsHeapSuffix(owner identity.ID, suffix keyspace.Key) bool {
	_, ok := c.heapSuffixes[boundaryHeapSuffix{owner: owner, suffix: suffix}]
	return ok
}

func (c *BoundaryClosure) addValueIdentity(reg *axis.Registry, value product.Value) {
	if id, ok := identityvalue.ExactID(reg, value); ok {
		c.identities[id] = struct{}{}
	}
}

func (c BoundaryClosure) hasPath(path keyspace.Key) bool {
	_, ok := c.paths[path]
	return ok
}

func (c BoundaryClosure) pathTouches(keys *keyspace.KeySpace, path keyspace.Key) bool {
	if path.Kind == keyspace.KindInvalid {
		return false
	}
	if c.hasPath(path) {
		return true
	}
	for root := range c.paths {
		if keys.HasPrefix(path, root) {
			return true
		}
	}
	return false
}

// RebaseBoundaryPath substitutes the longest matching structural root and
// preserves the exact suffix. No spelling/hash comparison participates.
func RebaseBoundaryPath(fromKeys, toKeys *keyspace.KeySpace, roots BoundaryRootMap, path keyspace.Key) (keyspace.Key, bool) {
	if fromKeys == nil || toKeys == nil || !fromKeys.Valid() || !toKeys.Valid() {
		return keyspace.Key{}, false
	}
	var selected BoundaryRootBinding
	selectedDepth := -1
	found := false
	for _, binding := range roots {
		if fromKeys.FormatReadOnly(binding.From) == "" || toKeys.FormatReadOnly(binding.To) == "" {
			return keyspace.Key{}, false
		}
		if !fromKeys.HasPrefix(path, binding.From) {
			continue
		}
		depth, ok := fromKeys.SegmentLen(binding.From)
		if !ok {
			return keyspace.Key{}, false
		}
		if found && depth == selectedDepth {
			if binding.From != selected.From || binding.To != selected.To {
				return keyspace.Key{}, false
			}
			continue
		}
		if !found || depth > selectedDepth {
			selected = binding
			selectedDepth = depth
			found = true
		}
	}
	if !found {
		return keyspace.Key{}, false
	}
	importedPath, ok := toKeys.ImportKey(fromKeys, path)
	if !ok {
		return keyspace.Key{}, false
	}
	importedFrom, ok := toKeys.ImportKey(fromKeys, selected.From)
	if !ok {
		return keyspace.Key{}, false
	}
	if importedPath == importedFrom {
		return selected.To, true
	}
	rebased, ok := toKeys.Rebase(importedPath, importedFrom, selected.To)
	return rebased, ok
}

// RebaseBoundaryIdentity performs exact allocation substitution. It returns
// false for an unmapped identity so callers cannot partially rebase a lane.
func RebaseBoundaryIdentity(allocations BoundaryAllocationMap, id identity.ID) (identity.ID, bool) {
	if id == (identity.ID{}) {
		return identity.ID{}, true
	}
	next, ok := allocations[id]
	return next, ok && next != (identity.ID{})
}
