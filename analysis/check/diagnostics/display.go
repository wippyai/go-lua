package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	unknownSourceName = "assigned value"
	maxInlineTypeLen  = 96
)

func selectedChannelPathEvidence(channel string) string {
	return "branch chain checks channel " + codeName(channel)
}

func handledChannelCasesEvidence(cases string) string {
	return "handled cases: " + cases
}

func missingChannelCasesEvidence(cases string) string {
	return "missing cases: " + cases
}

func missingChannelDefaultEvidence() string {
	return "no default case handles the remaining channel cases"
}

func selectedDiscriminantPathEvidence(path string) string {
	return "branch chain checks discriminant " + codeName(path)
}

func possibleDiscriminantCasesEvidence(cases string) string {
	return "possible cases: " + cases
}

func handledDiscriminantCasesEvidence(cases string) string {
	return "handled cases: " + cases
}

func missingDiscriminantCasesEvidence(cases string) string {
	return "missing cases: " + cases
}

func missingDiscriminantDefaultEvidence() string {
	return "no default branch handles the remaining union cases"
}

func resultShapeUnionEvidence(receiver, discriminant string) string {
	return codeName(receiver) + " is a union discriminated by " + codeName(discriminant)
}

func resultShapeFieldCaseEvidence(readPath, requiredCase string) string {
	return codeName(readPath) + " exists only for " + codeName(requiredCase)
}

func resultShapeMissingProofEvidence(requiredCase string) string {
	return "no stable guard proves " + codeName(requiredCase) + " before this read"
}

func selectedOptionalPathEvidence(path string) string {
	return "branch checks optional " + codeName(path)
}

func optionalPossibleCasesEvidence(path string) string {
	return "possible cases: " + codeName(path+" ~= nil") + ", " + codeName(path+" == nil")
}

func optionalConsumedCaseEvidence(path string) string {
	return "consumed case: " + codeName(path+" ~= nil")
}

func optionalMissingCasesEvidence(cases string) string {
	return "missing cases: " + cases
}

func optionalMissingDefaultEvidence() string {
	return "no else branch handles the remaining optional case"
}

func dispatchLookupEvidence(table, discriminant string) string {
	return codeName(table) + " is indexed by discriminant " + codeName(discriminant)
}

func dispatchTableKeysEvidence(keys string) string {
	return "dispatch table provides keys: " + keys
}

func missingDispatchKeysEvidence(keys string) string {
	return "missing dispatch keys: " + keys
}

func registrationDispatchEvidence(registry, discriminant string) string {
	return codeName(registry) + " is dispatched with discriminant " + codeName(discriminant)
}

func registeredCasesEvidence(cases string) string {
	return "registered cases: " + cases
}

func missingRegistrationsEvidence(registrations string) string {
	return "missing registrations: " + registrations
}

func frozenAssignmentEvidence(containerName string) string {
	return fmt.Sprintf("this assignment mutates table %q", containerName)
}

func frozenCallMutationEvidence(containerName string) string {
	return fmt.Sprintf("this call mutates table %q", containerName)
}

func frozenIncomingStateEvidence(containerName string) string {
	return fmt.Sprintf("table %q is already frozen here", containerName)
}

func frozenAssignmentProofEvidence(containerName string) string {
	return fmt.Sprintf("table %q was frozen by this call before the assignment", containerName)
}

func frozenCallProofEvidence(containerName string) string {
	return fmt.Sprintf("table %q was frozen by this call before the mutating call", containerName)
}

func operandLabel(side string) string {
	if side == "" {
		return "operand"
	}
	return side + " operand"
}

type diagnosticDisplay struct{}

var display diagnosticDisplay

func formatType(t typ.Type) string {
	return display.Type(t)
}

func assignmentSourceTypeEvidence(sourceName string, t typ.Type) string {
	return display.SourceTypeEvidence(sourceName, t)
}

func declaredTypeEvidence(name string, annotation ast.TypeExpr, fallback typ.Type) string {
	return display.DeclaredTypeEvidence(name, annotation, fallback)
}

func argumentTypeMismatchHelpForEvidence(subject string, argName string, got typ.Type, evidence []diagnostic.Evidence) string {
	if evidenceNeedsValidationProof(got, evidence) {
		return display.ArgumentValidationProofHelp(argName)
	}
	if argName != "" && argName != unknownSourceName {
		return fmt.Sprintf("Pass `%s` as a value compatible with the parameter type, or change the callee signature if that argument is valid.", argName)
	}
	if subject != "" {
		return fmt.Sprintf("Pass a value for %s that satisfies the parameter type, or change the callee signature if that argument is valid.", subject)
	}
	return "Pass a value compatible with the parameter type, or change the callee signature if that argument is valid."
}

func evidenceNeedsValidationProof(got typ.Type, evidence []diagnostic.Evidence) bool {
	if typ.AbsentOrTopLike(got) {
		return true
	}
	for _, item := range evidence {
		if item.Kind == diagnostic.EvidencePrecisionBoundary && item.Reason == diagnostic.EvidenceReasonExplicitBoundaryValidation {
			return true
		}
		if item.Kind == diagnostic.EvidenceUserAssertion && item.Reason == diagnostic.EvidenceReasonUserAssertedAny {
			return true
		}
	}
	return false
}

func explicitBoundaryProofMessage(want typ.Type) string {
	return display.ExplicitBoundaryProofMessage(want)
}

func explicitBoundaryProofMessageForSubject(subject string, want typ.Type) string {
	return display.ExplicitBoundaryProofMessageForSubject(subject, want)
}

func userAssertedAnyEvidence() string {
	return display.UserAssertedAnyEvidence()
}

func missingBoundaryProofMessage(want typ.Type) string {
	return display.MissingBoundaryProofMessage(want)
}

func missingBoundaryProofMessageForSubject(subject string, want typ.Type) string {
	return display.MissingBoundaryProofMessageForSubject(subject, want)
}

func missingIndexReadProofMessage(want typ.Type) string {
	return display.MissingIndexReadProofMessage(want)
}

func annotatedTypeEvidence(name string, t typ.Type) string {
	return display.AnnotatedTypeEvidence(name, t)
}

func assignmentMessage(sourceName string, got, want typ.Type) string {
	return display.AssignmentMessage(sourceName, got, want)
}

func assignmentMessageDisplay(sourceName string, got, want typ.Type, wantDisplay string) string {
	return display.AssignmentMessageDisplay(sourceName, got, want, wantDisplay)
}

func memberAssignmentMessage(memberName string, sourceName string, got, want typ.Type) string {
	return display.MemberAssignmentMessage(memberName, sourceName, got, want)
}

func memberAssignmentMessageDisplay(memberName string, sourceName string, got, want typ.Type, wantDisplay string) string {
	return display.MemberAssignmentMessageDisplay(memberName, sourceName, got, want, wantDisplay)
}

func assignmentHelp(sourceName string, missingNilProof bool) string {
	return display.AssignmentHelp(sourceName, missingNilProof)
}

func underSuppliedTargetEvidence(name, sourceName string, resultIndex int) string {
	return display.UnderSuppliedTargetEvidence(name, sourceName, resultIndex)
}

func underSuppliedTargetHelp(name string) string {
	return display.UnderSuppliedTargetHelp(name)
}

func assignmentTargetTypeEvidence(targetName string, want typ.Type) string {
	return display.AssignmentTargetTypeEvidence(targetName, want)
}

func optionalAssignmentTargetMessage(containerName string) string {
	return display.OptionalAssignmentTargetMessage(containerName)
}

func optionalAssignmentTargetContainerEvidence(containerName string, containerType typ.Type) string {
	return display.OptionalAssignmentTargetContainerEvidence(containerName, containerType)
}

func optionalAssignmentTargetWriteEvidence(targetName string) string {
	return display.OptionalAssignmentTargetWriteEvidence(targetName)
}

func optionalAssignmentTargetHelp(containerName string) string {
	return display.OptionalAssignmentTargetHelp(containerName)
}

func missingRequiredFieldMessage(field string) string {
	return display.MissingRequiredFieldMessage(field)
}

func missingRequiredFieldEvidence(field string) string {
	return display.MissingRequiredFieldEvidence(field)
}

func missingRequiredFieldPathEvidence(path string, t typ.Type) string {
	return display.MissingRequiredFieldPathEvidence(path, t)
}

func missingRequiredMethodMessage(contract typ.Type, method string) string {
	return display.MissingRequiredMethodMessage(contract, method)
}

func missingRequiredMethodTypeEvidence(contract typ.Type, method typ.Method) string {
	return display.MissingRequiredMethodTypeEvidence(contract, method)
}

func methodTypeMismatchMessage(contract typ.Type, method string, got, want typ.Type) string {
	return display.MethodTypeMismatchMessage(contract, method, got, want)
}

func argumentMissingRequiredMethodMessage(argument string, contract typ.Type, method string) string {
	return display.ArgumentMissingRequiredMethodMessage(argument, contract, method)
}

func argumentMethodTypeMismatchMessage(argument string, contract typ.Type, method string, got, want typ.Type) string {
	return display.ArgumentMethodTypeMismatchMessage(argument, contract, method, got, want)
}

func methodTypeMismatchEvidence(contract typ.Type, method string, got, want typ.Type) string {
	return display.MethodTypeMismatchEvidence(contract, method, got, want)
}

func objectLiteralShapeEvidence(t typ.Type) string {
	return display.ObjectLiteralShapeEvidence(t)
}

func missingRequiredFieldHelp(field string) string {
	return display.MissingRequiredFieldHelp(field)
}

func missingRequiredMethodHelp(method string) string {
	return display.MissingRequiredMethodHelp(method)
}

func returnDeclaredTypeEvidence(label string, want typ.Type) string {
	return display.ReturnDeclaredTypeEvidence(label, want)
}

func returnIndexedReadProofMessage(subject string) string {
	return display.ReturnIndexedReadProofMessage(subject)
}

func returnExplicitBoundaryProofMessage(subject string) string {
	return display.ReturnExplicitBoundaryProofMessage(subject)
}

func returnMissingProofMessage(subject string) string {
	return display.ReturnMissingProofMessage(subject)
}

func callResultAssignmentHelp(missingNilProof bool) string {
	return display.CallResultAssignmentHelp(missingNilProof)
}

func callResultDeclaredReturnEvidence(name, label string, got typ.Type) string {
	return display.CallResultDeclaredReturnEvidence(name, label, got)
}

func callArityMismatchMessage(name string, want, got int) string {
	return display.CallArityMismatchMessage(name, want, got)
}

func callArgumentCountEvidence(name string, got int) string {
	return display.CallArgumentCountEvidence(name, got)
}

func callParameterCountEvidence(name string, want int) string {
	return display.CallParameterCountEvidence(name, want)
}

func callParameterTypeEvidence(name string, index int, suffix string, want typ.Type) string {
	return display.CallParameterTypeEvidence(name, index, suffix, want)
}

func callParamObligationEvidence(name string, subject string, want typ.Type) string {
	return display.CallParamObligationEvidence(name, subject, want)
}

func memberCallParamObligationEvidence(name string, subject string, provider string, memberParam int, want typ.Type) string {
	return display.MemberCallParamObligationEvidence(name, subject, provider, memberParam, want)
}

func callArityHelp(want, got int) string {
	return display.CallArityHelp(want, got)
}

func codeName(name string) string {
	if name == "" {
		return ""
	}
	return "`" + strings.ReplaceAll(name, "`", "\\`") + "`"
}

func typeDisplayOr(displayType, fallback string) string {
	displayType = strings.TrimSpace(displayType)
	if displayType != "" {
		return displayType
	}
	return fallback
}

func displayTypeWithoutNil(displayType string) string {
	displayType = strings.TrimSpace(displayType)
	if strings.HasSuffix(displayType, "?") {
		return strings.TrimSpace(strings.TrimSuffix(displayType, "?"))
	}
	return displayType
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
