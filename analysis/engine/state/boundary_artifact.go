package state

import (
	"fmt"
	"sort"
	"sync"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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
	shape   boundaryArtifactShape
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
	allocations *BoundaryAllocationAuthority
	identities  *IdentitySubstitutionAuthority
	// identityImaged records that coordinate owner terms were already mapped
	// through the Apply topology image; boundary transport must preserve those
	// caller-owned formal terms while still rebasing paths and allocations.
	identityImaged bool
	quotient       boundaryInverseQuotient
	fromClosure    BoundaryClosure
	toClosure      BoundaryClosure
	// formalRekey is the sole formal root-image law. Ordinary structural
	// lanes use the same resolver-version quotient as coordinate families and
	// scalar transaction children; roots is retained for the exact inverse.
	formalRekey *CoordinateFormalRootRekey
	// structuralIdentity selects the full-State allocation quotient: paths,
	// slots, and state keys remain in the same owner keyspace while only exact
	// allocation identities are alpha-renamed.
	structuralIdentity bool
	// relationBottom is exact elimination by a Bottom formal image, not an
	// execution error. The owning relational transaction consumes it as its
	// unreachable element.
	relationBottom bool
}

type boundaryApplyContext struct {
	reg     *axis.Registry
	keys    *keyspace.KeySpace
	closure BoundaryClosure
}

type BoundaryExistentialNamespace = keyspace.ExistentialNamespace

// validBoundaryRootSlot is the single structural-boundary slot vocabulary.
// Expression cells are evaluator scratch and never structural roots. Symbols,
// final N5 tuple slots, and point-owned call-result carriers are addressable
// boundary coordinates.
func validBoundaryRootSlot(slot key.Value) bool {
	if slot == 0 {
		return true
	}
	if _, ok := key.ParseSymbolValue(slot); ok {
		return true
	}
	if _, ok := key.ParseReturnSlot(slot); ok {
		return true
	}
	_, _, ok := key.ParseCallResult(slot)
	return ok
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
		if !validBoundaryRootSlot(root.Slot) {
			return BoundaryArtifact{}, fmt.Errorf("state: malformed boundary root slot")
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
	artifact := BoundaryArtifact{reg: reg, keys: keys, closure: closure, world: out, roots: projectedRoots}
	artifact.shape = artifact.structuralShape()
	return artifact, nil
}

// BoundaryTransport is one opaque, immutable boundary transaction. Root
// relation, destination keyspace, existential namespace, and allocation route
// authority are sealed together before any artifact can be rebased.
type BoundaryTransport struct {
	authority    *BoundaryAllocationAuthority
	toKeys       *keyspace.KeySpace
	roots        BoundaryRootMap
	existentials BoundaryExistentialNamespace
	planMu       sync.Mutex
	plans        map[boundaryArtifactShape][]*boundaryTransportPlan
}

func (authority *BoundaryAllocationAuthority) BindTransport(toKeys *keyspace.KeySpace, roots BoundaryRootMap, existentials BoundaryExistentialNamespace) (*BoundaryTransport, error) {
	if authority == nil || toKeys == nil || !toKeys.Valid() {
		return nil, fmt.Errorf("state: boundary transport requires allocation and keyspace authority")
	}
	ownedRoots := canonicalBoundaryRootMap(roots)
	identity := boundaryTransportIdentityOf(toKeys, ownedRoots, existentials)
	authority.transportMu.Lock()
	defer authority.transportMu.Unlock()
	if authority.transportBuckets == nil {
		authority.transportBuckets = make(map[boundaryTransportIdentity][]*BoundaryTransport)
	}
	for _, prior := range authority.transportBuckets[identity] {
		if prior.toKeys == toKeys && prior.existentials == existentials && boundaryRootMapEqual(prior.roots, ownedRoots) {
			return prior, nil
		}
	}
	transport := &BoundaryTransport{authority: authority, toKeys: toKeys, roots: ownedRoots, existentials: existentials, plans: make(map[boundaryArtifactShape][]*boundaryTransportPlan)}
	authority.transportBuckets[identity] = append(authority.transportBuckets[identity], transport)
	return transport, nil
}

// Rebase atomically substitutes structural roots and lexical
// allocation identities into a new keyspace authority. An unmapped reachable
// path or identity rejects the complete artifact; no partial carrier escapes.
func (transport *BoundaryTransport) Rebase(reg *axis.Registry, artifact BoundaryArtifact) (BoundaryArtifact, error) {
	if transport == nil {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary rebase requires a sealed transport")
	}
	authority, toKeys, roots := transport.authority, transport.toKeys, transport.roots
	if reg == nil || artifact.reg != reg || artifact.keys == nil || !artifact.keys.Valid() || toKeys == nil || !toKeys.Valid() {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary rebase requires registry and valid keyspaces")
	}
	if authority == nil {
		return BoundaryArtifact{}, fmt.Errorf("state: boundary rebase requires frame allocation authority")
	}
	if !boundaryAllocationAuthorityCovers(artifact.closure, authority) {
		return BoundaryArtifact{}, fmt.Errorf("state: allocation authority omits a reachable template")
	}
	// A stabilized target boundary is commonly projected and admitted again at
	// the same structural addresses. Once allocation templates have already
	// been quotiented, that transport is the mathematical identity. Preserve
	// the immutable artifact directly instead of rebuilding its complete
	// closure and all seventeen lane fragments on every recursive equation.
	if boundaryTransportIsIdentity(artifact, toKeys, roots) {
		return artifact, nil
	}
	plan, err := transport.planFor(artifact)
	if err != nil {
		return BoundaryArtifact{}, err
	}
	return plan.apply(reg, artifact)
}

func boundaryTransportIsIdentity(artifact BoundaryArtifact, toKeys *keyspace.KeySpace, roots BoundaryRootMap) bool {
	if artifact.keys != toKeys || len(roots) != len(artifact.roots) {
		return false
	}
	for source := range artifact.closure.identities {
		if _, allocation := source.Allocation(); allocation {
			return false
		}
	}
	seen := make([]bool, len(roots))
	for _, binding := range roots {
		if binding.FromRoot < 0 || binding.FromRoot >= len(artifact.roots) || binding.ToRoot != binding.FromRoot || seen[binding.FromRoot] {
			return false
		}
		root := artifact.roots[binding.FromRoot]
		if binding.To != root.Path || binding.ToSlot != root.Slot {
			return false
		}
		seen[binding.FromRoot] = true
	}
	for _, present := range seen {
		if !present {
			return false
		}
	}
	return true
}

func buildBoundaryRootRelation(artifact BoundaryArtifact, toKeys *keyspace.KeySpace, bindings BoundaryRootMap) (boundaryPathMap, map[key.Value][]key.Value, [][2]keyspace.Key, int, bool) {
	if len(artifact.roots) == 0 {
		return nil, nil, nil, 0, len(bindings) == 0
	}
	seenFrom := make([]bool, len(artifact.roots))
	toOrdinals := make(map[int]struct{}, len(bindings))
	scalarOwners := make(map[int]int, len(bindings))
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
		if binding.To.Kind != keyspace.KindInvalid && toKeys.FormatReadOnly(binding.To) == "" {
			return nil, nil, nil, 0, false
		}
		if !validBoundaryRootSlot(binding.ToSlot) {
			return nil, nil, nil, 0, false
		}
		seenFrom[binding.FromRoot] = true
		toOrdinals[binding.ToRoot] = struct{}{}
		scalarOwners[binding.ToRoot]++
		if binding.ToRoot > maxTo {
			maxTo = binding.ToRoot
		}
		// A value-only actual has no caller structural address.  Do not install
		// an explicit source->Invalid path edge: completeBoundaryRootMap must give
		// any reachable source structure its frame-owned existential image.
		if source.Path.Kind != keyspace.KindInvalid && binding.To.Kind != keyspace.KindInvalid {
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
	for ordinal := 0; ordinal <= maxTo; ordinal++ {
		if scalarOwners[ordinal] < 1 {
			return nil, nil, nil, 0, false
		}
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

func boundaryAllocationAuthorityCovers(closure BoundaryClosure, lens *BoundaryAllocationAuthority) bool {
	for source := range closure.identities {
		if template, allocation := source.Allocation(); allocation {
			if lens == nil {
				return false
			}
			if _, ok := lens.bindings[template]; !ok {
				return false
			}
		}
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
	if destination.laneMask != artifact.world.laneMask {
		return State{}, fmt.Errorf("state: destination and boundary artifact lane inventories differ")
	}
	domain := RegisteredProductDomain(reg)
	if domain.mask != artifact.world.laneMask {
		ids := make([]LaneID, 0, len(defaultLaneCatalog.specs))
		for _, spec := range defaultLaneCatalog.specs {
			if artifact.world.laneMask.allows(spec.bit) {
				ids = append(ids, spec.id)
			}
		}
		var err error
		domain, err = defaultLaneCatalog.TryProductDomainWithLaneSet(reg, NewLaneSet(ids...))
		if err != nil {
			return State{}, err
		}
	}
	patch, err := domain.SealBoundaryPatch(keys, artifact)
	if err != nil {
		return State{}, err
	}
	return patch.Apply(destination)
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
	return BoundaryClosure{slots: map[key.Value]struct{}{}, paths: map[keyspace.Key]struct{}{}, identities: map[identity.Term]struct{}{}, heapSuffixes: map[boundaryHeapSuffix]struct{}{}}
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
func rebaseBoundaryClosure(from, to *keyspace.KeySpace, in BoundaryClosure, roots boundaryPathMap, slots map[key.Value][]key.Value, lens *BoundaryAllocationAuthority) (BoundaryClosure, error) {
	out := BoundaryClosure{
		slots: make(map[key.Value]struct{}, len(in.slots)), paths: make(map[keyspace.Key]struct{}, len(in.paths)),
		identities: make(map[identity.Term]struct{}, len(in.identities)), allIdentities: in.allIdentities, heapSuffixes: make(map[boundaryHeapSuffix]struct{}, len(in.heapSuffixes)),
	}
	for slot := range in.slots {
		next, ok := slots[slot]
		if !ok || len(next) == 0 {
			return BoundaryClosure{}, fmt.Errorf("slot %d has no destination", slot)
		}
		for _, value := range next {
			out.slots[value] = struct{}{}
		}
	}
	for path := range in.paths {
		next, ok := rebaseBoundaryPaths(from, to, roots, path)
		if !ok {
			return BoundaryClosure{}, fmt.Errorf("path %q has no destination", from.FormatReadOnly(path))
		}
		for _, value := range next {
			out.paths[value] = struct{}{}
		}
	}
	for id := range in.identities {
		next, ok := rebaseBoundaryIdentity(lens, id)
		if !ok {
			return BoundaryClosure{}, fmt.Errorf("identity has no allocation substitution")
		}
		out.identities[next] = struct{}{}
	}
	for suffix := range in.heapSuffixes {
		owner, ok := rebaseBoundaryIdentity(lens, suffix.owner)
		if !ok {
			return BoundaryClosure{}, fmt.Errorf("heap suffix owner has no allocation substitution")
		}
		next, ok := to.ImportKey(from, suffix.suffix)
		if !ok {
			return BoundaryClosure{}, fmt.Errorf("heap suffix belongs to another keyspace")
		}
		out.heapSuffixes[boundaryHeapSuffix{owner: owner, suffix: next}] = struct{}{}
	}
	return out, nil
}

func completeBoundaryRootMap(from, to *keyspace.KeySpace, closure BoundaryClosure, explicit boundaryPathMap, namespace BoundaryExistentialNamespace) (boundaryPathMap, error) {
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
		var err error
		out, err = completeVersionInsensitiveSymbolRoot(from, to, path, out, explicit)
		if err != nil {
			return nil, fmt.Errorf("version-insensitive companion for source path %q (kind %d): %w", from.FormatReadOnly(path), path.Kind, err)
		}
		if _, mapped := rebaseBoundaryPaths(from, to, out, path); mapped {
			continue
		}
		root, ok := from.StructuralRoot(path)
		if !ok {
			return nil, fmt.Errorf("source path %q (kind %d) has no structural root", from.FormatReadOnly(path), path.Kind)
		}
		toRoot, ok := to.ImportExistential(from, root, namespace)
		if !ok {
			return nil, fmt.Errorf("source root %q (kind %d) cannot enter existential namespace %+v", from.FormatReadOnly(root), root.Kind, namespace)
		}
		out = append(out, boundaryPathBinding{from: root, to: toRoot})
	}
	return out, nil
}

// completeVersionInsensitiveSymbolRoot derives the structural half of an
// exact resolver-root binding when a projected lane explicitly demanded the
// bare symN root. This is a relation-local bridge, not a global version
// wildcard: only exact resolver roots certified by the frame participate, and
// no descendant or sibling SSA version is included.
func completeVersionInsensitiveSymbolRoot(from, to *keyspace.KeySpace, path keyspace.Key, current, explicit boundaryPathMap) (boundaryPathMap, error) {
	if path.Kind != keyspace.KindUnversionedSym || path.Segs != 0 {
		return current, nil
	}
	targets := make([]keyspace.Key, 0, 1)
	for _, binding := range explicit {
		if binding.from.Kind != keyspace.KindResolverSym || binding.from.Sym != path.Sym || binding.from.Segs != 0 {
			continue
		}
		if (binding.to.Kind != keyspace.KindResolverSym && binding.to.Kind != keyspace.KindUnversionedSym) || binding.to.Segs != 0 || binding.to.Sym == 0 {
			// A captured/result source may legitimately map to ret[n] or another
			// non-symbol root. It is not the two-spelling symbol relation handled
			// here and retains the ordinary explicit substitution unchanged.
			continue
		}
		targets = append(targets, to.FromPath(pathdom.Path{Symbol: binding.to.Sym}))
	}
	if len(targets) == 0 {
		return current, nil
	}
	sort.Slice(targets, func(i, j int) bool { return to.Less(targets[i], targets[j]) })
	for index, target := range targets {
		if index != 0 && target == targets[index-1] {
			continue
		}
		duplicate := false
		for _, binding := range current {
			if binding.from == path && binding.to == target {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		current = append(current, boundaryPathBinding{from: path, to: target})
	}
	return current, nil
}

func rebaseBoundaryStateKeys(ctx *boundaryRebaseContext, in pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
	if ctx != nil && ctx.structuralIdentity {
		if in == "" {
			return []pathaddr.StateKey{""}, true
		}
		if _, ok := ctx.fromKeys.FromStateKey(pathdom.PathKey(in.String())); !ok || ctx.fromKeys != ctx.toKeys {
			return nil, false
		}
		return []pathaddr.StateKey{in}, true
	}
	path, ok := ctx.fromKeys.FromStateKey(pathdom.PathKey(in.String()))
	if !ok {
		return nil, false
	}
	next, ok := boundaryRebasePaths(ctx, path)
	if !ok {
		return nil, false
	}
	out := make([]pathaddr.StateKey, 0, len(next))
	for _, path := range next {
		value, valid := pathaddr.StateKeyFromPathKey(ctx.toKeys.FormatReadOnly(path))
		if !valid {
			return nil, false
		}
		if ctx.formalRekey != nil {
			preimages := ctx.quotient.stateKeys[value]
			seen := false
			for _, prior := range preimages {
				seen = seen || prior == in
			}
			if !seen {
				ctx.quotient.stateKeys[value] = append(preimages, in)
			}
		}
		out = append(out, value)
	}
	return out, true
}

func rebaseBoundaryValue(ctx *boundaryRebaseContext, value product.Value) (product.Value, bool) {
	current := product.Get(ctx.reg, value, identity.Key)
	term, exact := current.Term()
	if !exact {
		return value, true
	}
	next, ok := identityImage(ctx, term)
	if !ok {
		return product.Value{}, false
	}
	if next.IsBottom() {
		ctx.relationBottom = true
		return product.Bottom(ctx.reg), true
	}
	return product.Set(ctx.reg, value, identity.Key, next), true
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
