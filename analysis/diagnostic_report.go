package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/keyspace"
)

// DiagnosticCode and DiagnosticRule are Analysis-owned semantic policy
// vocabularies. They intentionally do not reuse engine runtime diagnostics.
type DiagnosticCode uint8
type DiagnosticRule uint8

const (
	DiagnosticCodeInvalid DiagnosticCode = iota
	DiagnosticCodeAlwaysTrueGuard
	DiagnosticCodeAlwaysFalseGuard
	DiagnosticCodeRedundantClaim
	DiagnosticCodeUnresolvedTypeReference
	DiagnosticCodeUnresolvedValueReference
	DiagnosticCodeUnusedLocal
)
const (
	DiagnosticRuleInvalid DiagnosticRule = iota
	DiagnosticRuleAlwaysTrueGuard
	DiagnosticRuleAlwaysFalseGuard
	DiagnosticRuleRedundantClaim
	DiagnosticRuleUnresolvedTypeReference
	DiagnosticRuleUnresolvedValueReference
	DiagnosticRuleUnusedLocal
)

type FindingSeverity uint8

const (
	FindingSeverityInvalid FindingSeverity = iota
	FindingSeverityError
	FindingSeverityWarning
	FindingSeverityHint
)

// DiagnosticCollectionFailure is a closed receipt-collector classification;
// it never changes inference status or Result identity.
type DiagnosticCollectionFailure uint8

const (
	DiagnosticCollectionOK DiagnosticCollectionFailure = iota
	DiagnosticCollectionSubjectQueryAbsent
	DiagnosticCollectionQueryUnreadable
	DiagnosticCollectionQueryInvalid
	DiagnosticCollectionValueShapeMismatch
)

// diagnosticTemplateText is a closed formatting vocabulary. The only dynamic
// values it accepts come from typed row fields below; it never accepts caller
// supplied diagnostic prose.
type diagnosticTemplateText uint8

const (
	diagnosticTemplateTextInvalid diagnosticTemplateText = iota
	diagnosticTemplateTextAlwaysTrueMessage
	diagnosticTemplateTextAlwaysFalseMessage
	diagnosticTemplateTextRedundantClaimMessage
	diagnosticTemplateTextUnresolvedTypeMessage
	diagnosticTemplateTextUnresolvedValueMessage
	diagnosticTemplateTextUnusedLocalMessage
	diagnosticTemplateTextAlwaysTrueEvidence
	diagnosticTemplateTextAlwaysFalseEvidence
	diagnosticTemplateTextRedundantClaimValueEvidence
	diagnosticTemplateTextRedundantClaimCheckEvidence
	diagnosticTemplateTextUnresolvedTypeEvidence
	diagnosticTemplateTextUnresolvedValueEvidence
	diagnosticTemplateTextUnusedLocalEvidence
	diagnosticTemplateTextAlwaysTrueHelp
	diagnosticTemplateTextAlwaysFalseHelp
	diagnosticTemplateTextRedundantClaimHelp
	diagnosticTemplateTextUnresolvedTypeHelp
	diagnosticTemplateTextUnresolvedValueHelp
	diagnosticTemplateTextUnusedLocalHelp
	diagnosticTemplateTextConstantGuardLabel
	diagnosticTemplateTextClaimSiteLabel
	diagnosticTemplateTextProvenValueLabel
	diagnosticTemplateTextUnknownTypeLabel
	diagnosticTemplateTextUnknownValueLabel
	diagnosticTemplateTextUnusedLocalLabel
)

// diagnosticSemanticName and diagnosticTargetType are deliberately distinct
// semantic fields. They keep the report contract from becoming a generic
// arbitrary-text rendering API while allowing a producer to carry the exact
// identifier and target already established by its own semantic row.
type diagnosticSemanticName struct{ value string }
type diagnosticTargetType struct{ value string }

func diagnosticTemplateTokenValid(value string) bool {
	// Report templates interpolate this value into message/evidence text. Keep
	// that interpolation closed over semantic names rather than accepting a
	// one-line prose fragment: only ASCII identifier segments separated by '.'
	// are valid (for example LocalPoint, missing_count, or module.Type).
	if value == "" {
		return false
	}
	segmentStart := true
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '.' {
			if segmentStart {
				return false
			}
			segmentStart = true
			continue
		}
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character == '_' {
			segmentStart = false
			continue
		}
		if !segmentStart && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return !segmentStart
}

func newDiagnosticSemanticName(value string) (diagnosticSemanticName, bool) {
	name := diagnosticSemanticName{value: value}
	return name, diagnosticTemplateTokenValid(value)
}

func newDiagnosticTargetType(value string) (diagnosticTargetType, bool) {
	target := diagnosticTargetType{value: value}
	return target, diagnosticTemplateTokenValid(value)
}

func (name diagnosticSemanticName) valid() bool { return diagnosticTemplateTokenValid(name.value) }
func (target diagnosticTargetType) valid() bool { return diagnosticTemplateTokenValid(target.value) }

type diagnosticClaimForm uint8

const (
	diagnosticClaimFormInvalid diagnosticClaimForm = iota
	diagnosticClaimFormTypeClaim
	diagnosticClaimFormTypeCastCall
)

func (form diagnosticClaimForm) valid() bool {
	return form == diagnosticClaimFormTypeClaim || form == diagnosticClaimFormTypeCastCall
}

type diagnosticTemplateDataRequirement uint8

const (
	diagnosticTemplateNeedsSubject diagnosticTemplateDataRequirement = 1 << iota
	diagnosticTemplateNeedsTarget
	diagnosticTemplateNeedsClaimForm
	diagnosticTemplateNeedsProofLocation
)

type diagnosticTemplateLocation uint8

const (
	diagnosticTemplateLocationInvalid diagnosticTemplateLocation = iota
	diagnosticTemplateLocationPrimary
	diagnosticTemplateLocationProof
)

type diagnosticEvidenceTemplate struct {
	location            diagnosticTemplateLocation
	kind, trust, reason string
	detail              diagnosticTemplateText
}

type diagnosticLabelTemplate struct {
	location diagnosticTemplateLocation
	text     diagnosticTemplateText
}

type diagnosticRenderSection uint8

const (
	diagnosticRenderSectionInvalid diagnosticRenderSection = iota
	diagnosticRenderSectionSummary
	diagnosticRenderSectionLocation
	diagnosticRenderSectionSource
	diagnosticRenderSectionEvidence
	diagnosticRenderSectionHelp
)

type diagnosticTemplate struct {
	rule            DiagnosticRule
	code            DiagnosticCode
	codeText        string
	defaultSeverity FindingSeverity
	requirements    diagnosticTemplateDataRequirement
	message, help   diagnosticTemplateText
	evidence        []diagnosticEvidenceTemplate
	labels          []diagnosticLabelTemplate
	render          []diagnosticRenderSection
}

// diagnosticCollectorSpec is the sole installation authority for a native
// producer. Templates describe every supported-or-pending public family;
// collector specs describe only the families a receipt collector can actually
// produce today. Policy validity and receipt dispatch both consult this same
// closed registry, so a presentation descriptor alone can never become an
// enabled-but-empty diagnostic.
type diagnosticCollectorSurface uint8

const (
	diagnosticCollectorSurfaceInvalid diagnosticCollectorSurface = iota
	diagnosticCollectorSurfaceBranch
	diagnosticCollectorSurfaceStatic
)

type diagnosticCollectorSpec struct {
	rule       DiagnosticRule
	code       DiagnosticCode
	surface    diagnosticCollectorSurface
	staticKind programartifact.DiagnosticObservationKind
}

var diagnosticCollectorRegistry = [...]diagnosticCollectorSpec{
	{rule: DiagnosticRuleAlwaysTrueGuard, code: DiagnosticCodeAlwaysTrueGuard, surface: diagnosticCollectorSurfaceBranch},
	{rule: DiagnosticRuleAlwaysFalseGuard, code: DiagnosticCodeAlwaysFalseGuard, surface: diagnosticCollectorSurfaceBranch},
	{rule: DiagnosticRuleUnresolvedTypeReference, code: DiagnosticCodeUnresolvedTypeReference, surface: diagnosticCollectorSurfaceStatic, staticKind: programartifact.DiagnosticObservationTypeReferenceUnresolved},
	{rule: DiagnosticRuleUnresolvedValueReference, code: DiagnosticCodeUnresolvedValueReference, surface: diagnosticCollectorSurfaceStatic, staticKind: programartifact.DiagnosticObservationValueReferenceUnresolved},
}

func diagnosticCollectorSpecForRule(rule DiagnosticRule) (diagnosticCollectorSpec, bool) {
	for _, spec := range diagnosticCollectorRegistry {
		if spec.rule == rule {
			return spec, true
		}
	}
	return diagnosticCollectorSpec{}, false
}

func diagnosticStaticCollectorSpec(kind programartifact.DiagnosticObservationKind) (diagnosticCollectorSpec, bool) {
	for _, spec := range diagnosticCollectorRegistry {
		if spec.surface == diagnosticCollectorSurfaceStatic && spec.staticKind == kind {
			return spec, true
		}
	}
	return diagnosticCollectorSpec{}, false
}

var diagnosticTemplateRegistry = [...]diagnosticTemplate{
	{
		rule: DiagnosticRuleAlwaysTrueGuard, code: DiagnosticCodeAlwaysTrueGuard, codeText: "advice.always_true_guard", defaultSeverity: FindingSeverityHint,
		message: diagnosticTemplateTextAlwaysTrueMessage, help: diagnosticTemplateTextAlwaysTrueHelp,
		evidence: []diagnosticEvidenceTemplate{{location: diagnosticTemplateLocationPrimary, kind: "abstract fact", trust: "proven", reason: "unspecified", detail: diagnosticTemplateTextAlwaysTrueEvidence}},
		labels:   []diagnosticLabelTemplate{{location: diagnosticTemplateLocationPrimary, text: diagnosticTemplateTextConstantGuardLabel}},
		render:   []diagnosticRenderSection{diagnosticRenderSectionSummary, diagnosticRenderSectionLocation, diagnosticRenderSectionSource, diagnosticRenderSectionEvidence, diagnosticRenderSectionHelp},
	},
	{
		rule: DiagnosticRuleAlwaysFalseGuard, code: DiagnosticCodeAlwaysFalseGuard, codeText: "advice.always_false_guard", defaultSeverity: FindingSeverityHint,
		message: diagnosticTemplateTextAlwaysFalseMessage, help: diagnosticTemplateTextAlwaysFalseHelp,
		evidence: []diagnosticEvidenceTemplate{{location: diagnosticTemplateLocationPrimary, kind: "abstract fact", trust: "proven", reason: "unspecified", detail: diagnosticTemplateTextAlwaysFalseEvidence}},
		labels:   []diagnosticLabelTemplate{{location: diagnosticTemplateLocationPrimary, text: diagnosticTemplateTextConstantGuardLabel}},
		render:   []diagnosticRenderSection{diagnosticRenderSectionSummary, diagnosticRenderSectionLocation, diagnosticRenderSectionSource, diagnosticRenderSectionEvidence, diagnosticRenderSectionHelp},
	},
	{
		rule: DiagnosticRuleRedundantClaim, code: DiagnosticCodeRedundantClaim, codeText: "advice.redundant_claim", defaultSeverity: FindingSeverityHint,
		requirements: diagnosticTemplateNeedsSubject | diagnosticTemplateNeedsTarget | diagnosticTemplateNeedsClaimForm | diagnosticTemplateNeedsProofLocation,
		message:      diagnosticTemplateTextRedundantClaimMessage, help: diagnosticTemplateTextRedundantClaimHelp,
		evidence: []diagnosticEvidenceTemplate{
			{location: diagnosticTemplateLocationProof, kind: "abstract fact", trust: "proven", reason: "unspecified", detail: diagnosticTemplateTextRedundantClaimValueEvidence},
			{location: diagnosticTemplateLocationPrimary, kind: "abstract fact", trust: "proven", reason: "unspecified", detail: diagnosticTemplateTextRedundantClaimCheckEvidence},
		},
		labels: []diagnosticLabelTemplate{{location: diagnosticTemplateLocationPrimary, text: diagnosticTemplateTextClaimSiteLabel}, {location: diagnosticTemplateLocationProof, text: diagnosticTemplateTextProvenValueLabel}},
		render: []diagnosticRenderSection{diagnosticRenderSectionSummary, diagnosticRenderSectionLocation, diagnosticRenderSectionSource, diagnosticRenderSectionEvidence, diagnosticRenderSectionHelp},
	},
	{
		rule: DiagnosticRuleUnresolvedTypeReference, code: DiagnosticCodeUnresolvedTypeReference, codeText: "type.reference.unresolved", defaultSeverity: FindingSeverityError,
		requirements: diagnosticTemplateNeedsSubject,
		message:      diagnosticTemplateTextUnresolvedTypeMessage, help: diagnosticTemplateTextUnresolvedTypeHelp,
		evidence: []diagnosticEvidenceTemplate{{location: diagnosticTemplateLocationPrimary, kind: "abstract fact", trust: "proven", reason: "unspecified", detail: diagnosticTemplateTextUnresolvedTypeEvidence}},
		labels:   []diagnosticLabelTemplate{{location: diagnosticTemplateLocationPrimary, text: diagnosticTemplateTextUnknownTypeLabel}},
		render:   []diagnosticRenderSection{diagnosticRenderSectionSummary, diagnosticRenderSectionLocation, diagnosticRenderSectionSource, diagnosticRenderSectionEvidence, diagnosticRenderSectionHelp},
	},
	{
		rule: DiagnosticRuleUnresolvedValueReference, code: DiagnosticCodeUnresolvedValueReference, codeText: "value.reference.unresolved", defaultSeverity: FindingSeverityError,
		requirements: diagnosticTemplateNeedsSubject,
		message:      diagnosticTemplateTextUnresolvedValueMessage, help: diagnosticTemplateTextUnresolvedValueHelp,
		evidence: []diagnosticEvidenceTemplate{{location: diagnosticTemplateLocationPrimary, kind: "abstract fact", trust: "proven", reason: "unspecified", detail: diagnosticTemplateTextUnresolvedValueEvidence}},
		labels:   []diagnosticLabelTemplate{{location: diagnosticTemplateLocationPrimary, text: diagnosticTemplateTextUnknownValueLabel}},
		render:   []diagnosticRenderSection{diagnosticRenderSectionSummary, diagnosticRenderSectionLocation, diagnosticRenderSectionSource, diagnosticRenderSectionEvidence, diagnosticRenderSectionHelp},
	},
	{
		rule: DiagnosticRuleUnusedLocal, code: DiagnosticCodeUnusedLocal, codeText: "lint.unused.local", defaultSeverity: FindingSeverityHint,
		requirements: diagnosticTemplateNeedsSubject,
		message:      diagnosticTemplateTextUnusedLocalMessage, help: diagnosticTemplateTextUnusedLocalHelp,
		evidence: []diagnosticEvidenceTemplate{{location: diagnosticTemplateLocationPrimary, kind: "abstract fact", trust: "proven", reason: "unspecified", detail: diagnosticTemplateTextUnusedLocalEvidence}},
		labels:   []diagnosticLabelTemplate{{location: diagnosticTemplateLocationPrimary, text: diagnosticTemplateTextUnusedLocalLabel}},
		render:   []diagnosticRenderSection{diagnosticRenderSectionSummary, diagnosticRenderSectionLocation, diagnosticRenderSectionSource, diagnosticRenderSectionEvidence, diagnosticRenderSectionHelp},
	},
}

func diagnosticTemplateForCode(code DiagnosticCode) (*diagnosticTemplate, bool) {
	for index := range diagnosticTemplateRegistry {
		template := &diagnosticTemplateRegistry[index]
		if template.code == code {
			return template, true
		}
	}
	return nil, false
}

func diagnosticTemplateForRule(rule DiagnosticRule) (*diagnosticTemplate, bool) {
	for index := range diagnosticTemplateRegistry {
		template := &diagnosticTemplateRegistry[index]
		if template.rule == rule {
			return template, true
		}
	}
	return nil, false
}

func (code DiagnosticCode) String() string {
	template, ok := diagnosticTemplateForCode(code)
	if !ok {
		return ""
	}
	return template.codeText
}

func (rule DiagnosticRule) Code() DiagnosticCode {
	template, ok := diagnosticTemplateForRule(rule)
	if !ok {
		return DiagnosticCodeInvalid
	}
	return template.code
}

func (rule DiagnosticRule) DefaultSeverity() FindingSeverity {
	template, ok := diagnosticTemplateForRule(rule)
	if !ok {
		return FindingSeverityInvalid
	}
	return template.defaultSeverity
}

func (severity FindingSeverity) String() string {
	switch severity {
	case FindingSeverityError:
		return "error"
	case FindingSeverityWarning:
		return "warning"
	case FindingSeverityHint:
		return "hint"
	default:
		return ""
	}
}

// DiagnosticPolicy is opt-in. A zero policy never collects semantic reports.
type DiagnosticPolicy struct {
	Enabled  []DiagnosticRule
	Severity map[DiagnosticRule]FindingSeverity
}

func (policy DiagnosticPolicy) enabled(rule DiagnosticRule) (FindingSeverity, bool) {
	for _, candidate := range policy.Enabled {
		if candidate == rule {
			severity := rule.DefaultSeverity()
			if policy.Severity != nil {
				if override, ok := policy.Severity[rule]; ok {
					severity = override
				}
			}
			return severity, severity != FindingSeverityInvalid
		}
	}
	return FindingSeverityInvalid, false
}

// collectorInstalled reports whether the native collector has this producer.
// A presentation template alone is deliberately insufficient: accepting a
// dormant rule would let an API caller receive a clean empty report for a
// family no producer has collected yet.
func (rule DiagnosticRule) collectorInstalled() bool {
	_, ok := diagnosticCollectorSpecForRule(rule)
	return ok
}
func (code DiagnosticCode) valid() bool { _, ok := diagnosticTemplateForCode(code); return ok }
func (severity FindingSeverity) valid() bool {
	return severity >= FindingSeverityError && severity <= FindingSeverityHint
}

// Valid rejects ambiguous policy authority before a solve starts: every rule
// is known and unique, and overrides may only refine an enabled known rule.
func (policy DiagnosticPolicy) Valid() bool {
	enabled := make(map[DiagnosticRule]struct{}, len(policy.Enabled))
	for _, rule := range policy.Enabled {
		if !rule.collectorInstalled() {
			return false
		}
		if _, duplicate := enabled[rule]; duplicate {
			return false
		}
		enabled[rule] = struct{}{}
	}
	for rule, severity := range policy.Severity {
		if !rule.collectorInstalled() || !severity.valid() {
			return false
		}
		if _, selected := enabled[rule]; !selected {
			return false
		}
	}
	return true
}

type diagnosticFinding struct {
	id, subject keyspace.ContentID
	code        DiagnosticCode
	severity    FindingSeverity
	location    DiagnosticLocation
	data        diagnosticTemplateData
}

// diagnosticTemplateData is the complete typed payload a descriptor may use.
// It is intentionally private and row-shaped: an upstream producer supplies
// semantic names, a target type, a claim form, and an already-authenticated
// proof anchor; no producer can inject pre-rendered message/evidence prose.
type diagnosticTemplateData struct {
	subject diagnosticSemanticName
	target  diagnosticTargetType
	claim   diagnosticClaimForm
	proof   DiagnosticLocation
}

func (data diagnosticTemplateData) validFor(template diagnosticTemplate) bool {
	if template.requirements&diagnosticTemplateNeedsSubject != 0 && !data.subject.valid() {
		return false
	}
	if template.requirements&diagnosticTemplateNeedsSubject == 0 && data.subject != (diagnosticSemanticName{}) {
		return false
	}
	if template.requirements&diagnosticTemplateNeedsTarget != 0 && !data.target.valid() {
		return false
	}
	if template.requirements&diagnosticTemplateNeedsTarget == 0 && data.target != (diagnosticTargetType{}) {
		return false
	}
	if template.requirements&diagnosticTemplateNeedsClaimForm != 0 && !data.claim.valid() {
		return false
	}
	if template.requirements&diagnosticTemplateNeedsClaimForm == 0 && data.claim != diagnosticClaimFormInvalid {
		return false
	}
	if template.requirements&diagnosticTemplateNeedsProofLocation != 0 && !data.proof.Available() {
		return false
	}
	if template.requirements&diagnosticTemplateNeedsProofLocation == 0 && data.proof != (DiagnosticLocation{}) {
		return false
	}
	return true
}

func (data diagnosticTemplateData) location(which diagnosticTemplateLocation, primary DiagnosticLocation) (DiagnosticLocation, bool) {
	switch which {
	case diagnosticTemplateLocationPrimary:
		return primary, primary.Available()
	case diagnosticTemplateLocationProof:
		return data.proof, data.proof.Available()
	default:
		return DiagnosticLocation{}, false
	}
}

func (text diagnosticTemplateText) render(data diagnosticTemplateData) string {
	switch text {
	case diagnosticTemplateTextAlwaysTrueMessage:
		return "condition is proven always true"
	case diagnosticTemplateTextAlwaysFalseMessage:
		return "condition is proven always false"
	case diagnosticTemplateTextRedundantClaimMessage:
		claim := "type claim"
		if data.claim == diagnosticClaimFormTypeCastCall {
			claim = "type cast call"
		}
		return fmt.Sprintf("%s is redundant; value is already %s", claim, data.target.value)
	case diagnosticTemplateTextUnresolvedTypeMessage:
		return "unknown type " + data.subject.value
	case diagnosticTemplateTextUnresolvedValueMessage:
		return "unknown value " + data.subject.value
	case diagnosticTemplateTextUnusedLocalMessage:
		return fmt.Sprintf("local %q is never read", data.subject.value)
	case diagnosticTemplateTextAlwaysTrueEvidence:
		return "condition is proven to be true on every reachable path"
	case diagnosticTemplateTextAlwaysFalseEvidence:
		return "condition is proven to be false on every reachable path"
	case diagnosticTemplateTextRedundantClaimValueEvidence:
		return fmt.Sprintf("%s is proven to be %s before the claim", data.subject.value, data.target.value)
	case diagnosticTemplateTextRedundantClaimCheckEvidence:
		return fmt.Sprintf("claim checks %s at this site", data.target.value)
	case diagnosticTemplateTextUnresolvedTypeEvidence:
		return fmt.Sprintf("no type named %s is declared in this scope", data.subject.value)
	case diagnosticTemplateTextUnresolvedValueEvidence:
		return fmt.Sprintf("no value named %s is declared, predeclared, imported, or configured global in this scope", data.subject.value)
	case diagnosticTemplateTextUnusedLocalEvidence:
		return fmt.Sprintf("no read of local %q was found in this scope", data.subject.value)
	case diagnosticTemplateTextAlwaysTrueHelp:
		return "Remove the guard or move the guarded code out of the branch."
	case diagnosticTemplateTextAlwaysFalseHelp:
		return "Remove the unreachable branch or invert the guard."
	case diagnosticTemplateTextRedundantClaimHelp:
		return "Remove the runtime type claim when the proven source type is sufficient."
	case diagnosticTemplateTextUnresolvedTypeHelp:
		return "Declare the type in scope"
	case diagnosticTemplateTextUnresolvedValueHelp:
		return "Declare the value"
	case diagnosticTemplateTextUnusedLocalHelp:
		return "Remove it, use it, or rename it with a leading _ when intentionally unused."
	case diagnosticTemplateTextConstantGuardLabel:
		return "constant guard"
	case diagnosticTemplateTextClaimSiteLabel:
		return "claim site"
	case diagnosticTemplateTextProvenValueLabel:
		return "proven value"
	case diagnosticTemplateTextUnknownTypeLabel:
		return "unknown type"
	case diagnosticTemplateTextUnknownValueLabel:
		return "unknown value"
	case diagnosticTemplateTextUnusedLocalLabel:
		return "unused local"
	default:
		return ""
	}
}

// DiagnosticEvidence is one immutable proof row attached to a Finding. The
// row is Analysis-owned presentation data; it is not an Engine fact or a
// policy input.
type DiagnosticEvidence struct {
	location       DiagnosticLocation
	kind, trust    string
	reason, detail string
}

func (evidence DiagnosticEvidence) Available() bool {
	return evidence.location.Available() && evidence.kind != "" && evidence.trust != "" && evidence.reason != "" && evidence.detail != ""
}
func (evidence DiagnosticEvidence) Location() (DiagnosticLocation, bool) {
	return evidence.location, evidence.Available()
}
func (evidence DiagnosticEvidence) Kind() string {
	if !evidence.Available() {
		return ""
	}
	return evidence.kind
}
func (evidence DiagnosticEvidence) Trust() string {
	if !evidence.Available() {
		return ""
	}
	return evidence.trust
}
func (evidence DiagnosticEvidence) Reason() string {
	if !evidence.Available() {
		return ""
	}
	return evidence.reason
}
func (evidence DiagnosticEvidence) Detail() string {
	if !evidence.Available() {
		return ""
	}
	return evidence.detail
}

// Text is an ergonomic alias for Detail when callers render evidence prose.
func (evidence DiagnosticEvidence) Text() string { return evidence.Detail() }

// DiagnosticLabel is one immutable source label attached to a Finding.
type DiagnosticLabel struct {
	location DiagnosticLocation
	text     string
}

func (label DiagnosticLabel) Available() bool { return label.location.Available() && label.text != "" }
func (label DiagnosticLabel) Location() (DiagnosticLocation, bool) {
	return label.location, label.Available()
}
func (label DiagnosticLabel) Text() string {
	if !label.Available() {
		return ""
	}
	return label.text
}

type DiagnosticReport struct {
	source, result    keyspace.ContentID
	findings          []diagnosticFinding
	collectionFailure DiagnosticCollectionFailure
	sealed            bool
}

func (report *DiagnosticReport) CollectionFailure() DiagnosticCollectionFailure {
	if !report.Available() {
		return DiagnosticCollectionSubjectQueryAbsent
	}
	return report.collectionFailure
}

// Finding is an immutable ordinal-fenced view into one detached report row.
type Finding struct {
	owner   *DiagnosticReport
	ordinal uint32
}

// DiagnosticLocation is the authored source span copied into the reusable
// ProgramArtifact while Program's Source proof is live. Its fields are private
// so callers can consume, but cannot forge, report locations.
type DiagnosticLocation struct {
	file                   string
	startLine, startColumn uint32
	endLine, endColumn     uint32
}

func newDiagnosticLocation(file string, startLine, startColumn, endLine, endColumn uint32) (DiagnosticLocation, bool) {
	location := DiagnosticLocation{file: file, startLine: startLine, startColumn: startColumn, endLine: endLine, endColumn: endColumn}
	return location, location.Available()
}

func (location DiagnosticLocation) Available() bool {
	if location.file == "" || location.startLine == 0 || location.startColumn == 0 || (location.endLine == 0) != (location.endColumn == 0) {
		return false
	}
	return location.endLine == 0 || location.endLine > location.startLine || location.endLine == location.startLine && location.endColumn >= location.startColumn
}
func (location DiagnosticLocation) File() string {
	if !location.Available() {
		return ""
	}
	return location.file
}
func (location DiagnosticLocation) Start() (line, column uint32) {
	if !location.Available() {
		return 0, 0
	}
	return location.startLine, location.startColumn
}
func (location DiagnosticLocation) End() (line, column uint32, ok bool) {
	if !location.Available() || location.endLine == 0 {
		return 0, 0, false
	}
	return location.endLine, location.endColumn, true
}

func (report *DiagnosticReport) Available() bool {
	if report == nil || !report.sealed || !report.source.Available() || !report.result.Available() {
		return false
	}
	for _, finding := range report.findings {
		if !validDiagnosticFinding(finding) {
			return false
		}
	}
	return true
}
func (report *DiagnosticReport) SourceID() keyspace.ContentID {
	if !report.Available() {
		return keyspace.ContentID{}
	}
	return report.source
}
func (report *DiagnosticReport) ResultID() keyspace.ContentID {
	if !report.Available() {
		return keyspace.ContentID{}
	}
	return report.result
}
func (report *DiagnosticReport) FindingCount() int {
	if !report.Available() {
		return 0
	}
	return len(report.findings)
}
func (report *DiagnosticReport) FindingAt(index int) (Finding, bool) {
	if !report.Available() || index < 0 || index >= len(report.findings) {
		return Finding{}, false
	}
	return Finding{owner: report, ordinal: uint32(index + 1)}, true
}
func (finding Finding) row() (diagnosticFinding, bool) {
	if finding.owner == nil || !finding.owner.Available() || finding.ordinal == 0 || uint64(finding.ordinal) > uint64(len(finding.owner.findings)) {
		return diagnosticFinding{}, false
	}
	row := finding.owner.findings[finding.ordinal-1]
	return row, validDiagnosticFinding(row)
}

func validDiagnosticFinding(row diagnosticFinding) bool {
	template, templateOK := diagnosticTemplateForCode(row.code)
	if !row.id.Available() || !row.subject.Available() || !templateOK || !row.severity.valid() || !row.location.Available() || !row.data.validFor(*template) {
		return false
	}
	return true
}
func (finding Finding) ID() (keyspace.ContentID, bool) { row, ok := finding.row(); return row.id, ok }
func (finding Finding) SubjectID() (keyspace.ContentID, bool) {
	row, ok := finding.row()
	return row.subject, ok
}
func (finding Finding) Code() DiagnosticCode {
	row, ok := finding.row()
	if !ok {
		return DiagnosticCodeInvalid
	}
	return row.code
}
func (finding Finding) Severity() FindingSeverity {
	row, ok := finding.row()
	if !ok {
		return FindingSeverityInvalid
	}
	return row.severity
}
func (finding Finding) Location() (DiagnosticLocation, bool) {
	row, ok := finding.row()
	return row.location, ok
}
func (finding Finding) EvidenceCount() int {
	row, ok := finding.row()
	if !ok {
		return 0
	}
	template, ok := diagnosticTemplateForCode(row.code)
	if !ok {
		return 0
	}
	return len(template.evidence)
}
func (finding Finding) EvidenceAt(index int) (DiagnosticEvidence, bool) {
	row, ok := finding.row()
	if !ok {
		return DiagnosticEvidence{}, false
	}
	template, ok := diagnosticTemplateForCode(row.code)
	if !ok || index < 0 || index >= len(template.evidence) {
		return DiagnosticEvidence{}, false
	}
	descriptor := template.evidence[index]
	location, locationOK := row.data.location(descriptor.location, row.location)
	detail := descriptor.detail.render(row.data)
	evidence := DiagnosticEvidence{location: location, kind: descriptor.kind, trust: descriptor.trust, reason: descriptor.reason, detail: detail}
	return evidence, locationOK && evidence.Available()
}
func (finding Finding) LabelCount() int {
	row, ok := finding.row()
	if !ok {
		return 0
	}
	template, ok := diagnosticTemplateForCode(row.code)
	if !ok {
		return 0
	}
	return len(template.labels)
}
func (finding Finding) LabelAt(index int) (DiagnosticLabel, bool) {
	row, ok := finding.row()
	if !ok {
		return DiagnosticLabel{}, false
	}
	template, ok := diagnosticTemplateForCode(row.code)
	if !ok || index < 0 || index >= len(template.labels) {
		return DiagnosticLabel{}, false
	}
	descriptor := template.labels[index]
	location, locationOK := row.data.location(descriptor.location, row.location)
	label := DiagnosticLabel{location: location, text: descriptor.text.render(row.data)}
	return label, locationOK && label.Available()
}
func (finding Finding) Message() string {
	row, ok := finding.row()
	if !ok {
		return ""
	}
	template, ok := diagnosticTemplateForCode(row.code)
	if !ok {
		return ""
	}
	return template.message.render(row.data)
}
func (finding Finding) Help() string {
	row, ok := finding.row()
	if !ok {
		return ""
	}
	template, ok := diagnosticTemplateForCode(row.code)
	if !ok {
		return ""
	}
	return template.help.render(row.data)
}

// Render returns the stable Analysis-owned diagnostic presentation. It uses
// fixed templates for the closed rule vocabulary and never incorporates raw
// Engine or artifact text.
func (finding Finding) Render() string {
	row, ok := finding.row()
	if !ok {
		return ""
	}
	return finding.render(row, "", false)
}

// RenderSource renders the complete deterministic presentation using
// caller-owned source text as display context. The source is never retained
// and cannot influence the finding's code, severity, evidence, labels, or
// help. It must identify the finding's exact file and contain its start line.
func (finding Finding) RenderSource(sourceFile, sourceText string) (string, bool) {
	row, ok := finding.row()
	if !ok || sourceFile == "" || sourceFile != row.location.File() {
		return "", false
	}
	line, ok := sourceLine(sourceText, row.location.startLine)
	if !ok {
		return "", false
	}
	return finding.render(row, strings.TrimSpace(line), true), true
}

func sourceLine(sourceText string, line uint32) (string, bool) {
	if line == 0 || sourceText == "" {
		return "", false
	}
	// Normalize CRLF/CR without changing the caller-owned string and retain
	// empty lines so source coordinates remain one-based and exact.
	text := strings.ReplaceAll(sourceText, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if uint64(line) > uint64(len(lines)) {
		return "", false
	}
	return lines[line-1], true
}

func (finding Finding) render(row diagnosticFinding, sourceLineText string, includeSource bool) string {
	line, column := row.location.Start()
	template, templateOK := diagnosticTemplateForCode(row.code)
	if !templateOK {
		return ""
	}
	var rendered strings.Builder
	for _, section := range template.render {
		switch section {
		case diagnosticRenderSectionSummary:
			fmt.Fprintf(&rendered, "%s[%s]: %s\n", row.severity.String(), template.codeText, template.message.render(row.data))
		case diagnosticRenderSectionLocation:
			fmt.Fprintf(&rendered, "--> %s:%d:%d\n", row.location.File(), line, column)
		case diagnosticRenderSectionSource:
			if includeSource {
				fmt.Fprintf(&rendered, "%d | %s\n", line, sourceLineText)
			}
		case diagnosticRenderSectionEvidence:
			if len(template.evidence) == 0 {
				continue
			}
			rendered.WriteString("because:\n")
			for index := range template.evidence {
				evidence, evidenceOK := finding.EvidenceAt(index)
				if !evidenceOK {
					return ""
				}
				fmt.Fprintf(&rendered, "%d. %s: %s\n", index+1, evidence.Trust(), evidence.Detail())
			}
		case diagnosticRenderSectionHelp:
			if help := template.help.render(row.data); help != "" {
				fmt.Fprintf(&rendered, "help: %s\n", help)
			}
		}
	}
	return strings.TrimSuffix(rendered.String(), "\n")
}

// SolveWithReport leaves inference untouched. Enabled rules collect only from
// reusable artifact subjects and observations already produced by the shared
// solve; policy selection never adds an Engine query or changes Result identity.
func (plan *Plan) SolveWithReport(ctx context.Context, options engine.SolveDiagnosticOptions, policy DiagnosticPolicy) (*Result, *DiagnosticReport, AnalyzeStatus, AnalyzeDiagnostics) {
	if !policy.Valid() {
		return nil, nil, AnalyzeInvalid, AnalyzeDiagnostics{Phase: AnalyzeDiagnosticPhaseSetup, Reason: AnalyzeDiagnosticReasonInvalidOptions}
	}
	return plan.solveWithPolicy(ctx, options, &policy, false)
}
