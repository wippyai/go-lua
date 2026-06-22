package diagnostics

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	unknownSourceName = "assigned value"
	maxInlineTypeLen  = 96
)

const (
	labelCallTarget           = "call target"
	labelCalleeDeclaration    = "callee declaration"
	labelCallExpression       = "call expression"
	labelCallResult           = "call result"
	labelArgumentValue        = "argument value"
	labelExtraArgument        = "extra argument"
	labelReturnedValue        = "returned value"
	labelMemberRead           = "member read"
	labelMemberCall           = "member call"
	labelMethodCall           = "method call"
	labelAssignedValue        = "assigned value"
	labelDeclaredType         = "declared type"
	labelDeclaredReturn       = "declared return type"
	labelAssignmentTarget     = "assignment target"
	labelObjectLiteral        = "object literal"
	labelPossiblyNilContainer = "possibly nil container"
	labelValueMayBeNil        = "value may be nil"
	labelValueAlwaysNil       = "value is always nil"
	labelUnusedLocal          = "unused local"
	labelDeadAssignment       = "dead assignment"
	labelOverwrite            = "overwriting assignment"
	labelExitBeforeRead       = "exit before read"
	labelConditionCheck       = "current check"
	labelProvingGuard         = "prior guard"
	labelUnknownType          = "unknown type"
	labelUnknownValue         = "unknown value"
	labelChannelCaseTest      = "channel case check"
	labelUnionCaseTest        = "union case check"
	labelResultFieldRead      = "result field read"
	labelDispatchLookup       = "dispatch lookup"
	labelDispatchTable        = "dispatch table"
	labelRegistrationCall     = "registration call"
	labelDispatchCall         = "dispatch call"
	labelFrozenTableMutation  = "mutation of frozen table"
	labelFrozenTableCall      = "mutating call on frozen table"
	labelFreezeProof          = "freeze proof"
	labelLifecycleAcquire     = "resource acquired"
	labelLifecycleTransition  = "lifecycle transition"
	labelLifecycleEscape      = "ownership escaped"
)

func sourceLabel(span diagnostic.Span, message string) diagnostic.Label {
	return diagnostic.Label{Span: span, Message: message, Placement: sourceLabelPlacement(message)}
}

func sourceLabelPlacement(message string) diagnostic.LabelPlacement {
	switch message {
	case labelCallTarget,
		labelCallExpression,
		labelCallResult,
		labelArgumentValue,
		labelExtraArgument,
		labelReturnedValue,
		labelMethodCall,
		labelMemberRead,
		labelMemberCall,
		labelAssignedValue,
		labelObjectLiteral,
		labelPossiblyNilContainer,
		labelValueMayBeNil,
		labelValueAlwaysNil,
		labelUnusedLocal,
		labelDeadAssignment,
		labelConditionCheck,
		labelUnknownType,
		labelUnknownValue,
		labelChannelCaseTest,
		labelUnionCaseTest,
		labelResultFieldRead,
		labelDispatchLookup,
		labelRegistrationCall,
		labelDispatchCall,
		labelFrozenTableMutation,
		labelFrozenTableCall,
		labelLifecycleAcquire,
		labelLifecycleTransition,
		labelLifecycleEscape:
		return diagnostic.LabelPlacementBelow
	case labelCalleeDeclaration,
		labelDeclaredType,
		labelDeclaredReturn,
		labelAssignmentTarget,
		labelOverwrite,
		labelExitBeforeRead,
		labelProvingGuard,
		labelDispatchTable,
		labelFreezeProof:
		return diagnostic.LabelPlacementAbove
	default:
		return diagnostic.LabelPlacementAuto
	}
}

func conditionCheckEvidence(check string) string {
	return display.ConditionCheckEvidence(check)
}

func truthyConditionCheck(path string) string {
	return display.TruthyConditionCheck(path)
}

func falsyConditionCheck(path string) string {
	return display.FalsyConditionCheck(path)
}

func nilConditionCheck(path string) string {
	return display.NilConditionCheck(path)
}

func nonNilConditionCheck(path string) string {
	return display.NonNilConditionCheck(path)
}

func conditionStabilityEvidence(path string) string {
	return display.ConditionStabilityEvidence(path)
}

func conditionPathProofEvidence(path, state string) string {
	return display.ConditionPathProofEvidence(path, state)
}

func conditionTypeProofEvidence(path, runtimeType string) string {
	return display.ConditionTypeProofEvidence(path, runtimeType)
}

func redundantConditionMessage(always bool) string {
	return display.RedundantConditionMessage(always)
}

func redundantConditionHelp(always bool) string {
	return display.RedundantConditionHelp(always)
}

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

func channelSelectExhaustivenessMessage(caseWord, cases string) string {
	return display.ChannelSelectExhaustivenessMessage(caseWord, cases)
}

func channelSelectExhaustivenessHelp() string {
	return display.ChannelSelectExhaustivenessHelp()
}

func discriminatedUnionExhaustivenessMessage(caseWord, cases string) string {
	return display.DiscriminatedUnionExhaustivenessMessage(caseWord, cases)
}

func discriminatedUnionExhaustivenessHelp() string {
	return display.DiscriminatedUnionExhaustivenessHelp()
}

func dispatchTableExhaustivenessMessage(keyWord, keys string) string {
	return display.DispatchTableExhaustivenessMessage(keyWord, keys)
}

func dispatchTableExhaustivenessHelp() string {
	return display.DispatchTableExhaustivenessHelp()
}

func registrationExhaustivenessMessage(registrationWord, registrations string) string {
	return display.RegistrationExhaustivenessMessage(registrationWord, registrations)
}

func registrationExhaustivenessHelp() string {
	return display.RegistrationExhaustivenessHelp()
}

func resultShapeExhaustivenessMessage(readPath, requiredCase string) string {
	return display.ResultShapeExhaustivenessMessage(readPath, requiredCase)
}

func resultShapeExhaustivenessHelp() string {
	return display.ResultShapeExhaustivenessHelp()
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
	return codeName(receiver) + " is result-shaped and discriminated by " + codeName(discriminant)
}

func resultShapeFieldCaseEvidence(readPath, requiredCase string) string {
	return codeName(readPath) + " exists only for " + codeName(requiredCase)
}

func resultShapeMissingProofEvidence(requiredCase string) string {
	return "no stable guard proves " + codeName(requiredCase) + " before this read"
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

func frozenTableMutationMessage(containerName string) string {
	return display.FrozenTableMutationMessage(containerName)
}

func frozenTableCallMutationMessage(containerName string) string {
	return display.FrozenTableCallMutationMessage(containerName)
}

func frozenTableAssignmentHelp() string {
	return display.FrozenTableAssignmentHelp()
}

func frozenTableCallHelp() string {
	return display.FrozenTableCallHelp()
}

func resourceUnreleasedMessage(resourceName, protocol, current, final string) string {
	return display.ResourceUnreleasedMessage(resourceName, protocol, current, final)
}

func resourceAcquireEvidence(resourceName, protocol, current, final string) string {
	return display.ResourceAcquireEvidence(resourceName, protocol, current, final)
}

func resourceTransitionEvidence(resourceName, protocol, from, to string) string {
	return display.ResourceTransitionEvidence(resourceName, protocol, from, to)
}

func resourceEscapeEvidence(resourceName, protocol string) string {
	return display.ResourceEscapeEvidence(resourceName, protocol)
}

func resourceExitObligationEvidence(resourceName, protocol, current, final string) string {
	return display.ResourceExitObligationEvidence(resourceName, protocol, current, final)
}

func resourceUnreleasedHelp(resourceName, final string) string {
	return display.ResourceUnreleasedHelp(resourceName, final)
}

func deadAssignmentMessage(name string, hasExit bool) string {
	return display.DeadAssignmentMessage(name, hasExit)
}

func deadAssignmentOverwriteEvidence(name string) string {
	return display.DeadAssignmentOverwriteEvidence(name)
}

func deadAssignmentExitEvidence(name string) string {
	return display.DeadAssignmentExitEvidence(name)
}

func deadAssignmentHelp(name string, hasExit bool) string {
	return display.DeadAssignmentHelp(name, hasExit)
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

func valueMayBeNil(t typ.Type) bool {
	return t != nil && !typ.Nil.Equals(t) && projectionHasNil(t)
}

func nilSafetyMismatch(got, want typ.Type) bool {
	return valueMayBeNil(got) && !projectionHasNil(want)
}

func assignmentSourceTypeEvidence(sourceName string, t typ.Type) string {
	return display.SourceTypeEvidence(sourceName, t)
}

func assignmentSourceTypeEvidenceDisplay(sourceName string, t typ.Type, displayType string) string {
	return display.SourceTypeEvidenceDisplay(sourceName, t, displayType)
}

func declaredTypeEvidence(name string, annotation ast.TypeExpr, fallback typ.Type) string {
	return display.DeclaredTypeEvidence(name, annotation, fallback)
}

func argumentTypeMismatchMessage(subject string, arg ast.Expr, got, want typ.Type) string {
	return display.ArgumentTypeMismatchMessage(subject, arg, got, want)
}

func argumentTypeMismatchMessageDisplay(subject string, arg ast.Expr, got typ.Type, gotDisplay string, want typ.Type, wantDisplay string) string {
	return display.ArgumentTypeMismatchMessageDisplay(subject, arg, got, gotDisplay, want, wantDisplay)
}

func argumentTypeMismatchHelp(argName string, got typ.Type) string {
	return display.ArgumentTypeMismatchHelp("", argName, got)
}

func argumentTypeMismatchHelpForEvidence(subject string, argName string, got typ.Type, evidence []diagnostic.Evidence) string {
	if argumentEvidenceNeedsValidationProof(got, evidence) {
		return display.ArgumentValidationProofHelp(argName)
	}
	return display.ArgumentTypeMismatchHelp(subject, argName, got)
}

func argumentEvidenceNeedsValidationProof(got typ.Type, evidence []diagnostic.Evidence) bool {
	if topLikeType(got) {
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

func returnContractMessage(label string, expr ast.Expr, got, want typ.Type) string {
	return display.ReturnContractMessage(label, expr, got, want)
}

func returnContractHelp(exprName string, got typ.Type) string {
	return display.ReturnContractHelp(exprName, got)
}

func explicitBoundaryProofMessage(want typ.Type) string {
	return display.ExplicitBoundaryProofMessage(want)
}

func explicitBoundaryProofMessageForSubject(subject string, want typ.Type) string {
	return display.ExplicitBoundaryProofMessageForSubject(subject, want)
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

func missingMemberMessage(receiver typ.Type, member string) string {
	return display.MissingMemberMessage(receiver, member)
}

func memberReadReceiverEvidence(readPath, member string, receiver typ.Type) string {
	return display.MemberReadReceiverEvidence(readPath, member, receiver)
}

func receiverForMemberEvidence(memberPath string, receiver typ.Type) string {
	return display.ReceiverForMemberEvidence(memberPath, receiver)
}

func missingMemberHelp(member string) string {
	return display.MissingMemberHelp(member)
}

func memberNotCallableMessage(memberPath string, receiver, memberType typ.Type, member string) string {
	return display.MemberNotCallableMessage(memberPath, receiver, memberType, member)
}

func memberTypeAtCallEvidence(memberPath string, memberType typ.Type) string {
	return display.MemberTypeAtCallEvidence(memberPath, memberType)
}

func memberNotCallableHelp(memberPath string) string {
	return display.MemberNotCallableHelp(memberPath)
}

func directNotCallableMessage(name string, calleeType typ.Type) string {
	return display.DirectNotCallableMessage(name, calleeType)
}

func directNotCallableHelp(name string) string {
	return display.DirectNotCallableHelp(name)
}

func annotatedTypeEvidence(name string, t typ.Type) string {
	return display.AnnotatedTypeEvidence(name, t)
}

func assignmentMessage(sourceName string, got, want typ.Type) string {
	return display.AssignmentMessage(sourceName, got, want)
}

func memberAssignmentMessage(memberName string, sourceName string, got, want typ.Type) string {
	return display.MemberAssignmentMessage(memberName, sourceName, got, want)
}

func assignmentHelp(sourceName string, got typ.Type) string {
	return display.AssignmentHelp(sourceName, got)
}

func assignmentTargetTypeEvidence(targetName string, want typ.Type) string {
	return display.AssignmentTargetTypeEvidence(targetName, want)
}

func reassignedCallResultFieldEvidenceMessage(rootName, readName string, replacement typ.Type) string {
	return display.ReassignedCallResultFieldEvidence(rootName, readName, replacement)
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

func missingRequiredMethodEvidence(contract typ.Type, method string) string {
	return display.MissingRequiredMethodEvidence(contract, method)
}

func missingRequiredMethodTypeEvidence(contract typ.Type, method typ.Method) string {
	return display.MissingRequiredMethodTypeEvidence(contract, method)
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

func missingNonNilGuardHereMessage(sourceName string) string {
	return display.MissingNonNilGuardHereMessage(sourceName)
}

func optionalReceiverReadEvidence(receiverName, memberName string) string {
	return display.OptionalReceiverReadEvidence(receiverName, memberName)
}

func indexedReadExpectedProofMessage(sourceName, expectedKind string) string {
	return display.IndexedReadExpectedProofMessage(sourceName, expectedKind)
}

func missingExpectedProofMessage(sourceName, expectedKind string) string {
	return display.MissingExpectedProofMessage(sourceName, expectedKind)
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

func callResultAssignmentHelp(got typ.Type) string {
	return display.CallResultAssignmentHelp(got)
}

func callResultDeclaredReturnEvidence(name, label string, got typ.Type) string {
	return display.CallResultDeclaredReturnEvidence(name, label, got)
}

func callResultMissingNonNilProofMessage(label string) string {
	return display.CallResultMissingNonNilProofMessage(label)
}

func possiblyNilCallTargetMessage(name string) string {
	return display.PossiblyNilCallTargetMessage(name)
}

func possiblyNilCalleeTypeEvidence(name string, calleeType typ.Type, callable bool) string {
	return display.PossiblyNilCalleeTypeEvidence(name, calleeType, callable)
}

func missingNonNilBeforeCallMessage(name string) string {
	return display.MissingNonNilBeforeCallMessage(name)
}

func possiblyNilCallTargetHelp(name string) string {
	return display.PossiblyNilCallTargetHelp(name)
}

func optionalMethodCallMessage() string {
	return display.OptionalMethodCallMessage()
}

func optionalMethodReceiverEvidence(subject, target string) string {
	return display.OptionalMethodReceiverEvidence(subject, target)
}

func optionalMethodMissingNilCheckEvidence(guardSubject, callTarget string) string {
	return display.OptionalMethodMissingNilCheckEvidence(guardSubject, callTarget)
}

func optionalMethodCallHelp(receiverName, callName string) string {
	return display.OptionalMethodCallHelp(receiverName, callName)
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

func callParameterTypeEvidenceDisplay(name string, index int, suffix string, want typ.Type, displayType string) string {
	return display.CallParameterTypeEvidenceDisplay(name, index, suffix, want, displayType)
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

func numericForOperandMessage(role string, got typ.Type) string {
	return display.NumericForOperandMessage(role, got)
}

func numericForOperandTypeEvidence(role string, got typ.Type) string {
	return display.NumericForOperandTypeEvidence(role, got)
}

func numericForOperandHelp(role string) string {
	return display.NumericForOperandHelp(role)
}

func concatOperandMessage(side string) string {
	return display.ConcatOperandMessage(side)
}

func concatOperandTypeEvidence(side, name string, got typ.Type) string {
	return display.ConcatOperandTypeEvidence(side, name, got)
}

func concatOperandHelp(name string) string {
	return display.ConcatOperandHelp(name)
}

func nonNilAssertAlwaysNilMessage(name string) string {
	return display.NonNilAssertAlwaysNilMessage(name)
}

func nonNilAssertAlwaysNilEvidence(name string) string {
	return display.NonNilAssertAlwaysNilEvidence(name)
}

func nonNilAssertAlwaysNilHelp(name string) string {
	return display.NonNilAssertAlwaysNilHelp(name)
}

func unresolvedTypeMessage(name string) string {
	return display.UnresolvedTypeMessage(name)
}

func unresolvedTypeEvidence(name string) string {
	return display.UnresolvedTypeEvidence(name)
}

func unresolvedTypeHelp() string {
	return display.UnresolvedTypeHelp()
}

func unresolvedValueMessage(name string) string {
	return display.UnresolvedValueMessage(name)
}

func unresolvedValueEvidence(name string) string {
	return display.UnresolvedValueEvidence(name)
}

func unresolvedValueHelp() string {
	return display.UnresolvedValueHelp()
}

func unusedLocalMessage(name string) string {
	return display.UnusedLocalMessage(name)
}

func unusedLocalEvidence(name string) string {
	return display.UnusedLocalEvidence(name)
}

func unusedLocalHelp() string {
	return display.UnusedLocalHelp()
}

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
	return fmt.Sprintf("result field read is not exhaustive; %s requires %s", codeName(readPath), codeName(requiredCase))
}

func (diagnosticDisplay) ResultShapeExhaustivenessHelp() string {
	return "Check the result case before reading this field, or return from the opposite case before continuing."
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

func (d diagnosticDisplay) ArgumentTypeMismatchMessage(subject string, arg ast.Expr, got, want typ.Type) string {
	return d.ArgumentTypeMismatchMessageDisplay(subject, arg, got, "", want, "")
}

func (d diagnosticDisplay) ArgumentTypeMismatchMessageDisplay(subject string, arg ast.Expr, got typ.Type, gotDisplay string, want typ.Type, wantDisplay string) string {
	argName := exprEvidenceNameOK(arg)
	if argName != "" && nilSafetyMismatch(got, want) {
		return fmt.Sprintf("cannot pass %s as %s because it may be nil", argName, subject)
	}
	return fmt.Sprintf("%s is %s, not %s", diagnosticSubjectWithExpr(subject, arg), typeDisplayOr(gotDisplay, d.Type(got)), typeDisplayOr(wantDisplay, d.Type(want)))
}

func diagnosticSubjectWithExpr(subject string, expr ast.Expr) string {
	name := exprEvidenceNameOK(expr)
	if name == "" {
		return subject
	}
	return fmt.Sprintf("%s (%s)", subject, name)
}

func (diagnosticDisplay) ArgumentTypeMismatchHelp(subject string, argName string, got typ.Type) string {
	if argName != "" && argName != unknownSourceName && valueMayBeNil(got) {
		return fmt.Sprintf("Guard `%s` with a nil check, provide a default argument value, or change the parameter type to accept nil.", argName)
	}
	if topLikeType(got) {
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

func (diagnosticDisplay) ArgumentValidationProofHelp(argName string) string {
	if argName != "" && argName != unknownSourceName {
		return fmt.Sprintf("Validate or narrow `%s` before passing it; any/unknown values do not prove parameter contracts.", argName)
	}
	return "Validate or narrow this argument before passing it; any/unknown values do not prove parameter contracts."
}

func (d diagnosticDisplay) ReturnContractMessage(label string, expr ast.Expr, got, want typ.Type) string {
	exprName := exprEvidenceNameOK(expr)
	if nilSafetyMismatch(got, want) {
		if exprName != "" {
			return fmt.Sprintf("cannot return %s as %s because it may be nil", exprName, label)
		}
		return fmt.Sprintf("cannot return %s because it may be nil", label)
	}
	subject := label
	if exprName != "" {
		subject = fmt.Sprintf("%s (%s)", label, exprName)
	}
	return fmt.Sprintf("%s is %s, not %s", subject, d.Type(got), d.Type(want))
}

func (diagnosticDisplay) ReturnContractHelp(exprName string, got typ.Type) string {
	if exprName != "" && exprName != unknownSourceName && valueMayBeNil(got) {
		return fmt.Sprintf("Guard `%s` with a nil check, return a default value, or change the return type to accept nil.", exprName)
	}
	return "Return a value compatible with the declared return type, or change the return annotation if the returned value is valid."
}

func (diagnosticDisplay) ExplicitBoundaryProofMessage(_ typ.Type) string {
	return "assigned value comes from any/unknown"
}

func (diagnosticDisplay) ExplicitBoundaryProofMessageForSubject(subject string, _ typ.Type) string {
	return fmt.Sprintf("%s comes from any/unknown", boundaryEvidenceSubject(subject))
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
	if sourceName != "" && sourceName != unknownSourceName && nilSafetyMismatch(got, want) {
		return fmt.Sprintf("cannot assign %s because it may be nil", sourceName)
	}
	if sourceName != "" && sourceName != unknownSourceName {
		return fmt.Sprintf("cannot assign %s because it is %s, not %s", sourceName, d.Type(got), d.Type(want))
	}
	return fmt.Sprintf("cannot assign %s to %s", d.Type(got), d.Type(want))
}

func (d diagnosticDisplay) MemberAssignmentMessage(memberName string, sourceName string, got, want typ.Type) string {
	if sourceName == "" || sourceName == unknownSourceName {
		if nilSafetyMismatch(got, want) {
			return fmt.Sprintf("cannot assign %s because assigned value may be nil", memberName)
		}
		return fmt.Sprintf("cannot assign %s because assigned value is %s, not %s", memberName, d.Type(got), d.Type(want))
	}
	if nilSafetyMismatch(got, want) {
		return fmt.Sprintf("cannot assign %s to %s because %s may be nil", sourceName, memberName, sourceName)
	}
	return fmt.Sprintf("cannot assign %s to %s because %s is %s, not %s", sourceName, memberName, sourceName, d.Type(got), d.Type(want))
}

func (diagnosticDisplay) AssignmentHelp(sourceName string, got typ.Type) string {
	if sourceName != "" && sourceName != unknownSourceName && valueMayBeNil(got) {
		return fmt.Sprintf("Guard `%s` with a nil check, provide a default value, or change the target type to accept nil.", sourceName)
	}
	if sourceName != "" && sourceName != unknownSourceName {
		return fmt.Sprintf("Use a value compatible with the expected type, or change the target type if `%s` is valid.", sourceName)
	}
	return "Use a value compatible with the expected type, or change the target type if the assigned value is valid."
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

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func (d diagnosticDisplay) AnnotationOrType(annotation ast.TypeExpr, fallback typ.Type) string {
	if s, ok := formatTypeAnnotation(annotation); ok && s != "" {
		return s
	}
	return d.Type(fallback)
}

func formatTypeAnnotation(expr ast.TypeExpr) (string, bool) {
	return formatTypeAnnotationDepth(expr, 0)
}

func formatTypeAnnotationDepth(expr ast.TypeExpr, depth int) (string, bool) {
	if expr == nil {
		return "", false
	}
	if depth > typ.DefaultRecursionDepth {
		return "...", true
	}
	nextDepth := depth + 1
	switch e := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		if e.Name == "" {
			return "", false
		}
		return e.Name, true
	case *ast.TypeRefExpr:
		if len(e.Path) == 0 {
			return "", false
		}
		return strings.Join(e.Path, "."), true
	case *ast.GenericTypeExpr:
		base, ok := formatTypeAnnotationDepth(e.Base, nextDepth)
		if !ok {
			return "", false
		}
		args, ok := formatTypeAnnotationListDepth(e.Args, ", ", nextDepth)
		if !ok {
			return "", false
		}
		return base + "<" + args + ">", true
	case *ast.OptionalTypeExpr:
		inner, ok := formatTypeAnnotationDepth(e.Inner, nextDepth)
		if !ok {
			return "", false
		}
		return maybeParenthesizeOptionalInner(e.Inner, inner) + "?", true
	case *ast.UnionTypeExpr:
		return formatTypeAnnotationListDepth(e.Types, " | ", nextDepth)
	case *ast.IntersectionTypeExpr:
		return formatTypeAnnotationListDepth(e.Types, " & ", nextDepth)
	case *ast.ArrayTypeExpr:
		elem, ok := formatTypeAnnotationDepth(e.Element, nextDepth)
		if !ok {
			return "", false
		}
		if e.Readonly {
			return "readonly {" + elem + "}", true
		}
		return "{" + elem + "}", true
	case *ast.MapTypeExpr:
		key, ok := formatTypeAnnotationDepth(e.Key, nextDepth)
		if !ok {
			return "", false
		}
		value, ok := formatTypeAnnotationDepth(e.Value, nextDepth)
		if !ok {
			return "", false
		}
		prefix := "{"
		if e.Readonly {
			prefix = "readonly {"
		}
		return prefix + "[" + key + "]: " + value + "}", true
	case *ast.RecordTypeExpr:
		return formatRecordTypeAnnotationDepth(e, nextDepth)
	case *ast.FunctionTypeExpr:
		return formatFunctionTypeAnnotationDepth(e, nextDepth)
	case *ast.LiteralTypeExpr:
		return formatLiteralTypeAnnotation(e.Value)
	case *ast.MetaTypeExpr:
		inner, ok := formatTypeAnnotationDepth(e.Inner, nextDepth)
		if !ok {
			return "", false
		}
		return "type<" + inner + ">", true
	case *ast.SelfTypeExpr:
		return "self", true
	case *ast.TupleTypeExpr:
		elems, ok := formatTypeAnnotationListDepth(e.Elements, ", ", nextDepth)
		if !ok {
			return "", false
		}
		return "(" + elems + ")", true
	case *ast.AssertsTypeExpr:
		if e.NarrowTo == nil {
			return "asserts " + e.ParamName, true
		}
		narrow, ok := formatTypeAnnotationDepth(e.NarrowTo, nextDepth)
		if !ok {
			return "", false
		}
		return "asserts " + e.ParamName + " is " + narrow, true
	case *ast.TypeOfExpr:
		name := exprEvidenceNameOK(e.Expr)
		if name == "" {
			name = "..."
		}
		return "typeof(" + name + ")", true
	case *ast.KeyOfExpr:
		inner, ok := formatTypeAnnotationDepth(e.Inner, nextDepth)
		if !ok {
			return "", false
		}
		return "keyof " + parenthesizeTypeOperatorInner(e.Inner, inner), true
	case *ast.IndexAccessExpr:
		object, ok := formatTypeAnnotationDepth(e.Object, nextDepth)
		if !ok {
			return "", false
		}
		index, ok := formatTypeAnnotationDepth(e.Index, nextDepth)
		if !ok {
			return "", false
		}
		return parenthesizeTypeOperatorInner(e.Object, object) + "[" + index + "]", true
	case *ast.ConditionalTypeExpr:
		check, ok := formatTypeAnnotationDepth(e.Check, nextDepth)
		if !ok {
			return "", false
		}
		extends, ok := formatTypeAnnotationDepth(e.Extends, nextDepth)
		if !ok {
			return "", false
		}
		thenType, ok := formatTypeAnnotationDepth(e.Then, nextDepth)
		if !ok {
			return "", false
		}
		elseType, ok := formatTypeAnnotationDepth(e.Else, nextDepth)
		if !ok {
			return "", false
		}
		return check + " extends " + extends + " ? " + thenType + " : " + elseType, true
	default:
		return "", false
	}
}

func formatTypeAnnotationList(exprs []ast.TypeExpr, sep string) (string, bool) {
	return formatTypeAnnotationListDepth(exprs, sep, 0)
}

func formatTypeAnnotationListDepth(exprs []ast.TypeExpr, sep string, depth int) (string, bool) {
	parts := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		part, ok := formatTypeAnnotationDepth(expr, depth+1)
		if !ok {
			return "", false
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, sep), true
}

func formatRecordTypeAnnotation(expr *ast.RecordTypeExpr) (string, bool) {
	return formatRecordTypeAnnotationDepth(expr, 0)
}

func formatRecordTypeAnnotationDepth(expr *ast.RecordTypeExpr, depth int) (string, bool) {
	if expr == nil {
		return "", false
	}
	fields := make([]string, 0, len(expr.Fields))
	for _, field := range expr.Fields {
		fieldType, ok := formatTypeAnnotationDepth(field.Type, depth+1)
		if !ok {
			return "", false
		}
		name := field.Name
		if field.Optional {
			name += "?"
		}
		fields = append(fields, name+": "+fieldType)
	}
	prefix := "{"
	if expr.Readonly {
		prefix = "readonly {"
	}
	return prefix + strings.Join(fields, ", ") + "}", true
}

func formatFunctionTypeAnnotation(expr *ast.FunctionTypeExpr) (string, bool) {
	return formatFunctionTypeAnnotationDepth(expr, 0)
}

func formatFunctionTypeAnnotationDepth(expr *ast.FunctionTypeExpr, depth int) (string, bool) {
	if expr == nil {
		return "", false
	}
	params := make([]string, 0, len(expr.Params)+1)
	for _, param := range expr.Params {
		paramType, ok := formatTypeAnnotationDepth(param.Type, depth+1)
		if !ok {
			return "", false
		}
		if param.Name != "" {
			params = append(params, param.Name+": "+paramType)
		} else {
			params = append(params, paramType)
		}
	}
	if expr.Variadic != nil {
		variadic, ok := formatTypeAnnotationDepth(expr.Variadic, depth+1)
		if !ok {
			return "", false
		}
		params = append(params, "...: "+variadic)
	}
	returns, ok := formatTypeAnnotationReturnsDepth(expr.Returns, depth+1)
	if !ok {
		return "", false
	}
	typeParams, ok := formatTypeParamAnnotations(expr.TypeParams, depth+1)
	if !ok {
		return "", false
	}
	name := "fun"
	if typeParams != "" {
		name += "<" + typeParams + ">"
	}
	return name + "(" + strings.Join(params, ", ") + ") -> " + returns, true
}

func formatTypeAnnotationReturns(exprs []ast.TypeExpr) (string, bool) {
	return formatTypeAnnotationReturnsDepth(exprs, 0)
}

func formatTypeAnnotationReturnsDepth(exprs []ast.TypeExpr, depth int) (string, bool) {
	if len(exprs) == 0 {
		return "()", true
	}
	if len(exprs) == 1 {
		return formatTypeAnnotationDepth(exprs[0], depth+1)
	}
	return formatTypeAnnotationListDepth(exprs, ", ", depth+1)
}

func formatTypeParamAnnotations(params []ast.TypeParamExpr, depth int) (string, bool) {
	if len(params) == 0 {
		return "", true
	}
	parts := make([]string, 0, len(params))
	for _, param := range params {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			return "", false
		}
		if param.Constraint != nil {
			constraint, ok := formatTypeAnnotationDepth(param.Constraint, depth+1)
			if !ok {
				return "", false
			}
			name += ": " + constraint
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", "), true
}

func formatLiteralTypeAnnotation(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	default:
		return "", false
	}
}

func maybeParenthesizeOptionalInner(expr ast.TypeExpr, rendered string) string {
	switch expr.(type) {
	case *ast.UnionTypeExpr, *ast.IntersectionTypeExpr, *ast.FunctionTypeExpr, *ast.TupleTypeExpr, *ast.ConditionalTypeExpr:
		return "(" + rendered + ")"
	default:
		return rendered
	}
}

func parenthesizeTypeOperatorInner(expr ast.TypeExpr, rendered string) string {
	switch expr.(type) {
	case *ast.UnionTypeExpr, *ast.IntersectionTypeExpr, *ast.FunctionTypeExpr, *ast.TupleTypeExpr, *ast.ConditionalTypeExpr:
		return "(" + rendered + ")"
	default:
		return rendered
	}
}
