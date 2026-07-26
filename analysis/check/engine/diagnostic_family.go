package engine

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// DiagnosticFamilyID is the stable identity of one diagnostic key family.
// Admission descriptors use these identities rather than restating key
// spellings.
type DiagnosticFamilyID uint8

const (
	DiagnosticFamilyRedundantClaim DiagnosticFamilyID = iota
	DiagnosticFamilyAlwaysTrueGuard
	DiagnosticFamilyRedundantCondition
	DiagnosticFamilyNilUnsafeUse
	DiagnosticFamilyAssignment
	DiagnosticFamilyOptionalAssignmentTarget
	DiagnosticFamilyReturnContract
	DiagnosticFamilyOptionalCallReceiver
	DiagnosticFamilyMissingMember
	DiagnosticFamilyConcatOperand
	DiagnosticFamilyComparisonOperand
	DiagnosticFamilySendIsolation
	DiagnosticFamilyFrozenMutation
	DiagnosticFamilyUnreleasedResource
	DiagnosticFamilyClosedChannelSend
	DiagnosticFamilyClosedChannelClose
	DiagnosticFamilyInvalidTypestateRequirement
	DiagnosticFamilyInvalidTypestateTransition
	DiagnosticFamilyUnprovenTypestateRequirement
	DiagnosticFamilyChannelSelectExhaustiveness
	DiagnosticFamilyUnionExhaustiveness
	DiagnosticFamilyUnprovenClaim
	DiagnosticFamilyCallArgumentType
	DiagnosticFamilyCallNotCallable
	DiagnosticFamilyCallTooFewArguments
	DiagnosticFamilyCallTooManyArguments
	DiagnosticFamilyUnusedLocal
	DiagnosticFamilyDeadAssignment
)

// DiagnosticAnchorKind describes the source-owned coordinate carried by a
// family's key. Narrators use it to select an already-indexed equation lane.
type DiagnosticAnchorKind uint8

const (
	DiagnosticAnchorNone DiagnosticAnchorKind = iota
	DiagnosticAnchorClaim
	DiagnosticAnchorApply
	DiagnosticAnchorApplyOrClaim
	DiagnosticAnchorBranch
	DiagnosticAnchorExpression
	DiagnosticAnchorEffect
	DiagnosticAnchorLifecycle
	DiagnosticAnchorSelect
	DiagnosticAnchorPublication
	DiagnosticAnchorPathReplacement
	DiagnosticAnchorMemberConsumer
)

type diagnosticNarrationContext struct {
	fact              equation.Fact
	key               string
	family            diagnosticFamily
	artifact          equation.Artifact
	closure           equation.OutputClosure
	claims            map[string]equation.Equation
	applies           map[string]equation.Equation
	expressions       map[string]equation.Equation
	writes            map[string]equation.Equation
	indexMutations    map[string]equation.Equation
	pathReplacements  map[string]equation.Equation
	publications      map[string]equation.Equation
	claimTargetSpans  map[string]wir.Span
	callSpans         map[string]wir.Span
	branchSpans       map[string]wir.Span
	returnSpans       map[string]wir.Span
	lifecycleEvidence map[string][]DiagnosticEvidence
	selectEvidence    map[string][]DiagnosticEvidence
}

type diagnosticNarrate func(PublishedDiagnostic, diagnosticNarrationContext) (PublishedDiagnostic, bool)

// diagnosticFamily is the one declaration of a published diagnostic family.
// KeyPrefix is equation vocabulary; Code is public vocabulary; PayloadKind
// names the structured payload variant normally carried by the family.
type diagnosticFamily struct {
	Code        string
	KeyPrefix   string
	PayloadKind string
	AnchorKind  DiagnosticAnchorKind
	Narrate     diagnosticNarrate
	Severity    diagnostic.Severity
}

// diagnosticFamilies is intentionally an array indexed by DiagnosticFamilyID.
// That makes family sets compact and pins every ID to exactly one registry row.
var diagnosticFamilies [DiagnosticFamilyDeadAssignment + 1]diagnosticFamily

var diagnosticFamilyByHead map[string]DiagnosticFamilyID

func init() {
	diagnosticFamilies = [...]diagnosticFamily{
		DiagnosticFamilyRedundantClaim: {
			Code: "advice.redundant_claim", KeyPrefix: "advice.redundant_claim/",
			AnchorKind: DiagnosticAnchorApplyOrClaim, Narrate: narrateRedundantClaim, Severity: diagnostic.SeverityHint,
		},
		DiagnosticFamilyAlwaysTrueGuard: {
			Code: "advice.always_true_guard", KeyPrefix: "advice.always_true_guard/",
			AnchorKind: DiagnosticAnchorBranch, Narrate: narrateConstantGuard, Severity: diagnostic.SeverityHint,
		},
		DiagnosticFamilyRedundantCondition: {
			Code: "lint.condition.redundant", KeyPrefix: "lint.condition.redundant/",
			AnchorKind: DiagnosticAnchorBranch, Narrate: narrateConstantGuard, Severity: diagnostic.SeverityHint,
		},
		DiagnosticFamilyNilUnsafeUse: {
			Code: "type.nil.unsafe_use", KeyPrefix: "type.nil.unsafe_use/",
			AnchorKind: DiagnosticAnchorClaim, Narrate: narrateIdentity, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyAssignment: {
			Code: "type.assignment", KeyPrefix: "type.assignment/", PayloadKind: diagnosticAssignmentMismatch,
			AnchorKind: DiagnosticAnchorClaim, Narrate: narrateAssignment, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyOptionalAssignmentTarget: {
			Code: "type.assignment.optional_target", KeyPrefix: "type.assignment.optional_target/", PayloadKind: diagnosticAssignmentMismatch,
			AnchorKind: DiagnosticAnchorPathReplacement, Narrate: narrateOptionalAssignmentTarget, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyReturnContract: {
			Code: "type.return.contract", KeyPrefix: "type.return.contract/", PayloadKind: diagnosticReturnContract,
			AnchorKind: DiagnosticAnchorPublication, Narrate: narrateReturnContract, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyOptionalCallReceiver: {
			Code: "type.call.optional_receiver", KeyPrefix: "type.call.optional_receiver/", PayloadKind: diagnosticCallNotCallable,
			AnchorKind: DiagnosticAnchorApply, Narrate: narrateOptionalCallReceiver, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyMissingMember: {
			Code: "type.member.missing", KeyPrefix: "type.member.missing/", PayloadKind: diagnosticMemberMissing,
			AnchorKind: DiagnosticAnchorMemberConsumer, Narrate: narrateMissingMember, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyConcatOperand: {
			Code: "type.operator.concat_operand", KeyPrefix: "type.operator.concat_operand/",
			AnchorKind: DiagnosticAnchorExpression, Narrate: narrateConcatOperand, Severity: diagnostic.SeverityWarning,
		},
		DiagnosticFamilyComparisonOperand: {
			Code: "type.operator.comparison_operand", KeyPrefix: "type.operator.comparison_operand/",
			AnchorKind: DiagnosticAnchorExpression, Narrate: narrateIdentity, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilySendIsolation: {
			Code: "send.isolation", KeyPrefix: "send.isolation/", PayloadKind: diagnosticSendIsolation,
			AnchorKind: DiagnosticAnchorApply, Narrate: narrateSendIsolation, Severity: diagnostic.SeverityHint,
		},
		DiagnosticFamilyFrozenMutation: {
			Code: "effect.freeze.mutation", KeyPrefix: "effect.freeze.mutation/",
			AnchorKind: DiagnosticAnchorEffect, Narrate: narrateFrozenMutation, Severity: diagnostic.SeverityWarning,
		},
		DiagnosticFamilyUnreleasedResource: {
			Code: "effect.lifecycle.unreleased", KeyPrefix: "effect.lifecycle.unreleased/", PayloadKind: diagnosticResourceUnreleased,
			AnchorKind: DiagnosticAnchorLifecycle, Narrate: narrateUnreleasedResource, Severity: diagnostic.SeverityWarning,
		},
		DiagnosticFamilyClosedChannelSend: {
			Code: "channel.send.closed", KeyPrefix: "channel.send.closed/", PayloadKind: diagnosticChannelLifecycle,
			AnchorKind: DiagnosticAnchorLifecycle, Narrate: narrateChannelLifecycle, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyClosedChannelClose: {
			Code: "channel.close.closed", KeyPrefix: "channel.close.closed/", PayloadKind: diagnosticChannelLifecycle,
			AnchorKind: DiagnosticAnchorLifecycle, Narrate: narrateChannelLifecycle, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyInvalidTypestateRequirement: {
			Code: "typestate.invalid_requirement", KeyPrefix: "typestate.invalid_requirement/", PayloadKind: diagnosticTypestateRequirement,
			AnchorKind: DiagnosticAnchorLifecycle, Narrate: narrateResourceTypestate, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyInvalidTypestateTransition: {
			Code: "typestate.invalid_transition", KeyPrefix: "typestate.invalid_transition/", PayloadKind: diagnosticTypestateTransition,
			AnchorKind: DiagnosticAnchorLifecycle, Narrate: narrateResourceTypestate, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyUnprovenTypestateRequirement: {
			Code: "typestate.unproven_requirement", KeyPrefix: "typestate.unproven_requirement/", PayloadKind: diagnosticTypestateUnproven,
			AnchorKind: DiagnosticAnchorLifecycle, Narrate: narrateResourceTypestate, Severity: diagnostic.SeverityWarning,
		},
		DiagnosticFamilyChannelSelectExhaustiveness: {
			Code: "channel.select.exhaustiveness", KeyPrefix: "channel.select.exhaustiveness/",
			AnchorKind: DiagnosticAnchorSelect, Narrate: narrateSelect, Severity: diagnostic.SeverityWarning,
		},
		DiagnosticFamilyUnionExhaustiveness: {
			Code: "lint.union.exhaustiveness", KeyPrefix: "lint.union.exhaustiveness/",
			AnchorKind: DiagnosticAnchorSelect, Narrate: narrateSelect, Severity: diagnostic.SeverityWarning,
		},
		DiagnosticFamilyUnprovenClaim: {
			Code: "lint.claim.unproven", KeyPrefix: "claim/unproven/", PayloadKind: diagnosticClaimUnproven,
			AnchorKind: DiagnosticAnchorClaim, Narrate: narrateUnprovenClaim, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyCallArgumentType: {
			Code: "type.call.direct.argument_type", KeyPrefix: "type.call.direct.argument_type/", PayloadKind: diagnosticCallArgument,
			AnchorKind: DiagnosticAnchorApply, Narrate: narrateDirectCall, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyCallNotCallable: {
			Code: "type.call.direct.not_callable", KeyPrefix: "type.call.direct.not_callable/", PayloadKind: diagnosticCallNotCallable,
			AnchorKind: DiagnosticAnchorApply, Narrate: narrateDirectCall, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyCallTooFewArguments: {
			Code: "type.call.direct.too_few_args", KeyPrefix: "type.call.direct.too_few_args/", PayloadKind: diagnosticCallArity,
			AnchorKind: DiagnosticAnchorApply, Narrate: narrateDirectCall, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyCallTooManyArguments: {
			Code: "type.call.direct.too_many_args", KeyPrefix: "type.call.direct.too_many_args/", PayloadKind: diagnosticCallArity,
			AnchorKind: DiagnosticAnchorApply, Narrate: narrateDirectCall, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyUnusedLocal: {
			Code: "lint.unused.local", KeyPrefix: "lint.unused.local/",
			AnchorKind: DiagnosticAnchorNone, Narrate: narrateIdentity, Severity: diagnostic.SeverityError,
		},
		DiagnosticFamilyDeadAssignment: {
			Code: "lint.dead.assignment", KeyPrefix: "lint.dead.assignment/",
			AnchorKind: DiagnosticAnchorNone, Narrate: narrateIdentity, Severity: diagnostic.SeverityError,
		},
	}
	diagnosticFamilyByHead = make(map[string]DiagnosticFamilyID, len(diagnosticFamilies))
	for id, family := range diagnosticFamilies {
		diagnosticFamilyByHead[strings.TrimSuffix(family.KeyPrefix, "/")] = DiagnosticFamilyID(id)
	}
}

func diagnosticFamilyPrefix(id DiagnosticFamilyID) string {
	if int(id) >= len(diagnosticFamilies) {
		return ""
	}
	return diagnosticFamilies[id].KeyPrefix
}

func lookupDiagnosticFamily(key string) (DiagnosticFamilyID, diagnosticFamily, string, bool) {
	inner := key
	for {
		next, child := childDiagnosticKey(inner)
		if !child {
			break
		}
		inner = next
	}
	head, tail, found := strings.Cut(inner, "/")
	if !found {
		return 0, diagnosticFamily{}, inner, false
	}
	id, found := diagnosticFamilyByHead[head]
	if !found {
		if next, _, nested := strings.Cut(tail, "/"); nested {
			id, found = diagnosticFamilyByHead[head+"/"+next]
		}
	}
	if !found {
		return 0, diagnosticFamily{}, inner, false
	}
	return id, diagnosticFamilies[id], inner, true
}

// DiagnosticFamilySet is the membership surface consumed by admission lanes.
// Its zero value is an empty set.
type DiagnosticFamilySet map[DiagnosticFamilyID]struct{}

// NewDiagnosticFamilySet constructs an obligation set without exposing key
// spellings to its consumer.
func NewDiagnosticFamilySet(ids ...DiagnosticFamilyID) DiagnosticFamilySet {
	set := make(DiagnosticFamilySet, len(ids))
	for _, id := range ids {
		if int(id) < len(diagnosticFamilies) {
			set[id] = struct{}{}
		}
	}
	return set
}

// ContainsKey reports whether key belongs to a family in the set.
func (set DiagnosticFamilySet) ContainsKey(key string) bool {
	id, _, _, found := lookupDiagnosticFamily(key)
	if !found {
		return false
	}
	_, found = set[id]
	return found
}

// RegisteredDiagnosticFamilies returns the complete registry membership.
func RegisteredDiagnosticFamilies() DiagnosticFamilySet {
	set := make(DiagnosticFamilySet, len(diagnosticFamilies))
	for id := range diagnosticFamilies {
		set[DiagnosticFamilyID(id)] = struct{}{}
	}
	return set
}

// DiagnosticFamilySeverity returns the registry-owned default severity for key.
func DiagnosticFamilySeverity(key string) (diagnostic.Severity, bool) {
	_, family, _, found := lookupDiagnosticFamily(key)
	return family.Severity, found
}

// DiagnosticCode projects a diagnostic fact key to its public code while
// preserving the historical fallback for unregistered control facts.
func DiagnosticCode(key string) string {
	return diagnosticCode(key)
}

func diagnosticFamilyMatches(key string, ids ...DiagnosticFamilyID) bool {
	id, _, _, found := lookupDiagnosticFamily(key)
	if !found {
		return false
	}
	for _, candidate := range ids {
		if id == candidate {
			return true
		}
	}
	return false
}

func diagnosticOperationName(key string) string {
	_, family, inner, found := lookupDiagnosticFamily(key)
	if found {
		name := strings.TrimPrefix(inner, family.KeyPrefix)
		if family.AnchorKind == DiagnosticAnchorExpression && family.Code == diagnosticFamilies[DiagnosticFamilyConcatOperand].Code {
			name, _, _ = strings.Cut(name, "/")
		}
		if family.AnchorKind == DiagnosticAnchorApply && strings.HasPrefix(family.Code, "type.call.direct.") {
			name, _, _ = strings.Cut(name, "/")
		}
		return name
	}
	parts := strings.Split(inner, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

func narrateIdentity(item PublishedDiagnostic, _ diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	return item, true
}

func narrateAssignment(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	if context.fact.Key != context.key {
		return item, true
	}
	name := strings.TrimPrefix(context.key, context.family.KeyPrefix)
	if _, found := context.claims[name]; found {
		// Claim-backed assignments share the evidence builder immediately below
		// the registry call in publishedDiagnostics.
		return item, false
	}
	if mutation, found := context.indexMutations[name]; found {
		return enrichClosedDynamicWriteDiagnostic(item, mutation, context.claimTargetSpans[name]), true
	}
	if replacement, found := context.pathReplacements[name]; found && functionContractDiagnostic(name, context.closure.Values) {
		return enrichFunctionContractWriteDiagnostic(item, replacement, context.claimTargetSpans[name], context.closure), true
	}
	return item, true
}

func narrateUnprovenClaim(item PublishedDiagnostic, _ diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "unproven claim"}}
	item.Help = "Prove the claim by narrowing the value to the claimed type, or remove the claim."
	return item, true
}

func narrateChannelLifecycle(item PublishedDiagnostic, _ diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	return enrichChannelLifecycleDiagnostic(item), true
}

func narrateResourceTypestate(item PublishedDiagnostic, _ diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	return enrichResourceTypestateDiagnostic(item), true
}

func narrateUnreleasedResource(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	return enrichUnreleasedLifecycleDiagnostic(item, context.lifecycleEvidence[context.fact.Key]), true
}

func narrateSelect(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	item.Evidence = append([]DiagnosticEvidence(nil), context.selectEvidence[context.fact.Key]...)
	if context.family.Code == diagnosticFamilies[DiagnosticFamilyUnionExhaustiveness].Code {
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "union case check"}}
		item.Help = "Handle each missing case, or add an else branch when a fallback is valid."
	} else {
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "channel case check"}}
		item.Help = "Add an elseif branch for each missing case, or add a default branch when a fallback is valid."
	}
	return item, true
}

func narrateFrozenMutation(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	return enrichFrozenMutationDiagnostic(item, context.artifact, context.callSpans), true
}

func narrateDirectCall(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.applies[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	return enrichDirectCallDiagnostic(item, operation, context.closure.Values), true
}

func narrateOptionalCallReceiver(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.applies[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	return enrichOptionalReceiverDiagnostic(item, operation), true
}

func narrateConcatOperand(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.expressions[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	return enrichConcatOperandDiagnostic(item, operation, context.closure.Values), true
}

func narrateRedundantClaim(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	name := diagnosticOperationName(context.key)
	if operation, found := context.applies[name]; found {
		return enrichRedundantCastDiagnostic(item, operation, context.callSpans), true
	}
	if operation, found := context.claims[name]; found {
		return enrichRedundantClaimDiagnostic(item, operation), true
	}
	return item, true
}

func narrateSendIsolation(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.applies[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	return enrichSendIsolationDiagnostic(item, operation), true
}

func narrateConstantGuard(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := branchOperation(context.artifact, diagnosticOperationName(context.key))
	if !found {
		return item, true
	}
	return enrichConstantGuardDiagnostic(item, operation, context.fact.Key, context.artifact, context.branchSpans), true
}

func narrateReturnContract(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	contract := strings.TrimPrefix(context.key, context.family.KeyPrefix)
	operationName, indexed, hasSlot := strings.Cut(contract, "/")
	slot, member, _ := strings.Cut(indexed, "/")
	if operation, found := context.publications[operationName]; found && hasSlot {
		return enrichReturnContractDiagnostic(item, operation, operationName, slot, member, context.returnSpans), true
	}
	return item, true
}

func narrateOptionalAssignmentTarget(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	name := strings.TrimPrefix(context.key, context.family.KeyPrefix)
	if operation, found := context.pathReplacements[name]; found {
		return enrichOptionalWriteTargetDiagnostic(item, operation, name, context.closure.Values), true
	}
	return item, true
}

func narrateMissingMember(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	name := strings.TrimPrefix(context.key, context.family.KeyPrefix)
	if operation, found := context.claims[name]; found {
		return enrichMissingMemberDiagnostic(item, operation, context.claimTargetSpans[name], context.closure), true
	}
	if operation, found := context.writes[name]; found {
		return enrichMissingStaticPathDiagnostic(item, operation), true
	}
	if operation, found := context.applies[name]; found {
		return enrichMissingMemberCallDiagnostic(item, operation), true
	}
	return item, true
}
