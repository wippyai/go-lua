package programartifact

import (
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	programsource "github.com/wippyai/go-lua/program/source"
)

// DiagnosticObservationKind is the closed reusable semantic-observation
// catalog. It contains no severity, message, or policy.
type DiagnosticObservationKind uint8

const (
	DiagnosticObservationInvalid DiagnosticObservationKind = iota
	DiagnosticObservationBranchCondition
	DiagnosticObservationTypeReferenceUnresolved
	DiagnosticObservationValueReferenceUnresolved
)

func diagnosticObservationKind(kind program.DiagnosticObservationKind) (DiagnosticObservationKind, bool) {
	switch kind {
	case program.DiagnosticObservationBranchCondition:
		return DiagnosticObservationBranchCondition, true
	case program.DiagnosticObservationTypeReferenceUnresolved:
		return DiagnosticObservationTypeReferenceUnresolved, true
	case program.DiagnosticObservationValueReferenceUnresolved:
		return DiagnosticObservationValueReferenceUnresolved, true
	default:
		return DiagnosticObservationInvalid, false
	}
}

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

type diagnosticBranchConditionRow struct {
	decision keyspace.ContentID
	value    keyspace.ContentID
	points   []keyspace.ContentID
}

func (payload diagnosticBranchConditionRow) available() bool {
	return payload.decision.Available() && payload.value.Available() && validDiagnosticEvidencePoints(payload.points)
}

func (payload diagnosticBranchConditionRow) empty() bool {
	return !payload.decision.Available() && !payload.value.Available() && len(payload.points) == 0
}

func (payload diagnosticBranchConditionRow) DecisionPathID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.decision
}

func (payload diagnosticBranchConditionRow) ValueSpanID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.value
}

func (payload diagnosticBranchConditionRow) EvidencePointCount() int {
	if !payload.available() {
		return 0
	}
	return len(payload.points)
}

func (payload diagnosticBranchConditionRow) EvidencePoints() ([]keyspace.ContentID, bool) {
	if !payload.available() {
		return nil, false
	}
	return append([]keyspace.ContentID(nil), payload.points...), true
}

func (payload diagnosticBranchConditionRow) EvidencePointAt(index int) (keyspace.ContentID, bool) {
	if !payload.available() || index < 0 || index >= len(payload.points) {
		return keyspace.ContentID{}, false
	}
	return payload.points[index], true
}

type diagnosticUnresolvedTypeReferenceRow struct {
	reference keyspace.ContentID
	root      keyspace.ContentID
	path      []string
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

func (payload diagnosticUnresolvedTypeReferenceRow) StaticReferenceID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.reference
}

func (payload diagnosticUnresolvedTypeReferenceRow) RootID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.root
}

func (payload diagnosticUnresolvedTypeReferenceRow) Path() ([]string, bool) {
	if !payload.available() {
		return nil, false
	}
	return append([]string(nil), payload.path...), true
}

type diagnosticUnresolvedValueReferenceRow struct {
	read keyspace.ContentID
	cell keyspace.ContentID
	name string
}

func (payload diagnosticUnresolvedValueReferenceRow) available() bool {
	return payload.read.Available() && payload.cell.Available() && payload.name != ""
}

func (payload diagnosticUnresolvedValueReferenceRow) empty() bool {
	return !payload.read.Available() && !payload.cell.Available() && payload.name == ""
}

func (payload diagnosticUnresolvedValueReferenceRow) ReadID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.read
}

func (payload diagnosticUnresolvedValueReferenceRow) CellID() keyspace.ContentID {
	if !payload.available() {
		return keyspace.ContentID{}
	}
	return payload.cell
}

func (payload diagnosticUnresolvedValueReferenceRow) Name() (string, bool) {
	return payload.name, payload.available()
}

// DiagnosticObservationRow is one immutable tagged observation row. Its
// typed payload union is exact: branch rows carry only branch geometry;
// unresolved-type rows carry only the static reference proof and lexical
// path. Lower layers therefore consume owner-issued facts instead of
// reconstructing semantic families from optional scalar fields.
type DiagnosticObservationRow struct {
	id         keyspace.ContentID
	kind       DiagnosticObservationKind
	location   programsource.Span
	branch     diagnosticBranchConditionRow
	unresolved diagnosticUnresolvedTypeReferenceRow
	value      diagnosticUnresolvedValueReferenceRow
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

func (row DiagnosticObservationRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}

func (row DiagnosticObservationRow) Kind() DiagnosticObservationKind {
	if !row.Available() {
		return DiagnosticObservationInvalid
	}
	return row.kind
}

func (row DiagnosticObservationRow) Location() (programsource.Span, bool) {
	return row.location, row.Available() && validDiagnosticSpan(row.location)
}

func (row DiagnosticObservationRow) BranchCondition() (diagnosticBranchConditionRow, bool) {
	if !row.Available() || row.kind != DiagnosticObservationBranchCondition {
		return diagnosticBranchConditionRow{}, false
	}
	return diagnosticBranchConditionRow{decision: row.branch.decision, value: row.branch.value, points: append([]keyspace.ContentID(nil), row.branch.points...)}, true
}

func (row DiagnosticObservationRow) UnresolvedTypeReference() (diagnosticUnresolvedTypeReferenceRow, bool) {
	if !row.Available() || row.kind != DiagnosticObservationTypeReferenceUnresolved {
		return diagnosticUnresolvedTypeReferenceRow{}, false
	}
	return diagnosticUnresolvedTypeReferenceRow{reference: row.unresolved.reference, root: row.unresolved.root, path: append([]string(nil), row.unresolved.path...)}, true
}

func (row DiagnosticObservationRow) UnresolvedValueReference() (diagnosticUnresolvedValueReferenceRow, bool) {
	if !row.Available() || row.kind != DiagnosticObservationValueReferenceUnresolved {
		return diagnosticUnresolvedValueReferenceRow{}, false
	}
	return row.value, true
}

func (compiler *compiler) admitDiagnosticObservation(parent program.DiagnosticObservation) bool {
	if !compiler.input.OwnsDiagnosticObservation(parent) {
		return false
	}
	kind, kindOK := diagnosticObservationKind(parent.Kind())
	location, locationOK := parent.Location()
	if !kindOK || !locationOK || !parent.ID().Available() {
		return false
	}
	row := DiagnosticObservationRow{id: parent.ID(), kind: kind, location: location}
	switch kind {
	case DiagnosticObservationBranchCondition:
		payload, payloadOK := parent.BranchCondition()
		points, pointsOK := payload.EvidencePoints()
		if !payloadOK || !pointsOK || !validDiagnosticEvidencePoints(points) {
			return false
		}
		for _, point := range points {
			if _, exists := compiler.points[point]; !exists {
				return false
			}
		}
		row.branch = diagnosticBranchConditionRow{decision: payload.DecisionPathID(), value: payload.ValueSpanID(), points: points}
	case DiagnosticObservationTypeReferenceUnresolved:
		payload, payloadOK := parent.UnresolvedTypeReference()
		path, pathOK := payload.Path()
		if !payloadOK || !pathOK {
			return false
		}
		row.unresolved = diagnosticUnresolvedTypeReferenceRow{
			reference: payload.StaticReferenceID(), root: payload.RootID(), path: path,
		}
	case DiagnosticObservationValueReferenceUnresolved:
		payload, payloadOK := parent.UnresolvedValueReference()
		name, nameOK := payload.Name()
		if !payloadOK || !nameOK {
			return false
		}
		row.value = diagnosticUnresolvedValueReferenceRow{read: payload.ReadID(), cell: payload.CellID(), name: name}
	default:
		return false
	}
	if !row.Available() {
		return false
	}
	if index, exists := compiler.diagnosticObservationByID[row.id]; exists {
		return index >= 0 && index < len(compiler.diagnosticObservations) && equalDiagnosticObservationRows(compiler.diagnosticObservations[index], row)
	}
	compiler.diagnosticObservationByID[row.id] = len(compiler.diagnosticObservations)
	compiler.diagnosticObservations = append(compiler.diagnosticObservations, row)
	return true
}

func validDiagnosticEvidencePoints(points []keyspace.ContentID) bool {
	if len(points) == 0 {
		return false
	}
	seen := make(map[keyspace.ContentID]struct{}, len(points))
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

// copyUnresolvedValueObservations consumes only Program's sparse implicit-read
// denominator. It never scans names or reconstructs binder absence from the
// ordinary storage catalog.
func (compiler *compiler) copyUnresolvedValueObservationsFailure() CompileFailure {
	count := compiler.input.ValueReferenceUnresolvedObservationCount()
	for index := 0; index < count; index++ {
		observation, ok := compiler.input.ValueReferenceUnresolvedObservationAt(index)
		if !ok || !compiler.admitDiagnosticObservation(observation) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
	}
	return CompileFailure{}
}
