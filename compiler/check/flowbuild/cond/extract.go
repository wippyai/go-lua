// Constraint extraction for flow analysis.
//
// IDENTITY MODEL:
// Extraction uses bindings (AST identity) for symbol resolution.
// Solver uses SSA visibility (SymbolAt) for runtime narrowing.
// No name-based resolution in extraction code.
//
// When extracting constraints from branch conditions:
// - Symbol identity comes from bindings.SymbolOf(ident)
// - Type lookup uses SymbolTypeResolver(point, symbolID)
// - Path extraction uses PathFromExprWithBindings
//
// This ensures extracted constraints have stable symbol identity
// that matches across function boundaries (including captured variables).
package cond

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	checkeffects "github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/numconst"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// newConditionExtractor returns the extractor for conditions at p.
func newConditionExtractor(fc *core.FlowContext, inputs *flow.Inputs, p cfg.Point) *ConditionExtractor {
	return &ConditionExtractor{
		P: p, SC: fc.Scopes[p], Inputs: inputs,
		Synth:            fc.Derived.Synth,
		SymResolver:      fc.Derived.SymResolver,
		TypeKeyRes:       fc.Derived.TypeKeyRes,
		ConstResolver:    predicate.BuildConstResolver(inputs, p),
		RefinementBySym:  fc.Derived.RefinementBySym,
		UnstableSymbols:  fc.Derived.CapturedReassignments,
		ReceiverRoots:    fc.Derived.ReceiverRoots,
		NilableRoots:     fc.Derived.NilableRoots,
		KnownNonNilPaths: fc.Derived.KnownNonNilPaths,
		ModuleBindings:   fc.ModuleBindings,
	}
}

// ConditionsFunc returns the conditions an expression establishes at a point
// when truthy and when falsy, as a branch on it puts them on its edges.
func ConditionsFunc(fc *core.FlowContext, inputs *flow.Inputs) api.ConditionFromExprFunc {
	return func(p cfg.Point, expr ast.Expr) (constraint.Condition, constraint.Condition) {
		if fc == nil || fc.Derived == nil || inputs == nil || expr == nil {
			return constraint.TrueCondition(), constraint.TrueCondition()
		}
		bc := newConditionExtractor(fc, inputs, p).conditionsFromEvaluatedExpr(expr)
		return bc.OnTrue, bc.OnFalse
	}
}

// ExtractEdgeConstraints extracts type constraints from branch conditions.
func ExtractEdgeConstraints(fc *core.FlowContext, inputs *flow.Inputs) {
	fc.Graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		succs := fc.Graph.Successors(p)
		if len(succs) < 2 {
			return
		}

		trueEdge, falseEdge := FindBranchEdges(fc.Graph, p, succs)
		if trueEdge == 0 && falseEdge == 0 {
			return
		}

		ce := newConditionExtractor(fc, inputs, p)
		constraints := ce.ConstraintsFromBranch(info)
		if info.Condition != nil {
			constraints = ce.conditionsFromEvaluatedExpr(info.Condition)
		}

		// For generic for loops, add NotNil and KeyOf constraints for loop variables
		if node := fc.Graph.CFG().Node(p); node != nil && len(node.LoopLocals) > 0 {
			var loopConstraints []constraint.Constraint
			for _, sym := range node.LoopLocals {
				if sym != 0 {
					root := fc.Graph.NameOf(sym)
					loopPath := path.WithVersion(constraint.Path{Root: root, Symbol: sym}, fc.Graph, p)
					loopConstraints = append(loopConstraints, constraint.NotNil{
						Path: loopPath,
					})
				}
			}

			// For keyed iterators (pairs), emit KeyOf constraint for the key variable
			if node.LoopPreheaderSet {
				if assignInfo, ok := fc.Graph.Info(node.LoopPreheader).(*cfg.AssignInfo); ok && len(assignInfo.IterExprs) > 0 {
					bindings := fc.Graph.Bindings()
					iterSource := resolve.ExtractIteratorSource(
						assignInfo.IterExprs, node.LoopPreheader,
						fc.Derived.Synth, fc.Derived.SymResolver, ce.ConstResolver, bindings,
					)
					if iterSource != nil && iterSource.Kind == flow.IterateKeyed && len(node.LoopLocals) > 0 {
						keySym := node.LoopLocals[0]
						if keySym != 0 {
							keyRoot := fc.Graph.NameOf(keySym)
							keyPath := path.WithVersion(constraint.Path{Root: keyRoot, Symbol: keySym}, fc.Graph, p)
							tablePath := constraint.Path{
								Root:     resolve.RootNameFromBindings(bindings, iterSource.Path.Symbol, iterSource.Path.Root),
								Symbol:   iterSource.Path.Symbol,
								Segments: iterSource.Path.Segments,
							}
							tablePath = path.WithVersion(tablePath, fc.Graph, p)
							loopConstraints = append(loopConstraints, constraint.KeyOf{
								Table: tablePath,
								Key:   keyPath,
							})
						}
					}

					// For indexed iterators (ipairs) over keys-provenance variables,
					// emit KeyOf constraint linking value variable to original table.
					// Pattern: local keys = sorted_keys(t); for _, k in ipairs(keys) do ... t[k] ...
					if iterSource != nil && iterSource.Kind == flow.IterateIndexed && len(node.LoopLocals) > 0 {
						iterSym := iterSource.Path.Symbol
						if iterSym != 0 && inputs.KeysProvenance != nil {
							if origTableSym, ok := inputs.KeysProvenance[iterSym]; ok && origTableSym != 0 {
								valueIndex := 1
								if len(node.LoopLocals) == 1 {
									valueIndex = 0
								}
								valueSym := node.LoopLocals[valueIndex]
								if valueSym != 0 {
									valueRoot := fc.Graph.NameOf(valueSym)
									valuePath := path.WithVersion(constraint.Path{Root: valueRoot, Symbol: valueSym}, fc.Graph, p)
									tableRoot := resolve.RootNameFromBindings(bindings, origTableSym, "")
									if tableRoot == "" {
										tableRoot = fc.Graph.NameOf(origTableSym)
									}
									tablePath := constraint.Path{
										Root:   tableRoot,
										Symbol: origTableSym,
									}
									tablePath = path.WithVersion(tablePath, fc.Graph, p)
									loopConstraints = append(loopConstraints, constraint.KeyOf{
										Table: tablePath,
										Key:   valuePath,
									})
								}
							}
						}
					}
				}
			}

			if len(loopConstraints) > 0 {
				loopCond := constraint.FromConstraints(loopConstraints...)
				if constraints.OnTrue.HasConstraints() {
					constraints.OnTrue = constraint.And(constraints.OnTrue, loopCond)
				} else {
					constraints.OnTrue = loopCond
				}
				if constraints.OnFalse.IsFalse() && !constraints.OnFalse.HasConstraints() {
					constraints.OnFalse = constraint.TrueCondition()
				}
			}
		}

		if (constraints.OnTrue.HasConstraints() || constraints.OnTrue.IsFalse()) && trueEdge != 0 {
			inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{
				From:      p,
				To:        trueEdge,
				Condition: constraints.OnTrue,
			})
		}
		if (constraints.OnFalse.HasConstraints() || constraints.OnFalse.IsFalse()) && falseEdge != 0 {
			inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{
				From:      p,
				To:        falseEdge,
				Condition: constraints.OnFalse,
			})
		}
	})
}

// ExtractNumericConstraints extracts numeric constraints from branch conditions.
func ExtractNumericConstraints(fc *core.FlowContext, inputs *flow.Inputs) {
	fc.Graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		succs := fc.Graph.Successors(p)
		if len(succs) < 2 {
			return
		}

		trueEdge, falseEdge := FindBranchEdges(fc.Graph, p, succs)
		if trueEdge == 0 && falseEdge == 0 {
			return
		}

		// Handle numeric for-loop bounds
		if info.CondCheck.Kind == cfg.CheckLimit && info.CondVar != "" {
			if forConstraints := NumericForConstraints(fc.Graph, p, info.CondVar, info.CondSymbol); len(forConstraints) > 0 {
				inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, flow.EdgeNumericConstraint{
					From:        p,
					To:          trueEdge,
					Constraints: forConstraints,
				})
			}
		}

		if info.Condition == nil {
			return
		}

		numConstraints := NumericConstraintsFromExpr(info.Condition, p, inputs)
		if len(numConstraints) == 0 {
			return
		}

		if trueEdge != 0 {
			inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, flow.EdgeNumericConstraint{
				From:        p,
				To:          trueEdge,
				Constraints: numConstraints,
			})
		}

		if falseEdge != 0 {
			var negated []constraint.NumericConstraint
			for _, nc := range numConstraints {
				if neg := numconst.NegateNumericConstraint(nc); neg != nil {
					negated = append(negated, neg)
				}
			}
			if len(negated) > 0 {
				inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, flow.EdgeNumericConstraint{
					From:        p,
					To:          falseEdge,
					Constraints: negated,
				})
			}
		}
	})
	// A normal return from a throwing assertion establishes the truth of its
	// argument. Carry numeric length facts along the same outgoing edges.
	fc.Graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
		args := runtimeCallArgs(info)
		if len(args) == 0 {
			return
		}
		eff := ExtractFunctionRefinement(info, p, fc.Derived.Synth, fc.Derived.RefinementBySym, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings)
		if eff == nil {
			return
		}
		for _, c := range eff.OnReturn.MustConstraints() {
			var numeric []constraint.NumericConstraint
			switch fact := c.(type) {
			case constraint.Truthy:
				if idx, ok := constraint.PlaceholderArgIndex(fact.Path, len(args)); ok {
					numeric = NumericConstraintsFromExpr(args[idx], p, inputs)
				}
			case constraint.EqPath:
				left, leftOK := constraint.PlaceholderArgIndex(fact.Left, len(args))
				right, rightOK := constraint.PlaceholderArgIndex(fact.Right, len(args))
				if leftOK && rightOK {
					numeric = NumericConstraintsFromExpr(&ast.RelationalOpExpr{Lhs: args[left], Rhs: args[right], Operator: "=="}, p, inputs)
				}
			}
			if len(numeric) == 0 {
				continue
			}
			for _, succ := range fc.Graph.Successors(p) {
				inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, flow.EdgeNumericConstraint{From: p, To: succ, Constraints: numeric})
			}
		}
	})
}

// numericForConstraints extracts numeric constraints from a numeric for-loop.
func NumericForConstraints(graph *cfg.Graph, branchPoint cfg.Point, varName string, varSymbol cfg.SymbolID) []constraint.NumericConstraint {
	node := graph.CFG().Node(branchPoint)
	if node == nil || !node.LoopPreheaderSet {
		return nil
	}

	preheader := node.LoopPreheader
	info, ok := graph.Info(preheader).(*cfg.AssignInfo)
	if !ok || info.NumericFor == nil {
		return nil
	}

	forInfo := info.NumericFor
	if forInfo.VarName != varName {
		return nil
	}

	initVal, initOk := numconst.IntConstFromExpr(forInfo.Init)
	if !initOk {
		return nil
	}

	root := varName
	if varSymbol != 0 {
		if name := graph.NameOf(varSymbol); name != "" {
			root = name
		}
	}
	varPath := path.WithVersion(constraint.Path{Root: root, Symbol: varSymbol}, graph, branchPoint)

	var result []constraint.NumericConstraint
	result = append(result, constraint.GeConst{X: varPath, C: initVal})

	if limitVal, limitOk := numconst.IntConstFromExpr(forInfo.Limit); limitOk {
		result = append(result, constraint.LeConst{X: varPath, C: limitVal})
	} else if arrPath, offset, ok := ExtractLenBound(forInfo.Limit, branchPoint, graph); ok {
		result = append(result, constraint.LeLenOf{X: varPath, Array: arrPath, Offset: offset})
	} else {
		return nil
	}

	return result
}

func ExtractLenPath(expr ast.Expr, p cfg.Point, graph *cfg.Graph) constraint.Path {
	lenOp, ok := expr.(*ast.UnaryLenOpExpr)
	if !ok || graph == nil {
		return constraint.Path{}
	}
	return path.FromExprWithBindingsAt(lenOp.Expr, nil, graph.Bindings(), graph, p)
}

// ExtractLenBound extracts symbolic len-path bound with an optional constant offset.
//
// Supported forms:
//   - #arr          => (arr, 0)
//   - #arr - K      => (arr, -K)
//   - #arr + K      => (arr, +K)
func ExtractLenBound(expr ast.Expr, p cfg.Point, graph *cfg.Graph) (constraint.Path, int64, bool) {
	if arrPath := ExtractLenPath(expr, p, graph); !arrPath.IsEmpty() {
		return arrPath, 0, true
	}
	op, ok := expr.(*ast.ArithmeticOpExpr)
	if !ok {
		return constraint.Path{}, 0, false
	}
	if op.Operator != "+" && op.Operator != "-" {
		return constraint.Path{}, 0, false
	}
	arrPath := ExtractLenPath(op.Lhs, p, graph)
	if arrPath.IsEmpty() {
		return constraint.Path{}, 0, false
	}
	k, ok := numconst.IntConstFromExpr(op.Rhs)
	if !ok {
		return constraint.Path{}, 0, false
	}
	if op.Operator == "-" {
		k = -k
	}
	return arrPath, k, true
}

// ExtractLenOfPath preserves legacy behavior for callers/tests that need only the path.
func ExtractLenOfPath(expr ast.Expr, p cfg.Point, graph *cfg.Graph) constraint.Path {
	arrPath, _, ok := ExtractLenBound(expr, p, graph)
	if !ok {
		return constraint.Path{}
	}
	return arrPath
}

// findBranchEdges determines which successor is the true vs false edge.
func FindBranchEdges(graph *cfg.Graph, p cfg.Point, succs []cfg.Point) (trueEdge, falseEdge cfg.Point) {
	for _, s := range succs {
		cond, ok := graph.EdgeCond(p, s)
		if !ok {
			continue
		}
		if cond {
			trueEdge = s
		} else {
			falseEdge = s
		}
	}
	return
}

// ExtractCallOnReturnConstraints extracts OnReturn constraints from function calls.
// Also marks dead points for calls to terminating functions.
func ExtractCallOnReturnConstraints(
	fc *core.FlowContext,
	inputs *flow.Inputs,
) map[EdgeKey]constraint.Condition {
	out := make(map[EdgeKey]constraint.Condition)
	if fc == nil || fc.Graph == nil || fc.Derived == nil || inputs == nil {
		return out
	}
	var rebindingCallees *CapturedRebindingFacts
	if len(fc.Derived.CapturedReassignments) != 0 {
		rebindingCallees = CapturedRebindingsByCallee(fc.Graph)
	}
	nestedAt := func(p cfg.Point, exprs []ast.Expr, assigned map[cfg.SymbolID]bool) constraint.Condition {
		sc := fc.Scopes[p]
		constResolver := predicate.BuildConstResolver(inputs, p)
		var combined constraint.Condition
		for _, expr := range exprs {
			for _, call := range evaluatedNestedCalls(expr, fc.Graph) {
				fact := ConstraintsFromCallOnReturn(call, p, sc, inputs, fc.Derived.Synth, fc.Derived.TypeKeyRes, fc.Derived.RefinementBySym, constResolver, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings)
				fact = stableNestedFacts(fact, fc.Graph, fc.Derived.CapturedReassignments, assigned)
				if fact.HasConstraints() {
					if combined.HasConstraints() {
						combined = constraint.And(combined, fact)
					} else {
						combined = fact
					}
				}
			}
		}
		return combined
	}

	for _, p := range fc.Graph.RPO() {
		if !PointHasTerminatingCallSite(fc.Graph, p, fc.Derived.Synth, fc.Derived.SymResolver, fc.Derived.RefinementBySym, fc.ModuleBindings) {
			continue
		}
		for _, succ := range fc.Graph.Successors(p) {
			preds := fc.Graph.Predecessors(succ)
			if len(preds) != 1 {
				continue
			}
			if inputs.DeadPoints == nil {
				inputs.DeadPoints = make(map[cfg.Point]bool)
			}
			inputs.DeadPoints[succ] = true
		}
	}

	fc.Graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
		sc := fc.Scopes[p]
		constResolver := predicate.BuildConstResolver(inputs, p)

		cond := ConstraintsFromCallOnReturn(info, p, sc, inputs, fc.Derived.Synth, fc.Derived.TypeKeyRes, fc.Derived.RefinementBySym, constResolver, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings)
		cond = stableCallConstraints(cond, info, fc.Derived.CapturedReassignments, rebindingCallees, fc.Graph.Bindings())
		if info != nil && info.Call != nil {
			// A normal return from the statement means its evaluated arguments
			// returned too. Their local-value facts survive the enclosing call.
			nested := nestedAt(p, []ast.Expr{info.Call}, nil)
			if nested.HasConstraints() {
				if cond.HasConstraints() {
					cond = constraint.And(cond, nested)
				} else {
					cond = nested
				}
			}
		}
		if !cond.HasConstraints() {
			return
		}
		for _, succ := range fc.Graph.Successors(p) {
			key := EdgeKey{From: p, To: succ}
			if existing, ok := out[key]; ok && existing.HasConstraints() {
				out[key] = constraint.And(existing, cond)
			} else {
				out[key] = cond
			}
		}
	})

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		sc := fc.Scopes[p]
		constResolver := predicate.BuildConstResolver(inputs, p)
		cond := ConstraintsFromAssignOnReturn(info, p, sc, inputs, fc.Derived.Synth, fc.Derived.TypeKeyRes, fc.Derived.RefinementBySym, constResolver, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings)
		for _, call := range info.SourceCalls {
			cond = stableCallConstraints(cond, call, fc.Derived.CapturedReassignments, rebindingCallees, fc.Graph.Bindings())
		}
		assigned := make(map[cfg.SymbolID]bool)
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				assigned[target.Symbol] = true
			}
		}
		if nested := nestedAt(p, info.Sources, assigned); nested.HasConstraints() {
			if cond.HasConstraints() {
				cond = constraint.And(cond, nested)
			} else {
				cond = nested
			}
		}
		if !cond.HasConstraints() {
			return
		}
		for _, succ := range fc.Graph.Successors(p) {
			key := EdgeKey{From: p, To: succ}
			if existing, ok := out[key]; ok && existing.HasConstraints() {
				out[key] = constraint.And(existing, cond)
			} else {
				out[key] = cond
			}
		}
	})

	return out
}

// A nested callee can rebind a captured local during the same call whose
// OnReturn fact mentions it. Such a fact describes the argument's old value,
// not necessarily the local's value when the call returns.
func stableCallConstraints(cond constraint.Condition, call *cfg.CallInfo, unstable map[cfg.SymbolID]bool, rebindingCallees *CapturedRebindingFacts, bindings *bind.BindingTable) constraint.Condition {
	if !cond.HasConstraints() || len(unstable) == 0 || call == nil {
		return cond
	}
	// Direct calls may be aliases of a local closure, so retain only facts
	// about locals that no nested closure can rebind.
	unsafe := unstable
	// The standard assert builtin only checks its already evaluated argument.
	// It cannot run a closure that rebinds a captured local between the check
	// and normal return. A shadowing local called assert is not the builtin.
	if call.CalleePath.Root == "assert" && len(call.CalleePath.Segments) == 0 && bindings != nil && call.CalleeSymbol != 0 {
		if k, ok := bindings.Kind(call.CalleeSymbol); ok && k == cfg.SymbolGlobal {
			unsafe = nil
		}
	}
	if len(call.CalleePath.Segments) != 0 {
		// A field call can reach a caller local through a locally stored closure
		// or through a callback argument supplied to an imported function.
		unsafe = make(map[cfg.SymbolID]bool)
		callee := call.CalleePath
		callee.Version = 0
		if rebindingCallees != nil {
			for sym := range rebindingCallees.ByPath[callee.Key()] {
				unsafe[sym] = true
			}
			for i, arg := range call.Args {
				if fn, ok := arg.(*ast.FunctionExpr); ok {
					for sym := range rebindingCallees.ByFunc[fn] {
						unsafe[sym] = true
					}
				}
				if i < len(call.ArgSymbols) {
					for sym := range rebindingCallees.BySymbol[call.ArgSymbols[i]] {
						unsafe[sym] = true
					}
				}
			}
		}
		if len(unsafe) == 0 {
			return cond
		}
	}
	var kept []constraint.Constraint
	for _, c := range cond.MustConstraints() {
		mayRebind := false
		constraint.VisitPaths(c, func(path constraint.Path) bool {
			if unsafe[path.Symbol] {
				mayRebind = true
				return true
			}
			return false
		})
		if !mayRebind {
			kept = append(kept, c)
		}
	}
	return constraint.FromConstraints(kept...)
}

// constraintsFromCallOnReturn extracts OnReturn constraints from a call.
func ConstraintsFromCallOnReturn(
	info *cfg.CallInfo,
	p cfg.Point,
	sc *scope.State,
	inputs *flow.Inputs,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	typeKeyResolver func(string, *scope.State) (narrow.TypeKey, bool),
	refinementLookupSym constraint.RefinementLookupBySym,
	constResolver func(string) *flow.ConstValue,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) constraint.Condition {
	if info == nil {
		return constraint.Condition{}
	}
	callArgs := runtimeCallArgs(info)
	if len(callArgs) == 0 {
		return immediateReturnedClosureConstraints(info, p, synthFn, graph, moduleBindings)
	}

	bindings := resolve.GetBindings(inputs)

	// TypeName(x) pattern - check metatype. Method calls such as x:TypeName()
	// are not type checks even when the method shares a type's name.
	if info.IsTypeCheck && info.Method == "" && typeKeyResolver != nil {
		if typeKey, ok := typeKeyResolver(info.TypeCheckName, sc); ok && !typeKey.IsZero() {
			if len(callArgs) > 0 {
				argPath := path.FromExprWithBindingsAt(callArgs[0], constResolver, bindings, graph, p)
				if !argPath.IsEmpty() {
					return constraint.FromConstraints(constraint.HasType{Path: argPath, Type: typeKey})
				}
			}
		}
	}

	eff := ExtractFunctionRefinement(info, p, synthFn, refinementLookupSym, symResolver, graph, moduleBindings)
	if eff == nil || !eff.OnReturn.HasConstraints() {
		return constraint.Condition{}
	}

	argPaths := make([]constraint.Path, len(callArgs))
	for i, arg := range callArgs {
		argPaths[i] = path.FromExprWithBindingsAt(arg, constResolver, bindings, graph, p)
	}

	ce := &ConditionExtractor{
		P: p, SC: sc, Inputs: inputs,
		Synth:           synthFn,
		SymResolver:     symResolver,
		TypeKeyRes:      typeKeyResolver,
		ConstResolver:   constResolver,
		RefinementBySym: refinementLookupSym,
	}

	// OnReturn summarizes all normal-return paths. At call sites we may only
	// apply constraints guaranteed across every disjunct; disjunct-local facts
	// are not branch-correlated in caller flow and cause unsound narrowing.
	templateMust := eff.OnReturn.MustConstraints()
	if len(templateMust) == 0 {
		return constraint.Condition{}
	}

	must := make([]constraint.Constraint, 0, len(templateMust))
	retTargets := callReturnTargets(info, p, graph)
	for _, c := range templateMust {
		switch v := c.(type) {
		case constraint.Falsy:
			if idx, ok := constraint.PlaceholderArgIndex(v.Path, len(callArgs)); ok && argPaths[idx].IsEmpty() {
				fallback := ce.ConditionFromExpr(callArgs[idx])
				if fallback.HasConstraints() {
					must = append(must, constraint.Not(fallback).MustConstraints()...)
				}
				continue
			}
		case constraint.Truthy:
			if idx, ok := constraint.PlaceholderArgIndex(v.Path, len(callArgs)); ok && argPaths[idx].IsEmpty() {
				fallback := ce.ConditionFromExpr(callArgs[idx])
				if fallback.HasConstraints() {
					must = append(must, fallback.MustConstraints()...)
				}
				continue
			}
		case constraint.EqPath:
			if fallback, ok := callConstraintFallbackFromArgs(ce, callArgs, argPaths, v, true); ok {
				must = append(must, fallback...)
				continue
			}
		case constraint.NotEqPath:
			if fallback, ok := callConstraintFallbackFromArgs(ce, callArgs, argPaths, v, false); ok {
				must = append(must, fallback...)
				continue
			}
		}

		sub := constraint.FromConstraints(c).Substitute(argPaths)
		for _, mc := range sub.MustConstraints() {
			must = append(must, substituteReturnConstraintPaths(mc, retTargets))
		}
	}
	must = normalizePathConstraints(must)
	if len(must) == 0 {
		return constraint.Condition{}
	}
	cond := constraint.FromConjunction(must)

	if cond.IsFalse() || !cond.HasConstraints() {
		return constraint.Condition{}
	}
	return cond
}

func runtimeCallArgs(info *cfg.CallInfo) []ast.Expr {
	if info == nil {
		return nil
	}
	n := callsite.RuntimeArgCount(info)
	if n == 0 {
		return nil
	}
	args := make([]ast.Expr, 0, n)
	for i := 0; i < n; i++ {
		if arg := callsite.RuntimeArgAt(info, i); arg != nil {
			args = append(args, arg)
		}
	}
	return args
}

func callReturnTargets(info *cfg.CallInfo, p cfg.Point, graph *cfg.Graph) map[int]constraint.Path {
	if info == nil || graph == nil {
		return nil
	}
	assign := graph.Assign(p)
	if assign == nil || len(assign.Targets) == 0 {
		return nil
	}
	out := make(map[int]constraint.Path)
	for i := range assign.Targets {
		call, retIdx := assign.CallForTarget(i)
		if call != info || retIdx < 0 {
			continue
		}
		target, ok := assign.TargetAt(i)
		if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			continue
		}
		out[retIdx] = path.WithVersion(constraint.Path{
			Root:   target.Name,
			Symbol: target.Symbol,
		}, graph, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func substituteReturnConstraintPaths(c constraint.Constraint, retTargets map[int]constraint.Path) constraint.Constraint {
	if len(retTargets) == 0 {
		return c
	}
	subPath := func(p constraint.Path) constraint.Path {
		if p.Symbol != 0 {
			return p
		}
		idx := constraint.ReturnIndexFromString(p.Root)
		if idx < 0 {
			return p
		}
		target, ok := retTargets[idx]
		if !ok || target.IsEmpty() {
			return p
		}
		out := target
		if len(p.Segments) > 0 {
			out.Segments = append(append([]constraint.Segment{}, out.Segments...), p.Segments...)
		}
		return out
	}
	return constraint.VisitConstraint(c, constraint.ConstraintVisitor[constraint.Constraint]{
		Truthy: func(v constraint.Truthy) constraint.Constraint { v.Path = subPath(v.Path); return v },
		Falsy:  func(v constraint.Falsy) constraint.Constraint { v.Path = subPath(v.Path); return v },
		IsNil:  func(v constraint.IsNil) constraint.Constraint { v.Path = subPath(v.Path); return v },
		NotNil: func(v constraint.NotNil) constraint.Constraint { v.Path = subPath(v.Path); return v },
		HasType: func(v constraint.HasType) constraint.Constraint {
			v.Path = subPath(v.Path)
			return v
		},
		NotHasType: func(v constraint.NotHasType) constraint.Constraint {
			v.Path = subPath(v.Path)
			return v
		},
		HasField: func(v constraint.HasField) constraint.Constraint {
			v.Path = subPath(v.Path)
			return v
		},
		FieldEquals: func(v constraint.FieldEquals) constraint.Constraint {
			v.Target = subPath(v.Target)
			return v
		},
		FieldNotEquals: func(v constraint.FieldNotEquals) constraint.Constraint {
			v.Target = subPath(v.Target)
			return v
		},
		IndexEquals: func(v constraint.IndexEquals) constraint.Constraint {
			v.Target = subPath(v.Target)
			return v
		},
		IndexNotEquals: func(v constraint.IndexNotEquals) constraint.Constraint {
			v.Target = subPath(v.Target)
			return v
		},
		EqPath: func(v constraint.EqPath) constraint.Constraint {
			v.Left = subPath(v.Left)
			v.Right = subPath(v.Right)
			return constraint.NewEqPath(v.Left, v.Right)
		},
		NotEqPath: func(v constraint.NotEqPath) constraint.Constraint {
			v.Left = subPath(v.Left)
			v.Right = subPath(v.Right)
			return constraint.NewNotEqPath(v.Left, v.Right)
		},
		FieldEqualsPath: func(v constraint.FieldEqualsPath) constraint.Constraint {
			v.Target = subPath(v.Target)
			v.Value = subPath(v.Value)
			return v
		},
		FieldNotEqualsPath: func(v constraint.FieldNotEqualsPath) constraint.Constraint {
			v.Target = subPath(v.Target)
			v.Value = subPath(v.Value)
			return v
		},
		IndexEqualsPath: func(v constraint.IndexEqualsPath) constraint.Constraint {
			v.Target = subPath(v.Target)
			v.Value = subPath(v.Value)
			return v
		},
		IndexNotEqualsPath: func(v constraint.IndexNotEqualsPath) constraint.Constraint {
			v.Target = subPath(v.Target)
			v.Value = subPath(v.Value)
			return v
		},
		KeyOf: func(v constraint.KeyOf) constraint.Constraint {
			v.Table = subPath(v.Table)
			v.Key = subPath(v.Key)
			return v
		},
		Default: func(constraint.Constraint) constraint.Constraint { return c },
	})
}

func normalizePathConstraints(conj []constraint.Constraint) []constraint.Constraint {
	if len(conj) == 0 {
		return conj
	}
	out := make([]constraint.Constraint, 0, len(conj))
	for _, c := range conj {
		out = append(out, normalizePathConstraint(c))
	}
	return out
}

func normalizePathConstraint(c constraint.Constraint) constraint.Constraint {
	switch v := c.(type) {
	case constraint.EqPath:
		if target, field, ok := constraint.SplitFieldPath(v.Left); ok {
			return constraint.FieldEqualsPath{Target: target, Field: field, Value: v.Right}
		}
		if target, field, ok := constraint.SplitFieldPath(v.Right); ok {
			return constraint.FieldEqualsPath{Target: target, Field: field, Value: v.Left}
		}
		if target, key, ok := path.SplitIndexPath(v.Left); ok {
			return constraint.IndexEqualsPath{Target: target, Key: key, Value: v.Right}
		}
		if target, key, ok := path.SplitIndexPath(v.Right); ok {
			return constraint.IndexEqualsPath{Target: target, Key: key, Value: v.Left}
		}
	case constraint.NotEqPath:
		if target, field, ok := constraint.SplitFieldPath(v.Left); ok {
			return constraint.FieldNotEqualsPath{Target: target, Field: field, Value: v.Right}
		}
		if target, field, ok := constraint.SplitFieldPath(v.Right); ok {
			return constraint.FieldNotEqualsPath{Target: target, Field: field, Value: v.Left}
		}
		if target, key, ok := path.SplitIndexPath(v.Left); ok {
			return constraint.IndexNotEqualsPath{Target: target, Key: key, Value: v.Right}
		}
		if target, key, ok := path.SplitIndexPath(v.Right); ok {
			return constraint.IndexNotEqualsPath{Target: target, Key: key, Value: v.Left}
		}
	}
	return c
}

// callConstraintFallbackFromArgs canonicalizes EqPath/NotEqPath placeholder
// constraints when one argument is non-path (for example literals or #expr).
// In these cases direct path substitution drops the constraint; we recover by
// re-extracting equivalent condition constraints from the original call args.
func callConstraintFallbackFromArgs(
	ce *ConditionExtractor,
	args []ast.Expr,
	argPaths []constraint.Path,
	c constraint.Constraint,
	equality bool,
) ([]constraint.Constraint, bool) {
	if ce == nil || len(args) == 0 {
		return nil, false
	}

	var left, right constraint.Path
	switch v := c.(type) {
	case constraint.EqPath:
		left, right = v.Left, v.Right
	case constraint.NotEqPath:
		left, right = v.Left, v.Right
	default:
		return nil, false
	}

	lIdx, lOK := constraint.PlaceholderArgIndex(left, len(args))
	rIdx, rOK := constraint.PlaceholderArgIndex(right, len(args))
	if !lOK || !rOK {
		return nil, false
	}
	if lIdx >= len(argPaths) || rIdx >= len(argPaths) {
		return nil, false
	}
	// If both arguments resolve to concrete paths, regular substitution keeps
	// the original relation and we should not duplicate constraints here.
	if !argPaths[lIdx].IsEmpty() && !argPaths[rIdx].IsEmpty() {
		return nil, false
	}

	var cond constraint.Condition
	if equality {
		cond = ce.ConditionFromEquality(args[lIdx], args[rIdx])
	} else {
		cond = ce.ConditionFromInequality(args[lIdx], args[rIdx])
	}
	if !cond.HasConstraints() {
		return nil, false
	}
	return cond.MustConstraints(), true
}

// ExtractFunctionRefinement extracts the function refinement from a call using symbol-based lookup.
// All functions in CFG have symbols, so this is the canonical refinement resolution path.
func ExtractFunctionRefinement(
	info *cfg.CallInfo,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	refinementLookupSym constraint.RefinementLookupBySym,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) *constraint.FunctionRefinement {
	var bindings *bind.BindingTable
	if graph != nil {
		bindings = graph.Bindings()
	}
	return callsite.ResolveCalleeEffect(
		info,
		p,
		graph,
		bindings,
		moduleBindings,
		refinementLookupSym,
		synthFn,
		symResolver,
		checkeffects.EffectFromType,
	)
}

// CallTerminates checks if a call is to a function that never returns.
// Uses symbol-based refinement lookup; all functions have symbols.
func CallTerminates(
	info *cfg.CallInfo,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	refinementLookupSym constraint.RefinementLookupBySym,
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) bool {
	if info == nil {
		return false
	}
	var bindings *bind.BindingTable
	if graph != nil {
		bindings = graph.Bindings()
	}
	if eff := callsite.ResolveCalleeEffect(
		info,
		p,
		graph,
		bindings,
		moduleBindings,
		refinementLookupSym,
		synthFn,
		symResolver,
		checkeffects.EffectFromType,
	); eff != nil && eff.Terminates {
		return true
	}
	return false
}

// PointHasTerminatingCallSite reports whether any callsite represented at point p
// definitely terminates control flow.
func PointHasTerminatingCallSite(
	graph *cfg.Graph,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	refinementLookupSym constraint.RefinementLookupBySym,
	moduleBindings *bind.BindingTable,
) bool {
	if graph == nil {
		return false
	}
	for _, callInfo := range graph.CallSitesAt(p) {
		if CallTerminates(callInfo, p, synthFn, symResolver, refinementLookupSym, graph, moduleBindings) {
			return true
		}
	}
	return false
}

// ConstraintsFromAssignOnReturn extracts OnReturn constraints from assignment RHS calls.
func ConstraintsFromAssignOnReturn(
	info *cfg.AssignInfo,
	p cfg.Point,
	sc *scope.State,
	inputs *flow.Inputs,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	typeKeyResolver func(string, *scope.State) (narrow.TypeKey, bool),
	refinementLookupSym constraint.RefinementLookupBySym,
	constResolver func(string) *flow.ConstValue,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) constraint.Condition {
	if info == nil {
		return constraint.Condition{}
	}
	var combined constraint.Condition
	info.EachSourceCall(func(_ int, callInfo *cfg.CallInfo) {
		if cond := ConstraintsFromCallOnReturn(callInfo, p, sc, inputs, synthFn, typeKeyResolver, refinementLookupSym, constResolver, symResolver, graph, moduleBindings); cond.HasConstraints() {
			if !combined.HasConstraints() {
				combined = cond
			} else {
				combined = constraint.And(combined, cond)
			}
		}
	})
	return combined
}

// ExtractPredicateLinkFromCallInfo extracts predicate constraints from pre-extracted CallInfo.
// returnIndex selects which return value carries predicate semantics.
func ExtractPredicateLinkFromCallInfo(
	callInfo *cfg.CallInfo,
	returnIndex int,
	p cfg.Point,
	sc *scope.State,
	inputs *flow.Inputs,
	typeKeyResolver func(string, *scope.State) (narrow.TypeKey, bool),
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	refinementLookupSym constraint.RefinementLookupBySym,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) *flow.PredicateLink {
	if callInfo == nil {
		return nil
	}
	if returnIndex < 0 {
		return nil
	}

	if callInfo.IsTypeCheck && typeKeyResolver != nil {
		typeKey, ok := typeKeyResolver(callInfo.TypeCheckName, sc)
		if ok && !typeKey.IsZero() {
			checkPath := callInfo.TypeCheckPath
			if !checkPath.IsEmpty() {
				// Type:is returns (value, err). Predicate semantics are on err == nil.
				if callInfo.Method == "is" && callInfo.Receiver != nil {
					if returnIndex == 1 {
						onTruthy := constraint.FromConstraints(constraint.NotHasType{Path: checkPath, Type: typeKey})
						onFalsy := constraint.FromConstraints(constraint.HasType{Path: checkPath, Type: typeKey})
						return &flow.PredicateLink{
							OnTruthy: onTruthy,
							OnFalsy:  onFalsy,
						}
					}
					return nil
				}
				// TypeName(x) is a cast, not a predicate (truthiness is unsafe).
				return nil
			}
		}
	}

	if returnIndex != 0 {
		return nil
	}
	callArgs := runtimeCallArgs(callInfo)
	if len(callArgs) == 0 {
		return nil
	}

	eff := ExtractFunctionRefinement(callInfo, p, synthFn, refinementLookupSym, symResolver, graph, moduleBindings)
	if eff == nil || !eff.HasPredicateSemantics() {
		return nil
	}

	bindings := resolve.GetBindings(inputs)
	constResolver := predicate.BuildConstResolver(inputs, p)
	argPaths := make([]constraint.Path, len(callArgs))
	for i, arg := range callArgs {
		argPaths[i] = path.FromExprWithBindingsAt(arg, constResolver, bindings, graph, p)
	}

	onTruthy := eff.OnTrue.Substitute(argPaths)
	onFalsy := eff.OnFalse.Substitute(argPaths)
	onTruthy = rebaseCapturedKeyOf(onTruthy, p, graph, bindings, inputs)
	onFalsy = rebaseCapturedKeyOf(onFalsy, p, graph, bindings, inputs)

	if !onTruthy.HasConstraints() && !onFalsy.HasConstraints() {
		return nil
	}

	return &flow.PredicateLink{
		OnTruthy: onTruthy,
		OnFalsy:  onFalsy,
	}
}

// A captured table has a different SSA version inside its predicate than in
// the caller. The call observes the caller's current binding, so attach that
// version to the returned key fact. A captured rebind prevents that identity
// from being stable across the call.
func rebaseCapturedKeyOf(cond constraint.Condition, p cfg.Point, graph *cfg.Graph, bindings *bind.BindingTable, inputs *flow.Inputs) constraint.Condition {
	if !cond.HasConstraints() || graph == nil || bindings == nil {
		return cond
	}
	reassigned := CapturedReassignments(graph)
	disjuncts := make([][]constraint.Constraint, 0, len(cond.Disjuncts))
	for _, disjunct := range cond.Disjuncts {
		updated := make([]constraint.Constraint, 0, len(disjunct))
		for _, c := range disjunct {
			keyOf, ok := c.(constraint.KeyOf)
			if !ok || keyOf.Table.Symbol == 0 || keyOf.Table.IsPlaceholder() {
				updated = append(updated, c)
				continue
			}
			kind, bound := bindings.Kind(keyOf.Table.Symbol)
			version := graph.VisibleVersion(p, keyOf.Table.Symbol)
			if !bound || kind != cfg.SymbolLocal || reassigned[keyOf.Table.Symbol] || version.IsZero() ||
				capturedTableMayHaveAlias(inputs, graph, p, keyOf.Table.Symbol) {
				continue
			}
			keyOf.Table.Version = version.ID
			updated = append(updated, keyOf)
		}
		if len(updated) == 0 {
			return constraint.TrueCondition()
		}
		disjuncts = append(disjuncts, updated)
	}
	return constraint.FromDisjuncts(disjuncts)
}

// An alias created before this call can mutate the captured table after the
// predicate returns. Do not publish a portable key fact in that case.
func capturedTableMayHaveAlias(inputs *flow.Inputs, graph *cfg.Graph, callPoint cfg.Point, tableSym cfg.SymbolID) bool {
	if inputs == nil {
		return true
	}
	captures := 0
	for _, nested := range graph.NestedFunctions() {
		if nested.Func == nil {
			continue
		}
		for _, sym := range graph.Bindings().CapturedSymbols(nested.Func) {
			if sym == tableSym {
				captures++
			}
		}
	}
	if captures != 1 {
		return true
	}
	for point, roots := range inputs.CallAliasRoots {
		if point != callPoint && !graph.Reachable(point, callPoint, true) {
			continue
		}
		for _, sym := range roots {
			if sym == tableSym {
				return true
			}
		}
	}
	for _, assignment := range inputs.Assignments {
		if assignment.SourcePath.Symbol != tableSym || len(assignment.SourcePath.Segments) != 0 ||
			assignment.TargetPath.Symbol == 0 ||
			(assignment.TargetPath.Symbol == tableSym && len(assignment.TargetPath.Segments) == 0) {
			continue
		}
		if graph.Reachable(assignment.Point, callPoint, true) {
			return true
		}
	}
	return false
}

// ComputeDeadPoints computes dead points from a graph using effect-based termination analysis.
func ComputeDeadPoints(
	graph *cfg.Graph,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	refinementLookupSym constraint.RefinementLookupBySym,
	moduleBindings *bind.BindingTable,
) map[cfg.Point]bool {
	dead := make(map[cfg.Point]bool)
	for _, p := range graph.RPO() {
		if PointHasTerminatingCallSite(graph, p, synthFn, symResolver, refinementLookupSym, moduleBindings) {
			for _, succ := range graph.Successors(p) {
				preds := graph.Predecessors(succ)
				if len(preds) == 1 {
					dead[succ] = true
				}
			}
		}
	}
	entry := graph.Entry()
	graph.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		if p == entry {
			return
		}
		if len(graph.Predecessors(p)) == 0 {
			dead[p] = true
		}
	})
	return dead
}
