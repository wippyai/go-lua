package programdiagnostic

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// DiagnosticRowsLawVersion is the pinned closed-observation replay law used
// by the CompileKey and Artifact identity. The diagnostic publication owns
// both the tagged parent payload and its dense evidence/path child planes.
const DiagnosticRowsLawVersion uint64 = 5

// WriteArtifactIdentityFields replays the canonical diagnostic portion of an
// Artifact identity from one sealed diagnostic View. Offset fields are cold
// storage geometry, so only the historical semantic payload and ordered child
// spans are written. The caller owns the surrounding identity domain.
func (view View) WriteArtifactIdentityFields(writer identity.StringIdentityWriter) bool {
	if writer == nil || !view.Available() {
		return false
	}
	diagnosticCount, diagnosticsPublished := view.DiagnosticObservationCount()
	evidenceCount, evidencePublished := view.DiagnosticEvidenceCount()
	pathCount, pathsPublished := view.DiagnosticPathCount()
	if !diagnosticsPublished || !evidencePublished || !pathsPublished ||
		!writer.WriteUint(DiagnosticRowsLawVersion) || !writer.WriteUint(uint64(diagnosticCount)) {
		return false
	}
	for index := 0; index < diagnosticCount; index++ {
		row, held := view.DiagnosticObservationAt(index)
		location, locationOK := row.Location()
		evidenceOffset, evidenceWidth, evidenceSpanOK := row.EvidenceSpan()
		pathOffset, pathWidth, pathSpanOK := row.PathSpan()
		position, positionOK := row.Position()
		if !held || !row.Available() || !locationOK || !evidenceSpanOK || !pathSpanOK ||
			(!positionOK && row.Kind() == structure.DiagnosticObservationTypeConformance) ||
			uint64(evidenceOffset)+uint64(evidenceWidth) > uint64(evidenceCount) ||
			uint64(pathOffset)+uint64(pathWidth) > uint64(pathCount) ||
			!writer.WriteContentID(row.ID()) || !writer.WriteUint(uint64(row.Kind())) ||
			!writer.WriteString(location.File) || !writer.WriteUint(uint64(location.StartLine)) ||
			!writer.WriteUint(uint64(location.StartCol)) || !writer.WriteUint(uint64(location.EndLine)) ||
			!writer.WriteUint(uint64(location.EndCol)) {
			return false
		}
		switch row.Kind() {
		case structure.DiagnosticObservationBranchCondition:
			if !writer.WriteContentID(row.DecisionPathID()) || !writer.WriteContentID(row.ValueSpanID()) || !writer.WriteUint(uint64(evidenceWidth)) {
				return false
			}
			for position := uint32(0); position < evidenceWidth; position++ {
				point, pointHeld := view.DiagnosticEvidenceAt(int(evidenceOffset + position))
				if !pointHeld || !point.Available() || !writer.WriteContentID(point.PointID()) {
					return false
				}
			}
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			if !writer.WriteContentID(row.StaticReferenceID()) || !writer.WriteContentID(row.RootID()) || !writer.WriteUint(uint64(pathWidth)) {
				return false
			}
			for position := uint32(0); position < pathWidth; position++ {
				component, componentHeld := view.DiagnosticPathAt(int(pathOffset + position))
				if !componentHeld || !component.Available() || !writer.WriteString(component.Component()) {
					return false
				}
			}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			if !writer.WriteContentID(row.ReadID()) || !writer.WriteContentID(row.CellID()) || !writer.WriteString(row.Name()) {
				return false
			}
		case structure.DiagnosticObservationTypeConformance:
			if !writer.WriteUint(uint64(row.Site())) || !writer.WriteContentID(row.OwnerID()) ||
				!writer.WriteContentID(row.MeasuredValueID()) || !writer.WriteContentID(row.DeclaredStaticTypeID()) ||
				!writer.WriteContentID(row.SpanID()) || !writer.WriteUint(uint64(position)) ||
				!writer.WriteUint(uint64(evidenceWidth)) {
				return false
			}
			for position := uint32(0); position < evidenceWidth; position++ {
				point, pointHeld := view.DiagnosticEvidenceAt(int(evidenceOffset + position))
				if !pointHeld || !point.Available() || !writer.WriteContentID(point.PointID()) {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}
