package state

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// BoundaryArtifact is the opaque, root-reachable portion of an abstract world
// that may cross one function boundary. Its State spelling and reachability
// authority stay private so consumers cannot accidentally publish a partial
// lane or treat the carrier as an independently solved State.
type BoundaryArtifact struct {
	reg     *axis.Registry
	keys    *keyspace.KeySpace
	closure BoundaryClosure
	world   State
	roots   BoundaryRoots
}

type boundaryProjectContext struct {
	reg     *axis.Registry
	keys    *keyspace.KeySpace
	closure BoundaryClosure
}

type boundaryRebaseContext struct {
	reg         *axis.Registry
	fromKeys    *keyspace.KeySpace
	toKeys      *keyspace.KeySpace
	roots       boundaryPathMap
	slots       map[key.Value][]key.Value
	allocations BoundaryAllocationMap
	fromClosure BoundaryClosure
	toClosure   BoundaryClosure
}

type boundaryApplyContext struct {
	reg     *axis.Registry
	keys    *keyspace.KeySpace
	closure BoundaryClosure
}

type BoundaryExistentialNamespace = keyspace.ExistentialNamespace

// BoundaryRebaseConfig carries the complete finite substitution authority for
// one μ application.
type BoundaryRebaseConfig struct {
	Roots        BoundaryRootMap
	Allocations  BoundaryAllocationMap
	Existentials BoundaryExistentialNamespace
}

// ProjectBoundary computes and projects the complete finite boundary closure.
// Every enabled lane is dispatched through its catalog-owned policy. No
// iteration or depth budget participates in closure construction.
func ProjectBoundary(reg *axis.Registry, keys *keyspace.KeySpace, source State, roots BoundaryRoots) (BoundaryArtifact, error) {
	if reg == nil || keys == nil || !keys.Valid() {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary projection requires registry and valid keyspace")
	}
	for _, root := range roots {
		if !product.BelongsToRegistry(reg, root.Value) {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary root value belongs to a foreign registry")
		}
		if root.Slot != 0 {
			if _, ok := key.ParseSymbolValue(root.Slot); !ok {
				if _, ok := key.ParseReturnSlot(root.Slot); !ok {
					return BoundaryArtifact{}, fmt.Errorf("state: malformed boundary root slot")
				}
			}
		}
		if root.Path.Kind != keyspace.KindInvalid && keys.FormatReadOnly(root.Path) == "" {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary root belongs to a foreign keyspace")
		}
	}
	closure, err := BuildBoundaryRootClosure(reg, keys, source, roots)
	if err != nil {
		return BoundaryArtifact{}, err
	}
	out, err := projectBoundaryWorld(reg, keys, source, closure)
	if err != nil {
		return BoundaryArtifact{}, err
	}
	projectedRoots := make(BoundaryRoots, len(roots))
	copy(projectedRoots, roots)
	for i := range projectedRoots {
		projectedRoots[i].Value = product.ProjectBoundary(reg, projectedRoots[i].Value)
	}
	return BoundaryArtifact{reg: reg, keys: keys, closure: closure, world: out, roots: projectedRoots}, nil
}

// RebaseBoundary atomically substitutes structural roots and lexical
// allocation identities into a new keyspace authority. An unmapped reachable
// path or identity rejects the complete artifact; no partial carrier escapes.
func RebaseBoundary(reg *axis.Registry, artifact BoundaryArtifact, toKeys *keyspace.KeySpace, config BoundaryRebaseConfig) (BoundaryArtifact, error) {
	if reg == nil || artifact.reg != reg || artifact.keys == nil || !artifact.keys.Valid() || toKeys == nil || !toKeys.Valid() {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary rebase requires registry and valid keyspaces")
	}
	if !boundaryAllocationMapInjective(artifact.closure, config.Allocations) {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary allocation map is incomplete or non-injective")
	}
	rootPaths, rootSlots, aliases, rootCount, ok := buildBoundaryRootRelation(artifact, toKeys, config.Roots)
	if !ok {
		return BoundaryArtifact{}, fmt.Errorf("state: invalid boundary root relation")
	}
	effectiveRoots, ok := completeBoundaryRootMap(artifact.keys, toKeys, artifact.closure, rootPaths, config.Existentials)
	if !ok {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary existential root construction failed")
	}
	closure, ok := rebaseBoundaryClosure(artifact.keys, toKeys, artifact.closure, effectiveRoots, rootSlots, config.Allocations)
	if !ok {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary closure rebase failed")
	}
	ctx := boundaryRebaseContext{
		reg: reg, fromKeys: artifact.keys, toKeys: toKeys, roots: effectiveRoots, slots: rootSlots,
		allocations: config.Allocations, fromClosure: artifact.closure, toClosure: closure,
	}
	out := State{laneMask: artifact.world.laneMask}
	for _, spec := range defaultLaneCatalog.specs {
		if !artifact.world.laneMask.allows(spec.bit) {
			continue
		}
		if !spec.boundary.rebase(&ctx, artifact.world, &out) {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary rebase failed in lane %q", spec.id)
		}
	}
	if len(aliases) != 0 {
		if out.laneMask.allows(defaultLaneCatalog.mustLaneBit(LanePathEvidence)) {
			for _, alias := range aliases {
				out = out.AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: alias[0], Other: alias[1]})
			}
		}
	}
	out.canonical = true
	rebasedRoots := make(BoundaryRoots, rootCount)
	rootSet := make([]bool, rootCount)
	valueDomain := product.Domain(reg)
	for _, binding := range config.Roots {
		root := artifact.roots[binding.FromRoot]
		root.Slot, root.Path = binding.ToSlot, binding.To
		root.Value, ok = rebaseBoundaryValue(&ctx, root.Value)
		if !ok {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary root value rebase failed")
		}
		if rootSet[binding.ToRoot] {
			if rebasedRoots[binding.ToRoot].Slot != root.Slot || rebasedRoots[binding.ToRoot].Path != root.Path {
				return BoundaryArtifact{}, fmt.Errorf("state: boundary destination root schema collision")
			}
			rebasedRoots[binding.ToRoot].Value = valueDomain.Join(rebasedRoots[binding.ToRoot].Value, root.Value)
		} else {
			rebasedRoots[binding.ToRoot] = root
			rootSet[binding.ToRoot] = true
		}
	}
	return BoundaryArtifact{reg: reg, keys: toKeys, closure: closure, world: out, roots: rebasedRoots}, nil
}

func buildBoundaryRootRelation(artifact BoundaryArtifact, toKeys *keyspace.KeySpace, bindings BoundaryRootMap) (boundaryPathMap, map[key.Value][]key.Value, [][2]keyspace.Key, int, bool) {
	if len(artifact.roots) == 0 {
		return nil, nil, nil, 0, len(bindings) == 0
	}
	seenFrom := make([]bool, len(artifact.roots))
	toOrdinals := make(map[int]struct{}, len(bindings))
	paths := make(boundaryPathMap, 0, len(bindings))
	slots := make(map[key.Value][]key.Value)
	aliases := make([][2]keyspace.Key, 0)
	pathsByFrom := make(map[keyspace.Key][]keyspace.Key)
	maxTo := -1
	for _, binding := range bindings {
		if binding.FromRoot < 0 || binding.FromRoot >= len(artifact.roots) || binding.ToRoot < 0 {
			return nil, nil, nil, 0, false
		}
		source := artifact.roots[binding.FromRoot]
		if (source.Path.Kind == keyspace.KindInvalid) != (binding.To.Kind == keyspace.KindInvalid) ||
			binding.To.Kind != keyspace.KindInvalid && toKeys.FormatReadOnly(binding.To) == "" {
			return nil, nil, nil, 0, false
		}
		if (source.Slot == 0) != (binding.ToSlot == 0) {
			return nil, nil, nil, 0, false
		}
		if binding.ToSlot != 0 {
			if _, ok := key.ParseSymbolValue(binding.ToSlot); !ok {
				if _, ok := key.ParseReturnSlot(binding.ToSlot); !ok {
					return nil, nil, nil, 0, false
				}
			}
		}
		seenFrom[binding.FromRoot] = true
		toOrdinals[binding.ToRoot] = struct{}{}
		if binding.ToRoot > maxTo {
			maxTo = binding.ToRoot
		}
		if source.Path.Kind != keyspace.KindInvalid {
			paths = append(paths, boundaryPathBinding{from: source.Path, to: binding.To})
			pathsByFrom[source.Path] = append(pathsByFrom[source.Path], binding.To)
		}
		if source.Slot != 0 {
			slots[source.Slot] = append(slots[source.Slot], binding.ToSlot)
		}
	}
	for _, seen := range seenFrom {
		if !seen {
			return nil, nil, nil, 0, false
		}
	}
	if len(toOrdinals) != maxTo+1 {
		return nil, nil, nil, 0, false
	}
	for source, destinations := range pathsByFrom {
		sort.Slice(destinations, func(i, j int) bool { return toKeys.Less(destinations[i], destinations[j]) })
		for i := 1; i < len(destinations); i++ {
			if destinations[i] != destinations[0] {
				aliases = append(aliases, [2]keyspace.Key{destinations[0], destinations[i]})
			}
		}
		pathsByFrom[source] = destinations
	}
	sort.Slice(aliases, func(i, j int) bool {
		if aliases[i][0] != aliases[j][0] {
			return toKeys.Less(aliases[i][0], aliases[j][0])
		}
		return toKeys.Less(aliases[i][1], aliases[j][1])
	})
	for source, destinations := range slots {
		sort.Slice(destinations, func(i, j int) bool { return destinations[i] < destinations[j] })
		slots[source] = compactValues(destinations)
	}
	return paths, slots, aliases, maxTo + 1, true
}

func compactValues(in []key.Value) []key.Value {
	out := in[:0]
	for _, value := range in {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func boundaryAllocationMapInjective(closure BoundaryClosure, allocations BoundaryAllocationMap) bool {
	seen := make(map[identity.ID]identity.ID, len(closure.identities))
	for from := range closure.identities {
		if from == (identity.ID{}) {
			continue
		}
		to, ok := allocations[from]
		if !ok || to == (identity.ID{}) {
			return false
		}
		if prior, exists := seen[to]; exists && prior != from {
			return false
		}
		seen[to] = from
	}
	return true
}

// BoundaryRootCount returns the arity of the projected root tuple.
func (a BoundaryArtifact) BoundaryRootCount() int { return len(a.roots) }

// BoundaryRootAt returns one projected root binding without exposing the
// artifact's State representation.
func (a BoundaryArtifact) BoundaryRootAt(index int) (BoundaryRoot, bool) {
	if index < 0 || index >= len(a.roots) {
		return BoundaryRoot{}, false
	}
	return a.roots[index], true
}

// ApplyBoundary replaces facts inside the artifact's destination closure and
// preserves every fact outside it. Lane hooks stage into a private State; any
// failure returns the zero State and publishes nothing.
func ApplyBoundary(reg *axis.Registry, keys *keyspace.KeySpace, destination State, artifact BoundaryArtifact) (State, error) {
	if reg == nil || artifact.reg != reg || keys == nil || !keys.Valid() || artifact.keys != keys {
		return State{}, fmt.Errorf("state: boundary apply requires the artifact keyspace authority")
	}
	ctx := boundaryApplyContext{reg: reg, keys: keys, closure: artifact.closure}
	if destination.laneMask != artifact.world.laneMask {
		return State{}, fmt.Errorf("state: destination and boundary artifact lane inventories differ")
	}
	out := destination
	for _, spec := range defaultLaneCatalog.specs {
		if !artifact.world.laneMask.allows(spec.bit) {
			continue
		}
		if !spec.boundary.apply(&ctx, destination, artifact.world, &out) {
			return State{}, fmt.Errorf("state: boundary apply failed in lane %q", spec.id)
		}
	}
	out.canonical = true
	return out, nil
}

// BoundaryEqual reports structural equality under one keyspace authority.
// Lane equality, not a digest, is the final semantic decision.
func BoundaryEqual(reg *axis.Registry, a, b BoundaryArtifact) bool {
	if reg == nil || a.reg != reg || b.reg != reg || a.keys == nil || a.keys != b.keys {
		return false
	}
	if a.world.laneMask != b.world.laneMask || !boundaryClosureEqual(a.closure, b.closure) || len(a.roots) != len(b.roots) {
		return false
	}
	for i := range a.roots {
		if a.roots[i].Slot != b.roots[i].Slot || a.roots[i].Path != b.roots[i].Path ||
			!product.Equal(reg, a.roots[i].Value, b.roots[i].Value) {
			return false
		}
	}
	for _, spec := range defaultLaneCatalog.specs {
		if a.world.laneMask.allows(spec.bit) && !spec.boundary.equal(reg, a.world, b.world) {
			return false
		}
	}
	return true
}
func emptyBoundaryClosure() BoundaryClosure {
	return BoundaryClosure{slots: map[key.Value]struct{}{}, paths: map[keyspace.Key]struct{}{}, identities: map[identity.ID]struct{}{}, heapSuffixes: map[boundaryHeapSuffix]struct{}{}}
}
func projectBoundaryWorld(reg *axis.Registry, keys *keyspace.KeySpace, source State, closure BoundaryClosure) (State, error) {
	ctx := boundaryProjectContext{reg: reg, keys: keys, closure: closure}
	out := State{laneMask: source.laneMask}
	for _, spec := range defaultLaneCatalog.specs {
		if !source.laneMask.allows(spec.bit) {
			continue
		}
		if !spec.boundary.project(&ctx, source, &out) {
			return State{}, fmt.Errorf("state: boundary projection failed in lane %q", spec.id)
		}
	}
	out.canonical = true
	return out, nil
}
func boundaryClosureSubset(a, b BoundaryClosure) bool {
	return (!a.allIdentities || b.allIdentities) && setSubset(a.slots, b.slots) && setSubset(a.paths, b.paths) && (b.allIdentities || setSubset(a.identities, b.identities)) && setSubset(a.heapSuffixes, b.heapSuffixes)
}
func setSubset[T comparable](a, b map[T]struct{}) bool {
	for value := range a {
		if _, ok := b[value]; !ok {
			return false
		}
	}
	return true
}
func rebaseBoundaryClosure(from, to *keyspace.KeySpace, in BoundaryClosure, roots boundaryPathMap, slots map[key.Value][]key.Value, allocations BoundaryAllocationMap) (BoundaryClosure, bool) {
	out := BoundaryClosure{
		slots: make(map[key.Value]struct{}, len(in.slots)), paths: make(map[keyspace.Key]struct{}, len(in.paths)),
		identities: make(map[identity.ID]struct{}, len(in.identities)), allIdentities: in.allIdentities, heapSuffixes: make(map[boundaryHeapSuffix]struct{}, len(in.heapSuffixes)),
	}
	for slot := range in.slots {
		next, ok := slots[slot]
		if !ok || len(next) == 0 {
			return BoundaryClosure{}, false
		}
		for _, value := range next {
			out.slots[value] = struct{}{}
		}
	}
	for path := range in.paths {
		next, ok := rebaseBoundaryPaths(from, to, roots, path)
		if !ok {
			return BoundaryClosure{}, false
		}
		for _, value := range next {
			out.paths[value] = struct{}{}
		}
	}
	for id := range in.identities {
		next, ok := RebaseBoundaryIdentity(allocations, id)
		if !ok {
			return BoundaryClosure{}, false
		}
		out.identities[next] = struct{}{}
	}
	for suffix := range in.heapSuffixes {
		owner, ok := RebaseBoundaryIdentity(allocations, suffix.owner)
		if !ok {
			return BoundaryClosure{}, false
		}
		next, ok := to.ImportKey(from, suffix.suffix)
		if !ok {
			return BoundaryClosure{}, false
		}
		out.heapSuffixes[boundaryHeapSuffix{owner: owner, suffix: next}] = struct{}{}
	}
	return out, true
}

func completeBoundaryRootMap(from, to *keyspace.KeySpace, closure BoundaryClosure, explicit boundaryPathMap, namespace BoundaryExistentialNamespace) (boundaryPathMap, bool) {
	out := append(boundaryPathMap(nil), explicit...)
	paths := make([]keyspace.Key, 0, len(closure.paths))
	for path := range closure.paths {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool { return from.Less(paths[i], paths[j]) })
	for _, path := range paths {
		if _, ok := rebaseBoundaryPaths(from, to, out, path); ok {
			continue
		}
		root := path
		root.Segs = 0
		if from.FormatReadOnly(root) == "" {
			return nil, false
		}
		toRoot, ok := to.ImportExistential(from, root, namespace)
		if !ok {
			return nil, false
		}
		out = append(out, boundaryPathBinding{from: root, to: toRoot})
	}
	return out, true
}

func rebaseBoundaryStateKeys(ctx *boundaryRebaseContext, in pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
	path, ok := ctx.fromKeys.FromStateKey(pathdom.PathKey(in.String()))
	if !ok {
		return nil, false
	}
	next, ok := rebaseBoundaryPaths(ctx.fromKeys, ctx.toKeys, ctx.roots, path)
	if !ok {
		return nil, false
	}
	out := make([]pathaddr.StateKey, 0, len(next))
	for _, path := range next {
		value, valid := pathaddr.StateKeyFromPathKey(ctx.toKeys.FormatReadOnly(path))
		if !valid {
			return nil, false
		}
		out = append(out, value)
	}
	return out, true
}

func rebaseBoundaryValue(ctx *boundaryRebaseContext, value product.Value) (product.Value, bool) {
	current := product.Get(ctx.reg, value, identity.Key)
	id, exact := current.ID()
	if !exact {
		return value, true
	}
	next, ok := RebaseBoundaryIdentity(ctx.allocations, id)
	if !ok {
		return product.Value{}, false
	}
	return product.Set(ctx.reg, value, identity.Key, identity.Singleton(next)), true
}

func boundaryClosureEqual(a, b BoundaryClosure) bool {
	return a.allIdentities == b.allIdentities && sameSet(a.slots, b.slots) && sameSet(a.paths, b.paths) &&
		sameSet(a.identities, b.identities) && sameSet(a.heapSuffixes, b.heapSuffixes)
}

func sameSet[T comparable](a, b map[T]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for value := range a {
		if _, ok := b[value]; !ok {
			return false
		}
	}
	return true
}
