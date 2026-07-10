package judgment

import "sort"

// CodeFamily groups judgment codes by semantic owner.
type CodeFamily string

const (
	FamilyAdvice     CodeFamily = "advice"
	FamilyAssertion  CodeFamily = "assertion"
	FamilyAssignment CodeFamily = "assignment"
	FamilyCall       CodeFamily = "call"
	FamilyChannel    CodeFamily = "channel"
	FamilyCondition  CodeFamily = "condition"
	FamilyEffect     CodeFamily = "effect"
	FamilyFor        CodeFamily = "for"
	FamilyLint       CodeFamily = "lint"
	FamilyMember     CodeFamily = "member"
	FamilyOperator   CodeFamily = "operator"
	FamilyReturn     CodeFamily = "return"
	FamilySend       CodeFamily = "send"
	FamilyType       CodeFamily = "type"
	FamilyUnion      CodeFamily = "union"
	FamilyValue      CodeFamily = "value"
)

// PolicyProfile names a descriptor-backed default severity table.
type PolicyProfile string

const (
	PolicyStrictnessTunableTypeError PolicyProfile = "strictness_tunable_type_error"
	PolicyRefutedError               PolicyProfile = "refuted_error"
	PolicyRefutedWarning             PolicyProfile = "refuted_warning"
	PolicyHint                       PolicyProfile = "hint"
	PolicyProvenHint                 PolicyProfile = "proven_hint"
)

// RenderKey names the diagnostics renderer bound to a judgment code.
type RenderKey string

const (
	RenderAdvice                   RenderKey = "advice"
	RenderAssignment               RenderKey = "assignment"
	RenderCallArity                RenderKey = "call_arity"
	RenderCallCallee               RenderKey = "call_callee"
	RenderChannelLifecycle         RenderKey = "channel_lifecycle"
	RenderChannelSelect            RenderKey = "channel_select"
	RenderConcatOperand            RenderKey = "concat_operand"
	RenderDeadAssignment           RenderKey = "dead_assignment"
	RenderDirectCallArgument       RenderKey = "direct_call_argument"
	RenderDiscriminatedUnion       RenderKey = "discriminated_union"
	RenderFrozenTable              RenderKey = "frozen_table"
	RenderLifecycle                RenderKey = "lifecycle"
	RenderMemberRead               RenderKey = "member_read"
	RenderNonNilAssertion          RenderKey = "nonnil_assertion"
	RenderNumericFor               RenderKey = "numeric_for"
	RenderOptional                 RenderKey = "optional"
	RenderOptionalAssignmentTarget RenderKey = "optional_assignment_target"
	RenderRedundantCondition       RenderKey = "redundant_condition"
	RenderRegistration             RenderKey = "registration"
	RenderResultShape              RenderKey = "result_shape"
	RenderReturn                   RenderKey = "return"
	RenderSendIsolation            RenderKey = "send_isolation"
	RenderTableDispatch            RenderKey = "table_dispatch"
	RenderTypestateInvalid         RenderKey = "typestate_invalid_transition"
	RenderTypestateRequirement     RenderKey = "typestate_requirement"
	RenderUnresolvedType           RenderKey = "unresolved_type"
	RenderUnresolvedValue          RenderKey = "unresolved_value"
	RenderUnusedLocal              RenderKey = "unused_local"
)

type DiagnosticDefault string

const (
	DiagnosticDefaultEnabled DiagnosticDefault = "enabled"
	DiagnosticDefaultOptIn   DiagnosticDefault = "opt_in"
)

type DiagnosticCode string

const (
	DiagnosticCodeAssignmentType               DiagnosticCode = "type.assignment"
	DiagnosticCodeMissingMember                DiagnosticCode = "type.member.missing"
	DiagnosticCodeOptionalMethodCall           DiagnosticCode = "type.call.optional_receiver"
	DiagnosticCodeNotCallable                  DiagnosticCode = "type.call.not_callable"
	DiagnosticCodeDirectCallNotCallable        DiagnosticCode = "type.call.direct.not_callable"
	DiagnosticCodeDirectCallTooFewArgs         DiagnosticCode = "type.call.direct.too_few_args"
	DiagnosticCodeDirectCallTooManyArgs        DiagnosticCode = "type.call.direct.too_many_args"
	DiagnosticCodeDirectCallArgType            DiagnosticCode = "type.call.direct.argument_type"
	DiagnosticCodeReturnContractType           DiagnosticCode = "type.return.contract"
	DiagnosticCodeDirectCallResultAssignment   DiagnosticCode = "type.call.direct.result_assignment"
	DiagnosticCodeOptionalAssignmentTarget     DiagnosticCode = "type.assignment.optional_target"
	DiagnosticCodeConcatOperand                DiagnosticCode = "type.operator.concat_operand"
	DiagnosticCodeNonNilAssertAlwaysNil        DiagnosticCode = "type.assert.nonnil_always_nil"
	DiagnosticCodeNumericForOperand            DiagnosticCode = "type.for.numeric_operand"
	DiagnosticCodeChannelSelectExhaustive      DiagnosticCode = "channel.select.exhaustiveness"
	DiagnosticCodeChannelSendClosed            DiagnosticCode = "channel.send.closed"
	DiagnosticCodeChannelDoubleClose           DiagnosticCode = "channel.close.closed"
	DiagnosticCodeUnresolvedTypeReference      DiagnosticCode = "type.reference.unresolved"
	DiagnosticCodeUnresolvedValueReference     DiagnosticCode = "value.reference.unresolved"
	DiagnosticCodeUnusedLocal                  DiagnosticCode = "lint.unused.local"
	DiagnosticCodeDeadAssignment               DiagnosticCode = "lint.dead.assignment"
	DiagnosticCodeRedundantCondition           DiagnosticCode = "lint.condition.redundant"
	DiagnosticCodeDiscriminatedUnionExhaustive DiagnosticCode = "lint.union.exhaustiveness"
	DiagnosticCodeFrozenTableMutation          DiagnosticCode = "effect.freeze.mutation"
	DiagnosticCodeResourceUnreleased           DiagnosticCode = "effect.lifecycle.unreleased"
	DiagnosticCodeTypestateInvalidTransition   DiagnosticCode = "typestate.invalid_transition"
	DiagnosticCodeTypestateInvalidRequirement  DiagnosticCode = "typestate.invalid_requirement"
	DiagnosticCodeTypestateUnprovenRequirement DiagnosticCode = "typestate.unproven_requirement"
	DiagnosticCodeSendIsolation                DiagnosticCode = "send.isolation"
	DiagnosticCodeAdviceRedundantClaim         DiagnosticCode = "advice.redundant_claim"
	DiagnosticCodeAdviceAlwaysTrueGuard        DiagnosticCode = "advice.always_true_guard"
	DiagnosticCodeAdviceInvariantLoopRead      DiagnosticCode = "advice.invariant_loop_read"
	DiagnosticCodeAdviceSplitBirthDiscriminant DiagnosticCode = "advice.split_birth_discriminant"
	DiagnosticCodeAdviceShapePolymorphic       DiagnosticCode = "advice.shape.polymorphic"
)

// RepairKind names a structured, renderer-independent repair family. A code
// spec only advertises a repair when its solved judgment contains enough
// semantic evidence for the service layer to project a candidate without
// guessing from diagnostic text.
type RepairKind string

const (
	RepairRemoveRedundantClaim   RepairKind = "remove_redundant_claim"
	RepairRemoveRedundantGuard   RepairKind = "remove_redundant_guard"
	RepairHoistInvariantRead     RepairKind = "hoist_invariant_read"
	RepairInitializeDiscriminant RepairKind = "initialize_discriminant"
	RepairConstructFixedShape    RepairKind = "construct_fixed_shape"
	RepairAddNilGuard            RepairKind = "add_nil_guard"
	RepairAddAnnotation          RepairKind = "add_annotation"
)

// RepairDescriptor is declarative repair metadata attached to a judgment
// code. The query service owns target selection and structured payload
// projection; renderers and clients remain free to choose titles and edits.
type RepairDescriptor struct {
	Kind RepairKind
}

// CodeSpec is the single registration record for a semantic judgment code.
// Renderers and policy layers may reference these specs, but producers should
// not invent code metadata locally.
type CodeSpec struct {
	Code              Code
	Family            CodeFamily
	SubjectKind       SubjectKind
	RequiredEvidence  []EvidenceKind
	DefaultVerdict    Verdict
	Policy            PolicyProfile
	DiagnosticCodes   []DiagnosticCode
	DiagnosticDefault DiagnosticDefault
	Render            RenderKey
	Repairs           []RepairDescriptor
}

// Registry is an immutable table of known judgment codes.
type Registry struct {
	specs map[Code]CodeSpec
}

var defaultRegistry = NewRegistry([]CodeSpec{
	codeSpecWithRepairs(CodeCallArgType, FamilyCall, SubjectCallArgument, VerdictUnknown, PolicyStrictnessTunableTypeError, DiagnosticDefaultEnabled, RenderDirectCallArgument, diag(DiagnosticCodeDirectCallArgType), repairs(RepairAddAnnotation), EvidenceAbstractFact, EvidenceUserAssertion, EvidenceMissingProof),
	codeSpec(CodeCallArity, FamilyCall, SubjectCallExpression, VerdictRefuted, PolicyStrictnessTunableTypeError, DiagnosticDefaultEnabled, RenderCallArity, diag(DiagnosticCodeDirectCallTooFewArgs, DiagnosticCodeDirectCallTooManyArgs), EvidenceAbstractFact, EvidenceUserAssertion, EvidenceMissingProof),
	codeSpecWithRepairs(CodeCallCallee, FamilyCall, SubjectCallExpression, VerdictUnknown, PolicyStrictnessTunableTypeError, DiagnosticDefaultEnabled, RenderCallCallee, diag(DiagnosticCodeDirectCallNotCallable, DiagnosticCodeOptionalMethodCall, DiagnosticCodeMissingMember, DiagnosticCodeNotCallable), repairs(RepairAddNilGuard), EvidenceAbstractFact, EvidenceUserAssertion, EvidenceMissingProof),
	codeSpecWithRepairs(CodeAssignment, FamilyAssignment, SubjectPath, VerdictUnknown, PolicyStrictnessTunableTypeError, DiagnosticDefaultEnabled, RenderAssignment, diag(DiagnosticCodeAssignmentType, DiagnosticCodeDirectCallResultAssignment), repairs(RepairAddAnnotation), EvidenceAbstractFact, EvidenceUserAssertion, EvidenceMissingProof),
	codeSpecWithRepairs(CodeAssignmentTarget, FamilyAssignment, SubjectPath, VerdictRefuted, PolicyStrictnessTunableTypeError, DiagnosticDefaultEnabled, RenderOptionalAssignmentTarget, diag(DiagnosticCodeOptionalAssignmentTarget), repairs(RepairAddNilGuard), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpecWithRepairs(CodeReturn, FamilyReturn, SubjectReturnValue, VerdictUnknown, PolicyStrictnessTunableTypeError, DiagnosticDefaultEnabled, RenderReturn, diag(DiagnosticCodeReturnContractType), repairs(RepairAddAnnotation), EvidenceAbstractFact, EvidenceUserAssertion, EvidenceMissingProof),
	codeSpec(CodeNonNilAssertion, FamilyAssertion, SubjectExpression, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderNonNilAssertion, diag(DiagnosticCodeNonNilAssertAlwaysNil), EvidenceAbstractFact),
	codeSpec(CodeNumericForOperand, FamilyFor, SubjectExpression, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderNumericFor, diag(DiagnosticCodeNumericForOperand), EvidenceAbstractFact),
	codeSpec(CodeFrozenTable, FamilyEffect, SubjectPath, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderFrozenTable, diag(DiagnosticCodeFrozenTableMutation), EvidenceAbstractFact),
	codeSpec(CodeLifecycle, FamilyEffect, SubjectPath, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultEnabled, RenderLifecycle, diag(DiagnosticCodeResourceUnreleased), EvidenceMissingProof),
	codeSpec(CodeTypestateInvalidTransition, FamilyEffect, SubjectExpression, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderTypestateInvalid, diag(DiagnosticCodeTypestateInvalidTransition), EvidenceAbstractFact),
	codeSpec(CodeTypestateInvalidRequirement, FamilyEffect, SubjectExpression, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderTypestateRequirement, diag(DiagnosticCodeTypestateInvalidRequirement), EvidenceAbstractFact),
	codeSpec(CodeTypestateUnprovenRequirement, FamilyEffect, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultEnabled, RenderTypestateRequirement, diag(DiagnosticCodeTypestateUnprovenRequirement), EvidenceMissingProof),
	codeSpec(CodeUnusedLocal, FamilyLint, SubjectPath, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderUnusedLocal, diag(DiagnosticCodeUnusedLocal), EvidenceAbstractFact),
	codeSpec(CodeDeadAssignment, FamilyLint, SubjectPath, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderDeadAssignment, diag(DiagnosticCodeDeadAssignment), EvidenceAbstractFact),
	codeSpec(CodeChannelSelect, FamilyChannel, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultEnabled, RenderChannelSelect, diag(DiagnosticCodeChannelSelectExhaustive), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpec(CodeChannelSendClosed, FamilyChannel, SubjectExpression, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderChannelLifecycle, diag(DiagnosticCodeChannelSendClosed), EvidenceAbstractFact),
	codeSpec(CodeChannelDoubleClose, FamilyChannel, SubjectExpression, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderChannelLifecycle, diag(DiagnosticCodeChannelDoubleClose), EvidenceAbstractFact),
	codeSpec(CodeDiscriminatedUnion, FamilyUnion, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderDiscriminatedUnion, diag(DiagnosticCodeDiscriminatedUnionExhaustive), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpec(CodeOptional, FamilyUnion, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderOptional, diag(DiagnosticCodeDiscriminatedUnionExhaustive), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpec(CodeResultShape, FamilyUnion, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderResultShape, diag(DiagnosticCodeDiscriminatedUnionExhaustive), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpec(CodeRegistration, FamilyUnion, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderRegistration, diag(DiagnosticCodeDiscriminatedUnionExhaustive), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpec(CodeTableDispatch, FamilyUnion, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderTableDispatch, diag(DiagnosticCodeDiscriminatedUnionExhaustive), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpec(CodeUnresolvedValue, FamilyValue, SubjectPath, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderUnresolvedValue, diag(DiagnosticCodeUnresolvedValueReference), EvidenceAbstractFact),
	codeSpec(CodeUnresolvedType, FamilyType, SubjectPath, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderUnresolvedType, diag(DiagnosticCodeUnresolvedTypeReference), EvidenceAbstractFact),
	codeSpec(CodeRedundantCondition, FamilyCondition, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultOptIn, RenderRedundantCondition, diag(DiagnosticCodeRedundantCondition), EvidenceAbstractFact),
	codeSpecWithRepairs(CodeMemberRead, FamilyMember, SubjectExpression, VerdictRefuted, PolicyRefutedError, DiagnosticDefaultEnabled, RenderMemberRead, diag(DiagnosticCodeMissingMember), repairs(RepairAddNilGuard), EvidenceAbstractFact, EvidenceMissingProof),
	codeSpec(CodeConcatOperand, FamilyOperator, SubjectExpression, VerdictRefuted, PolicyRefutedWarning, DiagnosticDefaultEnabled, RenderConcatOperand, diag(DiagnosticCodeConcatOperand), EvidenceAbstractFact),
	codeSpec(CodeSendIsolation, FamilySend, SubjectCallArgument, VerdictUnknown, PolicyHint, DiagnosticDefaultOptIn, RenderSendIsolation, diag(DiagnosticCodeSendIsolation), EvidenceAbstractFact),
	codeSpecWithRepairs(CodeAdviceRedundantClaim, FamilyAdvice, SubjectExpression, VerdictProven, PolicyProvenHint, DiagnosticDefaultOptIn, RenderAdvice, diag(DiagnosticCodeAdviceRedundantClaim), repairs(RepairRemoveRedundantClaim), EvidenceAbstractFact),
	codeSpecWithRepairs(CodeAdviceAlwaysTrueGuard, FamilyAdvice, SubjectExpression, VerdictProven, PolicyProvenHint, DiagnosticDefaultOptIn, RenderAdvice, diag(DiagnosticCodeAdviceAlwaysTrueGuard), repairs(RepairRemoveRedundantGuard), EvidenceAbstractFact),
	codeSpecWithRepairs(CodeAdviceInvariantLoopRead, FamilyAdvice, SubjectExpression, VerdictProven, PolicyProvenHint, DiagnosticDefaultOptIn, RenderAdvice, diag(DiagnosticCodeAdviceInvariantLoopRead), repairs(RepairHoistInvariantRead), EvidenceAbstractFact),
	codeSpecWithRepairs(CodeAdviceSplitBirthDiscriminant, FamilyAdvice, SubjectExpression, VerdictProven, PolicyProvenHint, DiagnosticDefaultOptIn, RenderAdvice, diag(DiagnosticCodeAdviceSplitBirthDiscriminant), repairs(RepairInitializeDiscriminant), EvidenceAbstractFact),
	codeSpecWithRepairs(CodeAdviceShapePolymorphic, FamilyAdvice, SubjectExpression, VerdictProven, PolicyProvenHint, DiagnosticDefaultOptIn, RenderAdvice, diag(DiagnosticCodeAdviceShapePolymorphic), repairs(RepairConstructFixedShape), EvidenceAbstractFact),
})

func diag(codes ...DiagnosticCode) []DiagnosticCode {
	return codes
}

func repairs(kinds ...RepairKind) []RepairDescriptor {
	out := make([]RepairDescriptor, len(kinds))
	for i, kind := range kinds {
		out[i] = RepairDescriptor{Kind: kind}
	}
	return out
}

func codeSpec(
	code Code,
	family CodeFamily,
	subject SubjectKind,
	defaultVerdict Verdict,
	policy PolicyProfile,
	diagnosticDefault DiagnosticDefault,
	render RenderKey,
	diagnosticCodes []DiagnosticCode,
	requiredEvidence ...EvidenceKind,
) CodeSpec {
	return codeSpecWithRepairs(code, family, subject, defaultVerdict, policy, diagnosticDefault, render, diagnosticCodes, nil, requiredEvidence...)
}

func codeSpecWithRepairs(
	code Code,
	family CodeFamily,
	subject SubjectKind,
	defaultVerdict Verdict,
	policy PolicyProfile,
	diagnosticDefault DiagnosticDefault,
	render RenderKey,
	diagnosticCodes []DiagnosticCode,
	repairDescriptors []RepairDescriptor,
	requiredEvidence ...EvidenceKind,
) CodeSpec {
	return CodeSpec{
		Code:              code,
		Family:            family,
		SubjectKind:       subject,
		RequiredEvidence:  requiredEvidence,
		DefaultVerdict:    defaultVerdict,
		Policy:            policy,
		DiagnosticCodes:   diagnosticCodes,
		DiagnosticDefault: diagnosticDefault,
		Render:            render,
		Repairs:           repairDescriptors,
	}
}

// DefaultRegistry returns the standard judgment-code registry.
func DefaultRegistry() Registry {
	return defaultRegistry
}

// NewRegistry builds a registry from specs. Code ownership is explicit and
// one-spec-per-code; empty or duplicate codes are construction errors.
func NewRegistry(specs []CodeSpec) Registry {
	out := make(map[Code]CodeSpec, len(specs))
	for _, spec := range specs {
		if spec.Code == "" {
			panic("judgment: empty code spec")
		}
		if _, exists := out[spec.Code]; exists {
			panic("judgment: duplicate code spec for " + string(spec.Code))
		}
		out[spec.Code] = cloneCodeSpec(spec)
	}
	return Registry{specs: out}
}

// Lookup returns the registered spec for code.
func (r Registry) Lookup(code Code) (CodeSpec, bool) {
	spec, ok := r.specs[code]
	if !ok {
		return CodeSpec{}, false
	}
	return cloneCodeSpec(spec), true
}

// Codes returns every registered code in deterministic order.
func (r Registry) Codes() []Code {
	out := make([]Code, 0, len(r.specs))
	for code := range r.specs {
		out = append(out, code)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

// Validate reports whether a judgment matches its code registration.
func (r Registry) Validate(j Judgment) bool {
	spec, ok := r.Lookup(j.Code)
	if !ok {
		return false
	}
	if spec.SubjectKind != SubjectUnknown && j.Subject.Kind != spec.SubjectKind {
		return false
	}
	for _, required := range spec.RequiredEvidence {
		if !j.Evidence.Has(required) {
			return false
		}
	}
	return true
}

func cloneCodeSpec(spec CodeSpec) CodeSpec {
	spec.RequiredEvidence = append([]EvidenceKind(nil), spec.RequiredEvidence...)
	spec.DiagnosticCodes = append([]DiagnosticCode(nil), spec.DiagnosticCodes...)
	spec.Repairs = append([]RepairDescriptor(nil), spec.Repairs...)
	return spec
}
