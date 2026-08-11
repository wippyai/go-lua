package causal

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/program/keyspace"
)

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
		return keyspace.TermFamily(id.From) == keyspace.FamilyCall && id.Mu == 0 && id.Decision != 0 && keyspace.TermFamily(id.Decision) == keyspace.FamilySelect && id.Truth && id.ResetCount == 0 && !id.ResetDigest.Available() && id.Digest == hashRoute(id)
	case BoundarySelectFalse:
		return keyspace.TermFamily(id.From) == keyspace.FamilyCall && id.Mu == 0 && id.Decision != 0 && keyspace.TermFamily(id.Decision) == keyspace.FamilySelect && !id.Truth && id.ResetCount == 0 && !id.ResetDigest.Available() && id.Digest == hashRoute(id)
	default:
		return keyspace.TermFamily(id.From) == keyspace.FamilyCall && id.Mu == 0 && id.Decision == 0 && !id.Truth && id.ResetCount == 0 && !id.ResetDigest.Available() && id.Digest == hashRoute(id)
	}
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
		descriptors = append(descriptors, routeDescriptor{id: id, ref: ref, resetSet: resetSet})
	}
	sort.Slice(descriptors, func(left, right int) bool {
		return bytes.Compare(descriptors[left].id.Digest[:], descriptors[right].id.Digest[:]) < 0
	})
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
		r.routeIndex[index] = routeLookup{digest: descriptor.id.Digest, ref: descriptor.ref}
	}
	r.routesReady = true
	return nil
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
	id := RouteIdentity{
		SourceID: r.sourceID, FlowID: r.flowID, StaticID: r.staticID, ModuleID: r.moduleID,
		From: row.Call, To: to, Decision: decision, Truth: truth, Mu: 0, Arm: ref.arm,
	}
	id.Digest = hashRoute(id)
	return id, nil, true
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
	id := RouteIdentity{
		SourceID: r.sourceID, FlowID: r.flowID, StaticID: r.staticID, ModuleID: r.moduleID,
		From: row.Call, To: to, Decision: decision, Truth: truth, Mu: 0, Arm: ref.arm, Digest: ref.routeDigest,
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

// canonicalResetSet validates the existing Mu range and returns a sorted
// seal-local copy. The reset store remains the only retained relation.
func (r *Result) canonicalResetSet(row edgeRow) ([]keyspace.Term, keyspace.ContentID, uint32, bool) {
	if row.Mu == 0 {
		if row.resetStart != 0 || row.resetPast != 0 || row.resetCount != 0 || row.resetDigest.Available() {
			return nil, keyspace.ContentID{}, 0, false
		}
		return nil, keyspace.ContentID{}, 0, true
	}
	start, end, ok := r.muRange(row.Mu)
	if !ok || row.resetPast < row.resetStart || uint64(row.resetPast) > uint64(end-start) ||
		uint64(start)+uint64(row.resetPast) > uint64(len(r.reset.streams)) || row.resetStart > row.resetPast {
		return nil, keyspace.ContentID{}, 0, false
	}
	count := row.resetPast - row.resetStart
	terms := make([]keyspace.Term, int(count))
	for index := uint32(0); index < count; index++ {
		term := r.reset.streams[uint64(start)+uint64(row.resetStart)+uint64(index)]
		if !isDecision(term) || !r.validResultTerm(term) {
			return nil, keyspace.ContentID{}, 0, false
		}
		terms[index] = term
	}
	sort.Slice(terms, func(left, right int) bool { return terms[left] < terms[right] })
	for index := 1; index < len(terms); index++ {
		if terms[index-1] == terms[index] {
			return nil, keyspace.ContentID{}, 0, false
		}
	}
	return terms, hashReset(terms), count, true
}

func hashReset(terms []keyspace.Term) keyspace.ContentID {
	var buffer bytes.Buffer
	buffer.WriteString(resetDigestDomain)
	buffer.WriteByte(0)
	var scalar [4]byte
	binary.BigEndian.PutUint32(scalar[:], uint32(len(terms)))
	buffer.Write(scalar[:])
	for _, term := range terms {
		binary.BigEndian.PutUint32(scalar[:], uint32(term))
		buffer.Write(scalar[:])
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
