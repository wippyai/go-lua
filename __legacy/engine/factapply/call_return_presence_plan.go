package factapply

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/value/returnpresence"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CallReturnPresenceRowTarget is one concrete path spelling for a result
// coordinate in a feasible lexical-call row.
type CallReturnPresenceRowTarget struct {
	Index int
	Path  keyspace.Key
	Value product.Value
}

// CallReturnPresenceTarget is one result-address coordinate which may carry a
// presence correlation across a call boundary.  It contains no row value: the
// finite coordinate universe is frozen before guarded execution chooses a
// feasible return row.
type CallReturnPresenceTarget struct {
	Index int
	Path  keyspace.Key
}

type callReturnPresenceTarget struct {
	path     keyspace.Key
	presence presence.Value
}

type callReturnPresenceTargetKey struct {
	index int
	path  keyspace.Key
}

// canonicalCallReturnPresencePath selects the same state owner used by
// RootOrVisibleKeyspaceKey: a bare symbol is the root Values coordinate, while
// members remain point-versioned local paths. Call-result lenses are minted at
// assignment points and therefore carry an SSA version even for a bare root;
// retaining that spelling would disconnect the N5 row from later Choice
// refinement of the symbol coordinate.
func canonicalCallReturnPresencePath(ks *keyspace.KeySpace, path keyspace.Key) keyspace.Key {
	decoded, ok := ks.StatePath(path)
	if !ok || decoded.Symbol == 0 || len(decoded.Segments) != 0 {
		return path
	}
	decoded.Version = 0
	return ks.FromPath(decoded)
}

func canonicalCallReturnPresencePaths(
	ks *keyspace.KeySpace,
	targets []CallReturnPresenceTarget,
) (map[int][]keyspace.Key, []int) {
	values := make(map[int][]keyspace.Key, len(targets))
	for _, target := range targets {
		target.Path = canonicalCallReturnPresencePath(ks, target.Path)
		if target.Index < 0 || target.Path.Kind == keyspace.KindInvalid || ks.FormatReadOnly(target.Path) == "" {
			continue
		}
		row := values[target.Index]
		duplicate := false
		for _, prior := range row {
			if prior == target.Path {
				duplicate = true
				break
			}
		}
		if !duplicate {
			values[target.Index] = append(row, target.Path)
		}
	}
	indices := make([]int, 0, len(values))
	for index, row := range values {
		indices = append(indices, index)
		sort.Slice(row, func(i, j int) bool { return ks.Less(row[i], row[j]) })
		values[index] = row
	}
	sort.Ints(indices)
	return values, indices
}

// PrepareCallReturnPresenceRow freezes one row into a deterministic implication
// inventory. Impossible trigger alternatives publish both target alternatives
// (vacuous truth), so must-join across feasible rows retains exactly the
// correlations common to the complete return relation.
func (a *PathSemanticAuthority) PrepareCallReturnPresenceRow(
	reg *axis.Registry,
	point cfg.Point,
	targets []CallReturnPresenceRowTarget,
) (PresenceImplicationPlan, error) {
	if reg == nil || !a.Valid() {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: call return row requires exact path authority")
	}
	plan, err := a.prepareCallReturnPresenceRowInKeySpace(reg, a.resolver.KeySpace(), point, targets)
	plan.resolver = a.resolver
	if err == nil {
		plan.access, err = freezePresenceKeyAccess(a.resolver, point, plan.keys, presenceImplicationPaths(plan.publications))
	}
	return plan, err
}

// PrepareCallReturnPresenceRowInKeySpace freezes the same N5 presence law over
// an already-sealed structural vocabulary. Formal Output roots and concrete
// ret[n] roots therefore share one implication constructor; neither path is an
// adapter over the other. The caller must supply paths owned by keys.
func (a *PathSemanticAuthority) PrepareCallReturnPresenceRowInKeySpace(
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	point cfg.Point,
	targets []CallReturnPresenceRowTarget,
) (PresenceImplicationPlan, error) {
	if reg == nil || !a.Valid() || keys == nil || !keys.Valid() {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: call return row requires an exact sealed keyspace")
	}
	return a.prepareCallReturnPresenceRowInKeySpace(reg, keys, point, targets)
}

func (a *PathSemanticAuthority) prepareCallReturnPresenceRowInKeySpace(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	point cfg.Point,
	targets []CallReturnPresenceRowTarget,
) (PresenceImplicationPlan, error) {
	known := make(map[callReturnPresenceTargetKey]presence.Value, len(targets))
	pathTargets := make([]CallReturnPresenceTarget, 0, len(targets))
	for _, target := range targets {
		target.Path = canonicalCallReturnPresencePath(ks, target.Path)
		value, exact := returnpresence.KnownPresence(product.PresenceOf(target.Value))
		if !exact || target.Index < 0 || target.Path.Kind == keyspace.KindInvalid || ks.FormatReadOnly(target.Path) == "" {
			continue
		}
		key := callReturnPresenceTargetKey{index: target.Index, path: target.Path}
		if _, duplicate := known[key]; !duplicate {
			known[key] = value
			pathTargets = append(pathTargets, CallReturnPresenceTarget{Index: target.Index, Path: target.Path})
		}
	}
	paths, indices := canonicalCallReturnPresencePaths(ks, pathTargets)
	values := make(map[int][]callReturnPresenceTarget, len(paths))
	for _, index := range indices {
		for _, path := range paths[index] {
			values[index] = append(values[index], callReturnPresenceTarget{
				path: path, presence: known[callReturnPresenceTargetKey{index: index, path: path}],
			})
		}
	}

	plan := PresenceImplicationPlan{reg: reg, keys: ks, point: point, barriers: ConcretePresenceImplicationTrailingBarrier}
	if len(values) < 2 {
		plan.access, _ = freezePresenceKeyAccess(nil, point, ks, nil)
		return plan, nil
	}
	total := 0
	for _, row := range values {
		total += len(row)
	}
	plan.publications = make([]pathevidence.PathPresenceImplication, 0, total*total*2)
	for _, triggerIndex := range indices {
		for _, targetIndex := range indices {
			if targetIndex == triggerIndex {
				continue
			}
			for _, trigger := range values[triggerIndex] {
				for _, target := range values[targetIndex] {
					for _, candidate := range []presence.Value{presence.Present(), presence.Absent()} {
						if presence.Equal(candidate, trigger.presence) {
							plan.publications = append(plan.publications,
								pathevidence.NewPathPresenceImplication(trigger.path, candidate, target.path, target.presence))
							continue
						}
						plan.publications = append(plan.publications,
							pathevidence.NewPathPresenceImplication(trigger.path, candidate, target.path, presence.Present()),
							pathevidence.NewPathPresenceImplication(trigger.path, candidate, target.path, presence.Absent()),
						)
					}
				}
			}
		}
	}
	canonical, ok := pathevidence.CanonicalPathPresenceImplications(reg, ks, plan.publications)
	if !ok {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: call return row produced invalid presence implications")
	}
	plan.publications = canonical
	var accessErr error
	plan.access, accessErr = freezePresenceKeyAccess(nil, point, ks, presenceImplicationPaths(canonical))
	if accessErr != nil {
		return PresenceImplicationPlan{}, accessErr
	}
	return plan, nil
}

// CallReturnPresenceCoordinateInventory freezes the complete finite identity
// universe for correlations which any feasible return row may publish.  It is
// deliberately non-executable: row values still select the sound subset via
// PrepareCallReturnPresenceRow, while branch consequence topology can be
// sealed before those values exist.
func (a *PathSemanticAuthority) CallReturnPresenceCoordinateInventory(
	domain state.ProductDomain,
	targets []CallReturnPresenceTarget,
) (state.CoordinateFactorInventory, error) {
	if !a.Valid() || !domain.Valid() || domain.Registry() == nil {
		return state.CoordinateFactorInventory{}, fmt.Errorf("factapply: call return presence inventory requires exact path authority")
	}
	paths, indices := canonicalCallReturnPresencePaths(a.resolver.KeySpace(), targets)
	publications := make([]pathevidence.PathPresenceImplication, 0)
	for _, triggerIndex := range indices {
		for _, targetIndex := range indices {
			if targetIndex == triggerIndex {
				continue
			}
			for _, trigger := range paths[triggerIndex] {
				for _, target := range paths[targetIndex] {
					for _, triggerPresence := range []presence.Value{presence.Present(), presence.Absent()} {
						for _, targetPresence := range []presence.Value{presence.Present(), presence.Absent()} {
							publications = append(publications, pathevidence.NewPathPresenceImplication(
								trigger, triggerPresence, target, targetPresence,
							))
						}
					}
				}
			}
		}
	}
	canonical, ok := pathevidence.CanonicalPathPresenceImplications(domain.Registry(), a.resolver.KeySpace(), publications)
	if !ok {
		return state.CoordinateFactorInventory{}, fmt.Errorf("factapply: call return presence inventory is invalid")
	}
	access, accessErr := freezePresenceKeyAccess(a.resolver, 0, a.resolver.KeySpace(), presenceImplicationPaths(canonical))
	if accessErr != nil {
		return state.CoordinateFactorInventory{}, accessErr
	}
	plan := PresenceImplicationPlan{reg: domain.Registry(), keys: a.resolver.KeySpace(), access: access, resolver: a.resolver, publications: canonical}
	return plan.CoordinateFactorInventory(domain)
}

// ApplyCoordinates publishes this row directly into the registry-selected
// presence-implication family. It is the guarded adapter: no LaneFactor or
// whole Product State is constructed, and family ownership is proven by the
// ProductDomain capability rather than a PathEvidence name check.
func (p PresenceImplicationPlan) ApplyCoordinates(
	domain state.ProductDomain,
	skeleton state.CoordinateFamilySkeleton,
	scalars []state.CoordinateScalarFactor,
) (state.CoordinateFamilySkeleton, []state.CoordinateScalarFactor, error) {
	if p.reg == nil || !domain.Valid() || domain.Registry() != p.reg {
		return state.CoordinateFamilySkeleton{}, nil, fmt.Errorf("factapply: invalid call return presence coordinate plan")
	}
	if len(p.publications) == 0 {
		return skeleton, append([]state.CoordinateScalarFactor(nil), scalars...), nil
	}
	return domain.ApplyCoordinatePresenceImplications(skeleton, scalars, p.publications)
}
