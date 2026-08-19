package artifact

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func (payload diagnosticBranchConditionRow) DecisionPathID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.decision
}

func (payload diagnosticBranchConditionRow) ValueSpanID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.value
}

func (payload diagnosticBranchConditionRow) EvidencePointCount() int {
	if !payload.available() {
		return 0
	}
	return len(payload.points)
}

func (payload diagnosticBranchConditionRow) EvidencePoints() ([]identity.ContentID, bool) {
	if !payload.available() {
		return nil, false
	}
	return append([]identity.ContentID(nil), payload.points...), true
}

func (payload diagnosticBranchConditionRow) EvidencePointAt(index int) (identity.ContentID, bool) {
	if !payload.available() || index < 0 || index >= len(payload.points) {
		return identity.ContentID{}, false
	}
	return payload.points[index], true
}

func (payload diagnosticUnresolvedTypeReferenceRow) StaticReferenceID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.reference
}

func (payload diagnosticUnresolvedTypeReferenceRow) RootID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.root
}

func (payload diagnosticUnresolvedTypeReferenceRow) Path() ([]string, bool) {
	if !payload.available() {
		return nil, false
	}
	return append([]string(nil), payload.path...), true
}

func (payload diagnosticUnresolvedTypeReferenceRow) Name() (string, bool) {
	if !payload.available() {
		return "", false
	}
	return strings.Join(payload.path, "."), true
}

func (payload diagnosticUnresolvedValueReferenceRow) ReadID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.read
}

func (payload diagnosticUnresolvedValueReferenceRow) CellID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.cell
}

func (payload diagnosticUnresolvedValueReferenceRow) Name() (string, bool) {
	return payload.name, payload.available()
}

func (row DiagnosticObservationRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row DiagnosticObservationRow) Kind() structure.DiagnosticObservationKind {
	if !row.Available() {
		return structure.DiagnosticObservationInvalid
	}
	return row.kind
}

func (row DiagnosticObservationRow) Location() (programsource.Span, bool) {
	return row.location, row.Available() && validDiagnosticSpan(row.location)
}

func (row DiagnosticObservationRow) BranchCondition() (diagnosticBranchConditionRow, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationBranchCondition {
		return diagnosticBranchConditionRow{}, false
	}
	return diagnosticBranchConditionRow{decision: row.branch.decision, value: row.branch.value, points: append([]identity.ContentID(nil), row.branch.points...)}, true
}

func (row DiagnosticObservationRow) UnresolvedTypeReference() (diagnosticUnresolvedTypeReferenceRow, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeReferenceUnresolved {
		return diagnosticUnresolvedTypeReferenceRow{}, false
	}
	return diagnosticUnresolvedTypeReferenceRow{reference: row.unresolved.reference, root: row.unresolved.root, path: append([]string(nil), row.unresolved.path...)}, true
}

func (row DiagnosticObservationRow) UnresolvedValueReference() (diagnosticUnresolvedValueReferenceRow, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationValueReferenceUnresolved {
		return diagnosticUnresolvedValueReferenceRow{}, false
	}
	return row.value, true
}

func (row DiagnosticObservationRow) TypeConformance() (diagnosticTypeConformanceRow, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance {
		return diagnosticTypeConformanceRow{}, false
	}
	payload := row.conformance
	payload.points = append([]identity.ContentID(nil), row.conformance.points...)
	return payload, true
}

func (payload diagnosticTypeConformanceRow) Site() schemadiag.Site {
	if !payload.available() {
		return schemadiag.SiteNone
	}
	switch payload.site {
	case diagnosticTypeConformanceSiteCallArgument:
		return schemadiag.SiteCallArgument
	case diagnosticTypeConformanceSiteAssignment:
		return schemadiag.SiteAssignment
	default:
		return schemadiag.SiteNone
	}
}

func (payload diagnosticTypeConformanceRow) CallID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.call
}

func (payload diagnosticTypeConformanceRow) ArgumentID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.argument
}

func (payload diagnosticTypeConformanceRow) DeclaredStaticTypeID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.declared
}

func (payload diagnosticTypeConformanceRow) SpanID() identity.ContentID {
	if !payload.available() {
		return identity.ContentID{}
	}
	return payload.span
}

func (payload diagnosticTypeConformanceRow) Position() (uint32, bool) {
	return payload.position, payload.available()
}

func (payload diagnosticTypeConformanceRow) EvidencePoints() ([]identity.ContentID, bool) {
	if !payload.available() {
		return nil, false
	}
	return append([]identity.ContentID(nil), payload.points...), true
}
