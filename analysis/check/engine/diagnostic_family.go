package engine

import (
	"fmt"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

// diagnosticExplanationBuilder is the shared construction surface for registry
// narrations. A narration consumes the structured payload and appends evidence
// in semantic order; the builder supplies the diagnostic anchor to evidence and
// labels unless the narration names a different published span explicitly.
type diagnosticExplanationBuilder struct {
	item    PublishedDiagnostic
	payload DiagnosticPayload
}

func explainDiagnostic(item PublishedDiagnostic) *diagnosticExplanationBuilder {
	return &diagnosticExplanationBuilder{item: item, payload: item.Payload}
}

func (builder *diagnosticExplanationBuilder) message(message string) {
	builder.item.Message = message
}

func (builder *diagnosticExplanationBuilder) evidence(kind diagnostic.EvidenceKind, trust diagnostic.TrustKind, reason diagnostic.EvidenceReason, message string) {
	builder.evidenceAt(builder.item.Span, kind, trust, reason, message)
}

func (builder *diagnosticExplanationBuilder) evidenceAt(span wir.Span, kind diagnostic.EvidenceKind, trust diagnostic.TrustKind, reason diagnostic.EvidenceReason, message string) {
	builder.item.Evidence = append(builder.item.Evidence, DiagnosticEvidence{
		Span: span, Kind: kind, Trust: trust, Reason: reason, Message: message,
	})
}

func (builder *diagnosticExplanationBuilder) label(message string) {
	builder.labelAt(builder.item.Span, message)
}

func (builder *diagnosticExplanationBuilder) labelAt(span wir.Span, message string) {
	builder.item.Labels = append(builder.item.Labels, DiagnosticLabel{Span: span, Message: message})
}

func (builder *diagnosticExplanationBuilder) causalEvidence() {
	for index := range builder.item.Evidence {
		builder.item.Evidence[index].CausalOrder = uint32(index + 1)
	}
}

func (builder *diagnosticExplanationBuilder) build(help string) (PublishedDiagnostic, bool) {
	builder.item.Help = help
	return builder.item, true
}

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

func (f diagnosticFamily) tail(key string) (string, bool) {
	return strings.CutPrefix(key, f.KeyPrefix)
}

func (f diagnosticFamily) body(key string) string {
	tail, _ := f.tail(key)
	return tail
}

func (f diagnosticFamily) segments(key string) []string {
	if _, ok := f.tail(key); !ok {
		return nil
	}
	return strings.Split(key, "/")
}

func diagnosticFamilyTail(id DiagnosticFamilyID, key string) (string, bool) {
	return diagnosticFamilies[id].tail(key)
}

func diagnosticFamilyBody(id DiagnosticFamilyID, key string) string {
	return diagnosticFamilies[id].body(key)
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
		name := family.body(inner)
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
	name := context.family.body(context.key)
	if operation, found := context.claims[name]; found {
		// A select-seeded child can carry the closed mismatch payload even when
		// its entry does not publish the source term again. In that case the
		// payload and authored claim are the complete narration authority.
		if item.Payload.Kind == diagnosticAssignmentMismatch && item.Payload.Observed != "" {
			operands, err := artifactOperandsByRole(operation.Operands, "value")
			if err == nil {
				if _, available := claimDiagnosticValue(operands["value"], operation, context.closure); !available {
					source, display := "value", "value"
					for _, operand := range operation.Operands {
						switch operand.Role {
						case "source-display":
							source = string(operand.Term.Encoding)
						case "display":
							display = string(operand.Term.Encoding)
						}
					}
					declared := claimDeclaredDisplay(operation, nil)
					if declared != "" {
						target := context.claimTargetSpans[name]
						if !target.Valid() {
							target = item.Span
						}
						builder := explainDiagnostic(item)
						builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s has type %s", source, builder.payload.Observed))
						builder.evidenceAt(target, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s is declared as %s", display, declared))
						builder.label("assigned value " + builder.payload.Observed)
						builder.labelAt(target, "declared type "+declared)
						return builder.build("Use a value compatible with the expected type, or change the target type if `" + display + "` is valid.")
					}
				}
			}
		}
		// Claim-backed assignments share the evidence builder immediately below
		// the registry call in publishedDiagnostics.
		return item, false
	}
	if mutation, found := context.indexMutations[name]; found {
		source, target := "value", "value"
		for _, operand := range mutation.Operands {
			switch operand.Role {
			case "source-display":
				source = string(operand.Term.Encoding)
			case "display":
				target = string(operand.Term.Encoding)
			}
		}
		builder := explainDiagnostic(item)
		valueType, contract := builder.payload.Observed, builder.payload.Required
		if builder.payload.Kind != diagnosticAssignmentMismatch || valueType == "" || contract == "" {
			return item, true
		}
		targetSpan := context.claimTargetSpans[name]
		if !targetSpan.Valid() {
			targetSpan = item.Span
		}
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s has type %s", source, valueType))
		builder.evidenceAt(targetSpan, diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("assignment target %s requires %s", target, contract))
		builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustUnknown, diagnostic.EvidenceReasonBoundaryValidationMissing, fmt.Sprintf("no proof on this path shows %s is %s", source, contract))
		builder.label("assigned value " + valueType)
		builder.labelAt(targetSpan, "assignment target "+target)
		return builder.build("Use a value compatible with the expected type, or change the target type if `" + source + "` is valid.")
	}
	if replacement, found := context.pathReplacements[name]; found && functionContractDiagnostic(name, context.closure.Values) {
		operands, err := artifactOperandsByRole(replacement.Operands, "display", "value")
		if err != nil {
			return item, true
		}
		value, available := claimDiagnosticValue(operands["value"], replacement, context.closure)
		if !available {
			return item, true
		}
		builder := explainDiagnostic(item)
		actual := assignmentEvidenceValue(value)
		display, declared := string(operands["display"]), builder.payload.Required
		if builder.payload.Kind != diagnosticFunctionWriteMismatch || declared == "" {
			return item, true
		}
		targetSpan := context.claimTargetSpans[name]
		if !targetSpan.Valid() {
			targetSpan = item.Span
		}
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "assigned value has literal value "+actual)
		builder.evidenceAt(targetSpan, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, display+" is declared as "+declared)
		builder.label("assigned value " + actual)
		builder.labelAt(targetSpan, "declared type "+declared)
		return builder.build("Use a value compatible with the expected type, or change the target type if the assigned value is valid.")
	}
	return item, true
}

func narrateUnprovenClaim(item PublishedDiagnostic, _ diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "unproven claim"}}
	item.Help = "Prove the claim by narrowing the value to the claimed type, or remove the claim."
	return item, true
}

func narrateChannelLifecycle(item PublishedDiagnostic, _ diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	builder := explainDiagnostic(item)
	display := builder.payload.Source
	if builder.payload.Kind != diagnosticChannelLifecycle || display == "" {
		return item, true
	}
	if item.Code == diagnosticFamilies[DiagnosticFamilyClosedChannelSend].Code {
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "this send call runs after `"+display+"` is proven closed")
		builder.label("channel lifecycle call")
		return builder.build("Send before closing the channel.")
	}
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "this close call runs after `"+display+"` is proven closed")
	builder.label("channel lifecycle call")
	return builder.build("Avoid closing the same channel twice.")
}

func narrateResourceTypestate(item PublishedDiagnostic, _ diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	builder := explainDiagnostic(item)
	transition := item.Code == diagnosticFamilies[DiagnosticFamilyInvalidTypestateTransition].Code
	unproven := item.Code == diagnosticFamilies[DiagnosticFamilyUnprovenTypestateRequirement].Code
	resource, expected, found := builder.payload.Source, builder.payload.Required, builder.payload.Observed
	if resource == "" || expected == "" || (!unproven && found == "") {
		return item, true
	}
	if unproven {
		builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustRefuted, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("no proof establishes `%s` in `%s` state at this call", resource, expected))
		builder.label("unproven typestate requirement")
		return builder.build(fmt.Sprintf("Establish that `%s` is in `%s` state before this call.", resource, expected))
	}
	if transition {
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("this transition requires `%s` to be in `%s`, but solved state is `%s`", resource, expected, found))
		builder.label("invalid lifecycle transition")
		return builder.build(fmt.Sprintf("Transition `%s` only when it is in `%s` state.", resource, expected))
	}
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("this call requires `%s` to be in `%s`, but solved state is `%s`", resource, expected, found))
	builder.label("invalid typestate requirement")
	return builder.build(fmt.Sprintf("Call this operation only when `%s` is in `%s` state.", resource, expected))
}

func narrateUnreleasedResource(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	builder := explainDiagnostic(item)
	display := builder.payload.Source
	if builder.payload.Kind != diagnosticResourceUnreleased || display == "" {
		return item, true
	}
	transition := context.lifecycleEvidence[context.fact.Key]
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "this call acquires `"+display+"` as connection:`open` and requires `closed` before local ownership ends")
	builder.item.Evidence = append(builder.item.Evidence, transition...)
	missing := "exit state still has `" + display + "` in protocol connection at `open`; no proof reaches `closed` or escapes ownership on every path"
	if len(transition) != 0 {
		missing = "exit state still has `" + display + "` in protocol connection at a non-final state; no proof reaches `closed` or escapes ownership on every path"
	}
	builder.evidenceAt(wir.Span{}, diagnostic.EvidenceMissingProof, diagnostic.TrustRefuted, diagnostic.EvidenceReasonUnspecified, missing)
	builder.label("resource acquired")
	if len(transition) != 0 {
		builder.labelAt(transition[0].Span, "lifecycle transition")
	}
	return builder.build("Transition `" + display + "` to `closed` or escape ownership on every return path.")
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
	parts := context.family.segments(item.Fact.Key)
	if len(parts) != 3 {
		return item, true
	}
	action, proof := parts[1], parts[2]
	var operation equation.Equation
	found := false
	for _, candidate := range context.artifact.Equations {
		if candidate.Target.Name == action {
			operation, found = candidate, true
			break
		}
	}
	if !found {
		return item, true
	}
	display, callMutation := "", operation.Occurrence.Kind == "apply"
	for _, operand := range operation.Operands {
		if operand.Role == "write-container-display" || (callMutation && operand.Role == equation.IndexedRole(equation.RoleFamilyArgumentDisplay, 0)) {
			display = string(operand.Term.Encoding)
		}
	}
	if display == "" {
		return item, true
	}
	mutation := "this assignment mutates table " + strconv.Quote(display)
	label := "mutation of frozen table"
	if callMutation {
		mutation = "this call mutates table " + strconv.Quote(display)
		label = "mutating call on frozen table"
	}
	proofMessage := "table " + strconv.Quote(display) + " is already frozen here"
	if proof != "guard" {
		suffix := "assignment"
		if callMutation {
			suffix = "mutating call"
		}
		proofMessage = "table " + strconv.Quote(display) + " was frozen by this call before the " + suffix
	}
	builder := explainDiagnostic(item)
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, mutation)
	builder.label(label)
	if proof != "guard" {
		if span, ok := context.callSpans[proof+"/call"]; ok {
			builder.evidenceAt(span, diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, proofMessage)
			builder.labelAt(span, "freeze proof")
		} else {
			builder.evidenceAt(wir.Span{}, diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, proofMessage)
		}
	} else {
		builder.evidenceAt(wir.Span{}, diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, proofMessage)
	}
	if callMutation {
		return builder.build("Create a mutable copy before calling the mutator, or call it before the table is frozen.")
	}
	return builder.build("Create a mutable copy before writing, or move this assignment before the table is frozen.")
}

func narrateDirectCall(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.applies[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	operands := make(map[equation.OperandRole]string, len(operation.Operands))
	for _, operand := range operation.Operands {
		operands[operand.Role] = string(operand.Term.Encoding)
	}
	callee := operands["callee-display"]
	if callee == "" {
		callee = strings.TrimPrefix(operands["callee"], "path/")
	}
	if callee == "" && operands["receiver-display"] != "" {
		if method, ok := callMethodName([]byte(operands["method"])); ok {
			callee = operands["receiver-display"] + "." + method
		}
	}
	if callee == "" {
		return item, true
	}
	code, _, subject, ok := directCallDiagnosticParts(item.Fact.Key)
	if !ok {
		return item, true
	}
	builder := explainDiagnostic(item)
	switch code {
	case "argument_type":
		return narrateCallArgument(builder, callee, subject, operands, context.closure.Values)
	case "too_few_args", "too_many_args":
		if builder.payload.Kind != diagnosticCallArity {
			return item, true
		}
		expected, got := builder.payload.Expected, builder.payload.Actual
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("call to %s passes %d argument%s", callee, got, plural(got)))
		builder.evidenceAt(wir.Span{}, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s declares %d parameter%s", callee, expected, plural(expected)))
		builder.label("call expression")
		if code == "too_few_args" {
			return builder.build("Pass the missing required arguments, or change the callee signature if fewer arguments are valid.")
		}
		return builder.build("Remove the extra arguments, or change the callee signature if they are valid.")
	case "not_callable":
		if builder.payload.Kind != diagnosticCallNotCallable {
			return item, true
		}
		if builder.payload.Flags&DiagnosticMayBeNil != 0 {
			builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s has a callable type, but may also be nil", callee))
			builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustUnknown, diagnostic.EvidenceReasonBoundaryValidationMissing, fmt.Sprintf("no guard on this path proves %s is non-nil before this call", callee))
			builder.label("call target")
			return builder.build(fmt.Sprintf("Guard `%s` with a nil check before calling it.", callee))
		}
		value := builder.payload.Observed
		if value == "" {
			return item, true
		}
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s has literal value %s", callee, value))
		builder.label("call target")
		return builder.build(fmt.Sprintf("Call a function value, or replace `%s` with a callable expression before this call.", callee))
	}
	return item, true
}

func narrateCallArgument(builder *diagnosticExplanationBuilder, callee, subject string, operands map[equation.OperandRole]string, values []equation.Fact) (PublishedDiagnostic, bool) {
	argumentIndex, suffix, ok := callArgumentSubject(subject)
	if !ok {
		return builder.item, true
	}
	if conflict := builder.payload.Conflict; builder.payload.Kind == diagnosticCallGenericConflict && conflict != nil {
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s has type %s", conflict.DemandedAt, conflict.Demanded))
		builder.evidenceAt(wir.Span{}, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s parameter %d%s states %s at both members", callee, argumentIndex, suffix, conflict.Parameter))
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s already binds %s to %s", conflict.BoundAt, conflict.Parameter, conflict.Bound))
		builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustRefuted, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("no binding of %s satisfies both members", conflict.Parameter))
		builder.label("conflicting " + conflict.Parameter + " binding")
		return builder.build(fmt.Sprintf("Pass members that agree on %s, or split the call so each %s binding has its own call site.", conflict.Parameter, conflict.Parameter))
	}
	if builder.payload.Kind != diagnosticCallArgument {
		return builder.item, true
	}
	value, expected := builder.payload.Observed, builder.payload.Required
	nilable := builder.payload.Flags&DiagnosticMayBeNil != 0
	if nilable {
		value = "may be nil"
	}
	argument := fmt.Sprintf("argument %d", argumentIndex) + suffix
	if display := operands[equation.IndexedRole(equation.RoleFamilyArgumentDisplay, argumentIndex-1)]; display != "" {
		argument += " (" + display + ")"
	}
	argumentTerm := []byte(operands[equation.IndexedRole(equation.RoleFamilyArgument, argumentIndex-1)])
	if summaryTypeIsAnyInFacts(argumentTerm, values) || sourceHasGradualLogicalBoundaryInFacts(argumentTerm, values) {
		display := strings.TrimPrefix(argument, fmt.Sprintf("argument %d (", argumentIndex))
		display = strings.TrimSuffix(display, ")")
		if display == argument {
			display = argument
		}
		builder.message(fmt.Sprintf("%s comes from any/unknown; no proof shows it is %s", argument, expected))
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s has type any", argument))
		builder.evidenceAt(wir.Span{}, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s parameter %d%s expects %s", callee, argumentIndex, suffix, expected))
		builder.evidence(diagnostic.EvidencePrecisionBoundary, diagnostic.TrustUnknown, diagnostic.EvidenceReasonExplicitBoundaryValidation, fmt.Sprintf("%s comes from any/unknown", display))
		builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustUnknown, diagnostic.EvidenceReasonBoundaryValidationMissing, fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", display))
		builder.label("argument value any")
		return builder.build(fmt.Sprintf("Validate or narrow `%s` before passing it; any/unknown values do not prove parameter contracts.", display))
	}
	if nilable {
		display := operands[equation.IndexedRole(equation.RoleFamilyArgumentDisplay, argumentIndex-1)]
		builder.message(fmt.Sprintf("cannot pass %s because it may be nil", argument))
		if display != "" {
			builder.message(fmt.Sprintf("cannot pass %s as argument %d because it may be nil", display, argumentIndex))
		} else {
			display = argument
		}
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s can be %s or nil here", argument, expected))
		builder.evidenceAt(wir.Span{}, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s parameter %d expects %s", callee, argumentIndex, expected))
		builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustRefuted, diagnostic.EvidenceReasonBoundaryValidationMissing, fmt.Sprintf("no guard on this path proves %s is non-nil", display))
		builder.label("argument value")
		return builder.build(fmt.Sprintf("Guard `%s` with a nil check, provide a default argument value, or change the parameter type to accept nil.", display))
	}
	builder.message(fmt.Sprintf("%s is %s, not %s", argument, value, expected))
	valueFact := fmt.Sprintf("%s has type %s", argument, value)
	if callDiagnosticValueIsLiteral(value) {
		valueFact = fmt.Sprintf("%s has literal value %s", argument, value)
	}
	parameter := fmt.Sprintf("%s parameter %d", callee, argumentIndex) + suffix
	missingProof := fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", argument)
	if display := operands[equation.IndexedRole(equation.RoleFamilyArgumentDisplay, argumentIndex-1)]; display != "" {
		missingProof = fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", display)
	}
	if field, record := firstRequiredRecordField(expected); record {
		parameter += "." + field
		if strings.HasPrefix(value, "{") {
			missingProof = fmt.Sprintf("object literal does not provide field %q", field)
		}
	}
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, valueFact)
	builder.evidenceAt(wir.Span{}, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s expects %s", parameter, expected))
	builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustRefuted, diagnostic.EvidenceReasonUnspecified, missingProof)
	builder.label("argument value " + value)
	if display := operands[equation.IndexedRole(equation.RoleFamilyArgumentDisplay, argumentIndex-1)]; display != "" {
		return builder.build(fmt.Sprintf("Pass `%s` as a value compatible with the parameter type, or change the callee signature if that argument is valid.", display))
	}
	return builder.build(fmt.Sprintf("Pass a value for argument %d that satisfies the parameter type, or change the callee signature if that argument is valid.", argumentIndex))
}

func narrateOptionalCallReceiver(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.applies[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	receiver, method := "", ""
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "receiver-display":
			receiver = string(operand.Term.Encoding)
		case "method":
			method, _ = callMethodName(operand.Term.Encoding)
		}
	}
	if receiver == "" || method == "" {
		return item, true
	}
	selector := receiver + "." + method
	builder := explainDiagnostic(item)
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("receiver %s is optional at call to %s", receiver, selector))
	builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustUnknown, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("no nil check proves receiver %s is present before calling %s", receiver, selector))
	builder.label("method call")
	return builder.build(fmt.Sprintf("check %s ~= nil before calling %s.", receiver, selector))
}

func narrateConcatOperand(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.expressions[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	_, operationName, subject, ok := concatOperandDiagnosticParts(item.Fact.Key)
	if !ok {
		return item, true
	}
	index, ok := concatOperandIndex(subject)
	if !ok {
		return item, true
	}
	display := "value"
	for _, operand := range operation.Operands {
		if operand.Role == equation.IndexedRole(equation.RoleFamilyValueDisplay, index) && len(operand.Term.Encoding) != 0 {
			display = string(operand.Term.Encoding)
			break
		}
	}
	side := "left"
	if index > 0 {
		side = "right"
	}
	builder := explainDiagnostic(item)
	builder.message(fmt.Sprintf("%s operand `%s` of `..` may be nil", side, display))
	if origin, exists := concatOperandOriginEvidence(operationName, index, display, context.closure.Values); exists {
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, origin)
	}
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s operand `%s` has type nil", side, display))
	builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustUnknown, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("no guard on this path proves %s is non-nil", display))
	builder.label("value may be nil")
	return builder.build(fmt.Sprintf("Guard `%s` or provide a default string before using `..`.", display))
}

func narrateRedundantClaim(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	name := diagnosticOperationName(context.key)
	if operation, found := context.applies[name]; found {
		var argument []byte
		value := "value"
		for _, operand := range operation.Operands {
			if operand.Role == equation.IndexedRole(equation.RoleFamilyArgument, 0) {
				argument = operand.Term.Encoding
			}
			if operand.Role == equation.IndexedRole(equation.RoleFamilyArgumentDisplay, 0) {
				value = string(operand.Term.Encoding)
			}
		}
		if len(argument) == 0 {
			return item, true
		}
		argumentSpan := context.callSpans[operation.Target.Name+"/argument-00000000"]
		if !argumentSpan.Valid() {
			argumentSpan = item.Span
		}
		if value == "value" {
			value = strings.TrimPrefix(string(argument), "path/")
		}
		return narrateRedundantTypeProof(item, value, "string", argumentSpan, true)
	}
	if operation, found := context.claims[name]; found {
		operands, err := artifactOperandsByRole(operation.Operands, "value", "type")
		if err != nil {
			return item, true
		}
		target, err := strconv.Unquote(strings.TrimPrefix(string(operands["type"]), "claim-type/"))
		if err != nil {
			return item, true
		}
		value := strings.TrimPrefix(string(operands["value"]), "path/")
		for _, operand := range operation.Operands {
			if operand.Role == "source-display" && len(operand.Term.Encoding) != 0 {
				value = string(operand.Term.Encoding)
			}
		}
		return narrateRedundantTypeProof(item, value, target, item.Span, false)
	}
	return item, true
}

// narrateRedundantTypeProof is shared by authored claims and cast-call claims:
// only the proven value, target type, and value span differ in their output.
func narrateRedundantTypeProof(item PublishedDiagnostic, value, target string, valueSpan wir.Span, cast bool) (PublishedDiagnostic, bool) {
	builder := explainDiagnostic(item)
	if cast {
		builder.message("type cast call is redundant; value is already string")
	} else {
		builder.message("type claim is redundant; value is already " + target)
	}
	builder.evidenceAt(valueSpan, diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s is proven to be %s before the claim", value, target))
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("claim checks %s at this site", target))
	builder.label("claim site")
	builder.labelAt(valueSpan, "proven value")
	return builder.build("Remove the runtime type claim when the proven source type is sufficient.")
}

func narrateSendIsolation(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := context.applies[diagnosticOperationName(context.key)]
	if !found {
		return item, true
	}
	builder := explainDiagnostic(item)
	builder.label("send payload")
	switch builder.payload.Name {
	case "isolated":
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "isolation proof: direct fresh object literal has no retained graph identity")
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "direct literal birth site has no retained graph identity")
		builder.label("send-safety proof")
	case "immutable":
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "immutable proof: sent exact identity is frozen")
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "exact identity is frozen before send")
		builder.label("send-safety proof")
	case "escaped":
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "escape proof: payload has already crossed a retaining boundary")
	case "fallback":
		_, reason := sendIsolationPayload(operation)
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustUnknown, diagnostic.EvidenceReasonUnspecified, reason)
		builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustUnknown, diagnostic.EvidenceReasonUnspecified, reason)
	}
	return builder.build(sendIsolationHelp)
}

func narrateConstantGuard(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	operation, found := branchOperation(context.artifact, diagnosticOperationName(context.key))
	if !found {
		return item, true
	}
	builder := explainDiagnostic(item)
	if context.family.Code == diagnosticFamilies[DiagnosticFamilyAlwaysTrueGuard].Code {
		builder.message("condition is proven always true")
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "condition is proven to be true on every reachable path")
		builder.label("constant guard")
		return builder.build("Remove the guard or move the guarded code out of the branch.")
	}
	if context.family.Code != diagnosticFamilies[DiagnosticFamilyRedundantCondition].Code {
		return item, true
	}
	if string(item.Fact.Value) == "true" {
		builder.message("condition is always true here")
		builder.item.Help = "Remove this repeated check, or move any needed work into the branch already guarded above."
	} else {
		builder.message("condition is always false here")
		builder.item.Help = "Remove this unreachable branch, or change the prior guard if this path should still run."
	}
	current, currentOK := branchPredicateDescription(operation)
	prior, priorSpan, priorOK := enclosingBranchProof(operation, context.artifact, context.branchSpans)
	if !currentOK || !priorOK {
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "condition is proven constant under its enclosing guard")
		builder.label("constant guard")
		return builder.item, true
	}
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "current check: "+current)
	builder.evidenceAt(priorSpan, diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, "prior guard established "+prior)
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, strings.Split(current, " ")[0]+" is unchanged between the prior guard and this check")
	builder.label("current check")
	builder.labelAt(priorSpan, "prior guard")
	return builder.item, true
}

func narrateReturnContract(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	contract := context.family.body(context.key)
	operationName, indexed, hasSlot := strings.Cut(contract, "/")
	slot, member, _ := strings.Cut(indexed, "/")
	operation, found := context.publications[operationName]
	if !found || !hasSlot {
		return item, true
	}
	index, err := strconv.Atoi(slot)
	if err != nil || index < 0 {
		return item, true
	}
	display := ""
	for _, operand := range operation.Operands {
		if operand.Role == equation.IndexedRole(equation.RoleFamilyReturnDisplay, index) {
			display = string(operand.Term.Encoding)
			break
		}
	}
	var declaredTarget typ.Type
	for _, operand := range operation.Operands {
		if operand.Role != equation.IndexedRole(equation.RoleFamilyDeclaredReturn, index) {
			continue
		}
		if target, ok := shapefact.DecodeTarget(operand.Term.Encoding); ok && target != nil {
			declaredTarget = target
		}
		break
	}
	if declaredTarget == nil {
		return item, true
	}
	declared, subject := typeformat.Short(declaredTarget), returnValueSubject(index, display)
	contractSubject := fmt.Sprintf("returned value %d", index+1)
	declaredSpan := context.returnSpans[fmt.Sprintf("%s/declared-return-%08d", operationName, index)]
	if !declaredSpan.Valid() {
		declaredSpan = item.Span
	}
	if member != "" {
		fieldType, exists := recordFieldType(declaredTarget, member)
		if !exists {
			return item, true
		}
		declared = typeformat.Short(fieldType)
		subject, contractSubject = returnMemberSubject(index, member), returnMemberSubject(index, member)
	}
	builder := explainDiagnostic(item)
	if builder.payload.Flags&DiagnosticAnyBoundary != 0 {
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s has type any", subject))
		builder.evidenceAt(declaredSpan, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s must satisfy declared return type %s", contractSubject, declared))
		builder.evidence(diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, userAssertedAnyEvidence)
		builder.evidence(diagnostic.EvidencePrecisionBoundary, diagnostic.TrustUnknown, diagnostic.EvidenceReasonExplicitBoundaryValidation, fmt.Sprintf("%s comes from any/unknown", subject))
		builder.evidence(diagnostic.EvidenceMissingProof, diagnostic.TrustUnknown, diagnostic.EvidenceReasonBoundaryValidationMissing, fmt.Sprintf("no proof on this path shows %s satisfies the declared return type", subject))
		builder.label("returned value any")
		builder.labelAt(declaredSpan, "declared return type "+declared)
		return builder.build(returnContractHelp)
	}
	valueEvidence := fmt.Sprintf("%s has literal value %s", subject, builder.payload.Observed)
	if builder.payload.Flags&DiagnosticMayBeNil != 0 {
		valueEvidence = fmt.Sprintf("%s can be nil here", subject)
	}
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, valueEvidence)
	builder.evidenceAt(declaredSpan, diagnostic.EvidenceUserAssertion, diagnostic.TrustClaimed, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s must satisfy declared return type %s", contractSubject, declared))
	builder.label("returned value")
	builder.labelAt(declaredSpan, "declared return type "+declared)
	return builder.build(returnContractHelp)
}

func narrateOptionalAssignmentTarget(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	name := context.family.body(context.key)
	if operation, found := context.pathReplacements[name]; found {
		target, container := "", ""
		for _, operand := range operation.Operands {
			switch operand.Role {
			case "display":
				target = string(operand.Term.Encoding)
			case "write-container-display":
				container = string(operand.Term.Encoding)
			}
		}
		if target == "" || container == "" {
			return item, true
		}
		var witness []byte
		for _, fact := range context.closure.Values {
			if fact.Key == factkey.OptionalWriteContainer.Key().String()+name {
				witness = fact.Value
				break
			}
		}
		builder := explainDiagnostic(item)
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, optionalContainerEvidence(container, witness))
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("writing %s requires its container to be non-nil", target))
		builder.label("possibly nil container")
		builder.label("assignment target")
		return builder.build(fmt.Sprintf("Guard `%s` with a nil check before assigning through it, or write to a non-optional container.", container))
	}
	return item, true
}

func narrateMissingMember(item PublishedDiagnostic, context diagnosticNarrationContext) (PublishedDiagnostic, bool) {
	name := context.family.body(context.key)
	if operation, found := context.claims[name]; found {
		operands, err := artifactOperandsByRole(operation.Operands, "value")
		if err != nil {
			return item, true
		}
		value, available := claimDiagnosticValue(operands["value"], operation, context.closure)
		source := "member"
		for _, operand := range operation.Operands {
			if operand.Role == "source-display" && len(operand.Term.Encoding) != 0 {
				source = string(operand.Term.Encoding)
			}
		}
		member := source[strings.LastIndex(source, ".")+1:]
		if member == "" || member == source {
			return item, true
		}
		receiver := ""
		if available {
			if resolved, ok := memberMissingReceiver(value); ok {
				receiver = typeformat.Short(resolved)
			}
		}
		if receiver == "" {
			receiver = item.Payload.Observed
		}
		return narrateMemberRead(item, source, member, receiver)
	}
	if operation, found := context.writes[name]; found {
		operands, err := artifactOperandsByRole(operation.Operands, "source-display")
		if err != nil {
			return item, true
		}
		source := string(operands["source-display"])
		member := source[strings.LastIndex(source, ".")+1:]
		if bracket := strings.LastIndex(member, "["); bracket >= 0 {
			member = strings.Trim(strings.TrimSuffix(member[bracket+1:], "]"), "\"")
		}
		if item.Payload.Kind != diagnosticMemberMissing {
			return item, true
		}
		return narrateMemberRead(item, source, member, item.Payload.Observed)
	}
	if operation, found := context.applies[name]; found {
		receiver, method := "", ""
		for _, operand := range operation.Operands {
			switch operand.Role {
			case "receiver-display":
				receiver = string(operand.Term.Encoding)
			case "method":
				method, _ = callMethodName(operand.Term.Encoding)
			}
		}
		receiverType := item.Payload.Observed
		if item.Payload.Kind != diagnosticMemberMissing || receiver == "" || method == "" || receiverType == "" {
			return item, true
		}
		builder := explainDiagnostic(item)
		builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s.%s has receiver type %s", receiver, method, receiverType))
		builder.label("member call")
		return builder.build(fmt.Sprintf("Narrow the receiver before reading `%s`, or add `%s` to every reachable receiver shape.", method, method))
	}
	return item, true
}

// narrateMemberRead is shared by claim-owned and environment-owned reads: once
// their structured payload and operation operands identify the same source,
// member, and receiver type, their published explanation is byte-identical.
func narrateMemberRead(item PublishedDiagnostic, source, member, receiver string) (PublishedDiagnostic, bool) {
	if source == "" || member == "" || receiver == "" {
		return item, true
	}
	builder := explainDiagnostic(item)
	builder.evidence(diagnostic.EvidenceAbstractFact, diagnostic.TrustProven, diagnostic.EvidenceReasonUnspecified, fmt.Sprintf("%s reads member %q from receiver type %s", source, member, receiver))
	builder.label("member read")
	return builder.build(fmt.Sprintf("Narrow the receiver before reading `%s`, or add `%s` to every reachable receiver shape.", member, member))
}
