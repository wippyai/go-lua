package state

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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

// BoundaryRootBinding maps one projected root ordinal to one destination root
// ordinal. The destination structural path and slot belong to the target
// keyspace/frame authority passed to RebaseBoundary.
type BoundaryRootBinding struct {
	// FromRoot is an ordinal in the artifact's ordered root tuple. Ordinals,
	// rather than path equality, preserve repeated/aliased arguments exactly.
	FromRoot int
	// ToRoot is the destination tuple ordinal. Multiple source roots may target
	// one ordinal; their values are joined deterministically.
	ToRoot int
	To     keyspace.Key
	ToSlot key.Value
}

// BoundaryRootMap is an ordered-tuple relation whose slice order is irrelevant.
// One source ordinal may clone to several destination ordinals; several source
// ordinals may coalesce into one destination ordinal.
type BoundaryRootMap []BoundaryRootBinding

type boundaryPathBinding struct {
	from keyspace.Key
	to   keyspace.Key
}

type boundaryPathMap []boundaryPathBinding

// BoundaryAllocationMap rebases lexical allocation templates atomically with
// path roots. Every nonzero identity transported by a lane must have an
// explicit entry; callers add identity mappings for intentionally stable IDs.
type BoundaryAllocationMap map[identity.ID]identity.ID

// BoundaryClosure is the least root-reachable set needed to project State
// lanes. It is state-owned so every lane can share one reachability authority
// rather than inventing its own approximation.
type BoundaryClosure struct {
	slots         map[key.Value]struct{}
	paths         map[keyspace.Key]struct{}
	identities    map[identity.ID]struct{}
	allIdentities bool
	heapSuffixes  map[boundaryHeapSuffix]struct{}
}

type boundaryHeapSuffix struct {
	owner  identity.ID
	suffix keyspace.Key
}

// BuildBoundaryRootClosure computes the least root-reachable set by folding
// every registered State lane. No iteration budget is involved: every
// expansion is monotone and adds an element from finite State maps.
func BuildBoundaryRootClosure(reg *axis.Registry, keys *keyspace.KeySpace, source State, roots BoundaryRoots) (BoundaryClosure, error) {
	return buildBoundaryRootClosure(reg, keys, source, roots, nil)
}

// buildBoundaryRootClosure accepts an explicit catalog inventory for tests. A
// nil inventory means the complete default catalog. The source State's sealed
// lane mask selects contributions; unscoped States use the whole inventory.
// An intentionally empty, non-nil inventory performs root seeding only.
func buildBoundaryRootClosure(reg *axis.Registry, keys *keyspace.KeySpace, source State, roots BoundaryRoots, specs []laneSpec) (BoundaryClosure, error) {
	if reg == nil || keys == nil || !keys.Valid() {
		return BoundaryClosure{}, fmt.Errorf("state: boundary closure requires registry and valid keyspace")
	}
	if specs == nil {
		specs = defaultLaneCatalog.specs
	}
	closure := BoundaryClosure{
		slots:        make(map[key.Value]struct{}, len(roots)),
		paths:        make(map[keyspace.Key]struct{}, len(roots)),
		identities:   make(map[identity.ID]struct{}),
		heapSuffixes: make(map[boundaryHeapSuffix]struct{}),
	}
	for _, root := range roots {
		if root.Slot != 0 {
			closure.slots[root.Slot] = struct{}{}
		}
		if root.Path.Kind != keyspace.KindInvalid {
			if keys.FormatReadOnly(root.Path) == "" {
				return BoundaryClosure{}, fmt.Errorf("state: boundary root belongs to a foreign keyspace")
			}
			closure.paths[root.Path] = struct{}{}
		}
		closure.addValueIdentity(reg, root.Value)
	}

	expansion := boundaryClosureExpansion{reg: reg, keys: keys, closure: &closure}
	for {
		expansion.changed = false
		for _, spec := range specs {
			if !source.laneMask.allows(spec.bit) {
				continue
			}
			spec.boundary.expand(&expansion, source)
		}
		if !expansion.changed {
			break
		}
	}
	return closure, nil
}

// ContainsSlot reports whether slot belongs to the boundary root tuple.
func (c BoundaryClosure) ContainsSlot(slot key.Value) bool {
	_, ok := c.slots[slot]
	return ok
}

// boundaryClosureExpansion is the monotone capability exposed to lane-owned
// expanders. It deliberately owns mutation so lane hooks cannot remove facts
// or terminate the fixed point early.
type boundaryClosureExpansion struct {
	reg     *axis.Registry
	keys    *keyspace.KeySpace
	closure *BoundaryClosure
	changed bool
}

func (e *boundaryClosureExpansion) addPath(path keyspace.Key) {
	if path.Kind == keyspace.KindInvalid || e.closure.hasPath(path) {
		return
	}
	e.closure.paths[path] = struct{}{}
	e.changed = true
}

func (e *boundaryClosureExpansion) addValue(value product.Value) {
	if id, ok := identityvalue.ExactID(e.reg, value); ok {
		if _, seen := e.closure.identities[id]; !seen {
			e.closure.identities[id] = struct{}{}
			e.changed = true
		}
		return
	}
	if identity.Equal(product.Get(e.reg, value, identity.Key), identity.Top()) && !e.closure.allIdentities {
		e.closure.allIdentities = true
		e.changed = true
	}
}

func (e *boundaryClosureExpansion) addIdentity(id identity.ID) {
	if id == (identity.ID{}) {
		return
	}
	if _, seen := e.closure.identities[id]; seen {
		return
	}
	e.closure.identities[id] = struct{}{}
	e.changed = true
}

func (e *boundaryClosureExpansion) addStateKey(raw interface{ String() string }) keyspace.Key {
	path, ok := e.keys.FromStateKey(pathdom.PathKey(raw.String()))
	if !ok {
		return keyspace.Key{}
	}
	return path
}

func (e *boundaryClosureExpansion) connect(paths ...keyspace.Key) bool {
	touches := false
	for _, path := range paths {
		touches = touches || e.closure.pathTouches(e.keys, path)
	}
	if touches {
		for _, path := range paths {
			e.addPath(path)
		}
	}
	return touches
}

func (e *boundaryClosureExpansion) addHeapSuffix(owner identity.ID, suffix keyspace.Key) {
	qualified := boundaryHeapSuffix{owner: owner, suffix: suffix}
	if _, seen := e.closure.heapSuffixes[qualified]; seen {
		return
	}
	e.closure.heapSuffixes[qualified] = struct{}{}
}

// ContainsPath reports whether path belongs to the least reachable closure.
func (c BoundaryClosure) ContainsPath(path keyspace.Key) bool {
	_, ok := c.paths[path]
	return ok
}

// ContainsIdentity reports whether id belongs to the least reachable closure.
func (c BoundaryClosure) ContainsIdentity(id identity.ID) bool {
	if c.allIdentities && id != (identity.ID{}) {
		return true
	}
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
		return
	}
	if identity.Equal(product.Get(reg, value, identity.Key), identity.Top()) {
		c.allIdentities = true
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
func rebaseBoundaryPaths(fromKeys, toKeys *keyspace.KeySpace, roots boundaryPathMap, path keyspace.Key) ([]keyspace.Key, bool) {
	if fromKeys == nil || toKeys == nil || !fromKeys.Valid() || !toKeys.Valid() {
		return nil, false
	}
	selectedDepth := -1
	var selected []boundaryPathBinding
	for _, binding := range roots {
		if binding.from.Kind == keyspace.KindInvalid && binding.to.Kind == keyspace.KindInvalid {
			continue
		}
		if fromKeys.FormatReadOnly(binding.from) == "" || toKeys.FormatReadOnly(binding.to) == "" {
			return nil, false
		}
		if !fromKeys.HasPrefix(path, binding.from) {
			continue
		}
		depth, ok := fromKeys.SegmentLen(binding.from)
		if !ok {
			return nil, false
		}
		if len(selected) != 0 && depth == selectedDepth {
			if binding.from != selected[0].from {
				return nil, false
			}
			selected = append(selected, binding)
			continue
		}
		if len(selected) == 0 || depth > selectedDepth {
			selected = []boundaryPathBinding{binding}
			selectedDepth = depth
		}
	}
	if len(selected) == 0 {
		return nil, false
	}
	importedPath, ok := toKeys.ImportKey(fromKeys, path)
	if !ok {
		return nil, false
	}
	importedFrom, ok := toKeys.ImportKey(fromKeys, selected[0].from)
	if !ok {
		return nil, false
	}
	out := make([]keyspace.Key, 0, len(selected))
	for _, binding := range selected {
		var next keyspace.Key
		if importedPath == importedFrom {
			next = binding.to
		} else {
			next, ok = toKeys.Rebase(importedPath, importedFrom, binding.to)
			if !ok {
				return nil, false
			}
		}
		out = append(out, next)
	}
	sort.Slice(out, func(i, j int) bool { return toKeys.Less(out[i], out[j]) })
	unique := out[:0]
	for _, value := range out {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	return unique, true
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
