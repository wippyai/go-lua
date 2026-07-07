package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (d diagnosticDisplay) DeclaredTypeEvidence(name string, annotation ast.TypeExpr, fallback typ.Type) string {
	if strings.ContainsAny(name, ".[") {
		return fmt.Sprintf("%s is declared as %s", name, d.Type(fallback))
	}
	return fmt.Sprintf("%s is declared as %s", name, d.AnnotationOrType(annotation, fallback))
}

func (d diagnosticDisplay) AssignmentTargetTypeEvidence(targetName string, want typ.Type) string {
	if targetName == "" || targetName == unknownSourceName {
		return fmt.Sprintf("assignment target requires %s", d.Type(want))
	}
	return fmt.Sprintf("assignment target %s requires %s", targetName, d.Type(want))
}

func (d diagnosticDisplay) ReassignedCallResultFieldEvidence(rootName, readName string, replacement typ.Type) string {
	if rootName == "" {
		rootName = "call result"
	}
	if readName == "" {
		readName = "the read"
	}
	if replacement != nil && !typ.IsAny(replacement) && !typ.IsUnknown(replacement) {
		if lit, ok := replacement.(*typ.Literal); ok {
			return fmt.Sprintf("%s is reassigned before the read; after that assignment, %s has literal value %s", rootName, readName, d.Type(lit))
		}
		return fmt.Sprintf("%s is reassigned before the read; after that assignment, %s has type %s", rootName, readName, d.Type(replacement))
	}
	return fmt.Sprintf("%s is reassigned before the read; %s may use that later assignment", rootName, readName)
}

func (diagnosticDisplay) ArgumentValidationProofHelp(argName string) string {
	if argName != "" && argName != unknownSourceName {
		return fmt.Sprintf("Validate or narrow `%s` before passing it; any/unknown values do not prove parameter contracts.", argName)
	}
	return "Validate or narrow this argument before passing it; any/unknown values do not prove parameter contracts."
}

func (diagnosticDisplay) ExplicitBoundaryProofMessage(_ typ.Type) string {
	return "assigned value comes from any/unknown"
}

func (diagnosticDisplay) ExplicitBoundaryProofMessageForSubject(subject string, _ typ.Type) string {
	return fmt.Sprintf("%s comes from any/unknown", boundaryEvidenceSubject(subject))
}

func (diagnosticDisplay) UserAssertedAnyEvidence() string {
	return "user asserted any; not abstract-interpreter proof"
}

func (d diagnosticDisplay) MissingBoundaryProofMessage(want typ.Type) string {
	return "no proof on this path shows assigned value is " + d.Type(want)
}

func (d diagnosticDisplay) MissingBoundaryProofMessageForSubject(subject string, want typ.Type) string {
	return fmt.Sprintf("no proof on this path shows %s is %s", boundaryEvidenceSubject(subject), d.Type(want))
}

func (d diagnosticDisplay) MissingIndexReadProofMessage(want typ.Type) string {
	return "indexed read can miss or read nil; no proof shows the selected slot satisfies " + d.Type(want) + " here"
}

func boundaryEvidenceSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" || subject == unknownSourceName {
		return "assigned value"
	}
	return subject
}

func (d diagnosticDisplay) MissingMemberMessage(receiver typ.Type, member string) string {
	return fmt.Sprintf("%s has no member %q", d.Type(receiver), member)
}

func (d diagnosticDisplay) MemberReadReceiverEvidence(readPath, member string, receiver typ.Type) string {
	if readPath != "" {
		return fmt.Sprintf("%s reads member %q from receiver type %s", readPath, member, d.Type(receiver))
	}
	return fmt.Sprintf("receiver type %s does not provide member %q", d.Type(receiver), member)
}

func (d diagnosticDisplay) ReceiverForMemberEvidence(memberPath string, receiver typ.Type) string {
	return fmt.Sprintf("%s has receiver type %s", memberPath, d.Type(receiver))
}

func (diagnosticDisplay) MissingMemberHelp(member string) string {
	if member != "" {
		return fmt.Sprintf("Narrow the receiver before reading `%s`, or add `%s` to every reachable receiver shape.", member, member)
	}
	return "Narrow the receiver before the read, or add the member to every reachable receiver shape."
}

func (d diagnosticDisplay) MemberNotCallableMessage(memberPath string, receiver, memberType typ.Type, member string) string {
	if typ.AbsentOrTopLike(memberType) {
		if memberPath != "" && memberPath != "receiver" {
			return fmt.Sprintf("%s comes from any/unknown; no proof shows it is callable", memberPath)
		}
		return fmt.Sprintf("%s comes from any/unknown; no proof shows it is callable", memberPathName(d.Type(receiver), member))
	}
	if memberPath != "" && memberPath != "receiver" {
		return fmt.Sprintf("%s is %s, not callable", memberPath, d.Type(memberType))
	}
	return fmt.Sprintf("%s is %s, not callable", memberPathName(d.Type(receiver), member), d.Type(memberType))
}

func (d diagnosticDisplay) MemberTypeAtCallEvidence(memberPath string, memberType typ.Type) string {
	return d.TypeObservation(memberPath, memberType) + " at call"
}

func (diagnosticDisplay) MemberNotCallableHelp(memberPath string) string {
	if memberPath != "" && memberPath != "receiver" {
		return fmt.Sprintf("Narrow `%s` to a function-valued member before calling it, or call a different member.", memberPath)
	}
	return "Narrow the member to a function before calling it, or call a different member."
}

func (d diagnosticDisplay) DirectNotCallableMessage(name string, calleeType typ.Type) string {
	if typ.AbsentOrTopLike(calleeType) {
		return fmt.Sprintf("%s comes from any/unknown; no proof shows it is callable", name)
	}
	return fmt.Sprintf("%s is %s, not callable", name, d.Type(calleeType))
}

func (diagnosticDisplay) DirectNotCallableHelp(name string) string {
	if name != "" && name != "call target" {
		return fmt.Sprintf("Call a function value, or replace `%s` with a callable expression before this call.", name)
	}
	return "Call a function value, or replace the target with a callable expression before this call."
}

func (d diagnosticDisplay) AnnotatedTypeEvidence(name string, t typ.Type) string {
	return fmt.Sprintf("%s is annotated %s", name, d.Type(t))
}

func (d diagnosticDisplay) AssignmentMessage(sourceName string, got, want typ.Type) string {
	return d.AssignmentMessageDisplay(sourceName, got, want, "")
}

func (d diagnosticDisplay) AssignmentMessageDisplay(sourceName string, got, want typ.Type, wantDisplay string) string {
	if wantDisplay == "" {
		wantDisplay = d.Type(want)
	}
	if sourceName != "" && sourceName != unknownSourceName {
		return fmt.Sprintf("cannot assign %s because it is %s, not %s", sourceName, d.Type(got), wantDisplay)
	}
	return fmt.Sprintf("cannot assign %s to %s", d.Type(got), wantDisplay)
}

func (d diagnosticDisplay) MemberAssignmentMessage(memberName string, sourceName string, got, want typ.Type) string {
	return d.MemberAssignmentMessageDisplay(memberName, sourceName, got, want, "")
}

func (d diagnosticDisplay) MemberAssignmentMessageDisplay(memberName string, sourceName string, got, want typ.Type, wantDisplay string) string {
	if wantDisplay == "" {
		wantDisplay = d.Type(want)
	}
	if sourceName == "" || sourceName == unknownSourceName {
		return fmt.Sprintf("cannot assign %s because assigned value is %s, not %s", memberName, d.Type(got), wantDisplay)
	}
	return fmt.Sprintf("cannot assign %s to %s because %s is %s, not %s", sourceName, memberName, sourceName, d.Type(got), wantDisplay)
}

func (diagnosticDisplay) AssignmentHelp(sourceName string, missingNilProof bool) string {
	if sourceName != "" && sourceName != unknownSourceName && missingNilProof {
		return fmt.Sprintf("Guard `%s` with a nil check, provide a default value, or change the target type to accept nil.", sourceName)
	}
	if sourceName != "" && sourceName != unknownSourceName {
		return fmt.Sprintf("Use a value compatible with the expected type, or change the target type if `%s` is valid.", sourceName)
	}
	return "Use a value compatible with the expected type, or change the target type if the assigned value is valid."
}

func (diagnosticDisplay) UnderSuppliedTargetEvidence(name, sourceName string, resultIndex int) string {
	if sourceName != "" && sourceName != unknownSourceName && resultIndex >= 0 {
		return fmt.Sprintf("%s receives result %d from `%s`, but no value was produced for that result slot", name, resultIndex+1, sourceName)
	}
	return fmt.Sprintf("%s has no supplied value in this assignment, so Lua fills it with nil", name)
}

func (diagnosticDisplay) UnderSuppliedTargetHelp(name string) string {
	return fmt.Sprintf("Provide a value for `%s`, remove the extra target, or change the target type to accept nil.", name)
}

func (diagnosticDisplay) OptionalAssignmentTargetMessage(containerName string) string {
	if containerName != "" && containerName != unknownSourceName {
		return fmt.Sprintf("cannot assign through optional %s without nil check", containerName)
	}
	return "cannot assign through an optional value without a nil check"
}

func (d diagnosticDisplay) OptionalAssignmentTargetContainerEvidence(containerName string, containerType typ.Type) string {
	if containerName != "" && containerName != unknownSourceName {
		return d.SourceTypeEvidence(containerName, containerType)
	}
	return fmt.Sprintf("assignment container has type %s and may be nil", d.Type(containerType))
}

func (diagnosticDisplay) OptionalAssignmentTargetWriteEvidence(targetName string) string {
	if targetName != "" && targetName != unknownSourceName {
		return fmt.Sprintf("writing %s requires its container to be non-nil", targetName)
	}
	return "this write requires the container to be non-nil"
}

func (diagnosticDisplay) OptionalAssignmentTargetHelp(containerName string) string {
	if containerName != "" && containerName != unknownSourceName {
		return fmt.Sprintf("Guard `%s` with a nil check before assigning through it, or write to a non-optional container.", containerName)
	}
	return "Guard the container with a nil check before assigning through it, or write to a non-optional container."
}

func (diagnosticDisplay) MissingRequiredFieldMessage(field string) string {
	return fmt.Sprintf("object literal is missing required field %q", field)
}

func (diagnosticDisplay) MissingRequiredFieldEvidence(field string) string {
	return fmt.Sprintf("object literal does not provide field %q", field)
}

func (d diagnosticDisplay) MissingRequiredFieldPathEvidence(path string, t typ.Type) string {
	if path == "" {
		return d.MissingRequiredFieldEvidence("")
	}
	return fmt.Sprintf("required field %s has type %s, but the object literal does not provide it", path, d.Type(t))
}

func (d diagnosticDisplay) MissingRequiredMethodMessage(contract typ.Type, method string) string {
	if contract == nil {
		return fmt.Sprintf("object literal is missing required method %q", method)
	}
	return fmt.Sprintf("object literal does not implement %s: missing method %q", d.Type(contract), method)
}

func (d diagnosticDisplay) MissingRequiredMethodEvidence(contract typ.Type, method string) string {
	if contract == nil {
		return fmt.Sprintf("object literal does not provide method %q", method)
	}
	return fmt.Sprintf("object literal does not provide method %q required by %s", method, d.Type(contract))
}

func (d diagnosticDisplay) MissingRequiredMethodTypeEvidence(contract typ.Type, method typ.Method) string {
	if method.Name == "" || method.Type == nil {
		return d.MissingRequiredMethodEvidence(contract, method.Name)
	}
	return fmt.Sprintf("required method %s has type %s, but the object literal does not provide it", method.Name, d.Type(method.Type))
}

func (d diagnosticDisplay) MethodTypeMismatchMessage(contract typ.Type, method string, got, want typ.Type) string {
	if contract == nil {
		return fmt.Sprintf("object literal method %q has type %s, not %s", method, d.Type(got), d.Type(want))
	}
	return fmt.Sprintf("object literal does not implement %s: method %q has type %s, not %s", d.Type(contract), method, d.Type(got), d.Type(want))
}

func (d diagnosticDisplay) ArgumentMissingRequiredMethodMessage(argument string, contract typ.Type, method string) string {
	if contract == nil {
		return fmt.Sprintf("%s is missing required method %q", argument, method)
	}
	return fmt.Sprintf("%s does not implement %s: missing method %q", argument, d.Type(contract), method)
}

func (d diagnosticDisplay) ArgumentMethodTypeMismatchMessage(argument string, contract typ.Type, method string, got, want typ.Type) string {
	if contract == nil {
		return fmt.Sprintf("%s method %q has type %s, not %s", argument, method, d.Type(got), d.Type(want))
	}
	return fmt.Sprintf("%s does not implement %s: method %q has type %s, not %s", argument, d.Type(contract), method, d.Type(got), d.Type(want))
}

func (d diagnosticDisplay) MethodTypeMismatchEvidence(contract typ.Type, method string, got, want typ.Type) string {
	if contract == nil {
		return fmt.Sprintf("method %s has type %s, not %s", method, d.Type(got), d.Type(want))
	}
	return fmt.Sprintf("method %s has type %s, but %s requires %s", method, d.Type(got), d.Type(contract), d.Type(want))
}

func (d diagnosticDisplay) ObjectLiteralShapeEvidence(t typ.Type) string {
	return fmt.Sprintf("object literal has type %s", d.AssignmentType(t))
}

func (diagnosticDisplay) MissingRequiredFieldHelp(field string) string {
	if field != "" {
		return fmt.Sprintf("Add field `%s`, or make it optional in the declared type if it may be absent.", field)
	}
	return "Add the missing field, or make it optional in the declared type if it may be absent."
}

func (diagnosticDisplay) MissingRequiredMethodHelp(method string) string {
	if method != "" {
		return fmt.Sprintf("Add method `%s`, or change the target interface if this value should not implement it.", method)
	}
	return "Add the missing method, or change the target interface if this value should not implement it."
}

func (diagnosticDisplay) MissingNonNilGuardHereMessage(sourceName string) string {
	return fmt.Sprintf("no guard on this path proves %s is non-nil", sourceName)
}

func (diagnosticDisplay) OptionalReceiverReadEvidence(receiverName, memberName string) string {
	if memberName == "" {
		return fmt.Sprintf("%s may be nil before the read", receiverName)
	}
	if strings.HasPrefix(memberName, "[") {
		return fmt.Sprintf("%s may be nil before indexing %s", receiverName, memberName)
	}
	return fmt.Sprintf("%s may be nil before reading %s", receiverName, memberName)
}

func (diagnosticDisplay) IndexedReadExpectedProofMessage(sourceName, expectedKind string) string {
	return fmt.Sprintf("%s is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the %s here", sourceName, expectedKind)
}

func (diagnosticDisplay) MissingExpectedProofMessage(sourceName, expectedKind string) string {
	return fmt.Sprintf("no proof on this path shows %s satisfies the %s", sourceName, expectedKind)
}

func (d diagnosticDisplay) ReturnDeclaredTypeEvidence(label string, want typ.Type) string {
	return fmt.Sprintf("%s must satisfy declared return type %s", label, d.Type(want))
}
