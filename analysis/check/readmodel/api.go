// Package readmodel defines the syntax-free read surface consumed by
// post-solve obligation producers.
package readmodel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Reader is the reduced obligation-query surface. Implementations project
// solved analysis state into these data records; producers must not reach past
// this interface into body, syntax, or engine state internals.
type Reader interface {
	ForEachCall(func(CallSite) bool) bool
	ForEachAssignment(func(Assignment) bool) bool
	ForEachOptionalAssignmentTarget(func(OptionalAssignmentTarget) bool) bool
}

// SourceSpan is a syntax-free source range exported by the read model.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Assignment is the solved read model for one annotated assignment target.
// It carries the target contract and source value evidence without exposing
// syntax or engine internals to obligation producers.
type Assignment struct {
	Point              cfg.Point
	TargetLabel        string
	SourceLabel        string
	TargetKey          string
	Value              product.Value
	ValueHash          uint64
	TypeWithPresence   typ.Type
	Expected           typ.Type
	ExpectedLabel      string
	SourceSpan         SourceSpan
	DeclarationSpan    SourceSpan
	NilableAccesses    []NilableAccessEvidence
	UntrustedTopOrigin bool
	Check              AssignmentCheck
}

// NilableAccessEvidence records an intermediate source access whose receiver
// may be nil before a later assignment reads from it.
type NilableAccessEvidence struct {
	Label  string
	Access string
	Span   SourceSpan
}

// AssignmentCheck is the solved proof result for an assignment source against
// its declared target type.
type AssignmentCheck struct {
	Assignment     *Assignment
	Expected       typ.Type
	Admissible     bool
	ProvenMismatch bool
	Mismatch       AssignmentMismatch
}

// AssignmentCheckPlan carries already-solved proof inputs for one assignment.
type AssignmentCheckPlan struct {
	Assignment               Assignment
	ValueAdmissible          bool
	ValueProvenMismatch      bool
	MissingRequiredField     string
	MissingRequiredFieldType typ.Type
	IsSubtype                func(typ.Type, typ.Type) bool
}

// AssignmentMismatchKind classifies a structural assignment mismatch reason
// discovered by the read model.
type AssignmentMismatchKind uint8

const (
	AssignmentMismatchNone AssignmentMismatchKind = iota
	AssignmentMismatchMissingRequiredField
)

// AssignmentMismatch carries structured mismatch detail without diagnostic
// wording.
type AssignmentMismatch struct {
	Kind  AssignmentMismatchKind
	Field string
	Type  typ.Type
}

// OptionalAssignmentTarget is the solved read model for a write through an
// optional container, e.g. `bag.name = ...` where `bag` may be nil.
type OptionalAssignmentTarget struct {
	Point          cfg.Point
	ContainerLabel string
	TargetLabel    string
	TargetKey      string
	ContainerType  typ.Type
	ContainerSpan  SourceSpan
	TargetSpan     SourceSpan
}

// PlanAssignmentCheck returns the complete proof result for one annotated
// assignment source against the declared target type.
func PlanAssignmentCheck(plan AssignmentCheckPlan) AssignmentCheck {
	assignment := plan.Assignment
	var mismatch AssignmentMismatch
	if plan.MissingRequiredField != "" {
		mismatch = AssignmentMismatch{
			Kind:  AssignmentMismatchMissingRequiredField,
			Field: plan.MissingRequiredField,
			Type:  plan.MissingRequiredFieldType,
		}
	}
	return AssignmentCheck{
		Assignment:     &assignment,
		Expected:       assignment.Expected,
		Admissible:     plan.assignmentProofAdmissible(),
		ProvenMismatch: plan.ValueProvenMismatch,
		Mismatch:       mismatch,
	}
}

func (plan AssignmentCheckPlan) assignmentProofAdmissible() bool {
	if plan.Assignment.UntrustedTopOrigin && typ.TypeEquals(plan.Assignment.TypeWithPresence, nil) {
		return false
	}
	if plan.ValueAdmissible {
		return true
	}
	if !typ.TypeEquals(plan.Assignment.TypeWithPresence, nil) &&
		!typ.TypeEquals(plan.Assignment.Expected, nil) &&
		plan.IsSubtype != nil &&
		!typ.IsAny(plan.Assignment.TypeWithPresence) &&
		!typ.IsUnknown(plan.Assignment.TypeWithPresence) &&
		!typ.IsNever(plan.Assignment.TypeWithPresence) &&
		!plan.IsSubtype(plan.Assignment.TypeWithPresence, plan.Assignment.Expected) {
		return false
	}
	if plan.Assignment.UntrustedTopOrigin || typ.TypeEquals(plan.Assignment.TypeWithPresence, nil) || typ.TypeEquals(plan.Assignment.Expected, nil) || plan.IsSubtype == nil {
		return false
	}
	if typ.IsAny(plan.Assignment.TypeWithPresence) || typ.IsUnknown(plan.Assignment.TypeWithPresence) || typ.IsNever(plan.Assignment.TypeWithPresence) {
		return false
	}
	return plan.IsSubtype(plan.Assignment.TypeWithPresence, plan.Assignment.Expected)
}

// CallSite is the solved read model for one call expression. It is the public
// obligation input: producers should consume this assembled record instead of
// rebuilding a call from independent reader queries.
type CallSite struct {
	Point      cfg.Point
	CallSpan   SourceSpan
	CalleeSpan SourceSpan
	Reports    []CallArgumentReport
	Arity      CallArityReport
	Callee     CallCalleeReport
}

// CallArityReportKind classifies a solved call-arity mismatch.
type CallArityReportKind uint8

const (
	CallArityReportNone CallArityReportKind = iota
	CallArityReportTooFew
	CallArityReportTooMany
)

// CallArityReport is the solved read model for a call argument-count
// obligation. It carries counts and source anchors only; renderers own wording
// and severity.
type CallArityReport struct {
	Kind            CallArityReportKind
	CallableName    string
	ExpectedCount   int
	ActualCount     int
	CallSpan        SourceSpan
	DeclarationSpan SourceSpan
	ExtraSpan       SourceSpan
}

// CallArityReportPlan carries the syntax-free inputs needed to classify a call
// arity report. Internal readmodels own extracting counts/spans; public
// readmodel owns the reporting decision.
type CallArityReportPlan struct {
	HasContract    bool
	CallableName   string
	ActualCount    int
	RequiredCount  int
	FixedCount     int
	HasVararg      bool
	CallSpan       SourceSpan
	ParameterSpans []SourceSpan
	ArgumentSpans  []SourceSpan
}

// PlanCallArityReport returns the call arity report for plan, or the zero
// report when the call satisfies the callable contract.
func PlanCallArityReport(plan CallArityReportPlan) CallArityReport {
	if !plan.HasContract {
		return CallArityReport{}
	}
	if plan.ActualCount < plan.RequiredCount {
		return CallArityReport{
			Kind:            CallArityReportTooFew,
			CallableName:    plan.CallableName,
			ExpectedCount:   plan.RequiredCount,
			ActualCount:     plan.ActualCount,
			CallSpan:        plan.CallSpan,
			DeclarationSpan: sourceSpanAt(plan.ParameterSpans, plan.ActualCount),
		}
	}
	if !plan.HasVararg && plan.ActualCount > plan.FixedCount {
		return CallArityReport{
			Kind:            CallArityReportTooMany,
			CallableName:    plan.CallableName,
			ExpectedCount:   plan.FixedCount,
			ActualCount:     plan.ActualCount,
			CallSpan:        plan.CallSpan,
			DeclarationSpan: sourceSpanAt(plan.ParameterSpans, plan.FixedCount-1),
			ExtraSpan:       sourceSpanAt(plan.ArgumentSpans, plan.FixedCount),
		}
	}
	return CallArityReport{}
}

func sourceSpanAt(spans []SourceSpan, index int) SourceSpan {
	if index < 0 || index >= len(spans) {
		return SourceSpan{}
	}
	return spans[index]
}

// CallCalleeReportKind classifies a solved direct-callee callable mismatch.
type CallCalleeReportKind uint8

const (
	CallCalleeReportNone CallCalleeReportKind = iota
	CallCalleeReportNotCallable
	CallCalleeReportMayBeNil
)

// CallCalleeReport is the solved read model for a direct-call callee
// obligation. Member-call failures are owned by member-call diagnostics; this
// report covers direct callees whose solved value is not callable or may be nil.
type CallCalleeReport struct {
	Kind         CallCalleeReportKind
	CallableName string
	Type         typ.Type
	Callable     bool
	Span         SourceSpan
}

// CallCalleeReportPlan carries the solved direct-callee information needed to
// classify callable failures. Internal readmodels own resolving the callee
// value; public readmodel owns deciding if it should report.
type CallCalleeReportPlan struct {
	CallableName string
	Type         typ.Type
	Callable     bool
	Span         SourceSpan
	CallSpan     SourceSpan
}

// PlanCallCalleeReport returns the direct-callee report for plan, or zero when
// the callee is definitely callable or too imprecise to report.
func PlanCallCalleeReport(plan CallCalleeReportPlan) CallCalleeReport {
	if plan.Type == nil || typ.IsAny(plan.Type) || typ.IsUnknown(plan.Type) || typ.IsNever(plan.Type) {
		return CallCalleeReport{}
	}
	name := plan.CallableName
	if name == "" {
		name = "call target"
	}
	span := sourceSpanOr(plan.Span, plan.CallSpan)
	if typevalue.TypeIncludesNil(plan.Type) {
		return CallCalleeReport{
			Kind:         CallCalleeReportMayBeNil,
			CallableName: name,
			Type:         plan.Type,
			Callable:     plan.Callable,
			Span:         span,
		}
	}
	if !plan.Callable {
		return CallCalleeReport{
			Kind:         CallCalleeReportNotCallable,
			CallableName: name,
			Type:         plan.Type,
			Span:         span,
		}
	}
	return CallCalleeReport{}
}

func sourceSpanOr(primary, fallback SourceSpan) SourceSpan {
	if primary.StartLine != 0 {
		return primary
	}
	return fallback
}

// CallCalleeDeclaredTypeMoreInformative reports whether a declared callee type
// should replace the solved boundary value type for callee reporting. This
// preserves the callable half of an optional declared callee when the solved
// value at the call site is nil.
func CallCalleeDeclaredTypeMoreInformative(valueType, declared typ.Type) bool {
	if declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return false
	}
	return typ.TypeEquals(valueType, typ.Nil) && typevalue.TypeIncludesNil(declared)
}

// CallCalleeDeclaredNilOwnedByDeclaration reports whether a nil solved callee
// value is already owned by the root local's non-nil callable declaration. The
// write/declaration contract should produce the user-facing diagnostic; the
// later call through that local would only be a cascade.
func CallCalleeDeclaredNilOwnedByDeclaration(valueType, declared typ.Type) bool {
	if declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return false
	}
	if !typ.TypeEquals(valueType, typ.Nil) || typevalue.TypeIncludesNil(declared) {
		return false
	}
	_, ok := typecall.Callable(declared)
	return ok
}

// CallArgument is the solved read model for one call argument.
type CallArgument struct {
	Index                int
	Value                product.Value
	ValueHash            uint64
	TypeWithPresence     typ.Type
	UntrustedTopOrigin   bool
	ProofCandidateValue  product.Value
	ProofCandidateHash   uint64
	ProofCandidateType   typ.Type
	ProofCandidateTop    bool
	HasProofCandidate    bool
	CallerOwnedParameter bool
	FunctionType         *typ.Function
	Span                 SourceSpan
	Label                string
	Mismatch             CallArgumentMismatch
}

// CallArgumentMismatchKind classifies a structural argument mismatch reason
// discovered by the read model.
type CallArgumentMismatchKind uint8

const (
	CallArgumentMismatchNone CallArgumentMismatchKind = iota
	CallArgumentMismatchMissingRequiredField
	CallArgumentMismatchMayBeNil
)

// CallArgumentMismatch carries structured mismatch detail without diagnostic
// wording.
type CallArgumentMismatch struct {
	Kind  CallArgumentMismatchKind
	Field string
}

// CallArgumentMismatchCandidate is one nested argument member that may become
// the report subject for an argument mismatch.
type CallArgumentMismatchCandidate struct {
	Argument    CallArgument
	Expected    typ.Type
	LabelSuffix string
	Admissible  bool
}

// CallArgumentMismatchSubjectPlan carries pre-projected object-literal
// mismatch candidates. Internal readmodels own extracting member values and
// expected member types; public readmodel owns which candidate becomes the
// user-facing report subject.
type CallArgumentMismatchSubjectPlan struct {
	Argument             CallArgument
	Expected             typ.Type
	Candidates           []CallArgumentMismatchCandidate
	MissingRequiredField string
}

// CallArgumentMismatchSubject is the selected report subject for one argument
// mismatch.
type CallArgumentMismatchSubject struct {
	Argument    CallArgument
	Expected    typ.Type
	LabelSuffix string
}

// PlanCallArgumentMismatchSubject selects the best user-facing subject for a
// call-argument mismatch. The first failing nested member wins; when all
// present members are admissible, a missing required field becomes the subject.
func PlanCallArgumentMismatchSubject(plan CallArgumentMismatchSubjectPlan) (CallArgumentMismatchSubject, bool) {
	for _, candidate := range plan.Candidates {
		if candidate.Expected == nil || candidate.Admissible {
			continue
		}
		return CallArgumentMismatchSubject{
			Argument:    candidate.Argument,
			Expected:    candidate.Expected,
			LabelSuffix: candidate.LabelSuffix,
		}, true
	}
	if plan.MissingRequiredField != "" {
		arg := plan.Argument
		arg.Mismatch = CallArgumentMismatch{
			Kind:  CallArgumentMismatchMissingRequiredField,
			Field: plan.MissingRequiredField,
		}
		return CallArgumentMismatchSubject{
			Argument:    arg,
			Expected:    plan.Expected,
			LabelSuffix: CallArgumentExpectedLabelSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: plan.MissingRequiredField}}),
		}, true
	}
	return CallArgumentMismatchSubject{}, false
}

// CallArgumentMayBeNilMismatch reports whether an argument may be nil while
// the expected type rejects nil.
func CallArgumentMayBeNilMismatch(got, want typ.Type) bool {
	return got != nil && want != nil && typevalue.TypeIncludesNil(got) && !typevalue.TypeIncludesNil(want)
}

// ObligationTypeReportable reports whether an expected type is precise enough
// to emit as a user-facing obligation. Gradual, unknown, and still-generic
// obligations are internal evidence, not standalone reports.
func ObligationTypeReportable(t typ.Type) bool {
	base := unwrap.Optional(t)
	return t != nil &&
		base != nil &&
		!typ.IsAny(base) &&
		!typ.IsUnknown(base) &&
		!refinement.ContainsFreeTypeParam(t)
}

// CallArgumentObligationTypeReportable is a compatibility alias for direct-call
// migration code that has not yet folded onto the generic obligation query.
func CallArgumentObligationTypeReportable(t typ.Type) bool {
	return ObligationTypeReportable(t)
}

// CallArgumentProofPlan carries the already-solved proof inputs needed to
// combine value-domain proof and contextual function-type proof for one
// argument. Internal readmodels compute raw proof facts; public readmodel owns
// how those facts become report-facing admissibility and mismatch verdicts.
type CallArgumentProofPlan struct {
	Argument            CallArgument
	Expected            typ.Type
	ValueAdmissible     bool
	ValueProvenMismatch bool
	IsSubtype           func(typ.Type, typ.Type) bool
}

// CallArgumentProofAdmissible reports whether an argument is proven admissible
// against Expected. A contextual function argument may be admissible even when
// the raw value proof cannot see through the function literal boundary.
func CallArgumentProofAdmissible(plan CallArgumentProofPlan) bool {
	if plan.ValueAdmissible {
		return true
	}
	return plan.Argument.FunctionType != nil && plan.isSubtype(plan.Argument.FunctionType, plan.Expected)
}

// CallArgumentWitnessProvenMismatch reports whether an argument has a concrete
// contradiction against Expected. Gradual expected types do not produce proven
// mismatches, but a contextual function argument can prove a mismatch when its
// function type rejects the expected type.
func CallArgumentWitnessProvenMismatch(plan CallArgumentProofPlan) bool {
	if plan.ValueProvenMismatch {
		return true
	}
	if plan.Argument.FunctionType == nil || plan.Expected == nil || typ.IsAny(plan.Expected) || typ.IsUnknown(plan.Expected) {
		return false
	}
	return !plan.isSubtype(plan.Argument.FunctionType, plan.Expected)
}

func (plan CallArgumentProofPlan) isSubtype(sub, super typ.Type) bool {
	if plan.IsSubtype == nil || sub == nil || super == nil {
		return false
	}
	return plan.IsSubtype(sub, super)
}

// CallArgumentCheck is the solved proof result for one argument against one
// expected type. It keeps refinement, admissibility, and mismatch verdict input
// together so producers do not reassemble the proof protocol from fragments.
type CallArgumentCheck struct {
	Argument       CallArgument
	Expected       typ.Type
	ExpectedLabel  string
	ExpectedSpan   SourceSpan
	ExpectedOrigin CallArgumentObligationOrigin
	Admissible     bool
	ProvenMismatch bool
}

// CallArgumentCheckPlan carries the solved facts needed to assemble the
// report-facing proof result for one argument. Internal readmodels supply facts;
// public readmodel owns nested-subject selection, nil mismatch classification,
// expected-label adjustment, and final proof verdict assembly.
type CallArgumentCheckPlan struct {
	Argument            CallArgument
	Expected            typ.Type
	ExpectedLabel       string
	ExpectedSpan        SourceSpan
	ExpectedOrigin      CallArgumentObligationOrigin
	ValueAdmissible     bool
	ValueProvenMismatch bool
	IsSubtype           func(typ.Type, typ.Type) bool
	SubjectPlan         *CallArgumentMismatchSubjectPlan
}

// PlanCallArgumentCheck returns the complete solved proof result for one
// argument against one expected type.
func PlanCallArgumentCheck(plan CallArgumentCheckPlan) CallArgumentCheck {
	arg := plan.Argument
	want := plan.Expected
	labelSuffix := ""
	if plan.SubjectPlan != nil {
		if subject, ok := PlanCallArgumentMismatchSubject(*plan.SubjectPlan); ok {
			arg = subject.Argument
			want = subject.Expected
			labelSuffix = subject.LabelSuffix
		}
	}
	if arg.Mismatch.Kind == CallArgumentMismatchNone && CallArgumentMayBeNilMismatch(arg.TypeWithPresence, want) {
		arg.Mismatch = CallArgumentMismatch{Kind: CallArgumentMismatchMayBeNil}
	}
	return CallArgumentCheck{
		Argument:       arg,
		Expected:       want,
		ExpectedLabel:  ExpectedLabelWithSuffix(plan.ExpectedLabel, labelSuffix),
		ExpectedSpan:   plan.ExpectedSpan,
		ExpectedOrigin: plan.ExpectedOrigin,
		Admissible: CallArgumentProofAdmissible(CallArgumentProofPlan{
			Argument:        arg,
			Expected:        want,
			ValueAdmissible: plan.ValueAdmissible,
			IsSubtype:       plan.IsSubtype,
		}),
		ProvenMismatch: CallArgumentWitnessProvenMismatch(CallArgumentProofPlan{
			Argument:            arg,
			Expected:            want,
			ValueProvenMismatch: plan.ValueProvenMismatch,
			IsSubtype:           plan.IsSubtype,
		}),
	}
}

// CallArgumentReportKind classifies the rendering path for one planned
// direct-call argument report.
type CallArgumentReportKind uint8

const (
	CallArgumentReportObligation CallArgumentReportKind = iota
	CallArgumentReportGenericConflict
)

// CallArgumentReport is one ordered report candidate for a call argument. The
// read model owns report ordering and index reservation so producers do not
// reimplement direct-call precedence.
type CallArgumentReport struct {
	Kind       CallArgumentReportKind
	Argument   CallArgument
	Obligation CallArgumentObligation
	Check      CallArgumentCheck
	Conflict   CallGenericInferenceConflict
}

// IndexedCallArgumentObligation pairs a call argument index with the expected
// type and report metadata for that slot.
type IndexedCallArgumentObligation struct {
	Index      int
	Obligation CallArgumentObligation
}

// CallArgumentReportPlan carries the already-solved inputs needed to order
// direct-call argument reports. It is deliberately syntax-free: internal
// readmodels assemble values and contracts, while this planner owns precedence
// and index reservation.
type CallArgumentReportPlan struct {
	Args               []CallArgument
	GenericConflicts   []CallGenericInferenceConflict
	GenericConstraints []IndexedCallArgumentObligation
	ExplicitParams     []IndexedCallArgumentObligation
	OutcomeParams      []IndexedCallArgumentObligation
	Check              func(CallArgument, CallArgumentObligation) CallArgumentCheck
}

// PlanCallArgumentReports returns ordered direct-call argument report
// candidates. Precedence is generic inference conflict, generic constraint,
// explicit callable parameter, then solved call-outcome obligation. Once an
// argument index has a report or a proven-admissible check, later sources for
// that index are reserved and suppressed.
func PlanCallArgumentReports(plan CallArgumentReportPlan) []CallArgumentReport {
	var out []CallArgumentReport
	argsByIndex := callArgumentsByIndex(plan.Args)
	reported := make(map[int]struct{})

	for _, conflict := range plan.GenericConflicts {
		if len(conflict.Contributions) < 2 {
			continue
		}
		arg, ok := argsByIndex[conflict.Index]
		if !ok {
			arg = CallArgument{Index: conflict.Index, Span: conflict.Span}
		}
		out = append(out, CallArgumentReport{
			Kind:     CallArgumentReportGenericConflict,
			Argument: arg,
			Conflict: conflict,
		})
		reported[conflict.Index] = struct{}{}
	}

	for _, indexed := range plan.GenericConstraints {
		if _, seen := reported[indexed.Index]; seen || !CallArgumentObligationTypeReportable(indexed.Obligation.Type) {
			continue
		}
		arg, ok := argsByIndex[indexed.Index]
		if !ok {
			continue
		}
		check := plan.check(arg, indexed.Obligation)
		if check.Admissible {
			reported[indexed.Index] = struct{}{}
			continue
		}
		out = append(out, CallArgumentReport{
			Kind:       CallArgumentReportObligation,
			Argument:   arg,
			Obligation: indexed.Obligation,
			Check:      check,
		})
		reported[indexed.Index] = struct{}{}
	}

	out = plan.appendObligations(out, reported, argsByIndex, plan.ExplicitParams)
	out = plan.appendObligations(out, reported, argsByIndex, plan.OutcomeParams)
	return out
}

func (plan CallArgumentReportPlan) appendObligations(
	out []CallArgumentReport,
	reported map[int]struct{},
	argsByIndex map[int]CallArgument,
	obligations []IndexedCallArgumentObligation,
) []CallArgumentReport {
	for _, indexed := range obligations {
		if _, seen := reported[indexed.Index]; seen || !CallArgumentObligationTypeReportable(indexed.Obligation.Type) {
			continue
		}
		arg, ok := argsByIndex[indexed.Index]
		if !ok {
			continue
		}
		check := plan.check(arg, indexed.Obligation)
		if check.Admissible {
			reported[indexed.Index] = struct{}{}
			continue
		}
		out = append(out, CallArgumentReport{
			Kind:       CallArgumentReportObligation,
			Argument:   arg,
			Obligation: indexed.Obligation,
			Check:      check,
		})
		reported[indexed.Index] = struct{}{}
	}
	return out
}

func (plan CallArgumentReportPlan) check(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
	if plan.Check == nil {
		return CallArgumentCheck{
			Argument: arg,
			Expected: obligation.Type,
		}
	}
	return plan.Check(arg, obligation)
}

func callArgumentsByIndex(args []CallArgument) map[int]CallArgument {
	out := make(map[int]CallArgument, len(args))
	for _, arg := range args {
		out[arg.Index] = arg
	}
	return out
}

// CallContractSourceKind classifies where a callable contract came from. The
// read model owns this report-facing source vocabulary so obligation producers
// receive stable labels and spans without re-deriving call provenance.
type CallContractSourceKind uint8

const (
	CallContractSourceUnknown CallContractSourceKind = iota
	CallContractSourceLocalFunction
	CallContractSourceImportedSignature
	CallContractSourceFunctionValue
	CallContractSourceMemberFunction
)

// CallContractSource identifies the source of a callable contract for
// report-facing parameter labels and declaration spans.
type CallContractSource struct {
	Kind           CallContractSourceKind
	Name           string
	ParameterSpans []SourceSpan
}

// ParameterLabel returns the stable display label for parameter index.
func (s CallContractSource) ParameterLabel(index int) string {
	param := fmt.Sprintf("parameter %d", index+1)
	if s.Name == "" {
		return param
	}
	switch s.Kind {
	case CallContractSourceImportedSignature:
		return fmt.Sprintf("%s parameter %d", s.Name, index+1)
	case CallContractSourceLocalFunction, CallContractSourceFunctionValue, CallContractSourceMemberFunction:
		return fmt.Sprintf("%s parameter %d", s.Name, index+1)
	default:
		return param
	}
}

// ParameterSpan returns the declaration span for parameter index, when known.
func (s CallContractSource) ParameterSpan(index int) SourceSpan {
	if index < 0 || index >= len(s.ParameterSpans) {
		return SourceSpan{}
	}
	return s.ParameterSpans[index]
}

// CallArgumentObligation is one expected type for one call argument in an
// already-planned report.
type CallArgumentObligation struct {
	Type          typ.Type
	ExpectedLabel string
	ExpectedSpan  SourceSpan
	Origin        CallArgumentObligationOrigin
}

// CallArgumentObligationOrigin records why a projected call-site obligation
// exists. Direct signature checks leave HasOrigin false; summary-projected
// member-call obligations use this to render the chain from caller argument to
// the member parameter that required the type.
type CallArgumentObligationOrigin struct {
	HasOrigin         bool
	FunctionName      string
	SubjectLabel      string
	ProviderLabel     string
	MemberParamNumber int
}

// CallGenericInferenceConflict records an argument whose nested uses of one
// generic type parameter imply incompatible concrete types.
type CallGenericInferenceConflict struct {
	Index         int
	ParamName     string
	Span          SourceSpan
	Contributions []CallGenericInferenceContribution
}

// CallGenericInferenceContribution records one nested argument position that
// contributed a concrete type to generic inference.
type CallGenericInferenceContribution struct {
	Type typ.Type
	Span SourceSpan
}

// GenericInferenceContributionSpanCandidate is one possible source span for a
// generic inference contribution.
type GenericInferenceContributionSpanCandidate struct {
	Span         SourceSpan
	SegmentDepth int
	Matches      bool
}

// GenericInferenceContributionSpanPlan carries already-projected span
// candidates for one generic inference contribution. Internal readmodels own
// producing candidate matches; public readmodel owns choosing the anchor.
type GenericInferenceContributionSpanPlan struct {
	Fallback   SourceSpan
	Candidates []GenericInferenceContributionSpanCandidate
}

// PlanGenericInferenceContributionSpan chooses the most specific matching span
// for a generic inference contribution, falling back to the whole argument.
func PlanGenericInferenceContributionSpan(plan GenericInferenceContributionSpanPlan) SourceSpan {
	best := plan.Fallback
	bestDepth := -1
	for _, candidate := range plan.Candidates {
		if !candidate.Matches || candidate.SegmentDepth <= bestDepth {
			continue
		}
		best = candidate.Span
		bestDepth = candidate.SegmentDepth
	}
	return best
}

// CallArgumentMemberLabel returns the stable label for a nested argument member
// that became the report subject.
func CallArgumentMemberLabel(index int, segs []segment.Segment, valueLabel string) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("argument ")
	b.WriteString(strconv.Itoa(index + 1))
	for _, seg := range segs {
		if !appendSegmentLabel(&b, seg) {
			return ""
		}
	}
	if valueLabel != "" {
		b.WriteString(" (")
		b.WriteString(valueLabel)
		b.WriteByte(')')
	}
	return b.String()
}

// ExpectedLabelWithSuffix appends a nested member suffix to an existing
// expected-label owner. Empty inputs keep the original label unchanged.
func ExpectedLabelWithSuffix(label, suffix string) string {
	if label == "" || suffix == "" {
		return label
	}
	return label + suffix
}

// CallArgumentExpectedLabelSuffix returns the stable expected-label suffix for
// a nested argument member.
func CallArgumentExpectedLabelSuffix(segs []segment.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range segs {
		if !appendSegmentLabel(&b, seg) {
			return ""
		}
	}
	return b.String()
}

func appendSegmentLabel(b *strings.Builder, seg segment.Segment) bool {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if seg.Name == "" {
			return false
		}
		b.WriteByte('.')
		b.WriteString(seg.Name)
	case segment.SegmentIndexInt:
		b.WriteByte('[')
		b.WriteString(strconv.FormatInt(int64(seg.Index), 10))
		b.WriteByte(']')
	default:
		return false
	}
	return true
}
