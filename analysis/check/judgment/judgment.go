// Package judgment defines the post-solve obligation records that diagnostics
// render. It carries semantic identities plus the stable descriptor metadata
// shared by policy and rendering layers; user-facing message construction stays
// in diagnostics.
package judgment

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Code is the stable semantic judgment code. Rendering layers may map it to a
// diagnostic code, message, and default severity.
type Code string

const (
	CodeCallArgType                  Code = "call.argument.type"
	CodeCallArity                    Code = "call.arity"
	CodeCallCallee                   Code = "call.callee"
	CodeAssignment                   Code = "assignment.type"
	CodeAssignmentTarget             Code = "assignment.optional_target"
	CodeReturn                       Code = "return.type"
	CodeNonNilAssertion              Code = "assertion.nonnil"
	CodeNumericForOperand            Code = "for.numeric.operand"
	CodeFrozenTable                  Code = "effect.freeze.mutation"
	CodeLifecycle                    Code = "effect.lifecycle.unreleased"
	CodeUnusedLocal                  Code = "lint.unused.local"
	CodeDeadAssignment               Code = "lint.dead.assignment"
	CodeChannelSelect                Code = "channel.select.exhaustiveness"
	CodeDiscriminatedUnion           Code = "union.discriminated.exhaustiveness"
	CodeOptional                     Code = "union.optional.exhaustiveness"
	CodeResultShape                  Code = "union.result_shape.exhaustiveness"
	CodeRegistration                 Code = "union.registration.exhaustiveness"
	CodeTableDispatch                Code = "union.table_dispatch.exhaustiveness"
	CodeUnresolvedValue              Code = "value.reference.unresolved"
	CodeUnresolvedType               Code = "type.reference.unresolved"
	CodeRedundantCondition           Code = "condition.redundant"
	CodeMemberRead                   Code = "member.read"
	CodeConcatOperand                Code = "operator.concat.operand"
	CodeSendIsolation                Code = "send.isolation"
	CodeAdviceRedundantClaim         Code = "advice.redundant_claim"
	CodeAdviceAlwaysTrueGuard        Code = "advice.always_true_guard"
	CodeAdviceInvariantLoopRead      Code = "advice.invariant_loop_read"
	CodeAdviceSplitBirthDiscriminant Code = "advice.split_birth_discriminant"
)

// Verdict classifies whether the solved state proves or refutes an obligation.
// Policy decides how each verdict maps to severity.
type Verdict uint8

const (
	VerdictUnknown Verdict = iota
	VerdictProven
	VerdictRefuted
)

// SubjectKind identifies the stable subject namespace used for deduplication
// and precedence.
type SubjectKind uint8

const (
	SubjectUnknown SubjectKind = iota
	SubjectExpression
	SubjectPath
	SubjectCallExpression
	SubjectCallArgument
	SubjectReturnValue
)

// SubjectRef is a renderer-independent identity for the code location or value
// being judged. Key is canonical within Kind and Point.
type SubjectRef struct {
	FunctionKey string
	Kind        SubjectKind
	Key         string
	Label       string
}

// NewSubjectRef builds a stable subject identity. FunctionKey should identify
// the analyzed body/content, not a rendered function name.
func NewSubjectRef(functionKey string, kind SubjectKind, key string) SubjectRef {
	return SubjectRef{FunctionKey: functionKey, Kind: kind, Key: key}
}

// WithLabel returns s with a renderer-facing subject label. The label is not
// part of StableKey; it preserves user-facing source identity without letting
// renderers inspect syntax.
func (s SubjectRef) WithLabel(label string) SubjectRef {
	s.Label = label
	return s
}

// StableKey returns a deterministic identity for dedup and precedence.
func (s SubjectRef) StableKey() string {
	var b strings.Builder
	b.WriteString(s.FunctionKey)
	b.WriteByte('|')
	b.WriteString(subjectKindString(s.Kind))
	b.WriteByte('|')
	b.WriteString(s.Key)
	return b.String()
}

// TypeRef names a resolved type. The concrete type value remains owned by the
// read model or contract provider; judgments keep a stable reference for
// matching and rendering.
type TypeRef struct {
	Key   string
	Type  typ.Type
	Label string
}

// ValueRef names a solved abstract value at a read boundary.
type ValueRef struct {
	Key           string
	ProjectedType typ.Type
	Label         string
}

// OriginRef points at the solved origin of evidence without exposing syntax
// nodes to judgment consumers.
type OriginRef struct {
	Point cfg.Point
	Key   string
}

const (
	OriginFrozenTableMutation = "frozen-table:mutation"
	OriginFrozenTableProof    = "frozen-table:proof"
	OriginSendSafety          = "send:safety"
	OriginSendIsolationProof  = "send:isolation-proof"
	OriginSendImmutableProof  = "send:immutable-proof"
)

// EvidenceKind classifies a structured proof step.
type EvidenceKind uint8

const (
	EvidenceUnknown EvidenceKind = iota
	EvidenceAbstractFact
	EvidenceUserAssertion
	EvidenceMissingProof
	EvidencePrecisionBoundary
)

// EvidenceTrust classifies how strongly an evidence node supports the
// judgment.
type EvidenceTrust uint8

const (
	EvidenceTrustUnknown EvidenceTrust = iota
	EvidenceTrustProven
	EvidenceTrustClaimed
	EvidenceTrustRefuted
)

// EvidenceDetailKind classifies structured evidence details that renderers may
// phrase for a diagnostic family. It is intentionally semantic data, not text.
type EvidenceDetailKind uint8

const (
	EvidenceDetailNone EvidenceDetailKind = iota
	EvidenceDetailMissingRequiredField
	EvidenceDetailMissingRequiredMethod
	EvidenceDetailMethodTypeMismatch
	EvidenceDetailMayBeNil
	EvidenceDetailGenericConflict
	EvidenceDetailArityTooFew
	EvidenceDetailArityTooMany
	EvidenceDetailCalleeNotCallable
	EvidenceDetailCalleeMayBeNil
	EvidenceDetailMemberMissing
	EvidenceDetailCallParamObligation
	EvidenceDetailAssignmentSourceContribution
	EvidenceDetailAssignmentCallInvalidation
	EvidenceDetailAssignmentParentActual
	EvidenceDetailAssignmentParentExpected
	EvidenceDetailDynamicAssignmentTarget
	EvidenceDetailUserAssertedAny
	EvidenceDetailCallResultAssignment
	EvidenceDetailIndexedReadMissingProof
	EvidenceDetailFrozenTableAssignment
	EvidenceDetailFrozenTableCall
	EvidenceDetailLifecycleAcquire
	EvidenceDetailLifecycleTransition
	EvidenceDetailLifecycleEscape
	EvidenceDetailLifecycleMissingProof
	EvidenceDetailDeadAssignmentOverwrite
	EvidenceDetailDeadAssignmentExit
	EvidenceDetailChannelSelectResult
	EvidenceDetailChannelSelectHandled
	EvidenceDetailChannelSelectMissing
	EvidenceDetailChannelSelectNoDefault
	EvidenceDetailDiscriminatedUnionTarget
	EvidenceDetailDiscriminatedUnionPossible
	EvidenceDetailDiscriminatedUnionHandled
	EvidenceDetailDiscriminatedUnionMissing
	EvidenceDetailDiscriminatedUnionNoDefault
	EvidenceDetailOptionalTarget
	EvidenceDetailOptionalPossible
	EvidenceDetailOptionalConsumed
	EvidenceDetailOptionalMissing
	EvidenceDetailOptionalNoDefault
	EvidenceDetailResultShapeUnion
	EvidenceDetailResultShapeFieldCase
	EvidenceDetailResultShapeMissingProof
	EvidenceDetailRegistrationDispatch
	EvidenceDetailRegistrationPossible
	EvidenceDetailRegistrationRegistered
	EvidenceDetailRegistrationMissing
	EvidenceDetailTableDispatchLookup
	EvidenceDetailTableDispatchPossible
	EvidenceDetailTableDispatchKeys
	EvidenceDetailTableDispatchMissing
	EvidenceDetailRedundantConditionCheck
	EvidenceDetailRedundantConditionProof
	EvidenceDetailRedundantConditionStability
	EvidenceDetailConcatOperand
	EvidenceDetailSendSafetyFact
	EvidenceDetailSendSafetyProof
	EvidenceDetailSendSafetyBlocker
	EvidenceDetailAdviceClaimSite
	EvidenceDetailAdviceProvenType
	EvidenceDetailAdviceGuardValue
	EvidenceDetailAdviceLoopInvariant
	EvidenceDetailAdviceReceiverNonNil
	EvidenceDetailAdviceTableBirth
	EvidenceDetailAdviceTagWrite
	EvidenceDetailAdvicePayloadWrite
	EvidenceDetailAdviceDiscriminantUse
)

// EvidenceDetail carries renderer-independent detail for one evidence node.
type EvidenceDetail struct {
	Kind           EvidenceDetailKind
	Field          string
	FieldType      typ.Type
	ActualType     typ.Type
	Param          string
	Callable       bool
	MemberAccess   bool
	ExpectedCount  int
	ActualCount    int
	FunctionName   string
	SubjectLabel   string
	ProviderLabel  string
	MemberParam    int
	ResultIndex    int
	UnderSupplied  bool
	ExpandedSource bool
	Resource       string
	Protocol       string
	CurrentState   string
	FromState      string
	ToState        string
	FinalState     string
	CaseList       string
	Message        string
	Always         bool
}

func RedundantConditionCheckEvidenceDetail(message string, always bool) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailRedundantConditionCheck, Message: message, Always: always}
}

func RedundantConditionProofEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailRedundantConditionProof, Message: message}
}

func RedundantConditionStabilityEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailRedundantConditionStability, Message: message}
}

func AdviceClaimSiteEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceClaimSite, Message: message}
}

func AdviceProvenTypeEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceProvenType, Message: message}
}

func AdviceGuardValueEvidenceDetail(message string, always bool) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceGuardValue, Message: message, Always: always}
}

func AdviceLoopInvariantEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceLoopInvariant, Message: message}
}

func AdviceReceiverNonNilEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceReceiverNonNil, Message: message}
}

func AdviceTableBirthEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceTableBirth, Message: message}
}

func AdviceTagWriteEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceTagWrite, Message: message}
}

func AdvicePayloadWriteEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdvicePayloadWrite, Message: message}
}

func AdviceDiscriminantUseEvidenceDetail(message string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailAdviceDiscriminantUse, Message: message}
}

func ResultShapeUnionEvidenceDetail(receiver, discriminant string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailResultShapeUnion, SubjectLabel: receiver, Field: discriminant}
}

func ResultShapeFieldCaseEvidenceDetail(readPath, requiredCase string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailResultShapeFieldCase, SubjectLabel: readPath, CaseList: requiredCase}
}

func ResultShapeMissingProofEvidenceDetail(requiredCase string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailResultShapeMissingProof, CaseList: requiredCase}
}

func RegistrationDispatchEvidenceDetail(registry, target string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailRegistrationDispatch, SubjectLabel: registry, Field: target}
}

func RegistrationPossibleEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailRegistrationPossible, CaseList: cases}
}

func RegistrationRegisteredEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailRegistrationRegistered, CaseList: cases}
}

func RegistrationMissingEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailRegistrationMissing, CaseList: cases}
}

func TableDispatchLookupEvidenceDetail(table, target string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailTableDispatchLookup, SubjectLabel: table, Field: target}
}

func TableDispatchPossibleEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailTableDispatchPossible, CaseList: cases}
}

func TableDispatchKeysEvidenceDetail(keys string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailTableDispatchKeys, CaseList: keys}
}

func TableDispatchMissingEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailTableDispatchMissing, CaseList: cases}
}

func DiscriminatedUnionTargetEvidenceDetail(target string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailDiscriminatedUnionTarget, SubjectLabel: target}
}

func DiscriminatedUnionPossibleEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailDiscriminatedUnionPossible, CaseList: cases}
}

func DiscriminatedUnionHandledEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailDiscriminatedUnionHandled, CaseList: cases}
}

func DiscriminatedUnionMissingEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailDiscriminatedUnionMissing, CaseList: cases}
}

func DiscriminatedUnionNoDefaultEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailDiscriminatedUnionNoDefault}
}

func OptionalTargetEvidenceDetail(target string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailOptionalTarget, SubjectLabel: target}
}

func OptionalPossibleEvidenceDetail(target string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailOptionalPossible, SubjectLabel: target}
}

func OptionalConsumedEvidenceDetail(target string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailOptionalConsumed, SubjectLabel: target}
}

func OptionalMissingEvidenceDetail(cases string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailOptionalMissing, CaseList: cases}
}

func OptionalNoDefaultEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailOptionalNoDefault}
}

// MissingRequiredFieldEvidenceDetail records that a structural proof failed
// because a required record field is absent.
func MissingRequiredFieldEvidenceDetail(field string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMissingRequiredField, Field: field}
}

// MissingRequiredFieldTypeEvidenceDetail records an absent required record
// field and its expected field type.
func MissingRequiredFieldTypeEvidenceDetail(field string, fieldType typ.Type) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMissingRequiredField, Field: field, FieldType: fieldType}
}

// MissingRequiredMethodTypeEvidenceDetail records an absent required interface
// method and its expected method type.
func MissingRequiredMethodTypeEvidenceDetail(method string, methodType typ.Type) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMissingRequiredMethod, Field: method, FieldType: methodType}
}

// MethodTypeMismatchEvidenceDetail records an interface method whose provided
// type does not satisfy the required method signature.
func MethodTypeMismatchEvidenceDetail(method string, actual, expected typ.Type) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMethodTypeMismatch, Field: method, ActualType: actual, FieldType: expected}
}

// MayBeNilEvidenceDetail records that the argument value may be nil while the
// parameter contract does not accept nil.
func MayBeNilEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMayBeNil}
}

// GenericConflictEvidenceDetail records that one generic type parameter was
// inferred from incompatible argument evidence.
func GenericConflictEvidenceDetail(param string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailGenericConflict, Param: param}
}

// ArityTooFewEvidenceDetail records a call with fewer arguments than the
// callable contract requires.
func ArityTooFewEvidenceDetail(expected, actual int) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailArityTooFew, ExpectedCount: expected, ActualCount: actual}
}

// ArityTooManyEvidenceDetail records a call with more arguments than the
// callable contract accepts.
func ArityTooManyEvidenceDetail(expected, actual int) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailArityTooMany, ExpectedCount: expected, ActualCount: actual}
}

// CalleeNotCallableEvidenceDetail records a call whose target has a concrete
// non-callable type.
func CalleeNotCallableEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailCalleeNotCallable}
}

// MemberCalleeNotCallableEvidenceDetail records a member call whose target has
// a concrete non-callable type.
func MemberCalleeNotCallableEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailCalleeNotCallable, MemberAccess: true}
}

// CalleeMayBeNilEvidenceDetail records a call whose target may be nil before
// it is invoked.
func CalleeMayBeNilEvidenceDetail(callable bool) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailCalleeMayBeNil, Callable: callable}
}

// MemberCalleeMayBeNilEvidenceDetail records a member call whose target may be
// nil before it is invoked.
func MemberCalleeMayBeNilEvidenceDetail(callable bool) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailCalleeMayBeNil, Callable: callable, MemberAccess: true}
}

// MemberMissingEvidenceDetail records a member call whose receiver lacks the
// requested member.
func MemberMissingEvidenceDetail(member string) EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailMemberMissing, Field: member, MemberAccess: true}
}

// CallParamObligationEvidenceDetail records that a caller argument is being
// checked because the callee body forwarded it into a member parameter.
func CallParamObligationEvidenceDetail(functionName, subjectLabel, providerLabel string, memberParam int) EvidenceDetail {
	return EvidenceDetail{
		Kind:          EvidenceDetailCallParamObligation,
		FunctionName:  functionName,
		SubjectLabel:  subjectLabel,
		ProviderLabel: providerLabel,
		MemberParam:   memberParam,
	}
}

// AssignmentSourceContributionEvidenceDetail records a prior assignment that
// contributes one concrete arm to a later source read.
func AssignmentSourceContributionEvidenceDetail(rootLabel, readLabel string, t typ.Type) EvidenceDetail {
	return EvidenceDetail{
		Kind:          EvidenceDetailAssignmentSourceContribution,
		ProviderLabel: rootLabel,
		SubjectLabel:  readLabel,
		FieldType:     t,
	}
}

// AssignmentCallInvalidationEvidenceDetail records a prior call that may have
// invalidated the assignment source read.
func AssignmentCallInvalidationEvidenceDetail(callLabel, invalidatedLabel, readLabel string) EvidenceDetail {
	return EvidenceDetail{
		Kind:          EvidenceDetailAssignmentCallInvalidation,
		ProviderLabel: callLabel,
		Field:         invalidatedLabel,
		SubjectLabel:  readLabel,
	}
}

// AssignmentParentActualEvidenceDetail records the source object that produced
// a projected member-assignment obligation.
func AssignmentParentActualEvidenceDetail(label string, t typ.Type) EvidenceDetail {
	return EvidenceDetail{
		Kind:         EvidenceDetailAssignmentParentActual,
		SubjectLabel: label,
		FieldType:    t,
	}
}

// AssignmentParentExpectedEvidenceDetail records the enclosing declared target
// type for a projected member-assignment obligation.
func AssignmentParentExpectedEvidenceDetail(label string, t typ.Type) EvidenceDetail {
	return EvidenceDetail{
		Kind:         EvidenceDetailAssignmentParentExpected,
		SubjectLabel: label,
		FieldType:    t,
	}
}

// DynamicAssignmentTargetEvidenceDetail records that an assignment target type
// comes from a dynamic write whose value must satisfy every possible target
// slot, rather than from an explicit local annotation.
func DynamicAssignmentTargetEvidenceDetail(subjectLabel string) EvidenceDetail {
	return EvidenceDetail{
		Kind:         EvidenceDetailDynamicAssignmentTarget,
		SubjectLabel: subjectLabel,
	}
}

// UserAssertedAnyEvidenceDetail records an explicit user any/unknown assertion
// that is not an abstract-interpreter proof.
func UserAssertedAnyEvidenceDetail(subjectLabel string) EvidenceDetail {
	return EvidenceDetail{
		Kind:         EvidenceDetailUserAssertedAny,
		SubjectLabel: subjectLabel,
	}
}

// IndexedReadMissingProofEvidenceDetail records that an indexed read may miss
// or read nil unless the current path proves the selected slot is valid.
func IndexedReadMissingProofEvidenceDetail() EvidenceDetail {
	return EvidenceDetail{Kind: EvidenceDetailIndexedReadMissingProof}
}

// CallResultAssignmentEvidenceDetail records that an assignment source is a
// specific result slot from a callable.
func CallResultAssignmentEvidenceDetail(functionName string, resultIndex int) EvidenceDetail {
	return EvidenceDetail{
		Kind:         EvidenceDetailCallResultAssignment,
		FunctionName: functionName,
		ResultIndex:  resultIndex,
	}
}

// UnderSuppliedCallResultAssignmentEvidenceDetail records that a target receives
// a call result slot the callee does not produce, so Lua fills the slot with nil.
func UnderSuppliedCallResultAssignmentEvidenceDetail(functionName string, resultIndex int) EvidenceDetail {
	detail := CallResultAssignmentEvidenceDetail(functionName, resultIndex)
	detail.UnderSupplied = true
	return detail
}

// Evidence is one structured proof or missing-proof step. It carries stable
// origin identity only; renderers own wording.
type Evidence struct {
	Kind   EvidenceKind
	Trust  EvidenceTrust
	Origin OriginRef
	Detail EvidenceDetail
	Span   SpanRef
}

// EvidenceChain is a deterministic, joinable list of evidence nodes.
type EvidenceChain []Evidence

// HasEvidence reports whether the judgment carries at least one evidence node
// of kind.
func (j Judgment) HasEvidence(kind EvidenceKind) bool {
	return j.Evidence.Has(kind)
}

// EvidenceTrustFor returns the trust of the first evidence node of kind.
func (j Judgment) EvidenceTrustFor(kind EvidenceKind) (EvidenceTrust, bool) {
	return j.Evidence.TrustFor(kind)
}

// FirstEvidence returns the first evidence node of kind.
func (j Judgment) FirstEvidence(kind EvidenceKind) (Evidence, bool) {
	return j.Evidence.First(kind)
}

// FirstEvidenceDetail returns the first evidence node carrying detail.
func (j Judgment) FirstEvidenceDetail(detail EvidenceDetailKind) (Evidence, bool) {
	return j.Evidence.FirstDetail(detail)
}

// FirstEvidenceKindDetail returns the first evidence node whose outer kind and
// structured detail both match.
func (j Judgment) FirstEvidenceKindDetail(kind EvidenceKind, detail EvidenceDetailKind) (Evidence, bool) {
	return j.Evidence.FirstKindDetail(kind, detail)
}

// HasEvidenceDetail reports whether any evidence node carries detail.
func (j Judgment) HasEvidenceDetail(detail EvidenceDetailKind) bool {
	return j.Evidence.HasDetail(detail)
}

// HasEvidenceKindDetail reports whether any evidence node matches both kind
// and detail.
func (j Judgment) HasEvidenceKindDetail(kind EvidenceKind, detail EvidenceDetailKind) bool {
	return j.Evidence.HasKindDetail(kind, detail)
}

// HasAnyEvidenceKindDetail reports whether any evidence node matches kind and
// one of the supplied details.
func (j Judgment) HasAnyEvidenceKindDetail(kind EvidenceKind, details ...EvidenceDetailKind) bool {
	return j.Evidence.HasAnyKindDetail(kind, details...)
}

// EvidenceKindDetails returns the evidence nodes matching kind and detail in
// chain order.
func (j Judgment) EvidenceKindDetails(kind EvidenceKind, detail EvidenceDetailKind) EvidenceChain {
	return j.Evidence.KindDetails(kind, detail)
}

// EvidenceOfKind returns evidence nodes of kind in chain order.
func (j Judgment) EvidenceOfKind(kind EvidenceKind) EvidenceChain {
	return j.Evidence.OfKind(kind)
}

// Has reports whether the chain carries at least one evidence node of kind.
func (c EvidenceChain) Has(kind EvidenceKind) bool {
	_, ok := c.TrustFor(kind)
	return ok
}

// TrustFor returns the trust of the first evidence node of kind.
func (c EvidenceChain) TrustFor(kind EvidenceKind) (EvidenceTrust, bool) {
	if item, ok := c.First(kind); ok {
		return item.Trust, true
	}
	return EvidenceTrustUnknown, false
}

// First returns the first evidence node of kind.
func (c EvidenceChain) First(kind EvidenceKind) (Evidence, bool) {
	for _, item := range c {
		if item.Kind == kind {
			return item, true
		}
	}
	return Evidence{}, false
}

// FirstDetail returns the first evidence node carrying detail.
func (c EvidenceChain) FirstDetail(detail EvidenceDetailKind) (Evidence, bool) {
	for _, item := range c {
		if item.Detail.Kind == detail {
			return item, true
		}
	}
	return Evidence{}, false
}

// FirstKindDetail returns the first evidence node whose outer kind and
// structured detail both match.
func (c EvidenceChain) FirstKindDetail(kind EvidenceKind, detail EvidenceDetailKind) (Evidence, bool) {
	for _, item := range c {
		if item.Kind == kind && item.Detail.Kind == detail {
			return item, true
		}
	}
	return Evidence{}, false
}

// HasDetail reports whether any evidence node carries detail.
func (c EvidenceChain) HasDetail(detail EvidenceDetailKind) bool {
	_, ok := c.FirstDetail(detail)
	return ok
}

// HasKindDetail reports whether any evidence node matches both kind and detail.
func (c EvidenceChain) HasKindDetail(kind EvidenceKind, detail EvidenceDetailKind) bool {
	_, ok := c.FirstKindDetail(kind, detail)
	return ok
}

// HasAnyKindDetail reports whether any evidence node matches kind and one of
// the supplied details.
func (c EvidenceChain) HasAnyKindDetail(kind EvidenceKind, details ...EvidenceDetailKind) bool {
	for _, detail := range details {
		if c.HasKindDetail(kind, detail) {
			return true
		}
	}
	return false
}

// KindDetails returns evidence nodes matching kind and detail in chain order.
func (c EvidenceChain) KindDetails(kind EvidenceKind, detail EvidenceDetailKind) EvidenceChain {
	var out EvidenceChain
	for _, item := range c {
		if item.Kind == kind && item.Detail.Kind == detail {
			out = append(out, item)
		}
	}
	return out
}

// OfKind returns evidence nodes of kind in chain order.
func (c EvidenceChain) OfKind(kind EvidenceKind) EvidenceChain {
	var out EvidenceChain
	for _, item := range c {
		if item.Kind == kind {
			out = append(out, item)
		}
	}
	return out
}

// JoinEvidenceChains joins branch evidence. Evidence present with the same
// shape and trust on both branches remains as-is. One-sided or conflicting
// evidence remains visible, but is degraded to an unknown precision-boundary
// node so obligation renderers cannot silently treat one branch as proof for
// all paths.
func JoinEvidenceChains(a, b EvidenceChain) EvidenceChain {
	if len(a) == 0 {
		return degradeOneSidedEvidence(b)
	}
	if len(b) == 0 {
		return degradeOneSidedEvidence(a)
	}
	merged := make(map[evidenceIdentity]evidenceJoinState, len(a)+len(b))
	for _, item := range a {
		state := merged[item.identity()]
		state.left = &item
		merged[item.identity()] = state
	}
	for _, item := range b {
		state := merged[item.identity()]
		state.right = &item
		merged[item.identity()] = state
	}
	ids := make([]evidenceIdentity, 0, len(merged))
	for id := range merged {
		ids = append(ids, id)
	}
	sortEvidenceIdentities(ids)

	out := make(EvidenceChain, 0, len(ids))
	for _, id := range ids {
		state := merged[id]
		switch {
		case state.left != nil && state.right != nil:
			if state.left.Trust == state.right.Trust {
				out = append(out, *state.left)
			} else {
				out = append(out, precisionBoundaryEvidence(*state.left))
			}
		case state.left != nil:
			out = append(out, precisionBoundaryEvidence(*state.left))
		case state.right != nil:
			out = append(out, precisionBoundaryEvidence(*state.right))
		}
	}
	return out
}

// SpanRef points at a source range captured during lowering or solved-state
// projection. File is optional when Point/Subject already determines it.
type SpanRef struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Judgment is the semantic obligation record emitted after solve and consumed
// by rendering/dedup/policy layers.
type Judgment struct {
	Code     Code
	Point    cfg.Point
	Subject  SubjectRef
	Expected TypeRef
	Actual   ValueRef
	Verdict  Verdict
	Evidence EvidenceChain
	Spans    []SpanRef
}

type evidenceJoinState struct {
	left  *Evidence
	right *Evidence
}

type evidenceIdentity struct {
	kind   EvidenceKind
	point  cfg.Point
	key    string
	detail EvidenceDetail
}

func (e Evidence) identity() evidenceIdentity {
	return evidenceIdentity{kind: e.Kind, point: e.Origin.Point, key: e.Origin.Key, detail: e.Detail}
}

func degradeOneSidedEvidence(in EvidenceChain) EvidenceChain {
	if len(in) == 0 {
		return nil
	}
	out := make(EvidenceChain, len(in))
	for i, item := range in {
		out[i] = precisionBoundaryEvidence(item)
	}
	return out
}

func precisionBoundaryEvidence(item Evidence) Evidence {
	item.Kind = EvidencePrecisionBoundary
	item.Trust = EvidenceTrustUnknown
	return item
}

func sortEvidenceIdentities(ids []evidenceIdentity) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && evidenceIdentityLess(ids[j], ids[j-1]); j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}

func evidenceIdentityLess(a, b evidenceIdentity) bool {
	if a.point != b.point {
		return a.point < b.point
	}
	if a.key != b.key {
		return a.key < b.key
	}
	if a.detail.Kind != b.detail.Kind {
		return a.detail.Kind < b.detail.Kind
	}
	if a.detail.Field != b.detail.Field {
		return a.detail.Field < b.detail.Field
	}
	if a.detail.Param != b.detail.Param {
		return a.detail.Param < b.detail.Param
	}
	if a.detail.Callable != b.detail.Callable {
		return !a.detail.Callable && b.detail.Callable
	}
	if a.detail.MemberAccess != b.detail.MemberAccess {
		return !a.detail.MemberAccess && b.detail.MemberAccess
	}
	if a.detail.ExpectedCount != b.detail.ExpectedCount {
		return a.detail.ExpectedCount < b.detail.ExpectedCount
	}
	if a.detail.ActualCount != b.detail.ActualCount {
		return a.detail.ActualCount < b.detail.ActualCount
	}
	if a.detail.ResultIndex != b.detail.ResultIndex {
		return a.detail.ResultIndex < b.detail.ResultIndex
	}
	if a.detail.UnderSupplied != b.detail.UnderSupplied {
		return !a.detail.UnderSupplied && b.detail.UnderSupplied
	}
	if a.detail.Message != b.detail.Message {
		return a.detail.Message < b.detail.Message
	}
	if a.detail.Always != b.detail.Always {
		return !a.detail.Always && b.detail.Always
	}
	return a.kind < b.kind
}

func subjectKindString(kind SubjectKind) string {
	switch kind {
	case SubjectExpression:
		return "expr"
	case SubjectPath:
		return "path"
	case SubjectCallExpression:
		return "call"
	case SubjectCallArgument:
		return "call_arg"
	case SubjectReturnValue:
		return "return"
	default:
		return "unknown"
	}
}
