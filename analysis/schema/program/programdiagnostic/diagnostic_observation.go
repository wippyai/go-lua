package programdiagnostic

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// DiagnosticObservationSite is the neutral site vocabulary carried by a
// type-conformance observation. Its values are the historical diagnostic
// payload ordinals; the diagnostic declaration table projects them at the
// consumer boundary.
type DiagnosticObservationSite uint8

const (
	DiagnosticObservationSiteInvalid DiagnosticObservationSite = iota
	DiagnosticObservationSiteCallArgument
	DiagnosticObservationSiteAssignment
	// DiagnosticObservationSiteMember measures one established constructor
	// member. The measured value is the member's own value row and the
	// declaration is the member's own node in the declared graph, so the
	// subject differs from an assignment only in where it sits inside a value.
	DiagnosticObservationSiteMember
	// DiagnosticObservationSiteMemberAbsent names one required declared field
	// the constructor's established key set does not supply. An absence has no
	// value of its own, so the row measures the allocation's own value against
	// the missing field's node.
	DiagnosticObservationSiteMemberAbsent
	diagnosticObservationSiteLimit
)

func (site DiagnosticObservationSite) Valid() bool {
	return site > DiagnosticObservationSiteInvalid && site < diagnosticObservationSiteLimit
}

// Structural reports whether the site measures inside a constructor rather
// than at the whole value a statement binds or passes. A structural site
// carries a member-granular declaration: its declared node is the field,
// element, or map-value node, never the node of the enclosing declaration.
func (site DiagnosticObservationSite) Structural() bool {
	return site == DiagnosticObservationSiteMember || site == DiagnosticObservationSiteMemberAbsent
}

// DiagnosticObservation is one immutable tagged observation. The payload is
// a closed scalar union. Evidence and unresolved-type path members are named
// by dense child spans rather than retained slices.
type DiagnosticObservation struct {
	id       identity.ContentID
	kind     structure.DiagnosticObservationKind
	location programsource.Span

	evidenceOffset uint32
	evidenceCount  uint32
	pathOffset     uint32
	pathCount      uint32

	decision  identity.ContentID
	value     identity.ContentID
	reference identity.ContentID
	root      identity.ContentID
	read      identity.ContentID
	cell      identity.ContentID
	name      string

	site     DiagnosticObservationSite
	owner    identity.ContentID
	measured identity.ContentID
	declared identity.ContentID
	span     identity.ContentID
	position uint32
	callee   string
}

// NewDiagnosticObservationBranchCondition constructs the branch-condition
// variant and its owned evidence span. The variant constructor only accepts
// the scalars that belong to this tag, so another payload family cannot be
// threaded into the row by accident.
func NewDiagnosticObservationBranchCondition(
	id identity.ContentID,
	location programsource.Span,
	evidenceOffset, evidenceCount uint32,
	decision, value identity.ContentID,
) (DiagnosticObservation, bool) {
	row := DiagnosticObservation{
		id: id, kind: structure.DiagnosticObservationBranchCondition, location: location,
		evidenceOffset: evidenceOffset, evidenceCount: evidenceCount,
		decision: decision, value: value,
	}
	return row, row.Available()
}

// NewDiagnosticObservationTypeReferenceUnresolved constructs the unresolved
// type-reference variant and its owned lexical path span.
func NewDiagnosticObservationTypeReferenceUnresolved(
	id identity.ContentID,
	location programsource.Span,
	pathOffset, pathCount uint32,
	reference, root identity.ContentID,
) (DiagnosticObservation, bool) {
	row := DiagnosticObservation{
		id: id, kind: structure.DiagnosticObservationTypeReferenceUnresolved, location: location,
		pathOffset: pathOffset, pathCount: pathCount,
		reference: reference, root: root,
	}
	return row, row.Available()
}

// NewDiagnosticObservationValueReferenceUnresolved constructs the unresolved
// value-reference variant from its read, cell, and source name scalars.
func NewDiagnosticObservationValueReferenceUnresolved(
	id identity.ContentID,
	location programsource.Span,
	read, cell identity.ContentID,
	name string,
) (DiagnosticObservation, bool) {
	row := DiagnosticObservation{
		id: id, kind: structure.DiagnosticObservationValueReferenceUnresolved, location: location,
		read: read, cell: cell, name: name,
	}
	return row, row.Available()
}

// NewDiagnosticObservationTypeConformance constructs the type-conformance
// variant and its owned evidence span.
func NewDiagnosticObservationTypeConformance(
	id identity.ContentID,
	location programsource.Span,
	evidenceOffset, evidenceCount uint32,
	site DiagnosticObservationSite,
	owner, measured, declared, span identity.ContentID,
	position uint32,
	subject string,
	callee ...string,
) (DiagnosticObservation, bool) {
	calleeName := ""
	if len(callee) == 1 {
		calleeName = callee[0]
	} else if len(callee) > 1 {
		return DiagnosticObservation{}, false
	}
	row := DiagnosticObservation{
		id: id, kind: structure.DiagnosticObservationTypeConformance, location: location,
		evidenceOffset: evidenceOffset, evidenceCount: evidenceCount,
		site: site, owner: owner, measured: measured, declared: declared,
		span: span, position: position, name: subject, callee: calleeName,
	}
	return row, row.Available()
}

// ValidSpan reports whether a Source-owned span carries the coordinates
// required by a diagnostic observation. Source owns coordinate ordering; the
// diagnostic plane additionally requires a file and a nonzero start.
func ValidSpan(span programsource.Span) bool {
	if span.File == "" || span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	_, ok := programsource.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	return ok
}

func (row DiagnosticObservation) Available() bool {
	if !row.id.Available() || !row.kind.Available() || !ValidSpan(row.location) {
		return false
	}
	if uint64(row.evidenceOffset)+uint64(row.evidenceCount) > uint64(^uint32(0)) ||
		uint64(row.pathOffset)+uint64(row.pathCount) > uint64(^uint32(0)) {
		return false
	}
	emptyEvidence := row.evidenceCount == 0
	emptyPath := row.pathCount == 0
	zero := identity.ContentID{}
	switch row.kind {
	case structure.DiagnosticObservationBranchCondition:
		return row.decision.Available() && row.value.Available() && !emptyEvidence && emptyPath &&
			row.reference == zero && row.root == zero && row.read == zero && row.cell == zero && row.name == "" &&
			row.site == DiagnosticObservationSiteInvalid && row.owner == zero && row.measured == zero && row.declared == zero && row.span == zero && row.position == 0
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		rootShape := (row.pathCount == 1 && row.root == zero) || (row.pathCount > 1 && row.root.Available())
		return row.reference.Available() && !emptyPath && rootShape && emptyEvidence && row.decision == zero && row.value == zero &&
			row.read == zero && row.cell == zero && row.name == "" && row.site == DiagnosticObservationSiteInvalid &&
			row.owner == zero && row.measured == zero && row.declared == zero && row.span == zero && row.position == 0
	case structure.DiagnosticObservationValueReferenceUnresolved:
		return row.read.Available() && row.cell.Available() && row.name != "" && emptyEvidence && emptyPath &&
			row.decision == zero && row.value == zero && row.reference == zero && row.root == zero && row.site == DiagnosticObservationSiteInvalid &&
			row.owner == zero && row.measured == zero && row.declared == zero && row.span == zero && row.position == 0
	case structure.DiagnosticObservationTypeConformance:
		return row.site.Valid() && row.owner.Available() && row.measured.Available() && row.declared.Available() && row.span.Available() &&
			!emptyEvidence && emptyPath && row.decision == zero && row.value == zero && row.reference == zero && row.root == zero &&
			row.read == zero && row.cell == zero &&
			(row.site != DiagnosticObservationSiteCallArgument || row.callee != "")
	default:
		return false
	}
}

func (row DiagnosticObservation) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row DiagnosticObservation) Kind() structure.DiagnosticObservationKind {
	if !row.Available() {
		return structure.DiagnosticObservationInvalid
	}
	return row.kind
}

func (row DiagnosticObservation) Location() (programsource.Span, bool) {
	return row.location, row.Available()
}

func (row DiagnosticObservation) EvidenceSpan() (offset, count uint32, ok bool) {
	return row.evidenceOffset, row.evidenceCount, row.Available()
}

func (row DiagnosticObservation) PathSpan() (offset, count uint32, ok bool) {
	return row.pathOffset, row.pathCount, row.Available()
}

func (row DiagnosticObservation) DecisionPathID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationBranchCondition {
		return identity.ContentID{}
	}
	return row.decision
}

func (row DiagnosticObservation) ValueSpanID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationBranchCondition {
		return identity.ContentID{}
	}
	return row.value
}

func (row DiagnosticObservation) StaticReferenceID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeReferenceUnresolved {
		return identity.ContentID{}
	}
	return row.reference
}

func (row DiagnosticObservation) RootID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeReferenceUnresolved {
		return identity.ContentID{}
	}
	return row.root
}

func (row DiagnosticObservation) ReadID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationValueReferenceUnresolved {
		return identity.ContentID{}
	}
	return row.read
}

func (row DiagnosticObservation) CellID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationValueReferenceUnresolved {
		return identity.ContentID{}
	}
	return row.cell
}

// Name is the authored spelling of the row's subject: the global identifier an
// unresolved read named, or the authored access path a conformance site
// measured. A conformance site whose subject the authored projection does not
// spell carries none, so the column is required on the first population and
// optional on the second.
func (row DiagnosticObservation) Name() string {
	if !row.Available() {
		return ""
	}
	switch row.kind {
	case structure.DiagnosticObservationValueReferenceUnresolved, structure.DiagnosticObservationTypeConformance:
		return row.name
	default:
		return ""
	}
}

// CalleeName is the authored spelling of the direct-call target that owns a
// call-argument conformance row. It is issued with the row while the compiler
// still has the authored call relation; Snapshot consumers do not reconstruct
// it from source or result data.
func (row DiagnosticObservation) CalleeName() string {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance || row.site != DiagnosticObservationSiteCallArgument {
		return ""
	}
	return row.callee
}

func (row DiagnosticObservation) Site() DiagnosticObservationSite {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance {
		return DiagnosticObservationSiteInvalid
	}
	return row.site
}

func (row DiagnosticObservation) OwnerID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance {
		return identity.ContentID{}
	}
	return row.owner
}

func (row DiagnosticObservation) MeasuredValueID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance {
		return identity.ContentID{}
	}
	return row.measured
}

func (row DiagnosticObservation) DeclaredStaticTypeID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance {
		return identity.ContentID{}
	}
	return row.declared
}

func (row DiagnosticObservation) SpanID() identity.ContentID {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance {
		return identity.ContentID{}
	}
	return row.span
}

func (row DiagnosticObservation) Position() (uint32, bool) {
	return row.position, row.Available() && row.kind == structure.DiagnosticObservationTypeConformance
}

// DiagnosticEvidence is one ordered evidence-point child.
type DiagnosticEvidence struct{ point identity.ContentID }

func NewDiagnosticEvidence(point identity.ContentID) (DiagnosticEvidence, bool) {
	row := DiagnosticEvidence{point: point}
	return row, row.Available()
}

func (row DiagnosticEvidence) Available() bool { return row.point.Available() }

func (row DiagnosticEvidence) PointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}

// DiagnosticPath is one ordered lexical component of an unresolved type
// reference. The parent path span is its complete path, in source order.
type DiagnosticPath struct{ component string }

func NewDiagnosticPath(component string) (DiagnosticPath, bool) {
	row := DiagnosticPath{component: component}
	return row, row.Available()
}

func (row DiagnosticPath) Available() bool { return row.component != "" }

func (row DiagnosticPath) Component() string {
	if !row.Available() {
		return ""
	}
	return row.component
}

// DiagnosticPathName joins one published path span at the read site. The
// name is derived from the canonical child sequence rather than retained as a
// second parent authority.
func DiagnosticPathName(path []DiagnosticPath) (string, bool) {
	if len(path) == 0 {
		return "", false
	}
	parts := make([]string, len(path))
	for index, row := range path {
		if !row.Available() {
			return "", false
		}
		parts[index] = row.component
	}
	return strings.Join(parts, "."), true
}
