package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// CoordinateFiber is the structural inverse-image identity used by boundary
// quotienting. Product values are deliberately absent: they are lattice
// payloads, not members of the structural fiber.
type CoordinateFiber struct {
	kind                              CoordinateKind
	path, other, triggerOther, target keyspace.Key
}

// ExpandCoordinateClosure adds the complete structural support of one coupled
// must fact when any of its paths touches the current boundary closure. This
// is the coordinate spelling of Lane's closure expansion law: a relation is
// transported atomically, so every endpoint must acquire a destination root.
func ExpandCoordinateClosure(source CoordinateKey, connect func(...keyspace.Key) bool, addValue func(product.Value)) {
	if connect == nil {
		return
	}
	switch source.kind {
	case coordinateRefinement, coordinateStaticMember:
		connect(source.path)
	case coordinateBranchProof:
		connect(source.proof.Path, source.proof.Other)
	case coordinatePathPresenceImplication:
		value := source.implication
		if connect(value.Trigger, value.TriggerOther, value.Target) && addValue != nil {
			if value.HasTriggerValue {
				addValue(value.TriggerValue)
			}
			if value.HasTargetValue {
				addValue(value.TargetValue)
			}
		}
	}
}

func ProjectCoordinate(source CoordinateKey, scalar CoordinateScalar, touches func(keyspace.Key) bool, projectValue func(product.Value) product.Value) (CoordinateKey, CoordinateScalar, bool) {
	source, keep := ProjectCoordinateKey(source, touches, projectValue)
	if !keep {
		return CoordinateKey{}, CoordinateScalar{}, false
	}
	return source, ProjectCoordinateScalar(scalar, projectValue), true
}

func ProjectCoordinateKey(source CoordinateKey, touches func(keyspace.Key) bool, projectValue func(product.Value) product.Value) (CoordinateKey, bool) {
	keep := false
	switch source.kind {
	case coordinateRefinement, coordinateStaticMember:
		keep = touches(source.path)
	case coordinateBranchProof:
		keep = touches(source.proof.Path) || touches(source.proof.Other)
	case coordinatePathPresenceImplication:
		keep = touches(source.implication.Trigger) || touches(source.implication.TriggerOther) || touches(source.implication.Target)
		if keep {
			source.implication.TriggerValue = projectValue(source.implication.TriggerValue)
			source.implication.TargetValue = projectValue(source.implication.TargetValue)
		}
	default:
		return CoordinateKey{}, false
	}
	return source, keep
}

func ProjectCoordinateScalar(scalar CoordinateScalar, projectValue func(product.Value) product.Value) CoordinateScalar {
	if scalar.valueBearing {
		scalar.value = projectValue(scalar.value)
	}
	if scalar.implicationBearing && !scalar.clauseBottom {
		clauses := make([]coordinateImplicationClause, len(scalar.clauses))
		for index, clause := range scalar.clauses {
			trigger, target := clause.trigger, clause.target
			if trigger != (product.Value{}) {
				trigger = projectValue(trigger)
			}
			if target != (product.Value{}) {
				target = projectValue(target)
			}
			clauses[index] = coordinateImplicationClause{trigger: trigger, target: target}
		}
		scalar.clauses = canonicalImplicationClauses(clauses)
	}
	return scalar
}

func RebaseCoordinateKey(source CoordinateKey, mapPath func(keyspace.Key) ([]keyspace.Key, bool), mapValue func(product.Value) (product.Value, bool)) ([]CoordinateKey, bool) {
	switch source.kind {
	case coordinateRefinement, coordinateStaticMember:
		paths, ok := mapPath(source.path)
		if !ok {
			return nil, false
		}
		out := make([]CoordinateKey, len(paths))
		for index, path := range paths {
			out[index] = source
			out[index].path = path
		}
		return out, true
	case coordinateBranchProof:
		paths, ok := mapOptionalPaths(source.proof.Path, mapPath)
		if !ok {
			return nil, false
		}
		others, ok := mapOptionalPaths(source.proof.Other, mapPath)
		if !ok {
			return nil, false
		}
		out := make([]CoordinateKey, 0, len(paths)*len(others))
		for _, path := range paths {
			for _, other := range others {
				next := source
				next.proof.Path, next.proof.Other = path, other
				out = append(out, next)
			}
		}
		return out, true
	case coordinatePathPresenceImplication:
		triggers, ok := mapOptionalPaths(source.implication.Trigger, mapPath)
		if !ok {
			return nil, false
		}
		others, ok := mapOptionalPaths(source.implication.TriggerOther, mapPath)
		if !ok {
			return nil, false
		}
		targets, ok := mapOptionalPaths(source.implication.Target, mapPath)
		if !ok {
			return nil, false
		}
		triggerValue, ok := mapValue(source.implication.TriggerValue)
		if !ok {
			return nil, false
		}
		targetValue, ok := mapValue(source.implication.TargetValue)
		if !ok {
			return nil, false
		}
		out := make([]CoordinateKey, 0, len(triggers)*len(others)*len(targets))
		for _, trigger := range triggers {
			for _, other := range others {
				for _, target := range targets {
					next := source
					next.implication.Trigger, next.implication.TriggerOther, next.implication.Target = trigger, other, target
					next.implication.TriggerValue, next.implication.TargetValue = triggerValue, targetValue
					if validPathPresenceImplication(next.implication) {
						out = append(out, next)
					}
				}
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func RebaseCoordinateScalar(source CoordinateScalar, mapValue func(product.Value) (product.Value, bool)) (CoordinateScalar, bool) {
	if !source.valueBearing && !source.implicationBearing || source.clauseBottom {
		return source, true
	}
	if source.valueBearing {
		value, ok := mapValue(source.value)
		if !ok {
			return CoordinateScalar{}, false
		}
		source.value = value
		return source, true
	}
	clauses := make([]coordinateImplicationClause, len(source.clauses))
	for index, clause := range source.clauses {
		trigger, target := clause.trigger, clause.target
		var ok bool
		if trigger != (product.Value{}) {
			trigger, ok = mapValue(trigger)
			if !ok {
				return CoordinateScalar{}, false
			}
		}
		if target != (product.Value{}) {
			target, ok = mapValue(target)
			if !ok {
				return CoordinateScalar{}, false
			}
		}
		clauses[index] = coordinateImplicationClause{trigger: trigger, target: target}
	}
	source.clauses = canonicalImplicationClauses(clauses)
	return source, true
}

func CoordinateSourceFiber(source CoordinateKey) CoordinateFiber {
	switch source.kind {
	case coordinateRefinement, coordinateStaticMember:
		return CoordinateFiber{kind: source.kind, path: source.path}
	case coordinateBranchProof:
		return CoordinateFiber{kind: source.kind, path: source.proof.Path, other: source.proof.Other}
	case coordinatePathPresenceImplication:
		return CoordinateFiber{kind: source.kind, path: source.implication.Trigger, triggerOther: source.implication.TriggerOther, target: source.implication.Target}
	default:
		return CoordinateFiber{}
	}
}

func CoordinateInverseFibers(destination CoordinateKey, inversePath func(keyspace.Key) ([]keyspace.Key, bool)) ([]CoordinateFiber, bool) {
	switch destination.kind {
	case coordinateRefinement, coordinateStaticMember:
		paths, ok := inversePath(destination.path)
		if !ok {
			return nil, false
		}
		out := make([]CoordinateFiber, len(paths))
		for index, path := range paths {
			out[index] = CoordinateFiber{kind: destination.kind, path: path}
		}
		return out, true
	case coordinateBranchProof:
		paths, ok := mapOptionalPaths(destination.proof.Path, inversePath)
		if !ok {
			return nil, false
		}
		others, ok := mapOptionalPaths(destination.proof.Other, inversePath)
		if !ok {
			return nil, false
		}
		out := make([]CoordinateFiber, 0, len(paths)*len(others))
		for _, path := range paths {
			for _, other := range others {
				out = append(out, CoordinateFiber{kind: destination.kind, path: path, other: other})
			}
		}
		return out, true
	case coordinatePathPresenceImplication:
		triggers, ok := mapOptionalPaths(destination.implication.Trigger, inversePath)
		if !ok {
			return nil, false
		}
		others, ok := mapOptionalPaths(destination.implication.TriggerOther, inversePath)
		if !ok {
			return nil, false
		}
		targets, ok := mapOptionalPaths(destination.implication.Target, inversePath)
		if !ok {
			return nil, false
		}
		out := make([]CoordinateFiber, 0, len(triggers)*len(others)*len(targets))
		for _, trigger := range triggers {
			for _, other := range others {
				for _, target := range targets {
					out = append(out, CoordinateFiber{kind: destination.kind, path: trigger, triggerOther: other, target: target})
				}
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// BoundaryAliasCoordinateEntries is the static post-incidence relation for
// boundary aliases. CoordinateKeySupported, not another key-generation path,
// determines whether each candidate is admitted by a runtime skeleton.
func BoundaryAliasCoordinateEntries(aliases [][2]keyspace.Key) []CoordinateEntry {
	proofs := BoundaryAliasPathEqualProofs(aliases)
	out := make([]CoordinateEntry, 0, len(proofs))
	for _, proof := range proofs {
		out = append(out, CoordinateEntry{
			Key:    CoordinateKey{kind: coordinateBranchProof, proof: proof},
			Scalar: CoordinateScalar{present: true},
		})
	}
	return out
}

// BoundaryAliasPathEqualProofs is the single semantic spelling of the alias
// relation while the whole-lane route still exists.
func BoundaryAliasPathEqualProofs(aliases [][2]keyspace.Key) []BranchProof {
	out := make([]BranchProof, len(aliases))
	for index, alias := range aliases {
		out[index] = BranchProof{Kind: BranchProofPathEqual, Path: alias[0], Other: alias[1]}
	}
	return out
}

func ApplyCoordinateRootReachability(skeleton CoordinateSkeleton, establishes bool) CoordinateSkeleton {
	if establishes {
		return skeleton.Reachable()
	}
	return skeleton
}

func BoundaryRootSlot(path keyspace.Key) (CoordinateKey, bool) {
	return CoordinateKey{kind: coordinateRefinement, path: path}, path.Kind != keyspace.KindInvalid
}

func BoundaryRootScalar(reg *axis.Registry, key CoordinateKey, value product.Value) (CoordinateScalar, bool) {
	if key.kind != coordinateRefinement || key.path.Kind == keyspace.KindInvalid {
		return CoordinateScalar{}, false
	}
	present := !product.Equal(reg, value, product.Bottom(reg))
	return CoordinateScalar{present: present, valueBearing: true, value: value}, true
}
