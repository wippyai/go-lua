package engine

import (
	"slices"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/dependency"
	"github.com/wippyai/go-lua/analysis/engine/internal/fiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// bodyProjection identifies one existing local equation in a compiled Body.
// It deliberately has no Candidate: candidates retain separate structural
// coordinates while equal formal projections may reuse this equation result.
type bodyProjection struct {
	key     bodyProjectionKey
	outputs []int
}

func (projection bodyProjection) available() bool {
	return projection.key.body.shard != 0 && projection.key.body.body != 0 && len(projection.outputs) != 0
}

func (solver *Solver) edgeBodyProjection(origin edgeOrigin, members []ruleDeclaration) (bodyProjection, bool) {
	if solver == nil || origin.candidate == (link.Candidate{}) || origin.shard == 0 || origin.edge.From() == 0 || origin.edge.To() == 0 {
		return bodyProjection{}, true
	}
	body, ok := solver.candidateBodyOrigin(origin.candidate, origin.shard, origin.edge.From())
	if !ok {
		return bodyProjection{}, false
	}
	outputs, cacheable := formalOutputs(members)
	if !cacheable {
		return bodyProjection{}, true
	}
	key, ok := normalizedBodyProjectionKey(body, origin.edge, 0, false)
	if !ok {
		return bodyProjection{}, false
	}
	return bodyProjection{key: key, outputs: outputs}, true
}

func (solver *Solver) localBodyProjection(origin termOrigin, member ruleDeclaration) (bodyProjection, bool) {
	if solver == nil || origin.candidate == (link.Candidate{}) || origin.shard == 0 || origin.term == 0 {
		return bodyProjection{}, true
	}
	body, ok := solver.candidateBodyOrigin(origin.candidate, origin.shard, origin.term)
	if !ok {
		return bodyProjection{}, false
	}
	outputs, cacheable := formalOutputs([]ruleDeclaration{member})
	if !cacheable {
		return bodyProjection{}, true
	}
	key, ok := normalizedBodyProjectionKey(body, program.Edge{}, origin.term, true)
	if !ok {
		return bodyProjection{}, false
	}
	return bodyProjection{key: key, outputs: outputs}, true
}

func (solver *Solver) candidateBodyOrigin(candidate link.Candidate, shard link.Shard, term program.Term) (bodyOrigin, bool) {
	if solver == nil || solver.link == nil || candidate == (link.Candidate{}) || shard == 0 || term == 0 {
		return bodyOrigin{}, false
	}
	selectedShard, body, ok := solver.link.CandidateBody(candidate)
	if !ok || selectedShard != shard {
		return bodyOrigin{}, false
	}
	p, ok := solver.link.Program(shard)
	if !ok || p == nil {
		return bodyOrigin{}, false
	}
	activation, ok := p.Activation(term)
	origin := bodyOrigin{shard: shard, body: body}
	_, compiled := solver.bodies[origin]
	return origin, ok && activation == body && compiled
}

func formalOutputs(members []ruleDeclaration) ([]int, bool) {
	if len(members) == 0 {
		return nil, false
	}
	outputs := make([]int, len(members))
	for index, member := range members {
		if member.slot == nil || member.formal == nil || !member.formal() {
			return nil, false
		}
		slot, ok := member.slot()
		if !ok || slot < 0 {
			return nil, false
		}
		outputs[index] = slot
	}
	slices.Sort(outputs)
	for index := 1; index < len(outputs); index++ {
		if outputs[index-1] == outputs[index] {
			return nil, false
		}
	}
	return outputs, true
}

type bodyProjectionKey struct {
	body  bodyOrigin
	edge  program.Edge
	term  program.Term
	local bool
}

// normalizedBodyProjectionKey is the sole specialization identity constructor.
// Candidate selects a Program Body, but is intentionally absent here: equal
// lexical activations must share one formal equation cache even when their
// calling coordinates and non-formal Factor columns differ.  Normalizing that
// structural selection before identity is what prevents a Candidate-indexed
// parallel executor from reappearing behind the cache.
func normalizedBodyProjectionKey(body bodyOrigin, edge program.Edge, term program.Term, local bool) (bodyProjectionKey, bool) {
	if body.shard == 0 || body.body == 0 {
		return bodyProjectionKey{}, false
	}
	if local {
		if term == 0 || edge != (program.Edge{}) {
			return bodyProjectionKey{}, false
		}
		return bodyProjectionKey{body: body, term: term, local: true}, true
	}
	if term != 0 || edge.From() == 0 || edge.To() == 0 {
		return bodyProjectionKey{}, false
	}
	return bodyProjectionKey{body: body, edge: edge}, true
}

// bodySpecialization is one immutable formal equation result. reads are
// typed closures rather than an erased payload: each validates one exact
// Factor/key/presence/value projection and re-registers that read through the
// normal action Equation on a hit.
type bodySpecialization struct {
	reads  []bodyProjectionRead
	result fiber.Guarded
}

// bodyProjectionRead is a typed semantic read descriptor. The compact fields
// select an adaptive index branch; validate is deliberately still the
// authority for semantic equality. Its monomorphized closures retain no
// erased value, reflection, or string key path.
type bodyProjectionRead struct {
	slot        int
	key         uint64
	present     bool
	fingerprint uint64
	observe     func(*transaction, dependency.Equation, coordinate.Coordinate, guard.Guard, fiber.Leaf) (bodyProjectionSample, bool)
	validate    func(*transaction, dependency.Equation, coordinate.Coordinate, guard.Guard, fiber.Leaf) bool
}

type bodyProjectionSample struct {
	present     bool
	fingerprint uint64
}

func (read bodyProjectionRead) sample() bodyProjectionSample {
	return bodyProjectionSample{present: read.present, fingerprint: read.fingerprint}
}

func (read bodyProjectionRead) valid() bool {
	return read.slot >= 0 && read.observe != nil && read.validate != nil
}

func (read bodyProjectionRead) sameLocation(other bodyProjectionRead) bool {
	return read.slot == other.slot && read.key == other.key
}

// bodyProjectionCapture exists only during one ordinary Rule application.
// The evaluator first runs the same compiled action. After that action
// succeeds, the captured typed reads become a body-owned reusable projection.
type bodyProjectionCapture struct {
	projection bodyProjection
	reads      []bodyProjectionRead
	usable     bool
}

// bodyProjectionIndex is one adaptive, exact decision index. Cold admission
// adds one descriptor path; it never rebuilds the index from every retained
// record. A warm hit walks selected typed reads and one fingerprint branch per
// level, then validates only the collision bucket. There is deliberately no
// all-record fallback scan.
type bodyProjectionIndex struct {
	records []bodySpecialization
	root    *bodyProjectionNode
}

type bodyProjectionNode struct {
	selector bodyProjectionRead
	branches map[bodyProjectionSample]*bodyProjectionNode
	bucket   *bodyProjectionBucket
}

// bodyProjectionBucket is an exact collision bucket. It exists only below a
// complete materialized path of uniform read descriptors, so a later distinct
// sample attaches at that path instead of repartitioning/copying old records.
// Only equal compact samples can reach the bucket; exact validators decide
// among those records.
type bodyProjectionBucket struct{ records []int }

// bodyReuse is Solver-private operational indexing of immutable formal body
// results. The Solver owns one flat canonical cache for a carrier epoch. A
// transaction's separate, bounded admission batch is never an overlay: it
// has the same exact index shape and is consulted only by the one reuse
// operation before the canonical cache, so equal projections share within a
// generation without extending a parent chain.
type bodyReuse struct {
	byProjection map[bodyProjectionKey]*bodyProjectionIndex
}

// bodyAdmissionBatch is transaction-private staging, never a second
// canonical cache. records supplies the existing same-generation reuse while
// proof owns one copy-on-write structural successor of each touched canonical
// index. The successor is never visible through Solver.bodyReuse: terminal
// publication replaces the canonical entry with it in one step.
type bodyAdmissionBatch struct {
	records *bodyReuse
	proof   map[bodyProjectionKey]bodyAdmissionProof
}

type bodyAdmissionProof struct {
	index     *bodyProjectionIndex
	published int
}

// bodyProjectionAdmission is the normalized, shape-checked cache decision
// for one cold body execution.  It is deliberately made before the result
// Fiber is retained: an equation that cannot join the existing exact index
// remains a successful cold execution, not a published cache root.
//
// A transaction is single-writer, so once admission succeeds commit has no
// competing mutation to re-check.  Keeping this capability private prevents
// callers from retaining first and treating an index rejection as success.
type bodyProjectionAdmission struct {
	key   bodyProjectionKey
	item  bodySpecialization
	index *bodyProjectionIndex
}

func newBodyReuse() *bodyReuse {
	return &bodyReuse{byProjection: make(map[bodyProjectionKey]*bodyProjectionIndex)}
}

func newBodyAdmissionBatch() *bodyAdmissionBatch {
	return &bodyAdmissionBatch{
		records: newBodyReuse(),
		proof:   make(map[bodyProjectionKey]bodyAdmissionProof),
	}
}

func (reuse *bodyReuse) append(projection bodyProjection, item bodySpecialization) bool {
	admission, ok := reuse.admit(projection, item)
	if !ok {
		return false
	}
	return reuse.commit(admission)
}

func (reuse *bodyReuse) appendKey(key bodyProjectionKey, item bodySpecialization) bool {
	if reuse == nil {
		return false
	}
	admission, ok := reuse.admitKey(key, item)
	if !ok {
		return false
	}
	return reuse.commit(admission)
}

// admit normalizes the exact read shape and proves that it can be inserted
// into this reuse index without modifying it.  Rejection is cacheability,
// not evaluation failure.
func (reuse *bodyReuse) admit(projection bodyProjection, item bodySpecialization) (bodyProjectionAdmission, bool) {
	if reuse == nil || !projection.available() {
		return bodyProjectionAdmission{}, false
	}
	return reuse.admitKey(projection.key, item)
}

// accepts is the non-mutating admission check for an existing canonical
// cache. It lets a transaction reject a result that could not be promoted
// before retaining its Fiber root, while keeping that cache unchanged until
// terminal publication.
func (reuse *bodyReuse) accepts(projection bodyProjection, item bodySpecialization) bool {
	if reuse == nil || !projection.available() {
		return false
	}
	reads, ok := normalizedBodyProjectionReads(item.reads)
	if !ok {
		return false
	}
	index := reuse.byProjection[projection.key]
	return index == nil || index.accepts(bodySpecialization{reads: reads, result: item.result})
}

// admit proves and records one candidate against the canonical index as it
// will stand after all earlier records in this transaction.  The proof is a
// private copy-on-write structural successor, created once per projection;
// it retains no Fiber root and cannot be observed by lookup.  Thus a rejected
// candidate remains a successful cold execution before Retain, while a later
// terminal publication can install the already-proved successor without
// replaying or rechecking cacheability after Freeze.
//
// Checking only canonical.accepts and pending.records.admit is unsound.
// Consider a published X=0 record, a pending X=1 record, then a Y-only
// record.  Each is independently admissible to the old indexes, but the
// evolved canonical tree selects X and cannot admit Y-only.  The private
// successor rejects that third record before its Fiber root is retained.
func (batch *bodyAdmissionBatch) admit(canonical *bodyReuse, projection bodyProjection, item bodySpecialization) (bodyProjectionAdmission, bool) {
	if batch == nil || batch.records == nil || canonical == nil || !projection.available() {
		return bodyProjectionAdmission{}, false
	}
	admission, ok := batch.records.admit(projection, item)
	if !ok {
		return bodyProjectionAdmission{}, false
	}
	proof, found := batch.proof[projection.key]
	if !found {
		proof.index = cloneBodyProjectionIndex(canonical.byProjection[projection.key])
		if proof.index == nil {
			return bodyProjectionAdmission{}, false
		}
		proof.published = len(proof.index.records)
	}
	if !proof.index.appendNormalized(admission.item) {
		return bodyProjectionAdmission{}, false
	}
	batch.proof[projection.key] = proof
	return admission, true
}

func (batch *bodyAdmissionBatch) commit(admission bodyProjectionAdmission) bool {
	return batch != nil && batch.records != nil && batch.records.commit(admission)
}

func (batch *bodyAdmissionBatch) lookup(projection bodyProjection, transaction *transaction, equation dependency.Equation, coordinate coordinate.Coordinate, source fiber.Guarded) (bodySpecialization, bool) {
	if batch == nil || batch.records == nil {
		return bodySpecialization{}, false
	}
	return batch.records.lookup(projection, transaction, equation, coordinate, source)
}

func (reuse *bodyReuse) admitKey(key bodyProjectionKey, item bodySpecialization) (bodyProjectionAdmission, bool) {
	if reuse == nil {
		return bodyProjectionAdmission{}, false
	}
	reads, ok := normalizedBodyProjectionReads(item.reads)
	if !ok {
		return bodyProjectionAdmission{}, false
	}
	item.reads = reads
	index := reuse.byProjection[key]
	if index == nil {
		index = &bodyProjectionIndex{}
	}
	if !index.accepts(item) {
		return bodyProjectionAdmission{}, false
	}
	return bodyProjectionAdmission{key: key, item: item, index: index}, true
}

// commit is the second half of a successful admission.  Its input came from
// admit in this transaction's single-writer cache, so rejection here is an
// internal invariant violation rather than a second cacheability branch.
func (reuse *bodyReuse) commit(admission bodyProjectionAdmission) bool {
	if reuse == nil || admission.index == nil {
		return false
	}
	index := reuse.byProjection[admission.key]
	if index == nil {
		if !admission.index.appendNormalized(admission.item) {
			return false
		}
		reuse.byProjection[admission.key] = admission.index
		return true
	}
	if index != admission.index || !index.appendNormalized(admission.item) {
		return false
	}
	return true
}

// cloneBodyProjectionIndex copies one sealed cache index for a transaction's
// private structural successor.  It is iterative because read descriptors may
// induce a deep decision path.  Records and node containers are copied, while
// their immutable typed read closures are deliberately shared.
func cloneBodyProjectionIndex(source *bodyProjectionIndex) *bodyProjectionIndex {
	result := &bodyProjectionIndex{}
	if source == nil {
		return result
	}
	result.records = append([]bodySpecialization(nil), source.records...)
	if source.root == nil {
		return result
	}
	result.root = &bodyProjectionNode{}
	type nodePair struct {
		source *bodyProjectionNode
		result *bodyProjectionNode
	}
	pending := []nodePair{{source: source.root, result: result.root}}
	for len(pending) != 0 {
		last := len(pending) - 1
		pair := pending[last]
		pending = pending[:last]
		if pair.source == nil || pair.result == nil {
			return nil
		}
		pair.result.selector = pair.source.selector
		if pair.source.bucket != nil {
			pair.result.bucket = &bodyProjectionBucket{records: append([]int(nil), pair.source.bucket.records...)}
		}
		if len(pair.source.branches) == 0 {
			continue
		}
		pair.result.branches = make(map[bodyProjectionSample]*bodyProjectionNode, len(pair.source.branches))
		for sample, child := range pair.source.branches {
			if child == nil {
				return nil
			}
			next := &bodyProjectionNode{}
			pair.result.branches[sample] = next
			pending = append(pending, nodePair{source: child, result: next})
		}
	}
	return result
}

// accepts is the pure structural half of admission.  Every internal selector
// on the walked path must be present in the new record; a missing selector is
// the exact reason this record cannot share the index.  A new branch or an
// existing bucket is otherwise admissible, without a linear fallback scan.
func (index *bodyProjectionIndex) accepts(item bodySpecialization) bool {
	if index == nil {
		return false
	}
	if index.root == nil {
		return len(index.records) == 0
	}
	if len(index.records) == 0 {
		return false
	}
	for node := index.root; node != nil; {
		if !node.selector.valid() {
			return node.bucket != nil && len(node.bucket.records) != 0
		}
		read, found := findBodyProjectionRead(item.reads, node.selector)
		if !found {
			return false
		}
		next := node.branches[read.sample()]
		if next == nil {
			return true
		}
		node = next
	}
	return false
}

// appendNormalized mutates an already admitted index.  It deliberately has
// no cacheability outcome: callers must make that decision before retaining
// a candidate Fiber root.
func (index *bodyProjectionIndex) appendNormalized(item bodySpecialization) bool {
	if index == nil || !index.accepts(item) {
		return false
	}
	recordIndex := len(index.records)
	index.records = append(index.records, item)
	if index.root == nil {
		index.root = bodyProjectionLeaf(recordIndex)
		return true
	}
	if !index.insert(recordIndex) {
		return false
	}
	return true
}

func bodyProjectionLeaf(recordIndex int) *bodyProjectionNode {
	bucket := &bodyProjectionBucket{}
	bucket.append(recordIndex)
	return &bodyProjectionNode{bucket: bucket}
}

func (bucket *bodyProjectionBucket) append(recordIndex int) {
	if bucket == nil {
		return
	}
	bucket.records = append(bucket.records, recordIndex)
}

// promote atomically replaces each touched canonical projection with its
// already-proved successor.  No structural admission happens after Freeze:
// all that remains is translating the retained pending Fiber roots through
// the terminal publication capability.
func (reuse *bodyReuse) promote(batch *bodyAdmissionBatch, publication fiber.Publication) {
	if reuse == nil || batch == nil || len(batch.proof) == 0 {
		return
	}
	for key, proof := range batch.proof {
		if proof.index == nil || proof.published < 0 || proof.published > len(proof.index.records) {
			continue
		}
		for index := proof.published; index < len(proof.index.records); index++ {
			proof.index.records[index].result = publication.Root(proof.index.records[index].result)
		}
		reuse.byProjection[key] = proof.index
	}
}

// captureBodyRead records the semantic equality witness for one actual
// typed Factor read. A matching fingerprint only selects a candidate record;
// exact Factor equality remains decisive. The closure deliberately observes
// through the live Factor runtime on validation, so ordinary reverse-key
// invalidation wakes the reused action just as it wakes a cold application.
func captureBodyRead[K ~uint64, V any](capture *bodyProjectionCapture, input *Factor[K, V], key K, value V, present bool) {
	if capture == nil || !capture.usable || input == nil || input.live == nil || input.same == nil || input.fingerprint == nil {
		return
	}
	fingerprint := input.fingerprint(value)
	capture.reads = append(capture.reads, bodyProjectionRead{
		slot:        input.slot,
		key:         uint64(key),
		present:     present,
		fingerprint: fingerprint,
		observe: func(transaction *transaction, equation dependency.Equation, coordinate coordinate.Coordinate, condition guard.Guard, leaf fiber.Leaf) (bodyProjectionSample, bool) {
			if transaction == nil || input.live == nil || input.live.transaction != transaction || input.live.scratch == nil || input.fingerprint == nil {
				return bodyProjectionSample{}, false
			}
			root, ok := input.binding.LeafRoot(input.live.scratch, leaf)
			if !ok {
				return bodyProjectionSample{}, false
			}
			next, nextPresent, ok := input.live.observe(equation, coordinate, root, key, condition)
			if !ok {
				return bodyProjectionSample{}, false
			}
			return bodyProjectionSample{present: nextPresent, fingerprint: input.fingerprint(next)}, true
		},
		validate: func(transaction *transaction, equation dependency.Equation, coordinate coordinate.Coordinate, condition guard.Guard, leaf fiber.Leaf) bool {
			if transaction == nil || input.live == nil || input.live.transaction != transaction || input.live.scratch == nil || input.same == nil || input.fingerprint == nil {
				return false
			}
			root, ok := input.binding.LeafRoot(input.live.scratch, leaf)
			if !ok {
				return false
			}
			next, nextPresent, ok := input.live.observe(equation, coordinate, root, key, condition)
			return ok && nextPresent == present && input.fingerprint(next) == fingerprint && input.same(next, value)
		},
	})
}

func (transaction *transaction) beginBodyProjection(projection bodyProjection, source fiber.Guarded) *bodyProjectionCapture {
	if transaction == nil || !projection.available() || transaction.fibers == nil || transaction.guards == nil || !transaction.fibers.Unconditional(source) {
		return nil
	}
	return &bodyProjectionCapture{projection: projection, usable: true}
}

func (transaction *transaction) reuseBodyProjection(projection bodyProjection, action *compiledAction, input coordinate.Coordinate, source fiber.Guarded) (fiber.Guarded, bool, bool) {
	if transaction == nil || action == nil || !projection.available() || transaction.fibers == nil || transaction.guards == nil || !transaction.fibers.Unconditional(source) {
		return fiber.Guarded{}, false, true
	}
	if transaction.solver == nil || transaction.solver.bodyReuse == nil || transaction.bodyPending == nil {
		return fiber.Guarded{}, false, false
	}
	// This is the one exact reuse operation. The bounded admission batch is
	// consulted first so an equal projection later in this generation shares
	// the first solved body; the flat Solver cache supplies earlier completed
	// generations. Neither index refers to a parent or walks a chain.
	record, found := transaction.bodyPending.lookup(projection, transaction, action.equation, input, source)
	if !found {
		record, found = transaction.solver.bodyReuse.lookup(projection, transaction, action.equation, input, source)
	}
	if !found {
		return fiber.Guarded{}, false, true
	}
	if !transaction.fibers.Valid(record.result) || !transaction.fibers.Unconditional(record.result) {
		return fiber.Guarded{}, false, true
	}
	next, ok := transaction.fibers.Overlay(source, record.result, projection.outputs)
	return next, ok, ok
}

func (reuse *bodyReuse) lookup(projection bodyProjection, transaction *transaction, equation dependency.Equation, coordinate coordinate.Coordinate, source fiber.Guarded) (bodySpecialization, bool) {
	if reuse == nil || !projection.available() {
		return bodySpecialization{}, false
	}
	index := reuse.byProjection[projection.key]
	if index == nil {
		return bodySpecialization{}, false
	}
	if record, found := index.lookup(transaction, equation, coordinate, source); found {
		return record, true
	}
	return bodySpecialization{}, false
}

func (index *bodyProjectionIndex) lookup(transaction *transaction, equation dependency.Equation, coordinate coordinate.Coordinate, source fiber.Guarded) (bodySpecialization, bool) {
	if index == nil || index.root == nil || transaction == nil || transaction.fibers == nil || transaction.guards == nil || !transaction.fibers.Unconditional(source) {
		return bodySpecialization{}, false
	}
	leaf, ok := transaction.fibers.UnconditionalLeaf(source)
	if !ok {
		return bodySpecialization{}, false
	}
	return index.lookupLeaf(transaction, equation, coordinate, transaction.guards.True(), leaf)
}

func (index *bodyProjectionIndex) lookupLeaf(transaction *transaction, equation dependency.Equation, coordinate coordinate.Coordinate, condition guard.Guard, leaf fiber.Leaf) (bodySpecialization, bool) {
	if index == nil || index.root == nil {
		return bodySpecialization{}, false
	}
	node := index.root
	for node != nil && node.selector.valid() {
		sample, ok := node.selector.observe(transaction, equation, coordinate, condition, leaf)
		if !ok {
			return bodySpecialization{}, false
		}
		node = node.branches[sample]
	}
	if node == nil {
		return bodySpecialization{}, false
	}
	if node.bucket == nil {
		return bodySpecialization{}, false
	}
	for _, recordIndex := range node.bucket.records {
		if recordIndex < 0 || recordIndex >= len(index.records) {
			return bodySpecialization{}, false
		}
		record := index.records[recordIndex]
		if validateBodyProjectionReads(record.reads, transaction, equation, coordinate, condition, leaf) {
			return record, true
		}
	}
	return bodySpecialization{}, false
}

func validateBodyProjectionReads(reads []bodyProjectionRead, transaction *transaction, equation dependency.Equation, coordinate coordinate.Coordinate, condition guard.Guard, leaf fiber.Leaf) bool {
	for _, read := range reads {
		if !read.valid() || !read.validate(transaction, equation, coordinate, condition, leaf) {
			return false
		}
	}
	return true
}

func normalizedBodyProjectionReads(reads []bodyProjectionRead) ([]bodyProjectionRead, bool) {
	result := append([]bodyProjectionRead(nil), reads...)
	for _, read := range result {
		if !read.valid() {
			return nil, false
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].slot != result[right].slot {
			return result[left].slot < result[right].slot
		}
		return result[left].key < result[right].key
	})
	write := 0
	for _, read := range result {
		if write != 0 && result[write-1].sameLocation(read) {
			if result[write-1].sample() != read.sample() {
				return nil, false
			}
			continue
		}
		result[write] = read
		write++
	}
	return result[:write], true
}

// insert incrementally updates the one decision tree. It is iterative so a
// wide formal read set cannot consume the Go stack. Each internal node names
// a descriptor proven present in every record beneath it; a record that lacks
// it is not admitted to this shape cache at all.
func (index *bodyProjectionIndex) insert(recordIndex int) bool {
	if index == nil || index.root == nil || recordIndex < 0 || recordIndex >= len(index.records) {
		return false
	}
	for node := index.root; node != nil; {
		if !node.selector.valid() {
			return index.insertLeaf(node, recordIndex)
		}
		read, found := findBodyProjectionRead(index.records[recordIndex].reads, node.selector)
		if !found {
			return false
		}
		sample := read.sample()
		child := node.branches[sample]
		if child == nil {
			node.branches[sample] = bodyProjectionLeaf(recordIndex)
			return true
		}
		node = child
	}
	return false
}

func (index *bodyProjectionIndex) insertLeaf(node *bodyProjectionNode, recordIndex int) bool {
	if index == nil || node == nil || node.bucket == nil || recordIndex < 0 || recordIndex >= len(index.records) || len(node.bucket.records) == 0 {
		return false
	}
	if len(node.bucket.records) == 1 {
		return index.splitPair(node, node.bucket.records[0], recordIndex)
	}
	// This bucket is already below every proven uniform descriptor. Keeping a
	// same-sample record here is the one unavoidable exact collision case;
	// do not rediscover or repartition its old records on later admission.
	node.bucket.append(recordIndex)
	return true
}

// splitPair materializes the complete common-uniform descriptor prefix when
// the first two records meet. That persistent path is the admission witness:
// a later distinct sample stops at its owning node and installs one new leaf
// in O(R), without scanning or copying the old collision bucket.
func (index *bodyProjectionIndex) splitPair(node *bodyProjectionNode, leftIndex, rightIndex int) bool {
	if index == nil || node == nil || leftIndex < 0 || rightIndex < 0 || leftIndex >= len(index.records) || rightIndex >= len(index.records) {
		return false
	}
	left := index.records[leftIndex].reads
	right := index.records[rightIndex].reads
	uniform := make([]bodyProjectionRead, 0, len(left))
	var splitLeft, splitRight bodyProjectionRead
	split := false
	for _, candidate := range left {
		other, found := findBodyProjectionRead(right, candidate)
		if !found {
			continue
		}
		if other.sample() != candidate.sample() {
			splitLeft, splitRight, split = candidate, other, true
			break
		}
		uniform = append(uniform, candidate)
	}
	var tail *bodyProjectionNode
	if split {
		tail = &bodyProjectionNode{
			selector: splitLeft,
			branches: map[bodyProjectionSample]*bodyProjectionNode{
				splitLeft.sample():  bodyProjectionLeaf(leftIndex),
				splitRight.sample(): bodyProjectionLeaf(rightIndex),
			},
		}
	} else {
		bucket := &bodyProjectionBucket{}
		bucket.append(leftIndex)
		bucket.append(rightIndex)
		tail = &bodyProjectionNode{bucket: bucket}
	}
	for position := len(uniform) - 1; position >= 0; position-- {
		selector := uniform[position]
		tail = &bodyProjectionNode{
			selector: selector,
			branches: map[bodyProjectionSample]*bodyProjectionNode{
				selector.sample(): tail,
			},
		}
	}
	*node = *tail
	return true
}

func findBodyProjectionRead(reads []bodyProjectionRead, wanted bodyProjectionRead) (bodyProjectionRead, bool) {
	left, right := 0, len(reads)
	for left < right {
		middle := left + (right-left)/2
		read := reads[middle]
		if read.slot < wanted.slot || read.slot == wanted.slot && read.key < wanted.key {
			left = middle + 1
		} else {
			right = middle
		}
	}
	if left >= len(reads) || !reads[left].sameLocation(wanted) {
		return bodyProjectionRead{}, false
	}
	return reads[left], true
}

func (transaction *transaction) retainBodyProjection(capture *bodyProjectionCapture, result fiber.Guarded) bool {
	if transaction == nil || capture == nil || !capture.usable || transaction.fibers == nil || !capture.projection.available() || !transaction.fibers.Unconditional(result) {
		return true
	}
	if transaction.bodyPending == nil || transaction.solver == nil || transaction.solver.bodyReuse == nil {
		return false
	}
	item := bodySpecialization{reads: capture.reads, result: result}
	// This is the cacheability cut: prove the candidate against the
	// canonical index as it will stand after every already staged result,
	// before retaining a Fiber root.  A rejection preserves the exact cold
	// evaluation and cannot surface later as a terminal publication failure.
	admission, cacheable := transaction.bodyPending.admit(transaction.solver.bodyReuse, capture.projection, item)
	if !cacheable {
		// The action result remains the exact cold result.  It simply has no
		// reusable formal read shape, so it must not pin a root into terminal
		// Fiber publication.
		return true
	}
	if !transaction.fibers.Retain(result) {
		return false
	}
	return transaction.bodyPending.commit(admission)
}
