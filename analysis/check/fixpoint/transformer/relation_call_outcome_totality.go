package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

type relationCallOutcomeOwner uint8

const (
	relationCallOutcomeOwnerInvalid relationCallOutcomeOwner = iota
	relationCallOutcomeTuple
	relationCallOutcomeDiagnostics
	relationCallOutcomeStateProjection
	relationCallOutcomeCorrelation
	relationCallOutcomeDerivedAuthority
	// These compatibility DTOs are not a second lexical fact path. Their
	// semantics are carried by the descriptor-total NormalReturnFacts projection
	// from stabilized State (or, for entry typestate requirements, by external
	// signature providers). A lexical RelationProgram outcome must leave them
	// empty.
	relationCallOutcomeCanonicalNormalFacts
	relationCallOutcomeExternalProvider
)

// relationCallOutcomeOwners is deliberately exhaustive over the canonical
// CallOutcome descriptor registry. Adding, removing, or renaming a field
// without assigning one replacement-engine owner fails at package init rather
// than silently publishing a partial lexical outcome.
var relationCallOutcomeOwners = bindRelationCallOutcomeOwners()

func bindRelationCallOutcomeOwners() map[string]relationCallOutcomeOwner {
	handlers := map[string]relationCallOutcomeOwner{
		"Results":                    relationCallOutcomeTuple,
		"PostReturnAuthority":        relationCallOutcomeDerivedAuthority,
		"SuspensionKnown":            relationCallOutcomeDiagnostics,
		"MaySuspend":                 relationCallOutcomeDiagnostics,
		"NormalReturnFacts":          relationCallOutcomeStateProjection,
		"ProtectedCallTypestate":     relationCallOutcomeTuple,
		"HeapTableObjects":           relationCallOutcomeStateProjection,
		"Placements":                 relationCallOutcomeStateProjection,
		"ParamObligations":           relationCallOutcomeDiagnostics,
		"PathObligations":            relationCallOutcomeDiagnostics,
		"TypestateRequirements":      relationCallOutcomeExternalProvider,
		"ParamPathRefinements":       relationCallOutcomeStateProjection,
		"ParamPathWrites":            relationCallOutcomeCanonicalNormalFacts,
		"ParamLengthFloors":          relationCallOutcomeCanonicalNormalFacts,
		"ParamPathInvalidations":     relationCallOutcomeCanonicalNormalFacts,
		"ParamConditions":            relationCallOutcomeStateProjection,
		"ParamPathRelations":         relationCallOutcomeStateProjection,
		"ReturnConditionRefinements": relationCallOutcomeTuple,
		"ReturnConditionSlots":       relationCallOutcomeCorrelation,
		"ReturnPresenceRelations":    relationCallOutcomeCorrelation,
		"ParamExposures":             relationCallOutcomeDiagnostics,
	}
	descriptors := callpayload.CallOutcomeDescriptors()
	if len(handlers) != len(descriptors) {
		panic(fmt.Sprintf("transformer: lexical CallOutcome ownership width %d differs from descriptor width %d", len(handlers), len(descriptors)))
	}
	seen := make(map[string]struct{}, len(descriptors))
	for _, descriptor := range descriptors {
		name := string(descriptor.Kind)
		owner, ok := handlers[name]
		if !ok || owner <= relationCallOutcomeOwnerInvalid || owner > relationCallOutcomeExternalProvider {
			panic(fmt.Sprintf("transformer: CallOutcome field %q has no lexical ownership", name))
		}
		if _, duplicate := seen[name]; duplicate {
			panic(fmt.Sprintf("transformer: duplicate CallOutcome descriptor %q", name))
		}
		seen[name] = struct{}{}
	}
	for name := range handlers {
		if _, ok := seen[name]; !ok {
			panic(fmt.Sprintf("transformer: orphan lexical CallOutcome owner %q", name))
		}
	}
	return handlers
}

func validateRelationCallOutcomeCanonicalLanes(out callpayload.CallOutcome) error {
	if len(out.TypestateRequirements) != 0 {
		return fmt.Errorf("transformer: lexical outcome published external-provider typestate requirements")
	}
	if len(out.ParamPathWrites) != 0 || len(out.ParamLengthFloors) != 0 || len(out.ParamPathInvalidations) != 0 {
		return fmt.Errorf("transformer: lexical outcome published a compatibility lane beside canonical NormalReturnFacts")
	}
	return nil
}
