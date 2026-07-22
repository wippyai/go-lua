package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// RekeyValueLanes re-interns every structural key carried by path evidence from
// one keyspace into another. It lets a path-evidence lane built in one analysis's
// keyspace be consumed in another (cross-summary entry-state transfer) without
// the per-keyspace intern ids diverging. Same-space imports still validate key
// ownership; nil provenance succeeds only when the lane carries no structural
// keys. Proofs and implications carry structural keys and are included.
func (l Lane) RekeyValueLanes(from, to *keyspace.KeySpace) (Lane, bool) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return l, false
	}
	if len(l.refinements)+len(l.staticMembers)+len(l.proofs)+len(l.pathPresenceImplications) == 0 {
		return l, true
	}
	if from == nil || to == nil {
		return l, false
	}
	out := l
	var ok bool
	if out.refinements, ok = rekeyHandleValueMap(from, to, l.refinements); !ok {
		return l, false
	}
	if out.staticMembers, ok = rekeyValueMap(from, to, l.staticMembers); !ok {
		return l, false
	}
	if out.proofs, ok = rekeyBranchProofs(from, to, l.proofs); !ok {
		return l, false
	}
	out.equalityRootMask = equalityRootMask{}
	for proof := range out.proofs {
		out.equalityRootMask.merge(equalityProofRootMask(proof))
	}
	if out.pathPresenceImplications, ok = rekeyPathPresenceImplications(from, to, l.pathPresenceImplications); !ok {
		return l, false
	}
	return out, true
}

func rekeyValueMap(from, to *keyspace.KeySpace, in map[keyspace.Key]product.Value) (map[keyspace.Key]product.Value, bool) {
	if len(in) == 0 {
		return nil, true
	}
	out := make(map[keyspace.Key]product.Value, len(in))
	for key, value := range in {
		rekeyed, ok := to.ImportKey(from, key)
		if !ok {
			return nil, false
		}
		out[rekeyed] = value
	}
	return out, true
}

func rekeyHandleValueMap(from, to *keyspace.KeySpace, in map[keyspace.KeyHandle]product.Value) (map[keyspace.KeyHandle]product.Value, bool) {
	if len(in) == 0 {
		return nil, true
	}
	out := make(map[keyspace.KeyHandle]product.Value, len(in))
	for handle, value := range in {
		key, ok := from.KeyByHandle(handle)
		if !ok {
			return nil, false
		}
		rekeyed, ok := to.ImportKey(from, key)
		if !ok || rekeyed.Handle() == 0 {
			return nil, false
		}
		out[rekeyed.Handle()] = value
	}
	return out, true
}

func rekeyBranchProofs(from, to *keyspace.KeySpace, in map[BranchProof]struct{}) (map[BranchProof]struct{}, bool) {
	if len(in) == 0 {
		return nil, true
	}
	out := make(map[BranchProof]struct{}, len(in))
	for proof := range in {
		path, ok := importRequiredKey(from, to, proof.Path)
		if !ok {
			return nil, false
		}
		other, ok := importOptionalKey(from, to, proof.Other)
		if !ok {
			return nil, false
		}
		proof.Path = path
		proof.Other = other
		out[proof] = struct{}{}
	}
	return out, true
}

func rekeyPathPresenceImplications(from, to *keyspace.KeySpace, in map[PathPresenceImplication]struct{}) (map[PathPresenceImplication]struct{}, bool) {
	if len(in) == 0 {
		return nil, true
	}
	out := make(map[PathPresenceImplication]struct{}, len(in))
	for implication := range in {
		trigger, ok := importRequiredKey(from, to, implication.Trigger)
		if !ok {
			return nil, false
		}
		triggerOther, ok := importOptionalKey(from, to, implication.TriggerOther)
		if !ok {
			return nil, false
		}
		target, ok := importRequiredKey(from, to, implication.Target)
		if !ok {
			return nil, false
		}
		implication.Trigger = trigger
		implication.TriggerOther = triggerOther
		implication.Target = target
		out[implication] = struct{}{}
	}
	return out, true
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
