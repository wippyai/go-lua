package causal

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Matches reports whether r belongs to the exact Source, authored Flow,
// Static, and Module identities supplied by the final assembly. Any
// unavailable identity fails closed.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
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
	row := &v.result.edges.rows[index]
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
	edge := &v.result.edges.rows[index]
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
	edge := &v.result.edges.rows[index]
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
	edge := &v.result.edges.rows[index]
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
		v.result.reset.decisionHead[family][ordinal] == 0 {
		return false
	}
	ranks := v.result.reset.decisionRank[family]
	if uint64(ordinal) >= uint64(len(ranks)) {
		return false
	}
	owner := v.result.reset.decisionHead[family][ordinal]
	ownerStart, ownerEnd, ownerOK := v.result.muRange(owner)
	if !ownerOK || ownerEnd < ownerStart || uint64(ranks[ordinal]) >= uint64(ownerEnd-ownerStart) {
		return false
	}
	position := uint64(ownerStart) + uint64(ranks[ordinal])
	resetStart := uint64(start) + uint64(edge.resetStart)
	resetPast := uint64(start) + uint64(edge.resetPast)
	return resetStart <= position && position < resetPast && resetPast <= uint64(end)
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
	row := &v.result.edges.rows[edgeIndex]
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
	row := &v.result.edges.rows[edgeIndex]
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
	row := &v.result.boundaries.rows[index]
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
	row := &v.result.boundaries.rows[slot-1]
	if !v.result.validBoundaryRow(row) {
		return CallBoundary{}, false
	}
	return row.CallBoundary, true
}

// Arm reissues one exact closed CallBoundary arm through the sealed boundary
// row and successor reference. It is O(1): callSlots selects the boundary
// row and the fixed arm slot selects its endpoint/proof; no successor range
// is scanned.
func (v Boundaries) Arm(call keyspace.Term, arm BoundaryArmKind) (Successor, bool) {
	if v.result == nil || !v.result.available() || !isBoundaryArm(arm) || keyspace.TermFamily(call) != keyspace.FamilyCall {
		return Successor{}, false
	}
	ordinal := keyspace.TermOrdinal(call)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.boundaries.callSlots)) {
		return Successor{}, false
	}
	slot := v.result.boundaries.callSlots[ordinal]
	if slot == 0 || uint64(slot-1) >= uint64(len(v.result.boundaries.rows)) {
		return Successor{}, false
	}
	row := &v.result.boundaries.rows[slot-1]
	if !v.result.validBoundaryRow(row) || row.Call != call {
		return Successor{}, false
	}
	to, decision, truth, ok := boundarySuccessor(row.CallBoundary, arm)
	if !ok {
		return Successor{}, false
	}
	ref := &row.refs[arm]
	if !ref.routeDigest.Available() || ref.index != slot-1 || ref.local || ref.arm != arm {
		return Successor{}, false
	}
	successor, ok := v.result.successorForRef(ref)
	if !ok || successor.To != to || successor.Decision != decision || successor.Truth != truth {
		return Successor{}, false
	}
	return successor, true
}

// Successors is the one allocation-free combined local-plus-boundary
// traversal view.
type Successors struct{ result *Result }

// Successors returns the exact union traversal view.
func (r *Result) Successors() Successors { return Successors{result: r} }

// TotalCount/TotalAt expose the already-sealed union index in its canonical
// publication order. They do not create a third route table: refs points to
// the existing Edge/CallBoundary rows and successorForRef reissues one exact
// route on demand.
func (v Successors) TotalCount() int {
	if v.result == nil || !v.result.available() {
		return 0
	}
	return len(v.result.index.refs)
}

func (v Successors) TotalAt(index int) (Successor, bool) {
	if v.result == nil || !v.result.available() || index < 0 || index >= len(v.result.index.refs) {
		return Successor{}, false
	}
	return v.result.successorForRef(&v.result.index.refs[index])
}

// SemanticIDAt projects the semantic identity already issued onto one sealed
// successor ref. Consumers that need only that identity do not materialize and
// revalidate the complete Edge or CallBoundary row merely to discard it.
func (v Successors) SemanticIDAt(index int) (identity.ContentID, bool) {
	if v.result == nil || !v.result.available() || index < 0 || index >= len(v.result.index.refs) {
		return identity.ContentID{}, false
	}
	ref := &v.result.index.refs[index]
	if !v.result.issuedRef(ref) {
		return identity.ContentID{}, false
	}
	semantic := ref.semanticPath
	return semantic, semantic.Available()
}

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
	return v.result.successorForRef(&v.result.index.refs[at])
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
	if ordinal == 0 || uint64(ordinal) >= uint64(len(v.result.index.writeCommitRouteIndexes)) {
		return Successor{}, false
	}
	routeIndex := v.result.index.writeCommitRouteIndexes[ordinal]
	if uint64(routeIndex) >= uint64(len(v.result.index.refs)) {
		return Successor{}, false
	}
	ref := &v.result.index.refs[routeIndex]
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
func (r *Result) successorForRef(ref *successorRef) (Successor, bool) {
	if r == nil || !r.available() {
		return Successor{}, false
	}
	if !isKnownArm(ref.arm) || ref.local != isLocalArm(ref.arm) {
		return Successor{}, false
	}
	if r.routesReady && !r.issuedRef(ref) {
		return Successor{}, false
	}
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) {
			return Successor{}, false
		}
		row := &r.edges.rows[ref.index]
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
			component: row.component, ref: *ref, refValid: true, result: r, edgeIndex: ref.index, edgeIndexValid: idOK}, true
	}
	if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
		return Successor{}, false
	}
	row := &r.boundaries.rows[ref.index]
	if !r.validBoundaryRow(row) {
		return Successor{}, false
	}
	boundary := row.CallBoundary
	to, decision, truth, armOK := boundarySuccessor(boundary, ref.arm)
	if !armOK {
		return Successor{}, false
	}
	proof := row.proofs[ref.arm]
	id, idOK := r.routeIdentityFastForRef(ref)
	if r.routesReady && !idOK {
		return Successor{}, false
	}
	return Successor{From: boundary.Call, To: to, Decision: decision, Truth: truth, Mu: proof.mu,
		Arm: ref.arm, routeDigest: ref.routeDigest, route: id, result: r,
		component: row.components[ref.arm], ref: *ref, refValid: true, edgeIndex: ref.index, edgeIndexValid: idOK}, true
}

// Component reports the recurrence-issued cyclic-component head carried by
// this exact final route. A false result is a genuine acyclic/cross-component
// route, not a missing certificate.
func (s Successor) Component() (keyspace.Term, bool) {
	if !isKnownArm(s.Arm) || s.result == nil || s.component == 0 || !s.result.componentIssued(s.component) {
		return 0, false
	}
	return s.component, true
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
	if s.result == nil || !s.refValid || !s.result.routesReady || !s.route.Issued() || s.route.Digest != s.routeDigest {
		return RouteIdentity{}, false
	}
	return s.route, true
}

// IssuedBy is the sealed-owner fence for a successor projection.  The route
// directory has already validated the complete route preimage; this hot
// check verifies only that the projection still points at that issued row and
// owner, without rebuilding the digest.
func (s Successor) IssuedBy(owner *Result) bool {
	return owner != nil && s.result == owner && s.refValid && s.result.routesReady &&
		s.result.issuedRef(&s.ref) && s.route.Issued() && s.route.Digest == s.routeDigest
}

func (s Successor) SemanticID() (identity.ContentID, bool) {
	if s.result == nil || !s.refValid {
		return identity.ContentID{}, false
	}
	return s.result.semanticRouteID(&s.ref)
}

// FromPoint/ToPoint resolve the exact parent-issued VertexCatalog points
// copied into this final route during the causal transaction. They never
// derive a point from an endpoint Term or Site.
func (s Successor) FromPoint() (WTOPoint, bool) {
	if s.result == nil || !s.refValid || !s.ref.fromPoint.Available() {
		return WTOPoint{}, false
	}
	index, ok := s.result.wto.pointByPath[s.ref.fromPoint]
	if !ok {
		return WTOPoint{}, false
	}
	return s.result.wtoPointAt(int(index))
}
func (s Successor) ToPoint() (WTOPoint, bool) {
	if s.result == nil || !s.refValid || !s.ref.toPoint.Available() {
		return WTOPoint{}, false
	}
	index, ok := s.result.wto.pointByPath[s.ref.toPoint]
	if !ok {
		return WTOPoint{}, false
	}
	return s.result.wtoPointAt(int(index))
}

// ResetCount, ResetAt, and ResetContains are route-local reset capabilities.
// They intentionally do not accept or expose an Edge ordinal.
func (s Successor) ResetCount() (int, bool) {
	if s.result == nil || !s.edgeIndexValid {
		return 0, false
	}
	if s.IsLocal() {
		return s.result.Edges().ResetCount(int(s.edgeIndex))
	}
	return s.result.boundaryResetCount(s.edgeIndex, s.Arm)
}

// HasResetWitness distinguishes a cyclic Mu edge with an empty reset interval
// from an acyclic/non-Mu route. Callers must never infer this from Count==0.
func (s Successor) HasResetWitness() bool {
	if s.result == nil || !s.refValid || s.Mu == 0 {
		return false
	}
	if s.IsLocal() {
		_, ok := s.result.Edges().ResetCount(int(s.edgeIndex))
		return ok
	}
	_, ok := s.result.boundaryResetCount(s.edgeIndex, s.Arm)
	return ok
}

func (s Successor) ResetAt(offset int) (keyspace.Term, bool) {
	if s.result == nil || !s.edgeIndexValid {
		return 0, false
	}
	if s.IsLocal() {
		return s.result.Edges().ResetAt(int(s.edgeIndex), offset)
	}
	return s.result.boundaryResetAt(s.edgeIndex, s.Arm, offset)
}

// ResetPathAt returns the parent-issued semantic reset-member path. It is
// O(1), immutable, and remains valid for an empty Mu witness (count zero).
func (s Successor) ResetPathAt(offset int) (identity.ContentID, bool) {
	if s.result == nil || !s.refValid || s.ref.resetMembers == nil || offset < 0 || uint64(offset) >= uint64(len(*s.ref.resetMembers)) {
		return identity.ContentID{}, false
	}
	path := (*s.ref.resetMembers)[offset]
	return path, path.Available()
}

func (s Successor) ResetContains(decision keyspace.Term) bool {
	if s.result == nil || !s.edgeIndexValid {
		return false
	}
	if s.IsLocal() {
		return s.result.Edges().ResetContains(int(s.edgeIndex), decision)
	}
	return s.result.boundaryResetContains(s.edgeIndex, s.Arm, decision)
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
	ref, refOK := v.result.routeLookupRef(uint32(found))
	if !refOK {
		return Successor{}, false
	}
	candidate, ok := v.result.routeIdentityFastForRef(ref)
	if !ok || !compareRouteID(candidate, identity) {
		return Successor{}, false
	}
	return v.result.successorForRef(ref)
}

func (r *Result) validResultTerm(term keyspace.Term) bool {
	if !r.available() {
		return false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 && ordinal <= r.index.familyCounts[family]
}

// validEdgeRow derives one local Edge row's structural well-formedness. It is
// a construction predicate: sealRows applies it to every retained row in its
// final form and fails the seal closed, after which the row plane is immutable
// and a reader projects the proven row instead of re-deriving it.
func (r *Result) validEdgeRow(row *edgeRow) bool {
	if r.rowsSealed {
		return true
	}
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
	if row.component != 0 && !r.componentIssued(row.component) {
		return false
	}
	if row.Mu == 0 {
		return row.resetStart == 0 && row.resetPast == 0 && row.resetCount == 0 && !row.resetDigest.Available()
	}
	if row.Mu != 0 {
		family := keyspace.TermFamily(row.Mu)
		if row.component == 0 || (family != keyspace.FamilyLabel && family != keyspace.FamilyLoop) || !r.validResultTerm(row.Mu) || row.resetPast < row.resetStart {
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

// validBoundaryRow derives one CallBoundary row's shape and the proof of every
// present arm. Like validEdgeRow it is a construction predicate proven once by
// sealRows; the sealed plane needs no per-read rederivation.
func (r *Result) validBoundaryRow(row *boundaryRow) bool {
	if r.rowsSealed {
		return true
	}
	b := row.CallBoundary
	if !r.validResultTerm(b.Call) || keyspace.TermFamily(b.Call) != keyspace.FamilyCall ||
		keyspace.TermFamily(b.Throw) != keyspace.FamilyOutcome || !r.validResultTerm(b.Throw) ||
		keyspace.TermFamily(b.Yield) != keyspace.FamilyOutcome || !r.validResultTerm(b.Yield) ||
		keyspace.TermFamily(b.Cancel) != keyspace.FamilyOutcome || !r.validResultTerm(b.Cancel) {
		return false
	}
	var shapeOK bool
	switch b.mode {
	case boundaryDirect:
		shapeOK = r.validResultTerm(b.Normal) && b.Normal != 0 && b.Other == 0 && b.TailReturn == 0
	case boundarySelectAnd, boundarySelectOr:
		shapeOK = keyspace.TermFamily(b.Normal) == keyspace.FamilySelect && r.validResultTerm(b.Normal) &&
			r.validResultTerm(b.Other) && b.Other != 0 && b.TailReturn == 0
	case boundaryTail:
		shapeOK = b.Normal == 0 && b.Other == 0 && r.validResultTerm(b.TailReturn) && keyspace.TermFamily(b.TailReturn) == keyspace.FamilyOutcome
	default:
		return false
	}
	if !shapeOK {
		return false
	}
	for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
		if boundaryArmPresent(row, arm) {
			if !r.validBoundaryProof(row, arm) {
				return false
			}
			continue
		}
		if row.components[arm] != 0 || row.proofs[arm] != (boundaryRecurrenceProof{}) {
			return false
		}
	}
	return true
}

// validBoundaryProof derives one arm's recurrence witness. Arm presence stays
// the residue after sealing: the caller supplies the arm, so which arms this
// proven row actually carries is the only thing a sealed read still decides.
func (r *Result) validBoundaryProof(row *boundaryRow, arm BoundaryArmKind) bool {
	if r.rowsSealed {
		return isBoundaryArm(arm) && boundaryArmPresent(row, arm)
	}
	if !isBoundaryArm(arm) || !boundaryArmPresent(row, arm) || (row.components[arm] != 0 && !r.componentIssued(row.components[arm])) {
		return false
	}
	proof := &row.proofs[arm]
	if proof.mu == 0 {
		return proof.resetStart == 0 && proof.resetPast == 0 && proof.resetCount == 0 && !proof.resetDigest.Available()
	}
	if row.components[arm] == 0 || proof.resetPast < proof.resetStart || !r.validResultTerm(proof.mu) {
		return false
	}
	family := keyspace.TermFamily(proof.mu)
	if family != keyspace.FamilyLabel && family != keyspace.FamilyLoop {
		return false
	}
	start, end, ok := r.muRange(proof.mu)
	if !ok || uint64(proof.resetPast) > uint64(end-start) || uint64(start)+uint64(proof.resetPast) > uint64(len(r.reset.streams)) {
		return false
	}
	if r.routesReady {
		_, _, ok := boundaryResetWitness(proof)
		return ok
	}
	// Route-index construction visits several arms of the same boundary.
	// Earlier visits may already have installed a digest, so accept either the
	// still-empty preimage witness or a fully self-consistent installed one.
	if proof.resetCount == 0 && !proof.resetDigest.Available() {
		return true
	}
	_, _, ok = boundaryResetWitness(proof)
	return ok
}

func (r *Result) componentIssued(component keyspace.Term) bool {
	if r == nil || !canonicalComponent(component, true) {
		return false
	}
	index, ok := r.componentIndex[component]
	if !ok || uint64(index) >= uint64(len(r.components)) {
		return false
	}
	return r.components[index] == component
}

func (r *Result) boundaryResetCount(index uint32, arm BoundaryArmKind) (int, bool) {
	if r == nil || uint64(index) >= uint64(len(r.boundaries.rows)) {
		return 0, false
	}
	row := &r.boundaries.rows[index]
	if !r.validBoundaryProof(row, arm) || row.proofs[arm].mu == 0 {
		return 0, false
	}
	proof := row.proofs[arm]
	return int(proof.resetPast - proof.resetStart), true
}

func (r *Result) boundaryResetAt(index uint32, arm BoundaryArmKind, offset int) (keyspace.Term, bool) {
	if offset < 0 || r == nil || uint64(index) >= uint64(len(r.boundaries.rows)) {
		return 0, false
	}
	row := &r.boundaries.rows[index]
	if !r.validBoundaryProof(row, arm) || row.proofs[arm].mu == 0 {
		return 0, false
	}
	proof := row.proofs[arm]
	if uint64(offset) >= uint64(proof.resetPast-proof.resetStart) {
		return 0, false
	}
	start, _, ok := r.muRange(proof.mu)
	if !ok {
		return 0, false
	}
	term := r.reset.streams[uint64(start)+uint64(proof.resetStart)+uint64(offset)]
	return term, isDecision(term)
}

func (r *Result) boundaryResetContains(index uint32, arm BoundaryArmKind, decision keyspace.Term) bool {
	if r == nil || uint64(index) >= uint64(len(r.boundaries.rows)) || !isDecision(decision) {
		return false
	}
	row := &r.boundaries.rows[index]
	if !r.validBoundaryProof(row, arm) || row.proofs[arm].mu == 0 {
		return false
	}
	proof := row.proofs[arm]
	family, ordinal := keyspace.TermFamily(decision), keyspace.TermOrdinal(decision)
	if uint64(ordinal) >= uint64(len(r.reset.decisionHead[family])) || r.reset.decisionHead[family][ordinal] == 0 {
		return false
	}
	ranks := r.reset.decisionRank[family]
	if uint64(ordinal) >= uint64(len(ranks)) {
		return false
	}
	start, end, ok := r.muRange(proof.mu)
	if !ok || proof.resetPast > end-start {
		return false
	}
	owner := r.reset.decisionHead[family][ordinal]
	ownerStart, ownerEnd, ownerOK := r.muRange(owner)
	if !ownerOK || ownerEnd < ownerStart || uint64(ranks[ordinal]) >= uint64(ownerEnd-ownerStart) {
		return false
	}
	position := uint64(ownerStart) + uint64(ranks[ordinal])
	resetStart := uint64(start) + uint64(proof.resetStart)
	resetPast := uint64(start) + uint64(proof.resetPast)
	return resetStart <= position && position < resetPast && resetPast <= uint64(end)
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
