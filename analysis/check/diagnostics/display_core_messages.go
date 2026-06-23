package diagnostics

import (
	"fmt"
	"strings"

	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (diagnosticDisplay) Type(t typ.Type) string {
	if t == nil {
		return "unknown"
	}
	return typeformat.Short(t)
}

func (d diagnosticDisplay) AssignmentType(t typ.Type) string {
	if _, ok := t.(*typ.Optional); ok {
		return d.Type(t) + " (may be nil)"
	}
	return d.Type(t)
}

func (d diagnosticDisplay) SourceTypeEvidence(sourceName string, t typ.Type) string {
	return d.SourceTypeEvidenceDisplay(sourceName, t, "")
}

func (d diagnosticDisplay) SourceTypeEvidenceDisplay(sourceName string, t typ.Type, displayType string) string {
	if sourceName != "" && sourceName != unknownSourceName && t != nil && !typ.Nil.Equals(t) && projectionHasNil(t) {
		if present := projectionWithoutNil(t); present != nil && !typ.IsNever(present) {
			rendered := displayTypeWithoutNil(displayType)
			if rendered == "" {
				rendered = d.Type(present)
			}
			if rendered != "" && len(rendered) <= maxInlineTypeLen {
				return fmt.Sprintf("%s can be %s or nil here", sourceName, rendered)
			}
		}
		return fmt.Sprintf("%s can be nil here", sourceName)
	}
	return d.TypeObservationDisplay(sourceName, t, displayType)
}

func (d diagnosticDisplay) TypeObservation(sourceName string, t typ.Type) string {
	return d.TypeObservationDisplay(sourceName, t, "")
}

func (d diagnosticDisplay) TypeObservationDisplay(sourceName string, t typ.Type, displayType string) string {
	if lit, ok := t.(*typ.Literal); ok {
		return fmt.Sprintf("%s has literal value %s", sourceName, d.Type(lit))
	}
	return fmt.Sprintf("%s has type %s", sourceName, typeDisplayOr(displayType, d.AssignmentType(t)))
}

func (diagnosticDisplay) TruthyConditionCheck(path string) string {
	return fmt.Sprintf("%s is checked as truthy", path)
}

func (diagnosticDisplay) FalsyConditionCheck(path string) string {
	return fmt.Sprintf("%s is checked as falsy", path)
}

func (diagnosticDisplay) NilConditionCheck(path string) string {
	return fmt.Sprintf("%s == nil", path)
}

func (diagnosticDisplay) NonNilConditionCheck(path string) string {
	return fmt.Sprintf("%s ~= nil", path)
}

func (diagnosticDisplay) ConditionCheckEvidence(check string) string {
	return "current check: " + check
}

func (diagnosticDisplay) ConditionPathProofEvidence(path, state string) string {
	return fmt.Sprintf("prior guard established %s is %s", path, state)
}

func (diagnosticDisplay) ConditionTypeProofEvidence(path, runtimeType string) string {
	return fmt.Sprintf("prior guard established type(%s) is %q", path, runtimeType)
}

func (diagnosticDisplay) ConditionStabilityEvidence(path string) string {
	return fmt.Sprintf("%s is unchanged between the prior guard and this check", path)
}

func (diagnosticDisplay) ChannelSelectExhaustivenessMessage(caseWord, cases string) string {
	return fmt.Sprintf("channel select is not exhaustive; missing %s: %s", caseWord, cases)
}

func (diagnosticDisplay) ChannelSelectExhaustivenessHelp() string {
	return "Add an elseif branch for each missing case, or add a default branch when a fallback is valid."
}

func (diagnosticDisplay) DiscriminatedUnionExhaustivenessMessage(caseWord, cases string) string {
	return fmt.Sprintf("discriminated union handling is not exhaustive; missing %s: %s", caseWord, cases)
}

func (diagnosticDisplay) DiscriminatedUnionExhaustivenessHelp() string {
	return "Handle each missing case, or add an else branch when a fallback is valid."
}

func (diagnosticDisplay) DispatchTableExhaustivenessMessage(keyWord, keys string) string {
	return fmt.Sprintf("dispatch table is not exhaustive; missing %s: %s", keyWord, keys)
}

func (diagnosticDisplay) DispatchTableExhaustivenessHelp() string {
	return "Add each missing dispatch key, or route through an explicit fallback when missing keys are intentional."
}

func (diagnosticDisplay) RegistrationExhaustivenessMessage(registrationWord, registrations string) string {
	return fmt.Sprintf("registered callbacks are not exhaustive; missing %s: %s", registrationWord, registrations)
}

func (diagnosticDisplay) RegistrationExhaustivenessHelp() string {
	return "Register each missing case, or dispatch through an explicit fallback when missing registrations are intentional."
}

func (diagnosticDisplay) ResultShapeExhaustivenessMessage(readPath, requiredCase string) string {
	return fmt.Sprintf("case-specific field read is not exhaustive; %s requires %s", codeName(readPath), codeName(requiredCase))
}

func (diagnosticDisplay) ResultShapeExhaustivenessHelp() string {
	return "Check the union case before reading this field, or return from the opposite case before continuing."
}

func (diagnosticDisplay) OptionalExhaustivenessMessage(caseWord, cases string) string {
	return fmt.Sprintf("optional handling is not exhaustive; missing %s: %s", caseWord, cases)
}

func (diagnosticDisplay) OptionalExhaustivenessHelp() string {
	return "Handle the nil case with an else branch, or return before continuing when nil is intentionally ignored."
}

func (diagnosticDisplay) FrozenTableMutationMessage(containerName string) string {
	return fmt.Sprintf("cannot mutate frozen table %q", containerName)
}

func (diagnosticDisplay) FrozenTableCallMutationMessage(containerName string) string {
	return fmt.Sprintf("cannot call mutator on frozen table %q", containerName)
}

func (diagnosticDisplay) FrozenTableAssignmentHelp() string {
	return "Create a mutable copy before writing, or move this assignment before the table is frozen."
}

func (diagnosticDisplay) FrozenTableCallHelp() string {
	return "Create a mutable copy before calling the mutator, or call it before the table is frozen."
}

func (diagnosticDisplay) ResourceUnreleasedMessage(resourceName, protocol, current, final string) string {
	if strings.TrimSpace(current) == "" {
		return fmt.Sprintf("resource %s remains in a non-final %s state at function exit; expected %s", codeName(resourceName), protocol, lifecycleStateName(final))
	}
	return fmt.Sprintf("resource %s remains in %s state %s at function exit; expected %s", codeName(resourceName), protocol, lifecycleStateName(current), lifecycleStateName(final))
}

func (diagnosticDisplay) ResourceAcquireEvidence(resourceName, protocol, current, final string) string {
	return fmt.Sprintf("this call acquires %s as %s:%s and requires %s before local ownership ends", codeName(resourceName), protocol, lifecycleStateName(current), lifecycleStateName(final))
}

func (diagnosticDisplay) ResourceTransitionEvidence(resourceName, protocol, from, to string) string {
	if from == "" {
		return fmt.Sprintf("this call transitions %s in protocol %s to %s on a reachable path", codeName(resourceName), protocol, lifecycleStateName(to))
	}
	return fmt.Sprintf("this call transitions %s in protocol %s from %s to %s on a reachable path", codeName(resourceName), protocol, lifecycleStateName(from), lifecycleStateName(to))
}

func (diagnosticDisplay) ResourceEscapeEvidence(resourceName, protocol string) string {
	return fmt.Sprintf("this call escapes local ownership of %s in protocol %s on a reachable path", codeName(resourceName), protocol)
}

func (diagnosticDisplay) ResourceExitObligationEvidence(resourceName, protocol, current, final string) string {
	return fmt.Sprintf("exit state still has %s in protocol %s at %s; no proof reaches %s or escapes ownership on every path", codeName(resourceName), protocol, lifecycleStateName(current), lifecycleStateName(final))
}

func (diagnosticDisplay) ResourceUnreleasedHelp(resourceName, final string) string {
	return fmt.Sprintf("Transition %s to %s or escape ownership on every return path.", codeName(resourceName), lifecycleStateName(final))
}

func lifecycleStateName(state string) string {
	if strings.TrimSpace(state) == "" {
		return "a non-final state"
	}
	if strings.Contains(state, "`") {
		return state
	}
	return codeName(state)
}

func (diagnosticDisplay) UnresolvedTypeMessage(name string) string {
	return fmt.Sprintf("unknown type %s", name)
}

func (diagnosticDisplay) UnresolvedTypeEvidence(name string) string {
	return fmt.Sprintf("no type named %s is declared in this scope, a parent scope, or an imported module", name)
}

func (diagnosticDisplay) UnresolvedTypeHelp() string {
	return "Declare the type in scope, import the module that exports it, or use the fully qualified exported type name."
}

func (diagnosticDisplay) UnresolvedValueMessage(name string) string {
	return fmt.Sprintf("unknown value %s", name)
}

func (diagnosticDisplay) UnresolvedValueEvidence(name string) string {
	return fmt.Sprintf("no value named %s is declared, predeclared, imported, or configured global in this scope", name)
}

func (diagnosticDisplay) UnresolvedValueHelp() string {
	return "Declare the value, import it through require, or add it to the configured globals when it is intentionally ambient."
}

func (diagnosticDisplay) UnusedLocalMessage(name string) string {
	return fmt.Sprintf("local %q is never read", name)
}

func (diagnosticDisplay) UnusedLocalEvidence(name string) string {
	return fmt.Sprintf("no read of local %q was found in this scope", name)
}

func (diagnosticDisplay) UnusedLocalHelp() string {
	return "Remove it, use it, or rename it with a leading _ when intentionally unused."
}

func (diagnosticDisplay) RedundantConditionMessage(always bool) string {
	if always {
		return "condition is always true here"
	}
	return "condition is always false here"
}

func (diagnosticDisplay) RedundantConditionHelp(always bool) string {
	if always {
		return "Remove this repeated check, or move any needed work into the branch already guarded above."
	}
	return "Remove this unreachable branch, or change the prior guard if this path should still run."
}
