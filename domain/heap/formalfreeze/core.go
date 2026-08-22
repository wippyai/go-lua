// Package formalfreeze consumes the exact Target freeze ownership row at an
// ordinary mounted call and projects it onto Heap's allocation roots.
//
// The package is deliberately a narrow consumer. Target remains the owner of
// FormalEffect rows, Pack remains the owner of mounted actual geometry, Value
// remains the owner of actual facts, and Heap remains the owner of allocation
// keys and freeze transitions. No Placement state is imported or retained.
package formalfreeze

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const formalFreezeInlineWidth = recentplan.InlineWidth

// operand is the owner-fenced mounted call coordinate carried by one Rule
// row. The Call key is the canonical call identity; the occurrence receipt is
// retained only so Pack can redeem its authored actual projection.
type operand struct {
	packs   *packdomain.Schema
	mounted calldomain.MountedCall
	key     calldomain.Key
	id      identity.ContentID
}

func mountedOperandID(module, occurrence, application, keyID identity.ContentID) identity.ContentID {
	const prefix = "wippy.analysis.heap.formalfreeze.invocation.v1\x00"
	hash := sha256.New()
	_, _ = hash.Write([]byte(prefix))
	_, _ = hash.Write(module[:])
	_, _ = hash.Write(occurrence[:])
	_, _ = hash.Write(application[:])
	_, _ = hash.Write(keyID[:])
	return identity.ContentID(hash.Sum(nil))
}

// mountedForOperand authenticates every constituent of an operand against the
// exact Pack/Call owners. It is intentionally the same mounted-call receipt
// seam used by Placement/formal, but does not import or retain Placement.
func mountedForOperand(packs *packdomain.Schema, algebra *calldomain.Algebra, candidate operand) (packdomain.MountedActualProjection, identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	if packs == nil || algebra == nil || !algebra.Valid() || candidate.packs != packs || !candidate.mounted.Valid() ||
		!packs.LinkOwner().Available() || !packs.LinkOwner().Matches(algebra.LinkOwner()) {
		return packdomain.MountedActualProjection{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	application, occurrence, module, _, _, identityOK := algebra.MountedCallIdentity(candidate.mounted)
	key, keyOK := algebra.KeyForMountedCall(candidate.mounted)
	keyID, keyIDOK := key.ContentID()
	actual, actualOK := packs.MountedActualProjection(module, occurrence)
	if !actualOK || !actual.Valid() || !actual.OwnedBy(packs) || !identityOK || !keyOK || !key.IsApplication() || !key.Valid() || !keyIDOK || !keyID.Available() || candidate.key != key {
		return packdomain.MountedActualProjection{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	expected := mountedOperandID(module, occurrence, application, keyID)
	if candidate.id != expected || !expected.Available() {
		return packdomain.MountedActualProjection{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	return actual, module, occurrence, application, true
}

func operandContent(packs *packdomain.Schema, algebra *calldomain.Algebra, candidate operand) (operand, [32]byte, bool) {
	_, _, _, _, ok := mountedForOperand(packs, algebra, candidate)
	if !ok {
		return operand{}, [32]byte{}, false
	}
	return candidate, [32]byte(candidate.id), true
}

func operandForOccurrence(packs *packdomain.Schema, algebra *calldomain.Algebra, module, occurrence identity.ContentID) (operand, bool) {
	if packs == nil || algebra == nil || !algebra.Valid() || !packs.LinkOwner().Available() || !packs.LinkOwner().Matches(algebra.LinkOwner()) {
		return operand{}, false
	}
	mounted, mountedOK := algebra.MountedCallForOccurrence(module, occurrence)
	application, callID, moduleID, _, _, identityOK := algebra.MountedCallIdentity(mounted)
	key, keyOK := algebra.KeyForMountedCall(mounted)
	keyID, keyIDOK := key.ContentID()
	if !mountedOK || !identityOK || callID != occurrence || moduleID != module || !keyOK || !key.IsApplication() || !key.Valid() || !keyIDOK || !keyID.Available() {
		return operand{}, false
	}
	actual, actualOK := packs.MountedActualProjection(module, occurrence)
	if !actualOK || !actual.Valid() || !actual.OwnedBy(packs) {
		return operand{}, false
	}
	id := mountedOperandID(module, occurrence, application, keyID)
	return operand{packs: packs, mounted: mounted, key: key, id: id}, id.Available()
}

// actualObservation is one mounted actual as the read boundary delivered it.
// It carries no presence bit: the read declares Value's default at an unwritten
// coordinate, so an observation always holds a value, and Bottom is refused by
// exactRecentAllocation on its own merits rather than by presence metadata.
type actualObservation struct {
	fact  valuedomain.Value
	valid bool
}

// recentplan is Heap-owned so formal and publication freeze share one exact
// Recent route-set authority without sharing their consumer policies.
type route = recentplan.Route
type routePlan = recentplan.Plan

func routeForTag(plan routePlan, tag heap.RawRouteTag) (route, bool) {
	return recentplan.RouteForTag(plan, tag)
}

// exactRecentAllocation accepts only one enumerable, owner-fenced Value
// atom. Opaque references, Summary/Exact materializations, scalar alternatives
// and ambiguous unions all fail closed rather than widening a freeze route.
func exactRecentAllocation(values *valuedomain.Schema, observation actualObservation) (heap.Key, bool) {
	if !observation.valid {
		return heap.Key{}, false
	}
	return values.ExactRecentAllocation(observation.fact, true)
}

// freezeParamSet is the allocation-free representation of one target's exact
// Freeze selectors. The inline prefix covers ordinary declarations; unusually
// wide formal rows use an invocation-local overflow suffix.
type freezeParamSet struct {
	inline [formalFreezeInlineWidth]int
	extra  []int
	size   int
}

func (set freezeParamSet) count() int {
	if set.size < 0 {
		return 0
	}
	return set.size
}

func (set freezeParamSet) at(index int) (int, bool) {
	if index < 0 || index >= set.size {
		return 0, false
	}
	if index < len(set.inline) {
		return set.inline[index], true
	}
	extra := index - len(set.inline)
	if extra < 0 || extra >= len(set.extra) {
		return 0, false
	}
	return set.extra[extra], true
}

func (set *freezeParamSet) add(param int) bool {
	if set == nil || set.size < 0 || param < 0 {
		return false
	}
	position := 0
	for position < set.size {
		current, ok := set.at(position)
		if !ok {
			return false
		}
		switch {
		case current == param:
			return true
		case current > param:
			goto insert
		default:
			position++
		}
	}

insert:
	if set.size < len(set.inline) {
		for index := set.size; index > position; index-- {
			set.inline[index] = set.inline[index-1]
		}
		set.inline[position] = param
		set.size++
		return true
	}
	if position < len(set.inline) {
		carried := set.inline[len(set.inline)-1]
		for index := len(set.inline) - 1; index > position; index-- {
			set.inline[index] = set.inline[index-1]
		}
		set.inline[position] = param
		set.extra = append(set.extra, 0)
		copy(set.extra[1:], set.extra[:len(set.extra)-1])
		set.extra[0] = carried
	} else {
		extra := position - len(set.inline)
		if extra < 0 || extra > len(set.extra) {
			return false
		}
		set.extra = append(set.extra, 0)
		copy(set.extra[extra+1:], set.extra[extra:len(set.extra)-1])
		set.extra[extra] = param
	}
	set.size++
	return true
}

// freezeParamsForTarget returns every exact fixed formal actual a known
// target justifies. Formal rows are exact-only here: an open row, an invalid
// target, a body target, a missing Freeze row, or an out-of-range parameter
// produces no set. Non-Freeze formal rows belong to other consumers and are
// ignored. Resolved duplicate selectors (for example explicit last-actual and
// Param=-1 on a fixed call) are canonicalized once.
func freezeParamsForTarget(targetContract *contract.Contract, target calldomain.Target, actualCount int, runtimeTail bool) (freezeParamSet, bool, bool) {
	if targetContract == nil || actualCount < 0 {
		return freezeParamSet{}, false, false
	}
	operation, operationOK := target.Operation()
	if !operationOK || operation == 0 {
		return freezeParamSet{}, false, true
	}
	declared, declaredOK := targetContract.Operations.OperationAt(int(operation) - 1)
	if !declaredOK || declared != operation {
		return freezeParamSet{}, false, false
	}
	tail, tailOK := targetContract.Operations.FormalEffectTail(operation)
	if !tailOK {
		return freezeParamSet{}, false, false
	}
	// Unknown-open formal rows are not an exact ownership proof. They do not
	// widen a freeze operation; they simply have no route in this consumer.
	if tail != vocabulary.RowClosed {
		return freezeParamSet{}, false, true
	}
	var params freezeParamSet
	found := false
	for index := 0; index < targetContract.Operations.FormalEffectCount(operation); index++ {
		spec, specOK := targetContract.Operations.FormalEffectAt(operation, index)
		if !specOK {
			return freezeParamSet{}, false, false
		}
		if spec.Kind != vocabulary.FormalEffectFreeze {
			continue
		}
		resolved := int(spec.Param)
		if spec.Param == -1 {
			// The last actual is exact only when the mounted row has no runtime
			// suffix beyond its fixed prefix.
			if runtimeTail || actualCount == 0 {
				return freezeParamSet{}, false, true
			}
			resolved = actualCount - 1
		}
		if resolved < 0 || resolved >= actualCount {
			return freezeParamSet{}, false, true
		}
		if !params.add(resolved) {
			return freezeParamSet{}, false, false
		}
		found = true
	}
	return params, found, true
}

// planFor is the sole formal-freeze reduction. Each known target contributes
// its exact Recent-root route set; the strong write is their route-tag
// intersection, so aliases can match even when authored actual ordinals
// differ. Semantic uncertainty is an empty, valid plan; malformed owner
// authority is a failed plan. This lets the routed Rule settle
// unresolved/open/opaque/ambiguous cases through its authenticated empty
// selection without fabricating Heap state.
func planFor(
	packs *packdomain.Schema,
	calls *calldomain.Algebra,
	schema heap.Schema,
	values *valuedomain.Schema,
	targetContract *contract.Contract,
	mounted calldomain.MountedCall,
	callFact calldomain.Value,
	observations []actualObservation,
) (routePlan, bool) {
	if packs == nil || calls == nil || !calls.Valid() || !schema.Valid() || values == nil || !values.Valid() || targetContract == nil || !calls.OwnsTargetContract(targetContract) || !values.OwnsHeapSchema(schema) || !values.LinkOwner().Matches(calls.LinkOwner()) || !packs.LinkOwner().Available() || !packs.LinkOwner().Matches(calls.LinkOwner()) {
		return routePlan{}, false
	}
	_, callID, module, _, _, identityOK := calls.MountedCallIdentity(mounted)
	actual, actualOK := packs.MountedActualProjection(module, callID)
	key, keyOK := calls.KeyForMountedCall(mounted)
	if !identityOK || !actualOK || !actual.Valid() || !actual.OwnedBy(packs) || !keyOK || !key.Valid() || !key.IsApplication() || len(observations) != actual.ActualCount() || !calls.Admits(key, callFact) {
		return routePlan{}, false
	}
	_, runtimeTail := actual.TailID()
	if !callFact.IsComplete() || callFact.IsEmpty() || callFact.KnownTargetCount() == 0 {
		return routePlan{}, true
	}
	if callFact.HasOpaqueAlternative() || callFact.IsTop() {
		return routePlan{}, true
	}
	var intersection routePlan
	haveRoutes := false
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK || !calls.OwnsTarget(target) {
			return routePlan{}, true
		}
		params, found, targetOK := freezeParamsForTarget(targetContract, target, actual.ActualCount(), runtimeTail)
		if !targetOK || !found {
			return routePlan{}, true
		}
		var targetRoutes routePlan
		for paramIndex := 0; paramIndex < params.count(); paramIndex++ {
			param, paramOK := params.at(paramIndex)
			if !paramOK || param < 0 || param >= len(observations) {
				return routePlan{}, true
			}
			observation := observations[param]
			candidate, candidateOK := exactRecentAllocation(values, observation)
			if !candidateOK {
				return routePlan{}, true
			}
			tag, tagOK := schema.RouteTag(candidate, materialization.Recent)
			if !tagOK {
				return routePlan{}, false
			}
			if !targetRoutes.Add(route{Key: candidate, Tag: tag}) {
				return routePlan{}, false
			}
		}
		if !haveRoutes {
			intersection = targetRoutes
			haveRoutes = true
			continue
		}
		var intersectionOK bool
		intersection, intersectionOK = intersection.Intersection(targetRoutes)
		if !intersectionOK {
			return routePlan{}, false
		}
		// Once no exact root is common to the known alternatives, no later
		// target can restore a strong write. This is the common mixed-target
		// case and avoids traversing the remaining formal rows on the hot path.
		if intersection.Count() == 0 {
			return routePlan{}, true
		}
	}
	if !haveRoutes || intersection.Count() == 0 {
		return routePlan{}, true
	}
	for index := 0; index < intersection.Count(); index++ {
		candidate, candidateOK := intersection.At(index)
		if !candidateOK || !candidate.Key.Valid() || candidate.Key.Kind() != heap.RootAllocation || !schema.OwnsKey(candidate.Key) || candidate.Tag == 0 {
			return routePlan{}, false
		}
	}
	return intersection, true
}
