package state

import (
	"fmt"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// CoordinateFormalDestinationKeySpace exposes the destination authority of a
// sealed structural-root rekey without exposing its binding representation.
// Operation plans use it to seal all key-bearing children in one namespace.
func (d ProductDomain) CoordinateFormalDestinationKeySpace(plan CoordinateFormalRootRekey) (*keyspace.KeySpace, bool) {
	return plan.to, plan.validFor(d)
}

func (d ProductDomain) rekeyRootAssignmentStateKeyFormal(plan CoordinateFormalRootRekey, source pathaddr.StateKey) (pathaddr.StateKey, error) {
	if !plan.validFor(d) || source == "" {
		return "", fmt.Errorf("%w: root-assignment formal state key", ErrInvalidLaneFactor)
	}
	key, ok := plan.from.InternStateKey(source)
	if !ok {
		return "", fmt.Errorf("%w: unresolved root-assignment formal state key", ErrInvalidLaneFactor)
	}
	target, ok := plan.rekey(key)
	if !ok {
		return "", fmt.Errorf("%w: unmapped root-assignment formal state key", ErrInvalidLaneFactor)
	}
	formatted := plan.to.FormatReadOnly(target)
	if formatted == "" {
		return "", fmt.Errorf("%w: invalid root-assignment formal state key", ErrInvalidLaneFactor)
	}
	return pathaddr.StateKey(formatted), nil
}

// RekeyRootAssignmentScalarFactorTransactionFormal maps the complete sealed
// scalar N4 transaction through the same root authority used by the tuple
// factors. A new key-bearing child cannot be added silently: this constructor
// owns every private field of RootAssignmentScalarTransfer.
func (d ProductDomain) RekeyRootAssignmentScalarFactorTransactionFormal(
	plan CoordinateFormalRootRekey,
	transaction RootAssignmentScalarFactorTransaction,
) (RootAssignmentScalarFactorTransaction, error) {
	if !plan.validFor(d) || !d.OwnsRootAssignmentScalarFactorTransaction(transaction) || transaction.transfer.keys != plan.from {
		return RootAssignmentScalarFactorTransaction{}, fmt.Errorf("%w: root-assignment scalar formal rekey", ErrInvalidLaneFactor)
	}
	transfer := transaction.transfer
	var err error
	transfer.target, err = d.RekeyStructuralKeyFormal(plan, transfer.target)
	if err != nil {
		return RootAssignmentScalarFactorTransaction{}, err
	}
	transfer.targetState = pathaddr.StateKey(plan.to.FormatReadOnly(transfer.target))
	if transfer.hasUserSource {
		transfer.userSource, err = d.RekeyStructuralKeyFormal(plan, transfer.userSource)
		if err != nil {
			return RootAssignmentScalarFactorTransaction{}, err
		}
	}
	for _, bound := range []*RootAssignmentNumBound{&transfer.numFloor, &transfer.numCeil} {
		if !bound.present || bound.exact {
			continue
		}
		bound.source, err = d.RekeyStructuralKeyFormal(plan, bound.source)
		if err != nil {
			return RootAssignmentScalarFactorTransaction{}, err
		}
		bound.sourceState = pathaddr.StateKey(plan.to.FormatReadOnly(bound.source))
	}
	transfer.keys = plan.to
	return RootAssignmentScalarFactorTransaction{seal: d.seal, transfer: transfer}, nil
}

// RekeyRootAssignmentCompletionFormal transports one already-sealed
// completion into the tuple namespace. The completion remains representation
// owned and is rebound to the same ProductDomain seal.
func (d ProductDomain) RekeyRootAssignmentCompletionFormal(
	plan CoordinateFormalRootRekey,
	transaction RootAssignmentFactorTransaction,
) (RootAssignmentFactorTransaction, error) {
	if !plan.validFor(d) || transaction.seal != d.seal || !transaction.completion.valid() {
		return RootAssignmentFactorTransaction{}, fmt.Errorf("%w: root-assignment completion formal rekey", ErrInvalidLaneFactor)
	}
	completion := transaction.completion
	var err error
	if completion.lenFloorKey.Kind != keyspace.KindInvalid {
		completion.lenFloorKey, err = d.RekeyStructuralKeyFormal(plan, completion.lenFloorKey)
		if err != nil {
			return RootAssignmentFactorTransaction{}, err
		}
	}
	for index := range completion.keyMemberships {
		membership := completion.keyMemberships[index]
		if membership.Key != "" {
			membership.Key, err = d.rekeyRootAssignmentStateKeyFormal(plan, membership.Key)
			if err != nil {
				return RootAssignmentFactorTransaction{}, err
			}
		}
		if membership.Container.Kind != keyspace.KindInvalid {
			membership.Container, err = d.RekeyStructuralKeyFormal(plan, membership.Container)
			if err != nil {
				return RootAssignmentFactorTransaction{}, err
			}
		}
		membership.Table, err = d.rekeyRootAssignmentStateKeyFormal(plan, membership.Table)
		if err != nil {
			return RootAssignmentFactorTransaction{}, err
		}
		completion.keyMemberships[index] = membership
	}
	return RootAssignmentFactorTransaction{seal: d.seal, completion: completion}, nil
}
