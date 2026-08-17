package analysis

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
)

// DiagnosticCode is the analyzer's published diagnostic identity. The
// declaration table owns the inventory: a code names one sealed declaration
// row, and every per-code answer below is a lookup on that row.
type DiagnosticCode = diagnostic.Code

const (
	DiagnosticCodeInvalid                  = diagnostic.CodeInvalid
	DiagnosticCodeAlwaysTrueGuard          = composite.DiagnosticCodeAlwaysTrueGuard
	DiagnosticCodeAlwaysFalseGuard         = composite.DiagnosticCodeAlwaysFalseGuard
	DiagnosticCodeRedundantClaim           = composite.DiagnosticCodeRedundantClaim
	DiagnosticCodeUnresolvedTypeReference  = composite.DiagnosticCodeUnresolvedTypeReference
	DiagnosticCodeUnresolvedValueReference = composite.DiagnosticCodeUnresolvedValueReference
	DiagnosticCodeUnusedLocal              = composite.DiagnosticCodeUnusedLocal
)

// FindingSeverity is the closed severity vocabulary the declaration table
// declares a default in and a policy may refine within.
type FindingSeverity = diagnostic.Severity

const (
	FindingSeverityInvalid = diagnostic.SeverityInvalid
	FindingSeverityError   = diagnostic.SeverityError
	FindingSeverityWarning = diagnostic.SeverityWarning
	FindingSeverityHint    = diagnostic.SeverityHint
)

// diagnosticDeclaration resolves one published code in the sealed declaration
// table. It is the only per-code lookup in Analysis; nothing here restates the
// inventory.
func diagnosticDeclaration(code DiagnosticCode) (*diagnostic.Entry, bool) {
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		return nil, false
	}
	return table.ForCode(code)
}

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

// diagnosticClaimForm is the closed claim vocabulary one payload may carry.
// Its rendered words are the only prose this file owns that a declaration row
// does not: they are the vocabulary itself, not a per-code message.
type diagnosticClaimForm uint8

const (
	diagnosticClaimFormInvalid diagnosticClaimForm = iota
	diagnosticClaimFormTypeClaim
	diagnosticClaimFormTypeCastCall
)

func (form diagnosticClaimForm) valid() bool {
	return form == diagnosticClaimFormTypeClaim || form == diagnosticClaimFormTypeCastCall
}

func (form diagnosticClaimForm) text() string {
	switch form {
	case diagnosticClaimFormTypeClaim:
		return "type claim"
	case diagnosticClaimFormTypeCastCall:
		return "type cast call"
	default:
		return ""
	}
}

// DiagnosticPolicy is opt-in. A zero policy never collects semantic reports.
type DiagnosticPolicy struct {
	Enabled  []DiagnosticCode
	Severity map[DiagnosticCode]FindingSeverity
}

func (policy DiagnosticPolicy) enabled(code DiagnosticCode) (FindingSeverity, bool) {
	for _, candidate := range policy.Enabled {
		if candidate != code {
			continue
		}
		entry, entryOK := diagnosticDeclaration(code)
		if !entryOK {
			return FindingSeverityInvalid, false
		}
		severity := entry.DefaultSeverity()
		if policy.Severity != nil {
			if override, ok := policy.Severity[code]; ok {
				severity = override
			}
		}
		return severity, severity.Available()
	}
	return FindingSeverityInvalid, false
}

// diagnosticCollectable reports whether the declaration table installs a
// producer for this code. A presentation row alone is deliberately
// insufficient: accepting a dormant code would let an API caller receive a
// clean empty report for a family no producer has collected yet.
func diagnosticCollectable(code DiagnosticCode) bool {
	entry, ok := diagnosticDeclaration(code)
	return ok && entry.Collectable()
}

// Valid rejects ambiguous policy authority before a solve starts: every code
// is known and unique, and overrides may only refine an enabled known code.
func (policy DiagnosticPolicy) Valid() bool {
	enabled := make(map[DiagnosticCode]struct{}, len(policy.Enabled))
	for _, code := range policy.Enabled {
		if !diagnosticCollectable(code) {
			return false
		}
		if _, duplicate := enabled[code]; duplicate {
			return false
		}
		enabled[code] = struct{}{}
	}
	for code, severity := range policy.Severity {
		if !diagnosticCollectable(code) || !severity.Available() {
			return false
		}
		if _, selected := enabled[code]; !selected {
			return false
		}
	}
	return true
}

type diagnosticFinding struct {
	id, subject identity.ContentID
	code        DiagnosticCode
	severity    FindingSeverity
	location    DiagnosticLocation
	data        diagnosticTemplateData
}

// diagnosticTemplateData is the complete typed payload a declaration may use.
// It is intentionally private and row-shaped: an upstream producer supplies
// semantic names, a target type, a claim form, and an already-authenticated
// proof anchor; no producer can inject pre-rendered message/evidence prose.
type diagnosticTemplateData struct {
	subject diagnosticSemanticName
	target  diagnosticTargetType
	claim   diagnosticClaimForm
	proof   DiagnosticLocation
}

// validFor states the payload contract of one declaration: exactly the fields
// the row requires are supplied, and nothing else is.
func (data diagnosticTemplateData) validFor(entry *diagnostic.Entry) bool {
	requirements := entry.Requirements()
	if !payloadFieldValid(requirements&diagnostic.RequiresSubject != 0, data.subject.valid(), data.subject == diagnosticSemanticName{}) {
		return false
	}
	if !payloadFieldValid(requirements&diagnostic.RequiresTarget != 0, data.target.valid(), data.target == diagnosticTargetType{}) {
		return false
	}
	if !payloadFieldValid(requirements&diagnostic.RequiresClaimForm != 0, data.claim.valid(), data.claim == diagnosticClaimFormInvalid) {
		return false
	}
	return payloadFieldValid(requirements&diagnostic.RequiresProofLocation != 0, data.proof.Available(), data.proof == DiagnosticLocation{})
}

// payloadFieldValid states one payload field's contract: a required field is
// supplied and valid, and an unrequired field is absent entirely. Carrying an
// unread value is a producer error, not a field a renderer may ignore.
func payloadFieldValid(required, valid, absent bool) bool {
	if required {
		return valid
	}
	return absent
}

func (data diagnosticTemplateData) location(anchor diagnostic.Anchor, primary DiagnosticLocation) (DiagnosticLocation, bool) {
	switch anchor {
	case diagnostic.AnchorPrimary:
		return primary, primary.Available()
	case diagnostic.AnchorProof:
		return data.proof, data.proof.Available()
	default:
		return DiagnosticLocation{}, false
	}
}

// renderDiagnosticLine substitutes one already-parsed declaration line. The
// surface proved at seal that every segment names a payload field this row
// requires, so rendering is a walk with no parsing and no unresolved read.
func renderDiagnosticLine(line diagnostic.Line, data diagnosticTemplateData) string {
	var rendered strings.Builder
	for index := 0; index < line.Count(); index++ {
		segment, ok := line.At(index)
		if !ok {
			return ""
		}
		switch segment.Placeholder {
		case diagnostic.PlaceholderInvalid:
			rendered.WriteString(segment.Literal)
		case diagnostic.PlaceholderSubject:
			rendered.WriteString(data.subject.value)
		case diagnostic.PlaceholderQuotedSubject:
			rendered.WriteString(strconv.Quote(data.subject.value))
		case diagnostic.PlaceholderTarget:
			rendered.WriteString(data.target.value)
		case diagnostic.PlaceholderClaimForm:
			rendered.WriteString(data.claim.text())
		default:
			return ""
		}
	}
	return rendered.String()
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
	source, result    identity.ContentID
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
func (report *DiagnosticReport) SourceID() identity.ContentID {
	if !report.Available() {
		return identity.ContentID{}
	}
	return report.source
}
func (report *DiagnosticReport) ResultID() identity.ContentID {
	if !report.Available() {
		return identity.ContentID{}
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
	entry, entryOK := diagnosticDeclaration(row.code)
	if !row.id.Available() || !row.subject.Available() || !entryOK || !row.severity.Available() || !row.location.Available() || !row.data.validFor(entry) {
		return false
	}
	return true
}
func (finding Finding) ID() (identity.ContentID, bool) { row, ok := finding.row(); return row.id, ok }
func (finding Finding) SubjectID() (identity.ContentID, bool) {
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

// declaration resolves the sealed row this finding publishes under.
func (finding Finding) declaration() (diagnosticFinding, *diagnostic.Entry, bool) {
	row, rowOK := finding.row()
	if !rowOK {
		return diagnosticFinding{}, nil, false
	}
	entry, entryOK := diagnosticDeclaration(row.code)
	return row, entry, entryOK
}
func (finding Finding) EvidenceCount() int {
	_, entry, ok := finding.declaration()
	if !ok {
		return 0
	}
	return entry.EvidenceCount()
}
func (finding Finding) EvidenceAt(index int) (DiagnosticEvidence, bool) {
	row, entry, ok := finding.declaration()
	if !ok {
		return DiagnosticEvidence{}, false
	}
	declared, declaredOK := entry.EvidenceAt(index)
	if !declaredOK {
		return DiagnosticEvidence{}, false
	}
	location, locationOK := row.data.location(declared.Anchor, row.location)
	detail := renderDiagnosticLine(declared.Detail, row.data)
	evidence := DiagnosticEvidence{location: location, kind: declared.Kind, trust: declared.Trust, reason: declared.Reason, detail: detail}
	return evidence, locationOK && evidence.Available()
}
func (finding Finding) LabelCount() int {
	_, entry, ok := finding.declaration()
	if !ok {
		return 0
	}
	return entry.LabelCount()
}
func (finding Finding) LabelAt(index int) (DiagnosticLabel, bool) {
	row, entry, ok := finding.declaration()
	if !ok {
		return DiagnosticLabel{}, false
	}
	declared, declaredOK := entry.LabelAt(index)
	if !declaredOK {
		return DiagnosticLabel{}, false
	}
	location, locationOK := row.data.location(declared.Anchor, row.location)
	label := DiagnosticLabel{location: location, text: renderDiagnosticLine(declared.Text, row.data)}
	return label, locationOK && label.Available()
}
func (finding Finding) Message() string {
	row, entry, ok := finding.declaration()
	if !ok {
		return ""
	}
	return renderDiagnosticLine(entry.Message(), row.data)
}
func (finding Finding) Help() string {
	row, entry, ok := finding.declaration()
	if !ok {
		return ""
	}
	return renderDiagnosticLine(entry.Help(), row.data)
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
	entry, entryOK := diagnosticDeclaration(row.code)
	if !entryOK {
		return ""
	}
	var rendered strings.Builder
	for position := 0; position < entry.RenderCount(); position++ {
		section, sectionOK := entry.RenderAt(position)
		if !sectionOK {
			return ""
		}
		switch section {
		case diagnostic.SectionSummary:
			fmt.Fprintf(&rendered, "%s[%s]: %s\n", row.severity.String(), row.code, renderDiagnosticLine(entry.Message(), row.data))
		case diagnostic.SectionLocation:
			fmt.Fprintf(&rendered, "--> %s:%d:%d\n", row.location.File(), line, column)
		case diagnostic.SectionSource:
			if includeSource {
				fmt.Fprintf(&rendered, "%d | %s\n", line, sourceLineText)
			}
		case diagnostic.SectionEvidence:
			if entry.EvidenceCount() == 0 {
				continue
			}
			rendered.WriteString("because:\n")
			for index := 0; index < entry.EvidenceCount(); index++ {
				evidence, evidenceOK := finding.EvidenceAt(index)
				if !evidenceOK {
					return ""
				}
				fmt.Fprintf(&rendered, "%d. %s: %s\n", index+1, evidence.Trust(), evidence.Detail())
			}
		case diagnostic.SectionHelp:
			if help := renderDiagnosticLine(entry.Help(), row.data); help != "" {
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
