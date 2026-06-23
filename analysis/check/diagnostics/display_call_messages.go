package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (diagnosticDisplay) ReturnIndexedReadProofMessage(subject string) string {
	return fmt.Sprintf("%s is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared return type here", subject)
}

func (diagnosticDisplay) ReturnExplicitBoundaryProofMessage(subject string) string {
	return fmt.Sprintf("%s comes from any/unknown", subject)
}

func (diagnosticDisplay) ReturnMissingProofMessage(subject string) string {
	return fmt.Sprintf("no proof on this path shows %s satisfies the declared return type", subject)
}

func (diagnosticDisplay) CallResultAssignmentHelp(got typ.Type) string {
	if valueMayBeNil(got) {
		return "Guard the call result before assigning it, provide a default value, or change the target type to accept nil."
	}
	return "Assign the call result to a compatible target type, or change the callee return type if this result is valid."
}

func (d diagnosticDisplay) CallResultDeclaredReturnEvidence(name, label string, got typ.Type) string {
	if name == "" {
		name = "callee"
	}
	if label == "" {
		label = "call result"
	}
	return fmt.Sprintf("%s declares %s as %s", name, label, d.Type(got))
}

func (diagnosticDisplay) CallResultMissingNonNilProofMessage(label string) string {
	if label == "" {
		label = "call result"
	}
	return fmt.Sprintf("no guard on this path proves %s is non-nil before assignment", label)
}

func (diagnosticDisplay) PossiblyNilCallTargetMessage(name string) string {
	return fmt.Sprintf("cannot call %s because it may be nil", name)
}

func (d diagnosticDisplay) PossiblyNilCalleeTypeEvidence(name string, calleeType typ.Type, callable bool) string {
	if calleeType == nil {
		return fmt.Sprintf("%s may be nil at the call", name)
	}
	if callable {
		return fmt.Sprintf("%s has a callable type, but may also be nil", name)
	}
	if present := projectionWithoutNil(calleeType); present != nil && !typ.IsNever(present) {
		return fmt.Sprintf("%s can be %s or nil at the call", name, d.Type(present))
	}
	return fmt.Sprintf("%s can be nil at the call", name)
}

func (diagnosticDisplay) MissingNonNilBeforeCallMessage(name string) string {
	return fmt.Sprintf("no guard on this path proves %s is non-nil before this call", name)
}

func (diagnosticDisplay) PossiblyNilCallTargetHelp(name string) string {
	if name != "" && name != "call target" {
		return fmt.Sprintf("Guard `%s` with a nil check before calling it.", name)
	}
	return "Guard the call target with a nil check before calling it."
}

func (diagnosticDisplay) OptionalMethodCallMessage() string {
	return "cannot call method on an optional value without a nil check"
}

func (diagnosticDisplay) OptionalMethodReceiverEvidence(subject, target string) string {
	if target = strings.TrimSpace(target); target != "" {
		return fmt.Sprintf("%s is optional %s", subject, target)
	}
	return fmt.Sprintf("%s is optional", subject)
}

func (diagnosticDisplay) OptionalMethodMissingNilCheckEvidence(guardSubject, callTarget string) string {
	return fmt.Sprintf("no nil check proves %s is present before %s", guardSubject, callTarget)
}

func (diagnosticDisplay) OptionalMethodCallHelp(receiverName, callName string) string {
	if receiverName != "" && callName != "" {
		return fmt.Sprintf("check %s ~= nil before calling %s.", receiverName, callName)
	}
	if receiverName != "" {
		return fmt.Sprintf("check %s ~= nil before calling a method on it.", receiverName)
	}
	return "check the receiver for nil before calling a method on it."
}

func (diagnosticDisplay) CallArityMismatchMessage(name string, want, got int) string {
	return fmt.Sprintf("%s expects %d %s, got %d", name, want, pluralize(want, "argument", "arguments"), got)
}

func (diagnosticDisplay) CallArgumentCountEvidence(name string, got int) string {
	return fmt.Sprintf("call to %s passes %d %s", name, got, pluralize(got, "argument", "arguments"))
}

func (diagnosticDisplay) CallParameterCountEvidence(name string, want int) string {
	return fmt.Sprintf("%s declares %d %s", name, want, pluralize(want, "parameter", "parameters"))
}

func (d diagnosticDisplay) CallParameterTypeEvidence(name string, index int, suffix string, want typ.Type) string {
	return d.CallParameterTypeEvidenceDisplay(name, index, suffix, want, "")
}

func (d diagnosticDisplay) CallParameterTypeEvidenceDisplay(name string, index int, suffix string, want typ.Type, displayType string) string {
	return fmt.Sprintf("%s parameter %d%s expects %s", name, index, suffix, typeDisplayOr(displayType, d.Type(want)))
}

func (d diagnosticDisplay) CallParamObligationEvidence(name string, subject string, want typ.Type) string {
	subject = obligationSubjectDisplay(subject)
	return fmt.Sprintf("inside %s, %s must satisfy %s", name, subject, d.Type(want))
}

func (d diagnosticDisplay) MemberCallParamObligationEvidence(name string, subject string, provider string, memberParam int, want typ.Type) string {
	subject = obligationSubjectDisplay(subject)
	return fmt.Sprintf("inside %s, %s is passed to %s parameter %d, which requires %s", name, subject, provider, memberParam, d.Type(want))
}

func obligationSubjectDisplay(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "the argument"
	}
	return subject
}

func (diagnosticDisplay) CallArityHelp(want, got int) string {
	if got < want {
		return "Pass the missing required arguments, or change the callee signature if fewer arguments are valid."
	}
	return "Remove the extra argument, or change the callee signature if the extra argument is valid."
}

func (d diagnosticDisplay) NumericForOperandMessage(role string, got typ.Type) string {
	return fmt.Sprintf("numeric for %s must be number, got %s", role, d.Type(got))
}

func (d diagnosticDisplay) NumericForOperandTypeEvidence(role string, got typ.Type) string {
	return d.SourceTypeEvidence(role, got)
}

func (diagnosticDisplay) NumericForOperandHelp(role string) string {
	return fmt.Sprintf("Use a number for the numeric for %s, or convert it before the loop.", role)
}

func (diagnosticDisplay) DeadAssignmentMessage(name string, hasExit bool) string {
	if hasExit {
		return fmt.Sprintf("assignment to %q is discarded before it is read", name)
	}
	return fmt.Sprintf("assignment to %q is overwritten before it is read", name)
}

func (diagnosticDisplay) DeadAssignmentOverwriteEvidence(name string) string {
	return fmt.Sprintf("later assignment replaces %q before the earlier value is read", name)
}

func (diagnosticDisplay) DeadAssignmentExitEvidence(name string) string {
	return fmt.Sprintf("control can leave before %q is read", name)
}

func (diagnosticDisplay) DeadAssignmentHelp(name string, hasExit bool) string {
	if hasExit {
		return fmt.Sprintf("Remove this assignment, or read `%s` before every later overwrite or exit.", name)
	}
	return fmt.Sprintf("Remove this assignment, or read `%s` before the later overwrite.", name)
}

func (diagnosticDisplay) ConcatOperandMessage(side string) string {
	if side == "" {
		return "operand of `..` may be nil"
	}
	return fmt.Sprintf("%s operand of `..` may be nil", side)
}

func (d diagnosticDisplay) ConcatOperandTypeEvidence(side, name string, got typ.Type) string {
	if name != "" {
		return d.SourceTypeEvidence(concatOperandSubject(side, name), got)
	}
	return d.SourceTypeEvidence(side+" operand", got)
}

func concatOperandSubject(side, name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == unknownSourceName {
		return operandLabel(side)
	}
	return operandLabel(side) + " " + codeName(name)
}

func (diagnosticDisplay) NonNilAssertAlwaysNilMessage(name string) string {
	if name != "" && name != unknownSourceName {
		return fmt.Sprintf("%s is asserted non-nil but is always nil", codeName(name))
	}
	return "value is asserted non-nil but is always nil"
}

func (d diagnosticDisplay) NonNilAssertAlwaysNilEvidence(name string) string {
	subject := "the asserted value"
	if name != "" && name != unknownSourceName {
		subject = codeName(name)
	}
	return fmt.Sprintf("%s is nil on every path here, so the `!` assertion always fails at runtime", subject)
}

func (diagnosticDisplay) NonNilAssertAlwaysNilHelp(name string) string {
	name = strings.TrimSpace(name)
	if name != "" && name != unknownSourceName {
		return fmt.Sprintf("Remove the `!` assertion on `%s` or assign a non-nil value before this point.", name)
	}
	return "Remove the `!` assertion or assign a non-nil value before this point."
}

func (diagnosticDisplay) ConcatOperandHelp(name string) string {
	name = strings.TrimSpace(name)
	if name != "" && name != unknownSourceName {
		return fmt.Sprintf("Guard `%s` or provide a default string before using `..`.", name)
	}
	return "Guard the value or provide a default string before using `..`."
}
