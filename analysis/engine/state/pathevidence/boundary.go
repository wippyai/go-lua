package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ProjectBoundary returns the exact finite facts selected by touches while
// preserving each must sublane's independent Bottom/Top spelling.
func (l Lane) ProjectBoundary(ks *keyspace.KeySpace, touches func(keyspace.Key) bool, projectValue func(product.Value) product.Value) Lane {
	out := Lane{
		refinementsBottom: l.refinementsBottom, staticMembersBottom: l.staticMembersBottom,
		proofsBottom: l.proofsBottom, pathPresenceImplicationsBottom: l.pathPresenceImplicationsBottom,
	}
	if !l.refinementsBottom {
		out.refinements = filterHandleValueMap(l.refinements, ks, touches, projectValue)
	}
	if !l.staticMembersBottom {
		out.staticMembers = filterHandleValueMap(l.staticMembers, ks, touches, projectValue)
	}
	if !l.proofsBottom {
		for proof := range l.proofs {
			if !touches(proof.Path) && !touches(proof.Other) {
				continue
			}
			if out.proofs == nil {
				out.proofs = make(map[BranchProof]struct{})
			}
			out.proofs[proof] = struct{}{}
			out.equalityRootMask.merge(equalityProofRootMask(proof))
		}
	}
	if !l.pathPresenceImplicationsBottom {
		for value := range l.pathPresenceImplications {
			if !touches(value.Trigger) && !touches(value.TriggerOther) && !touches(value.Target) {
				continue
			}
			if out.pathPresenceImplications == nil {
				out.pathPresenceImplications = make(map[PathPresenceImplication]struct{})
			}
			value.TriggerValue = projectValue(value.TriggerValue)
			value.TargetValue = projectValue(value.TargetValue)
			out.pathPresenceImplications[value] = struct{}{}
		}
	}
	return out
}

// RebaseBoundary atomically maps every retained path and product identity.
func (l Lane) RebaseBoundary(
	fromKeys, toKeys *keyspace.KeySpace,
	mapPath func(keyspace.Key) ([]keyspace.Key, bool),
	pathPreimages func(keyspace.Key) ([]keyspace.Key, bool),
	mapValue func(product.Value) (product.Value, bool),
	joinValue func(product.Value, product.Value) product.Value,
) (Lane, bool) {
	out := Lane{
		refinementsBottom: l.refinementsBottom, staticMembersBottom: l.staticMembersBottom,
		proofsBottom: l.proofsBottom, pathPresenceImplicationsBottom: l.pathPresenceImplicationsBottom,
	}
	var ok bool
	if out.refinements, ok = mapHandleValueMap(l.refinements, fromKeys, toKeys, mapPath, pathPreimages, mapValue, joinValue); !ok {
		return Lane{}, false
	}
	if out.staticMembers, ok = mapHandleValueMap(l.staticMembers, fromKeys, toKeys, mapPath, pathPreimages, mapValue, joinValue); !ok {
		return Lane{}, false
	}
	proofs, ok := lift.QuotientMustSet(l.proofs, func(proof BranchProof) ([]BranchProof, bool) {
		paths, valid := mapOptionalPaths(proof.Path, mapPath)
		if !valid {
			return nil, false
		}
		others, valid := mapOptionalPaths(proof.Other, mapPath)
		if !valid {
			return nil, false
		}
		mapped := make([]BranchProof, 0, len(paths)*len(others))
		for _, path := range paths {
			for _, other := range others {
				next := proof
				next.Path, next.Other = path, other
				mapped = append(mapped, next)
			}
		}
		return mapped, true
	}, func(proof BranchProof) BranchProof { return proof }, func(proof BranchProof) ([]BranchProof, bool) {
		paths, valid := mapOptionalPaths(proof.Path, pathPreimages)
		if !valid {
			return nil, false
		}
		others, valid := mapOptionalPaths(proof.Other, pathPreimages)
		if !valid {
			return nil, false
		}
		preimages := make([]BranchProof, 0, len(paths)*len(others))
		for _, path := range paths {
			for _, other := range others {
				next := proof
				next.Path, next.Other = path, other
				preimages = append(preimages, next)
			}
		}
		return preimages, true
	})
	if !ok {
		return Lane{}, false
	}
	out.proofs = proofs
	for proof := range proofs {
		out.equalityRootMask.merge(equalityProofRootMask(proof))
	}
	type implicationFiber struct {
		trigger      keyspace.Key
		triggerOther keyspace.Key
		target       keyspace.Key
	}
	implications, ok := lift.QuotientMustSet(l.pathPresenceImplications, func(value PathPresenceImplication) ([]PathPresenceImplication, bool) {
		triggers, valid := mapOptionalPaths(value.Trigger, mapPath)
		if !valid {
			return nil, false
		}
		triggerOthers, valid := mapOptionalPaths(value.TriggerOther, mapPath)
		if !valid {
			return nil, false
		}
		targets, valid := mapOptionalPaths(value.Target, mapPath)
		if !valid {
			return nil, false
		}
		if value.TriggerValue, ok = mapValue(value.TriggerValue); !ok {
			return nil, false
		}
		if value.TargetValue, ok = mapValue(value.TargetValue); !ok {
			return nil, false
		}
		mapped := make([]PathPresenceImplication, 0, len(triggers)*len(triggerOthers)*len(targets))
		for _, trigger := range triggers {
			for _, other := range triggerOthers {
				for _, target := range targets {
					next := value
					next.Trigger, next.TriggerOther, next.Target = trigger, other, target
					if validPathPresenceImplication(next) {
						mapped = append(mapped, next)
					}
				}
			}
		}
		return mapped, true
	}, func(value PathPresenceImplication) implicationFiber {
		return implicationFiber{trigger: value.Trigger, triggerOther: value.TriggerOther, target: value.Target}
	}, func(value PathPresenceImplication) ([]implicationFiber, bool) {
		triggers, valid := mapOptionalPaths(value.Trigger, pathPreimages)
		if !valid {
			return nil, false
		}
		triggerOthers, valid := mapOptionalPaths(value.TriggerOther, pathPreimages)
		if !valid {
			return nil, false
		}
		targets, valid := mapOptionalPaths(value.Target, pathPreimages)
		if !valid {
			return nil, false
		}
		preimages := make([]implicationFiber, 0, len(triggers)*len(triggerOthers)*len(targets))
		for _, trigger := range triggers {
			for _, other := range triggerOthers {
				for _, target := range targets {
					next := value
					next.Trigger, next.TriggerOther, next.Target = trigger, other, target
					if validPathPresenceImplication(next) {
						preimages = append(preimages, implicationFiber{trigger: trigger, triggerOther: other, target: target})
					}
				}
			}
		}
		return preimages, true
	})
	if !ok {
		return Lane{}, false
	}
	out.pathPresenceImplications = implications
	return out, true
}

// ApplyBoundary deletes destination facts touching the selected closure then
// overlays fragment facts, independently for every must sublane.
func (l Lane) ApplyBoundary(ks *keyspace.KeySpace, fragment Lane, touches func(keyspace.Key) bool) Lane {
	out := Lane{}
	out.refinements, out.refinementsBottom = applyHandleValueSublane(ks, l.refinements, l.refinementsBottom, fragment.refinements, fragment.refinementsBottom, touches)
	out.staticMembers, out.staticMembersBottom = applyHandleValueSublane(ks, l.staticMembers, l.staticMembersBottom, fragment.staticMembers, fragment.staticMembersBottom, touches)
	out.proofs, out.proofsBottom = applyProofSublane(l.proofs, l.proofsBottom, fragment.proofs, fragment.proofsBottom, touches)
	out.pathPresenceImplications, out.pathPresenceImplicationsBottom = applyImplicationSublane(l.pathPresenceImplications, l.pathPresenceImplicationsBottom, fragment.pathPresenceImplications, fragment.pathPresenceImplicationsBottom, touches)
	for proof := range out.proofs {
		out.equalityRootMask.merge(equalityProofRootMask(proof))
	}
	return out
}

func filterValueMap(in map[keyspace.Key]product.Value, keep func(keyspace.Key) bool, project func(product.Value) product.Value) map[keyspace.Key]product.Value {
	var out map[keyspace.Key]product.Value
	for key, value := range in {
		if keep(key) {
			if out == nil {
				out = make(map[keyspace.Key]product.Value)
			}
			out[key] = project(value)
		}
	}
	return out
}

func filterHandleValueMap(in map[keyspace.KeyHandle]product.Value, ks *keyspace.KeySpace, keep func(keyspace.Key) bool, project func(product.Value) product.Value) map[keyspace.KeyHandle]product.Value {
	var out map[keyspace.KeyHandle]product.Value
	for handle, value := range in {
		key, ok := ks.KeyByHandle(handle)
		if !ok {
			continue
		}
		if keep(key) {
			if out == nil {
				out = make(map[keyspace.KeyHandle]product.Value)
			}
			out[handle] = project(value)
		}
	}
	return out
}

func mapValueMap(
	in map[keyspace.Key]product.Value,
	mapPath func(keyspace.Key) ([]keyspace.Key, bool),
	pathPreimages func(keyspace.Key) ([]keyspace.Key, bool),
	mapValue func(product.Value) (product.Value, bool),
	joinValue func(product.Value, product.Value) product.Value,
) (map[keyspace.Key]product.Value, bool) {
	return lift.QuotientMustMap(in, mapPath, mapValue, func(path keyspace.Key) keyspace.Key { return path }, pathPreimages, joinValue)
}

func mapHandleValueMap(
	in map[keyspace.KeyHandle]product.Value,
	fromKeys, toKeys *keyspace.KeySpace,
	mapPath func(keyspace.Key) ([]keyspace.Key, bool),
	pathPreimages func(keyspace.Key) ([]keyspace.Key, bool),
	mapValue func(product.Value) (product.Value, bool),
	joinValue func(product.Value, product.Value) product.Value,
) (map[keyspace.KeyHandle]product.Value, bool) {
	if fromKeys == nil || toKeys == nil || !fromKeys.Valid() || !toKeys.Valid() {
		return nil, false
	}
	byKey := make(map[keyspace.Key]product.Value, len(in))
	for handle, value := range in {
		key, ok := fromKeys.KeyByHandle(handle)
		if !ok {
			return nil, false
		}
		byKey[key] = value
	}
	mapped, ok := mapValueMap(byKey, mapPath, pathPreimages, mapValue, joinValue)
	if !ok {
		return nil, false
	}
	out := make(map[keyspace.KeyHandle]product.Value, len(mapped))
	for key, value := range mapped {
		handle := key.Handle()
		if handle == 0 {
			return nil, false
		}
		out[handle] = value
	}
	return out, true
}
func mapOptionalPaths(path keyspace.Key, mapPath func(keyspace.Key) ([]keyspace.Key, bool)) ([]keyspace.Key, bool) {
	if path.Kind == keyspace.KindInvalid {
		return []keyspace.Key{path}, true
	}
	return mapPath(path)
}

func applyValueSublane(dst map[keyspace.Key]product.Value, dstBottom bool, frag map[keyspace.Key]product.Value, fragBottom bool, touches func(keyspace.Key) bool) (map[keyspace.Key]product.Value, bool) {
	if dstBottom || fragBottom {
		return nil, true
	}
	var out map[keyspace.Key]product.Value
	for key, value := range dst {
		if !touches(key) {
			if out == nil {
				out = make(map[keyspace.Key]product.Value)
			}
			out[key] = value
		}
	}
	if out == nil && len(frag) != 0 {
		out = make(map[keyspace.Key]product.Value, len(frag))
	}
	for key, value := range frag {
		out[key] = value
	}
	return out, false
}

func applyHandleValueSublane(ks *keyspace.KeySpace, dst map[keyspace.KeyHandle]product.Value, dstBottom bool, frag map[keyspace.KeyHandle]product.Value, fragBottom bool, touches func(keyspace.Key) bool) (map[keyspace.KeyHandle]product.Value, bool) {
	if dstBottom || fragBottom {
		return nil, true
	}
	var out map[keyspace.KeyHandle]product.Value
	for handle, value := range dst {
		key, ok := ks.KeyByHandle(handle)
		if !ok {
			continue
		}
		if !touches(key) {
			if out == nil {
				out = make(map[keyspace.KeyHandle]product.Value)
			}
			out[handle] = value
		}
	}
	if out == nil && len(frag) != 0 {
		out = make(map[keyspace.KeyHandle]product.Value, len(frag))
	}
	for handle, value := range frag {
		out[handle] = value
	}
	return out, false
}
func applyProofSublane(dst map[BranchProof]struct{}, dstBottom bool, frag map[BranchProof]struct{}, fragBottom bool, touches func(keyspace.Key) bool) (map[BranchProof]struct{}, bool) {
	if dstBottom || fragBottom {
		return nil, true
	}
	var out map[BranchProof]struct{}
	for value := range dst {
		if !touches(value.Path) && !touches(value.Other) {
			if out == nil {
				out = make(map[BranchProof]struct{})
			}
			out[value] = struct{}{}
		}
	}
	if out == nil && len(frag) != 0 {
		out = make(map[BranchProof]struct{}, len(frag))
	}
	for value := range frag {
		out[value] = struct{}{}
	}
	return out, false
}
func applyImplicationSublane(dst map[PathPresenceImplication]struct{}, dstBottom bool, frag map[PathPresenceImplication]struct{}, fragBottom bool, touches func(keyspace.Key) bool) (map[PathPresenceImplication]struct{}, bool) {
	if dstBottom || fragBottom {
		return nil, true
	}
	var out map[PathPresenceImplication]struct{}
	for value := range dst {
		if !touches(value.Trigger) && !touches(value.TriggerOther) && !touches(value.Target) {
			if out == nil {
				out = make(map[PathPresenceImplication]struct{})
			}
			out[value] = struct{}{}
		}
	}
	if out == nil && len(frag) != 0 {
		out = make(map[PathPresenceImplication]struct{}, len(frag))
	}
	for value := range frag {
		out[value] = struct{}{}
	}
	return out, false
}
