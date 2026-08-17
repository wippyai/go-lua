package sourcecontrol

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

const noNode = ^uint32(0)

// Arc is one semantic structural witness.  From and To disambiguate phases
// that share the same typed Source/Target anchor.  Decision is nonzero only
// for guarded Branch/Loop rows; Truth is meaningful exactly then.
type Arc struct {
	From     uint32
	To       uint32
	Source   keyspace.Term
	Target   keyspace.Term
	Decision keyspace.Term
	Truth    bool
}

type coordinateProof struct {
	bodyOffsets  []uint32
	loopDecision []uint32
	repeatBody   []uint32
	nodeCount    uint32
}

type adjacencyProof struct {
	forwardOffsets []uint32
	forwardTargets []uint32
	reverseOffsets []uint32
	reverseTargets []uint32
}

type witnessProof struct {
	rows          []Arc
	sourceOffsets [keyspace.FamilyCount][]uint32
	sourceIndexes [keyspace.FamilyCount][]uint32
}

// resumeProof is the sealed structural continuation projection for the two
// source-control families whose continuation is not recoverable from an
// ordinary executable root.  The slices are dense by their one-based family
// ordinal; a zero Term is never a valid sealed target.
type resumeProof struct {
	labels []keyspace.Term
	loops  []keyspace.Term
}

type Result struct {
	sourceID    identity.ContentID
	coordinates coordinateProof
	resumes     resumeProof
	adjacency   adjacencyProof
	witnesses   witnessProof
	reachable   []uint64
	dominance   dominanceProof
	flowID      identity.ContentID
	staticID    identity.ContentID
	moduleID    identity.ContentID
	// catalog is shared by copied Result values. Its lifecycle cannot be
	// forked by struct copying and release requires the assembler-held lease.
	catalog *catalogLifecycle
	// outcomePhases is the immutable SourceControl-owned extension for
	// non-Normal Outcome phases. It is deliberately separate from
	// Reachable/CSR: an Outcome phase is a schedule point, never a structural
	// fallthrough.
	outcomePhases *OutcomePhases
}

func (r *Result) ownerAvailable() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available()
}

// Matches reports whether r was sealed for the exact Source, authored Flow,
// Static, and Module identities supplied by the final assembly.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// available is the query fence for the published structural proof. A Result
// with plausible coordinates or graph rows but any unavailable owner identity
// is not a usable source-control authority. The semantic vertex catalog is a
// separate, temporary materialization lease: structural consumers (including
// Executable) must be able to consume the sealed coordinate space before that
// lease is installed, and Arc witnesses remain usable after it is released.
func (r *Result) available() bool {
	return r.ownerAvailable()
}

// NodeCount reports the dense source-control coordinate denominator.
func (r *Result) NodeCount() uint32 {
	if !r.available() {
		return 0
	}
	return r.coordinates.nodeCount
}

// Cursor resolves an ordinary Body cursor. Cursor zero is the Body start and
// the cursor equal to RootCount is its tail.
func (r *Result) Cursor(body keyspace.Term, cursor uint32) (uint32, bool) {
	if !r.available() || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.coordinates.bodyOffsets)) {
		return 0, false
	}
	start := r.coordinates.bodyOffsets[ordinal-1]
	end := r.coordinates.bodyOffsets[ordinal]
	if end <= start || cursor > end-start-1 || start+cursor >= r.coordinates.nodeCount {
		return 0, false
	}
	return start + cursor, true
}

// Tail resolves the Body tail coordinate, the cursor immediately following
// its final statement root.
func (r *Result) Tail(body keyspace.Term) (uint32, bool) {
	if !r.available() || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.coordinates.bodyOffsets)) {
		return 0, false
	}
	start := r.coordinates.bodyOffsets[ordinal-1]
	end := r.coordinates.bodyOffsets[ordinal]
	if end <= start || end-1 >= r.coordinates.nodeCount {
		return 0, false
	}
	return end - 1, true
}

// Decision resolves a hidden dynamic-loop decision. While and static loops
// return false. The returned coordinate is not a source Term coordinate.
func (r *Result) Decision(loop keyspace.Term) (uint32, bool) {
	if !r.available() || keyspace.TermFamily(loop) != keyspace.FamilyLoop {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(loop)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.coordinates.loopDecision)) {
		return 0, false
	}
	node := r.coordinates.loopDecision[ordinal]
	return node, node != noNode && node < r.coordinates.nodeCount
}

// Coordinate resolves an authored occurrence through Source's sealed
// Frontier relation. Repeat frontiers are remapped to their hidden decision;
// every other frontier maps directly to its Body cursor.
func (r *Result) Coordinate(view source.View, term keyspace.Term) (uint32, bool) {
	if !r.available() {
		return 0, false
	}
	return coordinateForTerm(view, r.sourceID, &r.coordinates, term)
}

// Resume resolves the canonical structural anchor selected after a Label or
// Loop source occurrence.  A returned Body denotes that Body's tail/Normal
// outcome; any other returned Term denotes the next direct root whose Entry
// is used by the later causal projection.  The dense family slices make this
// query allocation-free and independent of Source order after sealing.
func (r *Result) Resume(term keyspace.Term) (keyspace.Term, bool) {
	if !r.available() {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	var targets []keyspace.Term
	switch family {
	case keyspace.FamilyLabel:
		targets = r.resumes.labels
	case keyspace.FamilyLoop:
		targets = r.resumes.loops
	default:
		return 0, false
	}
	if ordinal == 0 || uint64(ordinal) > uint64(len(targets)) {
		return 0, false
	}
	target := targets[ordinal-1]
	return target, target != 0
}

// ArcCount reports the canonical semantic witness denominator.
func (r *Result) ArcCount() int {
	if !r.available() {
		return 0
	}
	return len(r.witnesses.rows)
}

// ArcAt returns one semantic witness in deterministic fixed owner-seal
// emission order.
func (r *Result) ArcAt(index int) (Arc, bool) {
	if !r.available() || index < 0 || index >= len(r.witnesses.rows) {
		return Arc{}, false
	}
	return r.witnesses.rows[index], true
}

// ArcCountAtSource reports the number of witnesses selected by one exact
// Source anchor. The grouped index is dense and allocation-free.
func (r *Result) ArcCountAtSource(sourceTerm keyspace.Term) int {
	_, start, end, ok := r.sourceRange(sourceTerm)
	if !ok {
		return 0
	}
	return int(end - start)
}

// ArcAtSource returns the global canonical Arc ordinal together with the
// witness selected by Source anchor and local index. The ordinal is the same
// zero-based index accepted by ArcAt, so callers can cite recurrence
// annotations without copying the Arc storage.
func (r *Result) ArcAtSource(sourceTerm keyspace.Term, index int) (int, Arc, bool) {
	family, start, end, ok := r.sourceRange(sourceTerm)
	if !ok || index < 0 || uint64(index) >= uint64(end-start) {
		return 0, Arc{}, false
	}
	indexes := r.witnesses.sourceIndexes[family]
	if uint64(start+uint32(index)) >= uint64(len(indexes)) {
		return 0, Arc{}, false
	}
	row := indexes[start+uint32(index)]
	if uint64(row) >= uint64(len(r.witnesses.rows)) {
		return 0, Arc{}, false
	}
	return int(row), r.witnesses.rows[row], true
}

// SuccessorCount reports unique structural destinations leaving node.
func (r *Result) SuccessorCount(node uint32) int {
	start, end, ok := r.forwardRange(node)
	if !ok {
		return 0
	}
	return int(end - start)
}

// SuccessorAt returns one unique structural destination in ascending order.
func (r *Result) SuccessorAt(node uint32, index int) (uint32, bool) {
	start, end, ok := r.forwardRange(node)
	if !ok || index < 0 || uint64(index) >= uint64(end-start) {
		return 0, false
	}
	to := r.adjacency.forwardTargets[start+uint32(index)]
	return to, to < r.coordinates.nodeCount
}

// PredecessorCount reports unique structural predecessors entering node.
func (r *Result) PredecessorCount(node uint32) int {
	start, end, ok := r.reverseRange(node)
	if !ok {
		return 0
	}
	return int(end - start)
}

// PredecessorAt returns one unique structural predecessor in ascending order.
func (r *Result) PredecessorAt(node uint32, index int) (uint32, bool) {
	start, end, ok := r.reverseRange(node)
	if !ok || index < 0 || uint64(index) >= uint64(end-start) {
		return 0, false
	}
	from := r.adjacency.reverseTargets[start+uint32(index)]
	return from, from < r.coordinates.nodeCount
}

// Reachable reports membership in the least structural reachable set.
func (r *Result) Reachable(node uint32) bool {
	return r.available() && node < r.coordinates.nodeCount && bitSet(r.reachable, node)
}

// Dominates answers the sealed virtual-rooted dominance proof in O(1).
func (r *Result) Dominates(ancestor, descendant uint32) bool {
	return r.available() && r.dominance.dominates(ancestor, descendant)
}

func (r *Result) sourceRange(term keyspace.Term) (keyspace.Family, uint32, uint32, bool) {
	if !r.available() {
		return keyspace.FamilyInvalid, 0, 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return keyspace.FamilyInvalid, 0, 0, false
	}
	offsets := r.witnesses.sourceOffsets[family]
	if uint64(ordinal) >= uint64(len(offsets)) {
		return keyspace.FamilyInvalid, 0, 0, false
	}
	start, end := offsets[ordinal-1], offsets[ordinal]
	indexes := r.witnesses.sourceIndexes[family]
	if end < start || uint64(end) > uint64(len(indexes)) {
		return keyspace.FamilyInvalid, 0, 0, false
	}
	return family, start, end, true
}

func (r *Result) forwardRange(node uint32) (uint32, uint32, bool) {
	if !r.available() || node >= r.coordinates.nodeCount || len(r.adjacency.forwardOffsets) != int(r.coordinates.nodeCount)+1 {
		return 0, 0, false
	}
	start, end := r.adjacency.forwardOffsets[node], r.adjacency.forwardOffsets[node+1]
	if end < start || uint64(end) > uint64(len(r.adjacency.forwardTargets)) {
		return 0, 0, false
	}
	return start, end, true
}

func (r *Result) reverseRange(node uint32) (uint32, uint32, bool) {
	if !r.available() || node >= r.coordinates.nodeCount || len(r.adjacency.reverseOffsets) != int(r.coordinates.nodeCount)+1 {
		return 0, 0, false
	}
	start, end := r.adjacency.reverseOffsets[node], r.adjacency.reverseOffsets[node+1]
	if end < start || uint64(end) > uint64(len(r.adjacency.reverseTargets)) {
		return 0, 0, false
	}
	return start, end, true
}

// coordinateForTerm is the sole occurrence-to-coordinate mapping used during
// sealing and after Result publication. Source's exact Root and Frontier are
// authoritative. Only a term rooted at a Repeat Loop and sitting at that
// Loop's child-body tail is remapped to the hidden decision; a trailing Label
// or another term sharing the tail frontier is never remapped.
func coordinateForTerm(view source.View, sourceID identity.ContentID, proof *coordinateProof, term keyspace.Term) (uint32, bool) {
	if proof == nil || !sourceID.Available() || !view.Identity().ContentID().Available() || view.Identity().ContentID() != sourceID {
		return 0, false
	}
	root, rootOK := view.Index().Root(term)
	body, cursor, frontierOK := view.Index().Frontier(term)
	if !rootOK || !frontierOK || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	if keyspace.TermFamily(root) == keyspace.FamilyLoop {
		rootOrdinal := keyspace.TermOrdinal(root)
		bodyOrdinal := keyspace.TermOrdinal(body)
		if rootOrdinal != 0 && uint64(rootOrdinal) < uint64(len(proof.loopDecision)) &&
			bodyOrdinal != 0 && uint64(bodyOrdinal) < uint64(len(proof.repeatBody)) {
			decision := proof.loopDecision[rootOrdinal]
			if decision != noNode && decision == proof.repeatBody[bodyOrdinal] {
				start, end := proof.bodyOffsets[bodyOrdinal-1], proof.bodyOffsets[bodyOrdinal]
				if end > start && cursor == int(end-start-1) && decision < proof.nodeCount {
					return decision, true
				}
			}
		}
	}
	return cursorForCoordinates(proof, body, cursor)
}

func cursorForCoordinates(proof *coordinateProof, body keyspace.Term, cursor int) (uint32, bool) {
	if proof == nil || cursor < 0 || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(proof.bodyOffsets)) {
		return 0, false
	}
	start, end := proof.bodyOffsets[ordinal-1], proof.bodyOffsets[ordinal]
	if end <= start || uint64(cursor) >= uint64(end-start) {
		return 0, false
	}
	node := start + uint32(cursor)
	return node, node < proof.nodeCount
}
