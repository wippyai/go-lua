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

// RetainedRelationSolve is the test-only form of a completed relation solve.
// It keeps the otherwise run-local execution capability only until the caller
// has observed the corresponding publication boundary. Production callers use
// Solve and therefore never retain this value.
type RetainedRelationSolve struct {
	view      StabilizedRelationView
	execution *formalRelationExecution
}

// RelationSolveExecution is an opaque, solve-owned capability supplied only
// to a retained-solve observer. It deliberately exposes no alternate solver
// entry point.
type RelationSolveExecution struct{ execution *formalRelationExecution }

// View returns the detached lexical publication produced by the retained
// solve. It has the same ownership and copy rules as Solve's return value.
func (s RetainedRelationSolve) View() StabilizedRelationView { return s.view }

// Program returns the frozen relation program that owned this solve.
func (s RetainedRelationSolve) Program() *RelationProgram { return s.view.program }

// withExecution is deliberately package-private: a retained execution is a
// test observation capability, not a second production evaluation API.
func (s RetainedRelationSolve) withExecution(visit func(*formalRelationExecution) error) error {
	if s.execution == nil || visit == nil {
		return fmt.Errorf("transformer: retained relation solve is unowned")
	}
	return visit(s.execution)
}

// Observe invokes visit while the formal execution still belongs to this
// publication transaction. It is intended exclusively for differential test
// harnesses; production code must not call SolveRetained.
func (s RetainedRelationSolve) Observe(visit func(*RelationProgram, RelationSolveExecution) error) error {
	if visit == nil {
		return fmt.Errorf("transformer: retained relation observer is nil")
	}
	return s.withExecution(func(execution *formalRelationExecution) error {
		return visit(s.view.program, RelationSolveExecution{execution: execution})
	})
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
	retained, err := p.solveRetained(ctx, bodyID, entry)
	if err != nil {
		return StabilizedRelationView{}, err
	}
	// Do not let the ordinary production path retain the solve-owned tuple
	// arenas beyond this return boundary.
	return retained.view, nil
}

// SolveRetained is a test-only retention seam. Callers must consume it at the
// publication boundary and must not retain it after their observation
// callback returns. The ordinary production path intentionally uses Solve.
func (p *RelationProgram) SolveRetained(ctx context.Context, bodyID lexicalidentity.StableLexicalBodyID, entry state.State) (RetainedRelationSolve, error) {
	return p.solveRetained(ctx, bodyID, entry)
}

func (p *RelationProgram) solveRetained(ctx context.Context, bodyID lexicalidentity.StableLexicalBodyID, entry state.State) (RetainedRelationSolve, error) {
	if ctx == nil || p == nil || p.registry == nil {
		return RetainedRelationSolve{}, fmt.Errorf("transformer: formal relation solve is unowned")
	}
	if _, present := p.byBody[bodyID]; !present {
		return RetainedRelationSolve{}, fmt.Errorf("transformer: formal relation solve has no body %s", bodyID)
	}
	if err := p.RequireObservation(ObservationConsumerSummaryProjection, "formal relation evaluator"); err != nil {
		return RetainedRelationSolve{}, err
	}
	execution, err := executeFormalRootRelation(ctx, p, bodyID, entry)
	if err != nil {
		return RetainedRelationSolve{}, err
	}
	lexicalBodies, err := projectFormalLexicalBodies(ctx, execution)
	if err != nil {
		return RetainedRelationSolve{}, err
	}
	if len(lexicalBodies) != len(p.bodies) {
		return RetainedRelationSolve{}, fmt.Errorf("transformer: formal relation solve published %d lexical bodies, want %d", len(lexicalBodies), len(p.bodies))
	}
	return RetainedRelationSolve{
		view:      StabilizedRelationView{lexicalBodies: lexicalBodies, program: p},
		execution: execution,
	}, nil
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
