package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// StabilizedRelationView is the complete route-free result of one formal WTO
// solve. The lexical coordinate records were projected from that stabilized
// execution before Solve returned; this value retains no scheduler, route,
// application environment, provider, or materialization callback.
type StabilizedRelationView struct {
	lexicalBodies []FormalLexicalBodyCoordinates
	program       *RelationProgram
}

// LexicalBodies returns exactly one detached coordinate record per frozen
// lexical body, in the canonical RelationProgram body order.
func (v StabilizedRelationView) LexicalBodies() []FormalLexicalBodyCoordinates {
	return append([]FormalLexicalBodyCoordinates(nil), v.lexicalBodies...)
}

// FunctionalApplyInstantiations reports the number of frozen caller-side
// substitutions. Apply never creates another body equation system.
func (v StabilizedRelationView) FunctionalApplyInstantiations() int {
	if v.program == nil || v.program.formalTemplate == nil {
		return 0
	}
	count := 0
	for _, equation := range v.program.formalTemplate.equations {
		operator, present := equation.terminalOperator()
		if present && operator.apply != nil {
			count++
		}
	}
	return count
}

// FunctionalSummaryBodyWork reports the cells and equations physically owned
// by one lexical body in the sole formal equation system.
func (v StabilizedRelationView) FunctionalSummaryBodyWork(body lexicalidentity.StableLexicalBodyID) (cells int, equations int, ok bool) {
	if v.program == nil || v.program.formalRegion == nil || v.program.formalTemplate == nil {
		return 0, 0, false
	}
	variable, present := v.program.byBody[body]
	if !present || variable == 0 {
		return 0, 0, false
	}
	for _, cell := range v.program.formalRegion.cells {
		if cell.Variable == variable {
			cells++
		}
	}
	for _, equation := range v.program.formalTemplate.equations {
		if equation.Cell.cell.Variable == variable {
			equations++
		}
	}
	return cells, equations, cells > 0 && equations > 0
}

// FormalEquationCount reports the complete frozen equation inventory, not a
// caller/application route count.
func (v StabilizedRelationView) FormalEquationCount() int {
	if v.program == nil || v.program.formalTemplate == nil {
		return 0
	}
	return len(v.program.formalTemplate.equations)
}

// Solve executes the one canonical formal forest through the generic WTO
// solver, detaches the observation witnesses produced by its stabilized Apply
// evaluations, and publishes exactly one route-free record per lexical body.
func (p *RelationProgram) Solve(ctx context.Context, bodyID lexicalidentity.StableLexicalBodyID, entry state.State) (StabilizedRelationView, error) {
	if ctx == nil || p == nil || p.registry == nil {
		return StabilizedRelationView{}, fmt.Errorf("transformer: formal relation solve is unowned")
	}
	if _, present := p.byBody[bodyID]; !present {
		return StabilizedRelationView{}, fmt.Errorf("transformer: formal relation solve has no body %s", bodyID)
	}
	if err := p.RequireObservation(ObservationConsumerSummaryProjection, "formal relation evaluator"); err != nil {
		return StabilizedRelationView{}, err
	}
	execution, err := executeFormalRootRelation(ctx, p, bodyID, entry)
	if err != nil {
		return StabilizedRelationView{}, err
	}
	lexicalBodies, err := projectFormalLexicalBodies(ctx, execution)
	if err != nil {
		return StabilizedRelationView{}, err
	}
	if len(lexicalBodies) != len(p.bodies) {
		return StabilizedRelationView{}, fmt.Errorf("transformer: formal relation solve published %d lexical bodies, want %d", len(lexicalBodies), len(p.bodies))
	}
	return StabilizedRelationView{lexicalBodies: lexicalBodies, program: p}, nil
}

// ObservationContract returns the immutable, canonical demand retained by the
// tier-3 product.  The returned value has no mutable exported representation.
func (p *RelationProgram) ObservationContract() ObservationContract {
	if p == nil {
		return ObservationContract{}
	}
	return ObservationContract{
		key:       p.relationDependencyFreeze.demand.key,
		consumers: append([]ObservationConsumer(nil), p.relationDependencyFreeze.demand.consumers...),
		classes:   append([]ObservationClass(nil), p.relationDependencyFreeze.demand.classes...),
	}
}

// RequireObservation is the coverage guard used by evaluators and providers.
// It never retries with a wider freeze.
func (p *RelationProgram) RequireObservation(consumer ObservationConsumer, provider string) error {
	if p == nil || !p.relationDependencyFreeze.validFor(p) {
		return fmt.Errorf("transformer: observation coverage has no sealed dependency product")
	}
	return p.relationDependencyFreeze.evaluator.coverage.require(consumer, provider)
}

// RequireObservationClass rejects a provider that tries to read a result
// surface outside the closure declared by its consumer contract.
func (p *RelationProgram) RequireObservationClass(consumer ObservationConsumer, class ObservationClass, provider string) error {
	if p == nil || !p.relationDependencyFreeze.validFor(p) {
		return fmt.Errorf("transformer: observation coverage has no sealed dependency product")
	}
	return p.relationDependencyFreeze.evaluator.coverage.requireClass(consumer, class, provider)
}
