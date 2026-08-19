package containment

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// validateStaticCrossOwnerCardinalities closes the Static sidecars whose
// denominators are owned by Flow. Static validates each sidecar against its
// own authored input; this boundary reconciles those rows with the live Flow
// families before any containment producer can consume them.
func validateStaticCrossOwnerCardinalities(
	staticView staticquery.View,
	view authored.View,
	counts [keyspace.FamilyCount]uint32,
) error {
	if !staticView.Available() {
		return errors.New("program/flow/containment: Static cross-owner view expired")
	}
	if !view.Cold().ContentID().Available() {
		return errors.New("program/flow/containment: Flow cross-owner view expired")
	}
	if view.Claims().Count() != int(counts[keyspace.FamilyValueClaim]) {
		return errors.New("program/flow/containment: Flow ValueClaim cardinality mismatch")
	}
	if functions := staticView.Contracts().Functions(); functions.Count() != int(counts[keyspace.FamilyFunction]) {
		return errors.New("program/flow/containment: Static Function contract cardinality mismatch")
	}
	if calls := staticView.Contracts().Calls(); calls.Count() != int(counts[keyspace.FamilyCall]) {
		return errors.New("program/flow/containment: Static Call contract cardinality mismatch")
	}
	if typeValues := staticView.Operands().TypeValues(); typeValues.Count() != int(counts[keyspace.FamilyTypeValue]) {
		return errors.New("program/flow/containment: Static TypeValue target cardinality mismatch")
	}

	flowClaims := view.Claims()
	staticClaims := staticView.Operands().Claims()
	expectedTargets := 0
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyValueClaim]; ordinal++ {
		claim := keyspace.MakeTerm(keyspace.FamilyValueClaim, ordinal)
		_, _, claimKind, ok := flowClaims.Get(claim)
		if !ok {
			return errors.New("program/flow/containment: missing Flow ValueClaim")
		}
		target, hasTarget := staticClaims.Target(claim)
		requiresTarget := false
		switch claimKind {
		case kind.ValueClaimTypeAs, kind.ValueClaimTypeColonColon:
			requiresTarget = true
		case kind.ValueClaimNonNil:
		default:
			return errors.New("program/flow/containment: invalid Flow ValueClaim kind")
		}
		if requiresTarget != hasTarget || (hasTarget && !validTerm(target, counts)) {
			return errors.New("program/flow/containment: Static ValueClaim target disagrees with Flow claim kind")
		}
		if hasTarget {
			expectedTargets++
		}
	}
	if staticClaims.Count() != expectedTargets {
		return errors.New("program/flow/containment: Static ValueClaim target cardinality mismatch")
	}
	for index := 0; index < staticClaims.Count(); index++ {
		claim, ok := staticClaims.At(index)
		if !ok || keyspace.TermFamily(claim) != keyspace.FamilyValueClaim ||
			keyspace.TermOrdinal(claim) == 0 || keyspace.TermOrdinal(claim) > counts[keyspace.FamilyValueClaim] {
			return errors.New("program/flow/containment: Static ValueClaim target escapes Flow denominator")
		}
	}
	return nil
}
