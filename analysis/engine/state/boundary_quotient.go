package state

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// boundaryInverseQuotient is the single inverse authority for one boundary
// substitution. Forward rekeying alone is insufficient when several source
// subjects coalesce: may facts union, but a must fact is valid at the quotient
// only when every source preimage proves it. The authority is built once from
// the complete boundary closure and shared by every lane.
type boundaryInverseQuotient struct {
	paths              map[keyspace.Key][]keyspace.Key
	stateKeys          map[pathaddr.StateKey][]pathaddr.StateKey
	slots              map[key.Value][]key.Value
	identities         map[identity.Term][]identity.Term
	allIdentities      bool
	structuralIdentity bool
	identityStructural bool
	// inverseRoots is a sealed lazy structural inverse used by formal rekey.
	// It preserves descendant suffixes and many-to-one root fibers without
	// enumerating every path in an ordinary lane before applying its must law.
	inverseFrom  *keyspace.KeySpace
	inverseTo    *keyspace.KeySpace
	inverseRoots boundaryPathMap
}

func buildBoundaryInverseQuotient(
	fromKeys, toKeys *keyspace.KeySpace,
	closure BoundaryClosure,
	roots boundaryPathMap,
	slots map[key.Value][]key.Value,
	lens *BoundaryAllocationAuthority,
) (boundaryInverseQuotient, bool) {
	q := boundaryInverseQuotient{
		paths:         make(map[keyspace.Key][]keyspace.Key, len(closure.paths)),
		stateKeys:     make(map[pathaddr.StateKey][]pathaddr.StateKey, len(closure.paths)),
		slots:         make(map[key.Value][]key.Value, len(closure.slots)),
		identities:    make(map[identity.Term][]identity.Term, len(closure.identities)),
		allIdentities: closure.allIdentities,
		// A quotient is complete only with its sealed structural denominator.
		// Keeping this relation constructor-owned prevents runtime transport and
		// static footprint discovery from observing different inverse fibers when
		// the eager closure currently materializes only one side of a collision.
		inverseFrom: toKeys, inverseTo: fromKeys,
		inverseRoots: invertBoundaryPathMap(roots),
	}
	for source := range closure.paths {
		targets, ok := rebaseBoundaryPaths(fromKeys, toKeys, roots, source)
		if !ok || len(targets) == 0 {
			return boundaryInverseQuotient{}, false
		}
		for _, target := range targets {
			q.paths[target] = append(q.paths[target], source)
		}
	}
	for source := range closure.slots {
		targets := slots[source]
		if len(targets) == 0 {
			return boundaryInverseQuotient{}, false
		}
		for _, target := range targets {
			q.slots[target] = append(q.slots[target], source)
		}
	}
	for source := range closure.identities {
		target, ok := rebaseBoundaryIdentity(lens, source)
		if !ok || !target.Valid() {
			return boundaryInverseQuotient{}, false
		}
		q.identities[target] = append(q.identities[target], source)
	}
	for target, sources := range q.paths {
		sort.Slice(sources, func(i, j int) bool { return fromKeys.Less(sources[i], sources[j]) })
		sources = compactComparable(sources)
		q.paths[target] = sources
		targetState, valid := pathaddr.StateKeyFromPathKey(toKeys.FormatReadOnly(target))
		if !valid {
			continue
		}
		stateSources := make([]pathaddr.StateKey, len(sources))
		for i, source := range sources {
			stateSources[i], valid = pathaddr.StateKeyFromPathKey(fromKeys.FormatReadOnly(source))
			if !valid {
				return boundaryInverseQuotient{}, false
			}
		}
		q.stateKeys[targetState] = stateSources
	}
	for target, sources := range q.slots {
		sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
		q.slots[target] = compactComparable(sources)
	}
	for target, sources := range q.identities {
		if concrete, ok := target.Concrete(); lens != nil && ok {
			sources = append(sources, lens.preimages[concrete]...)
		}
		if closure.allIdentities {
			found := false
			for _, source := range sources {
				found = found || source == target
			}
			if !found {
				sources = append(sources, target)
			}
		}
		sort.Slice(sources, func(i, j int) bool { return identityTermLess(sources[i], sources[j]) })
		q.identities[target] = compactComparable(sources)
	}
	return q, true
}

func compactComparable[T comparable](in []T) []T {
	out := in[:0]
	for _, value := range in {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func (q boundaryInverseQuotient) pathPreimages(target keyspace.Key) ([]keyspace.Key, bool) {
	if q.structuralIdentity && target.Kind != keyspace.KindInvalid {
		return []keyspace.Key{target}, true
	}
	sources := q.paths[target]
	// The eager map is already complete for the overwhelmingly common
	// one-root case. Inspecting the sealed root relation is allocation-free and
	// avoids rebuilding descendant keys on every coordinate query; only a
	// genuinely underfilled many-root fiber takes the lazy materialization.
	sealedUpperBound := boundaryInverseRootPreimageUpperBound(q.inverseFrom, q.inverseRoots, target)
	if len(sources) < sealedUpperBound && q.inverseFrom != nil && q.inverseTo != nil {
		sealed, _ := rebaseBoundaryPaths(q.inverseFrom, q.inverseTo, q.inverseRoots, target)
		if len(sealed) != 0 {
			// paths is the eager image of the currently materialized closure;
			// inverseRoots is the sealed structural denominator. The latter is
			// not a fallback: a must coordinate must retain every static root
			// preimage even when one of them already appears in the eager map.
			complete := true
			for _, candidate := range sealed {
				present := false
				for _, source := range sources {
					if source == candidate {
						present = true
						break
					}
				}
				if !present {
					complete = false
					break
				}
			}
			if !complete {
				sources = append(append([]keyspace.Key(nil), sources...), sealed...)
				sort.Slice(sources, func(i, j int) bool { return q.inverseTo.Less(sources[i], sources[j]) })
				sources = compactComparable(sources)
			} else if len(sources) == 0 {
				sources = sealed
			}
		}
	}
	return sources, len(sources) != 0
}

func boundaryInverseRootPreimageUpperBound(keys *keyspace.KeySpace, roots boundaryPathMap, target keyspace.Key) int {
	if keys == nil || !keys.Valid() || target.Kind == keyspace.KindInvalid {
		return 0
	}
	selectedDepth, count := -1, 0
	var selectedRoot keyspace.Key
	for _, binding := range roots {
		if binding.from.Kind == keyspace.KindInvalid && binding.to.Kind == keyspace.KindInvalid {
			continue
		}
		depth, valid := keys.SegmentLen(binding.from)
		if !valid || target != binding.from && !keys.HasPrefix(target, binding.from) {
			continue
		}
		switch {
		case depth > selectedDepth:
			selectedDepth, selectedRoot, count = depth, binding.from, 1
		case depth == selectedDepth && binding.from == selectedRoot:
			count++
		}
	}
	return count
}

func (q boundaryInverseQuotient) optionalPathPreimages(target keyspace.Key) ([]keyspace.Key, bool) {
	if target.Kind == keyspace.KindInvalid {
		return []keyspace.Key{{}}, true
	}
	return q.pathPreimages(target)
}

func (q boundaryInverseQuotient) stateKeyPreimages(target pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
	if q.structuralIdentity && target != "" {
		return []pathaddr.StateKey{target}, true
	}
	sources := q.stateKeys[target]
	if len(sources) == 0 && q.inverseFrom != nil && q.inverseTo != nil && target != "" {
		path, ok := q.inverseFrom.FromStateKey(pathdom.PathKey(target.String()))
		if !ok {
			return nil, false
		}
		paths, ok := q.pathPreimages(path)
		if !ok {
			return nil, false
		}
		sources = make([]pathaddr.StateKey, len(paths))
		for index, source := range paths {
			sources[index], ok = pathaddr.StateKeyFromPathKey(q.inverseTo.FormatReadOnly(source))
			if !ok {
				return nil, false
			}
		}
	}
	return sources, len(sources) != 0
}

func (q boundaryInverseQuotient) optionalStateKeyPreimages(target pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
	if target == "" {
		return []pathaddr.StateKey{""}, true
	}
	return q.stateKeyPreimages(target)
}

func (q boundaryInverseQuotient) identityPreimages(target identity.Term) ([]identity.Term, bool) {
	if !target.Valid() {
		return []identity.Term{{}}, true
	}
	if q.identityStructural {
		return []identity.Term{target}, true
	}
	sources := q.identities[target]
	// allIdentities denotes every concrete stable identity, including a prior
	// instance of an allocation site that was not explicitly enumerated by the
	// closure. Since non-template identities rebase to themselves, this is its
	// exact additional preimage. Templates never self-map.
	if len(sources) == 0 && q.allIdentities {
		return []identity.Term{target}, true
	}
	return sources, len(sources) != 0
}

type boundaryPair[A, B comparable] struct {
	first  A
	second B
}

type boundaryTriple[A, B, C comparable] struct {
	first  A
	second B
	third  C
}

func boundaryPairs[A, B comparable](as []A, bs []B) []boundaryPair[A, B] {
	out := make([]boundaryPair[A, B], 0, len(as)*len(bs))
	for _, a := range as {
		for _, b := range bs {
			out = append(out, boundaryPair[A, B]{first: a, second: b})
		}
	}
	return out
}

func boundaryTriples[A, B, C comparable](as []A, bs []B, cs []C) []boundaryTriple[A, B, C] {
	out := make([]boundaryTriple[A, B, C], 0, len(as)*len(bs)*len(cs))
	for _, a := range as {
		for _, b := range bs {
			for _, c := range cs {
				out = append(out, boundaryTriple[A, B, C]{first: a, second: b, third: c})
			}
		}
	}
	return out
}

func rebaseBoundaryMustSet[S, D, F comparable](
	source map[S]struct{},
	image func(S) ([]D, bool),
	sourceFiber func(S) F,
	inverseFiber func(D) ([]F, bool),
) (map[D]struct{}, bool) {
	return lift.QuotientMustSet(source, image, sourceFiber, inverseFiber)
}

func rebaseBoundaryMustMap[S, D, F comparable, V any](
	source map[S]V,
	imageKey func(S) ([]D, bool),
	imageValue func(V) (V, bool),
	sourceFiber func(S) F,
	inverseFiber func(D) ([]F, bool),
	join func(V, V) V,
) (map[D]V, bool) {
	return lift.QuotientMustMap(source, imageKey, imageValue, sourceFiber, inverseFiber, join)
}
