package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/equation"

// SourceDecision, SourceScope, SourceExpr, SourceDecisionMap, and
// SourceReindex are immutable control capabilities issued by one exact
// SourceAssembly. Their equation representations never cross the engine
// boundary, and a capability from another source owner cannot be combined.
type SourceDecision struct {
	owner *SourceAssembly
	value equation.Decision
}

type SourceScope struct {
	owner *SourceAssembly
	value equation.Scope
}

type SourceExpr struct {
	owner *SourceAssembly
	value equation.Expr
}

type SourceDecisionMap struct {
	owner *SourceAssembly
	value equation.DecisionMap
}

type SourceReindex struct {
	owner *SourceAssembly
	value equation.Reindex
}

// controlAvailable is the common lifecycle fence for immutable control
// construction. Controls may be authored while the source transaction is
// open or after it seals; boundary admission itself is pre-Seal only. No new
// capability can be minted after Assemble has claimed the source.
func controlAvailable(state *sourceAssemblyState) bool {
	return state != nil && state.composition != nil && state.composition.Sealed() && !state.failed.Load() && !state.assembled.Load()
}

// Decision issues one symbolic decision identity owned by this source.
func (assembly *SourceAssembly) Decision(semantic SemanticKey) (SourceDecision, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceDecision{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || !semantic.Available() {
		return SourceDecision{}, false
	}
	value, ok := equation.NewDecision(semantic.compositionKey())
	return SourceDecision{owner: assembly, value: value}, ok
}

// Scope seals one canonical finite decision universe. The empty argument list
// is the one canonical empty scope; duplicate or foreign decisions fail.
func (assembly *SourceAssembly) Scope(decisions ...SourceDecision) (SourceScope, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceScope{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) {
		return SourceScope{}, false
	}
	raw := make([]equation.Decision, len(decisions))
	for index, decision := range decisions {
		if decision.owner != assembly || !decision.value.Available() {
			return SourceScope{}, false
		}
		raw[index] = decision.value
	}
	value, ok := equation.NewScope(raw...)
	return SourceScope{owner: assembly, value: value}, ok
}

// TrueExpr and FalseExpr issue the two canonical Boolean terminals.
func (assembly *SourceAssembly) TrueExpr() (SourceExpr, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceExpr{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) {
		return SourceExpr{}, false
	}
	return SourceExpr{owner: assembly, value: equation.TrueExpr()}, true
}

func (assembly *SourceAssembly) FalseExpr() (SourceExpr, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceExpr{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) {
		return SourceExpr{}, false
	}
	return SourceExpr{owner: assembly, value: equation.FalseExpr()}, true
}

// DecisionExpr issues the formula represented by one source decision.
func (assembly *SourceAssembly) DecisionExpr(decision SourceDecision) (SourceExpr, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceExpr{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || decision.owner != assembly {
		return SourceExpr{}, false
	}
	value, ok := equation.DecisionExpr(decision.value)
	return SourceExpr{owner: assembly, value: value}, ok
}

func (assembly *SourceAssembly) NotExpr(value SourceExpr) (SourceExpr, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceExpr{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || value.owner != assembly {
		return SourceExpr{}, false
	}
	result, ok := equation.NotExpr(value.value)
	return SourceExpr{owner: assembly, value: result}, ok
}

func (assembly *SourceAssembly) AndExpr(left, right SourceExpr) (SourceExpr, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceExpr{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || left.owner != assembly || right.owner != assembly {
		return SourceExpr{}, false
	}
	result, ok := equation.AndExpr(left.value, right.value)
	return SourceExpr{owner: assembly, value: result}, ok
}

func (assembly *SourceAssembly) OrExpr(left, right SourceExpr) (SourceExpr, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceExpr{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || left.owner != assembly || right.owner != assembly {
		return SourceExpr{}, false
	}
	result, ok := equation.OrExpr(left.value, right.value)
	return SourceExpr{owner: assembly, value: result}, ok
}

func (assembly *SourceAssembly) ITEExpr(test, whenTrue, whenFalse SourceExpr) (SourceExpr, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceExpr{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || test.owner != assembly || whenTrue.owner != assembly || whenFalse.owner != assembly {
		return SourceExpr{}, false
	}
	result, ok := equation.ITEExpr(test.value, whenTrue.value, whenFalse.value)
	return SourceExpr{owner: assembly, value: result}, ok
}

// IdentityMap, ForgetMap, RenameMap, and SubstituteMap issue the complete
// source-decision dispositions later consumed by one simultaneous Reindex.
func (assembly *SourceAssembly) IdentityMap(source SourceDecision) (SourceDecisionMap, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceDecisionMap{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || source.owner != assembly || !source.value.Available() {
		return SourceDecisionMap{}, false
	}
	return SourceDecisionMap{owner: assembly, value: equation.Identity(source.value)}, true
}

func (assembly *SourceAssembly) ForgetMap(source SourceDecision) (SourceDecisionMap, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceDecisionMap{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || source.owner != assembly || !source.value.Available() {
		return SourceDecisionMap{}, false
	}
	return SourceDecisionMap{owner: assembly, value: equation.Forget(source.value)}, true
}

func (assembly *SourceAssembly) RenameMap(source, target SourceDecision) (SourceDecisionMap, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceDecisionMap{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || source.owner != assembly || target.owner != assembly || !source.value.Available() ||
		!target.value.Available() || source.value == target.value {
		return SourceDecisionMap{}, false
	}
	return SourceDecisionMap{owner: assembly, value: equation.Rename(source.value, target.value)}, true
}

func (assembly *SourceAssembly) SubstituteMap(source SourceDecision, expression SourceExpr) (SourceDecisionMap, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceDecisionMap{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || source.owner != assembly || expression.owner != assembly || !source.value.Available() || !expression.value.Available() {
		return SourceDecisionMap{}, false
	}
	return SourceDecisionMap{owner: assembly, value: equation.Substitute(source.value, expression.value)}, true
}

// IdentityReindex issues the one total scope-preserving identity transport.
func (assembly *SourceAssembly) IdentityReindex(scope SourceScope) (SourceReindex, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceReindex{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || scope.owner != assembly || !scope.value.Available() {
		return SourceReindex{}, false
	}
	value := equation.IdentityReindex(scope.value)
	return SourceReindex{owner: assembly, value: value}, value.Available()
}

// Reindex seals one total simultaneous source-to-target decision transport.
// Exactly one map is required for every source decision; equation performs the
// sole semantic validation against both issued scopes.
func (assembly *SourceAssembly) Reindex(source, target SourceScope, mappings ...SourceDecisionMap) (SourceReindex, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceReindex{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !controlAvailable(state) || source.owner != assembly || target.owner != assembly {
		return SourceReindex{}, false
	}
	raw := make([]equation.DecisionMap, len(mappings))
	for index, mapping := range mappings {
		if mapping.owner != assembly {
			return SourceReindex{}, false
		}
		raw[index] = mapping.value
	}
	value, ok := equation.NewReindex(source.value, target.value, raw)
	return SourceReindex{owner: assembly, value: value}, ok
}

// Site is the sole source Site admission path. Scope, formula, and presence
// disposition enter the one existing equation Batch atomically.
func (assembly *SourceAssembly) Site(source SemanticKey, scope SourceScope, init SourceExpr, present bool) (SourceSite, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceSite{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.batch.Sealed() || !source.Available() || scope.owner != assembly || init.owner != assembly {
		return SourceSite{}, false
	}
	disposition := equation.InitAbsent
	if present {
		disposition = equation.InitPresent
	}
	value, ok := state.admission.admitSite(source.compositionKey(), scope.value, init.value, disposition)
	return SourceSite{owner: assembly, value: value}, ok
}

// Boundary is the sole SourceBoundary admission path. It records one exact
// descriptor while the source Batch is open; Seal later validates and
// materializes every descriptor through equation.BoundaryInput atomically.
func (assembly *SourceAssembly) Boundary(source, target SourceSite, provenance SemanticKey, pre SourceExpr, reindex SourceReindex, post SourceExpr) (SourceBoundary, bool) {
	state := assembly.assemblyState()
	if state == nil {
		return SourceBoundary{}, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.failed.Load() || state.sealed.Load() || state.assembled.Load() || !state.batch.OwnsOpenSite(source.value) || !state.batch.OwnsOpenSite(target.value) ||
		!provenance.Available() || source.owner != assembly || target.owner != assembly || pre.owner != assembly || reindex.owner != assembly || post.owner != assembly ||
		!pre.value.Available() || !reindex.value.Available() || !post.value.Available() {
		return SourceBoundary{}, false
	}
	descriptor := &sourceBoundaryDescriptor{
		source: source.value, target: target.value, provenance: provenance.compositionKey(),
		pre: pre.value, reindex: reindex.value, post: post.value,
	}
	state.pending = append(state.pending, descriptor)
	return SourceBoundary{owner: assembly, descriptor: descriptor}, true
}
