package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

// pathValueMap is the native value-store carrier for canonical flow paths. The
// encoded string form is still used at parser/API boundaries, but semantic state
// is keyed by constraint.PathKey.
type pathValueMap map[constraint.PathKey]product.AbstractValue

// fieldOverlayCacheKey identifies root overlays in the point-sensitive mutable store.
type fieldOverlayCacheKey struct {
	point cfg.Point
	root  constraint.PathKey
}

type fieldMergeCacheKey struct {
	point cfg.Point
	root  constraint.PathKey
	base  typ.Type
}

type pointRootKey struct {
	point cfg.Point
	root  constraint.PathKey
}

// pathSuffixKey is the canonical encoded form of a non-root flow path suffix.
//
// It is intentionally a named cache key rather than a raw string. Solver state
// stays keyed by constraint.PathKey and semantic code converts suffixes back to
// []constraint.Segment before reasoning about them; this type only identifies
// the deterministic suffix representation used for indexes and memo tables.
type pathSuffixKey string

func newPathSuffixKey(s string) (pathSuffixKey, bool) {
	if s == "" {
		return "", false
	}
	if s[0] != '.' && s[0] != '[' {
		return "", false
	}
	if len(pathkey.ParseSuffix(s)) == 0 {
		return "", false
	}
	return pathSuffixKey(s), true
}

func (k pathSuffixKey) String() string {
	return string(k)
}

func (k pathSuffixKey) Segments() []constraint.Segment {
	return pathkey.ParseSuffix(string(k))
}

func (k pathSuffixKey) PathUnder(root constraint.PathKey) constraint.PathKey {
	if root == "" || k == "" {
		return ""
	}
	return constraint.PathKey(string(root) + string(k))
}

func (s *Solution) ensureValueSuffixIndex() {
	if s == nil || s.valueSuffixIndex != nil {
		return
	}
	s.valueSuffixIndex = make(map[constraint.PathKey][]pathSuffixKey)
	for key := range s.values {
		s.indexValueSuffix(key)
	}
}

func (s *Solution) indexValueSuffix(key constraint.PathKey) {
	root, suffix, ok := indexedPathSuffix(key)
	if !ok {
		return
	}
	if s.valueSuffixIndex == nil {
		s.valueSuffixIndex = make(map[constraint.PathKey][]pathSuffixKey, 1)
	}
	s.valueSuffixIndex[root] = insertSortedSuffix(s.valueSuffixIndex[root], suffix)
}

func (s *Solution) removeValueSuffix(key constraint.PathKey) {
	root, suffix, ok := indexedPathSuffix(key)
	if !ok || s == nil || s.valueSuffixIndex == nil {
		return
	}
	next := removeSortedSuffix(s.valueSuffixIndex[root], suffix)
	if len(next) == 0 {
		delete(s.valueSuffixIndex, root)
		return
	}
	s.valueSuffixIndex[root] = next
}

func (s *Solution) indexMutableSuffixesForPoint(p cfg.Point) {
	if s == nil {
		return
	}
	state := s.mutableOut[p]
	s.replaceMutableSuffixesForPoint(p, nil, state)
}

func (s *Solution) replaceMutableSuffixesForPoint(p cfg.Point, old, next pathValueMap) {
	if s == nil {
		return
	}
	for key := range old {
		s.removeMutableSuffix(p, key)
	}
	if s.mutableSuffixIndexed == nil {
		s.mutableSuffixIndexed = make(map[cfg.Point]bool, 1)
	}
	s.mutableSuffixIndexed[p] = true
	if len(next) == 0 {
		return
	}
	if s.mutableSuffixIndex == nil {
		s.mutableSuffixIndex = make(map[pointRootKey][]pathSuffixKey, len(next))
	}
	for key := range next {
		s.indexMutableSuffix(p, key)
	}
}

func (s *Solution) indexMutableSuffix(p cfg.Point, key constraint.PathKey) {
	root, suffix, ok := indexedPathSuffix(key)
	if !ok {
		return
	}
	if s.mutableSuffixIndex == nil {
		s.mutableSuffixIndex = make(map[pointRootKey][]pathSuffixKey, 1)
	}
	indexKey := pointRootKey{point: p, root: root}
	s.mutableSuffixIndex[indexKey] = insertSortedSuffix(s.mutableSuffixIndex[indexKey], suffix)
}

func (s *Solution) removeMutableSuffix(p cfg.Point, key constraint.PathKey) {
	root, suffix, ok := indexedPathSuffix(key)
	if !ok || s == nil || s.mutableSuffixIndex == nil {
		return
	}
	indexKey := pointRootKey{point: p, root: root}
	next := removeSortedSuffix(s.mutableSuffixIndex[indexKey], suffix)
	if len(next) == 0 {
		delete(s.mutableSuffixIndex, indexKey)
		return
	}
	s.mutableSuffixIndex[indexKey] = next
}

func indexedPathSuffix(key constraint.PathKey) (constraint.PathKey, pathSuffixKey, bool) {
	root, suffix, ok := pathkey.ParseRootAndSuffix(key)
	if !ok || root == "" {
		return "", "", false
	}
	suffixKey, ok := newPathSuffixKey(suffix)
	if !ok {
		return "", "", false
	}
	return constraint.PathKey(root), suffixKey, true
}

func insertSortedSuffix(values []pathSuffixKey, suffix pathSuffixKey) []pathSuffixKey {
	i := sort.Search(len(values), func(i int) bool {
		return values[i] >= suffix
	})
	if i < len(values) && values[i] == suffix {
		return values
	}
	values = append(values, "")
	copy(values[i+1:], values[i:])
	values[i] = suffix
	return values
}

func removeSortedSuffix(values []pathSuffixKey, suffix pathSuffixKey) []pathSuffixKey {
	i := sort.Search(len(values), func(i int) bool {
		return values[i] >= suffix
	})
	if i >= len(values) || values[i] != suffix {
		return values
	}
	copy(values[i:], values[i+1:])
	return values[:len(values)-1]
}

func sortPathKeys(keys []constraint.PathKey) {
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
}

// beginPointMutableState computes the Kildall IN-state for p (the join of every
// predecessor OUT-state), installs a working clone of it where the point's own
// transfers read and write (mutableOut[p]), and returns the prior committed
// OUT-state. commitPointMutableState then widens the working store against that
// prior OUT to keep the iterate monotone and diffs the two to drive propagation.
func (s *Solution) beginPointMutableState(p cfg.Point) pathValueMap {
	if s == nil {
		return nil
	}
	if s.mutableOut == nil {
		s.mutableOut = make(map[cfg.Point]pathValueMap)
	}
	oldOut := cloneValueMap(s.mutableOut[p])
	incoming := s.joinPredecessorMutableState(p)
	if s.mutableIn == nil {
		s.mutableIn = make(map[cfg.Point]pathValueMap)
	}
	if len(incoming) == 0 {
		delete(s.mutableIn, p)
	} else {
		s.mutableIn[p] = cloneValueMap(incoming)
	}
	working := cloneValueMap(incoming)
	if len(working) == 0 {
		delete(s.mutableOut, p)
		if s.mutablePresence != nil {
			delete(s.mutablePresence, p)
		}
	} else {
		s.mutableOut[p] = working
		s.rebuildMutablePresenceForPoint(p)
	}
	if s.fieldOverlayCache != nil {
		clear(s.fieldOverlayCache)
	}
	if s.fieldMergeCache != nil {
		clear(s.fieldMergeCache)
	}
	if s.narrowedTypeCache != nil {
		clear(s.narrowedTypeCache)
	}
	if s.childTypesCache != nil {
		clear(s.childTypesCache)
	}
	s.replaceMutableSuffixesForPoint(p, oldOut, working)
	return oldOut
}

// commitPointMutableState widens the working OUT-store the transfers built against
// the prior committed OUT-state per key, installs the widened result as the new
// committed OUT, and returns the keys whose committed fact changed. The per-key
// product.Widen keeps the store fixpoint monotone and bounded; the presence-aware
// product.Equal diff drives successor requeue.
func (s *Solution) commitPointMutableState(oldOut pathValueMap, p cfg.Point) []constraint.PathKey {
	if s == nil {
		return nil
	}
	working := s.mutableOut[p]
	if len(oldOut) == 0 && len(working) == 0 {
		return nil
	}
	seen := make(map[constraint.PathKey]struct{}, len(oldOut)+len(working))
	for key := range oldOut {
		seen[key] = struct{}{}
	}
	for key := range working {
		seen[key] = struct{}{}
	}
	var newOut pathValueMap
	if len(seen) > 0 {
		newOut = make(pathValueMap, len(seen))
	}
	var changed []constraint.PathKey
	for key := range seen {
		oldAV, oldOK := oldOut[key]
		curAV, curOK := working[key]
		var nextAV product.AbstractValue
		nextOK := oldOK || curOK
		switch {
		case oldOK && curOK:
			nextAV = product.Widen(oldAV, curAV)
		case curOK:
			nextAV = curAV
		default:
			nextAV = oldAV
		}
		if nextOK {
			newOut[key] = nextAV
		}
		if oldOK != nextOK || (oldOK && nextOK && !product.Equal(oldAV, nextAV)) {
			changed = append(changed, key)
		}
	}
	if len(newOut) == 0 {
		delete(s.mutableOut, p)
		if s.mutablePresence != nil {
			delete(s.mutablePresence, p)
		}
	} else {
		s.mutableOut[p] = newOut
		s.rebuildMutablePresenceForPoint(p)
	}
	s.replaceMutableSuffixesForPoint(p, working, newOut)
	sortPathKeys(changed)
	return changed
}

// clearPointMutableState drops p's committed OUT-state when the point becomes
// unreachable. The OUT lattice top for an unreachable point is empty, so every key
// present in the prior OUT is reported as removed to requeue successors; the
// monotone widen of commitPointMutableState does not apply once reachability flips.
func (s *Solution) clearPointMutableState(oldOut pathValueMap, p cfg.Point) []constraint.PathKey {
	if s == nil {
		return nil
	}
	working := s.mutableOut[p]
	if len(working) > 0 {
		delete(s.mutableOut, p)
		if s.mutablePresence != nil {
			delete(s.mutablePresence, p)
		}
	}
	s.replaceMutableSuffixesForPoint(p, working, nil)
	if len(oldOut) == 0 {
		return nil
	}
	changed := make([]constraint.PathKey, 0, len(oldOut))
	for key := range oldOut {
		changed = append(changed, key)
	}
	sortPathKeys(changed)
	return changed
}

// joinPredecessorMutableState merges the committed OUT-state of each predecessor
// into the incoming IN-state for p. The per-key merge is product.Join, the
// component-wise least upper bound, so the presence axis survives (Present and
// Absent are incomparable and join to Maybe) and the store stays product-native
// end-to-end with no typ.Type round-trip. It reads only mutableOut[pred]; when p is
// its own predecessor over a back-edge, that read naturally observes p's prior
// committed OUT (the working store is installed after the join).
func (s *Solution) joinPredecessorMutableState(p cfg.Point) pathValueMap {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || len(s.mutableOut) == 0 {
		return nil
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return nil
	}

	var out pathValueMap
	if len(preds) == 1 {
		// Single-predecessor edges carry the predecessor OUT-state verbatim.
		out = cloneValueMap(s.mutableOut[preds[0]])
	} else {
		keySet := make(map[constraint.PathKey]struct{})
		for _, pred := range preds {
			for key := range s.mutableOut[pred] {
				keySet[key] = struct{}{}
			}
		}
		if len(keySet) > 0 {
			out = make(pathValueMap, len(keySet))
			keys := make([]constraint.PathKey, 0, len(keySet))
			for key := range keySet {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
			for _, key := range keys {
				var merged product.AbstractValue
				have := false
				for _, pred := range preds {
					next, ok := s.predMutableValue(pred, key)
					if !ok {
						continue
					}
					if !have {
						merged = next
						have = true
						continue
					}
					merged = product.Join(merged, next)
				}
				if !have {
					continue
				}
				out[key] = merged
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// predMutableValue returns pred's contributed carrier for key. A key stored
// directly in pred's committed OUT contributes its carrier verbatim (no round
// trip). A key absent from pred's OUT contributes the canonical fact derived from
// pred's predecessor/global state; a presence key with no fact on pred's path
// contributes Absent (nil) so a present/absent split joins to Maybe.
func (s *Solution) predMutableValue(pred cfg.Point, pathKey constraint.PathKey) (product.AbstractValue, bool) {
	if state := s.mutableOut[pred]; state != nil {
		if av, ok := state[pathKey]; ok {
			return av, true
		}
	}
	key := string(pathKey)
	t := s.canonicalKeyTypeAt(pred, key)
	if t == nil && isPresenceKey(key) {
		t = typ.Nil
	}
	if t == nil || typ.IsNever(t) {
		return product.AbstractValue{}, false
	}
	return liftFlowValue(t), true
}

func (s *Solution) canonicalKeyTypeAt(p cfg.Point, key string) typ.Type {
	if s == nil || key == "" {
		return nil
	}
	pathKey := constraint.PathKey(key)
	if state := s.mutableOut[p]; state != nil {
		if av, ok := state[pathKey]; ok {
			return s.projectPresenceAtPoint(p, key, projectFlowValue(av))
		}
	}
	if av, ok := s.values[pathKey]; ok {
		return s.projectPresenceAtPoint(p, key, projectFlowValue(av))
	}
	if s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil {
		return nil
	}
	sym, version, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(key))
	if !ok || sym == 0 || version == 0 {
		return nil
	}
	visible := s.inputs.Graph.VisibleVersion(p, sym)
	if visible.ID != version {
		return nil
	}
	path := constraint.Path{Root: visible.Root, Symbol: sym, Version: version}
	if suffix == "" {
		return s.lookupDeclaredType(path)
	}
	suffixKey, ok := newPathSuffixKey(suffix)
	if !ok {
		return nil
	}
	segments := s.parseSuffixCached(suffixKey)
	if segments == nil {
		return nil
	}
	baseKey := s.pkResolver.KeyAtVersion(sym, version, nil)
	var base typ.Type
	if state := s.mutableOut[p]; state != nil {
		if av, ok := state[baseKey]; ok {
			base = projectFlowValue(av)
		}
	}
	if base == nil {
		if av, ok := s.values[baseKey]; ok {
			base = projectFlowValue(av)
		}
	}
	if base != nil && typ.IsUnknown(base) {
		if declared := s.lookupDeclaredType(path); declared != nil && !typ.IsUnknown(declared) {
			base = declared
		}
	}
	if base == nil {
		base = s.lookupDeclaredType(path)
	}
	if base == nil {
		return nil
	}
	derived, ok := deriveTypeFrom(s.resolver, base, segments)
	if !ok {
		return nil
	}
	return s.projectPresenceAtPoint(p, key, derived)
}

// valueAtPoint returns the stored abstract-state fact for key at p, projected to
// a typ.Type at the egress boundary for the typ.Type transfer math and queries.
func (s *Solution) valueAtPoint(p cfg.Point, key string) typ.Type {
	if s == nil || key == "" {
		return nil
	}
	pathKey := constraint.PathKey(key)
	if state := s.mutableOut[p]; state != nil {
		if av, ok := state[pathKey]; ok {
			return projectFlowValue(av)
		}
	}
	if av, ok := s.values[pathKey]; ok {
		return projectFlowValue(av)
	}
	return nil
}

// abstractValueAt returns the raw stored carrier for key at p without egress
// projection. It is the read used by the worklist no-op detection so convergence
// compares product identity rather than a projected typ.Type.
func (s *Solution) abstractValueAt(p cfg.Point, key string) (product.AbstractValue, bool) {
	if s == nil || key == "" {
		return product.AbstractValue{}, false
	}
	pathKey := constraint.PathKey(key)
	if state := s.mutableOut[p]; state != nil {
		if av, ok := state[pathKey]; ok {
			return av, true
		}
	}
	av, ok := s.values[pathKey]
	return av, ok
}

// baseSymbolValue returns the stored fact for a base-symbol key from the global
// values store, the home a base-symbol assignment writes through setValue. It
// bypasses the point-mutable overlay so a base symbol's self-convergence compares
// against the same store its write targets, immune to a mutator-widened shadow the
// overlay may carry for a prior incarnation of the same key.
func (s *Solution) baseSymbolValue(key string) typ.Type {
	if s == nil || key == "" {
		return nil
	}
	if av, ok := s.values[constraint.PathKey(key)]; ok {
		return projectFlowValue(av)
	}
	return nil
}

// overwriteMutableShadow replaces a base key's point-mutable overlay entry at p
// with the freshly assigned fact when the overlay shadows it. A base-symbol
// redefinition supersedes any value the overlay carried into p (a mutator-widened
// prior incarnation propagated across a loop back-edge), so the egress read and
// successor joins observe the live value rather than the dead one.
func (s *Solution) overwriteMutableShadow(p cfg.Point, key string, t typ.Type) {
	if s == nil || key == "" || t == nil {
		return
	}
	state := s.mutableOut[p]
	if state == nil {
		return
	}
	if _, ok := state[constraint.PathKey(key)]; !ok {
		return
	}
	s.storeMutableValue(p, key, liftFlowValue(t), t)
}

func (s *Solution) setMutableValue(p cfg.Point, key string, t typ.Type) {
	if s == nil || key == "" || t == nil {
		return
	}
	// Admission boundary for the point-sensitive mutable store; bookkeeping uses
	// the caller's typ.Type directly.
	s.storeMutableValue(p, key, liftFlowValue(t), t)
}

// setMutableValueAV stores an already-built carrier at a point. It is the native
// ingress for the phi field-suffix merge so a product.Join/Widen result is stored
// without a project-then-relift round trip (which would break product identity).
func (s *Solution) setMutableValueAV(p cfg.Point, key string, av product.AbstractValue) {
	if s == nil || key == "" || av.IsZero() {
		return
	}
	s.storeMutableValue(p, key, av, projectFlowValue(av))
}

func (s *Solution) storeMutableValue(p cfg.Point, key string, av product.AbstractValue, t typ.Type) {
	if s.mutableOut == nil {
		s.mutableOut = make(map[cfg.Point]pathValueMap)
	}
	state := s.mutableOut[p]
	if state == nil {
		state = make(pathValueMap, 1)
		s.mutableOut[p] = state
	}
	pathKey := constraint.PathKey(key)
	state[pathKey] = av
	if s.mutableSuffixIndexed == nil {
		s.mutableSuffixIndexed = make(map[cfg.Point]bool, 1)
	}
	s.mutableSuffixIndexed[p] = true
	s.setMutablePresence(p, key, t)
	s.indexMutableSuffix(p, pathKey)
	s.invalidateQueryCachesForPointWrite(p, key)
}

func (s *Solution) clearMutableDescendantsAtPoint(p cfg.Point, baseKey string) []constraint.PathKey {
	if s == nil || baseKey == "" {
		return nil
	}
	state := s.mutableOut[p]
	if len(state) == 0 {
		return nil
	}
	var changed []constraint.PathKey
	baseLen := len(baseKey)
	for key := range state {
		keyString := string(key)
		if len(keyString) <= baseLen || keyString[:baseLen] != baseKey {
			continue
		}
		suffix := keyString[baseLen:]
		if suffix == "" || (suffix[0] != '.' && suffix[0] != '[') {
			continue
		}
		delete(state, key)
		if s.mutablePresence != nil {
			if presence := s.mutablePresence[p]; presence != nil {
				delete(presence, constraint.PathKey(key))
				if len(presence) == 0 {
					delete(s.mutablePresence, p)
				}
			}
		}
		s.removeMutableSuffix(p, key)
		changed = append(changed, key)
	}
	if len(changed) > 0 {
		s.invalidateQueryCachesForPointWrite(p, baseKey)
		sortPathKeys(changed)
	}
	return changed
}

func (s *Solution) invalidateQueryCachesForPointWrite(_ cfg.Point, key string) {
	if s == nil {
		return
	}
	s.bumpStateEpoch()
	s.invalidateReachabilityForWrite(key)
	if s.narrowedTypeCache != nil {
		clear(s.narrowedTypeCache)
	}
	if s.childTypesCache != nil {
		clear(s.childTypesCache)
	}
	if s.fieldOverlayCache != nil {
		clear(s.fieldOverlayCache)
	}
	if s.fieldMergeCache != nil {
		clear(s.fieldMergeCache)
	}
}

func cloneValueMap(in pathValueMap) pathValueMap {
	if len(in) == 0 {
		return nil
	}
	out := make(pathValueMap, len(in))
	for key, av := range in {
		out[key] = av
	}
	return out
}

// projectFlowValue is the egress boundary that recovers the typ.Type a stored
// AbstractValue carries. The zero carrier (an absent map slot) projects to nil so
// callers treat it as "no fact", matching the prior typ.Type map read.
func projectFlowValue(av product.AbstractValue) typ.Type {
	if av.IsZero() {
		return nil
	}
	return av.ProjectValue()
}
