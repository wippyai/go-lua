package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

// callOutcomeRoleSources is the exhaustive neutral publication contract. The
// callpayload registry supplies order and cardinality; this table only assigns
// ownership. Adding a CallOutcome field without a producer rejects evaluation.
var callOutcomeRoleSources = map[string]evaluated.CallOutcomeRoleSource{
	"Results":                    evaluated.CallOutcomeRoleDirect,
	"PostReturnAuthority":        evaluated.CallOutcomeRoleSummary,
	"SuspensionKnown":            evaluated.CallOutcomeRoleSummary,
	"MaySuspend":                 evaluated.CallOutcomeRoleSummary,
	"NormalReturnFacts":          evaluated.CallOutcomeRoleSummary,
	"ProtectedCallTypestate":     evaluated.CallOutcomeRoleSummary,
	"HeapTableObjects":           evaluated.CallOutcomeRoleSummary,
	"Placements":                 evaluated.CallOutcomeRoleSummary,
	"ParamObligations":           evaluated.CallOutcomeRoleSummary,
	"PathObligations":            evaluated.CallOutcomeRoleSummary,
	"TypestateRequirements":      evaluated.CallOutcomeRoleNeutralEffect,
	"ParamPathRefinements":       evaluated.CallOutcomeRoleSummary,
	"ParamPathWrites":            evaluated.CallOutcomeRoleNeutralEffect,
	"ParamLengthFloors":          evaluated.CallOutcomeRoleNeutralEffect,
	"ParamPathInvalidations":     evaluated.CallOutcomeRoleNeutralEffect,
	"ParamConditions":            evaluated.CallOutcomeRoleSummary,
	"ParamPathRelations":         evaluated.CallOutcomeRoleSummary,
	"ReturnConditionRefinements": evaluated.CallOutcomeRoleSummary,
	"ReturnConditionSlots":       evaluated.CallOutcomeRoleSummary,
	"ReturnPresenceRelations":    evaluated.CallOutcomeRoleSummary,
	"ParamExposures":             evaluated.CallOutcomeRoleSummary,
}

func sealedCallOutcomeRoles() ([]evaluated.CallOutcomeRole, error) {
	registered := callpayload.CallOutcomeFieldRoles()
	out := make([]evaluated.CallOutcomeRole, 0, len(registered))
	seen := make(map[string]struct{}, len(registered))
	for _, role := range registered {
		source, ok := callOutcomeRoleSources[role.FieldName]
		if !ok || source == evaluated.CallOutcomeRoleInvalid || source == evaluated.CallOutcomeRoleUnsupported {
			return nil, fmt.Errorf("transformer: call-outcome role %q has no neutral producer", role.FieldName)
		}
		seen[role.FieldName] = struct{}{}
		out = append(out, evaluated.CallOutcomeRole{FieldName: role.FieldName, Source: source})
	}
	for field := range callOutcomeRoleSources {
		if _, ok := seen[field]; !ok {
			return nil, fmt.Errorf("transformer: orphan call-outcome role producer %q", field)
		}
	}
	return out, nil
}
