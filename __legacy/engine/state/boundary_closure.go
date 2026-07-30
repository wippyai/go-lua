package state

import (
	"fmt"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
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
	// To and ToSlot are the destination address at which ApplyBoundary
	// materializes the rebased root value. A source rvalue may have neither a
	// path nor a slot while its destination formal has both; in that case no
	// structural source lanes are invented, but the exact value still acquires
	// its callee-owned address.
	To     keyspace.Key
	ToSlot key.Value
}

// BoundaryRootMap is an ordered-tuple relation whose slice order is irrelevant.
// One source ordinal may clone to several destination ordinals. Every
// destination has at least one preimage. Many-to-one quotients remain valid
// and join their value owners deterministically.
type BoundaryRootMap []BoundaryRootBinding

type boundaryPathBinding struct {
	from keyspace.Key
	to   keyspace.Key
}

type boundaryPathMap []boundaryPathBinding

// BoundaryAllocationAuthority is the immutable allocation quotient owned by
// one structural invocation route: either a forest root or one lexical Apply.
// Destinations are derived from route authority; callers cannot supply
// arbitrary identity maps. Concrete non-template identities are stable by
// definition. Reverse fibers are frozen beside the forward map so every must
// lane consumes the same quotient authority.
type BoundaryAllocationAuthority struct {
	bindings         map[identity.AllocationTemplate]identity.ID
	preimages        map[identity.ID][]identity.Term
	target           lexicalidentity.StableLexicalBodyID
	caller           lexicalidentity.StableLexicalBodyID
	point            uint32
	occurrence       uint32
	root             bool
	transportMu      sync.Mutex
	transportBuckets map[boundaryTransportIdentity][]*BoundaryTransport
}

// BoundaryAllocationRoute is the typed structural invocation coordinate for
// allocation substitution. Root entry and lexical Apply are disjoint forms.
type BoundaryAllocationRoute struct {
	target     lexicalidentity.StableLexicalBodyID
	caller     lexicalidentity.StableLexicalBodyID
	point      uint32
	occurrence uint32
	root       bool
}

func RootBoundaryAllocationRoute(target lexicalidentity.StableLexicalBodyID) BoundaryAllocationRoute {
	return BoundaryAllocationRoute{target: target, root: true}
}

func ApplyBoundaryAllocationRoute(target, caller lexicalidentity.StableLexicalBodyID, point, occurrence uint32) BoundaryAllocationRoute {
	return BoundaryAllocationRoute{target: target, caller: caller, point: point, occurrence: occurrence}
}

// NewBoundaryAllocationAuthority derives every instantiated allocation identity
// once, while the invocation route is frozen. Tuple-mu reuses this authority
// through all iterations and allocates no identity strings.
func NewBoundaryAllocationAuthority(route BoundaryAllocationRoute, templates []identity.AllocationTemplate) (*BoundaryAllocationAuthority, error) {
	if route.target == (lexicalidentity.StableLexicalBodyID{}) || route.root && (route.caller != (lexicalidentity.StableLexicalBodyID{}) || route.point != 0 || route.occurrence != 0) ||
		!route.root && (route.caller == (lexicalidentity.StableLexicalBodyID{}) || route.point == 0) {
		return nil, fmt.Errorf("state: allocation authority requires a structural root or Apply route")
	}
	authority := &BoundaryAllocationAuthority{target: route.target, caller: route.caller, point: route.point, occurrence: route.occurrence, root: route.root}
	if len(templates) == 0 {
		return authority, nil
	}
	authority.bindings = make(map[identity.AllocationTemplate]identity.ID, len(templates))
	authority.preimages = make(map[identity.ID][]identity.Term, len(templates))
	actuals := make(map[identity.ID]struct{}, len(templates))
	for _, template := range templates {
		if !template.Valid() || template.Owner() != route.target {
			return nil, fmt.Errorf("state: allocation authority contains invalid template")
		}
		if _, duplicate := authority.bindings[template]; duplicate {
			return nil, fmt.Errorf("state: allocation authority contains duplicate template")
		}
		actual := identity.RootBoundaryAllocation(template)
		if !route.root {
			actual = identity.BoundaryAllocation(template, route.caller, route.point, route.occurrence)
		}
		if actual == (identity.ID{}) {
			return nil, fmt.Errorf("state: allocation authority could not derive fresh identity")
		}
		if _, collision := actuals[actual]; collision {
			return nil, fmt.Errorf("state: allocation authority identities collide")
		}
		authority.bindings[template] = actual
		authority.preimages[actual] = []identity.Term{identity.AllocationTerm(template)}
		actuals[actual] = struct{}{}
	}
	return authority, nil
}

// MatchesFrame reports whether this is the exact immutable authority linked to
// one structural call frame. It is O(1) and allocates nothing in Apply.
func (p *BoundaryAllocationAuthority) MatchesFrame(target, caller lexicalidentity.StableLexicalBodyID, callPoint, occurrence uint32) bool {
	return p != nil && !p.root && p.target == target && p.caller == caller && p.point == callPoint && p.occurrence == occurrence &&
		target != (lexicalidentity.StableLexicalBodyID{}) && caller != (lexicalidentity.StableLexicalBodyID{}) && callPoint != 0
}

func (p *BoundaryAllocationAuthority) MatchesRoot(target lexicalidentity.StableLexicalBodyID) bool {
	return p != nil && p.root && p.target == target && target != (lexicalidentity.StableLexicalBodyID{})
}

// Empty reports that the route has no lexical allocation templates to
// substitute. The authority and its route identity remain valid.
func (p *BoundaryAllocationAuthority) Empty() bool { return p != nil && len(p.bindings) == 0 }

// BoundaryClosure is the least root-reachable set needed to project State
// lanes. It is state-owned so every lane can share one reachability authority
// rather than inventing its own approximation.
type BoundaryClosure struct {
	slots         map[key.Value]struct{}
	paths         map[keyspace.Key]struct{}
	identities    map[identity.Term]struct{}
	allIdentities bool
	heapSuffixes  map[boundaryHeapSuffix]struct{}
}

type boundaryHeapSuffix struct {
	owner  identity.Term
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
	catalog := defaultLaneCatalog
	if specs != nil {
		catalog = newLaneCatalog(specs)
	}
	domain := catalog.ProductDomain(reg)
	factors, err := domain.Decompose(source)
	if err != nil {
		return BoundaryClosure{}, err
	}
	programs := make([]BoundaryReachabilityProgram, 0, len(factors))
	for _, factor := range factors {
		program, prepareErr := domain.PrepareBoundaryFactorReachability(keys, factor)
		if prepareErr != nil {
			return BoundaryClosure{}, prepareErr
		}
		programs = append(programs, program)
	}
	if len(programs) == 0 {
		empty, sealErr := newBoundaryReachabilityProgramBuilder(reg, keys).seal()
		if sealErr != nil {
			return BoundaryClosure{}, sealErr
		}
		programs = append(programs, empty)
	}
	programSet, err := SealBoundaryReachabilityProgramSet(programs...)
	if err != nil {
		return BoundaryClosure{}, err
	}
	factorRoots := make([]BoundaryFactorRoot, len(roots))
	seedValues := make([]product.Value, 0, len(roots)*2+1)
	if source.values.top {
		seedValues = append(seedValues, product.Top())
	}
	for index, root := range roots {
		factorRoots[index] = BoundaryFactorRoot{Slot: root.Slot, Path: root.Path}
		seedValues = append(seedValues, root.Value)
		if !source.values.top && root.Slot != 0 {
			if value, present := source.values.get(root.Slot); present {
				seedValues = append(seedValues, value)
			}
		}
	}
	selection, err := SealBoundaryFactorSelection(keys, factorRoots, nil, false)
	if err != nil {
		return BoundaryClosure{}, err
	}
	selection, err = programSet.Close(selection, seedValues)
	if err != nil {
		return BoundaryClosure{}, err
	}
	return selection.closure, nil
}

// ContainsSlot reports whether slot belongs to the boundary root tuple.
func (c BoundaryClosure) ContainsSlot(slot key.Value) bool {
	_, ok := c.slots[slot]
	return ok
}

// ContainsPath reports whether path belongs to the least reachable closure.
func (c BoundaryClosure) ContainsPath(path keyspace.Key) bool {
	_, ok := c.paths[path]
	return ok
}

// ContainsIdentity reports whether id belongs to the least reachable closure.
func (c BoundaryClosure) ContainsIdentity(id identity.ID) bool {
	return c.ContainsIdentityTerm(identity.ConcreteTerm(id))
}

func (c BoundaryClosure) ContainsIdentityTerm(term identity.Term) bool {
	if c.allIdentities && term.Valid() {
		return true
	}
	_, ok := c.identities[term]
	return ok
}

// ContainsHeapSuffix reports whether a rootless static-member suffix is
// reachable through owner. Rootless heap suffixes deliberately never enter the
// absolute path namespace.
func (c BoundaryClosure) ContainsHeapSuffix(owner identity.ID, suffix keyspace.Key) bool {
	return c.ContainsHeapSuffixTerm(identity.ConcreteTerm(owner), suffix)
}

func (c BoundaryClosure) ContainsHeapSuffixTerm(owner identity.Term, suffix keyspace.Key) bool {
	_, ok := c.heapSuffixes[boundaryHeapSuffix{owner: owner, suffix: suffix}]
	return ok
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
		// Structural ownership is proved by the keyspace itself. Formatting the
		// two roots here used to rebuild their complete strings once per source
		// path, turning boundary quotienting into an allocation-heavy
		// paths-by-roots string loop.
		depth, fromValid := fromKeys.SegmentLen(binding.from)
		_, toValid := toKeys.SegmentLen(binding.to)
		if !fromValid || !toValid {
			return nil, false
		}
		// KeySpace.HasPrefix is a strict descendant query for some root kinds;
		// boundary substitution also includes the root itself.
		if path != binding.from && !fromKeys.HasPrefix(path, binding.from) {
			continue
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
		} else if importedFrom == binding.to {
			// Crossing lexical bodies can change only KeySpace ownership while
			// preserving the exact symbol/version spelling (notably captures).
			// KeySpace.Rebase deliberately rejects an unchanged spelling, but the
			// boundary substitution is still real: importedPath is now owned by
			// the destination keyspace and retains the certified suffix exactly.
			next = importedPath
		} else {
			next, ok = toKeys.Rebase(importedPath, importedFrom, binding.to)
			if !ok {
				next, ok = toKeys.RebaseToExistential(importedPath, importedFrom, binding.to)
			}
			if !ok {
				// One source root may deliberately have both a structural-value
				// destination and a certified SSA/existential destination. A
				// descendant belongs only to destinations whose address space can
				// represent its exact suffix; the root-only alias still carries the
				// root value and is not authority to erase the descendant route.
				continue
			}
		}
		out = append(out, next)
	}
	if len(out) == 0 {
		return nil, false
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

// rebaseBoundaryIdentity performs exact allocation substitution. It returns
// false for an unmapped identity so callers cannot partially rebase a lane.
func rebaseBoundaryIdentity(lens *BoundaryAllocationAuthority, term identity.Term) (identity.Term, bool) {
	if concrete, ok := term.Concrete(); ok {
		return identity.ConcreteTerm(concrete), true
	}
	template, allocation := term.Allocation()
	if !allocation || lens == nil {
		return identity.Term{}, false
	}
	next, ok := lens.bindings[template]
	return identity.ConcreteTerm(next), ok && next != (identity.ID{})
}

// boundaryAllocationTermAlreadyOutsideFrame identifies an allocation term
// that was introduced by an inner sealed boundary. The current Apply cannot
// mint a second image for another owner's template; the owning frame's
// authority is the only authority permitted to do that.
func boundaryAllocationTermAlreadyOutsideFrame(lens *BoundaryAllocationAuthority, term identity.Term) bool {
	template, allocation := term.Allocation()
	return allocation && lens != nil && template.Owner() != lens.target
}

// RebaseAllocation exposes the lens's typed allocation substitution. Concrete
// identities never enter this authority.
func (p *BoundaryAllocationAuthority) RebaseAllocation(template identity.AllocationTemplate) (identity.ID, bool) {
	if p == nil || !template.Valid() {
		return identity.ID{}, false
	}
	next, ok := p.bindings[template]
	return next, ok && next != (identity.ID{})
}
