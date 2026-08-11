package causal

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/program/keyspace"
)

// Matches reports whether r belongs to the exact Source, authored Flow,
// Static, and Module identities supplied by the final assembly. Any
// unavailable identity fails closed.
func Matches(r *Result, sourceID, flowID, staticID, moduleID keyspace.ContentID) bool {
	return r.available() && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// Edges is the typed local-plane view. It owns no storage; every method is a
// projection over the one immutable local Edge plane in Result.
type Edges struct{ result *Result }

// Edges returns the local Edge/reset/body/activation view.
func (r *Result) Edges() Edges { return Edges{result: r} }

func (v Edges) Count() int {
	if !v.result.available() {
		return 0
	}
	return len(v.result.edges.rows)
}

func (v Edges) At(index int) (Edge, bool) {
	if !v.result.available() || index < 0 || index >= len(v.result.edges.rows) {
		return Edge{}, false
	}
	row := v.result.edges.rows[index]
	if !v.result.validEdgeRow(row) {
		return Edge{}, false
	}
	return row.Edge, true
}

func (v Edges) Decision(index int) (keyspace.Term, bool, bool) {
	edge, ok := v.At(index)
	if !ok || edge.Decision == 0 {
		return 0, false, false
	}
	return edge.Decision, edge.Truth, true
}

func (v Edges) Mu(index int) (keyspace.Term, bool) {
	edge, ok := v.At(index)
	if !ok || edge.Mu == 0 {
		return 0, false
	}
	return edge.Mu, true
}

func (v Edges) ResetCount(index int) (int, bool) {
	if !v.result.available() || index < 0 || index >= len(v.result.edges.rows) {
		return 0, false
	}
	edge := v.result.edges.rows[index]
	if !v.result.validEdgeRow(edge) {
		return 0, false
	}
	if edge.Mu == 0 || edge.resetPast < edge.resetStart {
		return 0, false
	}
	start, end, ok := v.result.muRange(edge.Mu)
	if !ok || uint64(edge.resetPast) > uint64(end-start) || uint64(start)+uint64(edge.resetPast) > uint64(len(v.result.reset.streams)) {
		return 0, false
	}
	count := uint64(edge.resetPast - edge.resetStart)
	if count > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(count), true
}

func (v Edges) ResetAt(index, offset int) (keyspace.Term, bool) {
	if !v.result.available() || index < 0 || index >= len(v.result.edges.rows) || offset < 0 {
		return 0, false
	}
	edge := v.result.edges.rows[index]
	if !v.result.validEdgeRow(edge) {
		return 0, false
	}
	if edge.Mu == 0 || edge.resetPast < edge.resetStart ||
		uint64(offset) >= uint64(edge.resetPast-edge.resetStart) {
		return 0, false
	}
	start, end, ok := v.result.muRange(edge.Mu)
	if !ok || edge.resetPast > end-start || edge.resetStart > edge.resetPast {
		return 0, false
	}
	streamIndex := uint64(start) + uint64(edge.resetStart) + uint64(offset)
	if streamIndex >= uint64(end) || streamIndex >= uint64(len(v.result.reset.streams)) {
		return 0, false
	}
	term := v.result.reset.streams[streamIndex]
	if !isDecision(term) {
		return 0, false
	}
	return term, true
}

func (v Edges) ResetContains(index int, decision keyspace.Term) bool {
	if !v.result.available() || index < 0 || index >= len(v.result.edges.rows) || !isDecision(decision) {
		return false
	}
	edge := v.result.edges.rows[index]
	if !v.result.validEdgeRow(edge) {
		return false
	}
	if edge.Mu == 0 || edge.resetPast < edge.resetStart {
		return false
	}
	start, end, ok := v.result.muRange(edge.Mu)
	if !ok || edge.resetStart > edge.resetPast || uint64(edge.resetPast) > uint64(end-start) {
		return false
	}
	family, ordinal := keyspace.TermFamily(decision), keyspace.TermOrdinal(decision)
	if ordinal == 0 || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		uint64(ordinal) >= uint64(len(v.result.reset.decisionHead[family])) ||
		v.result.reset.decisionHead[family][ordinal] != edge.Mu {
		return false
	}
	ranks := v.result.reset.decisionRank[family]
	if uint64(ordinal) >= uint64(len(ranks)) {
		return false
	}
	rank := ranks[ordinal]
	return uint64(rank) < uint64(end-start) && edge.resetStart <= rank && rank < edge.resetPast
}

func (v Edges) BodyCount(body keyspace.Term) (int, bool) {
	if !v.result.available() || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.edges.bodyRanges)) {
		return 0, false
	}
	rangeValue := v.result.edges.bodyRanges[ordinal]
	if rangeValue.end < rangeValue.start || uint64(rangeValue.end) > uint64(len(v.result.edges.bodyIndexes)) {
		return 0, false
	}
	return int(rangeValue.end - rangeValue.start), true
}

func (v Edges) BodyAt(body keyspace.Term, index int) (Edge, bool) {
	if !v.result.available() || index < 0 || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return Edge{}, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.edges.bodyRanges)) {
		return Edge{}, false
	}
	rangeValue := v.result.edges.bodyRanges[ordinal]
	if rangeValue.end < rangeValue.start || uint64(rangeValue.end) > uint64(len(v.result.edges.bodyIndexes)) ||
		uint64(index) >= uint64(rangeValue.end-rangeValue.start) {
		return Edge{}, false
	}
	edgeIndex := v.result.edges.bodyIndexes[rangeValue.start+uint32(index)]
	if uint64(edgeIndex) >= uint64(len(v.result.edges.rows)) {
		return Edge{}, false
	}
	row := v.result.edges.rows[edgeIndex]
	if !v.result.validEdgeRow(row) {
		return Edge{}, false
	}
	return row.Edge, true
}

func (v Edges) ActivationCount(body keyspace.Term) (int, bool) {
	if !v.result.available() || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.edges.activationRanges)) ||
		uint64(ordinal) >= uint64(len(v.result.edges.activationRoots)) || !v.result.edges.activationRoots[ordinal] {
		return 0, false
	}
	rangeValue := v.result.edges.activationRanges[ordinal]
	if rangeValue.end < rangeValue.start || uint64(rangeValue.end) > uint64(len(v.result.edges.activationIndexes)) {
		return 0, false
	}
	return int(rangeValue.end - rangeValue.start), true
}

func (v Edges) ActivationAt(body keyspace.Term, index int) (Edge, bool) {
	if !v.result.available() || index < 0 || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return Edge{}, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.edges.activationRanges)) ||
		uint64(ordinal) >= uint64(len(v.result.edges.activationRoots)) || !v.result.edges.activationRoots[ordinal] {
		return Edge{}, false
	}
	rangeValue := v.result.edges.activationRanges[ordinal]
	if rangeValue.end < rangeValue.start || uint64(rangeValue.end) > uint64(len(v.result.edges.activationIndexes)) ||
		uint64(index) >= uint64(rangeValue.end-rangeValue.start) {
		return Edge{}, false
	}
	edgeIndex := v.result.edges.activationIndexes[rangeValue.start+uint32(index)]
	if uint64(edgeIndex) >= uint64(len(v.result.edges.rows)) {
		return Edge{}, false
	}
	row := v.result.edges.rows[edgeIndex]
	if !v.result.validEdgeRow(row) {
		return Edge{}, false
	}
	return row.Edge, true
}

// Boundaries is the typed CallBoundary view.
type Boundaries struct{ result *Result }

// Boundaries returns the typed dynamic-call view.
func (r *Result) Boundaries() Boundaries { return Boundaries{result: r} }

func (v Boundaries) Count() int {
	if !v.result.available() {
		return 0
	}
	return len(v.result.boundaries.rows)
}

func (v Boundaries) At(index int) (CallBoundary, bool) {
	if !v.result.available() || index < 0 || index >= len(v.result.boundaries.rows) {
		return CallBoundary{}, false
	}
	row := v.result.boundaries.rows[index]
	if !v.result.validBoundaryRow(row) {
		return CallBoundary{}, false
	}
	return row.CallBoundary, true
}

func (v Boundaries) For(call keyspace.Term) (CallBoundary, bool) {
	if !v.result.available() || keyspace.TermFamily(call) != keyspace.FamilyCall {
		return CallBoundary{}, false
	}
	ordinal := keyspace.TermOrdinal(call)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.boundaries.callSlots)) {
		return CallBoundary{}, false
	}
	slot := v.result.boundaries.callSlots[ordinal]
	if slot == 0 || uint64(slot-1) >= uint64(len(v.result.boundaries.rows)) {
		return CallBoundary{}, false
	}
	row := v.result.boundaries.rows[slot-1]
	if !v.result.validBoundaryRow(row) {
		return CallBoundary{}, false
	}
	return row.CallBoundary, true
}

// Successors is the one allocation-free combined local-plus-boundary
// traversal view.
type Successors struct{ result *Result }

// Successors returns the exact union traversal view.
func (r *Result) Successors() Successors { return Successors{result: r} }

func (v Successors) Count(from keyspace.Term) int {
	start, end, ok := v.result.successorRange(from)
	if !ok || uint64(end-start) > uint64(^uint(0)>>1) {
		return 0
	}
	return int(end - start)
}

func (v Successors) At(from keyspace.Term, index int) (Successor, bool) {
	start, end, ok := v.result.successorRange(from)
	if !ok || index < 0 || uint64(index) >= uint64(end-start) {
		return Successor{}, false
	}
	at := uint64(start) + uint64(index)
	if at >= uint64(len(v.result.index.refs)) {
		return Successor{}, false
	}
	return v.result.successorForRef(v.result.index.refs[at])
}

// AssignmentPredecessor returns the owner-issued Successor for the reverse
// commit edge that terminates one authored Write. Evaluation routes may also
// enter a Write, so this projection is sealed at the assignment commit
// emission site rather than inferred from a generic incoming-route scan.
func (v Successors) AssignmentPredecessor(write keyspace.Term) (Successor, bool) {
	if v.result == nil || !v.result.available() || keyspace.TermFamily(write) != keyspace.FamilyWrite {
		return Successor{}, false
	}
	ordinal := keyspace.TermOrdinal(write)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.index.writeCommitRefs)) {
		return Successor{}, false
	}
	ref := v.result.index.writeCommitRefs[ordinal]
	if !ref.local || ref.arm != BoundaryLocal || uint64(ref.index) >= uint64(len(v.result.edges.rows)) {
		return Successor{}, false
	}
	successor, ok := v.result.successorForRef(ref)
	if !ok || !successor.IsLocal() || successor.To != write {
		return Successor{}, false
	}
	return successor, true
}

// successorForRef projects one already-indexed route. The ref is an existing
// entry in the sealed combined successor plane; no route row, endpoint copy,
// or source-local ordinal is manufactured for this projection.
func (r *Result) successorForRef(ref successorRef) (Successor, bool) {
	if r == nil || !r.available() {
		return Successor{}, false
	}
	if !isKnownArm(ref.arm) || ref.local != isLocalArm(ref.arm) {
		return Successor{}, false
	}
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) {
			return Successor{}, false
		}
		row := r.edges.rows[ref.index]
		if !r.validEdgeRow(row) {
			return Successor{}, false
		}
		edge := row.Edge
		id, idOK := r.routeIdentityFastForRef(ref)
		if r.routesReady && !idOK {
			return Successor{}, false
		}
		return Successor{From: edge.From, To: edge.To, Decision: edge.Decision, Truth: edge.Truth,
			Mu: edge.Mu, Arm: BoundaryLocal, routeDigest: ref.routeDigest, route: id,
			result: r, edgeIndex: ref.index, edgeIndexValid: idOK}, true
	}
	if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
		return Successor{}, false
	}
	row := r.boundaries.rows[ref.index]
	if !r.validBoundaryRow(row) {
		return Successor{}, false
	}
	boundary := row.CallBoundary
	to, decision, truth, armOK := boundarySuccessor(boundary, ref.arm)
	if !armOK {
		return Successor{}, false
	}
	id, idOK := r.routeIdentityFastForRef(ref)
	if r.routesReady && !idOK {
		return Successor{}, false
	}
	return Successor{From: boundary.Call, To: to, Decision: decision, Truth: truth,
		Arm: ref.arm, routeDigest: ref.routeDigest, route: id, result: r,
		edgeIndex: ref.index, edgeIndexValid: idOK}, true
}

// IsBoundary reports whether the route belongs to the typed call-boundary
// arms. The arm vocabulary itself is the closed semantic sum.
func (s Successor) IsBoundary() bool { return isBoundaryArm(s.Arm) }

// IsLocal reports whether the route is a local Edge. Zero and unknown arm
// values are malformed and fail closed.
func (s Successor) IsLocal() bool { return isLocalArm(s.Arm) }

// Identity returns the owner-fenced semantic route identity. A manufactured,
// unavailable, or partially sealed Successor has no identity.
func (s Successor) Identity() (RouteIdentity, bool) {
	if !s.route.available() || s.route.Digest != s.routeDigest {
		return RouteIdentity{}, false
	}
	return s.route, true
}

// ResetCount, ResetAt, and ResetContains are route-local reset capabilities.
// They intentionally do not accept or expose an Edge ordinal.
func (s Successor) ResetCount() (int, bool) {
	if !s.IsLocal() || s.result == nil || !s.edgeIndexValid {
		return 0, false
	}
	return s.result.Edges().ResetCount(int(s.edgeIndex))
}

func (s Successor) ResetAt(offset int) (keyspace.Term, bool) {
	if !s.IsLocal() || s.result == nil || !s.edgeIndexValid {
		return 0, false
	}
	return s.result.Edges().ResetAt(int(s.edgeIndex), offset)
}

func (s Successor) ResetContains(decision keyspace.Term) bool {
	return s.IsLocal() && s.result != nil && s.edgeIndexValid && s.result.Edges().ResetContains(int(s.edgeIndex), decision)
}

// Resolve performs owner-fenced semantic lookup. The inverse is sorted by
// digest and points back into the existing two typed planes.
func (v Successors) Resolve(identity RouteIdentity) (Successor, bool) {
	if v.result == nil || !v.result.routesReady || !identity.available() ||
		!Matches(v.result, identity.SourceID, identity.FlowID, identity.StaticID, identity.ModuleID) {
		return Successor{}, false
	}
	left := sort.Search(len(v.result.routeIndex), func(index int) bool {
		return bytes.Compare(v.result.routeIndex[index].digest[:], identity.Digest[:]) >= 0
	})
	found := -1
	for index := left; index < len(v.result.routeIndex) && v.result.routeIndex[index].digest == identity.Digest; index++ {
		if found != -1 {
			// A duplicate/ambiguous inverse is never selected by physical order.
			return Successor{}, false
		}
		found = index
	}
	if found == -1 {
		return Successor{}, false
	}
	ref := v.result.routeIndex[found].ref
	candidate, ok := v.result.routeIdentityFastForRef(ref)
	if !ok || !compareRouteID(candidate, identity) {
		return Successor{}, false
	}
	entry := v.result.routeIndex[found]
	return v.result.successorForRef(entry.ref)
}

func (r *Result) validResultTerm(term keyspace.Term) bool {
	if !r.available() {
		return false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 && ordinal <= r.index.familyCounts[family]
}

func (r *Result) validEdgeRow(row edgeRow) bool {
	if !r.validResultTerm(row.From) || !r.validResultTerm(row.To) || keyspace.TermFamily(row.From) == keyspace.FamilyCall {
		return false
	}
	// Keep the public row fence identical to retained index validation: a
	// self-edge is observable only with its nonzero recurrence Mu witness.
	if row.From == row.To && row.Mu == 0 {
		return false
	}
	if row.Decision == 0 {
		if row.Truth {
			return false
		}
	} else if !isDecision(row.Decision) || !r.validResultTerm(row.Decision) {
		return false
	}
	if row.Mu == 0 {
		return row.resetStart == 0 && row.resetPast == 0 && row.resetCount == 0 && !row.resetDigest.Available()
	}
	if row.Mu != 0 {
		family := keyspace.TermFamily(row.Mu)
		if (family != keyspace.FamilyLabel && family != keyspace.FamilyLoop) || !r.validResultTerm(row.Mu) || row.resetPast < row.resetStart {
			return false
		}
		// Before route sealing the witness fields must still be empty; they
		// are populated atomically from the canonical Mu stream during the
		// route-index cut. A partially copied witness is malformed rather
		// than an alternate reset authority.
		if !r.routesReady && (row.resetCount != 0 || row.resetDigest.Available()) {
			return false
		}
		start, end, ok := r.muRange(row.Mu)
		if !ok || uint64(row.resetPast) > uint64(end-start) || uint64(start)+uint64(row.resetPast) > uint64(len(r.reset.streams)) {
			return false
		}
		if r.routesReady {
			if _, _, ok := edgeResetWitness(row); !ok {
				return false
			}
		}
	}
	return true
}

func (r *Result) validBoundaryRow(row boundaryRow) bool {
	b := row.CallBoundary
	if !r.validResultTerm(b.Call) || keyspace.TermFamily(b.Call) != keyspace.FamilyCall ||
		keyspace.TermFamily(b.Throw) != keyspace.FamilyOutcome || !r.validResultTerm(b.Throw) ||
		keyspace.TermFamily(b.Yield) != keyspace.FamilyOutcome || !r.validResultTerm(b.Yield) ||
		keyspace.TermFamily(b.Cancel) != keyspace.FamilyOutcome || !r.validResultTerm(b.Cancel) {
		return false
	}
	switch b.mode {
	case boundaryDirect:
		return r.validResultTerm(b.Normal) && b.Normal != 0 && b.Other == 0 && b.TailReturn == 0
	case boundarySelectAnd, boundarySelectOr:
		return keyspace.TermFamily(b.Normal) == keyspace.FamilySelect && r.validResultTerm(b.Normal) &&
			r.validResultTerm(b.Other) && b.Other != 0 && b.TailReturn == 0
	case boundaryTail:
		return b.Normal == 0 && b.Other == 0 && r.validResultTerm(b.TailReturn) && keyspace.TermFamily(b.TailReturn) == keyspace.FamilyOutcome
	default:
		return false
	}
}

func (r *Result) successorRange(from keyspace.Term) (uint32, uint32, bool) {
	if !r.available() {
		return 0, 0, false
	}
	family, ordinal := keyspace.TermFamily(from), keyspace.TermOrdinal(from)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || ordinal > r.index.familyCounts[family] {
		return 0, 0, false
	}
	plane := r.index.planes[family]
	if plane.denominator == 0 || ordinal >= uint32(len(plane.ranges)) {
		return 0, 0, false
	}
	rangeValue := plane.ranges[ordinal]
	if rangeValue.end < rangeValue.start || uint64(rangeValue.end) > uint64(len(r.index.refs)) {
		return 0, 0, false
	}
	return rangeValue.start, rangeValue.end, true
}

func (r *Result) muRange(head keyspace.Term) (uint32, uint32, bool) {
	if !r.available() {
		return 0, 0, false
	}
	family, ordinal := keyspace.TermFamily(head), keyspace.TermOrdinal(head)
	if (family != keyspace.FamilyLabel && family != keyspace.FamilyLoop) || ordinal == 0 ||
		uint64(ordinal) >= uint64(len(r.reset.headRanges[family])) {
		return 0, 0, false
	}
	rangeValue := r.reset.headRanges[family][ordinal]
	if rangeValue.end < rangeValue.start || uint64(rangeValue.end) > uint64(len(r.reset.streams)) {
		return 0, 0, false
	}
	return rangeValue.start, rangeValue.end, true
}

func boundarySuccessor(b CallBoundary, arm BoundaryArmKind) (keyspace.Term, keyspace.Term, bool, bool) {
	if !isBoundaryArm(arm) {
		return 0, 0, false, false
	}
	switch arm {
	case BoundaryResume:
		if b.mode != boundaryDirect || b.Normal == 0 {
			return 0, 0, false, false
		}
		return b.Normal, 0, false, true
	case BoundarySelectTrue:
		if b.mode != boundarySelectAnd && b.mode != boundarySelectOr {
			return 0, 0, false, false
		}
		if b.mode == boundarySelectAnd {
			return b.Other, b.Normal, true, b.Other != 0
		}
		return b.Normal, b.Normal, true, b.Normal != 0
	case BoundarySelectFalse:
		if b.mode != boundarySelectAnd && b.mode != boundarySelectOr {
			return 0, 0, false, false
		}
		if b.mode == boundarySelectAnd {
			return b.Normal, b.Normal, false, b.Normal != 0
		}
		return b.Other, b.Normal, false, b.Other != 0
	case BoundaryTail:
		if b.mode != boundaryTail || b.TailReturn == 0 {
			return 0, 0, false, false
		}
		return b.TailReturn, 0, false, true
	case BoundaryThrow:
		return b.Throw, 0, false, b.Throw != 0
	case BoundaryYield:
		return b.Yield, 0, false, b.Yield != 0
	case BoundaryCancel:
		return b.Cancel, 0, false, b.Cancel != 0
	default:
		return 0, 0, false, false
	}
}
