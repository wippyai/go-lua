package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/rowidentity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	programstorage "github.com/wippyai/go-lua/analysis/program/storage"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
)

// copyDiagnosticObservationsFailure builds the complete immutable diagnostic
// column directly from canonical Program/Flow/Source views. No Program
// diagnostic union is retained or reopened by the artifact compiler.
func (compiler *compiler) copyDiagnosticObservationsFailure() programconstruction.Fault {
	if compiler == nil || !compiler.input.Available() {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, -1, -1)
	}
	compiler.diagnosticObservations = compiler.diagnosticObservations[:0]
	compiler.diagnosticEvidence = compiler.diagnosticEvidence[:0]
	compiler.diagnosticPaths = compiler.diagnosticPaths[:0]
	if compiler.diagnosticObservationByID == nil {
		compiler.diagnosticObservationByID = make(map[identity.ContentID]int)
	} else {
		clear(compiler.diagnosticObservationByID)
	}
	routes := compiler.input.Program.Flow().Causal().Successors()
	for index := 0; index < routes.TotalCount(); index++ {
		route, routeOK := routes.FinalAt(index)
		if !routeOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteUnavailable, index, -1)
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
	return programconstruction.Fault{}
}

// admitDiagnosticBranchFailure copies one eligible Branch route. The
// population is the guarded routes: an unguarded route carries no decision and
// states no branch condition, so it is outside this observation rather than a
// dropped row. A guarded route from another decision family, or a Branch whose
// arm rewrite is not scope-preserving, is likewise outside the population.
// Every remaining route is either admitted as a row or refused under a named
// construction issue.
func (compiler *compiler) admitDiagnosticBranchFailure(route causal.FinalRoute, rowIndex int) programconstruction.Fault {
	if !route.Available() {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteUnavailable, rowIndex, -1)
	}
	if !route.Guarded() {
		return programconstruction.Fault{}
	}
	_, fromOK := route.From()
	_, toOK := route.To()
	if !fromOK || !toOK {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteUnavailable, rowIndex, -1)
	}
	guard, guardOK := route.GuardProof()
	if !guardOK {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteGuard, rowIndex, -1)
	}
	identityValue, identityOK := route.Identity()
	if !identityOK {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteGuard, rowIndex, -1)
	}
	decisionTerm := identityValue.Decision
	termOK := decisionTerm != 0
	if !termOK || keyspace.TermFamily(decisionTerm) != keyspace.FamilyBranch {
		return programconstruction.Fault{}
	}
	// Branches.Get returns (owner, condition, true arm, false arm, ok).
	owner, branchCondition, branchTrue, branchFalse, branchRelationOK := compiler.input.Program.Flow().Authored().Control().Branches().Get(decisionTerm)
	_ = owner
	containment := compiler.input.Program.Flow().Containment()
	if !branchRelationOK || containment == nil || branchTrue == branchFalse {
		return programconstruction.Fault{}
	}
	trueSensitive, trueScopeOK := containment.ScopeSensitiveBody(branchTrue)
	falseSensitive, falseScopeOK := containment.ScopeSensitiveBody(branchFalse)
	if !trueScopeOK || !falseScopeOK || trueSensitive || falseSensitive {
		return programconstruction.Fault{}
	}
	span, spanOK := compiler.input.Program.Span(branchCondition)
	location, locationOK := compiler.input.Program.Source().Identity().Span(branchCondition)
	finish, finishOK := span.Finish()
	decisionPath, pathOK := guard.DecisionPathID()
	if !spanOK || !compiler.input.Program.OwnsSpan(span) || !locationOK || !programdiagnostic.ValidSpan(location) ||
		!finishOK || !compiler.input.Program.OwnsSite(finish) || compiler.input.Program.Flow().LocalWTO().PointPathsForSite(finish).Count() == 0 ||
		!pathOK || !decisionPath.Available() {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteGuard, rowIndex, -1)
	}
	attachments := compiler.input.Program.Flow().LocalWTO().PointPathsForSite(finish)
	points, pointsOK := compiler.pointPaths(attachments)
	if !pointsOK || !compiler.validUniqueEvidence(points) {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteGuard, rowIndex, -1)
	}
	row, rowOK := programdiagnostic.NewDiagnosticObservationBranchCondition(
		programdiagnostic.BranchConditionIdentity(compiler.input.Program.ContentID(), location, decisionPath, span.ContextID(), points),
		location, uint32(len(compiler.diagnosticEvidence)), uint32(len(points)), decisionPath, span.ContextID(),
	)
	if !rowOK || !compiler.admitDiagnosticObservation(row, points, nil) {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticRouteGuard, rowIndex, -1)
	}
	return programconstruction.Fault{}
}

func (compiler *compiler) copyUnresolvedTypeObservationsFailure() programconstruction.Fault {
	view := compiler.input.Program.Static()
	types := view.StaticTypes()
	references := view.References()
	for index := 0; index < types.Count(); index++ {
		ref, refOK := types.At(index)
		if !refOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		term := ref.Term()
		resolution, target, rootTerm, relationOK := references.Get(term)
		if !relationOK || resolution != staticrefs.Unresolved || target != 0 {
			continue
		}
		location, locationOK := compiler.input.Program.Source().Identity().Span(term)
		count, countOK := references.SourceCount(term)
		if !locationOK || !programdiagnostic.ValidSpan(location) || !countOK || count == 0 {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		path := make([]string, count)
		for pathIndex := range path {
			key, keyOK := references.SourceAt(term, pathIndex)
			componentLiteral, componentOK := compiler.input.Program.Source().Keys().Exact(key)
			componentOK = componentOK && componentLiteral.Kind == keyspace.LiteralString
			component := componentLiteral.String
			if !keyOK || !componentOK || component == "" {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, pathIndex)
			}
			path[pathIndex] = component
		}
		root := identity.ContentID{}
		if len(path) == 1 {
			if rootTerm != 0 {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
			}
		} else {
			if rootTerm == 0 || keyspace.TermFamily(rootTerm) != keyspace.FamilyCell {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
			}
			var rootOK bool
			root, rootOK = staticquery.ScopeID(compiler.input.Program.ContentID(), rootTerm)
			if !rootOK {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
			}
		}
		reference, referenceOK := staticquery.TypeReferenceID(compiler.input.Program.ContentID(), ref)
		if !referenceOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		row, rowOK := programdiagnostic.NewDiagnosticObservationTypeReferenceUnresolved(
			programdiagnostic.TypeReferenceUnresolvedIdentity(compiler.input.Program.ContentID(), location, reference, root, path),
			location, uint32(len(compiler.diagnosticPaths)), uint32(len(path)), reference, root,
		)
		if !rowOK || !compiler.admitDiagnosticObservation(row, nil, path) {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
	}
	return programconstruction.Fault{}
}

// copyUnresolvedValueObservationsFailure consumes only Flow's sparse
// implicit-read denominator. It never scans names or infers binder absence
// from the ordinary Read catalog.
func (compiler *compiler) copyUnresolvedValueObservationsFailure() programconstruction.Fault {
	reads := compiler.input.Program.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.ImplicitCount(); index++ {
		term, termOK := reads.ImplicitAt(index)
		owner, sourceTerm, implicit, relationOK := reads.Get(term)
		ordinal := keyspace.TermOrdinal(term)
		readID, issuedTerm, readOK := programstorage.ReadIdentityAt(compiler.input.Program, int(ordinal-1))
		cellID, cellOK := rowidentity.StorageCellID(compiler.input.Program.ContentID(), compiler.input.Program.Flow(), sourceTerm)
		kind, body, key, cellRelationOK := compiler.input.Program.Flow().Authored().Storage().Cells().Get(sourceTerm)
		literal, literalOK := compiler.input.Program.Source().Keys().Exact(key)
		location, locationOK := compiler.input.Program.Source().Identity().Span(term)
		if !termOK || !relationOK || !implicit || owner == 0 || ordinal == 0 || !readOK ||
			issuedTerm != term || !readID.Available() || !cellOK || !cellID.Available() ||
			!cellRelationOK || kind != authored.CellGlobal || body != 0 || key == 0 ||
			!literalOK || literal.Kind != keyspace.LiteralString || literal.String == "" ||
			!locationOK || !programdiagnostic.ValidSpan(location) {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticStorageRead, index, -1)
		}
		row, rowOK := programdiagnostic.NewDiagnosticObservationValueReferenceUnresolved(
			programdiagnostic.ValueReferenceUnresolvedIdentity(compiler.input.Program.ContentID(), location, readID, cellID, literal.String),
			location, readID, cellID, literal.String,
		)
		if !rowOK || !compiler.admitDiagnosticObservation(row, nil, nil) {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticStorageRead, index, -1)
		}
	}
	return programconstruction.Fault{}
}

// The evidence points of a conformance row are the argument's own evaluation
// finish, not the call's: the measured value is the argument's, and its base
// witness is where that value is established.
//
// copyTypeConformanceObservationsFailure issues one TypeConformance row per
// selected direct-call argument whose formal declares a static type. Selection
// is the sealed DirectFunctions join already stored on the cold Call row: uncalled
// interiors do not emit. Method, tail, generic, and vararg calls stay silent.
func (compiler *compiler) copyTypeConformanceObservationsFailure() programconstruction.Fault {
	if compiler == nil || !compiler.input.Available() {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, -1, -1)
	}
	directFunctions := compiler.input.Program.Flow().DirectFunctions()
	for index, call := range compiler.calls {
		if !call.Available() {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, -1)
		}
		targetBody, targetOK := call.DirectTargetBody()
		if !targetOK {
			continue
		}
		ownerSelected, selectedOK := directFunctions.SelectedBodyPath(call.BodyID())
		if !selectedOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, -1)
		}
		if !ownerSelected {
			continue
		}
		if _, hasTail := call.TailID(); call.Form() != programschema.CallFormPlain || hasTail || call.TypeArgumentCount() != 0 {
			continue
		}
		boundaries := compiler.input.Program.Flow().FunctionBoundaries()
		if boundaries == nil {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, -1)
		}
		callTerm, callTermOK := compiler.input.Program.Flow().Authored().Calls().At(index)
		_, calleeTerm, _, _, callRelationOK := compiler.input.Program.Flow().Authored().Calls().Get(callTerm)
		functionTerm, functionTermOK := compiler.input.Program.Flow().DirectFunctions().Call(callTerm)
		targetBoundary, targetBoundaryOK := boundaries.For(functionTerm)
		calleeSubject, calleeSubjectOK := compiler.input.Program.SubjectSpelling(calleeTerm)
		if !callTermOK || !callRelationOK || !calleeSubjectOK || !functionTermOK || !targetBoundaryOK || !targetBoundary.Available() {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, -1)
		}
		bodyTerm, bodyTermOK := targetBoundary.Body()
		bodyPath, bodyPathOK := compiler.input.Program.Flow().BodyPath(bodyTerm)
		if !bodyTermOK || !bodyPathOK || bodyPath != targetBody {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, -1)
		}
		functionBoundary, functionBoundaryOK := boundaries.ForFunctionBody(bodyTerm)
		if !functionBoundaryOK || !functionBoundary.Available() {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, -1)
		}
		argumentOffset, argumentCount, argumentSpanOK := call.ArgumentSpan()
		_, hasVararg := functionBoundary.Vararg()
		if !argumentSpanOK || hasVararg || functionBoundary.FormalCount() != int(argumentCount) {
			continue
		}
		for argumentIndex := 0; argumentIndex < int(argumentCount); argumentIndex++ {
			if uint64(argumentOffset)+uint64(argumentIndex) >= uint64(len(compiler.input.CallArguments)) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, argumentIndex)
			}
			argument := compiler.input.CallArguments[int(argumentOffset)+argumentIndex]
			if !argument.Available() || argument.CallID() != call.ID() || argument.Index() != uint32(argumentIndex) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, index, argumentIndex)
			}
			formalTerm, formalOK := functionBoundary.FormalAt(argumentIndex)
			declaredTerm, declared, declaredOK := rowidentity.DeclaredStaticType(compiler.input.Program.ContentID(), compiler.input.Program.Static(), formalTerm)
			if !formalOK || !declaredOK {
				continue
			}
			source, memberOK := compiler.input.Program.Flow().CallArgumentSource(argument.ID())
			memberTerm, sourceIndex := source.Term(), source.CallIndex()
			location, locationOK := compiler.input.Program.Source().Identity().Span(memberTerm)
			if !memberOK || !locationOK || !programdiagnostic.ValidSpan(location) || !argument.SpanID().Available() {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, sourceIndex, argumentIndex)
			}
			memberSpan, memberSpanOK := compiler.input.Program.Span(memberTerm)
			memberFinish, memberFinishOK := memberSpan.Finish()
			if !memberSpanOK || !compiler.input.Program.OwnsSpan(memberSpan) || !memberFinishOK || !compiler.input.Program.OwnsSite(memberFinish) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, sourceIndex, argumentIndex)
			}
			pointPaths := compiler.input.Program.Flow().LocalWTO().PointPathsForSite(memberFinish)
			if pointPaths.Count() == 0 {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, sourceIndex, argumentIndex)
			}
			points, pointsOK := compiler.pointPaths(pointPaths)
			if !pointsOK {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, sourceIndex, argumentIndex)
			}
			site := programdiagnostic.DiagnosticObservationSiteCallArgument
			argumentSubject, _ := compiler.input.Program.SubjectSpelling(memberTerm)
			row, rowOK := programdiagnostic.NewDiagnosticObservationTypeConformance(
				programdiagnostic.TypeConformanceIdentity(compiler.input.Program.ContentID(), location, site, call.ID(), argument.MemberID(), declared, argument.SpanID(), uint32(argumentIndex), points),
				location, uint32(len(compiler.diagnosticEvidence)), uint32(len(points)),
				site, call.ID(), argument.MemberID(), declared, argument.SpanID(), uint32(argumentIndex),
				argumentSubject, calleeSubject,
			)
			if !rowOK || !compiler.admitDiagnosticObservation(row, points, nil) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, sourceIndex, argumentIndex)
			}
			if !compiler.copyStructuralMemberConformanceObservationsFailure(call.ID(), argument.MemberID(), declared, declaredTerm, memberTerm) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, sourceIndex, argumentIndex)
			}
		}
	}
	return programconstruction.Fault{}
}

// diagnosticEvidenceLinearScanLimit is the width below which a direct
// pairwise scan over evidence beats a map: observed evidence widths across
// the fixture corpus are 0 or 1, and a benchmarked comparison shows the
// linear scan winning through width 8; this bound keeps the same scan sound
// at any width by falling back to the reused compiler-level scratch map
// above it, so the row's own admission cost never regresses on a
// pathological evidence count.
const diagnosticEvidenceLinearScanLimit = 16

// validUniqueEvidence reports whether evidence holds only available, distinct
// points. Below diagnosticEvidenceLinearScanLimit it scans in place with no
// allocation; above it, it dedups through compiler.diagnosticEvidenceScratch,
// a map reused and cleared per row instead of allocated per row.
func (compiler *compiler) validUniqueEvidence(evidence []identity.ContentID) bool {
	if len(evidence) <= diagnosticEvidenceLinearScanLimit {
		for index, point := range evidence {
			if !point.Available() {
				return false
			}
			for prior := 0; prior < index; prior++ {
				if evidence[prior] == point {
					return false
				}
			}
		}
		return true
	}
	if compiler.diagnosticEvidenceScratch == nil {
		compiler.diagnosticEvidenceScratch = make(map[identity.ContentID]struct{}, len(evidence))
	} else {
		clear(compiler.diagnosticEvidenceScratch)
	}
	for _, point := range evidence {
		if !point.Available() {
			return false
		}
		if _, duplicate := compiler.diagnosticEvidenceScratch[point]; duplicate {
			return false
		}
		compiler.diagnosticEvidenceScratch[point] = struct{}{}
	}
	return true
}

func (compiler *compiler) admitDiagnosticObservation(
	row programdiagnostic.DiagnosticObservation,
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
	if !compiler.validUniqueEvidence(evidence) {
		return false
	}
	for _, component := range path {
		if component == "" {
			return false
		}
	}
	for _, point := range evidence {
		child, childOK := programdiagnostic.NewDiagnosticEvidence(point)
		if !childOK {
			return false
		}
		compiler.diagnosticEvidence = append(compiler.diagnosticEvidence, child)
	}
	for _, component := range path {
		child, childOK := programdiagnostic.NewDiagnosticPath(component)
		if !childOK {
			return false
		}
		compiler.diagnosticPaths = append(compiler.diagnosticPaths, child)
	}
	compiler.diagnosticObservationByID[id] = len(compiler.diagnosticObservations)
	compiler.diagnosticObservations = append(compiler.diagnosticObservations, row)
	return true
}

func equalDiagnosticObservationCanonical(left, right programdiagnostic.DiagnosticObservation, evidence []programdiagnostic.DiagnosticEvidence, paths []programdiagnostic.DiagnosticPath, wantEvidence []identity.ContentID, wantPath []string) bool {
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
		left.CalleeName() == right.CalleeName() &&
		left.Site() == right.Site() && left.OwnerID() == right.OwnerID() && left.MeasuredValueID() == right.MeasuredValueID() &&
		left.DeclaredStaticTypeID() == right.DeclaredStaticTypeID() && left.SpanID() == right.SpanID()
}

// copyWriteConformanceObservationsFailure is the reassignment half of the same
// relation: a write whose target cell was authored with a declared type is
// measured against that declaration, with the written value's own evaluation
// finish as its evidence. An index or lens write has no declared cell and
// contributes no row.
func (compiler *compiler) copyWriteConformanceObservationsFailure() programconstruction.Fault {
	if compiler == nil || !compiler.input.Available() {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, -1, -1)
	}
	directFunctions := compiler.input.Program.Flow().DirectFunctions()
	view := compiler.input.Program.Flow()
	assigns := view.Authored().Storage().Assigns()
	writes := view.Authored().Storage().Writes()
	authoredValues := view.Authored().Values()
	for index := 0; index < assigns.Count(); index++ {
		term, termOK := assigns.At(index)
		owner, valuesTerm, relationOK := assigns.Get(term)
		width, widthOK := assigns.WriteCount(term)
		if !termOK || !relationOK || !widthOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		if !view.Executable().Contains(term) {
			continue
		}
		bodyPath, bodyOK := view.BodyPath(owner)
		if !bodyOK || !bodyPath.Available() {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		ownerSelected, selectedOK := directFunctions.SelectedBodyPath(bodyPath)
		if !selectedOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		if !ownerSelected {
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
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, position)
			}
			declaredTerm, declared, declaredOK := rowidentity.DeclaredStaticType(compiler.input.Program.ContentID(), compiler.input.Program.Static(), target)
			memberTerm, memberOK := authoredValues.Member(valuesTerm, position)
			member, memberRowOK := compiler.valueMemberAt(valueRow, position)
			if !declaredOK || !memberOK || !memberRowOK || !member.Available() {
				continue
			}
			if !compiler.admitConformanceObservation(programdiagnostic.DiagnosticObservationSiteAssignment, assignmentID, member.ID(), declared, memberTerm, uint32(position)) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, position)
			}
			if !compiler.copyStructuralMemberConformanceObservationsFailure(assignmentID, member.ID(), declared, declaredTerm, memberTerm) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, position)
			}
		}
	}
	return programconstruction.Fault{}
}

// admitConformanceObservation mints and admits one conformance row from the
// coordinates every site shares: the owning statement, the measured value, the
// declaration, and the measured value's own evaluation span.
func (compiler *compiler) admitConformanceObservation(site programdiagnostic.DiagnosticObservationSite, owner, value, declared identity.ContentID, measured keyspace.Term, position uint32) bool {
	subject, _ := compiler.input.Program.SubjectSpelling(measured)
	location, locationOK := compiler.input.Program.Source().Identity().Span(measured)
	span, spanOK := compiler.input.Program.Span(measured)
	finish, finishOK := span.Finish()
	if !locationOK || !programdiagnostic.ValidSpan(location) || !spanOK || !compiler.input.Program.OwnsSpan(span) ||
		!finishOK || !compiler.input.Program.OwnsSite(finish) {
		return false
	}
	pointPaths := compiler.input.Program.Flow().LocalWTO().PointPathsForSite(finish)
	if pointPaths.Count() == 0 {
		return false
	}
	points, pointsOK := compiler.pointPaths(pointPaths)
	if !pointsOK {
		return false
	}
	row, rowOK := programdiagnostic.NewDiagnosticObservationTypeConformance(
		programdiagnostic.TypeConformanceIdentity(compiler.input.Program.ContentID(), location, site, owner, value, declared, span.ContextID(), position, points),
		location, uint32(len(compiler.diagnosticEvidence)), uint32(len(points)),
		site, owner, value, declared, span.ContextID(), position, subject,
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
func (compiler *compiler) copyAssignmentConformanceObservationsFailure() programconstruction.Fault {
	if compiler == nil || !compiler.input.Available() {
		return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, -1, -1)
	}
	directFunctions := compiler.input.Program.Flow().DirectFunctions()
	view := compiler.input.Program.Flow()
	binds := view.Authored().Storage().Binds()
	authoredValues := view.Authored().Values()
	for index := 0; index < binds.Count(); index++ {
		term, termOK := binds.At(index)
		owner, valuesTerm, relationOK := binds.Get(term)
		width, widthOK := compiler.input.Program.Source().Binds().Len(term)
		if !termOK || !relationOK || !widthOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		if !view.Executable().Contains(term) {
			continue
		}
		bodyPath, bodyOK := view.BodyPath(owner)
		if !bodyOK || !bodyPath.Available() {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		ownerSelected, selectedOK := directFunctions.SelectedBodyPath(bodyPath)
		if !selectedOK {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		if !ownerSelected {
			continue
		}
		bindID, bindIDOK := rowidentity.StorageBindID(compiler.input.Program, compiler.input.Program.ContentID(), index)
		if !bindIDOK || !bindID.Available() {
			return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, -1)
		}
		valueRow, valueRowOK := compiler.valueRowForTerm(valuesTerm)
		if !valueRowOK {
			continue
		}
		for position := 0; position < width; position++ {
			cellTerm, cellOK := compiler.input.Program.Source().Binds().At(term, position)
			if !cellOK {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, position)
			}
			declaredTerm, declared, declaredOK := rowidentity.DeclaredStaticType(compiler.input.Program.ContentID(), compiler.input.Program.Static(), cellTerm)
			memberTerm, memberOK := authoredValues.Member(valuesTerm, position)
			member, memberRowOK := compiler.valueMemberAt(valueRow, position)
			if !declaredOK || !memberOK || !memberRowOK || !member.Available() {
				continue
			}
			if !compiler.admitConformanceObservation(programdiagnostic.DiagnosticObservationSiteAssignment, bindID, member.ID(), declared, memberTerm, uint32(position)) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, position)
			}
			if !compiler.copyStructuralMemberConformanceObservationsFailure(bindID, member.ID(), declared, declaredTerm, memberTerm) {
				return programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticUnavailable, index, position)
			}
		}
	}
	return programconstruction.Fault{}
}
