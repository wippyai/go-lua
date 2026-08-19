package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// copyDiagnosticObservationsFailure builds the complete immutable diagnostic
// column directly from canonical Program/Flow/Source views. No Program
// diagnostic union is retained or reopened by the artifact compiler.
func (compiler *compiler) copyDiagnosticObservationsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	compiler.diagnosticObservations = compiler.diagnosticObservations[:0]
	if compiler.diagnosticObservationByID == nil {
		compiler.diagnosticObservationByID = make(map[identity.ContentID]int)
	} else {
		clear(compiler.diagnosticObservationByID)
	}
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
	if failure := compiler.copyTypeConformanceObservationsFailure(); failure.Available() {
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
		if point, known := compiler.pointGeometry[points[index]]; !known || !point.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteGuard)
		}
		seen[points[index]] = struct{}{}
	}
	branch := diagnosticBranchConditionRow{decision: decisionPath, value: span.ContextID(), points: points}
	row := DiagnosticObservationRow{
		id:   diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationBranchCondition, location, branch, diagnosticUnresolvedTypeReferenceRow{}, diagnosticUnresolvedValueReferenceRow{}, diagnosticTypeConformanceRow{}),
		kind: structure.DiagnosticObservationBranchCondition, location: location, branch: branch,
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
		if !relationOK || resolution != staticrefs.Unresolved || target != 0 {
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
			componentLiteral, componentOK := compiler.input.Source().Keys().Exact(key)
			componentOK = componentOK && componentLiteral.Kind == keyspace.LiteralString
			component := componentLiteral.String
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
			root, rootOK = programstatic.ScopeID(compiler.input.ContentID(), rootTerm)
			if !rootOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		reference, referenceOK := programstatic.TypeReferenceID(compiler.input.ContentID(), ref)
		if !referenceOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		payload := diagnosticUnresolvedTypeReferenceRow{reference: reference, root: root, path: path}
		row := DiagnosticObservationRow{
			id:   diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationTypeReferenceUnresolved, location, diagnosticBranchConditionRow{}, payload, diagnosticUnresolvedValueReferenceRow{}, diagnosticTypeConformanceRow{}),
			kind: structure.DiagnosticObservationTypeReferenceUnresolved, location: location, unresolved: payload,
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
			id:   diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationValueReferenceUnresolved, location, diagnosticBranchConditionRow{}, diagnosticUnresolvedTypeReferenceRow{}, payload, diagnosticTypeConformanceRow{}),
			kind: structure.DiagnosticObservationValueReferenceUnresolved, location: location, value: payload,
		}
		if !row.Available() || !compiler.admitDiagnosticObservationRow(row) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
	}
	return CompileFailure{}
}

// copyTypeConformanceObservationsFailure issues one TypeConformance row per
// selected direct-call argument whose formal declares a static type. Selection
// is the sealed DirectFunctions join already stored on CallRow: uncalled
// interiors do not emit. Method, tail, generic, and vararg calls stay silent.
func (compiler *compiler) copyTypeConformanceObservationsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	selected, selectedOK := compiler.selectedDirectCalleeBodies()
	if !selectedOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	boundaries := make(map[identity.ContentID]FunctionBoundaryRow, len(compiler.functionBoundaries))
	for _, row := range compiler.functionBoundaries {
		if !row.Available() || !row.BodyID().Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
		}
		if _, duplicate := boundaries[row.BodyID()]; duplicate {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
		}
		boundaries[row.BodyID()] = row
	}
	flowView := compiler.input.Flow()
	calls := flowView.Authored().Calls()
	values := flowView.Authored().Values()
	for index := 0; index < calls.Count(); index++ {
		call, callOK := compiler.callConstruction(index)
		if !callOK || !call.targetBody.Available() {
			continue
		}
		if _, ownerSelected := selected[call.bodyPath]; !ownerSelected {
			continue
		}
		if call.form != flow.CallFormPlain || call.tail.Available() || len(call.typeArguments) != 0 {
			continue
		}
		boundary, boundaryOK := boundaries[call.targetBody]
		if !boundaryOK || !boundary.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if _, hasVararg := boundary.Vararg(); hasVararg || boundary.FormalCount() != len(call.arguments) {
			continue
		}
		term, termOK := calls.At(index)
		_, _, _, actualsTerm, actualsOK := calls.Get(term)
		if !termOK || !actualsOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		points := compiler.pointIDs(call.finish)
		if len(points) == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		for argumentIndex, argument := range call.arguments {
			formal, formalOK := boundary.FormalAt(argumentIndex)
			declared, declaredOK := formal.DeclaredStaticTypeID()
			if !formalOK || !declaredOK {
				continue
			}
			memberTerm, memberOK := values.Member(actualsTerm, argumentIndex)
			location, locationOK := compiler.input.Source().Identity().Span(memberTerm)
			if !memberOK || !locationOK || !validDiagnosticSpan(location) || !argument.span.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			payload := diagnosticTypeConformanceRow{
				site:     diagnosticTypeConformanceSiteCallArgument,
				call:     call.id,
				argument: argument.id,
				declared: declared,
				span:     argument.span,
				position: uint32(argumentIndex),
				points:   append([]identity.ContentID(nil), points...),
			}
			row := DiagnosticObservationRow{
				id:   diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationTypeConformance, location, diagnosticBranchConditionRow{}, diagnosticUnresolvedTypeReferenceRow{}, diagnosticUnresolvedValueReferenceRow{}, payload),
				kind: structure.DiagnosticObservationTypeConformance, location: location, conformance: payload,
			}
			if !row.Available() || !compiler.admitDiagnosticObservationRow(row) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) selectedDirectCalleeBodies() (map[identity.ContentID]struct{}, bool) {
	if compiler == nil {
		return nil, false
	}
	selected := make(map[identity.ContentID]struct{})
	callable := make(map[identity.ContentID]struct{})
	for _, body := range compiler.bodies {
		if !body.Available() {
			return nil, false
		}
		if body.Callable() {
			callable[body.ID()] = struct{}{}
			continue
		}
		selected[body.ID()] = struct{}{}
	}
	if len(selected) == 0 {
		return nil, false
	}
	for changed := true; changed; {
		changed = false
		for _, call := range compiler.calls {
			target, targetOK := call.DirectTargetBody()
			if !targetOK {
				continue
			}
			if _, ownerSelected := selected[call.BodyID()]; !ownerSelected {
				continue
			}
			if _, already := selected[target]; already {
				continue
			}
			if _, isCallable := callable[target]; !isCallable {
				return nil, false
			}
			selected[target] = struct{}{}
			changed = true
		}
	}
	return selected, true
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
