package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// formalRootEntrySeed is one invocation-owned concrete root boundary after
// the canonical entry transaction has been frozen into the formal product
// vocabulary. It owns no State and is never installed into the sealed
// relation template.
type formalRootEntrySeed struct {
	program  *RelationProgram
	variable relationVar
	constant formalRelationTupleConstant
}

// formalRootEntrySubstitution is the sole invocation-owned operation over a
// frozen relation.  It deliberately owns only the already-frozen formal
// constant for one selected root: the template and its recursive equations
// remain entry-free syntax.  Keeping this as a capability, rather than an
// algebra field named after an entry tuple, makes the eventual summary path
// explicit: a stabilized relation will consume this operation after, not
// during, its fixed point.
//
// Today the executor still installs the result at the root equation before
// solving.  That compatibility bridge is intentionally narrow and is not a
// summary cache: relation completion must first make every root-reading factor
// law symbolic.  In particular, no caller may turn this into a second solve
// or a concrete fallback.
type formalRootEntrySubstitution struct {
	seed formalRootEntrySeed
}

func newFormalRootEntrySubstitution(seed formalRootEntrySeed) (formalRootEntrySubstitution, error) {
	if !seed.validFor(seed.program) {
		return formalRootEntrySubstitution{}, fmt.Errorf("transformer: formal root substitution is unowned")
	}
	return formalRootEntrySubstitution{seed: seed}, nil
}

func (s formalRootEntrySubstitution) validFor(program *RelationProgram) bool {
	return s.seed.validFor(program)
}

func (s formalRootEntrySubstitution) substitute(a *formalTupleAlgebra, root *formalRootInputTemplate) (formalRelationTuple, bool, error) {
	if a == nil || root == nil || !s.validFor(a.program) {
		return formalRelationTuple{}, false, fmt.Errorf("transformer: formal root substitution is foreign")
	}
	if s.seed.variable != root.variable {
		return formalRelationTuple{}, false, nil
	}
	tuple, err := a.instantiatePreparedConstant(s.seed.constant)
	return tuple, true, err
}

func (s formalRootEntrySeed) validFor(program *RelationProgram) bool {
	return program != nil && s.program == program && s.variable != 0 &&
		int(s.variable) <= len(program.bodies) && s.constant.valid() &&
		s.constant.forest == program.formalFibers && s.constant.variable == s.variable
}

// prepareRelationRootEntry is the sole concrete edge law for a root
// invocation. InitialStatePlan owns the entry coordinate when present;
// EntrySeedPlan then fills missing Values; reachability and domain
// normalization close the transaction. Both coordinate and formal executors
// consume this exact law.
func prepareRelationRootEntry(program *RelationProgram, body *relationProgramBody, entry state.State) (state.State, error) {
	if program == nil || program.registry == nil || body == nil || body.graph == nil ||
		!body.initialStatePlan.ValidFor(body.body, body.graph.ID(), body.graph.Size()) || !body.entrySeedPlan.Valid() {
		return state.State{}, fmt.Errorf("transformer: root entry transaction is unowned")
	}
	seed := entry
	if initial, present := body.initialStatePlan.At(state.InitialCoordinate(body.graph.Entry())); present {
		seed = initial
	}
	seed, err := body.entrySeedPlan.Apply(program.registry, seed)
	if err != nil {
		return state.State{}, fmt.Errorf("transformer: invocation root EntrySeed: %w", err)
	}
	return state.NormalizeForDomain(body.domain, state.Reachable(seed)), nil
}

// freezeFormalRootEntrySeed transposes one selected production invocation at
// the edge. The returned capability retains only registered full-product
// factors in the body's formal vocabulary.
func freezeFormalRootEntrySeed(program *RelationProgram, bodyID lexicalidentity.StableLexicalBodyID, entry state.State) (formalRootEntrySeed, error) {
	if program == nil || program.formalTemplate == nil || !program.formalTemplate.validFor(program) {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: formal root entry has no sealed equation system")
	}
	variable, present := program.byBody[bodyID]
	if !present || variable == 0 || int(variable) > len(program.bodies) {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: formal root entry has no body %s", bodyID)
	}
	body := &program.bodies[variable-1]
	prepared, err := prepareRelationRootEntry(program, body, entry)
	if err != nil {
		return formalRootEntrySeed{}, err
	}
	constant, err := freezeFormalRelationTupleConstant(program, variable, prepared)
	if err != nil {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: freeze formal root entry: %w", err)
	}
	seed := formalRootEntrySeed{program: program, variable: variable, constant: constant}
	if !seed.validFor(program) {
		return formalRootEntrySeed{}, fmt.Errorf("transformer: formal root entry is incomplete")
	}
	return seed, nil
}
