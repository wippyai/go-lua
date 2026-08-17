package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

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
