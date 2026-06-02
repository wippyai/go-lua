// inference.go implements function refinement inference from flow analysis results.
//
// Function refinements describe how a function's return value relates to its parameters.
// For predicate functions (returning boolean), refinements capture when parameters are
// narrowed based on the return value (OnTrue/OnFalse). For assert-style functions,
// refinements capture constraints that hold after the function returns (OnReturn).
//
// Refinements enable interprocedural narrowing: calling a predicate function in a
// conditional allows the checker to narrow argument types in the appropriate branch.
package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// ParamInfo describes a function parameter for effect inference.
//
// Symbol is the canonical identity. Name is retained only for legacy constraints
// that were emitted before a symbol was attached to the path.
type ParamInfo struct {
	Name   string
	Symbol cfg.SymbolID
	Type   typ.Type
}

// InferFunctionRefinement computes a FunctionRefinement from solved flow analysis.
//
// This is the post-flow variant that uses the complete flow solution to determine
// which constraints hold at each return point. It examines return points to compute:
//
//   - OnTrue: Constraints that hold when returning true (boolean predicates)
//   - OnFalse: Constraints that hold when returning false (boolean predicates)
//   - OnReturn: Constraints that hold on normal return (assert-style functions)
//   - Terminates: Whether the function never returns normally
//
// For boolean-returning functions, OnTrue/OnFalse are populated based on return
// statement analysis. Return statements with literal true/false are classified via
// ReturnKinds. Return statements with predicate expressions use ReturnConstraints.
//
// For non-boolean functions, OnReturn is populated from path conditions at return points.
//
// All constraints are converted to use placeholders ($0, $1, ...) for parameters,
// enabling the effect to be instantiated with actual arguments at call sites.
//
// Example: For function `function is_string(x) return type(x) == "string" end`:
//   - OnTrue: HasType($0, string)
//   - OnFalse: NotHasType($0, string)
func InferFunctionRefinement(
	solution *Solution,
	g *cfg.CFG,
	params []ParamInfo,
	returnType typ.Type,
) *constraint.FunctionRefinement {
	if solution == nil || g == nil {
		return nil
	}

	src := refinementSource{conditionAt: solution.ConditionAt}
	if solution.inputs != nil {
		src.returnConstraints = solution.inputs.ReturnConstraints
		src.returnKinds = solution.inputs.ReturnKinds
	}

	return inferFunctionRefinementCore(src, g, params, returnType)
}

// InferFunctionRefinementFromInputs computes a FunctionRefinement without running full flow analysis.
//
// This is the pre-flow variant that uses only the extracted return constraints without
// propagating conditions through the CFG. It produces conservative effects based on:
//
//   - ReturnConstraints: Direct constraint extraction from return expressions
//   - CFG structure: Detecting terminating functions (no return/exit nodes)
//
// The pre-flow variant is faster but less precise than post-flow inference because
// it cannot account for path conditions from prior conditionals. It's suitable for
// bootstrapping refinement extraction before the full type checking pass.
//
// Example: For function `function assert_string(x) assert(type(x) == "string") end`:
//   - OnReturn: HasType($0, string) (from assert expression)
//   - Terminates: false (has implicit return via exit)
func InferFunctionRefinementFromInputs(
	inputs *Inputs,
	g *cfg.CFG,
	params []ParamInfo,
	returnType typ.Type,
) *constraint.FunctionRefinement {
	if inputs == nil || g == nil {
		return nil
	}

	src := refinementSource{returnConstraints: inputs.ReturnConstraints}

	return inferFunctionRefinementCore(src, g, params, returnType)
}

// ParameterCondition keeps only parameter-dependent constraints from cond and
// rewrites those paths to parameter placeholders ($0, $1, ...). This is the
// canonical flow-level representation for exporting path-sensitive facts
// through contracts or function products.
func ParameterCondition(cond constraint.Condition, params []ParamInfo) constraint.Condition {
	projection := newParameterProjection(params)
	return substituteToPlaceholdersCondition(filterParamCondition(cond, projection), projection)
}

// refinementSource abstracts the data sources for refinement inference.
//
// This struct allows the same inference algorithm to work with both pre-flow
// and post-flow data by providing optional access to different data sources:
//
//   - returnConstraints: Always available, contains constraints extracted from
//     return expressions (e.g., return type(x) == "string" yields HasType(x, string))
//
//   - returnKinds: Post-flow only, classifies return statements as ReturnTrue,
//     ReturnFalse, or ReturnUnknown based on constant analysis
//
//   - conditionAt: Post-flow only, returns the full DNF condition at a CFG point
//     after propagation
//
// Nil fields indicate the data is unavailable, causing the inference to skip
// the corresponding analysis step.
type refinementSource struct {
	returnConstraints map[cfg.Point]ReturnExprConstraints
	returnKinds       map[cfg.Point]ReturnKind
	conditionAt       func(cfg.Point) constraint.Condition
}

// inferFunctionRefinementCore is the shared implementation for effect inference.
//
// This method walks the CFG to find all return and exit points, collecting
// constraints that hold at each. The algorithm:
//
//  1. Build a parameter projection for filtering and placeholder substitution
//  2. Iterate over CFG points in RPO order, processing NodeReturn and NodeExit
//  3. For each return point, combine base condition with return expression constraints
//  4. Accumulate conditions into OnTrue/OnFalse/OnReturn based on return kind
//  5. Filter to keep only parameter-referencing constraints
//  6. Substitute parameter names/symbols with placeholders ($0, $1, ...)
//  7. Detect terminating functions (no return nodes, exit unreachable)
//
// The refinementSource parameter routes queries to the appropriate data source
// (pre-flow inputs vs. post-flow solution).
func inferFunctionRefinementCore(
	src refinementSource,
	g *cfg.CFG,
	params []ParamInfo,
	returnType typ.Type,
) *constraint.FunctionRefinement {
	projection := newParameterProjection(params)

	// Count return nodes to detect terminating functions
	hasReturnNode := false

	// Collect conditions at return points
	var onTrueCond constraint.Condition
	var onFalseCond constraint.Condition
	var onReturnCond constraint.Condition
	var exitCond constraint.Condition

	returnsBool := isBooleanReturnType(returnType)

	for _, p := range g.RPO() {
		node := g.Node(p)
		if node == nil {
			continue
		}

		// Consider both return and exit nodes for constraint collection
		if node.Kind != cfg.NodeReturn && node.Kind != cfg.NodeExit {
			continue
		}

		if node.Kind == cfg.NodeReturn {
			hasReturnNode = true
		}

		baseCond := constraint.TrueCondition()
		if src.conditionAt != nil {
			baseCond = src.conditionAt(p)
		}

		// Check for return expression constraints from predicate/assert calls
		if src.returnConstraints != nil {
			if rc, ok := src.returnConstraints[p]; ok {
				if rc.OnReturn.HasConstraints() {
					cond := constraint.And(baseCond, rc.OnReturn)
					onReturnCond = orCondition(onReturnCond, cond)
				}
				if rc.OnTrue.HasConstraints() {
					cond := constraint.And(baseCond, rc.OnTrue)
					onTrueCond = orCondition(onTrueCond, cond)
				}
				if rc.OnFalse.HasConstraints() {
					cond := constraint.And(baseCond, rc.OnFalse)
					onFalseCond = orCondition(onFalseCond, cond)
				}
				continue
			}
		}

		// For explicit boolean returns, check for literal true/false return statements
		if returnsBool && src.returnKinds != nil {
			switch src.returnKinds[p] {
			case ReturnTrue:
				onTrueCond = orCondition(onTrueCond, baseCond)
				continue
			case ReturnFalse:
				onFalseCond = orCondition(onFalseCond, baseCond)
				continue
			}
		}

		// For non-boolean or unclassified returns at NodeReturn (not NodeExit),
		// collect baseCond only if no return expression constraint was found.
		// Exit nodes don't represent actual return statements with expressions.
		if node.Kind == cfg.NodeReturn && src.conditionAt != nil {
			onReturnCond = orCondition(onReturnCond, baseCond)
		}

		// Collect exit node conditions for implicit returns
		if node.Kind == cfg.NodeExit && src.conditionAt != nil && baseCond.HasConstraints() {
			exitCond = orCondition(exitCond, baseCond)
		}
	}

	// For non-boolean functions without explicit return nodes,
	// merge exit conditions into onReturnCond (implicit fallthrough return)
	if !returnsBool && !hasReturnNode && !exitCond.IsFalse() {
		onReturnCond = orCondition(onReturnCond, exitCond)
	}

	if !onTrueCond.IsFalse() {
		onTrueCond = substituteToPlaceholdersCondition(filterParamCondition(onTrueCond, projection), projection)
	}
	if !onFalseCond.IsFalse() {
		onFalseCond = substituteToPlaceholdersCondition(filterParamCondition(onFalseCond, projection), projection)
	}
	if !onReturnCond.IsFalse() {
		onReturnCond = substituteToPlaceholdersCondition(filterParamCondition(onReturnCond, projection), projection)
	}

	exitHasPredecessors := len(graphPredecessors(g, g.Exit())) > 0
	terminates := !hasReturnNode && !exitHasPredecessors

	eff := &constraint.FunctionRefinement{
		OnTrue:     onTrueCond,
		OnFalse:    onFalseCond,
		OnReturn:   onReturnCond,
		Terminates: terminates,
	}
	if eff.IsEmpty() {
		return nil
	}
	return eff
}

type parameterProjection struct {
	bySymbol     map[cfg.SymbolID]int
	legacyByRoot map[string]int
}

func newParameterProjection(params []ParamInfo) parameterProjection {
	projection := parameterProjection{
		bySymbol:     make(map[cfg.SymbolID]int),
		legacyByRoot: make(map[string]int),
	}
	for i, p := range params {
		if p.Symbol != 0 {
			projection.bySymbol[p.Symbol] = i
		}
		if p.Name != "" {
			projection.legacyByRoot[p.Name] = i
		}
	}
	return projection
}

func (p parameterProjection) slotForPath(path constraint.Path) (int, bool) {
	if path.Symbol != 0 {
		idx, ok := p.bySymbol[path.Symbol]
		return idx, ok
	}
	if path.Root == "" {
		return 0, false
	}
	idx, ok := p.legacyByRoot[path.Root]
	return idx, ok
}

func (p parameterProjection) references(path constraint.Path) bool {
	_, ok := p.slotForPath(path)
	return ok
}

func (p parameterProjection) placeholderRoot(path constraint.Path) (string, bool) {
	idx, ok := p.slotForPath(path)
	if !ok {
		return "", false
	}
	placeholder := constraint.ParamPath(idx)
	if placeholder.IsEmpty() {
		return "", false
	}
	return placeholder.Root, true
}

// isBooleanReturnType checks if the return type is boolean.
//
// A function is considered to return boolean if its return type is:
//   - The boolean primitive type
//   - A boolean literal (true or false)
//
// This determines whether the function qualifies as a predicate for
// OnTrue/OnFalse effect inference.
func isBooleanReturnType(t typ.Type) bool {
	if t == nil {
		return false
	}

	switch t.Kind() {
	case kind.Boolean:
		return true
	case kind.Literal:
		if lit, ok := t.(*typ.Literal); ok {
			return lit.Base == kind.Boolean
		}
	}

	return false
}

// filterParamConstraints returns only constraints that reference function parameters.
//
// Effects are only meaningful when they constrain parameter types. Constraints that
// reference only local variables or globals are filtered out since they cannot be
// used at call sites for argument narrowing.
//
// Uses symbol identity when present. Root-name identity is a legacy fallback only
// for unresolved paths with Symbol=0; a nonzero symbol mismatch is final.
func filterParamConstraints(set []constraint.Constraint, projection parameterProjection) []constraint.Constraint {
	if len(set) == 0 {
		return nil
	}

	var filtered []constraint.Constraint

	for _, c := range set {
		references := false
		for _, path := range c.Paths() {
			if projection.references(path) {
				references = true
				break
			}
		}
		if references {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return constraint.NewConjunction(filtered...)
}

// substituteToPlaceholders converts parameter references to placeholders ($0, $1, ...).
//
// Effects must be portable across call sites, so parameter-specific names/symbols
// are replaced with positional placeholders. The placeholder $N refers to the Nth
// parameter (0-indexed).
//
// This substitution enables effect instantiation: at a call site, $0 is replaced
// with the actual first argument's path, $1 with the second, etc.
//
// Uses symbol identity when present. Root-name identity is a legacy fallback only
// for unresolved paths with Symbol=0.
func substituteToPlaceholders(set []constraint.Constraint, projection parameterProjection) []constraint.Constraint {
	if len(set) == 0 {
		return nil
	}

	var substituted []constraint.Constraint
	for _, c := range set {
		if sub := substitutePathsInConstraint(c, projection); sub != nil {
			substituted = append(substituted, sub)
		}
	}

	if len(substituted) == 0 {
		return nil
	}

	return constraint.NewConjunction(substituted...)
}

// substitutePathsInConstraint replaces path roots with placeholders.
//
// This function implements the core substitution logic for a single constraint.
// It handles each constraint type, replacing parameter paths with placeholder roots.
// The constraint type determines which paths to substitute:
//
//   - Unary constraints (Truthy, IsNil, HasType, etc.): Substitute Path
//   - Binary path constraints (EqPath, FieldEqualsPath): Substitute both paths
//   - Field/Index constraints: Substitute Target path
//
// Returns nil if the constraint cannot be expressed in the exported refinement
// vocabulary. Exported path references must be parameter placeholders or explicit
// return paths; callee locals/globals must not leak into a caller-facing effect.
func substitutePathsInConstraint(c constraint.Constraint, projection parameterProjection) constraint.Constraint {
	return newParameterPathSubstituter(projection).constraint(c)
}

// pathWithNewRoot creates a new path with a placeholder root.
//
// The Symbol is cleared because placeholder paths ($0, $1, ...) are templates
// that will be instantiated with actual argument paths at call sites. The original
// SSA symbol is not valid outside the defining function's scope.
//
// Segments are preserved so field/index paths can reference argument members:
// $0.field narrows the first argument's field, not just the argument itself.
func pathWithNewRoot(p constraint.Path, newRoot string) constraint.Path {
	return constraint.Path{
		Root:     newRoot,
		Symbol:   0,
		Segments: p.Segments,
	}
}

// filterParamCondition filters a DNF condition to keep only parameter-referencing constraints.
//
// Applies filterParamConstraints to each disjunct. If any disjunct becomes empty
// (all constraints filtered out), the entire condition becomes True (no restriction).
// This is sound: an empty disjunct in DNF is True, and True OR anything is True.
func filterParamCondition(cond constraint.Condition, projection parameterProjection) constraint.Condition {
	if cond.IsFalse() {
		return cond
	}

	var disjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		filtered := filterParamConstraints(d, projection)
		if len(filtered) == 0 {
			return constraint.TrueCondition()
		}
		disjuncts = append(disjuncts, filtered)
	}

	if len(disjuncts) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromDisjuncts(disjuncts)
}

// substituteToPlaceholdersCondition substitutes parameter paths in a DNF condition.
//
// Applies substituteToPlaceholders to each disjunct. If any disjunct becomes empty
// (no constraints survive substitution), the entire condition becomes True.
func substituteToPlaceholdersCondition(cond constraint.Condition, projection parameterProjection) constraint.Condition {
	if cond.IsFalse() {
		return cond
	}

	var disjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		sub := substituteToPlaceholders(d, projection)
		if len(sub) == 0 {
			return constraint.TrueCondition()
		}
		disjuncts = append(disjuncts, sub)
	}

	if len(disjuncts) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromDisjuncts(disjuncts)
}

// orCondition combines two conditions with OR, handling empty/false cases.
//
// This is the accumulator pattern for building OnTrue/OnFalse/OnReturn conditions
// from multiple return points. Each return point contributes a path condition,
// and OR combines them (a value can come from any return point).
//
// If either condition is False (zero disjuncts), returns the other unchanged.
// This avoids polluting the result with unreachable return point conditions.
func orCondition(acc, next constraint.Condition) constraint.Condition {
	if acc.IsFalse() {
		return next
	}
	if next.IsFalse() {
		return acc
	}
	return constraint.Or(acc, next)
}
