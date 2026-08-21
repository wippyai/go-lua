package programdiagnostic

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Publication is the construction-only diagnostic payload for manifest slots
// 39 through 41. Readers enter through View over authenticated program state.
type Publication struct {
	DiagnosticObservations []DiagnosticObservation
	DiagnosticEvidence     []DiagnosticEvidence
	DiagnosticPaths        []DiagnosticPath
}

// Append writes the diagnostic plane in canonical manifest order.
func (publication Publication) Append(builder *snapshot.FrozenBuilder, catalogID identity.ContentID) bool {
	if builder == nil || !catalogID.Available() {
		return false
	}
	return DiagnosticObservationFamily().Put(builder, publication.DiagnosticObservations, catalogID) &&
		DiagnosticEvidenceFamily().Put(builder, publication.DiagnosticEvidence, catalogID) &&
		DiagnosticPathFamily().Put(builder, publication.DiagnosticPaths, catalogID)
}
