package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func (kind DiagnosticObservationKind) valid() bool {
	return kind == DiagnosticObservationBranchCondition || kind == DiagnosticObservationTypeReferenceUnresolved || kind == DiagnosticObservationValueReferenceUnresolved
}

// validDiagnosticSpan is the artifact admission/seal predicate for the
// owner-issued Source span. Source owns coordinate ordering; this lane adds
// the diagnostic requirement that file and start coordinates are present.
func validDiagnosticSpan(span programsource.Span) bool {
	if span.File == "" || span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	_, ok := programsource.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	return ok
}

func (payload diagnosticBranchConditionRow) available() bool {
	return payload.decision.Available() && payload.value.Available() && validDiagnosticEvidencePoints(payload.points)
}

func (payload diagnosticBranchConditionRow) empty() bool {
	return !payload.decision.Available() && !payload.value.Available() && len(payload.points) == 0
}

func (payload diagnosticUnresolvedTypeReferenceRow) available() bool {
	if !payload.reference.Available() || len(payload.path) == 0 {
		return false
	}
	for _, component := range payload.path {
		if component == "" {
			return false
		}
	}
	return (len(payload.path) == 1 && !payload.root.Available()) || (len(payload.path) > 1 && payload.root.Available())
}

func (payload diagnosticUnresolvedTypeReferenceRow) empty() bool {
	return !payload.reference.Available() && !payload.root.Available() && len(payload.path) == 0
}

func (payload diagnosticUnresolvedValueReferenceRow) available() bool {
	return payload.read.Available() && payload.cell.Available() && payload.name != ""
}

func (payload diagnosticUnresolvedValueReferenceRow) empty() bool {
	return !payload.read.Available() && !payload.cell.Available() && payload.name == ""
}

func (row DiagnosticObservationRow) Available() bool {
	if !row.id.Available() || !row.kind.valid() || !validDiagnosticSpan(row.location) {
		return false
	}
	switch row.kind {
	case DiagnosticObservationBranchCondition:
		return row.branch.available() && row.unresolved.empty() && row.value.empty()
	case DiagnosticObservationTypeReferenceUnresolved:
		return row.unresolved.available() && row.branch.empty() && row.value.empty()
	case DiagnosticObservationValueReferenceUnresolved:
		return row.value.available() && row.branch.empty() && row.unresolved.empty()
	default:
		return false
	}
}

func validDiagnosticEvidencePoints(points []identity.ContentID) bool {
	if len(points) == 0 {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(points))
	for _, point := range points {
		if !point.Available() {
			return false
		}
		if _, duplicate := seen[point]; duplicate {
			return false
		}
		seen[point] = struct{}{}
	}
	return true
}

func equalDiagnosticObservationRows(left, right DiagnosticObservationRow) bool {
	if left.id != right.id || left.kind != right.kind || left.location != right.location {
		return false
	}
	switch left.kind {
	case DiagnosticObservationBranchCondition:
		if left.branch.decision != right.branch.decision || left.branch.value != right.branch.value || len(left.branch.points) != len(right.branch.points) {
			return false
		}
		for index := range left.branch.points {
			if left.branch.points[index] != right.branch.points[index] {
				return false
			}
		}
		return true
	case DiagnosticObservationTypeReferenceUnresolved:
		if left.unresolved.reference != right.unresolved.reference || left.unresolved.root != right.unresolved.root ||
			len(left.unresolved.path) != len(right.unresolved.path) {
			return false
		}
		for index := range left.unresolved.path {
			if left.unresolved.path[index] != right.unresolved.path[index] {
				return false
			}
		}
		return true
	case DiagnosticObservationValueReferenceUnresolved:
		return left.value == right.value
	default:
		return false
	}
}

func diagnosticObservationID(owner identity.ContentID, kind DiagnosticObservationKind, location programsource.Span, branch diagnosticBranchConditionRow, unresolved diagnosticUnresolvedTypeReferenceRow, value diagnosticUnresolvedValueReferenceRow) identity.ContentID {
	if !owner.Available() || !kind.valid() || !validDiagnosticSpan(location) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/transformer/diagnostic-observation", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(owner[:]) != nil || writer.Uint(uint64(kind)) != nil || writer.String(location.File) != nil ||
		writer.Uint(uint64(location.StartLine)) != nil || writer.Uint(uint64(location.StartCol)) != nil ||
		writer.Uint(uint64(location.EndLine)) != nil || writer.Uint(uint64(location.EndCol)) != nil {
		return identity.ContentID{}
	}
	switch kind {
	case DiagnosticObservationBranchCondition:
		if !branch.available() || writer.Bytes(branch.decision[:]) != nil || writer.Bytes(branch.value[:]) != nil ||
			writer.Count(uint64(len(branch.points))) != nil || !writeDiagnosticEvidencePoints(&writer, branch.points) {
			return identity.ContentID{}
		}
	case DiagnosticObservationTypeReferenceUnresolved:
		if !unresolved.available() || writer.Bytes(unresolved.reference[:]) != nil || writer.Bytes(unresolved.root[:]) != nil ||
			writer.Count(uint64(len(unresolved.path))) != nil {
			return identity.ContentID{}
		}
		for _, component := range unresolved.path {
			if writer.String(component) != nil {
				return identity.ContentID{}
			}
		}
	case DiagnosticObservationValueReferenceUnresolved:
		if !value.available() || writer.Bytes(value.read[:]) != nil || writer.Bytes(value.cell[:]) != nil || writer.String(value.name) != nil {
			return identity.ContentID{}
		}
	default:
		return identity.ContentID{}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func writeDiagnosticEvidencePoints(writer *framing.Writer, points []identity.ContentID) bool {
	for _, point := range points {
		if !point.Available() || writer.Bytes(point[:]) != nil {
			return false
		}
	}
	return true
}
