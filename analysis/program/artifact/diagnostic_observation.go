package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
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
	decision identity.ContentID
	value    identity.ContentID
	points   []identity.ContentID
}

func (payload diagnosticBranchConditionRow) available() bool {
	return payload.decision.Available() && payload.value.Available() && validDiagnosticEvidencePoints(payload.points)
}

func (payload diagnosticBranchConditionRow) empty() bool {
	return !payload.decision.Available() && !payload.value.Available() && len(payload.points) == 0
}

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

type diagnosticUnresolvedTypeReferenceRow struct {
	reference identity.ContentID
	root      identity.ContentID
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

type diagnosticUnresolvedValueReferenceRow struct {
	read identity.ContentID
	cell identity.ContentID
	name string
}

func (payload diagnosticUnresolvedValueReferenceRow) available() bool {
	return payload.read.Available() && payload.cell.Available() && payload.name != ""
}

func (payload diagnosticUnresolvedValueReferenceRow) empty() bool {
	return !payload.read.Available() && !payload.cell.Available() && payload.name == ""
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

// DiagnosticObservationRow is one immutable tagged observation row. Its
// typed payload union is exact: branch rows carry only branch geometry;
// unresolved-type rows carry only the static reference proof and lexical
// path. Lower layers therefore consume owner-issued facts instead of
// reconstructing semantic families from optional scalar fields.
type DiagnosticObservationRow struct {
	id         identity.ContentID
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

func (row DiagnosticObservationRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
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
	return diagnosticBranchConditionRow{decision: row.branch.decision, value: row.branch.value, points: append([]identity.ContentID(nil), row.branch.points...)}, true
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

// copyDiagnosticObservationsFailure builds the complete immutable diagnostic
// column directly from canonical Program/Flow/Source views. No Program
// diagnostic union is retained or reopened by the artifact compiler.
func (compiler *compiler) copyDiagnosticObservationsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	compiler.diagnosticObservations = compiler.diagnosticObservations[:0]
	compiler.diagnosticObservationByID = make(map[identity.ContentID]int)
	routes := compiler.input.Flow().Causal().Successors()
	for index := 0; index < routes.TotalCount(); index++ {
		route, routeOK := routes.FinalAt(index)
		if !routeOK {
			return compileFailure(CompileStageRoutes, CompileRowRoute, index, -1, CompileReasonRouteUnavailable)
		}
		if failure := compiler.admitDiagnosticBranchFailure(route, index); failure.Available() {
			return failure
		}
	}
	if failure := compiler.copyUnresolvedTypeObservationsFailure(); failure.Available() {
		return failure
	}
	if failure := compiler.copyUnresolvedValueObservationsFailure(); failure.Available() {
		return failure
	}
	return CompileFailure{}
}

// admitDiagnosticBranchFailure copies one eligible Branch route. A guarded
// route from another decision family, or a Branch whose arm rewrite is not
// scope-preserving, intentionally emits no diagnostic row.
func (compiler *compiler) admitDiagnosticBranchFailure(route flow.FinalRoute, rowIndex int) CompileFailure {
	if !route.Available() {
		return CompileFailure{}
	}
	if _, fromOK := route.From(); !fromOK {
		return CompileFailure{}
	}
	if _, toOK := route.To(); !toOK {
		return CompileFailure{}
	}
	guard, guardOK := route.GuardProof()
	if !guardOK {
		return CompileFailure{}
	}
	identityValue, identityOK := route.Identity()
	if !identityOK {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
	}
	decisionTerm := identityValue.Decision()
	termOK := decisionTerm != 0
	if !termOK || keyspace.TermFamily(decisionTerm) != keyspace.FamilyBranch {
		return CompileFailure{}
	}
	// Branches.Get returns (owner, condition, true arm, false arm, ok).
	owner, branchCondition, branchTrue, branchFalse, branchRelationOK := compiler.input.Flow().Authored().Control().Branches().Get(decisionTerm)
	_ = owner
	if !branchRelationOK || !diagnosticBranchScopeRewriteSafe(compiler.input, branchTrue, branchFalse) {
		return CompileFailure{}
	}
	span, spanOK := compiler.input.Span(branchCondition)
	location, locationOK := compiler.input.Source().Identity().Span(branchCondition)
	finish, finishOK := span.Finish()
	decisionPath, pathOK := guard.DecisionPathID()
	if !spanOK || !compiler.input.OwnsSpan(span) || !locationOK || !validDiagnosticSpan(location) ||
		!finishOK || !compiler.input.OwnsSite(finish) || len(compiler.pointIDs(finish)) == 0 ||
		!pathOK || !decisionPath.Available() {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
	}
	attachments := compiler.pointIDs(finish)
	points := make([]identity.ContentID, len(attachments))
	seen := make(map[identity.ContentID]struct{}, len(points))
	for index := range points {
		points[index] = attachments[index]
		if !points[index].Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteGuard)
		}
		if _, duplicate := seen[points[index]]; duplicate {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteGuard)
		}
		if _, known := compiler.points[points[index]]; !known {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteGuard)
		}
		seen[points[index]] = struct{}{}
	}
	branch := diagnosticBranchConditionRow{decision: decisionPath, value: span.ContextID(), points: points}
	row := DiagnosticObservationRow{
		id:   diagnosticObservationID(compiler.input.ContentID(), DiagnosticObservationBranchCondition, location, branch, diagnosticUnresolvedTypeReferenceRow{}, diagnosticUnresolvedValueReferenceRow{}),
		kind: DiagnosticObservationBranchCondition, location: location, branch: branch,
	}
	if !row.Available() || !compiler.admitDiagnosticObservationRow(row) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
	}
	return CompileFailure{}
}

// diagnosticBranchScopeRewriteSafe is the artifact builder's copy of the
// source-rewrite eligibility law. It consumes only canonical Flow/Static
// rows; no Source or authored term is retained after row construction.
func diagnosticBranchScopeRewriteSafe(input *program.Program, whenTrue, whenFalse keyspace.Term) bool {
	if !input.Available() || keyspace.TermFamily(whenTrue) != keyspace.FamilyBody || keyspace.TermOrdinal(whenTrue) == 0 ||
		keyspace.TermFamily(whenFalse) != keyspace.FamilyBody || keyspace.TermOrdinal(whenFalse) == 0 || whenTrue == whenFalse {
		return false
	}
	arm := func(owner keyspace.Term) bool { return owner == whenTrue || owner == whenFalse }
	authored := input.Flow().Authored()
	cells := authored.Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		term, termOK := cells.At(index)
		kind, body, key, rowOK := cells.Get(term)
		if !termOK || !rowOK {
			return false
		}
		switch kind {
		case flow.CellLocal:
			if key != 0 || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
				return false
			}
			if arm(body) {
				return false
			}
		case flow.CellGlobal:
			if body != 0 || key == 0 {
				return false
			}
		default:
			return false
		}
	}
	labels := authored.Control().Labels()
	for index := 0; index < labels.Count(); index++ {
		term, termOK := labels.At(index)
		owner, rowOK := labels.Get(term)
		if !termOK || !rowOK || arm(owner) {
			return false
		}
	}
	programOwner := input
	if programOwner == nil {
		return false
	}
	static := programOwner.Static().Declarations()
	aliases := static.Aliases()
	for index := 0; index < aliases.Count(); index++ {
		term, termOK := aliases.At(index)
		owner, _, _, _, rowOK := aliases.Get(term)
		if !termOK || !rowOK || arm(owner) {
			return false
		}
	}
	interfaces := static.Interfaces()
	for index := 0; index < interfaces.Count(); index++ {
		term, termOK := interfaces.At(index)
		owner, _, _, rowOK := interfaces.Get(term)
		if !termOK || !rowOK || arm(owner) {
			return false
		}
	}
	return true
}

func (compiler *compiler) copyUnresolvedTypeObservationsFailure() CompileFailure {
	view := compiler.input.Static()
	types := view.StaticTypes()
	references := view.References()
	for index := 0; index < types.Count(); index++ {
		ref, refOK := types.At(index)
		if !refOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		term := ref.Term()
		resolution, target, rootTerm, relationOK := references.Get(term)
		if !relationOK || resolution != programstatic.TypeRefUnresolved || target != 0 {
			continue
		}
		location, locationOK := compiler.input.Source().Identity().Span(term)
		count, countOK := references.SourceCount(term)
		if !locationOK || !validDiagnosticSpan(location) || !countOK || count == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		path := make([]string, count)
		for pathIndex := range path {
			key, keyOK := references.SourceAt(term, pathIndex)
			component, componentOK := compiler.input.StaticKeyText(key)
			if !keyOK || !componentOK || component == "" {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, pathIndex, CompileReasonOccurrenceUnavailable)
			}
			path[pathIndex] = component
		}
		root := identity.ContentID{}
		if len(path) == 1 {
			if rootTerm != 0 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		} else {
			if rootTerm == 0 || keyspace.TermFamily(rootTerm) != keyspace.FamilyCell {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			var rootOK bool
			root, rootOK = program.StaticScopeID(compiler.input.ContentID(), rootTerm)
			if !rootOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		reference, referenceOK := program.StaticTypeReferenceID(compiler.input.ContentID(), ref)
		if !referenceOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		payload := diagnosticUnresolvedTypeReferenceRow{reference: reference, root: root, path: path}
		row := DiagnosticObservationRow{
			id:   diagnosticObservationID(compiler.input.ContentID(), DiagnosticObservationTypeReferenceUnresolved, location, diagnosticBranchConditionRow{}, payload, diagnosticUnresolvedValueReferenceRow{}),
			kind: DiagnosticObservationTypeReferenceUnresolved, location: location, unresolved: payload,
		}
		if !row.Available() || !compiler.admitDiagnosticObservationRow(row) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}

// copyUnresolvedValueObservationsFailure consumes only Flow's sparse
// implicit-read denominator. It never scans names or infers binder absence
// from the ordinary Read catalog.
func (compiler *compiler) copyUnresolvedValueObservationsFailure() CompileFailure {
	reads := compiler.input.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.ImplicitCount(); index++ {
		term, termOK := reads.ImplicitAt(index)
		owner, sourceTerm, implicit, relationOK := reads.Get(term)
		ordinal := keyspace.TermOrdinal(term)
		read, readOK := compiler.storageReadAt(int(ordinal - 1))
		kind, body, key, cellRelationOK := compiler.input.Flow().Authored().Storage().Cells().Get(sourceTerm)
		literal, literalOK := compiler.input.Source().Keys().Exact(key)
		location, locationOK := compiler.input.Source().Identity().Span(term)
		if !termOK || !relationOK || !implicit || owner == 0 || ordinal == 0 || !readOK ||
			!read.id.Available() || !read.cell.Available() ||
			!cellRelationOK || kind != flow.CellGlobal || body != 0 || key == 0 ||
			!literalOK || literal.Kind != keyspace.LiteralString || literal.String == "" ||
			!locationOK || !validDiagnosticSpan(location) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
		payload := diagnosticUnresolvedValueReferenceRow{read: read.id, cell: read.cell, name: literal.String}
		row := DiagnosticObservationRow{
			id:   diagnosticObservationID(compiler.input.ContentID(), DiagnosticObservationValueReferenceUnresolved, location, diagnosticBranchConditionRow{}, diagnosticUnresolvedTypeReferenceRow{}, payload),
			kind: DiagnosticObservationValueReferenceUnresolved, location: location, value: payload,
		}
		if !row.Available() || !compiler.admitDiagnosticObservationRow(row) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) admitDiagnosticObservationRow(row DiagnosticObservationRow) bool {
	if compiler == nil || !row.Available() {
		return false
	}
	if compiler.diagnosticObservationByID == nil {
		compiler.diagnosticObservationByID = make(map[identity.ContentID]int)
	}
	if index, exists := compiler.diagnosticObservationByID[row.id]; exists {
		return index >= 0 && index < len(compiler.diagnosticObservations) && equalDiagnosticObservationRows(compiler.diagnosticObservations[index], row)
	}
	compiler.diagnosticObservationByID[row.id] = len(compiler.diagnosticObservations)
	compiler.diagnosticObservations = append(compiler.diagnosticObservations, row)
	return true
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
