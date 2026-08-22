package diagnostic

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
	typedomain "github.com/wippyai/go-lua/domain/type"
)

// DiagnosticCode is the analyzer's published diagnostic identity. The
// declaration table owns the inventory: a code names one sealed declaration
// row, and every per-code answer below is a lookup on that row.
type DiagnosticCode = schemadiag.Code

const (
	DiagnosticCodeInvalid                     = schemadiag.CodeInvalid
	DiagnosticCodeAlwaysTrueGuard             = composite.DiagnosticCodeAlwaysTrueGuard
	DiagnosticCodeAlwaysFalseGuard            = composite.DiagnosticCodeAlwaysFalseGuard
	DiagnosticCodeRedundantClaim              = composite.DiagnosticCodeRedundantClaim
	DiagnosticCodeUnresolvedTypeReference     = composite.DiagnosticCodeUnresolvedTypeReference
	DiagnosticCodeUnresolvedValueReference    = composite.DiagnosticCodeUnresolvedValueReference
	DiagnosticCodeUnusedLocal                 = composite.DiagnosticCodeUnusedLocal
	DiagnosticCodeChannelSelectExhaustiveness = typedomain.ChannelSelectExhaustivenessCode
)

// FindingSeverity is the closed severity vocabulary the declaration table
// declares a default in and a policy may refine within.
type FindingSeverity = schemadiag.Severity

const (
	FindingSeverityInvalid = schemadiag.SeverityInvalid
	FindingSeverityError   = schemadiag.SeverityError
	FindingSeverityWarning = schemadiag.SeverityWarning
	FindingSeverityHint    = schemadiag.SeverityHint
)

// findingSeveritySpelling is the declared name one severity renders under. The
// severity vocabulary is declared on the structural surface, so a rendered
// label is read from the declaration rather than from a switch kept here.
func FindingSeverityName(vocabulary structure.Table, severity FindingSeverity) (string, bool) {
	return findingSeveritySpelling(vocabulary, severity)
}

func findingSeveritySpelling(vocabulary structure.Table, severity FindingSeverity) (string, bool) {
	if !severity.Available() {
		return "", false
	}
	member, memberOK := vocabulary.At(structure.CategoryDiagnosticSeverity, severity.Ordinal())
	if !memberOK {
		return "", false
	}
	return member.Spelling(), true
}

// diagnosticDeclaration resolves one published code in the sealed declaration
// table. It is the only per-code lookup in Analysis; nothing here restates the
// inventory.
func Declaration(table schemadiag.Table, code DiagnosticCode) (*schemadiag.Entry, bool) {
	return diagnosticDeclaration(table, code)
}

func ForgedFinding(report *DiagnosticReport, ordinal uint32) Finding {
	return Finding{owner: report, ordinal: ordinal}
}

func UnsafeTemplateData(subject, target string, claim diagnosticClaimForm, witnesses ...DiagnosticLocation) diagnosticTemplateData {
	return diagnosticTemplateData{subject: diagnosticSemanticName{value: subject}, target: diagnosticTargetType{value: target}, claim: claim, witnesses: append([]DiagnosticLocation(nil), witnesses...)}
}

func diagnosticDeclaration(table schemadiag.Table, code DiagnosticCode) (*schemadiag.Entry, bool) {
	if !table.Available() {
		return nil, false
	}
	return table.ForCode(code)
}

// DiagnosticCollectionFailure is a closed collector classification;
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

// diagnosticTemplateTokenValid admits exactly the closed access-path grammar a
// finding names its subject by. Report templates interpolate this value into
// message and evidence text, so the grammar exists to keep that interpolation
// closed over names and paths rather than opening it to a prose fragment.
//
// The grammar is one identifier followed by any number of authored access
// steps:
//
//	path   := ident step*
//	step   := "." ident | "[" index "]" | ":" ident "(...)" | "(...)"
//	index  := integer | quoted-string
//
// A plain dotted name (LocalPoint, missing_count, module.Type) is the one-step
// subset of it, so every token the earlier declaration-name grammar admitted is
// still admitted. Anything with whitespace, an unbalanced bracket, an
// unterminated string, or an actual list that is not the elision is prose and
// is refused.
func diagnosticTemplateTokenValid(value string) bool {
	cursor, ok := templateTokenIdentifier(value, 0)
	if !ok {
		return false
	}
	for cursor < len(value) {
		cursor, ok = templateTokenStep(value, cursor)
		if !ok {
			return false
		}
	}
	return true
}

// templateTokenStep consumes one authored access step at cursor.
func templateTokenStep(value string, cursor int) (int, bool) {
	switch value[cursor] {
	case '.':
		return templateTokenIdentifier(value, cursor+1)
	case '[':
		return templateTokenIndex(value, cursor+1)
	case ':':
		next, ok := templateTokenIdentifier(value, cursor+1)
		if !ok {
			return 0, false
		}
		return templateTokenElision(value, next)
	case '(':
		return templateTokenElision(value, cursor)
	default:
		return 0, false
	}
}

// templateTokenIdentifier consumes one ASCII identifier at cursor.
func templateTokenIdentifier(value string, cursor int) (int, bool) {
	start := cursor
	for cursor < len(value) {
		character := value[cursor]
		letter := character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character == '_'
		digit := character >= '0' && character <= '9'
		if !letter && !(digit && cursor > start) {
			break
		}
		cursor++
	}
	return cursor, cursor > start
}

// templateTokenIndex consumes one bracket subscript body and its closing
// bracket. The body is a decimal integer or a double-quoted string with no
// control characters and no escape other than an escaped quote or backslash.
func templateTokenIndex(value string, cursor int) (int, bool) {
	if cursor >= len(value) {
		return 0, false
	}
	if value[cursor] == '"' {
		cursor++
		for cursor < len(value) {
			switch character := value[cursor]; {
			case character == '"':
				return templateTokenByte(value, cursor+1, ']')
			case character == '\\':
				if cursor+1 >= len(value) || (value[cursor+1] != '"' && value[cursor+1] != '\\') {
					return 0, false
				}
				cursor += 2
			case character < 0x20 || character == 0x7f:
				return 0, false
			default:
				cursor++
			}
		}
		return 0, false
	}
	start := cursor
	if cursor < len(value) && value[cursor] == '-' {
		cursor++
	}
	for cursor < len(value) && value[cursor] >= '0' && value[cursor] <= '9' {
		cursor++
	}
	if cursor == start || (cursor == start+1 && value[start] == '-') {
		return 0, false
	}
	return templateTokenByte(value, cursor, ']')
}

// templateTokenElision consumes the authored actual-list elision.
func templateTokenElision(value string, cursor int) (int, bool) {
	const elision = "(...)"
	if len(value)-cursor < len(elision) || value[cursor:cursor+len(elision)] != elision {
		return 0, false
	}
	return cursor + len(elision), true
}

// templateTokenByte consumes one expected byte at cursor.
func templateTokenByte(value string, cursor int, expected byte) (int, bool) {
	if cursor >= len(value) || value[cursor] != expected {
		return 0, false
	}
	return cursor + 1, true
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
func (name diagnosticSemanticName) Valid() bool { return name.valid() }
func (target diagnosticTargetType) Valid() bool { return target.valid() }

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

// diagnosticObservedSpelling is the spelling of the value a finding measured.
// It is closed the same way the claim vocabulary is: a producer names either
// one exact scalar constant or the runtime families the value may carry, and
// this file renders it. A producer cannot hand in a rendered fragment of its
// own, so the observed half of a message stays as declarative as the declared
// half.
type diagnosticObservedSpelling struct{ value string }

func (spelling diagnosticObservedSpelling) valid() bool { return spelling.value != "" }

func (spelling diagnosticObservedSpelling) text() string { return spelling.value }

// ObservedLiteral is the spelling of a proved scalar constant. The rendering
// is Lua's own: a string constant renders quoted, a float renders in the
// shortest form that reads back, and an integer and a boolean render as
// themselves.
func ObservedLiteral(literal keyspace.LiteralValue) (diagnosticObservedSpelling, bool) {
	switch literal.Kind {
	case keyspace.LiteralBool:
		return diagnosticObservedSpelling{value: strconv.FormatBool(literal.Bool)}, true
	case keyspace.LiteralInteger:
		return diagnosticObservedSpelling{value: strconv.FormatInt(literal.Integer, 10)}, true
	case keyspace.LiteralFloat:
		return diagnosticObservedSpelling{value: strconv.FormatFloat(math.Float64frombits(literal.FloatBits), 'g', -1, 64)}, true
	case keyspace.LiteralString:
		return diagnosticObservedSpelling{value: strconv.Quote(literal.String)}, true
	default:
		return diagnosticObservedSpelling{}, false
	}
}

// ObservedNil is the spelling of the one value that has no literal of its own.
func ObservedNil() diagnosticObservedSpelling {
	return diagnosticObservedSpelling{value: "nil"}
}

// ObservedFamilies is the spelling of a value that narrowed to families rather
// than to a constant. The names are the declared spellings of the runtime
// family vocabulary, read from the sealed structural table, so this file holds
// no second list of them.
func ObservedFamilies(vocabulary structure.Table, kinds runtimekind.Set) (diagnosticObservedSpelling, bool) {
	if !kinds.Valid() || kinds == 0 {
		return diagnosticObservedSpelling{}, false
	}
	var rendered strings.Builder
	for index := 0; index < kinds.Members(); index++ {
		kind, kindOK := kinds.MemberAt(index)
		if !kindOK {
			return diagnosticObservedSpelling{}, false
		}
		member, memberOK := vocabulary.At(structure.CategoryRuntimeKind, uint16(kind))
		if !memberOK || member.Spelling() == "" {
			return diagnosticObservedSpelling{}, false
		}
		if index > 0 {
			rendered.WriteString(" or ")
		}
		rendered.WriteString(member.Spelling())
	}
	spelling := diagnosticObservedSpelling{value: rendered.String()}
	return spelling, spelling.valid()
}

// DiagnosticPolicy is opt-in. A zero policy never collects semantic reports.
type DiagnosticPolicy struct {
	Enabled  []DiagnosticCode
	Severity map[DiagnosticCode]FindingSeverity
}

func (policy DiagnosticPolicy) EnabledFor(table schemadiag.Table, code DiagnosticCode) (FindingSeverity, bool) {
	return policy.enabled(table, code)
}

func (policy DiagnosticPolicy) enabled(table schemadiag.Table, code DiagnosticCode) (FindingSeverity, bool) {
	for _, candidate := range policy.Enabled {
		if candidate != code {
			continue
		}
		entry, entryOK := diagnosticDeclaration(table, code)
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
func diagnosticCollectable(table schemadiag.Table, code DiagnosticCode) bool {
	entry, ok := diagnosticDeclaration(table, code)
	return ok && entry.Collectable()
}

// Valid rejects ambiguous policy authority before a solve starts: every code
// is known and unique, and overrides may only refine an enabled known code.
func (policy DiagnosticPolicy) Valid(table schemadiag.Table) bool {
	if !table.Available() {
		return false
	}
	enabled := make(map[DiagnosticCode]struct{}, len(policy.Enabled))
	for _, code := range policy.Enabled {
		if !diagnosticCollectable(table, code) {
			return false
		}
		if _, duplicate := enabled[code]; duplicate {
			return false
		}
		enabled[code] = struct{}{}
	}
	for code, severity := range policy.Severity {
		if !diagnosticCollectable(table, code) || !severity.Available() {
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
	// verdict is the declared answer this finding renders under. A row with one
	// presentation of its own answers under none; a row whose collector answers
	// in a declared vocabulary carries the member it answered with, and the
	// declaration decides every rendered byte from it.
	verdict  uint16
	severity FindingSeverity
	location DiagnosticLocation
	data     diagnosticTemplateData
}

// diagnosticTemplateData is the complete typed payload a declaration may use.
// It is intentionally private and row-shaped: an upstream producer supplies
// semantic names, a target type, a claim form, and an already-authenticated
// proof anchor; no producer can inject pre-rendered message/evidence prose.
type diagnosticNameList struct{ names []string }

func (list diagnosticNameList) valid() bool {
	if len(list.names) == 0 {
		return false
	}
	for _, name := range list.names {
		if !diagnosticTemplateTokenValid(name) {
			return false
		}
	}
	return true
}

func (list diagnosticNameList) text() string {
	if !list.valid() {
		return ""
	}
	var rendered strings.Builder
	for index, name := range list.names {
		if index > 0 {
			rendered.WriteString(", ")
		}
		rendered.WriteByte('`')
		rendered.WriteString(name)
		rendered.WriteByte('`')
	}
	return rendered.String()
}

type diagnosticTemplateData struct {
	subject diagnosticSemanticName
	target  diagnosticTargetType
	claim   diagnosticClaimForm
	// witnesses is the located roster a row's non-primary anchors read, in the
	// order the row declares them. An anchor names one by its one-based ordinal,
	// so a finding whose proof spans several places carries every one of them.
	witnesses []DiagnosticLocation
	handled   diagnosticNameList
	missing   diagnosticNameList
	actual    diagnosticObservedSpelling
	member    diagnosticSemanticName
}

// witnessAt resolves one declared witness ordinal.
func (data diagnosticTemplateData) witnessAt(witness uint8) (DiagnosticLocation, bool) {
	if witness == 0 || int(witness) > len(data.witnesses) {
		return DiagnosticLocation{}, false
	}
	location := data.witnesses[witness-1]
	return location, location.Available()
}

// validFor states the payload contract of one declaration: exactly the fields
// the presentation this finding renders from requires are supplied, and
// nothing else is. A row that renders per verdict states that contract per
// answer, so the payload one answer names is never owed by another's producer.
func (data diagnosticTemplateData) ValidFor(entry *schemadiag.Entry, verdict uint16) bool {
	return data.validFor(entry, verdict)
}

func (data diagnosticTemplateData) validFor(entry *schemadiag.Entry, verdict uint16) bool {
	presentation, presentationOK := entry.Presentation(verdict)
	if !presentationOK {
		return false
	}
	requirements := presentation.Requirements
	if !payloadFieldValid(requirements&schemadiag.RequiresSubject != 0, data.subject.valid(), data.subject == diagnosticSemanticName{}) {
		return false
	}
	if !payloadFieldValid(requirements&schemadiag.RequiresTarget != 0, data.target.valid(), data.target == diagnosticTargetType{}) {
		return false
	}
	if !payloadFieldValid(requirements&schemadiag.RequiresClaimForm != 0, data.claim.valid(), data.claim == diagnosticClaimFormInvalid) {
		return false
	}
	if !payloadFieldValid(requirements&schemadiag.RequiresHandled != 0, data.handled.valid(), len(data.handled.names) == 0) {
		return false
	}
	if !payloadFieldValid(requirements&schemadiag.RequiresMissing != 0, data.missing.valid(), len(data.missing.names) == 0) {
		return false
	}
	if !payloadFieldValid(requirements&schemadiag.RequiresActual != 0, data.actual.valid(), data.actual == diagnosticObservedSpelling{}) {
		return false
	}
	if !payloadFieldValid(requirements&schemadiag.RequiresMember != 0, data.member.valid(), data.member == diagnosticSemanticName{}) {
		return false
	}
	return data.witnessRosterValid(requirements&schemadiag.RequiresWitness != 0, entry.Witnesses())
}

// witnessRosterValid states the located-witness half of the payload contract:
// a row that reads witnesses is supplied exactly the count it declares, each
// one locatable, and a row that reads none is supplied none.
func (data diagnosticTemplateData) witnessRosterValid(required bool, declared uint8) bool {
	if !required {
		return len(data.witnesses) == 0
	}
	if declared == 0 || len(data.witnesses) != int(declared) {
		return false
	}
	for _, location := range data.witnesses {
		if !location.Available() {
			return false
		}
	}
	return true
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

func (data diagnosticTemplateData) location(anchor schemadiag.Anchor, witness uint8, primary DiagnosticLocation) (DiagnosticLocation, bool) {
	switch anchor {
	case schemadiag.AnchorPrimary:
		return primary, primary.Available()
	case schemadiag.AnchorWitness:
		return data.witnessAt(witness)
	default:
		return DiagnosticLocation{}, false
	}
}

// renderDiagnosticLine substitutes one already-parsed declaration line. The
// surface proved at seal that every segment names a payload field this row
// requires, so rendering is a walk with no parsing and no unresolved read.
func renderDiagnosticLine(line schemadiag.Line, data diagnosticTemplateData) string {
	var rendered strings.Builder
	for index := 0; index < line.Count(); index++ {
		segment, ok := line.At(index)
		if !ok {
			return ""
		}
		switch segment.Placeholder {
		case schemadiag.PlaceholderInvalid:
			rendered.WriteString(segment.Literal)
		case schemadiag.PlaceholderSubject:
			rendered.WriteString(data.subject.value)
		case schemadiag.PlaceholderQuotedSubject:
			rendered.WriteString(strconv.Quote(data.subject.value))
		case schemadiag.PlaceholderTarget:
			rendered.WriteString(data.target.value)
		case schemadiag.PlaceholderClaimForm:
			rendered.WriteString(data.claim.text())
		case schemadiag.PlaceholderHandled:
			rendered.WriteString(data.handled.text())
		case schemadiag.PlaceholderMissing:
			rendered.WriteString(data.missing.text())
		case schemadiag.PlaceholderActual:
			rendered.WriteString(data.actual.text())
		case schemadiag.PlaceholderMember:
			rendered.WriteString(data.member.value)
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
	compilation       composite.Compilation
	vocabulary        structure.Table
	declarations      schemadiag.Table
	collections       composite.DiagnosticCollections
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
	if report == nil || !report.sealed || !report.source.Available() || !report.result.Available() || !report.declarations.Available() || !report.collections.Available() {
		return false
	}
	for _, finding := range report.findings {
		if !validDiagnosticFinding(report, finding) {
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
	return row, validDiagnosticFinding(finding.owner, row)
}

func validDiagnosticFinding(owner *DiagnosticReport, row diagnosticFinding) bool {
	entry, entryOK := owner.declarations.ForCode(row.code)
	if owner == nil || !row.id.Available() || !row.subject.Available() || !entryOK || !row.severity.Available() || !row.location.Available() || !row.data.validFor(entry, row.verdict) {
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

// declaration resolves the sealed presentation this finding publishes from:
// the row its code names, under the answer its verdict selects.
func (finding Finding) declaration() (diagnosticFinding, schemadiag.Presentation, bool) {
	row, rowOK := finding.row()
	if !rowOK {
		return diagnosticFinding{}, schemadiag.Presentation{}, false
	}
	entry, entryOK := finding.owner.declarations.ForCode(row.code)
	if !entryOK {
		return diagnosticFinding{}, schemadiag.Presentation{}, false
	}
	presentation, presentationOK := entry.Presentation(row.verdict)
	return row, presentation, presentationOK
}
func (finding Finding) EvidenceCount() int {
	_, presentation, ok := finding.declaration()
	if !ok {
		return 0
	}
	return len(presentation.Evidence)
}
func (finding Finding) EvidenceAt(index int) (DiagnosticEvidence, bool) {
	row, presentation, ok := finding.declaration()
	if !ok || index < 0 || index >= len(presentation.Evidence) {
		return DiagnosticEvidence{}, false
	}
	declared := presentation.Evidence[index]
	location, locationOK := row.data.location(declared.Anchor, declared.Witness, row.location)
	detail := renderDiagnosticLine(declared.Detail, row.data)
	evidence := DiagnosticEvidence{location: location, kind: declared.Kind, trust: declared.Trust, reason: declared.Reason, detail: detail}
	return evidence, locationOK && evidence.Available()
}
func (finding Finding) LabelCount() int {
	_, presentation, ok := finding.declaration()
	if !ok {
		return 0
	}
	return len(presentation.Labels)
}
func (finding Finding) LabelAt(index int) (DiagnosticLabel, bool) {
	row, presentation, ok := finding.declaration()
	if !ok || index < 0 || index >= len(presentation.Labels) {
		return DiagnosticLabel{}, false
	}
	declared := presentation.Labels[index]
	location, locationOK := row.data.location(declared.Anchor, declared.Witness, row.location)
	label := DiagnosticLabel{location: location, text: renderDiagnosticLine(declared.Text, row.data)}
	return label, locationOK && label.Available()
}
func (finding Finding) Message() string {
	row, presentation, ok := finding.declaration()
	if !ok {
		return ""
	}
	return renderDiagnosticLine(presentation.Message, row.data)
}
func (finding Finding) Help() string {
	row, presentation, ok := finding.declaration()
	if !ok {
		return ""
	}
	return renderDiagnosticLine(presentation.Help, row.data)
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
	if _, ok := sourceLine(sourceText, row.location.startLine); !ok {
		return "", false
	}
	return finding.render(row, sourceText, true), true
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

func (finding Finding) render(row diagnosticFinding, sourceText string, includeSource bool) string {
	line, column := row.location.Start()
	entry, entryOK := finding.owner.declarations.ForCode(row.code)
	severity, severityOK := findingSeveritySpelling(finding.owner.vocabulary, row.severity)
	if !entryOK || !severityOK {
		return ""
	}
	presentation, presentationOK := entry.Presentation(row.verdict)
	if !presentationOK {
		return ""
	}
	var rendered strings.Builder
	for position := 0; position < entry.RenderCount(); position++ {
		section, sectionOK := entry.RenderAt(position)
		if !sectionOK {
			return ""
		}
		switch section {
		case schemadiag.SectionSummary:
			fmt.Fprintf(&rendered, "%s[%s]: %s\n", severity, row.code, renderDiagnosticLine(presentation.Message, row.data))
		case schemadiag.SectionLocation:
			fmt.Fprintf(&rendered, "--> %s:%d:%d\n", row.location.File(), line, column)
		case schemadiag.SectionSource:
			if includeSource {
				renderSourceLine(&rendered, sourceText, line)
			}
		case schemadiag.SectionEvidence:
			if len(presentation.Evidence) == 0 {
				continue
			}
			rendered.WriteString("because:\n")
			for index := 0; index < len(presentation.Evidence); index++ {
				evidence, evidenceOK := finding.EvidenceAt(index)
				if !evidenceOK {
					return ""
				}
				fmt.Fprintf(&rendered, "%d. %s: %s\n", index+1, evidenceRenderLabel(evidence), evidence.Detail())
				// A proof line established somewhere other than the reported site
				// names that place. Repeating the finding's own location under
				// every line would say nothing, so a line anchored at the primary
				// location renders exactly as it always has.
				evidenceLocation, evidenceLocationOK := evidence.Location()
				if !evidenceLocationOK {
					return ""
				}
				renderElsewhere(&rendered, evidenceLocation, row.location, sourceText, includeSource)
			}
		case schemadiag.SectionContext:
			location, locationOK := row.data.witnessAt(entry.Context())
			if !locationOK {
				return ""
			}
			rendered.WriteString("where:\n")
			renderLocated(&rendered, location, sourceText, includeSource)
		case schemadiag.SectionHelp:
			if help := renderDiagnosticLine(presentation.Help, row.data); help != "" {
				fmt.Fprintf(&rendered, "help: %s\n", help)
			}
		}
	}
	return strings.TrimSuffix(rendered.String(), "\n")
}

// renderElsewhere shows one located line only when it is somewhere other than
// the finding's own reported position.
func renderElsewhere(rendered *strings.Builder, location, primary DiagnosticLocation, sourceText string, includeSource bool) {
	if !location.Available() || location == primary {
		return
	}
	renderLocated(rendered, location, sourceText, includeSource)
}

// renderLocated shows one place: its coordinates, and the source line at those
// coordinates when the caller supplied the text they belong to.
func renderLocated(rendered *strings.Builder, location DiagnosticLocation, sourceText string, includeSource bool) {
	if !location.Available() {
		return
	}
	line, column := location.Start()
	fmt.Fprintf(rendered, "--> %s:%d:%d\n", location.File(), line, column)
	if includeSource {
		renderSourceLine(rendered, sourceText, line)
	}
}

// renderSourceLine shows the numbered source line at one position. A position
// the caller's text does not hold shows nothing rather than a blank frame.
func renderSourceLine(rendered *strings.Builder, sourceText string, line uint32) {
	text, ok := sourceLine(sourceText, line)
	if !ok {
		return
	}
	fmt.Fprintf(rendered, "%d | %s\n", line, strings.TrimSpace(text))
}

// NewReport opens one sealed collector for a completed Result.
func NewReport(source, result identity.ContentID, compilation composite.Compilation, vocabulary structure.Table, declarations schemadiag.Table, collections composite.DiagnosticCollections) *DiagnosticReport {
	return &DiagnosticReport{source: source, result: result, compilation: compilation, vocabulary: vocabulary, declarations: declarations, collections: collections, findings: make([]diagnosticFinding, 0), sealed: true}
}

// SetCollectionFailure records a collector refusal. An empty report stays
// sealed so callers can distinguish a clean zero from a failed collection.
func (report *DiagnosticReport) SetCollectionFailure(failure DiagnosticCollectionFailure) {
	if report == nil {
		return
	}
	report.collectionFailure = failure
}

// AppendFinding records one already-authenticated row.
func (report *DiagnosticReport) AppendFinding(finding FindingRow) {
	if report == nil {
		return
	}
	report.findings = append(report.findings, diagnosticFinding(finding))
}

// SortFindingsByID orders rows by finding identity.
func (report *DiagnosticReport) SortFindingsByID() {
	if report == nil {
		return
	}
	sort.Slice(report.findings, func(i, j int) bool {
		return bytes.Compare(report.findings[i].id[:], report.findings[j].id[:]) < 0
	})
}

// FindingRow is the collector-facing finding payload.
type FindingRow diagnosticFinding

func NewLocation(file string, startLine, startColumn, endLine, endColumn uint32) (DiagnosticLocation, bool) {
	return newDiagnosticLocation(file, startLine, startColumn, endLine, endColumn)
}

func NewSemanticName(value string) (diagnosticSemanticName, bool) {
	return newDiagnosticSemanticName(value)
}

func NewTargetType(value string) (diagnosticTargetType, bool) {
	return newDiagnosticTargetType(value)
}

func EmptyTemplateData() diagnosticTemplateData { return diagnosticTemplateData{} }

func EmptyName() diagnosticSemanticName { return diagnosticSemanticName{} }

func EmptyTarget() diagnosticTargetType { return diagnosticTargetType{} }

func NewTemplateData(subject diagnosticSemanticName, target diagnosticTargetType, claim diagnosticClaimForm, witnesses ...DiagnosticLocation) diagnosticTemplateData {
	return diagnosticTemplateData{subject: subject, target: target, claim: claim, witnesses: append([]DiagnosticLocation(nil), witnesses...)}
}

func NewNameList(names []string) (diagnosticNameList, bool) {
	list := diagnosticNameList{names: append([]string(nil), names...)}
	return list, list.valid()
}

func NewCaseTemplateData(subject diagnosticSemanticName, handled, missing diagnosticNameList) diagnosticTemplateData {
	return diagnosticTemplateData{subject: subject, handled: handled, missing: missing}
}

func evidenceRenderLabel(evidence DiagnosticEvidence) string {
	if evidence.Kind() == "missing proof" {
		return evidence.Kind()
	}
	return evidence.Trust()
}

func TypeCastClaim() diagnosticClaimForm { return diagnosticClaimFormTypeCastCall }

func TypeClaim() diagnosticClaimForm { return diagnosticClaimFormTypeClaim }

func NewFindingRow(id, subject identity.ContentID, code DiagnosticCode, severity FindingSeverity, location DiagnosticLocation, data diagnosticTemplateData) FindingRow {
	return FindingRow{id: id, subject: subject, code: code, severity: severity, location: location, data: data}
}

// NewVerdictFindingRow is one finding of a row whose presentation is per
// verdict. The verdict is the declared answer the collector reached, and it
// selects the presentation rather than contributing to it.
func NewVerdictFindingRow(id, subject identity.ContentID, code DiagnosticCode, verdict uint16, severity FindingSeverity, location DiagnosticLocation, data diagnosticTemplateData) FindingRow {
	return FindingRow{id: id, subject: subject, code: code, verdict: verdict, severity: severity, location: location, data: data}
}

// NewConformanceTemplateData is the payload one conformance finding renders
// from: the subject the finding names, the declared type it was measured
// against, the spelling of what was observed, and the member an absent-member
// answer names. A field an answer does not read is left absent, which is the
// same contract every other payload is admitted under.
func NewConformanceTemplateData(subject diagnosticSemanticName, target diagnosticTargetType, actual diagnosticObservedSpelling, member diagnosticSemanticName) diagnosticTemplateData {
	return diagnosticTemplateData{subject: subject, target: target, actual: actual, member: member}
}

// EmptyObservedSpelling is the absent observed spelling.
func EmptyObservedSpelling() diagnosticObservedSpelling { return diagnosticObservedSpelling{} }

func (row FindingRow) ID() identity.ContentID { return row.id }
