package evaluated

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// CallOutcomeRoleSource identifies the neutral producer which owns one
// CallOutcome field. It is intentionally not a CallOutcome value: caller-state,
// signature, allocation-identity, and member-sensitive lowering remains at the
// program/check boundary.
type CallOutcomeRoleSource uint8

const (
	CallOutcomeRoleInvalid CallOutcomeRoleSource = iota
	CallOutcomeRoleDirect
	CallOutcomeRoleSummary
	CallOutcomeRoleNeutralEffect
	CallOutcomeRoleUnsupported
)

// CallOutcomeRole is one registry-ordered field provenance certificate.
type CallOutcomeRole struct {
	FieldName string
	Source    CallOutcomeRoleSource
}

// CallOutcomeFragment is one exact guarded application of a final lexical
// relation. Results are repeated in indexed form because they are the direct
// call-producer payload; Summary owns every remaining summary-derived fact.
type CallOutcomeFragment struct {
	Worlds  WorldSet
	Results []IndexedValue
	Summary summary.Summary
	Roles   []CallOutcomeRole
}

// CallOutcomeBoundary is the owner-local call occurrence paired with its
// exact final lexical target. No post-call CFG row is retained or consulted.
type CallOutcomeBoundary struct {
	Slot       uint32
	Point      cfg.Point
	Owner      lexicalidentity.StableLexicalBodyID
	Occurrence engineobservation.Occurrence
	Target     lexicalidentity.StableLexicalBodyID
	Fragments  []CallOutcomeFragment
}

func validCallOutcomeRoles(roles []CallOutcomeRole) error {
	registered := callpayload.CallOutcomeFieldRoles()
	if len(roles) != len(registered) {
		return fmt.Errorf("evaluated: call-outcome role seal has %d/%d roles", len(roles), len(registered))
	}
	for index, role := range roles {
		if role.FieldName != registered[index].FieldName {
			return fmt.Errorf("evaluated: call-outcome role %d is not registry ordered", index)
		}
		switch role.Source {
		case CallOutcomeRoleDirect, CallOutcomeRoleSummary, CallOutcomeRoleNeutralEffect:
		case CallOutcomeRoleUnsupported:
			return fmt.Errorf("evaluated: call-outcome role %q is unsupported", role.FieldName)
		default:
			return fmt.Errorf("evaluated: call-outcome role %q has no producer", role.FieldName)
		}
	}
	return nil
}

func cloneCallOutcomeRoles(in []CallOutcomeRole) []CallOutcomeRole {
	return append([]CallOutcomeRole(nil), in...)
}

func cloneCallOutcomes(in []CallOutcomeBoundary) []CallOutcomeBoundary {
	out := make([]CallOutcomeBoundary, len(in))
	for index, boundary := range in {
		out[index] = boundary
		out[index].Fragments = make([]CallOutcomeFragment, len(boundary.Fragments))
		for fragmentIndex, fragment := range boundary.Fragments {
			out[index].Fragments[fragmentIndex] = CallOutcomeFragment{
				Worlds: fragment.Worlds, Results: append([]IndexedValue(nil), fragment.Results...),
				Summary: fragment.Summary.Clone(), Roles: cloneCallOutcomeRoles(fragment.Roles),
			}
		}
	}
	return out
}
