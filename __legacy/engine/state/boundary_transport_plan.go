package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// boundaryArtifactShape identifies only structural transport input. Dynamic
// lane payloads and root values deliberately do not participate.
type boundaryArtifactShape struct {
	keys        *keyspace.KeySpace
	closureHash uint64
	rootHash    uint64
	closureLen  int
	rootLen     int
}

type boundaryRootSchema struct {
	slot key.Value
	path keyspace.Key
}

type boundaryTransportIdentity struct {
	toKeys      *keyspace.KeySpace
	existential BoundaryExistentialNamespace
	rootsHash   uint64
	rootsLen    int
}

// boundaryTransportPlan is an immutable proof for one exact source closure and
// root schema. It contains no State result or decision reference.
type boundaryTransportPlan struct {
	transport      *BoundaryTransport
	shape          boundaryArtifactShape
	sourceClosure  BoundaryClosure
	sourceRoots    []boundaryRootSchema
	rootPaths      boundaryPathMap
	rootSlots      map[key.Value][]key.Value
	aliases        [][2]keyspace.Key
	rootCount      int
	effectiveRoots boundaryPathMap
	baseClosure    BoundaryClosure
	quotient       boundaryInverseQuotient
}

func boundaryTransportIdentityOf(toKeys *keyspace.KeySpace, roots BoundaryRootMap, existential BoundaryExistentialNamespace) boundaryTransportIdentity {
	return boundaryTransportIdentity{toKeys: toKeys, existential: existential, rootsHash: hashBoundaryRootMap(roots), rootsLen: len(roots)}
}

// canonicalBoundaryRootMap takes ownership of the relation's spelling once.
// BoundaryRootMap is a mathematical relation, so its input slice order cannot
// create distinct transports or change the order in which quotient owners are
// joined.
func canonicalBoundaryRootMap(roots BoundaryRootMap) BoundaryRootMap {
	out := append(BoundaryRootMap(nil), roots...)
	sort.Slice(out, func(i, j int) bool { return boundaryRootBindingLess(out[i], out[j]) })
	return out
}

func boundaryRootBindingLess(left, right BoundaryRootBinding) bool {
	if left.FromRoot != right.FromRoot {
		return left.FromRoot < right.FromRoot
	}
	if left.ToRoot != right.ToRoot {
		return left.ToRoot < right.ToRoot
	}
	if left.ToSlot != right.ToSlot {
		return left.ToSlot < right.ToSlot
	}
	if left.To.Kind != right.To.Kind {
		return left.To.Kind < right.To.Kind
	}
	if left.To.Sym != right.To.Sym {
		return left.To.Sym < right.To.Sym
	}
	if left.To.Ver != right.To.Ver {
		return left.To.Ver < right.To.Ver
	}
	if left.To.Root != right.To.Root {
		return left.To.Root < right.To.Root
	}
	if left.To.Segs != right.To.Segs {
		return left.To.Segs < right.To.Segs
	}
	return !left.To.Canon && right.To.Canon
}

func boundaryRootMapEqual(left, right BoundaryRootMap) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (artifact BoundaryArtifact) structuralShape() boundaryArtifactShape {
	if artifact.shape.keys == artifact.keys && artifact.shape.rootLen == len(artifact.roots) && artifact.shape.closureLen == boundaryClosureWidth(artifact.closure) {
		return artifact.shape
	}
	return boundaryArtifactShape{
		keys: artifact.keys, closureHash: hashBoundaryClosure(artifact.closure), rootHash: hashBoundaryRootSchema(artifact.roots),
		closureLen: boundaryClosureWidth(artifact.closure), rootLen: len(artifact.roots),
	}
}

func (transport *BoundaryTransport) planFor(artifact BoundaryArtifact) (*boundaryTransportPlan, error) {
	shape := artifact.structuralShape()
	transport.planMu.Lock()
	defer transport.planMu.Unlock()
	if transport.plans == nil {
		transport.plans = make(map[boundaryArtifactShape][]*boundaryTransportPlan)
	}
	for _, prior := range transport.plans[shape] {
		if prior.matches(artifact) {
			return prior, nil
		}
	}
	plan, err := compileBoundaryTransportPlan(transport, artifact, shape)
	if err != nil {
		return nil, err
	}
	transport.plans[shape] = append(transport.plans[shape], plan)
	return plan, nil
}

func compileBoundaryTransportPlan(transport *BoundaryTransport, artifact BoundaryArtifact, shape boundaryArtifactShape) (*boundaryTransportPlan, error) {
	rootPaths, rootSlots, aliases, rootCount, ok := buildBoundaryRootRelation(artifact, transport.toKeys, transport.roots)
	if !ok {
		return nil, fmt.Errorf("state: invalid boundary root relation")
	}
	effectiveRoots, err := completeBoundaryRootMap(artifact.keys, transport.toKeys, artifact.closure, rootPaths, transport.existentials)
	if err != nil {
		return nil, fmt.Errorf("state: boundary existential root construction failed: %w", err)
	}
	closure, err := rebaseBoundaryClosure(artifact.keys, transport.toKeys, artifact.closure, effectiveRoots, rootSlots, transport.authority)
	if err != nil {
		return nil, fmt.Errorf("state: boundary closure rebase failed: %w", err)
	}
	for _, binding := range transport.roots {
		if binding.ToSlot != 0 {
			closure.slots[binding.ToSlot] = struct{}{}
		}
		if binding.To.Kind != keyspace.KindInvalid {
			closure.paths[binding.To] = struct{}{}
		}
	}
	quotient, ok := buildBoundaryInverseQuotient(artifact.keys, transport.toKeys, artifact.closure, effectiveRoots, rootSlots, transport.authority)
	if !ok {
		return nil, fmt.Errorf("state: boundary quotient construction failed")
	}
	return &boundaryTransportPlan{
		transport: transport, shape: shape, sourceClosure: cloneBoundaryClosure(artifact.closure), sourceRoots: boundaryRootSchemas(artifact.roots),
		rootPaths: append(boundaryPathMap(nil), rootPaths...), rootSlots: cloneBoundarySlotMap(rootSlots), aliases: append([][2]keyspace.Key(nil), aliases...), rootCount: rootCount,
		effectiveRoots: append(boundaryPathMap(nil), effectiveRoots...), baseClosure: cloneBoundaryClosure(closure), quotient: quotient,
	}, nil
}

func (plan *boundaryTransportPlan) matches(artifact BoundaryArtifact) bool {
	return plan != nil && plan.shape == artifact.structuralShape() && boundaryClosureEqual(plan.sourceClosure, artifact.closure) && boundaryRootSchemaEqual(plan.sourceRoots, artifact.roots)
}

func (plan *boundaryTransportPlan) apply(reg *axis.Registry, artifact BoundaryArtifact) (BoundaryArtifact, error) {
	// planFor is the sole caller-side authority: it already matched the exact
	// closure and root schema (or compiled this plan from them). Repeating that
	// complete set comparison here made every cache hit scan the boundary twice.
	if plan == nil || plan.transport == nil || reg == nil || artifact.reg != reg {
		return BoundaryArtifact{}, fmt.Errorf("state: invalid boundary transport plan application")
	}
	transport := plan.transport
	closure := withBoundaryEffectStructuralCompanions(transport.toKeys, artifact.world, plan.rootPaths, plan.baseClosure)
	ctx := boundaryRebaseContext{
		reg: reg, fromKeys: artifact.keys, toKeys: transport.toKeys, roots: plan.effectiveRoots, slots: plan.rootSlots,
		allocations: transport.authority, quotient: plan.quotient, fromClosure: artifact.closure, toClosure: closure,
	}
	out := State{laneMask: artifact.world.laneMask}
	for _, spec := range defaultLaneCatalog.specs {
		if !artifact.world.laneMask.allows(spec.bit) {
			continue
		}
		if !spec.boundary.rebase(&ctx, artifact.world, &out) {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary rebase failed in lane %q", spec.id)
		}
		if !spec.boundary.postRebase(&ctx, plan.aliases, &out) {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary post-rebase failed in lane %q", spec.id)
		}
	}
	out.canonical = true
	rebasedRoots := make(BoundaryRoots, plan.rootCount)
	rootSet := make([]bool, plan.rootCount)
	valueDomain := product.Domain(reg)
	for _, binding := range transport.roots {
		root := artifact.roots[binding.FromRoot]
		root.Slot, root.Path = binding.ToSlot, binding.To
		var ok bool
		root.Value, ok = rebaseBoundaryValue(&ctx, root.Value)
		if !ok {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary root value rebase failed")
		}
		if rootSet[binding.ToRoot] {
			if rebasedRoots[binding.ToRoot].Slot != root.Slot || rebasedRoots[binding.ToRoot].Path != root.Path {
				return BoundaryArtifact{}, fmt.Errorf("state: boundary destination root schema collision")
			}
			rebasedRoots[binding.ToRoot].Value = valueDomain.Join(rebasedRoots[binding.ToRoot].Value, root.Value)
			continue
		}
		rebasedRoots[binding.ToRoot] = root
		rootSet[binding.ToRoot] = true
	}
	for _, set := range rootSet {
		if !set {
			return BoundaryArtifact{}, fmt.Errorf("state: boundary destination has no scalar owner")
		}
	}
	result := BoundaryArtifact{reg: reg, keys: transport.toKeys, closure: closure, world: out, roots: rebasedRoots}
	result.shape = result.structuralShape()
	return result, nil
}

func withBoundaryEffectStructuralCompanions(to *keyspace.KeySpace, source State, explicit boundaryPathMap, base BoundaryClosure) BoundaryClosure {
	if source.effectDeltas.top {
		return base
	}
	return withBoundaryEffectStructuralTargets(to, explicit, base, func(visit func(keyspace.Key)) {
		for effectKey := range source.effectDeltas.values {
			visit(effectKey.Target)
		}
	})
}

// withBoundaryEffectStructuralTargets is the single concrete/factor law for
// mutation closure. Effects are stated on version-insensitive roots while a
// call edge commonly binds a resolver version; the affected destination is the
// unversioned structural root of that resolver symbol. Keeping this conversion
// here prevents the whole-State and factor transports from invalidating two
// different path fibers.
func withBoundaryEffectStructuralTargets(
	to *keyspace.KeySpace,
	explicit boundaryPathMap,
	base BoundaryClosure,
	visitTargets func(func(keyspace.Key)),
) BoundaryClosure {
	if to == nil || !to.Valid() || visitTargets == nil {
		return base
	}
	additions := make(map[keyspace.Key]struct{})
	visitTargets(func(target keyspace.Key) {
		if target.Kind != keyspace.KindUnversionedSym || target.Sym == 0 || target.Segs != 0 {
			return
		}
		for _, binding := range explicit {
			if binding.from != target || (binding.to.Kind != keyspace.KindResolverSym && binding.to.Kind != keyspace.KindUnversionedSym) || binding.to.Sym == 0 || binding.to.Segs != 0 {
				continue
			}
			path := to.FromPath(pathdom.Path{Symbol: binding.to.Sym})
			if _, exists := base.paths[path]; !exists {
				additions[path] = struct{}{}
			}
		}
	})
	if len(additions) == 0 {
		return base
	}
	out := base
	out.paths = cloneSet(base.paths)
	for path := range additions {
		out.paths[path] = struct{}{}
	}
	return out
}

// The helpers below keep fingerprints deterministic; equality remains the
// authority, so collisions only share a bucket.
func hashMix(hash, value uint64) uint64 { return (hash ^ value) * 1099511628211 }

func hashBoundaryKey(hash uint64, value keyspace.Key) uint64 {
	hash = hashMix(hash, uint64(value.Sym))
	hash = hashMix(hash, uint64(value.Ver))
	hash = hashMix(hash, uint64(value.Root))
	hash = hashMix(hash, uint64(value.Segs))
	hash = hashMix(hash, uint64(value.Kind))
	if value.Canon {
		hash = hashMix(hash, 1)
	}
	return hash
}

func hashBoundaryRootMap(roots BoundaryRootMap) uint64 {
	h := uint64(1469598103934665603)
	for _, root := range roots {
		h = hashMix(h, uint64(root.FromRoot))
		h = hashMix(h, uint64(root.ToRoot))
		h = hashBoundaryKey(h, root.To)
		h = hashMix(h, uint64(root.ToSlot))
	}
	return h
}

func hashBoundaryRootSchema(roots BoundaryRoots) uint64 {
	h := uint64(1469598103934665603)
	for _, root := range roots {
		h = hashMix(h, uint64(root.Slot))
		h = hashBoundaryKey(h, root.Path)
	}
	return h
}

func boundaryClosureWidth(value BoundaryClosure) int {
	return len(value.slots) + len(value.paths) + len(value.identities) + len(value.heapSuffixes)
}

type boundarySetHash struct {
	xor    uint64
	sum    uint64
	square uint64
	count  uint64
}

func (h *boundarySetHash) add(value uint64) {
	h.xor ^= value
	h.sum += value
	h.square += value * value
	h.count++
}

func (h boundarySetHash) appendTo(seed uint64) uint64 {
	seed = hashMix(seed, h.xor)
	seed = hashMix(seed, h.sum)
	seed = hashMix(seed, h.square)
	return hashMix(seed, h.count)
}

func hashBoundaryClosure(value BoundaryClosure) uint64 {
	h := uint64(1469598103934665603)
	var slots boundarySetHash
	for slot := range value.slots {
		slots.add(hashMix(1469598103934665603, uint64(slot)))
	}
	h = slots.appendTo(h)

	var paths boundarySetHash
	for path := range value.paths {
		paths.add(hashBoundaryKey(1469598103934665603, path))
	}
	h = paths.appendTo(h)

	var identities boundarySetHash
	for term := range value.identities {
		identities.add(term.Hash())
	}
	h = identities.appendTo(h)

	var suffixes boundarySetHash
	for suffix := range value.heapSuffixes {
		ownerHash := suffix.owner.Hash()
		suffixes.add(hashBoundaryKey(ownerHash, suffix.suffix))
	}
	h = suffixes.appendTo(h)
	if value.allIdentities {
		h = hashMix(h, 1)
	}
	return h
}

func boundaryRootSchemas(roots BoundaryRoots) []boundaryRootSchema {
	out := make([]boundaryRootSchema, len(roots))
	for index, root := range roots {
		out[index] = boundaryRootSchema{slot: root.Slot, path: root.Path}
	}
	return out
}

func boundaryRootSchemaEqual(schema []boundaryRootSchema, roots BoundaryRoots) bool {
	if len(schema) != len(roots) {
		return false
	}
	for index := range schema {
		if schema[index].slot != roots[index].Slot || schema[index].path != roots[index].Path {
			return false
		}
	}
	return true
}

func cloneSet[T comparable](input map[T]struct{}) map[T]struct{} {
	out := make(map[T]struct{}, len(input))
	for value := range input {
		out[value] = struct{}{}
	}
	return out
}

func cloneBoundaryClosure(in BoundaryClosure) BoundaryClosure {
	return BoundaryClosure{
		slots: cloneSet(in.slots), paths: cloneSet(in.paths), identities: cloneSet(in.identities),
		allIdentities: in.allIdentities, heapSuffixes: cloneSet(in.heapSuffixes),
	}
}

func cloneBoundarySlotMap(in map[key.Value][]key.Value) map[key.Value][]key.Value {
	out := make(map[key.Value][]key.Value, len(in))
	for slot, values := range in {
		out[slot] = append([]key.Value(nil), values...)
	}
	return out
}
