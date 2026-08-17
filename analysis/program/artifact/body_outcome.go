package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// OutcomeKind is the artifact-owned closed Body Outcome vocabulary. It is
// converted exhaustively from Program and is not an alias of Flow's enum.
type OutcomeKind uint8

const (
	OutcomeInvalid OutcomeKind = iota
	OutcomeNormal
	OutcomeReturn
	OutcomeThrow
	OutcomeBreak
	OutcomeGoto
	OutcomeYield
	OutcomeCancel
)

func (kind OutcomeKind) valid() bool { return kind >= OutcomeNormal && kind <= OutcomeCancel }

// BodyRow is one immutable BodyPath and its contiguous ordered Outcome range.
// The physical range is private artifact storage, never a semantic identity.
type BodyRow struct {
	id           identity.ContentID
	context      identity.ContentID
	entry        identity.ContentID
	function     identity.ContentID
	formal       identity.ContentID
	callable     bool
	entryPoints  []identity.ContentID
	roots        []RootRow
	outcomeStart uint32
	outcomeEnd   uint32
	sealed       bool
}

// RootRow is one Body-owned sealed executable root descriptor. The semantic
// ID is issued by Program while Flow's semantic-path proof is live; no raw
// Source/Flow Term, authored Root, or Span is retained in the artifact.
type RootRow struct {
	id     identity.ContentID
	family keyspace.Family
}

func (row RootRow) Available() bool {
	return row.id.Available() && row.family != keyspace.FamilyInvalid
}
func (row RootRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row RootRow) Family() keyspace.Family {
	if !row.Available() {
		return keyspace.FamilyInvalid
	}
	return row.family
}

func (row BodyRow) Available() bool {
	return row.sealed && row.id.Available() && row.context.Available() && row.entry.Available() && row.outcomeEnd >= row.outcomeStart &&
		(!row.callable || row.function.Available() && row.formal.Available()) &&
		(row.callable || !row.function.Available() && !row.formal.Available())
}

// EntryID is the exact Program-local semantic identity of this Body's entry
// Site. EntryPointAt exposes its local WTO materializations separately; Link
// and Runtime must not reconstruct this boundary from a Body path.
func (row BodyRow) EntryID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.entry
}

func (row BodyRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

// ContextID is the exact Program Body boundary identity captured during
// compilation.  It is distinct from ID, which is the lexical Body path.
func (row BodyRow) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.context
}

// Callable reports whether this Body has the exact transformer Function
// proof required by a closure allocation target.
func (row BodyRow) Callable() bool { return row.Available() && row.callable }

// FunctionContextID and CallFormalID expose the parent-issued IDs needed to
// construct Call target rows.  Non-callable bodies fail closed.
func (row BodyRow) FunctionContextID() (identity.ContentID, bool) {
	return row.function, row.Callable() && row.function.Available()
}
func (row BodyRow) CallFormalID() (identity.ContentID, bool) {
	return row.formal, row.Callable() && row.formal.Available()
}

// CallTargetRow is the exact closure-allocation-to-callable-body proof.  It
// is artifact data, not a domain coordinate: all fields are Program-issued
// IDs captured while the allocation and Body proofs were live.
type CallTargetRow struct {
	allocation identity.ContentID
	body       identity.ContentID
	context    identity.ContentID
	function   identity.ContentID
	formal     identity.ContentID
	sealed     bool
}

func (row CallTargetRow) Available() bool {
	return row.sealed && row.allocation.Available() && row.body.Available() && row.context.Available() && row.function.Available() && row.formal.Available()
}
func (row CallTargetRow) AllocationID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.allocation
}
func (row CallTargetRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row CallTargetRow) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.context
}
func (row CallTargetRow) FunctionContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.function
}
func (row CallTargetRow) CallFormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.formal
}

func (row BodyRow) OutcomeCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.outcomeEnd - row.outcomeStart)
}

// EntryPointCount and EntryPointAt expose the exact existing LocalWTO point
// memberships for this Body's entry Site. They are retained from the sealed
// Program attachment row and are never derived from a Body path.
func (row BodyRow) EntryPointCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.entryPoints)
}
func (row BodyRow) EntryPointAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.entryPoints) {
		return identity.ContentID{}, false
	}
	return row.entryPoints[index], row.entryPoints[index].Available()
}

// RootCount and RootAt expose the exact dense executable-root denominator in
// source order. These are artifact rows, never a runtime Program/Flow query.
func (row BodyRow) RootCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.roots)
}
func (row BodyRow) RootAt(index int) (RootRow, bool) {
	if !row.Available() || index < 0 || index >= len(row.roots) {
		return RootRow{}, false
	}
	root := row.roots[index]
	return root, root.Available()
}

// OutcomeRow is one immutable Body-owned semantic Outcome. Target and
// propagation are optional semantic references; returnStart/returnEnd name a
// contiguous range in the artifact's ordered ReturnValue plane.
type OutcomeRow struct {
	id             identity.ContentID
	body           identity.ContentID
	target         identity.ContentID
	propagation    identity.ContentID
	kind           OutcomeKind
	hasTarget      bool
	hasPropagation bool
	returnStart    uint32
	returnEnd      uint32
	points         []identity.ContentID
	sealed         bool
}

func (row OutcomeRow) Available() bool {
	if !row.sealed || !row.id.Available() || !row.body.Available() || !row.kind.valid() || row.returnEnd < row.returnStart ||
		row.hasPropagation != row.propagation.Available() {
		return false
	}
	switch row.kind {
	case OutcomeBreak, OutcomeGoto:
		if !row.hasTarget || !row.target.Available() {
			return false
		}
	default:
		if row.hasTarget || row.target.Available() {
			return false
		}
	}
	return row.returnStart == row.returnEnd || row.kind == OutcomeReturn
}

func (row OutcomeRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row OutcomeRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row OutcomeRow) Kind() OutcomeKind {
	if !row.Available() {
		return OutcomeInvalid
	}
	return row.kind
}

func (row OutcomeRow) TargetID() (identity.ContentID, bool) {
	return row.target, row.Available() && row.hasTarget
}

func (row OutcomeRow) PropagationID() (identity.ContentID, bool) {
	return row.propagation, row.Available() && row.hasPropagation
}

func (row OutcomeRow) ReturnValueCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.returnEnd - row.returnStart)
}

// PointCount and PointAt expose the exact LocalWTO memberships of the
// Outcome Causal Site. A terminal without a Causal Site, or a sealed Site that
// is intentionally unscheduled, retains no point membership; callers must
// fail closed rather than invent one from Outcome ID.
func (row OutcomeRow) PointCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.points)
}
func (row OutcomeRow) PointAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.points) {
		return identity.ContentID{}, false
	}
	return row.points[index], row.points[index].Available()
}

// ReturnValue is one ordered reference to an already-copied Values row. The
// same Values ID may occur under several propagated Return Outcomes; order and
// multiplicity belong to each Outcome range and are intentionally retained.
type ReturnValue struct{ id identity.ContentID }

func (value ReturnValue) Available() bool { return value.id.Available() }
func (value ReturnValue) ID() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.id
}

func outcomeKind(kind flowkind.OutcomeKind) (OutcomeKind, bool) {
	switch kind {
	case flowkind.OutcomeNormal:
		return OutcomeNormal, true
	case flowkind.OutcomeReturn:
		return OutcomeReturn, true
	case flowkind.OutcomeThrow:
		return OutcomeThrow, true
	case flowkind.OutcomeBreak:
		return OutcomeBreak, true
	case flowkind.OutcomeGoto:
		return OutcomeGoto, true
	case flowkind.OutcomeYield:
		return OutcomeYield, true
	case flowkind.OutcomeCancel:
		return OutcomeCancel, true
	default:
		return OutcomeInvalid, false
	}
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }

func (compiler *compiler) copyBodiesAndOutcomesFailure() CompileFailure {
	bodyCount := compiler.input.BodyCount()
	if bodyCount <= 0 || !fitsUint32(bodyCount) {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	valueIDs := make(map[identity.ContentID]struct{}, len(compiler.values))
	for _, row := range compiler.values {
		if !row.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, -1, -1, CompileReasonReturnValueReference)
		}
		valueIDs[row.id] = struct{}{}
	}
	bodies := make([]BodyRow, bodyCount)
	outcomes := make([]OutcomeRow, 0)
	returnValues := make([]ReturnValue, 0)
	flowView := compiler.input.Flow()
	boundaries := flowView.FunctionBoundaries()
	bodyReturns := flowView.BodyReturns()
	outcomesView := flowView.Outcomes()
	sites := flowView.Causal().Sites()
	seenBodies := make(map[identity.ContentID]struct{}, bodyCount)
	seenBodyContexts := make(map[identity.ContentID]struct{}, bodyCount)
	seenOutcomes := make(map[identity.ContentID]int)

	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		body, ok := compiler.input.BodyAt(bodyIndex)
		if !ok || !body.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		if !compiler.input.OwnsBody(body) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyForeign)
		}
		bodyID := body.PathID()
		if !bodyID.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyIdentity)
		}
		if _, duplicate := seenBodies[bodyID]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		context := body.ContextID()
		bodyBoundary, bodyBoundaryOK := boundaries.ResolveBodyContextID(context)
		bodyTerm, bodyTermOK := bodyBoundary.Body()
		if !bodyBoundaryOK || !bodyBoundary.Available() || !bodyTermOK || bodyBoundary.ContextID() != context || !boundaries.OwnsBody(bodyBoundary) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		entry, entryOK := body.EntrySite()
		entryID := entry.PathID()
		entryPoints := compiler.pointIDs(entry)
		if !context.Available() || !entryOK || !entryID.Available() || !compiler.input.OwnsSite(entry) || len(entryPoints) == 0 {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenBodyContexts[context]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		functionID, formalID := identity.ContentID{}, identity.ContentID{}
		callable := false
		if function, functionOK := body.Function(); functionOK {
			formal, formalOK := body.CallTarget()
			var functionIDOK bool
			functionID, functionIDOK = compiler.input.FunctionID(function)
			if formalOK {
				formalID, formalOK = formal.ID()
			}
			if !formalOK || !functionIDOK || !functionID.Available() || !formalID.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			callable = true
		}
		rootCatalog, rootsOK := body.ExecutableRoots()
		rootCount := rootCatalog.Count()
		if !rootsOK || !compiler.input.OwnsExecutableRoots(rootCatalog) || !fitsUint32(rootCount) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		roots := make([]RootRow, rootCount)
		seenRoots := make(map[identity.ContentID]struct{}, rootCount)
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			root, rootOK := rootCatalog.At(rootIndex)
			if !rootOK || !compiler.input.OwnsExecutableRoot(root) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyUnavailable)
			}
			row := RootRow{id: root.ID(), family: root.Family()}
			if !row.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyIdentity)
			}
			if _, duplicate := seenRoots[row.id]; duplicate {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyDuplicate)
			}
			seenRoots[row.id] = struct{}{}
			roots[rootIndex] = row
		}
		seenBodies[bodyID] = struct{}{}
		seenBodyContexts[context] = struct{}{}
		if !fitsUint32(len(outcomes)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		start := uint32(len(outcomes))

		returned, hasReturn := bodyReturns.ForBody(bodyBoundary)
		returnedID := identity.ContentID{}
		if hasReturn {
			returnSite, siteOK := returned.Outcome()
			returnedTerm, termOK := returnSite.Term()
			returnExit, _, returnOK := bodyBoundary.OutcomeForTerm(returnedTerm)
			if !siteOK || !sites.Owns(returnSite) || !termOK || !returnOK || returnExit.Outcome != returnedTerm || returnExit.Kind != flowkind.OutcomeReturn || returnExit.Target != 0 {
				hasReturn = false
			} else {
				returnedID, _ = flowView.SemanticTermPath(returnedTerm)
			}
		}
		matchedReturn := false
		outcomeCount := bodyBoundary.OutcomeCount()
		if outcomeCount <= 0 || !fitsUint32(outcomeCount) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		for outcomeIndex := 0; outcomeIndex < outcomeCount; outcomeIndex++ {
			exit, outcomeOK := bodyBoundary.OutcomeAt(outcomeIndex)
			if !outcomeOK || exit.Outcome == 0 {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeUnavailable)
			}
			ownerBoundary, ownerOK := boundaries.ForOutcome(exit.Outcome)
			ownerBody, ownerBodyOK := ownerBoundary.Body()
			ownerExit, ownerOrdinal, ownerExitOK := ownerBoundary.OutcomeForTerm(exit.Outcome)
			metadata, metadataOK := outcomesView.Get(exit.Outcome)
			if !ownerOK || !ownerBoundary.Available() || !boundaries.OwnsBody(ownerBoundary) || !ownerBodyOK || ownerBody != bodyTerm || !ownerExitOK || ownerOrdinal != outcomeIndex || ownerExit.Outcome != exit.Outcome || !metadataOK || metadata.Body != bodyTerm || metadata.Kind != exit.Kind || metadata.Target != exit.Target {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeForeign)
			}
			if site, siteOK := sites.ForTerm(exit.Outcome); siteOK && (!site.Available() || !sites.Owns(site)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeUnavailable)
			}
			outcomeID, outcomePathOK := flowView.SemanticTermPath(exit.Outcome)
			if !outcomePathOK || !outcomeID.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeIdentity)
			}
			if _, duplicate := seenOutcomes[outcomeID]; duplicate {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeDuplicate)
			}
			kind, converted := outcomeKind(exit.Kind)
			if !converted {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeKind)
			}
			target := identity.ContentID{}
			hasTarget := false
			switch kind {
			case OutcomeBreak:
				if keyspace.TermFamily(exit.Target) != keyspace.FamilyLoop {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				var targetOK bool
				target, targetOK = flowView.SemanticTermPath(exit.Target)
				if !targetOK || !target.Available() {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				hasTarget = true
			case OutcomeGoto:
				if keyspace.TermFamily(exit.Target) != keyspace.FamilyLabel {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				var targetOK bool
				target, targetOK = flowView.SemanticTermPath(exit.Target)
				if !targetOK || !target.Available() {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				hasTarget = true
			}

			propagationID := identity.ContentID{}
			if nextTerm, propagated := outcomesView.Propagation(exit.Outcome); propagated {
				nextBoundary, nextOK := boundaries.ForOutcome(nextTerm)
				nextBody, nextBodyOK := nextBoundary.Body()
				nextExit, _, nextExitOK := nextBoundary.OutcomeForTerm(nextTerm)
				nextMetadata, nextMetadataOK := outcomesView.Get(nextTerm)
				nextPath, nextPathOK := flowView.SemanticTermPath(nextTerm)
				nextSite, nextSiteOK := sites.ForTerm(nextTerm)
				if nextSiteOK && (!nextSite.Available() || !sites.Owns(nextSite)) {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomePropagation)
				}
				if !nextOK || !nextBoundary.Available() || !boundaries.OwnsBody(nextBoundary) || !nextBodyOK || nextBody == 0 || !nextExitOK || nextExit.Outcome != nextTerm || !nextMetadataOK || nextMetadata.Body != nextBody || nextExit.Kind != exit.Kind || nextExit.Target != exit.Target || !nextPathOK || !nextPath.Available() {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomePropagation)
				}
				propagationID = nextPath
				if propagationID == outcomeID {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomePropagation)
				}
			}
			points := []identity.ContentID(nil)
			if site, siteOK := sites.ForTerm(exit.Outcome); siteOK {
				if !site.Available() || !sites.Owns(site) {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeAttachment)
				}
				points = compiler.pointIDs(site)
			}

			if !fitsUint32(len(returnValues)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, outcomeIndex, CompileReasonOutcomeRange)
			}
			returnStart := uint32(len(returnValues))
			if hasReturn && outcomeID == returnedID {
				if matchedReturn || kind != OutcomeReturn {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeReturn)
				}
				matchedReturn = true
				for returnIndex := 0; returnIndex < returned.ValuesCount(); returnIndex++ {
					valueSite, valueOK := returned.ValueAt(returnIndex)
					valueTerm, termOK := valueSite.Term()
					valueRow, rowOK := compiler.valueRowForTerm(valueTerm)
					valuesID := valueRow.ID()
					if !valueOK || !valueSite.Available() || !sites.Owns(valueSite) || !termOK || !rowOK || !valuesID.Available() {
						return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, returnIndex, CompileReasonReturnValueUnavailable)
					}
					if _, exists := valueIDs[valuesID]; !exists {
						return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, returnIndex, CompileReasonReturnValueReference)
					}
					returnValues = append(returnValues, ReturnValue{id: valuesID})
				}
			}
			if !fitsUint32(len(returnValues)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, outcomeIndex, CompileReasonOutcomeRange)
			}
			row := OutcomeRow{
				id: outcomeID, body: bodyID, target: target, propagation: propagationID, kind: kind,
				hasTarget: hasTarget, hasPropagation: propagationID.Available(),
				returnStart: returnStart, returnEnd: uint32(len(returnValues)), points: points, sealed: true,
			}
			if !row.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeShape)
			}
			seenOutcomes[outcomeID] = len(outcomes)
			outcomes = append(outcomes, row)
		}
		if hasReturn != matchedReturn || !fitsUint32(len(outcomes)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, -1, CompileReasonOutcomeReturn)
		}
		bodies[bodyIndex] = BodyRow{id: bodyID, context: context, entry: entryID, function: functionID, formal: formalID, callable: callable, entryPoints: entryPoints, roots: roots, outcomeStart: start, outcomeEnd: uint32(len(outcomes)), sealed: true}
		if !bodies[bodyIndex].Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
	}

	for index, row := range outcomes {
		if !row.hasPropagation {
			continue
		}
		nextIndex, exists := seenOutcomes[row.propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next := outcomes[nextIndex]
		if next.kind != row.kind || next.hasTarget != row.hasTarget || next.target != row.target {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, index, -1, CompileReasonOutcomePropagation)
		}
	}
	compiler.bodies, compiler.outcomes, compiler.returnValues = bodies, outcomes, returnValues
	return CompileFailure{}
}

// copyCallTargetsFailure captures the exact closure-allocation mapping once
// from the canonical Flow allocation and function-boundary rows. Call later
// consumes only these immutable IDs and never scans Program construction state.
func (compiler *compiler) copyCallTargetsFailure() CompileFailure {
	if compiler == nil || len(compiler.bodies) == 0 {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	bodyByContext := make(map[identity.ContentID]BodyRow, len(compiler.bodies))
	for index, body := range compiler.bodies {
		if !body.Available() || !body.ContextID().Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		bodyByContext[body.ContextID()] = body
	}
	rows := make([]CallTargetRow, 0)
	seenAllocations := make(map[identity.ContentID]struct{})
	seenBodies := make(map[identity.ContentID]struct{})
	flowView := compiler.input.Flow()
	boundaries := flowView.FunctionBoundaries()
	for index, allocation := range compiler.allocationRows {
		if allocation.role != flow.AllocationClosure {
			continue
		}
		boundary, boundaryOK := boundaries.For(allocation.term)
		functionTerm, functionTermOK := boundary.Function()
		bodyTerm, bodyTermOK := boundary.Body()
		body, bodyOK := compiler.input.Body(bodyTerm)
		function, functionOK := body.Function()
		formal, formalOK := body.CallTarget()
		allocationID, bodyID := allocation.template, body.PathID()
		context := body.ContextID()
		functionID, functionIDOK := compiler.input.FunctionID(function)
		formalID, formalIDOK := formal.ID()
		copied, copiedOK := bodyByContext[context]
		owner, authoredBody, _, authoredOK := flowView.Authored().Functions().Get(allocation.term)
		if !boundaryOK || !boundaries.OwnsFunction(boundary) || !functionTermOK || functionTerm != allocation.term || !bodyTermOK || owner == 0 || authoredBody != bodyTerm || !authoredOK || !bodyOK || !functionOK || !functionIDOK || !formalOK || !formalIDOK || !allocationID.Available() || !bodyID.Available() || !context.Available() || !functionID.Available() || !formalID.Available() || !copiedOK || !copied.Callable() || copied.ID() != bodyID || copied.ContextID() != context {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		copiedFunction, copiedFunctionOK := copied.FunctionContextID()
		copiedFormal, copiedFormalOK := copied.CallFormalID()
		if !copiedFunctionOK || !copiedFormalOK || copiedFunction != functionID || copiedFormal != formalID {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenAllocations[allocationID]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenBodies[context]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		seenAllocations[allocationID], seenBodies[context] = struct{}{}, struct{}{}
		rows = append(rows, CallTargetRow{allocation: allocationID, body: bodyID, context: context, function: functionID, formal: formalID, sealed: true})
	}
	compiler.callTargets = rows
	return CompileFailure{}
}
