package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	compiler.diagnosticEvidence = compiler.diagnosticEvidence[:0]
	compiler.diagnosticPaths = compiler.diagnosticPaths[:0]
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
	if failure := compiler.copyAssignmentConformanceObservationsFailure(); failure.Available() {
		return failure
	}
	if failure := compiler.copyWriteConformanceObservationsFailure(); failure.Available() {
		return failure
	}
	return CompileFailure{}
}

// admitDiagnosticBranchFailure copies one eligible Branch route. A guarded
// route from another decision family, or a Branch whose arm rewrite is not
// scope-preserving, intentionally emits no diagnostic row.
func (compiler *compiler) admitDiagnosticBranchFailure(route causal.FinalRoute, rowIndex int) CompileFailure {
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
	decisionTerm := identityValue.Decision
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
		seen[points[index]] = struct{}{}
	}
	branch := diagnosticBranchConditionRow{decision: decisionPath, value: span.ContextID(), points: points}
	row, rowOK := programschema.NewDiagnosticObservationBranchCondition(
		diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationBranchCondition, location, branch, diagnosticUnresolvedTypeReferenceRow{}, diagnosticUnresolvedValueReferenceRow{}, diagnosticTypeConformanceRow{}),
		location, uint32(len(compiler.diagnosticEvidence)), uint32(len(points)), branch.decision, branch.value,
	)
	if !rowOK || !compiler.admitDiagnosticObservation(row, points, nil) {
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
	authoredView := input.Flow().Authored()
	cells := authoredView.Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		term, termOK := cells.At(index)
		kind, body, key, rowOK := cells.Get(term)
		if !termOK || !rowOK {
			return false
		}
		switch kind {
		case authored.CellLocal:
			if key != 0 || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
				return false
			}
			if arm(body) {
				return false
			}
		case authored.CellGlobal:
			if body != 0 || key == 0 {
				return false
			}
		default:
			return false
		}
	}
	labels := authoredView.Control().Labels()
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
			root, rootOK = staticquery.ScopeID(compiler.input.ContentID(), rootTerm)
			if !rootOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		reference, referenceOK := staticquery.TypeReferenceID(compiler.input.ContentID(), ref)
		if !referenceOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		payload := diagnosticUnresolvedTypeReferenceRow{reference: reference, root: root, path: path}
		row, rowOK := programschema.NewDiagnosticObservationTypeReferenceUnresolved(
			diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationTypeReferenceUnresolved, location, diagnosticBranchConditionRow{}, payload, diagnosticUnresolvedValueReferenceRow{}, diagnosticTypeConformanceRow{}),
			location, uint32(len(compiler.diagnosticPaths)), uint32(len(path)), payload.reference, payload.root,
		)
		if !rowOK || !compiler.admitDiagnosticObservation(row, nil, path) {
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
			!read.ID.Available() || !read.Cell.Available() ||
			!cellRelationOK || kind != authored.CellGlobal || body != 0 || key == 0 ||
			!literalOK || literal.Kind != keyspace.LiteralString || literal.String == "" ||
			!locationOK || !validDiagnosticSpan(location) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
		payload := diagnosticUnresolvedValueReferenceRow{read: read.ID, cell: read.Cell, name: literal.String}
		row, rowOK := programschema.NewDiagnosticObservationValueReferenceUnresolved(
			diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationValueReferenceUnresolved, location, diagnosticBranchConditionRow{}, diagnosticUnresolvedTypeReferenceRow{}, payload, diagnosticTypeConformanceRow{}),
			location, payload.read, payload.cell, payload.name,
		)
		if !rowOK || !compiler.admitDiagnosticObservation(row, nil, nil) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
	}
	return CompileFailure{}
}

// The evidence points of a conformance row are the argument's own evaluation
// finish, not the call's: the measured value is the argument's, and its base
// witness is where that value is established.
//
// copyTypeConformanceObservationsFailure issues one TypeConformance row per
// selected direct-call argument whose formal declares a static type. Selection
// is the sealed DirectFunctions join already stored on the cold Call row: uncalled
// interiors do not emit. Method, tail, generic, and vararg calls stay silent.
func (compiler *compiler) copyTypeConformanceObservationsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	selected, selectedOK := compiler.selectedDirectCalleeBodies()
	if !selectedOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
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
		if call.form != accessgeometry.CallFormPlain || call.tail.Available() || len(call.typeArguments) != 0 {
			continue
		}
		boundary, boundaryOK := compiler.functionBoundaryForBody(call.targetBody)
		if !boundaryOK || !boundary.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if boundary.HasVararg() || boundary.FormalCount() != len(call.arguments) {
			continue
		}
		term, termOK := calls.At(index)
		_, _, _, actualsTerm, actualsOK := calls.Get(term)
		if !termOK || !actualsOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		for argumentIndex, argument := range call.arguments {
			formal, formalOK := compiler.functionFormalAt(boundary, argumentIndex)
			declared, declaredOK := formal.DeclaredStaticTypeID()
			if !formalOK || !declaredOK {
				continue
			}
			memberTerm, memberOK := values.Member(actualsTerm, argumentIndex)
			location, locationOK := compiler.input.Source().Identity().Span(memberTerm)
			if !memberOK || !locationOK || !validDiagnosticSpan(location) || !argument.span.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			memberSpan, memberSpanOK := compiler.input.Span(memberTerm)
			memberFinish, memberFinishOK := memberSpan.Finish()
			if !memberSpanOK || !compiler.input.OwnsSpan(memberSpan) || !memberFinishOK || !compiler.input.OwnsSite(memberFinish) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			points := compiler.pointIDs(memberFinish)
			if len(points) == 0 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			payload := diagnosticTypeConformanceRow{
				site:     diagnosticTypeConformanceSiteCallArgument,
				owner:    call.id,
				value:    argument.member,
				declared: declared,
				span:     argument.span,
				position: uint32(argumentIndex),
				points:   append([]identity.ContentID(nil), points...),
			}
			row, rowOK := programschema.NewDiagnosticObservationTypeConformance(
				diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationTypeConformance, location, diagnosticBranchConditionRow{}, diagnosticUnresolvedTypeReferenceRow{}, diagnosticUnresolvedValueReferenceRow{}, payload),
				location, uint32(len(compiler.diagnosticEvidence)), uint32(len(points)),
				programschema.DiagnosticObservationSiteCallArgument, payload.owner, payload.value, payload.declared, payload.span, payload.position,
			)
			if !rowOK || !compiler.admitDiagnosticObservation(row, points, nil) {
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

func (compiler *compiler) admitDiagnosticObservation(
	row programschema.DiagnosticObservation,
	evidence []identity.ContentID,
	path []string,
) bool {
	if compiler == nil || !row.Available() || len(evidence) > int(^uint32(0)) || len(path) > int(^uint32(0)) || len(compiler.diagnosticEvidence) > int(^uint32(0)) || len(compiler.diagnosticPaths) > int(^uint32(0)) {
		return false
	}
	evidenceOffset, evidenceCount, evidenceSpanOK := row.EvidenceSpan()
	pathOffset, pathCount, pathSpanOK := row.PathSpan()
	if !evidenceSpanOK || !pathSpanOK || evidenceCount != uint32(len(evidence)) || pathCount != uint32(len(path)) ||
		(evidenceCount > 0 && evidenceOffset != uint32(len(compiler.diagnosticEvidence))) ||
		(pathCount > 0 && pathOffset != uint32(len(compiler.diagnosticPaths))) {
		return false
	}
	if compiler.diagnosticObservationByID == nil {
		compiler.diagnosticObservationByID = make(map[identity.ContentID]int)
	}
	id := row.ID()
	if index, exists := compiler.diagnosticObservationByID[id]; exists {
		if index < 0 || index >= len(compiler.diagnosticObservations) {
			return false
		}
		prior := compiler.diagnosticObservations[index]
		if !equalDiagnosticObservationCanonical(prior, row, compiler.diagnosticEvidence, compiler.diagnosticPaths, evidence, path) {
			return false
		}
		return true
	}
	seenEvidence := make(map[identity.ContentID]struct{}, len(evidence))
	for _, point := range evidence {
		if !point.Available() {
			return false
		}
		if _, duplicate := seenEvidence[point]; duplicate {
			return false
		}
		seenEvidence[point] = struct{}{}
	}
	for _, component := range path {
		if component == "" {
			return false
		}
	}
	for _, point := range evidence {
		child, childOK := programschema.NewDiagnosticEvidence(point)
		if !childOK {
			return false
		}
		compiler.diagnosticEvidence = append(compiler.diagnosticEvidence, child)
	}
	for _, component := range path {
		child, childOK := programschema.NewDiagnosticPath(component)
		if !childOK {
			return false
		}
		compiler.diagnosticPaths = append(compiler.diagnosticPaths, child)
	}
	compiler.diagnosticObservationByID[id] = len(compiler.diagnosticObservations)
	compiler.diagnosticObservations = append(compiler.diagnosticObservations, row)
	return true
}

func equalDiagnosticObservationCanonical(left, right programschema.DiagnosticObservation, evidence []programschema.DiagnosticEvidence, paths []programschema.DiagnosticPath, wantEvidence []identity.ContentID, wantPath []string) bool {
	if left.ID() != right.ID() || left.Kind() != right.Kind() {
		return false
	}
	leftLocation, leftLocationOK := left.Location()
	rightLocation, rightLocationOK := right.Location()
	if !leftLocationOK || !rightLocationOK || leftLocation != rightLocation {
		return false
	}
	leftOffset, leftCount, leftSpanOK := left.EvidenceSpan()
	_, rightCount, rightSpanOK := right.EvidenceSpan()
	if !leftSpanOK || !rightSpanOK || leftCount != rightCount || int(leftCount) != len(wantEvidence) || uint64(leftOffset)+uint64(leftCount) > uint64(len(evidence)) {
		return false
	}
	for index, point := range wantEvidence {
		if evidence[int(leftOffset)+index].PointID() != point {
			return false
		}
	}
	leftOffset, leftCount, leftSpanOK = left.PathSpan()
	_, rightCount, rightSpanOK = right.PathSpan()
	if !leftSpanOK || !rightSpanOK || leftCount != rightCount || int(leftCount) != len(wantPath) || uint64(leftOffset)+uint64(leftCount) > uint64(len(paths)) {
		return false
	}
	for index, component := range wantPath {
		if paths[int(leftOffset)+index].Component() != component {
			return false
		}
	}
	return left.DecisionPathID() == right.DecisionPathID() && left.ValueSpanID() == right.ValueSpanID() &&
		left.StaticReferenceID() == right.StaticReferenceID() && left.RootID() == right.RootID() &&
		left.ReadID() == right.ReadID() && left.CellID() == right.CellID() && left.Name() == right.Name() &&
		left.Site() == right.Site() && left.OwnerID() == right.OwnerID() && left.MeasuredValueID() == right.MeasuredValueID() &&
		left.DeclaredStaticTypeID() == right.DeclaredStaticTypeID() && left.SpanID() == right.SpanID()
}

// copyWriteConformanceObservationsFailure is the reassignment half of the same
// relation: a write whose target cell was authored with a declared type is
// measured against that declaration, with the written value's own evaluation
// finish as its evidence. An index or lens write has no declared cell and
// contributes no row.
func (compiler *compiler) copyWriteConformanceObservationsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	selected, selectedOK := compiler.selectedDirectCalleeBodies()
	if !selectedOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	view := compiler.input.Flow()
	assigns := view.Authored().Storage().Assigns()
	writes := view.Authored().Storage().Writes()
	authoredValues := view.Authored().Values()
	for index := 0; index < assigns.Count(); index++ {
		term, termOK := assigns.At(index)
		owner, valuesTerm, relationOK := assigns.Get(term)
		width, widthOK := assigns.WriteCount(term)
		if !termOK || !relationOK || !widthOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if !view.Executable().Contains(term) {
			continue
		}
		bodyPath, bodyOK := view.BodyPath(owner)
		if !bodyOK || !bodyPath.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, ownerSelected := selected[bodyPath]; !ownerSelected {
			continue
		}
		assignmentID, assignmentIDOK := view.StorageAssignmentID(term)
		valueRow, valueRowOK := compiler.valueRowForTerm(valuesTerm)
		if !assignmentIDOK || !assignmentID.Available() || !valueRowOK {
			continue
		}
		for position := 0; position < width; position++ {
			writeTerm, writeOK := assigns.WriteAt(term, position)
			writeAssign, target, writeRelationOK := writes.Get(writeTerm)
			if !writeOK || !writeRelationOK || writeAssign != term {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceUnavailable)
			}
			declared, declaredOK := declaredStaticTypeID(compiler.programID, compiler.input.Static(), target)
			memberTerm, memberOK := authoredValues.Member(valuesTerm, position)
			member, memberRowOK := valueRow.MemberAt(position)
			if !declaredOK || !memberOK || !memberRowOK || !member.Available() {
				continue
			}
			if !compiler.admitConformanceObservation(diagnosticTypeConformanceSiteAssignment, assignmentID, member.ID(), declared, memberTerm, uint32(position)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}

// admitConformanceObservation mints and admits one conformance row from the
// coordinates every site shares: the owning statement, the measured value, the
// declaration, and the measured value's own evaluation span.
func (compiler *compiler) admitConformanceObservation(site uint8, owner, value, declared identity.ContentID, measured keyspace.Term, position uint32) bool {
	location, locationOK := compiler.input.Source().Identity().Span(measured)
	span, spanOK := compiler.input.Span(measured)
	finish, finishOK := span.Finish()
	if !locationOK || !validDiagnosticSpan(location) || !spanOK || !compiler.input.OwnsSpan(span) ||
		!finishOK || !compiler.input.OwnsSite(finish) {
		return false
	}
	points := compiler.pointIDs(finish)
	if len(points) == 0 {
		return false
	}
	payload := diagnosticTypeConformanceRow{
		site: site, owner: owner, value: value, declared: declared,
		span: span.ContextID(), position: position,
		points: append([]identity.ContentID(nil), points...),
	}
	row, rowOK := programschema.NewDiagnosticObservationTypeConformance(
		diagnosticObservationID(compiler.input.ContentID(), structure.DiagnosticObservationTypeConformance, location, diagnosticBranchConditionRow{}, diagnosticUnresolvedTypeReferenceRow{}, diagnosticUnresolvedValueReferenceRow{}, payload),
		location, uint32(len(compiler.diagnosticEvidence)), uint32(len(points)),
		programschema.DiagnosticObservationSite(payload.site), payload.owner, payload.value, payload.declared, payload.span, payload.position,
	)
	return rowOK && compiler.admitDiagnosticObservation(row, points, nil)
}

// copyAssignmentConformanceObservationsFailure issues one TypeConformance row
// per bound cell that was authored with a declared type and receives a fixed
// initializer. The measured value is the initializer's, the declaration is the
// cell's, and the evidence is the initializer's own evaluation finish, so the
// row is the assignment half of the same relation a call actual carries.
//
// Selection matches the call-argument half: only bodies the sealed
// DirectFunctions closure reaches emit, so an uncalled interior stays silent.
// A bind position with no declared type, no fixed member, or an open tail
// contributes no row rather than a row measured against nothing.
func (compiler *compiler) copyAssignmentConformanceObservationsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	selected, selectedOK := compiler.selectedDirectCalleeBodies()
	if !selectedOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	view := compiler.input.Flow()
	binds := view.Authored().Storage().Binds()
	authoredValues := view.Authored().Values()
	for index := 0; index < binds.Count(); index++ {
		term, termOK := binds.At(index)
		owner, valuesTerm, relationOK := binds.Get(term)
		width, widthOK := compiler.input.Source().Binds().Len(term)
		if !termOK || !relationOK || !widthOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if !view.Executable().Contains(term) {
			continue
		}
		bodyPath, bodyOK := view.BodyPath(owner)
		if !bodyOK || !bodyPath.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, ownerSelected := selected[bodyPath]; !ownerSelected {
			continue
		}
		bindID, bindIDOK := compiler.diagnosticStorageBindIdentityAt(index)
		if !bindIDOK || !bindID.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		valueRow, valueRowOK := compiler.valueRowForTerm(valuesTerm)
		if !valueRowOK {
			continue
		}
		for position := 0; position < width; position++ {
			cellTerm, cellOK := compiler.input.Source().Binds().At(term, position)
			if !cellOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceUnavailable)
			}
			declared, declaredOK := declaredStaticTypeID(compiler.programID, compiler.input.Static(), cellTerm)
			memberTerm, memberOK := authoredValues.Member(valuesTerm, position)
			member, memberRowOK := valueRow.MemberAt(position)
			if !declaredOK || !memberOK || !memberRowOK || !member.Available() {
				continue
			}
			if !compiler.admitConformanceObservation(diagnosticTypeConformanceSiteAssignment, bindID, member.ID(), declared, memberTerm, uint32(position)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}
