package transformer

import "fmt"

// formalScopedGuardKey is exact immutable syntax identity. The cache is owned
// by one formalTupleAlgebra run because decisionRef belongs to its run-local
// ROBDD kernel; no reference can cross solves.
type formalScopedGuardKey struct {
	variable relationVar
	scope    loopMuTerm
	arena    *Arena
	guard    Guard
}

// decisionForGuard is the sole Arena-Guard -> formal-ROBDD compilation seam.
// WTO revisits, Choice siblings, and Apply leaves reuse the same O(1) lookup;
// the immutable vocabulary and sealed Arena never acquire runtime state.
func (a *formalTupleAlgebra) decisionForGuard(variable relationVar, scope loopMuTerm, arena *Arena, guard Guard) (decisionRef, error) {
	if a == nil || a.ctx == nil || a.program == nil || a.program.formalGuards == nil ||
		variable == 0 || int(variable) > len(a.program.bodies) || arena == nil || guard == 0 {
		return 0, fmt.Errorf("transformer: formal scoped guard is unowned")
	}
	key := formalScopedGuardKey{variable: variable, scope: scope, arena: arena, guard: guard}
	if decision, ok := a.guards[key]; ok {
		return decision, nil
	}
	decision, err := a.program.formalGuards.decision(a.ctx, &a.decisions, variable, scope, arena, guard)
	if err != nil {
		return 0, err
	}
	a.guards[key] = decision
	return decision, nil
}
