package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ProofContext owns diagnostic-facing proof interpretation. Renderers ask it
// for a presentation view instead of reclassifying judgment evidence locally.
type ProofContext struct{}

type assignmentProofPresentation struct {
	Evidence                []diagnostic.Evidence
	DynamicTarget           bool
	MissingProof            bool
	MissingNilProof         bool
	NonNilArmMismatch       bool
	PrecisionBoundary       bool
	IndexedReadMissingProof bool
}

type assignmentStructurePresentation struct {
	Has            bool
	MissingField   bool
	MissingMethod  bool
	MethodMismatch bool
	SourceEvidence string
	Message        string
	Help           string
	Evidence       []diagnostic.Evidence
	SourceLabel    string
}

type assignmentDiagnosticPresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

type assignmentCallResultPresentation struct {
	Message  string
	Help     string
	Evidence []diagnostic.Evidence
	Labels   []diagnostic.Label
}

type optionalAssignmentTargetPresentation struct {
	Message     string
	Help        string
	Explanation diagnostic.Explanation
	Labels      []diagnostic.Label
}

func NewProofContext() ProofContext {
	return ProofContext{}
}

func (ctx ProofContext) AssignmentDiagnostic(item judgment.Judgment, target string, sourceName string, got, want typ.Type, sourceSpan diagnostic.Span, expectedDisplay string) assignmentDiagnosticPresentation {
	underSuppliedDetail, underSupplied := item.AssignmentUnderSuppliedCallResultDetail()
	declSpan := diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceUserAssertion)
	evidenceSourceName := sourceName
	if evidenceSourceName == "" {
		evidenceSourceName = "assigned value"
	}
	if underSupplied {
		sourceName = target
		evidenceSourceName = target
	}

	proofView := ctx.AssignmentProof(item, evidenceSourceName, got, want, sourceSpan)
	structureView := ctx.AssignmentStructure(item, target, got, want, sourceSpan)
	extraEvidence := proofView.Evidence
	if underSupplied {
		source := item.Actual.Label
		if source == "" {
			source = underSuppliedDetail.FunctionName
		}
		underSuppliedEvidence := diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    sourceSpan,
			Message: underSuppliedTargetEvidence(target, source, underSuppliedDetail.ResultIndex),
		}
		extraEvidence = append([]diagnostic.Evidence{underSuppliedEvidence}, extraEvidence...)
	}

	sourceEvidence := assignmentSourceTypeEvidence(evidenceSourceName, got)
	sourceEvidence = proofView.SourceEvidence(evidenceSourceName, got, sourceEvidence)
	if structureView.Has {
		sourceEvidence = structureView.SourceEvidence
	}

	message := proofView.AssignmentMessage(sourceName, got, want, expectedDisplay)
	if assignmentJudgmentTargetLooksMember(target, sourceName) && !proofView.DynamicTarget {
		message = proofView.MemberAssignmentMessage(target, sourceName, got, want, expectedDisplay)
	}
	help := proofView.Help(sourceName, got, want)
	if structureView.Message != "" {
		message = structureView.Message
	}
	if structureView.Help != "" {
		help = structureView.Help
	}
	if underSupplied {
		help = underSuppliedTargetHelp(target)
	}

	sourceLabelText := assignmentJudgmentSourceLabel(structureView.MissingField)
	if structureView.SourceLabel != "" {
		sourceLabelText = structureView.SourceLabel
	}
	if underSupplied {
		sourceLabelText = labelCallResult
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceAbstractFact, diagnostic.TrustProven),
			Span:    diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceAbstractFact),
			Message: sourceEvidence,
		},
		{
			Kind:    assignmentJudgmentExpectedEvidenceKind(item),
			Trust:   assignmentJudgmentExpectedEvidenceTrust(item),
			Span:    declSpan,
			Message: assignmentJudgmentExpectedEvidence(item, target, want, expectedDisplay),
		},
	}
	evidence = append(evidence, extraEvidence...)
	evidence = append(evidence, structureView.Evidence...)
	labels := []diagnostic.Label{sourceLabel(sourceSpan, sourceLabelText)}
	if !diagnosticSpanEqual(declSpan, sourceSpan) {
		expectedLabel := labelDeclaredType
		if proofView.DynamicTarget {
			expectedLabel = labelAssignmentTarget
		}
		labels = append(labels, sourceLabel(declSpan, expectedLabel))
	}
	return assignmentDiagnosticPresentation{
		Message:  message,
		Help:     help,
		Evidence: evidence,
		Labels:   labels,
	}
}

func (ProofContext) AssignmentProof(item judgment.Judgment, sourceName string, got, want typ.Type, sourceSpan diagnostic.Span) assignmentProofPresentation {
	proof := item.AssignmentProof()
	evidence := assignmentProofEvidence(item, proof, sourceName, got, want, sourceSpan)
	return assignmentProofPresentation{
		Evidence:                evidence,
		DynamicTarget:           proof.DynamicTarget,
		MissingProof:            proof.MissingProof,
		MissingNilProof:         proof.MayBeNil,
		NonNilArmMismatch:       readmodel.NonNilProjectionProvesMismatch(got, want),
		PrecisionBoundary:       proof.PrecisionBoundary,
		IndexedReadMissingProof: proof.IndexedRead,
	}
}

func (ProofContext) AssignmentStructure(item judgment.Judgment, target string, got, want typ.Type, sourceSpan diagnostic.Span) assignmentStructurePresentation {
	if detail, ok := assignmentJudgmentMissingRequiredField(item); ok {
		path := target
		if path != "" && detail.Field != "" {
			path += "." + detail.Field
		}
		return assignmentStructurePresentation{
			Has:            true,
			MissingField:   true,
			SourceEvidence: objectLiteralShapeEvidence(got),
			Message:        missingRequiredFieldMessage(detail.Field),
			Help:           missingRequiredFieldHelp(detail.Field),
			SourceLabel:    labelObjectLiteral,
			Evidence: []diagnostic.Evidence{
				{
					Kind:    diagnostic.EvidenceAbstractFact,
					Trust:   diagnostic.TrustProven,
					Span:    sourceSpan,
					Message: missingRequiredFieldPathEvidence(path, detail.FieldType),
				},
			},
		}
	}
	if detail, ok := assignmentJudgmentMissingRequiredMethod(item); ok {
		return assignmentStructurePresentation{
			Has:            true,
			MissingMethod:  true,
			SourceEvidence: objectLiteralShapeEvidence(got),
			Message:        missingRequiredMethodMessage(want, detail.Field),
			Help:           missingRequiredMethodHelp(detail.Field),
			SourceLabel:    labelObjectLiteral,
			Evidence: []diagnostic.Evidence{
				{
					Kind:    diagnostic.EvidenceMissingProof,
					Trust:   diagnostic.TrustUnknown,
					Span:    sourceSpan,
					Message: missingRequiredMethodTypeEvidence(want, typ.Method{Name: detail.Field, Type: functionTypeOrNil(detail.FieldType)}),
				},
			},
		}
	}
	if detail, ok := assignmentJudgmentMethodTypeMismatch(item); ok {
		return assignmentStructurePresentation{
			Has:            true,
			MethodMismatch: true,
			SourceEvidence: objectLiteralShapeEvidence(got),
			Message:        methodTypeMismatchMessage(want, detail.Field, detail.ActualType, detail.FieldType),
			SourceLabel:    labelObjectLiteral,
			Evidence: []diagnostic.Evidence{
				{
					Kind:    diagnostic.EvidenceMissingProof,
					Trust:   diagnostic.TrustUnknown,
					Span:    sourceSpan,
					Message: methodTypeMismatchEvidence(want, detail.Field, detail.ActualType, detail.FieldType),
				},
			},
		}
	}
	return assignmentStructurePresentation{}
}

func (ctx ProofContext) AssignmentCallResult(item judgment.Judgment, detail judgment.EvidenceDetail, got, want typ.Type, callSpan diagnostic.Span, typeSpan diagnostic.Span) assignmentCallResultPresentation {
	label := callResultSubject(detail.ResultIndex)
	name := detail.FunctionName
	if name == "" {
		name = item.Actual.Label
	}
	if name == "" {
		name = "call"
	}
	target := item.Subject.Label
	if target == "" {
		target = "assignment target"
	}
	proofView := ctx.AssignmentProof(item, label, got, want, callSpan)
	if proofView.PrecisionBoundary {
		got = typ.Any
	}
	evidence := make([]diagnostic.Evidence, 0, 3+len(proofView.Evidence))
	if retSpan, ok := assignmentJudgmentCallResultReturnSpan(item); ok {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    retSpan,
			Message: callResultDeclaredReturnEvidence(name, label, got),
		})
	} else {
		evidence = append(evidence, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    callSpan,
			Message: fmt.Sprintf("%s returns %s", name, formatType(got)),
		})
	}
	evidence = append(evidence, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceUserAssertion,
		Trust:   diagnostic.TrustClaimed,
		Span:    typeSpan,
		Message: assignmentTargetTypeEvidence(target, want),
	})
	evidence = append(evidence, proofView.Evidence...)

	labels := []diagnostic.Label{
		sourceLabel(callSpan, labelCallResult),
		sourceLabel(typeSpan, labelDeclaredType),
	}
	if retSpan, ok := assignmentJudgmentCallResultReturnSpan(item); ok {
		labels = append(labels, sourceLabel(retSpan, labelDeclaredReturn))
	}
	return assignmentCallResultPresentation{
		Message:  fmt.Sprintf("%s is %s, not %s", label, formatType(got), formatType(want)),
		Help:     callResultAssignmentHelp(proofView.NeedsNilGuardHelp(got, want)),
		Evidence: evidence,
		Labels:   labels,
	}
}

func (ctx ProofContext) AssignmentCallResultForItem(item judgment.Judgment, got, want typ.Type, callSpan diagnostic.Span) (assignmentCallResultPresentation, bool) {
	detail, ok := item.AssignmentCallResultDetail()
	if !ok || detail.UnderSupplied || !assignmentJudgmentHasCallResultReturnSpan(item) {
		return assignmentCallResultPresentation{}, false
	}
	typeSpan := diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceUserAssertion)
	return ctx.AssignmentCallResult(item, detail, got, want, callSpan, typeSpan), true
}

func (ProofContext) OptionalAssignmentTarget(item judgment.Judgment, targetSpan diagnostic.Span) optionalAssignmentTargetPresentation {
	containerName := item.Actual.Label
	if containerName == "" {
		containerName = "value"
	}
	targetName := item.Subject.Label
	if targetName == "" {
		targetName = containerName
	}
	containerType := item.Actual.ProjectedType
	containerSpan := diagnosticEvidenceSpanOrPrimary(item, judgment.EvidenceAbstractFact)
	return optionalAssignmentTargetPresentation{
		Message: optionalAssignmentTargetMessage(containerName),
		Help:    optionalAssignmentTargetHelp(containerName),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    containerSpan,
				Message: optionalAssignmentTargetContainerEvidence(containerName, containerType),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    targetSpan,
				Message: optionalAssignmentTargetWriteEvidence(targetName),
			},
		),
		Labels: []diagnostic.Label{
			sourceLabel(containerSpan, labelPossiblyNilContainer),
			sourceLabel(targetSpan, labelAssignmentTarget),
		},
	}
}

func callResultSubject(index int) string {
	if index >= 0 {
		return fmt.Sprintf("call result %d", index+1)
	}
	return "call result"
}

func assignmentJudgmentCallResultReturnSpan(item judgment.Judgment) (diagnostic.Span, bool) {
	span, ok := item.AssignmentCallResultReturnSpan()
	if ok {
		return diagnosticSpanFromJudgment(span), true
	}
	return diagnostic.Span{}, false
}

func assignmentJudgmentHasCallResultReturnSpan(item judgment.Judgment) bool {
	_, ok := assignmentJudgmentCallResultReturnSpan(item)
	return ok
}

func diagnosticSpanEqual(a, b diagnostic.Span) bool {
	return a.StartLine == b.StartLine &&
		a.StartCol == b.StartCol &&
		a.EndLine == b.EndLine &&
		a.EndCol == b.EndCol
}

func assignmentJudgmentTargetLooksMember(target string, sourceName string) bool {
	if target == "" || target == sourceName {
		return false
	}
	return strings.Contains(target, ".") || strings.Contains(target, "[")
}

func assignmentJudgmentMissingRequiredField(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentMissingRequiredField()
}

func assignmentJudgmentMissingRequiredMethod(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentMissingRequiredMethod()
}

func assignmentJudgmentMethodTypeMismatch(item judgment.Judgment) (judgment.EvidenceDetail, bool) {
	return item.AssignmentMethodTypeMismatch()
}

func functionTypeOrNil(t typ.Type) *typ.Function {
	fn, _ := t.(*typ.Function)
	return fn
}

func assignmentJudgmentExpectedTypeLabel(item judgment.Judgment, target string, fallback typ.Type) string {
	if item.Expected.Label != "" && item.Expected.Label != target {
		return item.Expected.Label
	}
	return formatType(fallback)
}

func assignmentJudgmentExpectedEvidence(item judgment.Judgment, target string, fallback typ.Type, expectedDisplay string) string {
	if evidence, ok := item.FirstEvidenceDetail(judgment.EvidenceDetailDynamicAssignmentTarget); ok {
		label := evidence.Detail.SubjectLabel
		if label == "" {
			label = target
		}
		return fmt.Sprintf("assignment target %s requires %s", label, formatType(fallback))
	}
	if expectedDisplay == "" {
		expectedDisplay = assignmentJudgmentExpectedTypeLabel(item, target, fallback)
	}
	return fmt.Sprintf("%s is declared as %s", target, expectedDisplay)
}

func assignmentJudgmentExpectedEvidenceKind(item judgment.Judgment) diagnostic.EvidenceKind {
	if item.HasEvidenceDetail(judgment.EvidenceDetailDynamicAssignmentTarget) {
		return diagnostic.EvidenceAbstractFact
	}
	return diagnostic.EvidenceUserAssertion
}

func assignmentJudgmentExpectedEvidenceTrust(item judgment.Judgment) diagnostic.TrustKind {
	if item.HasEvidenceDetail(judgment.EvidenceDetailDynamicAssignmentTarget) {
		return diagnostic.TrustProven
	}
	return diagnostic.TrustClaimed
}

func assignmentJudgmentSourceLabel(missingField bool) string {
	if missingField {
		return labelObjectLiteral
	}
	return labelAssignedValue
}

func (p assignmentProofPresentation) AssignmentMessage(sourceName string, got, want typ.Type, wantDisplay string) string {
	if p.IndexedReadMissingProof && sourceName != "" && sourceName != unknownSourceName {
		return "cannot assign " + sourceName + " because it may be nil"
	}
	if p.NeedsNilGuardHelp(got, want) && sourceName != "" && sourceName != unknownSourceName {
		return "cannot assign " + sourceName + " because it may be nil"
	}
	if p.sameRenderedTypeNeedsValidationProof(got, want) {
		subject := boundaryEvidenceSubject(sourceName)
		return "cannot assign " + sourceName + " because " + subject + " comes from any/unknown; no proof shows it satisfies " + assignmentDeclaredTypePhrase(wantDisplay)
	}
	return assignmentMessageDisplay(sourceName, got, want, wantDisplay)
}

func (p assignmentProofPresentation) MemberAssignmentMessage(memberName string, sourceName string, got, want typ.Type, wantDisplay string) string {
	if p.NeedsNilGuardHelp(got, want) {
		if sourceName == "" || sourceName == unknownSourceName {
			return "cannot assign " + memberName + " because assigned value may be nil"
		}
		return "cannot assign " + sourceName + " to " + memberName + " because " + sourceName + " may be nil"
	}
	if p.sameRenderedTypeNeedsValidationProof(got, want) {
		subject := boundaryEvidenceSubject(sourceName)
		return "cannot assign " + sourceName + " to " + memberName + " because " + subject + " comes from any/unknown; no proof shows it satisfies " + assignmentDeclaredTypePhrase(wantDisplay)
	}
	return memberAssignmentMessageDisplay(memberName, sourceName, got, want, wantDisplay)
}

func assignmentDeclaredTypePhrase(wantDisplay string) string {
	if wantDisplay == "" {
		return "the declared type"
	}
	return "the declared type " + wantDisplay
}

func (p assignmentProofPresentation) Help(sourceName string, got, want typ.Type) string {
	return assignmentHelp(sourceName, p.NeedsNilGuardHelp(got, want))
}

func (p assignmentProofPresentation) NeedsNilGuardHelp(got, want typ.Type) bool {
	return !p.PrecisionBoundary &&
		p.MissingNilProof &&
		!p.NonNilArmMismatch &&
		(!typ.Nil.Equals(got) || p.IndexedReadMissingProof) &&
		p.MissingProof
}

func (p assignmentProofPresentation) SourceEvidence(sourceName string, got typ.Type, fallback string) string {
	if p.IndexedReadMissingProof && typ.Nil.Equals(got) && sourceName != "" && sourceName != unknownSourceName {
		return sourceName + " can be nil here"
	}
	return fallback
}

func (p assignmentProofPresentation) sameRenderedTypeNeedsValidationProof(got, want typ.Type) bool {
	if typ.TypeEquals(got, want) && p.PrecisionBoundary {
		return true
	}
	return p.PrecisionBoundary && typ.IsAny(got) && assignmentStructuredDeclaredType(want)
}

func assignmentStructuredDeclaredType(t typ.Type) bool {
	switch unwrap.Alias(unwrap.Annotations(t)).(type) {
	case *typ.Record, *typ.Array, *typ.Tuple, *typ.Map, *typ.ReadonlyMap, *typ.Interface:
		return true
	default:
		return false
	}
}

func assignmentProofEvidence(item judgment.Judgment, proof judgment.AssignmentProofSummary, sourceName string, got, want typ.Type, sourceSpan diagnostic.Span) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	out = append(out, assignmentJudgmentParentContextEvidence(item)...)
	out = append(out, assignmentJudgmentUserAssertionEvidence(item)...)
	if proof.PrecisionBoundary {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidencePrecisionBoundary,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidencePrecisionBoundary, diagnostic.TrustUnknown),
			Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
			Span:    sourceSpan,
			Message: fmt.Sprintf("%s comes from any/unknown", boundaryEvidenceSubject(sourceName)),
		})
	}
	if sourceName != "" && sourceName != unknownSourceName &&
		typ.Nil.Equals(got) &&
		proof.MayBeNil &&
		!proof.PrecisionBoundary &&
		!proof.IndexedRead {
		out = append(out, assignmentJudgmentNilableAccessEvidence(item)...)
		out = append(out, assignmentJudgmentSourceContributionEvidence(item)...)
		out = append(out, assignmentJudgmentCallInvalidationEvidence(item)...)
		if !evidenceHasKind(out, diagnostic.EvidenceMissingProof) {
			out = append(out, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   assignmentMissingProofTrust(item, proof),
				Reason:  assignmentJudgmentMissingProofReason(proof),
				Span:    sourceSpan,
				Message: assignmentJudgmentMissingProofMessage(item, proof, sourceName, got, want),
			})
		}
		return out
	}
	if sourceName != "" && sourceName != unknownSourceName && proof.MayBeNil && !proof.PrecisionBoundary {
		out = append(out, assignmentJudgmentSourceContributionEvidence(item)...)
		out = append(out, assignmentJudgmentNilableAccessEvidence(item)...)
		out = append(out, assignmentJudgmentCallInvalidationEvidence(item)...)
		if proof.IndexedRead {
			return appendMissingNilGuardEvidence(out, sourceName, sourceSpan, true)
		}
		return appendMissingNilGuardEvidence(out, sourceName, sourceSpan, false)
	}
	out = append(out, assignmentJudgmentNilableAccessEvidence(item)...)
	out = append(out, assignmentJudgmentSourceContributionEvidence(item)...)
	out = append(out, assignmentJudgmentCallInvalidationEvidence(item)...)
	if item.Verdict == judgment.VerdictUnknown || proof.PrecisionBoundary {
		if !evidenceHasKind(out, diagnostic.EvidenceMissingProof) {
			out = append(out, diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   assignmentMissingProofTrust(item, proof),
				Reason:  assignmentJudgmentMissingProofReason(proof),
				Span:    sourceSpan,
				Message: assignmentJudgmentMissingProofMessage(item, proof, sourceName, got, want),
			})
		}
	}
	if proof.DynamicTarget && !evidenceHasKind(out, diagnostic.EvidenceMissingProof) {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Reason:  diagnostic.EvidenceReasonBoundaryValidationMissing,
			Span:    sourceSpan,
			Message: assignmentJudgmentMissingProofMessage(item, proof, sourceName, got, want),
		})
	}
	return out
}

func evidenceHasKind(items []diagnostic.Evidence, kind diagnostic.EvidenceKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func appendMissingNilGuardEvidence(items []diagnostic.Evidence, sourceName string, sourceSpan diagnostic.Span, sourceIndexed ...bool) []diagnostic.Evidence {
	indexed := len(sourceIndexed) > 0 && sourceIndexed[0]
	if sourceName == "" ||
		sourceName == unknownSourceName ||
		evidenceHasKind(items, diagnostic.EvidenceMissingProof) {
		return items
	}
	reason := diagnostic.EvidenceReasonBoundaryValidationMissing
	message := display.MissingNonNilGuardHereMessage(sourceName)
	if indexed {
		reason = diagnostic.EvidenceReasonIndexReadValidationMissing
		message = display.IndexedReadExpectedProofMessage(sourceName, "declared type")
	}
	return append(items, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Reason:  reason,
		Span:    sourceSpan,
		Message: message,
	})
}

func assignmentJudgmentMissingProofMessage(item judgment.Judgment, proof judgment.AssignmentProofSummary, sourceName string, got typ.Type, want typ.Type) string {
	subject := boundaryEvidenceSubject(sourceName)
	if proof.IndexedRead {
		return display.IndexedReadExpectedProofMessage(subject, "declared type")
	}
	if proof.BoundaryProofMissing() {
		return missingBoundaryProofMessageForSubject(subject, want)
	}
	if sourceName == "assigned value" || typ.Nil.Equals(got) || item.Expected.Label == "" {
		return missingBoundaryProofMessageForSubject(subject, want)
	}
	return display.MissingExpectedProofMessage(subject, "declared type")
}

func assignmentJudgmentUserAssertionEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentUserAssertedAnyEvidence() {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceUserAssertion, diagnostic.TrustClaimed),
			Reason:  diagnostic.EvidenceReasonUserAssertedAny,
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: userAssertedAnyEvidence(),
		})
	}
	return out
}

func assignmentJudgmentNilableAccessEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentNilableAccessEvidence() {
		if evidence.Detail.SubjectLabel == "" || evidence.Detail.Field == "" {
			continue
		}
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: assignmentNilableAccessMessage(evidence.Detail.SubjectLabel, evidence.Detail.Field),
		})
	}
	return out
}

func assignmentJudgmentSourceContributionEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentSourceContributionEvidence() {
		out = append(out, diagnostic.Evidence{
			Kind:  diagnostic.EvidenceAbstractFact,
			Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
			Span:  diagnosticSpanFromJudgment(evidence.Span),
			Message: diagnosticDisplay{}.ReassignedCallResultFieldEvidence(
				evidence.Detail.ProviderLabel,
				evidence.Detail.SubjectLabel,
				evidence.Detail.FieldType,
			),
		})
	}
	return out
}

func assignmentJudgmentParentContextEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentParentActualEvidence() {
		label := evidence.Detail.SubjectLabel
		if label == "" {
			label = "assigned value"
		}
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: fmt.Sprintf("%s has type %s", label, formatType(evidence.Detail.FieldType)),
		})
	}
	for _, evidence := range item.AssignmentParentExpectedEvidence() {
		label := evidence.Detail.SubjectLabel
		if label == "" {
			label = "assignment target"
		}
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustClaimed),
			Span:    diagnosticSpanFromJudgment(evidence.Span),
			Message: fmt.Sprintf("%s is declared as %s", label, formatType(evidence.Detail.FieldType)),
		})
	}
	return out
}

func assignmentJudgmentCallInvalidationEvidence(item judgment.Judgment) []diagnostic.Evidence {
	var out []diagnostic.Evidence
	for _, evidence := range item.AssignmentCallInvalidationEvidence() {
		out = append(out, diagnostic.Evidence{
			Kind:  diagnostic.EvidenceAbstractFact,
			Trust: diagnosticTrustFromJudgmentTrust(evidence.Trust, diagnostic.TrustProven),
			Span:  diagnosticSpanFromJudgment(evidence.Span),
			Message: assignmentCallInvalidationMessage(
				evidence.Detail.ProviderLabel,
				evidence.Detail.Field,
				evidence.Detail.SubjectLabel,
			),
		})
	}
	return out
}

func assignmentMissingProofTrust(item judgment.Judgment, proof judgment.AssignmentProofSummary) diagnostic.TrustKind {
	if proof.CallInvalidated {
		return diagnostic.TrustUnknown
	}
	return diagnosticTrustFromJudgmentEvidence(item, judgment.EvidenceMissingProof, missingProofTrustFromJudgment(item.Verdict))
}

func assignmentCallInvalidationMessage(callLabel, invalidatedLabel, readLabel string) string {
	if callLabel == "" {
		callLabel = "call"
	}
	if invalidatedLabel == "" {
		invalidatedLabel = "the source"
	}
	if readLabel == "" {
		readLabel = "assigned value"
	}
	return fmt.Sprintf("%s may change %s, so the read of %s needs a fresh check", callLabel, invalidatedLabel, readLabel)
}

func assignmentNilableAccessMessage(label, access string) string {
	if len(access) > 0 && access[0] == '[' {
		return fmt.Sprintf("%s may be nil before indexing %s", label, access)
	}
	return fmt.Sprintf("%s may be nil before reading %s", label, access)
}

func assignmentJudgmentMissingProofReason(proof judgment.AssignmentProofSummary) diagnostic.EvidenceReason {
	switch proof.Reason() {
	case judgment.AssignmentProofReasonIndexedRead:
		return diagnostic.EvidenceReasonIndexReadValidationMissing
	case judgment.AssignmentProofReasonBoundaryValidation:
		return diagnostic.EvidenceReasonBoundaryValidationMissing
	default:
		return diagnostic.EvidenceReasonUnspecified
	}
}
