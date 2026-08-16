package causal

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/keyspace"
)

func (r *Result) semanticTermPath(term keyspace.Term) (keyspace.ContentID, bool) {
	if r == nil || !validIdentityTerm(term) {
		return keyspace.ContentID{}, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if r.structuralPaths == nil {
		return keyspace.ContentID{}, false
	}
	return r.structuralPaths.At(family, ordinal)
}

func (r *Result) semanticResetRange(mu keyspace.Term, start, past uint32) (uint64, bool) {
	if mu == 0 {
		return 0, start == 0 && past == 0
	}
	if past < start {
		return 0, false
	}
	base, end, ok := r.muRange(mu)
	if !ok || uint64(past) > uint64(end-base) || uint64(base)+uint64(past) > uint64(len(r.reset.streams)) {
		return 0, false
	}
	return uint64(base), true
}

func (r *Result) semanticResetMembers(ref successorRef) ([]keyspace.ContentID, bool) {
	if r == nil || !isKnownArm(ref.arm) {
		return nil, false
	}
	var mu keyspace.Term
	var start, past uint32
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) {
			return nil, false
		}
		row := r.edges.rows[ref.index]
		mu, start, past = row.Mu, row.resetStart, row.resetPast
	} else {
		if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
			return nil, false
		}
		row := r.boundaries.rows[ref.index]
		proof := row.proofs[ref.arm]
		mu, start, past = proof.mu, proof.resetStart, proof.resetPast
	}
	base, ok := r.semanticResetRange(mu, start, past)
	if !ok {
		return nil, false
	}
	members := make([]keyspace.ContentID, 0, int(past-start))
	for index := base + uint64(start); index < base+uint64(past); index++ {
		path, pathOK := r.semanticTermPath(r.reset.streams[index])
		if !pathOK {
			return nil, false
		}
		members = append(members, path)
	}
	radixContentIDs(members)
	for index := 1; index < len(members); index++ {
		if members[index-1] == members[index] {
			return nil, false
		}
	}
	return members, true
}

func radixContentIDs(ids []keyspace.ContentID) {
	if len(ids) < 2 {
		return
	}
	work := make([]keyspace.ContentID, len(ids))
	for byteIndex := len(keyspace.ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var counts [256]int
		for _, id := range ids {
			counts[id[byteIndex]]++
		}
		at := 0
		for index := range counts {
			at, counts[index] = at+counts[index], at
		}
		for _, id := range ids {
			bucket := id[byteIndex]
			work[counts[bucket]] = id
			counts[bucket]++
		}
		copy(ids, work)
	}
}

func (r *Result) semanticRoutePath(ref successorRef) (keyspace.ContentID, bool) {
	if r == nil || !isKnownArm(ref.arm) {
		return keyspace.ContentID{}, false
	}
	var from, to, decision, mu keyspace.Term
	var truth bool
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) {
			return keyspace.ContentID{}, false
		}
		row := r.edges.rows[ref.index]
		from, to, decision, truth, mu = row.From, row.To, row.Decision, row.Truth, row.Mu
	} else {
		if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
			return keyspace.ContentID{}, false
		}
		row := r.boundaries.rows[ref.index]
		var ok bool
		from = row.CallBoundary.Call
		to, decision, truth, ok = boundarySuccessor(row.CallBoundary, ref.arm)
		if !ok {
			return keyspace.ContentID{}, false
		}
		proof := row.proofs[ref.arm]
		mu = proof.mu
	}
	// Endpoints are SourceControl VertexCatalog phases, not term/Site paths.
	// They were copied into the exact successor ref while the catalog lease
	// was live, so a later semantic receipt cannot silently collapse two
	// phases which share an authored term.
	fromPath, toPath := ref.fromPoint, ref.toPoint
	if !fromPath.Available() || !toPath.Available() {
		return keyspace.ContentID{}, false
	}
	var encoded bytes.Buffer
	encoded.WriteString("wippy/program/flow/causal-route-path-v1")
	encoded.WriteByte(0)
	encoded.Write(fromPath[:])
	encoded.Write(toPath[:])
	encoded.WriteByte(byte(ref.arm))
	// Distinct evaluation edges can share the same phase endpoints. Include
	// owner-neutral semantic paths for their final successor terms; raw terms,
	// route digests, and provenance IDs must never enter this reusable path.
	for _, term := range []keyspace.Term{from, to} {
		path, ok := r.semanticTermPath(term)
		if !ok {
			return keyspace.ContentID{}, false
		}
		encoded.Write(path[:])
	}
	if truth {
		encoded.WriteByte(1)
	} else {
		encoded.WriteByte(0)
	}
	for _, term := range []keyspace.Term{decision, mu} {
		if term == 0 {
			encoded.Write(make([]byte, 32))
			continue
		}
		path, ok := r.semanticTermPath(term)
		if !ok {
			return keyspace.ContentID{}, false
		}
		encoded.Write(path[:])
	}
	resetMembers, resetOK := r.semanticResetMembers(ref)
	if !resetOK {
		return keyspace.ContentID{}, false
	}
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(resetMembers)))
	encoded.Write(count[:])
	for _, path := range resetMembers {
		encoded.Write(path[:])
	}
	return keyspace.ContentID(sha256.Sum256(encoded.Bytes())), true
}

func (r *Result) routeMu(ref successorRef) keyspace.Term {
	if r == nil || !isKnownArm(ref.arm) {
		return 0
	}
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) {
			return 0
		}
		return r.edges.rows[ref.index].Mu
	}
	if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
		return 0
	}
	return r.boundaries.rows[ref.index].proofs[ref.arm].mu
}

func (r *Result) semanticResetPath(ref successorRef) (keyspace.ContentID, bool) {
	if r == nil || !isKnownArm(ref.arm) {
		return keyspace.ContentID{}, false
	}
	var mu keyspace.Term
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) {
			return keyspace.ContentID{}, false
		}
		row := r.edges.rows[ref.index]
		mu = row.Mu
	} else {
		if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
			return keyspace.ContentID{}, false
		}
		row := r.boundaries.rows[ref.index]
		proof := row.proofs[ref.arm]
		mu = proof.mu
	}
	if mu == 0 {
		return keyspace.ContentID{}, true
	}
	resetMembers, resetOK := r.semanticResetMembers(ref)
	if !resetOK {
		return keyspace.ContentID{}, false
	}
	return r.semanticResetPathMembers(ref, resetMembers)
}

func (r *Result) semanticResetPathMembers(ref successorRef, resetMembers []keyspace.ContentID) (keyspace.ContentID, bool) {
	routeID, routeOK := r.semanticRoutePath(ref)
	if !routeOK {
		return keyspace.ContentID{}, false
	}
	var encoded bytes.Buffer
	encoded.WriteString("wippy/program/flow/causal-reset-path-v1")
	encoded.WriteByte(0)
	encoded.Write(routeID[:])
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(resetMembers)))
	encoded.Write(count[:])
	for _, path := range resetMembers {
		encoded.Write(path[:])
	}
	return keyspace.ContentID(sha256.Sum256(encoded.Bytes())), true
}

// installRouteSemanticPaths publishes semantic route receipts into the sole
// canonical ref directory and its already-sealed sorted inverse. The route
// ordinal stamped by buildRouteIndex is the exact join key; this method is
// intentionally separate from the final WTO publication cut so its linear
// installation law can be checked without constructing a hierarchy.
func (r *Result) installRouteSemanticPaths() bool {
	if r == nil || !r.available() || r.structuralPaths == nil {
		return false
	}
	for index, ref := range r.index.refs {
		path, ok := r.semanticRoutePath(ref)
		if !ok {
			return false
		}
		r.index.refs[index].semanticPath = path
		mu := r.routeMu(ref)
		if mu == 0 {
			// Ordinary intra-SCC routes retain no reset witness. In particular,
			// do not mint a digest for an empty non-Mu member list.
			r.index.refs[index].resetMembers = nil
			r.index.refs[index].semanticResetPath = keyspace.ContentID{}
		} else {
			members, membersOK := r.semanticResetMembers(ref)
			if !membersOK {
				return false
			}
			memberReceipt := append([]keyspace.ContentID(nil), members...)
			r.index.refs[index].resetMembers = &memberReceipt
			resetPath, resetOK := r.semanticResetPathMembers(ref, members)
			if !resetOK {
				return false
			}
			r.index.refs[index].semanticResetPath = resetPath
		}
		if mu != 0 {
			muPath, muOK := r.semanticTermPath(mu)
			if !muOK {
				return false
			}
			r.index.refs[index].semanticMuPath = muPath
		}
	}
	// Guard contexts are fully materialized while the private structural term
	// lease is available. Later proof validation reads this copied scalar; it
	// never reopens a term-to-path plane.
	for index := range r.index.refs {
		ref := &r.index.refs[index]
		var decision keyspace.Term
		var truth bool
		if ref.local {
			if uint64(ref.index) >= uint64(len(r.edges.rows)) {
				return false
			}
			decision, truth = r.edges.rows[ref.index].Decision, r.edges.rows[ref.index].Truth
		} else {
			if uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
				return false
			}
			_, decision, truth, _ = boundarySuccessor(r.boundaries.rows[ref.index].CallBoundary, ref.arm)
		}
		if decision == 0 {
			continue
		}
		decisionPath, ok := r.semanticTermPath(decision)
		if !ok || !ref.semanticPath.Available() {
			return false
		}
		ref.guardDecisionPath = decisionPath
		var encoded [len("wippy/program/flow/causal-route-guard") + 1 + 32 + 32 + 1]byte
		offset := copy(encoded[:], "wippy/program/flow/causal-route-guard")
		encoded[offset] = 0
		offset++
		offset += copy(encoded[offset:], ref.semanticPath[:])
		offset += copy(encoded[offset:], decisionPath[:])
		if truth {
			encoded[offset] = 1
		}
		offset++
		ref.guardContext = keyspace.ContentID(sha256.Sum256(encoded[:offset]))
	}
	// buildRouteIndex stamped each canonical ref with its exact slot in the
	// sorted route directory. Publish semantic receipts directly through that
	// directory: this is linear in the route count and does not scan/rebuild
	// the existing ref authority.
	for _, ref := range r.index.refs {
		index := ref.routeIndexOrdinal
		if uint64(index) >= uint64(len(r.routeIndex)) {
			return false
		}
		lookup := &r.routeIndex[index].ref
		if !routeRefsEqual(ref, *lookup) || lookup.routeIndexOrdinal != index {
			return false
		}
		lookup.semanticPath = ref.semanticPath
		lookup.semanticResetPath = ref.semanticResetPath
		lookup.resetMembers = ref.resetMembers
		lookup.semanticMuPath = ref.semanticMuPath
		lookup.guardContext = ref.guardContext
		lookup.guardDecisionPath = ref.guardDecisionPath
		if !ref.local {
			if uint64(ref.index) >= uint64(len(r.boundaries.rows)) || !isBoundaryArm(ref.arm) {
				return false
			}
			rowRef := &r.boundaries.rows[ref.index].refs[ref.arm]
			if !routeRefsEqual(ref, *rowRef) {
				return false
			}
			rowRef.routeIndexOrdinal = index
			rowRef.semanticPath = ref.semanticPath
			rowRef.semanticResetPath = ref.semanticResetPath
			rowRef.resetMembers = ref.resetMembers
			rowRef.semanticMuPath = ref.semanticMuPath
			rowRef.guardContext = ref.guardContext
			rowRef.guardDecisionPath = ref.guardDecisionPath
		}
	}
	for index := range r.index.writeCommitRefs {
		ref := &r.index.writeCommitRefs[index]
		if !ref.routeDigest.Available() {
			continue
		}
		if uint64(ref.routeIndexOrdinal) >= uint64(len(r.routeIndex)) {
			return false
		}
		canonical := r.routeIndex[ref.routeIndexOrdinal].ref
		ref.semanticPath = canonical.semanticPath
		ref.semanticResetPath = canonical.semanticResetPath
		ref.resetMembers = canonical.resetMembers
		ref.semanticMuPath = canonical.semanticMuPath
		ref.guardContext = canonical.guardContext
		ref.guardDecisionPath = canonical.guardDecisionPath
	}
	for index := range r.index.refs {
		r.index.refs[index].routeIndexOrdinal = 0
	}
	for index := range r.routeIndex {
		r.routeIndex[index].ref.routeIndexOrdinal = 0
	}
	for row := range r.boundaries.rows {
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			r.boundaries.rows[row].refs[arm].routeIndexOrdinal = 0
		}
	}
	for index := range r.index.writeCommitRefs {
		r.index.writeCommitRefs[index].routeIndexOrdinal = 0
	}
	// All semantic derivations have been copied into exact refs/Sites. The
	// Source term path lease cannot remain as a second published route plane.
	r.structuralPaths = nil
	return true
}

func (r *Result) semanticRouteID(ref successorRef) (keyspace.ContentID, bool) {
	if r == nil || !ref.semanticPath.Available() {
		return keyspace.ContentID{}, false
	}
	return ref.semanticPath, true
}

func routeRefsEqual(left, right successorRef) bool {
	return left.index == right.index && left.local == right.local && left.arm == right.arm && left.routeDigest == right.routeDigest
}

// RouteIdentity is the immutable semantic identity of one final causal
// route. It is intentionally a value, not a row/index capability: every
// field is canonical program data or an owner identity, and Digest commits to
// the complete reset relation. The public flow package projects this type
// without exposing this internal package.
//
// ResetDigest is a commitment to the sorted set of reset decisions. It is
// kept here so a sealed query can authenticate the relation in O(1); the
// relation itself remains in Result.reset's one existing stream store.
type RouteIdentity struct {
	SourceID    keyspace.ContentID
	FlowID      keyspace.ContentID
	StaticID    keyspace.ContentID
	ModuleID    keyspace.ContentID
	From        keyspace.Term
	To          keyspace.Term
	Decision    keyspace.Term
	Truth       bool
	Mu          keyspace.Term
	Arm         BoundaryArmKind
	ResetDigest keyspace.ContentID
	ResetCount  uint32
	Digest      keyspace.ContentID
}

func (id RouteIdentity) available() bool {
	if !id.SourceID.Available() || !id.FlowID.Available() || !id.StaticID.Available() || !id.ModuleID.Available() ||
		!validIdentityTerm(id.From) || !validIdentityTerm(id.To) || !id.Digest.Available() || !isKnownArm(id.Arm) {
		return false
	}
	if id.Decision == 0 {
		if id.Truth {
			return false
		}
	} else if !isDecision(id.Decision) {
		return false
	}
	switch id.Arm {
	case BoundaryLocal:
		if keyspace.TermFamily(id.From) == keyspace.FamilyCall {
			return false
		}
		if id.Mu == 0 {
			return id.ResetCount == 0 && !id.ResetDigest.Available() && id.Digest == hashRoute(id)
		}
		muFamily := keyspace.TermFamily(id.Mu)
		return keyspace.TermOrdinal(id.Mu) != 0 &&
			(muFamily == keyspace.FamilyLabel || muFamily == keyspace.FamilyLoop) && id.ResetDigest.Available() && id.Digest == hashRoute(id)
	case BoundarySelectTrue:
		return keyspace.TermFamily(id.From) == keyspace.FamilyCall && id.Decision != 0 && keyspace.TermFamily(id.Decision) == keyspace.FamilySelect && id.Truth && validIdentityMu(id) && id.Digest == hashRoute(id)
	case BoundarySelectFalse:
		return keyspace.TermFamily(id.From) == keyspace.FamilyCall && id.Decision != 0 && keyspace.TermFamily(id.Decision) == keyspace.FamilySelect && !id.Truth && validIdentityMu(id) && id.Digest == hashRoute(id)
	default:
		return keyspace.TermFamily(id.From) == keyspace.FamilyCall && id.Decision == 0 && !id.Truth && validIdentityMu(id) && id.Digest == hashRoute(id)
	}
}

func validIdentityMu(id RouteIdentity) bool {
	if id.Mu == 0 {
		return id.ResetCount == 0 && !id.ResetDigest.Available()
	}
	family := keyspace.TermFamily(id.Mu)
	return keyspace.TermOrdinal(id.Mu) != 0 && (family == keyspace.FamilyLabel || family == keyspace.FamilyLoop) && id.ResetDigest.Available()
}

func validIdentityTerm(term keyspace.Term) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0
}

// Available reports whether the identity is a complete, well-formed sealed
// route value. It is exported only so the parent flow package can preserve
// the same fail-closed predicate without exposing the identity fields.
func (id RouteIdentity) Available() bool { return id.available() }

const (
	resetDigestDomain  = "wippy/program/flow/causal-reset"
	routeDigestDomain  = "wippy/program/flow/causal-route"
	routeDigestVersion = uint64(1)
)

type routeDescriptor struct {
	id       RouteIdentity
	ref      successorRef
	resetSet []keyspace.Term
	// sourceIndex points back to the sole canonical successor-ref directory.
	// It is seal-local provenance used to stamp routeIndexOrdinal after the
	// digest sort; no parallel lookup table is retained.
	sourceIndex int
}

// RouteRecurrence is the sealed recurrence witness for one final route. It
// borrows the existing Successor capability and carries no second route or
// component table. The component identity is issued here, where the
// recurrence directory is authoritative; consumers must not derive it from
// endpoints or the route digest.
type RouteRecurrence struct {
	successor   Successor
	componentID keyspace.ContentID
	resetID     keyspace.ContentID
	muID        keyspace.ContentID
}

// RouteGuardProof is the sealed guarded-disposition witness for one final
// route. The decision coordinate is retained only inside the parent proof;
// endpoint Site projection is optional because a decision need not itself be
// a causal endpoint.
type RouteGuardProof struct {
	successor Successor
	context   keyspace.ContentID
}

func guardProofContext(result *Result, successor Successor) keyspace.ContentID {
	if result == nil || !successor.route.Available() || successor.Decision == 0 || !successor.ref.guardContext.Available() || !successor.ref.guardDecisionPath.Available() {
		return keyspace.ContentID{}
	}
	return successor.ref.guardContext
}

func (s Successor) GuardProof() (RouteGuardProof, bool) {
	if s.result == nil || !s.refValid || !s.route.Available() || !isDecision(s.Decision) {
		return RouteGuardProof{}, false
	}
	proof := RouteGuardProof{successor: s, context: guardProofContext(s.result, s)}
	return proof, proof.Available()
}

func (proof RouteGuardProof) Available() bool {
	if proof.successor.result == nil || !proof.successor.refValid || !proof.successor.route.Available() ||
		!isDecision(proof.successor.Decision) || proof.context != guardProofContext(proof.successor.result, proof.successor) {
		return false
	}
	issued, ok := proof.successor.result.successorForRef(proof.successor.ref)
	return ok && issued.route == proof.successor.route && issued.routeDigest == proof.successor.routeDigest && issued.Decision == proof.successor.Decision &&
		issued.Truth == proof.successor.Truth
}

func (r *Result) OwnsRouteGuardProof(proof RouteGuardProof) bool {
	return r != nil && proof.successor.result == r && proof.Available()
}

func (proof RouteGuardProof) ContextID() keyspace.ContentID {
	if !proof.Available() {
		return keyspace.ContentID{}
	}
	return proof.context
}

func (proof RouteGuardProof) RouteID() (keyspace.ContentID, bool) {
	if !proof.Available() {
		return keyspace.ContentID{}, false
	}
	return proof.successor.SemanticID()
}

func (proof RouteGuardProof) RouteIdentity() (RouteIdentity, bool) {
	if !proof.Available() {
		return RouteIdentity{}, false
	}
	return proof.successor.route, true
}

func (proof RouteGuardProof) Decision() (keyspace.Term, bool) {
	if !proof.Available() {
		return 0, false
	}
	return proof.successor.Decision, true
}

// DecisionPathID returns the portable structural semantic identity of the
// guarded decision. Unlike the optional decision Site, this is available even
// when the control term is not itself a final route endpoint.
func (proof RouteGuardProof) DecisionPathID() (keyspace.ContentID, bool) {
	if !proof.Available() || !proof.successor.ref.guardDecisionPath.Available() {
		return keyspace.ContentID{}, false
	}
	return proof.successor.ref.guardDecisionPath, true
}

func (proof RouteGuardProof) Truth() (bool, bool) {
	if !proof.Available() {
		return false, false
	}
	return proof.successor.Truth, true
}

const recurrenceComponentDomain = "wippy/program/flow/causal-recurrence-component"

func recurrenceComponentID(result *Result, component keyspace.Term) keyspace.ContentID {
	if result == nil || !canonicalComponent(component, true) {
		return keyspace.ContentID{}
	}
	path, pathOK := result.componentPath(component)
	if !pathOK {
		return keyspace.ContentID{}
	}
	var encoded [len(recurrenceComponentDomain) + 1 + 32]byte
	offset := copy(encoded[:], recurrenceComponentDomain)
	encoded[offset] = 0
	offset++
	offset += copy(encoded[offset:], path[:])
	return keyspace.ContentID(sha256.Sum256(encoded[:offset]))
}

func (s Successor) Recurrence() (RouteRecurrence, bool) {
	if s.result == nil || !s.refValid || !s.route.Available() || !canonicalComponent(s.component, true) {
		return RouteRecurrence{}, false
	}
	componentID := recurrenceComponentID(s.result, s.component)
	if !componentID.Available() {
		return RouteRecurrence{}, false
	}
	resetID := s.ref.semanticResetPath
	if s.Mu != 0 && !resetID.Available() {
		return RouteRecurrence{}, false
	}
	muID := s.ref.semanticMuPath
	if s.Mu != 0 && !muID.Available() {
		return RouteRecurrence{}, false
	}
	proof := RouteRecurrence{successor: s, componentID: componentID, resetID: resetID, muID: muID}
	return proof, proof.Available()
}

func (proof RouteRecurrence) Available() bool {
	if proof.successor.result == nil || !proof.successor.refValid ||
		!proof.successor.route.Available() || !canonicalComponent(proof.successor.component, true) ||
		proof.componentID != recurrenceComponentID(proof.successor.result, proof.successor.component) {
		return false
	}
	resetID := proof.successor.ref.semanticResetPath
	if proof.successor.Mu != 0 && (!resetID.Available() || resetID != proof.resetID) {
		return false
	}
	if proof.successor.Mu == 0 {
		if proof.muID.Available() || proof.resetID.Available() {
			return false
		}
	} else if !proof.successor.ref.semanticMuPath.Available() || proof.successor.ref.semanticMuPath != proof.muID {
		return false
	}
	issued, ok := proof.successor.result.successorForRef(proof.successor.ref)
	if !ok || issued.result != proof.successor.result || issued.route != proof.successor.route || issued.routeDigest != proof.successor.routeDigest ||
		issued.component != proof.successor.component {
		return false
	}
	return issued.route.Available()
}

func (proof RouteRecurrence) RouteIdentity() (RouteIdentity, bool) {
	if !proof.Available() {
		return RouteIdentity{}, false
	}
	return proof.successor.route, true
}

func (r *Result) OwnsRouteRecurrence(proof RouteRecurrence) bool {
	return r != nil && proof.successor.result == r && proof.Available()
}

func (proof RouteRecurrence) ComponentID() keyspace.ContentID {
	if !proof.Available() {
		return keyspace.ContentID{}
	}
	return proof.componentID
}

func (proof RouteRecurrence) RouteID() (keyspace.ContentID, bool) {
	if !proof.Available() {
		return keyspace.ContentID{}, false
	}
	return proof.successor.SemanticID()
}

func (proof RouteRecurrence) HasMu() bool {
	return proof.Available() && proof.successor.Mu != 0
}

func (proof RouteRecurrence) MuPathID() (keyspace.ContentID, bool) {
	if !proof.Available() || !proof.muID.Available() {
		return keyspace.ContentID{}, false
	}
	return proof.muID, true
}

func (proof RouteRecurrence) Component() (keyspace.Term, bool) {
	if !proof.Available() {
		return 0, false
	}
	return proof.successor.component, true
}

func (proof RouteRecurrence) Mu() (keyspace.Term, bool) {
	if !proof.Available() || proof.successor.Mu == 0 {
		return 0, false
	}
	return proof.successor.Mu, true
}

func (proof RouteRecurrence) ResetCount() (int, bool) {
	if !proof.Available() {
		return 0, false
	}
	return proof.successor.ResetCount()
}

func (proof RouteRecurrence) ResetDigest() (keyspace.ContentID, bool) {
	if !proof.Available() || !proof.resetID.Available() {
		return keyspace.ContentID{}, false
	}
	return proof.resetID, true
}

// buildRouteIndex derives semantic identities from the existing two-plane
// rows and combined successor refs. It retains only the sorted digest/ref
// inverse after validation; all descriptors and reset copies die at return.
func (r *Result) buildRouteIndex() error {
	if r == nil || !r.available() {
		return errors.New("program/flow/causal: route owner is unavailable")
	}
	descriptors := make([]routeDescriptor, 0, len(r.index.refs))
	for refIndex, ref := range r.index.refs {
		id, resetSet, ok := r.routeIdentityForRef(ref)
		if !ok {
			return errors.New("program/flow/causal: successor route preimage is malformed")
		}
		ref.routeDigest = id.Digest
		r.index.refs[refIndex].routeDigest = id.Digest
		if !ref.local && uint64(ref.index) < uint64(len(r.boundaries.rows)) && isBoundaryArm(ref.arm) {
			r.boundaries.rows[ref.index].refs[ref.arm] = r.index.refs[refIndex]
		}
		descriptors = append(descriptors, routeDescriptor{id: id, ref: ref, resetSet: resetSet, sourceIndex: refIndex})
	}
	radixRouteDescriptors(descriptors)
	for index := 1; index < len(descriptors); index++ {
		previous, current := descriptors[index-1], descriptors[index]
		if previous.id.Digest != current.id.Digest {
			continue
		}
		if routePreimageEqual(previous, current) {
			return fmt.Errorf("program/flow/causal: duplicate semantic route %v -> %v", current.id.From, current.id.To)
		}
		// A full-width digest collision is not a safe route lookup. Refuse to
		// publish rather than making Resolve choose by physical order.
		return errors.New("program/flow/causal: semantic route digest collision")
	}
	r.routeIndex = make([]routeLookup, len(descriptors))
	for index, descriptor := range descriptors {
		if descriptor.sourceIndex < 0 || descriptor.sourceIndex >= len(r.index.refs) {
			return errors.New("program/flow/causal: route directory source is unavailable")
		}
		// Stamp the exact sorted directory slot onto the existing canonical ref.
		// Both the canonical ref and the lookup copy carry the same receipt.
		r.index.refs[descriptor.sourceIndex].routeIndexOrdinal = uint32(index)
		descriptor.ref.routeIndexOrdinal = uint32(index)
		// The boundary row is an alias of the canonical combined-index ref.
		// Stamp its private directory slot at the same time; otherwise a
		// direct-call arm retains the pre-directory zero and cannot be proven
		// equal to its canonical route during endpoint receipt installation.
		if !descriptor.ref.local && uint64(descriptor.ref.index) < uint64(len(r.boundaries.rows)) && isBoundaryArm(descriptor.ref.arm) {
			r.boundaries.rows[descriptor.ref.index].refs[descriptor.ref.arm].routeIndexOrdinal = uint32(index)
		}
		r.routeIndex[index] = routeLookup{digest: descriptor.id.Digest, ref: descriptor.ref}
	}
	r.routesReady = true
	return nil
}

func radixRouteDescriptors(rows []routeDescriptor) {
	if len(rows) < 2 {
		return
	}
	work := make([]routeDescriptor, len(rows))
	for byteIndex := len(keyspace.ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var counts [256]int
		for _, row := range rows {
			counts[row.id.Digest[byteIndex]]++
		}
		at := 0
		for index := range counts {
			at, counts[index] = at+counts[index], at
		}
		for _, row := range rows {
			bucket := row.id.Digest[byteIndex]
			work[counts[bucket]] = row
			counts[bucket]++
		}
		copy(rows, work)
	}
}

func routePreimageEqual(left, right routeDescriptor) bool {
	if left.id.SourceID != right.id.SourceID || left.id.FlowID != right.id.FlowID ||
		left.id.StaticID != right.id.StaticID || left.id.ModuleID != right.id.ModuleID ||
		left.id.From != right.id.From || left.id.To != right.id.To || left.id.Decision != right.id.Decision ||
		left.id.Truth != right.id.Truth || left.id.Mu != right.id.Mu || left.id.Arm != right.id.Arm ||
		left.id.ResetCount != right.id.ResetCount || left.id.ResetDigest != right.id.ResetDigest {
		return false
	}
	if len(left.resetSet) != len(right.resetSet) {
		return false
	}
	for index := range left.resetSet {
		if left.resetSet[index] != right.resetSet[index] {
			return false
		}
	}
	return true
}

// routeIdentityForRef reconstructs a route from existing Edge/Boundary rows.
// It deliberately accepts only the private successor ref produced by the
// combined index; no public ordinal can reach this helper.
func (r *Result) routeIdentityForRef(ref successorRef) (RouteIdentity, []keyspace.Term, bool) {
	if r == nil || !isKnownArm(ref.arm) || ref.local && uint64(ref.index) >= uint64(len(r.edges.rows)) || !ref.local && uint64(ref.index) >= uint64(len(r.boundaries.rows)) {
		return RouteIdentity{}, nil, false
	}
	if ref.local {
		row := r.edges.rows[ref.index]
		if !r.validEdgeRow(row) || !isLocalArm(ref.arm) {
			return RouteIdentity{}, nil, false
		}
		var resetSet []keyspace.Term
		var resetDigest keyspace.ContentID
		var resetCount uint32
		var ok bool
		if !r.routesReady {
			resetSet, resetDigest, resetCount, ok = r.canonicalResetSet(row)
			if !ok {
				return RouteIdentity{}, nil, false
			}
			// These are route-key witnesses, not a second reset store. The
			// existing stream/range remains authoritative for ResetAt/Contains.
			r.edges.rows[ref.index].resetDigest = resetDigest
			r.edges.rows[ref.index].resetCount = resetCount
		} else {
			resetDigest, resetCount = row.resetDigest, row.resetCount
			if row.Mu != 0 && (!resetDigest.Available() || row.resetPast < row.resetStart) {
				return RouteIdentity{}, nil, false
			}
		}
		id := RouteIdentity{
			SourceID: r.sourceID, FlowID: r.flowID, StaticID: r.staticID, ModuleID: r.moduleID,
			From: row.From, To: row.To, Decision: row.Decision, Truth: row.Truth, Mu: row.Mu,
			Arm: BoundaryLocal, ResetDigest: resetDigest, ResetCount: resetCount,
		}
		id.Digest = hashRoute(id)
		return id, resetSet, true
	}
	row := r.boundaries.rows[ref.index]
	if !r.validBoundaryRow(row) || !isBoundaryArm(ref.arm) || !boundaryArmPresent(row, ref.arm) {
		return RouteIdentity{}, nil, false
	}
	to, decision, truth, ok := boundarySuccessor(row.CallBoundary, ref.arm)
	if !ok {
		return RouteIdentity{}, nil, false
	}
	proof := row.proofs[ref.arm]
	resetSet, resetDigest, resetCount, proofOK := r.boundaryResetSet(ref.index, ref.arm, row)
	if !proofOK {
		return RouteIdentity{}, nil, false
	}
	id := RouteIdentity{
		SourceID: r.sourceID, FlowID: r.flowID, StaticID: r.staticID, ModuleID: r.moduleID,
		From: row.Call, To: to, Decision: decision, Truth: truth, Mu: proof.mu, Arm: ref.arm,
		ResetDigest: resetDigest, ResetCount: resetCount,
	}
	id.Digest = hashRoute(id)
	if !r.routesReady {
		r.boundaries.rows[ref.index].proofs[ref.arm].resetDigest = resetDigest
		r.boundaries.rows[ref.index].proofs[ref.arm].resetCount = resetCount
	}
	return id, resetSet, true
}

// routeIdentityFastForRef projects the already-sealed route witness without
// rebuilding the canonical reset set or hashing the preimage. It is the hot
// query path used by At and Resolve: the sorted inverse has already attached
// the digest to this existing successor ref, while the immutable row carries
// only the reset commitment needed to authenticate the rest of the preimage.
func (r *Result) routeIdentityFastForRef(ref successorRef) (RouteIdentity, bool) {
	if r == nil || !r.available() {
		return RouteIdentity{}, false
	}
	if !isKnownArm(ref.arm) {
		return RouteIdentity{}, false
	}
	if ref.local {
		if uint64(ref.index) >= uint64(len(r.edges.rows)) || !isLocalArm(ref.arm) {
			return RouteIdentity{}, false
		}
		row := r.edges.rows[ref.index]
		if !r.validEdgeRow(row) {
			return RouteIdentity{}, false
		}
		resetDigest, resetCount, ok := edgeResetWitness(row)
		if !ok {
			return RouteIdentity{}, false
		}
		id := RouteIdentity{
			SourceID: r.sourceID, FlowID: r.flowID, StaticID: r.staticID, ModuleID: r.moduleID,
			From: row.From, To: row.To, Decision: row.Decision, Truth: row.Truth, Mu: row.Mu,
			Arm: BoundaryLocal, ResetDigest: resetDigest, ResetCount: resetCount, Digest: ref.routeDigest,
		}
		return id, id.available()
	}
	if uint64(ref.index) >= uint64(len(r.boundaries.rows)) || !isBoundaryArm(ref.arm) {
		return RouteIdentity{}, false
	}
	row := r.boundaries.rows[ref.index]
	if !r.validBoundaryRow(row) || !boundaryArmPresent(row, ref.arm) {
		return RouteIdentity{}, false
	}
	to, decision, truth, ok := boundarySuccessor(row.CallBoundary, ref.arm)
	if !ok {
		return RouteIdentity{}, false
	}
	proof := row.proofs[ref.arm]
	if !r.validBoundaryProof(row, ref.arm) {
		return RouteIdentity{}, false
	}
	resetDigest, resetCount, ok := boundaryResetWitness(proof)
	if !ok {
		return RouteIdentity{}, false
	}
	id := RouteIdentity{
		SourceID: r.sourceID, FlowID: r.flowID, StaticID: r.staticID, ModuleID: r.moduleID,
		From: row.Call, To: to, Decision: decision, Truth: truth, Mu: proof.mu, Arm: ref.arm,
		ResetDigest: resetDigest, ResetCount: resetCount, Digest: ref.routeDigest,
	}
	return id, id.available()
}

func edgeResetWitness(row edgeRow) (keyspace.ContentID, uint32, bool) {
	if row.Mu == 0 {
		if row.resetStart != 0 || row.resetPast != 0 || row.resetCount != 0 || row.resetDigest.Available() {
			return keyspace.ContentID{}, 0, false
		}
		return keyspace.ContentID{}, 0, true
	}
	if !row.resetDigest.Available() || row.resetPast < row.resetStart || row.resetCount != row.resetPast-row.resetStart {
		return keyspace.ContentID{}, 0, false
	}
	return row.resetDigest, row.resetCount, true
}

func boundaryResetWitness(proof boundaryRecurrenceProof) (keyspace.ContentID, uint32, bool) {
	if proof.mu == 0 {
		if proof.resetStart != 0 || proof.resetPast != 0 || proof.resetCount != 0 || proof.resetDigest.Available() {
			return keyspace.ContentID{}, 0, false
		}
		return keyspace.ContentID{}, 0, true
	}
	if !proof.resetDigest.Available() || proof.resetPast < proof.resetStart || proof.resetCount != proof.resetPast-proof.resetStart {
		return keyspace.ContentID{}, 0, false
	}
	return proof.resetDigest, proof.resetCount, true
}

// canonicalResetSet validates the existing Mu range and returns a sorted
// seal-local copy. The reset store remains the only retained relation.
func (r *Result) canonicalResetSet(row edgeRow) ([]keyspace.Term, keyspace.ContentID, uint32, bool) {
	return r.canonicalResetRange(row.Mu, row.resetStart, row.resetPast)
}

type resetEntry struct {
	term keyspace.Term
	path keyspace.ContentID
}

func (r *Result) canonicalResetRange(mu keyspace.Term, resetStart, resetPast uint32) ([]keyspace.Term, keyspace.ContentID, uint32, bool) {
	if mu == 0 {
		if resetStart != 0 || resetPast != 0 {
			return nil, keyspace.ContentID{}, 0, false
		}
		return nil, keyspace.ContentID{}, 0, true
	}
	start, end, ok := r.muRange(mu)
	if !ok || resetPast < resetStart || uint64(resetPast) > uint64(end-start) ||
		uint64(start)+uint64(resetPast) > uint64(len(r.reset.streams)) || resetStart > resetPast {
		return nil, keyspace.ContentID{}, 0, false
	}
	count := resetPast - resetStart
	entries := make([]resetEntry, int(count))
	for index := uint32(0); index < count; index++ {
		term := r.reset.streams[uint64(start)+uint64(resetStart)+uint64(index)]
		if !isDecision(term) || !r.validResultTerm(term) {
			return nil, keyspace.ContentID{}, 0, false
		}
		path, pathOK := r.semanticTermPath(term)
		if !pathOK {
			return nil, keyspace.ContentID{}, 0, false
		}
		entries[index] = resetEntry{term: term, path: path}
	}
	radixResetEntries(entries)
	terms := make([]keyspace.Term, len(entries))
	paths := make([]keyspace.ContentID, len(entries))
	for index, entry := range entries {
		terms[index], paths[index] = entry.term, entry.path
	}
	for index := 1; index < len(entries); index++ {
		if entries[index-1].path == entries[index].path {
			return nil, keyspace.ContentID{}, 0, false
		}
	}
	return terms, hashResetPaths(paths), count, true
}

func radixResetEntries(entries []resetEntry) {
	if len(entries) < 2 {
		return
	}
	work := make([]resetEntry, len(entries))
	for byteIndex := len(keyspace.ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var counts [256]int
		for _, entry := range entries {
			counts[entry.path[byteIndex]]++
		}
		at := 0
		for index := range counts {
			at, counts[index] = at+counts[index], at
		}
		for _, entry := range entries {
			bucket := entry.path[byteIndex]
			work[counts[bucket]] = entry
			counts[bucket]++
		}
		copy(entries, work)
	}
}

func (r *Result) boundaryResetSet(index uint32, arm BoundaryArmKind, row boundaryRow) ([]keyspace.Term, keyspace.ContentID, uint32, bool) {
	if !r.validBoundaryProof(row, arm) {
		return nil, keyspace.ContentID{}, 0, false
	}
	proof := row.proofs[arm]
	return r.canonicalResetRange(proof.mu, proof.resetStart, proof.resetPast)
}

func hashResetPaths(paths []keyspace.ContentID) keyspace.ContentID {
	var buffer bytes.Buffer
	buffer.WriteString(resetDigestDomain)
	buffer.WriteByte(0)
	var scalar [4]byte
	binary.BigEndian.PutUint32(scalar[:], uint32(len(paths)))
	buffer.Write(scalar[:])
	for _, path := range paths {
		buffer.Write(path[:])
	}
	return keyspace.ContentID(sha256.Sum256(buffer.Bytes()))
}

func hashRoute(id RouteIdentity) keyspace.ContentID {
	var encoded [256]byte
	offset := copy(encoded[:], routeDigestDomain)
	encoded[offset] = 0
	offset++
	binary.BigEndian.PutUint64(encoded[offset:], routeDigestVersion)
	offset += 8
	for _, owner := range [...]keyspace.ContentID{id.SourceID, id.FlowID, id.StaticID, id.ModuleID} {
		offset += copy(encoded[offset:], owner[:])
	}
	var term [4]byte
	for _, value := range [...]keyspace.Term{id.From, id.To, id.Decision, id.Mu} {
		binary.BigEndian.PutUint32(term[:], uint32(value))
		offset += copy(encoded[offset:], term[:])
	}
	if id.Truth {
		encoded[offset] = 1
	} else {
		encoded[offset] = 0
	}
	offset++
	encoded[offset] = byte(id.Arm)
	offset++
	binary.BigEndian.PutUint32(term[:], id.ResetCount)
	offset += copy(encoded[offset:], term[:])
	offset += copy(encoded[offset:], id.ResetDigest[:])
	return keyspace.ContentID(sha256.Sum256(encoded[:offset]))
}

func compareRouteID(left, right RouteIdentity) bool {
	return left.available() && right.available() && left == right
}
