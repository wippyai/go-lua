package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/readmodel"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (diagnosticDisplay) Type(t typ.Type) string {
	if t == nil {
		return "unknown"
	}
	if readmodel.ProjectionHasNil(t) {
		if present := readmodel.ProjectionWithoutNil(t); present != nil && !typ.IsNever(present) && typ.AbsentOrTopLike(present) {
			return typeformat.Short(present)
		}
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
	if sourceName != "" && sourceName != unknownSourceName && t != nil && !typ.Nil.Equals(t) && readmodel.ProjectionHasNil(t) {
		if present := readmodel.ProjectionWithoutNil(t); present != nil && !typ.IsNever(present) {
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

func (diagnosticDisplay) ConditionStabilityEvidence(path string) string {
	return fmt.Sprintf("%s is unchanged between the prior guard and this check", path)
}

func (diagnosticDisplay) ChannelSelectExhaustivenessMessage(caseWord, cases string) string {
	return fmt.Sprintf("channel select is not exhaustive; missing %s: %s", caseWord, cases)
}

func (diagnosticDisplay) ChannelSelectExhaustivenessHelp() string {
	return "Add an elseif branch for each missing case, or add a default branch when a fallback is valid."
}

func (diagnosticDisplay) ChannelLifecycleMessage(operation, channel string) string {
	switch operation {
	case "close":
		return "cannot close already closed channel " + codeName(channel)
	default:
		return "cannot send on closed channel " + codeName(channel)
	}
}

func (diagnosticDisplay) ChannelLifecycleClosedEvidence(operation, channel string) string {
	switch operation {
	case "close":
		return "this close call runs after " + codeName(channel) + " is proven closed"
	default:
		return "this send call runs after " + codeName(channel) + " is proven closed"
	}
}

func (diagnosticDisplay) ChannelLifecycleHelp(operation string) string {
	switch operation {
	case "close":
		return "Avoid closing the same channel twice, or guard ownership before closing it."
	default:
		return "Send before closing the channel, or use a fresh channel for later sends."
	}
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

func (d diagnosticDisplay) AdviceRedundantClaimMessage(claim string, t typ.Type) string {
	if claim == "" {
		claim = "type claim"
	}
	return fmt.Sprintf("%s is redundant; value is already %s", claim, d.Type(t))
}

func (diagnosticDisplay) AdviceRedundantClaimHelp() string {
	return "Remove the runtime type claim when the proven source type is sufficient."
}

func (diagnosticDisplay) AdviceAlwaysTrueGuardMessage(always bool) string {
	if always {
		return "condition is proven always true"
	}
	return "condition is proven always false"
}

func (diagnosticDisplay) AdviceAlwaysTrueGuardHelp(always bool) string {
	if always {
		return "Remove the guard or move the guarded code out of the branch."
	}
	return "Remove the unreachable branch or change the condition if it should still run."
}

func (diagnosticDisplay) AdviceInvariantLoopReadMessage(read string) string {
	if read == "" {
		return "loop read is invariant and can be hoisted"
	}
	return fmt.Sprintf("%s is loop-invariant and can be hoisted", read)
}

func (diagnosticDisplay) AdviceInvariantLoopReadHelp(read string) string {
	if read == "" {
		return "Read the value once before the loop when that makes the code clearer or cheaper."
	}
	return fmt.Sprintf("Read `%s` once before the loop when that makes the code clearer or cheaper.", read)
}

func (diagnosticDisplay) AdviceSplitBirthDiscriminantMessage(tag string) string {
	if tag == "" {
		return "discriminant tag is assigned apart from its payload"
	}
	return fmt.Sprintf("%s is assigned apart from its payload", tag)
}

func (diagnosticDisplay) AdviceSplitBirthDiscriminantHelp() string {
	return "Construct the variant in one table literal so the tag and payload are born atomically."
}

func (diagnosticDisplay) AdviceShapePolymorphicMessage(receiver string) string {
	if receiver == "" {
		return "table has a path-dependent field shape"
	}
	return fmt.Sprintf("%s has a path-dependent field shape", receiver)
}

func (diagnosticDisplay) AdviceShapePolymorphicHelp() string {
	return "Construct all variants with one fixed-shape constructor (all fields present, absent ones nil/default)."
}
