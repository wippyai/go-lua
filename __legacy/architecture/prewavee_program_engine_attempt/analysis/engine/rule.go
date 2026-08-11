package engine

import (
	"sort"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

// Rule owns one typed conclusion.  Its binding is fixed while the Solver is
// open: either one existing Program term, one existing Program edge, or one
// explicit Link relation.  There is deliberately no fourth, inferred form.
// In particular, a relation never obtains an endpoint, direction, or payload
// vocabulary from the engine.
type Rule[K ~uint64, V any] struct {
	solver   *Solver
	base     *ruleBase
	identity *ruleIdentity
	output   *Factor[K, V]
	semantic SemanticKey
	run      func(Access[K, V]) bool
	reads    []ruleRead
	writes   []uint64
}

// ruleForm is declaration-only topology.  It is neither a coordinate kind
// nor a value carried by a Factor or State.
type ruleForm uint8

const (
	ruleAt ruleForm = iota + 1
	ruleFrom
	ruleRelation
)

// ruleAnchor is the complete immutable topology selected by RuleBinding.
// inputArity is exact mathematical relation arity, not a resource limit.
type ruleAnchor struct {
	form        ruleForm
	shard       link.Shard
	term        program.Term
	edge        program.Edge
	application link.Application
	inputArity  int
}

type ruleBase struct {
	solver *Solver
	anchor ruleAnchor
	bound  bool
}

// RuleBinding exists only during DeclareRule.  Its private capability cannot
// be retained to reopen or change a sealed declaration.
type RuleBinding struct {
	base *ruleBase
	live bool
}

// At binds a local reduction at one existing Program occurrence.  A local
// rule always has exactly one input: the confluent Fiber at that occurrence.
func (binding *RuleBinding) At(shard link.Shard, term program.Term) bool {
	if !binding.claim() || !binding.base.solver.validAnchor(shard, term) {
		return false
	}
	binding.base.anchor = ruleAnchor{form: ruleAt, shard: shard, term: term, inputArity: 1}
	binding.base.bound = true
	return true
}

// From binds one existing Program edge.  Its output is edge.To and its one
// input is edge.From.  Local control flow alone preserves the whole Fiber;
// Rule code changes only its declared output Factor.
func (binding *RuleBinding) From(shard link.Shard, edge program.Edge) bool {
	if !binding.claim() || binding.base == nil || binding.base.solver == nil || !binding.base.solver.validRuleEdge(shard, edge.To(), edge) {
		return false
	}
	binding.base.anchor = ruleAnchor{form: ruleFrom, shard: shard, term: edge.To(), edge: edge, inputArity: 1}
	binding.base.bound = true
	return true
}

// Relation binds one target Rule to one existing Link Application.  The
// resolver supplied to Activate later chooses its exact Program operands; no
// Candidate or endpoint is baked into this declaration.
func (binding *RuleBinding) Relation(application link.Application, inputArity int) bool {
	if !binding.claim() || binding.base == nil || binding.base.solver == nil || inputArity <= 0 {
		return false
	}
	if _, _, ok := binding.base.solver.link.ApplicationOccurrence(application); !ok {
		return false
	}
	binding.base.anchor = ruleAnchor{form: ruleRelation, application: application, inputArity: inputArity}
	binding.base.bound = true
	return true
}

func (binding *RuleBinding) claim() bool {
	return binding != nil && binding.live && binding.base != nil && binding.base.solver != nil && !binding.base.solver.sealed && !binding.base.bound
}

// ruleRead is one O(1) declared input coordinate. Position and semantic
// identity stay explicit so identical Factors at two relation inputs remain
// two distinct semantic reads. Runtime dependency recording belongs to the
// one row-scoped Facts execution frame, never to this sealed declaration.
type ruleRead struct {
	position int
	slot     func() (int, bool)
	semantic func() SemanticKey
	exact    bool
	key      uint64
}

type ruleReadSchema struct {
	position int
	factor   SemanticKey
	exact    bool
	key      uint64
}

// ruleIdentity contains only sealed semantic identity.  Its pointer is an
// in-process capability; ordering, cache identity, and active-relation
// equality use the complete data below, never pointer or declaration order.
type ruleIdentity struct {
	semantic SemanticKey
	output   SemanticKey
	anchor   ruleAnchor
	reads    []ruleReadSchema
	writes   []uint64
	sealed   bool
}

type ruleDeclaration struct {
	base           *ruleBase
	identity       func() *ruleIdentity
	outputSemantic func() SemanticKey
	slot           func() (int, bool)
	semantic       func() SemanticKey
	reads          func() ([]ruleRead, bool)
	writes         func() ([]uint64, bool)
	apply          func(*ruleExecution) bool
}

// DeclareRule is the sole Rule declaration route.  The binding callback must
// choose exactly one form before returning.  No compatibility overload keeps
// the old boundary template API alive.
func DeclareRule[K ~uint64, V any](solver *Solver, output *Factor[K, V], semantic SemanticKey, bind func(*RuleBinding) bool, run func(Access[K, V]) bool) (*Rule[K, V], bool) {
	if solver == nil || solver.sealed || output == nil || output.solver != solver || bind == nil || run == nil {
		return nil, false
	}
	base := &ruleBase{solver: solver}
	binding := &RuleBinding{base: base, live: true}
	accepted := bind(binding)
	binding.live = false
	if !accepted || !base.bound || !solver.validRuleAnchor(base.anchor) {
		return nil, false
	}
	identity := &ruleIdentity{semantic: semantic, anchor: base.anchor}
	rule := &Rule[K, V]{solver: solver, base: base, identity: identity, output: output, semantic: semantic, run: run}
	appendRuleDeclaration(solver, rule)
	return rule, true
}

func appendRuleDeclaration[K ~uint64, V any](solver *Solver, rule *Rule[K, V]) {
	if solver == nil || rule == nil || rule.base == nil || rule.identity == nil || rule.output == nil {
		return
	}
	output := rule.output
	solver.rules = append(solver.rules, ruleDeclaration{
		base:     rule.base,
		identity: func() *ruleIdentity { return rule.identity },
		outputSemantic: func() SemanticKey {
			return output.semantic
		},
		slot: func() (int, bool) {
			return output.slot, output.slot >= 0
		},
		semantic: func() SemanticKey { return rule.semantic },
		reads: func() ([]ruleRead, bool) {
			result := append([]ruleRead(nil), rule.reads...)
			for _, read := range result {
				if read.position < 0 || read.slot == nil || read.semantic == nil {
					return nil, false
				}
				if _, ok := read.slot(); !ok {
					return nil, false
				}
			}
			return result, true
		},
		writes: func() ([]uint64, bool) {
			result := append([]uint64(nil), rule.writes...)
			for index, key := range result {
				if !output.admits(K(key)) || index != 0 && result[index-1] >= key {
					return nil, false
				}
			}
			return result, true
		},
		apply: func(execution *ruleExecution) bool {
			if execution == nil || rule.run == nil {
				return false
			}
			epoch, valid := execution.openAccess(rule.identity)
			if !valid {
				return false
			}
			access := Access[K, V]{frame: execution, epoch: epoch, identity: rule.identity, output: output}
			defer func() {
				// Retained Access values are invalidated before a sibling Rule may
				// borrow this callback frame. Carry state belongs to that frame,
				// never to a public capability copy.
				execution.closeAccess(epoch)
			}()
			accepted := rule.run(access)
			return accepted
		},
	})
}

// Read declares one exact typed Factor coordinate of a Rule input.  The
// returned opaque ReadRef is the only capability accepted by ReadAt and
// Carry, so a callback cannot probe a different position or Factor.
func Read[OK ~uint64, OV any, K ~uint64, V any](rule *Rule[OK, OV], position int, input *Factor[K, V]) (ReadRef[K, V], bool) {
	if rule == nil || rule.base == nil || rule.identity == nil || input == nil || rule.solver == nil || rule.solver.sealed || input.solver != rule.solver || input.slot < 0 || !rule.base.bound || !rule.validReadPosition(position) {
		return ReadRef[K, V]{}, false
	}
	for _, prior := range rule.reads {
		if prior.position != position || prior.slot == nil {
			continue
		}
		slot, valid := prior.slot()
		if valid && slot == input.slot {
			return ReadRef[K, V]{}, false
		}
	}
	binding := &readBinding[K, V]{owner: rule.identity, factor: input, position: position}
	rule.reads = append(rule.reads, ruleRead{
		position: position,
		slot: func() (int, bool) {
			return input.slot, input.slot >= 0
		},
		semantic: func() SemanticKey { return input.semantic },
	})
	return ReadRef[K, V]{binding: binding}, true
}

// ReadExact declares one direct-key input.  Unlike Read, it is a complete
// static dependency signature: ReadAt accepts only key, and the compiler may
// order it against an exact writer of that same Factor/key.  Several distinct
// exact keys from one Factor/input position are allowed; a dynamic Read and
// an exact Read of the same Factor/position are mutually exclusive.
func ReadExact[OK ~uint64, OV any, K ~uint64, V any](rule *Rule[OK, OV], position int, input *Factor[K, V], key K) (ReadRef[K, V], bool) {
	if rule == nil || rule.base == nil || rule.identity == nil || input == nil || rule.solver == nil || rule.solver.sealed || input.solver != rule.solver || input.slot < 0 || !input.admits(key) || !rule.base.bound || !rule.validReadPosition(position) {
		return ReadRef[K, V]{}, false
	}
	for _, prior := range rule.reads {
		if prior.position != position || prior.slot == nil {
			continue
		}
		slot, valid := prior.slot()
		if !valid || slot != input.slot {
			continue
		}
		if !prior.exact || prior.key == uint64(key) {
			return ReadRef[K, V]{}, false
		}
	}
	binding := &readBinding[K, V]{owner: rule.identity, factor: input, position: position, exact: true, key: uint64(key)}
	rule.reads = append(rule.reads, ruleRead{
		position: position,
		slot: func() (int, bool) {
			return input.slot, input.slot >= 0
		},
		semantic: func() SemanticKey { return input.semantic },
		exact:    true,
		key:      uint64(key),
	})
	return ReadRef[K, V]{binding: binding}, true
}

// WriteExact declares one direct key this Rule may Set or Join.  Declaring
// any exact output turns the Rule into a closed direct-write signature:
// Access rejects every other key.  Carry is consequently unavailable for
// such a Rule, since a whole-plane transfer cannot prove a finite key set.
// Rules without WriteExact retain the existing dynamic output contract.
func WriteExact[K ~uint64, V any](rule *Rule[K, V], key K) bool {
	if rule == nil || rule.base == nil || rule.identity == nil || rule.output == nil || rule.solver == nil || rule.solver.sealed || rule.output.solver != rule.solver || !rule.base.bound || !rule.output.admits(key) {
		return false
	}
	value := uint64(key)
	index := sort.Search(len(rule.writes), func(index int) bool { return rule.writes[index] >= value })
	if index < len(rule.writes) && rule.writes[index] == value {
		return false
	}
	rule.writes = append(rule.writes, 0)
	copy(rule.writes[index+1:], rule.writes[index:])
	rule.writes[index] = value
	return true
}

func (rule *Rule[K, V]) validReadPosition(position int) bool {
	return rule != nil && rule.base != nil && position >= 0 && position < rule.base.anchor.inputArity
}

func (solver *Solver) validRuleAnchor(anchor ruleAnchor) bool {
	if solver == nil || solver.link == nil || anchor.inputArity <= 0 {
		return false
	}
	switch anchor.form {
	case ruleAt:
		return anchor.inputArity == 1 && anchor.edge == (program.Edge{}) && solver.validAnchor(anchor.shard, anchor.term)
	case ruleFrom:
		return anchor.inputArity == 1 && solver.validRuleEdge(anchor.shard, anchor.term, anchor.edge)
	case ruleRelation:
		_, _, ok := solver.link.ApplicationOccurrence(anchor.application)
		return ok
	default:
		return false
	}
}

func (solver *Solver) validRuleEdge(shard link.Shard, at program.Term, edge program.Edge) bool {
	if solver == nil || solver.link == nil || shard == 0 {
		return false
	}
	p, ok := solver.link.Program(shard)
	if !ok || p == nil || !p.ValidEdge(edge) || edge.To() != at || edge.From() == 0 {
		return false
	}
	fromActivation, fromOK := p.Activation(edge.From())
	atActivation, atOK := p.Activation(at)
	return fromOK && atOK && fromActivation == atActivation
}

func (solver *Solver) validAnchor(shard link.Shard, term program.Term) bool {
	if solver == nil || solver.link == nil || shard == 0 || term == 0 {
		return false
	}
	p, ok := solver.link.Program(shard)
	if !ok || p == nil {
		return false
	}
	_, ok = p.Activation(term)
	return ok
}

// validEntryAnchor is root-demand policy, deliberately narrower than Rule
// declaration validity.  A relation can use an Entry as an operand without
// making it an independent seed.
func (solver *Solver) validEntryAnchor(shard link.Shard, term program.Term) bool {
	if !solver.validAnchor(shard, term) {
		return false
	}
	p, ok := solver.link.Program(shard)
	if !ok || p == nil {
		return false
	}
	activation, ok := p.Activation(term)
	entry, entryOK := p.Entry()
	return ok && entryOK && activation == entry
}

func (solver *Solver) validCandidateAnchor(candidate link.Candidate, shard link.Shard, term program.Term) bool {
	if solver == nil || solver.link == nil || candidate == (link.Candidate{}) || shard == 0 || term == 0 {
		return false
	}
	selectedShard, body, ok := solver.link.CandidateBody(candidate)
	if !ok || selectedShard != shard {
		return false
	}
	p, ok := solver.link.Program(shard)
	if !ok || p == nil {
		return false
	}
	activation, ok := p.Activation(term)
	return ok && activation == body
}

func (solver *Solver) validateRules() bool {
	if solver == nil || solver.link == nil {
		return false
	}
	identities := make([]*ruleIdentity, 0, len(solver.rules))
	for _, declaration := range solver.rules {
		if declaration.base == nil || declaration.identity == nil || declaration.outputSemantic == nil || declaration.slot == nil || declaration.reads == nil || declaration.writes == nil || declaration.apply == nil {
			return false
		}
		identity := declaration.identity()
		if identity == nil || identity.sealed || !solver.validRuleAnchor(declaration.base.anchor) {
			return false
		}
		if _, ok := declaration.slot(); !ok {
			return false
		}
		output := declaration.outputSemantic()
		if !availableSemanticKey(identity.semantic) || !availableSemanticKey(output) {
			return false
		}
		reads, ok := declaration.reads()
		if !ok {
			return false
		}
		schema := make([]ruleReadSchema, len(reads))
		for index, read := range reads {
			if read.position < 0 || read.position >= declaration.base.anchor.inputArity || read.semantic == nil {
				return false
			}
			key := read.semantic()
			if !availableSemanticKey(key) {
				return false
			}
			schema[index] = ruleReadSchema{position: read.position, factor: key, exact: read.exact, key: read.key}
		}
		sort.Slice(schema, func(left, right int) bool {
			if schema[left].position != schema[right].position {
				return schema[left].position < schema[right].position
			}
			if order := compareSemanticKey(schema[left].factor, schema[right].factor); order != 0 {
				return order < 0
			}
			if schema[left].exact != schema[right].exact {
				return !schema[left].exact
			}
			return schema[left].key < schema[right].key
		})
		for index := 1; index < len(schema); index++ {
			if schema[index-1].position == schema[index].position && compareSemanticKey(schema[index-1].factor, schema[index].factor) == 0 && (!schema[index-1].exact || !schema[index].exact || schema[index-1].key == schema[index].key) {
				return false
			}
		}
		writes, ok := declaration.writes()
		if !ok {
			return false
		}
		identity.output = output
		identity.anchor = declaration.base.anchor
		identity.reads = schema
		identity.writes = writes
		identity.sealed = true
		identities = append(identities, identity)
	}
	for left := range identities {
		for right := 0; right < left; right++ {
			if solver.compareRuleIdentity(identities[left], identities[right]) == 0 {
				return false
			}
		}
	}
	return true
}
