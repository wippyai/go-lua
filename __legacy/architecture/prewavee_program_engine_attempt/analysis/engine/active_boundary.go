package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

// activationSource is the exact active selector: one Rule semantic identity
// and its current caller occurrence.  It is scheduler-private provenance, not
// a Factor value, source graph, or artifact row.
type activationSource struct {
	rule   *ruleIdentity
	caller coordinate.Coordinate
}

// activeRelation is one resolved n-ary Link contribution.  Its target Rule,
// selecting source, primary Candidate, ordered operands, and result are all
// explicit.  The Rule pointer is an in-process capability only; canonical
// equality/order uses the sealed Program/Link/Rule identities.
type activeRelation struct {
	target    *ruleIdentity
	source    activationSource
	candidate link.Candidate
	inputs    []termOrigin
	output    termOrigin
}

// relationFrame is transaction-owned resolver storage. It never leaves the
// engine. Its epoch is the sole authority generation for its public Relation
// value capabilities.
type relationFrame struct {
	solver      *Solver
	transaction *transaction
	target      *ruleIdentity
	source      activationSource
	candidate   link.Candidate
	epoch       uint64
	live        bool
	bound       bool
	result      activeRelation
	inputs      []termOrigin
	termMark    int
	depth       int
	inline      [4]termOrigin
}

// Relation is a callback-scoped, generation-stamped resolver capability. It
// is deliberately a value: a resolver retained by a Rule keeps its old epoch,
// so reusing its private frame for a later activation cannot revive it.
type Relation struct {
	frame *relationFrame
	epoch uint64
	_     [0]func()
}

// RelationRef is an opaque, non-comparable term-origin capability.  It has no
// public coordinate, endpoint/context enum, or wrapper term vocabulary.
type RelationRef struct {
	owner  *relationFrame
	origin termOrigin
	epoch  uint64
	_      [0]func()
}

// Activate resolves exactly one selected Candidate into exactly one target
// relation Rule.  It does not scan templates, broadcast to peers, retain a
// closure, or infer a direction from lexical bodies.
func Activate[OK ~uint64, OV any, TK ~uint64, TV any](access Access[OK, OV], target *Rule[TK, TV], candidate link.Candidate, resolve func(Relation) bool) bool {
	if !access.valid() || access.output.solver == nil || target == nil || target.solver != access.output.solver || target.base == nil || target.identity == nil || target.base.anchor.form != ruleRelation || candidate == (link.Candidate{}) || resolve == nil {
		return false
	}
	execution := access.frame
	if execution.transaction == nil || execution.transaction.solver != access.output.solver || execution.transaction.guards == nil || !execution.output.Valid() || !execution.region.Valid() || execution.region.Manager() != execution.transaction.guards {
		return false
	}
	return execution.transaction.activateRelation(activationSource{rule: access.identity, caller: execution.output}, target.identity, candidate, execution.region, resolve)
}

func (transaction *transaction) activateRelation(source activationSource, target *ruleIdentity, candidate link.Candidate, when support.Mask, resolve func(Relation) bool) bool {
	if transaction == nil || transaction.canceled() || transaction.solver == nil || transaction.guards == nil || target == nil || target.anchor.form != ruleRelation || source.rule == nil || !source.caller.Valid() || candidate == (link.Candidate{}) || !when.Valid() || when.Manager() != transaction.guards || resolve == nil || !transaction.acceptsActivationSource(source) {
		return false
	}
	if !transaction.solver.link.CandidateCorresponds(candidate, target.anchor.application) {
		return false
	}
	_, callerShard, callerTerm, ok := transaction.solver.coordinate.Semantic(source.caller)
	if !ok {
		return false
	}
	applicationShard, applicationTerm, ok := transaction.solver.link.ApplicationOccurrence(target.anchor.application)
	if !ok || callerShard != applicationShard || callerTerm != applicationTerm {
		return false
	}
	resolver, ok := transaction.openRelation(source, target, candidate)
	if !ok {
		return false
	}
	accepted := resolve(resolver)
	stopped := transaction.canceled()
	// Read the accepted contribution before revoking the callback capability.
	// Escaped Relation/RelationRef values retain only a stale generation; the
	// frame loses every authority-bearing reference before the resolver exits.
	result, bound := resolver.frame.result, resolver.frame.bound
	transaction.closeRelation(resolver)
	if stopped || !accepted || !bound || !transaction.solver.validActiveRelation(result) {
		return false
	}
	// Ensure the caller provenance used by Relation.Caller is exactly the
	// selector's current coordinate, even for a root caller.
	if result.source != source || result.source.caller != source.caller || result.candidate != candidate {
		return false
	}
	return transaction.recordRelation(result, when)
}

// openRelation borrows callback scratch from this transaction.  A Relation is
// never a durable evaluator object: every field carrying authority is cleared
// by closeRelation before the resolver callback returns. The common
// capability and its small ordered tuple use transaction/Relation-embedded
// storage; rare overflow remains transaction-owned rather than allocating one
// resolver for every activation.
func (transaction *transaction) openRelation(source activationSource, target *ruleIdentity, candidate link.Candidate) (Relation, bool) {
	if transaction == nil || target == nil || target.anchor.inputArity <= 0 || transaction.relationDepth < 0 {
		return Relation{}, false
	}
	// This is a capability identity, not a cyclic generation counter.  An
	// exhausted transaction must fail closed: wrapping could make a retained
	// Relation or RelationRef match a reused frame again.
	epoch := transaction.nextRelationEpoch()
	if epoch == 0 {
		return Relation{}, false
	}
	depth := transaction.relationDepth
	var frame *relationFrame
	if depth < len(transaction.relationFrames) {
		frame = &transaction.relationFrames[depth]
	} else {
		overflow := depth - len(transaction.relationFrames)
		if overflow >= len(transaction.relationOverflow) {
			transaction.relationOverflow = append(transaction.relationOverflow, &relationFrame{})
		}
		frame = transaction.relationOverflow[overflow]
	}
	if frame == nil || frame.live {
		return Relation{}, false
	}
	*frame = relationFrame{
		solver: transaction.solver, transaction: transaction, target: target,
		source: source, candidate: candidate, epoch: epoch,
		live: true, termMark: transaction.relationTermTop, depth: depth,
	}
	if target.anchor.inputArity <= len(frame.inline) {
		frame.inputs = frame.inline[:target.anchor.inputArity]
	} else {
		end := transaction.relationTermTop + target.anchor.inputArity
		if end > cap(transaction.relationTerms) {
			terms := make([]termOrigin, end)
			copy(terms, transaction.relationTerms)
			transaction.relationTerms = terms
		} else {
			transaction.relationTerms = transaction.relationTerms[:end]
		}
		clear(transaction.relationTerms[transaction.relationTermTop:end])
		frame.inputs = transaction.relationTerms[transaction.relationTermTop:end]
		transaction.relationTermTop = end
	}
	transaction.relationDepth++
	return Relation{frame: frame, epoch: frame.epoch}, true
}

func (transaction *transaction) closeRelation(relation Relation) {
	frame := relation.frame
	if transaction == nil || frame == nil || frame.transaction != transaction || !relation.valid() || frame.depth < 0 || frame.depth != transaction.relationDepth-1 {
		if frame != nil {
			frame.expire()
		}
		return
	}
	if frame.termMark < 0 || frame.termMark > transaction.relationTermTop {
		frame.expire()
		return
	}
	transaction.relationTermTop = frame.termMark
	transaction.relationDepth--
	frame.expire()
}

// expire revokes a resolver frame synchronously at the callback cut. A later
// activation may reuse the frame, but every retained Relation/RelationRef has
// the old epoch and therefore remains invalid.
func (frame *relationFrame) expire() {
	if frame == nil {
		return
	}
	frame.solver = nil
	frame.transaction = nil
	frame.target = nil
	frame.source = activationSource{}
	frame.candidate = link.Candidate{}
	frame.live = false
	frame.bound = false
	frame.result = activeRelation{}
	frame.inputs = nil
	frame.termMark = 0
	frame.depth = 0
}

func (relation Relation) valid() bool {
	return relation.frame != nil && relation.epoch != 0 && relation.frame.live && relation.frame.epoch == relation.epoch
}

func (relation Relation) Candidate() (link.Candidate, bool) {
	if !relation.valid() || relation.frame.candidate == (link.Candidate{}) {
		return link.Candidate{}, false
	}
	return relation.frame.candidate, true
}

func (relation Relation) Application() (link.Application, bool) {
	if !relation.valid() || relation.frame.target == nil || relation.frame.target.anchor.form != ruleRelation {
		return link.Application{}, false
	}
	return relation.frame.target.anchor.application, true
}

// Caller retains the selected Rule's exact current caller provenance and
// validates that term belongs to that same existing Program activation.
func (relation Relation) Caller(term program.Term) (RelationRef, bool) {
	if !relation.valid() || relation.frame.solver == nil || term == 0 {
		return RelationRef{}, false
	}
	candidate, shard, _, ok := relation.frame.solver.coordinate.Semantic(relation.frame.source.caller)
	if !ok {
		return RelationRef{}, false
	}
	return relation.reference(termOrigin{candidate: candidate, shard: shard}, term)
}

// Root creates a zero-Candidate reference in an existing shard Entry
// activation.  It does not seed that entry; seeding is derived solely from
// explicitly registered root Queries.
func (relation Relation) Root(shard link.Shard, term program.Term) (RelationRef, bool) {
	if !relation.valid() || relation.frame.solver == nil || !relation.frame.solver.validEntryAnchor(shard, term) {
		return RelationRef{}, false
	}
	return relation.newReference(termOrigin{shard: shard, term: term})
}

// Selected retains the primary Candidate's existing body provenance.  Seed
// candidates fail closed here: they have no Program body to reconstruct.
func (relation Relation) Selected(term program.Term) (RelationRef, bool) {
	if !relation.valid() || relation.frame.solver == nil || !relation.frame.solver.validCandidateAnchor(relation.frame.candidate, relation.candidateShard(), term) {
		return RelationRef{}, false
	}
	shard, _, ok := relation.frame.solver.link.CandidateBody(relation.frame.candidate)
	if !ok {
		return RelationRef{}, false
	}
	return relation.newReference(termOrigin{candidate: relation.frame.candidate, shard: shard, term: term})
}

// Body creates a reference to an explicitly supplied body Candidate.  It is
// useful when a relation combines two existing activations; it never assumes
// that the primary Candidate is a body candidate.
func (relation Relation) Body(candidate link.Candidate, term program.Term) (RelationRef, bool) {
	if !relation.valid() || relation.frame.solver == nil || candidate == (link.Candidate{}) {
		return RelationRef{}, false
	}
	shard, _, ok := relation.frame.solver.link.CandidateBody(candidate)
	if !ok || !relation.frame.solver.validCandidateAnchor(candidate, shard, term) {
		return RelationRef{}, false
	}
	return relation.newReference(termOrigin{candidate: candidate, shard: shard, term: term})
}

// Within borrows an earlier validated provenance and changes only its term in
// the same existing Program activation.  A foreign or expired reference is
// rejected rather than becoming an ambient coordinate capability.
func (relation Relation) Within(ref RelationRef, term program.Term) (RelationRef, bool) {
	if !relation.valid() || ref.owner != relation.frame || ref.epoch == 0 || ref.epoch != relation.epoch || term == 0 {
		return RelationRef{}, false
	}
	return relation.reference(ref.origin, term)
}

// Bind sets this Relation's sole ordered n-ary contribution.  It is one-shot;
// duplicate operands remain duplicate ordered inputs because Facts.Product
// preserves exact tuple positions.
func (relation Relation) Bind(output RelationRef, inputs ...RelationRef) bool {
	if !relation.valid() || relation.frame.bound || relation.frame.target == nil || relation.frame.target.anchor.form != ruleRelation || output.owner != relation.frame || output.epoch == 0 || output.epoch != relation.epoch || len(inputs) != relation.frame.target.anchor.inputArity {
		return false
	}
	if len(relation.frame.inputs) != len(inputs) {
		return false
	}
	for index, input := range inputs {
		if input.owner != relation.frame || input.epoch == 0 || input.epoch != relation.epoch {
			return false
		}
		relation.frame.inputs[index] = input.origin
	}
	relation.frame.result = activeRelation{
		target:    relation.frame.target,
		source:    relation.frame.source,
		candidate: relation.frame.candidate,
		inputs:    relation.frame.inputs,
		output:    output.origin,
	}
	relation.frame.bound = true
	return true
}

func (relation Relation) reference(base termOrigin, term program.Term) (RelationRef, bool) {
	if !relation.valid() || term == 0 {
		return RelationRef{}, false
	}
	base.term = term
	return relation.newReference(base)
}

func (relation Relation) newReference(origin termOrigin) (RelationRef, bool) {
	if !relation.valid() || relation.frame.solver == nil || !relation.frame.solver.validTermOrigin(origin) {
		return RelationRef{}, false
	}
	return RelationRef{owner: relation.frame, origin: origin, epoch: relation.epoch}, true
}

func (relation Relation) candidateShard() link.Shard {
	if !relation.valid() || relation.frame.solver == nil || relation.frame.candidate == (link.Candidate{}) {
		return 0
	}
	shard, _, ok := relation.frame.solver.link.CandidateBody(relation.frame.candidate)
	if !ok {
		return 0
	}
	return shard
}

func (solver *Solver) validTermOrigin(origin termOrigin) bool {
	if solver == nil || solver.link == nil || origin.shard == 0 || origin.term == 0 {
		return false
	}
	if origin.candidate == (link.Candidate{}) {
		return solver.validEntryAnchor(origin.shard, origin.term)
	}
	return solver.validCandidateAnchor(origin.candidate, origin.shard, origin.term)
}

func (solver *Solver) validActiveRelation(relation activeRelation) bool {
	if solver == nil || relation.target == nil || !relation.target.sealed || relation.target.anchor.form != ruleRelation || relation.source.rule == nil || !relation.source.rule.sealed || !relation.source.caller.Valid() || relation.candidate == (link.Candidate{}) || len(relation.inputs) != relation.target.anchor.inputArity || !solver.validTermOrigin(relation.output) {
		return false
	}
	if !solver.link.CandidateCorresponds(relation.candidate, relation.target.anchor.application) {
		return false
	}
	for _, input := range relation.inputs {
		if !solver.validTermOrigin(input) {
			return false
		}
	}
	return true
}
