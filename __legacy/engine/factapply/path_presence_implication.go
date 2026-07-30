package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func pathValuePresenceImplicationAt(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	fact factflow.PathValuePresenceImplication,
) (pathevidence.PathPresenceImplication, bool) {
	if resolver == nil {
		return pathevidence.PathPresenceImplication{}, false
	}
	trigger, ok := visibility.AddressAt(resolver, ctx.Point, fact.TriggerPathRef()).RootOrVisibleKeyspaceKey()
	if !ok {
		return pathevidence.PathPresenceImplication{}, false
	}
	var other keyspace.Key
	if fact.HasTriggerPathEqual() {
		var otherOK bool
		other, otherOK = visibility.AddressAt(resolver, ctx.Point, fact.TriggerOtherPathRef()).RootOrVisibleKeyspaceKey()
		if !otherOK {
			return pathevidence.PathPresenceImplication{}, false
		}
	}
	target, ok := visibility.AddressAt(resolver, ctx.Point, fact.TargetPathRef()).RootOrVisibleKeyspaceKey()
	if !ok {
		return pathevidence.PathPresenceImplication{}, false
	}
	var implication pathevidence.PathPresenceImplication
	if fact.HasTriggerPathEqual() {
		if fact.HasTargetValue() {
			implication = pathevidence.NewPathEqualValueRefinementImplication(
				trigger,
				other,
				target,
				fact.TargetValue(),
			)
		} else {
			return pathevidence.PathPresenceImplication{}, false
		}
	} else if fact.HasTargetValue() {
		if fact.HasTriggerPresence() {
			implication = pathevidence.NewPathTruthyValueRefinementImplication(
				trigger,
				fact.TriggerValue(),
				target,
				fact.TargetValue(),
			)
		} else {
			implication = pathevidence.NewPathValueRefinementImplication(
				trigger,
				fact.TriggerValue(),
				target,
				fact.TargetValue(),
			)
		}
	} else {
		implication = pathevidence.NewPathValuePresenceImplication(
			trigger,
			fact.TriggerValue(),
			target,
			fact.TargetPresence(),
		)
	}
	return implication, true
}

func activatePathPresenceImplications(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
) state.State {
	return activatePathPresenceImplicationsWithToken(reg, resolver, point, out, nil)
}

// activatePathPresenceImplicationsWithToken closes implication consequences to
// a local fixed point. A consequence can require structural state comparison
// whose type-witness equality walks recursive types, so cancellation belongs
// inside this loop rather than only in its enclosing transfer.
func activatePathPresenceImplicationsWithToken(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	token *cancellation.Token,
) state.State {
	result := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: reg,
		Resolver: resolver,
		Point:    point,
		Output:   out,
		Token:    token,
	})
	return result.Output
}

func pathPresenceImplicationTriggered(
	reg *axis.Registry,
	access presenceKeyAccess,
	storage presenceImplicationStorage,
	implication pathevidence.PathPresenceImplication,
) (bool, error) {
	if implication.HasTriggerPathEqual {
		return pathPresenceImplicationPathEqualTriggered(storage, implication)
	}
	if implication.HasTriggerTruthiness {
		current, ok := readPresenceStoragePathValue(reg, access, storage, implication.Trigger)
		if !ok || product.Equal(reg, current, product.Bottom(reg)) {
			return false, nil
		}
		canTruthy := valuerefine.CanBeTruthy(reg, current)
		canFalsy := valuerefine.CanBeFalsy(reg, current)
		return canTruthy != canFalsy && canTruthy == implication.TriggerTruthy, nil
	}
	if implication.HasTriggerValue {
		current, ok := readPresenceStoragePathValue(reg, access, storage, implication.Trigger)
		if !ok || product.Equal(reg, current, product.Bottom(reg)) {
			return false, nil
		}
		if implication.HasTriggerPresence {
			presenceConstraint := product.NewWithPresence(reg, product.ShapeTop, implication.TriggerPresence)
			proven, valid := storage.HasProof(pathevidence.BranchProof{
				Kind:     pathevidence.BranchProofPathPresence,
				Path:     implication.Trigger,
				Presence: implication.TriggerPresence,
			})
			if !valid {
				return false, fmt.Errorf("factapply: invalid presence proof storage")
			}
			if !product.Domain(reg).LessOrEq(current, presenceConstraint) && !proven {
				return false, nil
			}
			meet := product.Meet(reg, current, implication.TriggerValue)
			return !product.Equal(reg, meet, product.Bottom(reg)) && !presence.Equal(product.PresenceOf(meet), presence.Bottom()), nil
		}
		return product.Domain(reg).LessOrEq(current, implication.TriggerValue), nil
	}
	if !presenceIsConcrete(implication.TriggerPresence) {
		return false, nil
	}
	// Stable root keys name the lexical binding directly. Reading that slot is
	// the exact selected-edge fact and avoids routing an unversioned tuple
	// relation back through a point-versioned resolver spelling.
	if slot, ok := rootValueDependencyForKey(access.keys, implication.Trigger); ok {
		value, valid := storage.ReadRoot(slot)
		if !valid {
			return false, fmt.Errorf("factapply: undeclared presence root read")
		}
		current := product.PresenceOf(value)
		if presence.Equal(current, implication.TriggerPresence) {
			return true, nil
		}
	}
	current, ok := readPresenceStoragePathValue(reg, access, storage, implication.Trigger)
	return ok && presence.Equal(product.PresenceOf(current), implication.TriggerPresence), nil
}

func pathPresenceImplicationPathEqualTriggered(
	storage presenceImplicationStorage,
	implication pathevidence.PathPresenceImplication,
) (bool, error) {
	if storage == nil {
		return false, fmt.Errorf("factapply: invalid equality trigger storage")
	}
	forward, valid := storage.HasProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  implication.Trigger,
		Other: implication.TriggerOther,
	})
	if !valid {
		return false, fmt.Errorf("factapply: invalid equality proof storage")
	}
	reverse, valid := storage.HasProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  implication.TriggerOther,
		Other: implication.Trigger,
	})
	if !valid {
		return false, fmt.Errorf("factapply: invalid equality proof storage")
	}
	if forward || reverse {
		return true, nil
	}
	equal, valid := storage.HasEquivalentKey(implication.Trigger, implication.TriggerOther)
	if !valid {
		return false, fmt.Errorf("factapply: invalid equality storage")
	}
	return equal, nil
}

type presenceImplicationTargetLocation struct {
	root bool
	slot key.ValueDependency
	path keyspace.Key
}

type presenceImplicationRoundTarget struct {
	location   presenceImplicationTargetLocation
	constraint product.Value
	initialize bool
}

type presenceImplicationRound struct {
	targets       map[presenceImplicationTargetLocation]int
	ordered       []presenceImplicationRoundTarget
	invalidations []pathdom.PathKey
}

func (r *presenceImplicationRound) addTarget(reg *axis.Registry, location presenceImplicationTargetLocation, constraint product.Value, initialize bool) {
	if index, ok := r.targets[location]; ok {
		target := &r.ordered[index]
		target.constraint = product.Meet(reg, target.constraint, constraint)
		target.initialize = target.initialize || initialize
		return
	}
	if r.targets == nil {
		r.targets = make(map[presenceImplicationTargetLocation]int)
	}
	r.targets[location] = len(r.ordered)
	r.ordered = append(r.ordered, presenceImplicationRoundTarget{location: location, constraint: constraint, initialize: initialize})
}

func accumulatePathPresenceImplicationTarget(
	reg *axis.Registry,
	access presenceKeyAccess,
	storage presenceImplicationStorage,
	implication pathevidence.PathPresenceImplication,
	round *presenceImplicationRound,
) error {
	if !implication.HasTargetValue && !presenceIsConcrete(implication.TargetPresence) {
		return nil
	}
	// A segment-free endpoint denotes the lexical symbol carrier, regardless
	// of which point-version spelling was current when the implication was
	// published. This mirrors trigger activation and prevents a later CFG point
	// from hiding a valid tuple correlation behind a different resolver version.
	if _, ok := rootValueDependencyForKey(access.keys, implication.Target); ok {
		accumulatePathPresenceImplicationTargetKey(reg, access.keys, implication, implication.Target, round)
		return nil
	}
	targetKeys := []keyspace.Key{implication.Target}
	seen := map[keyspace.Key]struct{}{implication.Target: {}}
	equivalents, valid := storage.EquivalentKeys(implication.Target)
	if !valid {
		return fmt.Errorf("factapply: invalid equivalent-key storage")
	}
	for _, equivalent := range equivalents {
		if _, ok := seen[equivalent]; ok {
			continue
		}
		seen[equivalent] = struct{}{}
		targetKeys = append(targetKeys, equivalent)
	}
	for _, targetKey := range targetKeys {
		// Visibility is a property of a semantic equality class, not of the
		// representative chosen by a producer. Boundary existentials are not
		// caller-visible themselves, but their proven root representative is.
		if !access.readable(targetKey) {
			continue
		}
		accumulatePathPresenceImplicationTargetKey(reg, access.keys, implication, targetKey, round)
	}
	return nil
}

func accumulatePathPresenceImplicationTargetKey(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	implication pathevidence.PathPresenceImplication,
	targetKey keyspace.Key,
	round *presenceImplicationRound,
) {
	constraint := implication.TargetValue
	if !implication.HasTargetValue {
		constraint = product.NewWithPresence(reg, product.ShapeTop, implication.TargetPresence)
	}
	if slot, ok := rootValueDependencyForKey(ks, targetKey); ok {
		location := presenceImplicationTargetLocation{root: true, slot: slot}
		round.addTarget(reg, location, constraint, implication.HasTargetValue)
	} else {
		location := presenceImplicationTargetLocation{path: targetKey}
		// Local path refinements are a sparse map: a missing Bottom entry is
		// initialized by the first consequence. Root Values instead refine an
		// already-existing lexical carrier unless the implication carries an exact
		// assigned value.
		round.addTarget(reg, location, constraint, true)
	}
	if presenceImplicationTargetInvalidatesDescendants(implication) {
		round.invalidations = append(round.invalidations, ks.Format(targetKey))
	}
}

// applyPresenceImplicationRound commits the complete conjunctive consequence
// set from one immutable trigger snapshot. No downstream row can observe a
// transient first writer before a conflicting writer has been met into the
// same target. The storage fork also makes cross-axis invalidation atomic.
func applyPresenceImplicationRound(
	reg *axis.Registry,
	storage presenceImplicationStorage,
	round presenceImplicationRound,
) (bool, error) {
	staged := storage.Fork()
	if staged == nil {
		return false, fmt.Errorf("factapply: presence implication storage cannot fork")
	}
	changed := false
	for _, target := range round.ordered {
		var current product.Value
		var valid bool
		if target.location.root {
			current, valid = staged.ReadRoot(target.location.slot)
		} else {
			current, valid = staged.ReadPath(target.location.path)
		}
		if !valid {
			return false, fmt.Errorf("factapply: undeclared presence target read")
		}
		if !product.BelongsToRegistry(reg, current) || !product.BelongsToRegistry(reg, target.constraint) {
			return false, fmt.Errorf("%w: foreign presence implication value", state.ErrInvalidLaneFactor)
		}
		next := product.Meet(reg, current, target.constraint)
		if target.initialize && product.Equal(reg, current, product.Bottom(reg)) {
			next = target.constraint
		}
		// An established conjunction that is Bottom is an infeasible abstract
		// environment, not an absent sparse-map entry. Collapse the route now so
		// no later closure can reinterpret the contradiction as unobserved and
		// resurrect it.
		if (product.Equal(reg, next, product.Bottom(reg)) || presence.Equal(product.PresenceOf(next), presence.Bottom())) &&
			(target.initialize || !product.Equal(reg, current, product.Bottom(reg))) {
			if !staged.MakeUnreachable() || !storage.Commit(staged) {
				return false, fmt.Errorf("factapply: presence contradiction commit failed")
			}
			return true, nil
		}
		var wrote bool
		if target.location.root {
			wrote, valid = staged.WriteRoot(target.location.slot, next)
		} else {
			wrote, valid = staged.WritePath(target.location.path, next)
		}
		if !valid {
			return false, fmt.Errorf("factapply: undeclared presence target write")
		}
		changed = changed || wrote
	}
	seenInvalidation := make(map[pathdom.PathKey]struct{}, len(round.invalidations))
	for _, root := range round.invalidations {
		if _, duplicate := seenInvalidation[root]; duplicate {
			continue
		}
		seenInvalidation[root] = struct{}{}
		invalidated, valid := staged.InvalidateDescendants(root)
		if !valid {
			return false, fmt.Errorf("factapply: invalid descendant invalidation storage")
		}
		changed = changed || invalidated
	}
	if !storage.Commit(staged) {
		return false, fmt.Errorf("factapply: presence implication storage commit failed")
	}
	return changed, nil
}

func readPresenceStoragePathValue(reg *axis.Registry, access presenceKeyAccess, storage presenceImplicationStorage, path keyspace.Key) (product.Value, bool) {
	if reg == nil || !access.valid() || storage == nil {
		return product.Value{}, false
	}
	candidates := []keyspace.Key{path}
	equivalents, valid := storage.EquivalentKeys(path)
	if !valid {
		return product.Value{}, false
	}
	seen := map[keyspace.Key]struct{}{path: {}}
	for _, equivalent := range equivalents {
		if _, duplicate := seen[equivalent]; duplicate {
			continue
		}
		seen[equivalent] = struct{}{}
		candidates = append(candidates, equivalent)
	}
	var out product.Value
	observed := false
	declared := false
	for _, candidate := range candidates {
		if !access.readable(candidate) {
			continue
		}
		value, ok := readPresenceStorageKey(access.keys, storage, candidate)
		if !ok {
			continue
		}
		declared = true
		// A missing sparse path refinement is not an equality constraint.
		// Root Values supply the concrete observation when the producer's
		// representative is a boundary existential.
		if product.Equal(reg, value, product.Bottom(reg)) {
			continue
		}
		if !observed {
			out, observed = value, true
			continue
		}
		out = product.Meet(reg, out, value)
	}
	if observed {
		return out, true
	}
	return product.Bottom(reg), declared
}

func readPresenceStorageKey(keys *keyspace.KeySpace, storage presenceImplicationStorage, path keyspace.Key) (product.Value, bool) {
	if storage == nil {
		return product.Value{}, false
	}
	if slot, root := rootValueDependencyForKey(keys, path); root {
		return storage.ReadRoot(slot)
	}
	return storage.ReadPath(path)
}

func presenceImplicationTargetInvalidatesDescendants(implication pathevidence.PathPresenceImplication) bool {
	targetPresence := implication.TargetPresence
	if implication.HasTargetValue {
		targetPresence = product.PresenceOf(implication.TargetValue)
	}
	return presence.Equal(targetPresence, presence.Absent())
}

func pathKeyCurrentlyVisibleKey(resolver *visibility.Resolver, point cfg.Point, pathKey keyspace.Key) bool {
	if resolver == nil || pathKey.Kind == keyspace.KindInvalid {
		return false
	}
	if pathKey.Kind == keyspace.KindUnversionedSym && pathKey.Segs == 0 && pathKey.Sym != 0 {
		return true
	}
	segments, ok := resolver.KeySpace().SegmentsView(pathKey)
	if !ok {
		return false
	}
	current, ok := factKeyspaceKeyAt(resolver, point, pathdom.Path{Symbol: pathKey.Sym, Segments: segments})
	return ok && current == pathKey
}

func rootValueDependencyForKey(keys *keyspace.KeySpace, path keyspace.Key) (key.ValueDependency, bool) {
	if path.Segs != 0 {
		return key.ValueDependency{}, false
	}
	return pathevidence.PathValueDependency(keys, path)
}

func presenceIsConcrete(value presence.Value) bool {
	return presence.Equal(value, presence.Present()) || presence.Equal(value, presence.Absent())
}
