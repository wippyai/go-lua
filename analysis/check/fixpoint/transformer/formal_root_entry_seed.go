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
