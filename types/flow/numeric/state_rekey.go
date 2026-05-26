package numeric

import "github.com/wippyai/go-lua/types/constraint"

func rekeyState(s *State, remap map[constraint.PathKey]constraint.PathKey) *State {
	result := &State{
		bounds:    makeIntervalMap(len(s.bounds)),
		modular:   makeModularMap(len(s.modular)),
		relations: makeRelationMap(len(s.relations)),
		lenRefs:   makeLenRefMap(len(s.lenRefs)),
		lenBounds: makeIntervalMap(len(s.lenBounds)),
	}
	if !rekeyIntervalsInto(result.bounds, s.bounds, remap) {
		return Bottom()
	}
	if !rekeyModularInto(result.modular, s.modular, remap) {
		return Bottom()
	}
	rekeyRelationsInto(result.relations, s.relations, remap)
	rekeyLenRefsInto(result.lenRefs, s.lenRefs, remap)
	if !rekeyIntervalsInto(result.lenBounds, s.lenBounds, remap) {
		return Bottom()
	}
	return result
}

func makeIntervalMap(size int) map[constraint.PathKey]Interval {
	if size == 0 {
		return nil
	}
	return make(map[constraint.PathKey]Interval, size)
}

func makeModularMap(size int) map[constraint.PathKey]ModResidue {
	if size == 0 {
		return nil
	}
	return make(map[constraint.PathKey]ModResidue, size)
}

func makeRelationMap(size int) map[relationKey]int64 {
	if size == 0 {
		return nil
	}
	return make(map[relationKey]int64, size)
}

func makeLenRefMap(size int) map[constraint.PathKey]lenRefBound {
	if size == 0 {
		return nil
	}
	return make(map[constraint.PathKey]lenRefBound, size)
}

func rekeyIntervalsInto(
	out map[constraint.PathKey]Interval,
	in map[constraint.PathKey]Interval,
	remap map[constraint.PathKey]constraint.PathKey,
) bool {
	for _, key := range constraint.SortedPathKeys(in) {
		mapped := rekeyPath(key, remap)
		interval := in[key]
		if existing, ok := out[mapped]; ok {
			interval = intersectIntervals(existing, interval)
			if interval.Lower > interval.Upper {
				return false
			}
		}
		out[mapped] = interval
	}
	return true
}

func rekeyModularInto(
	out map[constraint.PathKey]ModResidue,
	in map[constraint.PathKey]ModResidue,
	remap map[constraint.PathKey]constraint.PathKey,
) bool {
	for _, key := range constraint.SortedPathKeys(in) {
		mapped := rekeyPath(key, remap)
		residue := in[key]
		if existing, ok := out[mapped]; ok && existing != residue {
			return false
		}
		out[mapped] = residue
	}
	return true
}

func rekeyRelationsInto(
	out map[relationKey]int64,
	in map[relationKey]int64,
	remap map[constraint.PathKey]constraint.PathKey,
) {
	for _, rel := range sortedRelationKeys(in) {
		mapped := relationKey{
			X: rekeyPath(rel.X, remap),
			Y: rekeyPath(rel.Y, remap),
		}
		bound := in[rel]
		if existing, ok := out[mapped]; ok {
			bound = minInt64(existing, bound)
		}
		out[mapped] = bound
	}
}

func rekeyLenRefsInto(
	out map[constraint.PathKey]lenRefBound,
	in map[constraint.PathKey]lenRefBound,
	remap map[constraint.PathKey]constraint.PathKey,
) {
	for _, key := range constraint.SortedPathKeys(in) {
		mappedKey := rekeyPath(key, remap)
		ref := in[key]
		ref.Array = rekeyPath(ref.Array, remap)
		if existing, ok := out[mappedKey]; ok && existing != ref {
			delete(out, mappedKey)
			continue
		}
		out[mappedKey] = ref
	}
}

func rekeyPath(key constraint.PathKey, remap map[constraint.PathKey]constraint.PathKey) constraint.PathKey {
	if mapped, ok := remap[key]; ok {
		return mapped
	}
	return key
}
