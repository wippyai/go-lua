package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

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

// DiagnosticObservationRow is one immutable tagged observation row. Its
// typed payload union is exact: branch rows carry only branch geometry;
// unresolved-type rows carry only the static reference proof and lexical
// path. Lower layers therefore consume owner-issued facts instead of
// reconstructing semantic families from optional scalar fields.
type DiagnosticObservationRow struct {
	id         identity.ContentID
	kind       structure.DiagnosticObservationKind
	location   programsource.Span
	branch     diagnosticBranchConditionRow
	unresolved diagnosticUnresolvedTypeReferenceRow
	value      diagnosticUnresolvedValueReferenceRow
}

func (artifact *Artifact) DiagnosticObservationCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.diagnosticObservations)
}

func (artifact *Artifact) DiagnosticObservationAt(index int) (DiagnosticObservationRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.diagnosticObservations) {
		return DiagnosticObservationRow{}, false
	}
	return artifact.diagnosticObservations[index], true
}
