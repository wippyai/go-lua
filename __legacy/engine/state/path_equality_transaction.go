package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// PathEqualityTransaction is the ProductDomain-sealed finite equality
// quotient produced by publishing one path equality. PathEvidence owns proof
// publication; every other axis must explicitly register either a participant
// or an independence proof for the resulting quotient rewrite.
type PathEqualityTransaction struct {
	seal      *productDomainSeal
	keys      *keyspace.KeySpace
	proof     pathevidence.BranchProof
	quotient  pathevidence.EqualityQuotient
	transient bool
}

// PrepareCoordinateTransientPathEqualityTransaction seals a point-local
// value equality. It may derive closure among already-retained proofs, but the
// synthetic equation itself is never published as path identity.
func (d ProductDomain) PrepareCoordinateTransientPathEqualityTransaction(carrier *CoordinatePathEvidenceCarrier[statekey.Value], proof pathevidence.BranchProof) (PathEqualityTransaction, error) {
	return PrepareCoordinateTransientPathEqualityTransaction(d, carrier, proof)
}

func PrepareCoordinateTransientPathEqualityTransaction[K comparable](d ProductDomain, carrier *CoordinatePathEvidenceCarrier[K], proof pathevidence.BranchProof) (PathEqualityTransaction, error) {
	if !d.Valid() || carrier == nil || carrier.domain.seal != d.seal || !validPathEqualityProof(carrier.keys, proof) {
		return PathEqualityTransaction{}, fmt.Errorf("state: invalid transient path equality transaction")
	}
	staged := carrier.Clone()
	if staged == nil {
		return PathEqualityTransaction{}, fmt.Errorf("state: cannot stage transient path equality transaction")
	}
	if _, ok := staged.CloseProofsAcrossTransientEquality(proof.Path, proof.Other); !ok {
		return PathEqualityTransaction{}, fmt.Errorf("state: transient path equality closure is not authorized")
	}
	return PathEqualityTransaction{seal: d.seal, keys: carrier.keys, proof: proof, transient: true}, nil
}

func validPathEqualityProof(keys *keyspace.KeySpace, proof pathevidence.BranchProof) bool {
	if keys == nil || !keys.Valid() || proof.Kind != pathevidence.BranchProofPathEqual ||
		proof.Path.Kind == keyspace.KindInvalid || proof.Other.Kind == keyspace.KindInvalid || proof.Path == proof.Other {
		return false
	}
	_, leftOK := keys.SegmentsView(proof.Path)
	_, rightOK := keys.SegmentsView(proof.Other)
	return leftOK && rightOK
}

// PrepareCoordinatePathEqualityTransaction derives the equality quotient
// from the registered coordinate carrier. The staged clone proves the frozen
// coordinate inventory authorizes publication; it is discarded after sealing.
func (d ProductDomain) PrepareCoordinatePathEqualityTransaction(carrier *CoordinatePathEvidenceCarrier[statekey.Value], proof pathevidence.BranchProof) (PathEqualityTransaction, error) {
	return PrepareCoordinatePathEqualityTransaction(d, carrier, proof)
}

func PrepareCoordinatePathEqualityTransaction[K comparable](d ProductDomain, carrier *CoordinatePathEvidenceCarrier[K], proof pathevidence.BranchProof) (PathEqualityTransaction, error) {
	if !d.Valid() || carrier == nil || carrier.domain.seal != d.seal || !validPathEqualityProof(carrier.keys, proof) {
		return PathEqualityTransaction{}, fmt.Errorf("state: invalid coordinate path equality transaction")
	}
	staged := carrier.Clone()
	if staged == nil {
		return PathEqualityTransaction{}, fmt.Errorf("state: cannot stage path equality transaction")
	}
	if _, ok := staged.AddProof(proof); !ok {
		return PathEqualityTransaction{}, fmt.Errorf("state: path equality publication is not authorized")
	}
	quotient, ok := staged.EqualityQuotient()
	if !ok {
		return PathEqualityTransaction{}, fmt.Errorf("state: cannot seal coordinate path equality quotient")
	}
	return PathEqualityTransaction{seal: d.seal, keys: carrier.keys, proof: proof, quotient: quotient}, nil
}

func (d ProductDomain) ownsPathEqualityTransaction(transaction PathEqualityTransaction) bool {
	return d.Valid() && transaction.seal == d.seal && validPathEqualityProof(transaction.keys, transaction.proof) &&
		(transaction.transient || transaction.quotient.Valid())
}

// PathEqualityQuotientLanes returns the exact non-coordinate participant
// inventory. New axes cannot silently miss equality publication because lane
// catalog admission requires an explicit law for this capability.
func (d ProductDomain) PathEqualityQuotientLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	pathFamily, hasPathFamily := d.PathEvidenceCoordinateFamily()
	out := make([]ProductLane, 0, 1)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathEqualityQuotient)
		if !declared || !law.participates || hasPathFamily && runtime.lane.Ordinal() == pathFamily.Lane().Ordinal() {
			continue
		}
		out = append(out, runtime.lane)
	}
	return out
}

// ApplyPathEqualityTransactionFactor applies one registered participant
// without reconstructing State. Unchanged factors retain identity.
func (d ProductDomain) ApplyPathEqualityTransactionFactor(transaction PathEqualityTransaction, current LaneFactor) (LaneFactor, error) {
	if !d.ownsPathEqualityTransaction(transaction) {
		return LaneFactor{}, fmt.Errorf("state: foreign path equality transaction")
	}
	if transaction.transient {
		return current, nil
	}
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathEqualityQuotient)
	if !declared || !law.participates {
		return LaneFactor{}, fmt.Errorf("%w: lane %q does not participate in path equality", ErrInvalidLaneFactor, runtime.lane.id)
	}
	next, changed, valid := law.applyFactor(current.payload, pathEqualityQuotientRequest{reg: d.reg, keys: transaction.keys, quotient: transaction.quotient})
	if !valid {
		return LaneFactor{}, fmt.Errorf("state: lane %q rejected path equality quotient", runtime.lane.id)
	}
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}

// ApplyCoordinatePathEqualityTransaction publishes the proof through the
// already-open path-evidence carrier. Participant factors consume the same
// transaction via ApplyPathEqualityTransactionFactor.
func (d ProductDomain) ApplyCoordinatePathEqualityTransaction(transaction PathEqualityTransaction, carrier *CoordinatePathEvidenceCarrier[statekey.Value]) (bool, error) {
	return ApplyCoordinatePathEqualityTransaction(d, transaction, carrier)
}

func ApplyCoordinatePathEqualityTransaction[K comparable](d ProductDomain, transaction PathEqualityTransaction, carrier *CoordinatePathEvidenceCarrier[K]) (bool, error) {
	if !d.ownsPathEqualityTransaction(transaction) || carrier == nil || carrier.domain.seal != d.seal || carrier.keys != transaction.keys {
		return false, fmt.Errorf("state: foreign coordinate path equality transaction")
	}
	var changed, ok bool
	if transaction.transient {
		changed, ok = carrier.CloseProofsAcrossTransientEquality(transaction.proof.Path, transaction.proof.Other)
	} else {
		changed, ok = carrier.AddProof(transaction.proof)
	}
	if !ok {
		return false, fmt.Errorf("state: path equality publication rejected")
	}
	return changed, nil
}

// ApplyPathEqualityProof atomically publishes one equality through the unique
// path carrier and every registered quotient participant. It is the concrete
// composition of the same factor laws consumed by formal execution.
func (d ProductDomain) ApplyPathEqualityProof(keys *keyspace.KeySpace, proof pathevidence.BranchProof, input State) (State, error) {
	family, ok := d.PathEvidenceCoordinateFamily()
	if !ok || !validPathEqualityProof(keys, proof) {
		return State{}, fmt.Errorf("state: invalid path equality proof")
	}
	pathFactors, err := d.DecomposeLanes(input, []ProductLane{family.Lane()})
	if err != nil {
		return State{}, err
	}
	skeleton, scalars, err := d.DecomposeCoordinateFamily(pathFactors[0], family, keys)
	if err != nil {
		return State{}, err
	}
	slot, err := d.PathBranchProofCoordinateSlot(keys, proof)
	if err != nil {
		return State{}, err
	}
	empty, err := d.SealCoordinateFactorInventory(keys, nil)
	if err != nil {
		return State{}, err
	}
	writes, err := d.SealCoordinateFactorInventory(keys, []CoordinateSlot{slot})
	if err != nil {
		return State{}, err
	}
	authority, err := SealCoordinatePathEvidenceAuthority(
		d, keys, nil, nil, empty, writes, false, true,
		func(slot statekey.Value) bool { return slot != 0 },
	)
	if err != nil {
		return State{}, err
	}
	carrier, err := d.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, ValueLaneFactor{}, true,
		authority, PathDescendantMutationFactors{},
	)
	if err != nil {
		return State{}, err
	}
	transaction, err := d.PrepareCoordinatePathEqualityTransaction(carrier, proof)
	if err != nil {
		return State{}, err
	}
	if _, err := d.ApplyCoordinatePathEqualityTransaction(transaction, carrier); err != nil {
		return State{}, err
	}
	nextSkeleton, nextScalars, _, _, _, _, err := carrier.Freeze()
	if err != nil {
		return State{}, err
	}
	pathFactor, err := d.ReplaceCoordinateFamily(pathFactors[0], nextSkeleton, nextScalars)
	if err != nil {
		return State{}, err
	}
	lanes := d.PathEqualityQuotientLanes()
	factors, err := d.DecomposeLanes(input, lanes)
	if err != nil {
		return State{}, err
	}
	for index := range factors {
		factors[index], err = d.ApplyPathEqualityTransactionFactor(transaction, factors[index])
		if err != nil {
			return State{}, err
		}
	}
	replacements := make([]LaneFactor, 0, len(factors)+1)
	replacements = append(replacements, pathFactor)
	replacements = append(replacements, factors...)
	return d.PatchLaneFactors(input, replacements)
}
