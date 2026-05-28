package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/typ"
)

// fieldOverlayCacheKey identifies root overlays in the point-sensitive mutable store.
type fieldOverlayCacheKey struct {
	point cfg.Point
	root  string
}

type fieldMergeCacheKey struct {
	point cfg.Point
	root  string
	base  typ.Type
}

type pointRootKey struct {
	point cfg.Point
	root  string
}

func (s *Solution) ensureValueSuffixIndex() {
	if s == nil || s.valueSuffixIndex != nil {
		return
	}
	s.valueSuffixIndex = make(map[string][]string)
	for key := range s.values {
		s.indexValueSuffix(key)
	}
}

func (s *Solution) indexValueSuffix(key string) {
	root, suffix, ok := indexedPathSuffix(key)
	if !ok {
		return
	}
	if s.valueSuffixIndex == nil {
		s.valueSuffixIndex = make(map[string][]string, 1)
	}
	s.valueSuffixIndex[root] = insertSortedSuffix(s.valueSuffixIndex[root], suffix)
}

func (s *Solution) removeValueSuffix(key string) {
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
	state := s.mutableValues[p]
	s.replaceMutableSuffixesForPoint(p, nil, state)
}

func (s *Solution) replaceMutableSuffixesForPoint(p cfg.Point, old, next map[string]product.AbstractValue) {
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
		s.mutableSuffixIndex = make(map[pointRootKey][]string, len(next))
	}
	for key := range next {
		s.indexMutableSuffix(p, key)
	}
}

func (s *Solution) indexMutableSuffix(p cfg.Point, key string) {
	root, suffix, ok := indexedPathSuffix(key)
	if !ok {
		return
	}
	if s.mutableSuffixIndex == nil {
		s.mutableSuffixIndex = make(map[pointRootKey][]string, 1)
	}
	indexKey := pointRootKey{point: p, root: root}
	s.mutableSuffixIndex[indexKey] = insertSortedSuffix(s.mutableSuffixIndex[indexKey], suffix)
}

func (s *Solution) removeMutableSuffix(p cfg.Point, key string) {
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

func indexedPathSuffix(key string) (string, string, bool) {
	root, suffix, ok := pathkey.ParseRootAndSuffix(constraint.PathKey(key))
	if !ok || root == "" || suffix == "" {
		return "", "", false
	}
	if suffix[0] != '.' && suffix[0] != '[' {
		return "", "", false
	}
	return root, suffix, true
}

func insertSortedSuffix(values []string, suffix string) []string {
	i := sort.SearchStrings(values, suffix)
	if i < len(values) && values[i] == suffix {
		return values
	}
	values = append(values, "")
	copy(values[i+1:], values[i:])
	values[i] = suffix
	return values
}

func removeSortedSuffix(values []string, suffix string) []string {
	i := sort.SearchStrings(values, suffix)
	if i >= len(values) || values[i] != suffix {
		return values
	}
	copy(values[i:], values[i+1:])
	return values[:len(values)-1]
}

// beginPointMutableState installs the incoming mutable store for p and returns
// the previous post-state. Transfer functions then mutate s.mutableValues[p];
// processPointReturnChangedKeys diffs the final post-state against the old one.
func (s *Solution) beginPointMutableState(p cfg.Point) map[string]product.AbstractValue {
	if s == nil {
		return nil
	}
	if s.mutableValues == nil {
		s.mutableValues = make(map[cfg.Point]map[string]product.AbstractValue)
	}
	old := cloneValueMap(s.mutableValues[p])
	s.seedPointMutableSelfCarry(p, old)
	incoming := s.joinPredecessorMutableState(p)
	if sameFlowValueMap(old, incoming) {
		return old
	}
	if len(incoming) == 0 {
		delete(s.mutableValues, p)
		if s.mutablePresence != nil {
			delete(s.mutablePresence, p)
		}
	} else {
		s.mutableValues[p] = incoming
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
	s.replaceMutableSuffixesForPoint(p, old, incoming)
	return old
}

func (s *Solution) mutableStateChangedKeys(old map[string]product.AbstractValue, p cfg.Point) []string {
	if s == nil {
		return nil
	}
	current := s.mutableValues[p]
	if len(old) == 0 && len(current) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(old)+len(current))
	for key := range old {
		seen[key] = struct{}{}
	}
	for key := range current {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		oldAV, oldOK := old[key]
		curAV, curOK := current[key]
		changed := oldOK != curOK || !oldAV.Equal(curAV) ||
			pathPresenceFromType(projectFlowValue(oldAV)) != pathPresenceFromType(projectFlowValue(curAV))
		if !changed {
			continue
		}
		if s.isStableCarryMembershipFlap(p, key, oldAV, oldOK, curAV, curOK) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// isStableCarryMembershipFlap reports whether the change for key at p is only the
// loop-carried slot flapping present/absent at a stable value. The public predecessor
// join drops a self write-back slot whose value lives only in p's own post-state, so
// the public store toggles the key's membership across iterations even though the
// value never moves. Propagating that membership flap re-queues the SCC forever; the
// private carry holds the stable value, so a membership-only delta whose present side
// equals the carry is not a public value change and must not drive propagation. A real
// value change (present side differs from the carry) still propagates, keeping the
// fixpoint sound. The base slot stays addressable via s.values, so suppressing the
// redundant mutable-store entry does not lose a path fact.
func (s *Solution) isStableCarryMembershipFlap(p cfg.Point, key string, oldAV product.AbstractValue, oldOK bool, curAV product.AbstractValue, curOK bool) bool {
	if s == nil || s.mutableSelfCarry == nil || oldOK == curOK {
		return false
	}
	carry := s.mutableSelfCarry[p]
	if carry == nil {
		return false
	}
	carried, ok := carry[key]
	if !ok || carried.IsZero() {
		return false
	}
	present := curAV
	if !curOK {
		present = oldAV
	}
	return present.Equal(carried)
}

// joinPredecessorMutableState merges the post-state of each predecessor into the
// incoming abstract state for p. The per-key merge is product.CarryForward (the
// value-domain convergence-widening merge lifted onto AbstractValue), so the store
// fixpoint compares product identity and converges on recursive families.
//
// The result is the public incoming post-state and nothing else: every key comes
// from a predecessor edge. Keys whose value lives only in p's own post-state (a
// for..pairs self write-back, t[k] = v on a loop-carried map) are intentionally
// dropped here - their loop-carried presence is preserved separately by the private
// mutableSelfCarry seed (see seedPointMutableSelfCarry), so the public projection
// never observes the loop-widened slot, and the mutator still converges.
func (s *Solution) joinPredecessorMutableState(p cfg.Point) map[string]product.AbstractValue {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || len(s.mutableValues) == 0 {
		return nil
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return nil
	}

	var out map[string]product.AbstractValue
	if len(preds) == 1 {
		// Single-predecessor edges carry the predecessor post-state verbatim.
		out = cloneValueMap(s.mutableValues[preds[0]])
	} else {
		keySet := make(map[string]struct{})
		for _, pred := range preds {
			for key := range s.mutableValues[pred] {
				keySet[key] = struct{}{}
			}
		}
		if len(keySet) > 0 {
			out = make(map[string]product.AbstractValue, len(keySet))
			keys := make([]string, 0, len(keySet))
			for key := range keySet {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				var merged product.AbstractValue
				have := false
				for _, pred := range preds {
					t := s.canonicalKeyTypeAt(pred, key)
					if t == nil && isPresenceKey(key) {
						t = typ.Nil
					}
					if t == nil || typ.IsNever(t) {
						continue
					}
					next := liftFlowValue(t)
					if !have {
						merged = next
						have = true
						continue
					}
					merged = product.CarryForward(merged, next)
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

// seedPointMutableSelfCarry records the prior self-written post-state of p into the
// private termination-only carry. The keys are exactly the loop-carried slots whose
// value lives only in p's own post-state (a for..pairs self write-back, t[k] = v on
// a loop-carried map); they are dropped by the public predecessor join, so without a
// seed the mutator re-admits each iteration from nil and the key toggles present/
// absent forever. The carry is consumed only by the mutator transfer (mutableSelfCarryAt)
// as a widening/no-op seed - it never reaches a public projection or interproc summary.
func (s *Solution) seedPointMutableSelfCarry(p cfg.Point, prior map[string]product.AbstractValue) {
	if s == nil {
		return
	}
	if len(prior) == 0 {
		if s.mutableSelfCarry != nil {
			delete(s.mutableSelfCarry, p)
		}
		return
	}
	written := s.selfWrittenMutableKeys(p)
	if len(written) == 0 {
		if s.mutableSelfCarry != nil {
			delete(s.mutableSelfCarry, p)
		}
		return
	}
	var carry map[string]product.AbstractValue
	for key := range written {
		av, ok := prior[key]
		if !ok {
			continue
		}
		if carry == nil {
			carry = make(map[string]product.AbstractValue, len(written))
		}
		carry[key] = av
	}
	if len(carry) == 0 {
		if s.mutableSelfCarry != nil {
			delete(s.mutableSelfCarry, p)
		}
		return
	}
	if s.mutableSelfCarry == nil {
		s.mutableSelfCarry = make(map[cfg.Point]map[string]product.AbstractValue, 1)
	}
	s.mutableSelfCarry[p] = carry
}

// mutableSelfCarryAt returns the private termination-only carry value for key at p,
// projected to a typ.Type. It is the loop-carry seed the mutator reads when the
// public slot is absent so the self write-back converges. No public projection or
// interproc summary calls this.
func (s *Solution) mutableSelfCarryAt(p cfg.Point, key string) typ.Type {
	if s == nil || key == "" || s.mutableSelfCarry == nil {
		return nil
	}
	carry := s.mutableSelfCarry[p]
	if carry == nil {
		return nil
	}
	av, ok := carry[key]
	if !ok {
		return nil
	}
	return projectFlowValue(av)
}

// selfWrittenMutableKeys returns the mutable-store keys that the point's own
// transfer writes back into p's post-state: map writes (t[k] = v), table/container
// element widening, and structured field assignments. These are the keys whose
// value can live only in p's post-state on a loop self write-back, so they need
// monotone presence retention across the join to converge.
func (s *Solution) selfWrittenMutableKeys(p cfg.Point) map[string]struct{} {
	if s == nil || s.pkResolver == nil {
		return nil
	}
	keys := make(map[string]struct{})
	add := func(target constraint.Path) {
		if target.Symbol == 0 {
			return
		}
		if key := s.pkResolver.KeyAt(p, target); key != "" {
			keys[string(key)] = struct{}{}
		}
	}
	for _, mm := range s.mapMutatorAssignmentsAt(p) {
		add(mm.Target)
	}
	for _, tm := range s.tableMutatorAssignmentsAt(p) {
		add(tm.Target)
	}
	for _, cm := range s.containerMutatorAssignmentsAt(p) {
		add(cm.Target)
	}
	for _, assign := range s.assignmentsAt(p) {
		if len(assign.TargetPath.Segments) > 0 {
			add(assign.TargetPath)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

func (s *Solution) canonicalKeyTypeAt(p cfg.Point, key string) typ.Type {
	if s == nil || key == "" {
		return nil
	}
	if state := s.mutableValues[p]; state != nil {
		if av, ok := state[key]; ok {
			return s.projectPresenceAtPoint(p, key, projectFlowValue(av))
		}
	}
	if av, ok := s.values[key]; ok {
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
	segments := s.parseSuffixCached(suffix)
	if segments == nil {
		return nil
	}
	baseKey := s.pkResolver.KeyAtVersion(sym, version, nil)
	var base typ.Type
	if state := s.mutableValues[p]; state != nil {
		if av, ok := state[string(baseKey)]; ok {
			base = projectFlowValue(av)
		}
	}
	if base == nil {
		if av, ok := s.values[string(baseKey)]; ok {
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
	if state := s.mutableValues[p]; state != nil {
		if av, ok := state[key]; ok {
			return projectFlowValue(av)
		}
	}
	if av, ok := s.values[key]; ok {
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
	if state := s.mutableValues[p]; state != nil {
		if av, ok := state[key]; ok {
			return av, true
		}
	}
	av, ok := s.values[key]
	return av, ok
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
	if s.mutableValues == nil {
		s.mutableValues = make(map[cfg.Point]map[string]product.AbstractValue)
	}
	state := s.mutableValues[p]
	if state == nil {
		state = make(map[string]product.AbstractValue, 1)
		s.mutableValues[p] = state
	}
	state[key] = av
	if s.mutableSuffixIndexed == nil {
		s.mutableSuffixIndexed = make(map[cfg.Point]bool, 1)
	}
	s.mutableSuffixIndexed[p] = true
	s.setMutablePresence(p, key, t)
	s.indexMutableSuffix(p, key)
	s.invalidateQueryCachesForPointWrite(p, key)
}

func (s *Solution) clearMutableDescendantsAtPoint(p cfg.Point, baseKey string) []string {
	if s == nil || baseKey == "" {
		return nil
	}
	state := s.mutableValues[p]
	if len(state) == 0 {
		return nil
	}
	var changed []string
	baseLen := len(baseKey)
	for key := range state {
		if len(key) <= baseLen || key[:baseLen] != baseKey {
			continue
		}
		suffix := key[baseLen:]
		if suffix == "" || (suffix[0] != '.' && suffix[0] != '[') {
			continue
		}
		delete(state, key)
		if s.mutablePresence != nil {
			if presence := s.mutablePresence[p]; presence != nil {
				delete(presence, key)
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
		sort.Strings(changed)
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

func cloneValueMap(in map[string]product.AbstractValue) map[string]product.AbstractValue {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]product.AbstractValue, len(in))
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
