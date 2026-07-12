package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// RekeyValueLanes re-interns every structural key carried by path evidence from
// one keyspace into another. It lets a path-evidence lane built in one analysis's
// keyspace be consumed in another (cross-summary entry-state transfer) without
// the per-keyspace intern ids diverging. A nil keyspace, or from == to, returns
// the lane unchanged. The historical name is retained for API compatibility;
// proofs and implications now carry structural keys too and are rekeyed here.
func (l Lane) RekeyValueLanes(from, to *keyspace.KeySpace) Lane {
	if from == nil || to == nil || from == to {
		return l
	}
	out := l
	out.refinements = rekeyValueMap(from, to, l.refinements)
	out.staticMembers = rekeyValueMap(from, to, l.staticMembers)
	out.proofs = rekeyBranchProofs(from, to, l.proofs)
	out.equalityRootMask = equalityRootMask{}
	for proof := range out.proofs {
		out.equalityRootMask.merge(equalityProofRootMask(proof))
	}
	out.pathPresenceImplications = rekeyPathPresenceImplications(from, to, l.pathPresenceImplications)
	return out
}

func rekeyValueMap(from, to *keyspace.KeySpace, in map[keyspace.Key]product.Value) map[keyspace.Key]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[keyspace.Key]product.Value, len(in))
	for key, value := range in {
		rekeyed, ok := to.ImportKey(from, key)
		if !ok {
			continue
		}
		out[rekeyed] = value
	}
	return out
}

func rekeyBranchProofs(from, to *keyspace.KeySpace, in map[BranchProof]struct{}) map[BranchProof]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[BranchProof]struct{}, len(in))
	for proof := range in {
		path, ok := importRequiredKey(from, to, proof.Path)
		if !ok {
			continue
		}
		other, ok := importOptionalKey(from, to, proof.Other)
		if !ok {
			continue
		}
		proof.Path = path
		proof.Other = other
		out[proof] = struct{}{}
	}
	return out
}

func rekeyPathPresenceImplications(from, to *keyspace.KeySpace, in map[PathPresenceImplication]struct{}) map[PathPresenceImplication]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[PathPresenceImplication]struct{}, len(in))
	for implication := range in {
		trigger, ok := importRequiredKey(from, to, implication.Trigger)
		if !ok {
			continue
		}
		triggerOther, ok := importOptionalKey(from, to, implication.TriggerOther)
		if !ok {
			continue
		}
		target, ok := importRequiredKey(from, to, implication.Target)
		if !ok {
			continue
		}
		implication.Trigger = trigger
		implication.TriggerOther = triggerOther
		implication.Target = target
		out[implication] = struct{}{}
	}
	return out
}

func importRequiredKey(from, to *keyspace.KeySpace, key keyspace.Key) (keyspace.Key, bool) {
	return to.ImportKey(from, key)
}

func importOptionalKey(from, to *keyspace.KeySpace, key keyspace.Key) (keyspace.Key, bool) {
	if key.Kind == keyspace.KindInvalid {
		return keyspace.Key{}, true
	}
	return to.ImportKey(from, key)
}
