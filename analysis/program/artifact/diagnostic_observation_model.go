package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/internal/framing"
)

// These payloads are compiler-only drafts used while the source/Flow proofs
// are live. They are converted immediately into the canonical Program parent
// and dense child families; no draft is retained by Artifact or ingress.
type diagnosticBranchConditionRow struct {
	decision identity.ContentID
	value    identity.ContentID
	points   []identity.ContentID
}

type diagnosticUnresolvedTypeReferenceRow struct {
	reference identity.ContentID
	root      identity.ContentID
	path      []string
}

type diagnosticUnresolvedValueReferenceRow struct {
	read identity.ContentID
	cell identity.ContentID
	name string
}

const (
	diagnosticTypeConformanceSiteCallArgument uint8 = 1
	diagnosticTypeConformanceSiteAssignment   uint8 = 2
)

type diagnosticTypeConformanceRow struct {
	site     uint8
	owner    identity.ContentID
	value    identity.ContentID
	declared identity.ContentID
	span     identity.ContentID
	position uint32
	points   []identity.ContentID
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

func (payload diagnosticTypeConformanceRow) available() bool {
	switch payload.site {
	case diagnosticTypeConformanceSiteCallArgument, diagnosticTypeConformanceSiteAssignment:
	default:
		return false
	}
	return payload.owner.Available() && payload.value.Available() && payload.declared.Available() &&
		payload.span.Available() && validDiagnosticEvidencePoints(payload.points)
}

func (payload diagnosticTypeConformanceRow) empty() bool {
	return payload.site == 0 && !payload.owner.Available() && !payload.value.Available() &&
		!payload.declared.Available() && !payload.span.Available() && payload.position == 0 && len(payload.points) == 0
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

func diagnosticObservationID(owner identity.ContentID, kind structure.DiagnosticObservationKind, location programsource.Span, branch diagnosticBranchConditionRow, unresolved diagnosticUnresolvedTypeReferenceRow, value diagnosticUnresolvedValueReferenceRow, conformance diagnosticTypeConformanceRow) identity.ContentID {
	if !owner.Available() || !kind.Available() || !validDiagnosticSpan(location) {
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
	case structure.DiagnosticObservationBranchCondition:
		if !branch.available() || writer.Bytes(branch.decision[:]) != nil || writer.Bytes(branch.value[:]) != nil ||
			writer.Count(uint64(len(branch.points))) != nil || !writeDiagnosticEvidencePoints(&writer, branch.points) {
			return identity.ContentID{}
		}
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		if !unresolved.available() || writer.Bytes(unresolved.reference[:]) != nil || writer.Bytes(unresolved.root[:]) != nil ||
			writer.Count(uint64(len(unresolved.path))) != nil {
			return identity.ContentID{}
		}
		for _, component := range unresolved.path {
			if writer.String(component) != nil {
				return identity.ContentID{}
			}
		}
	case structure.DiagnosticObservationValueReferenceUnresolved:
		if !value.available() || writer.Bytes(value.read[:]) != nil || writer.Bytes(value.cell[:]) != nil || writer.String(value.name) != nil {
			return identity.ContentID{}
		}
	case structure.DiagnosticObservationTypeConformance:
		if !conformance.available() || writer.Uint(uint64(conformance.site)) != nil ||
			writer.Bytes(conformance.owner[:]) != nil || writer.Bytes(conformance.value[:]) != nil ||
			writer.Bytes(conformance.declared[:]) != nil || writer.Bytes(conformance.span[:]) != nil ||
			writer.Uint(uint64(conformance.position)) != nil || writer.Count(uint64(len(conformance.points))) != nil ||
			!writeDiagnosticEvidencePoints(&writer, conformance.points) {
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
