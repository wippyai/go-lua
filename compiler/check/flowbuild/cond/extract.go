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
	"github.com/wippyai/go-lua/compiler/check/callsite"
	checkeffects "github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/numconst"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/sibling"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

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

		constResolver := predicate.BuildConstResolver(inputs, p)

		ce := &ConditionExtractor{
			P: p, SC: fc.Scopes[p], Inputs: inputs,
			Synth:         fc.Derived.Synth,
			SymResolver:   fc.Derived.SymResolver,
			TypeKeyRes:    fc.Derived.TypeKeyRes,
			ConstResolver: constResolver,
			EffectBySym:   fc.Derived.EffectBySym,
		}
		constraints := ce.ConstraintsFromBranch(info)

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
						fc.Derived.Synth, fc.Derived.SymResolver, constResolver, bindings,
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
	} else if arrPath := ExtractLenOfPath(forInfo.Limit, branchPoint, graph); !arrPath.IsEmpty() {
		result = append(result, constraint.LeLenOf{X: varPath, Array: arrPath})
	} else {
		return nil
	}

	return result
}

// extractLenOfPath extracts the array path from a #arr expression.
func ExtractLenOfPath(expr ast.Expr, p cfg.Point, graph *cfg.Graph) constraint.Path {
	lenOp, ok := expr.(*ast.UnaryLenOpExpr)
	if !ok {
		return constraint.Path{}
	}

	ident, ok := lenOp.Expr.(*ast.IdentExpr)
	if !ok || ident.Value == "" {
		return constraint.Path{}
	}

	bindings := graph.Bindings()
	if bindings == nil {
		return constraint.Path{Root: ident.Value}
	}

	sym, found := bindings.SymbolOf(ident)
	if !found || sym == 0 {
		return constraint.Path{Root: ident.Value}
	}

	name := ident.Value
	if n := bindings.Name(sym); n != "" {
		name = n
	}
	lenPath := constraint.Path{Root: name, Symbol: sym}
	return path.WithVersion(lenPath, graph, p)
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

	for _, p := range fc.Graph.RPO() {
		if !PointHasTerminatingCallSite(fc.Graph, p, fc.Derived.Synth, fc.Derived.SymResolver, fc.Derived.EffectBySym, fc.ModuleBindings) {
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

		cond := ConstraintsFromCallOnReturn(info, p, sc, inputs, fc.Derived.Synth, fc.Derived.TypeKeyRes, fc.Derived.EffectBySym, constResolver, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings)
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
		cond := ConstraintsFromAssignOnReturn(info, p, sc, inputs, fc.Derived.Synth, fc.Derived.TypeKeyRes, fc.Derived.EffectBySym, constResolver, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings)
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

// constraintsFromCallOnReturn extracts OnReturn constraints from a call.
func ConstraintsFromCallOnReturn(
	info *cfg.CallInfo,
	p cfg.Point,
	sc *scope.State,
	inputs *flow.Inputs,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	typeKeyResolver func(string, *scope.State) (narrow.TypeKey, bool),
	effectLookupSym constraint.EffectLookupBySym,
	constResolver func(string) *flow.ConstValue,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) constraint.Condition {
	if info == nil || callsite.IsMethodLikeCallInfo(info) {
		return constraint.Condition{}
	}
	if len(info.Args) == 0 {
		return constraint.Condition{}
	}

	bindings := resolve.GetBindings(inputs)

	// TypeName(x) pattern - check metatype
	if info.CalleeName != "" && typeKeyResolver != nil {
		if typeKey, ok := typeKeyResolver(info.CalleeName, sc); ok && !typeKey.IsZero() {
			if len(info.Args) > 0 {
				argPath := path.FromExprWithBindingsAt(info.Args[0], constResolver, bindings, graph, p)
				if !argPath.IsEmpty() {
					return constraint.FromConstraints(constraint.HasType{Path: argPath, Type: typeKey})
				}
			}
		}
	}

	eff := ExtractFunctionEffect(info, p, synthFn, effectLookupSym, symResolver, graph, moduleBindings)
	if eff == nil || !eff.OnReturn.HasConstraints() {
		return constraint.Condition{}
	}

	argPaths := make([]constraint.Path, len(info.Args))
	for i, arg := range info.Args {
		argPaths[i] = path.FromExprWithBindingsAt(arg, constResolver, bindings, graph, p)
	}

	ce := &ConditionExtractor{
		P: p, SC: sc, Inputs: inputs,
		Synth:         synthFn,
		SymResolver:   symResolver,
		TypeKeyRes:    typeKeyResolver,
		ConstResolver: constResolver,
		EffectBySym:   effectLookupSym,
	}

	var result constraint.Condition
	for _, disj := range eff.OnReturn.Disjuncts {
		subDisj := constraint.SubstituteConjunction(disj, argPaths)
		subDisj = normalizePathConstraints(subDisj)
		subDisj = append(subDisj, siblingConstraintsFromOnReturn(subDisj, inputs, bindings, graph, p)...)
		cond := constraint.FromConjunction(subDisj)

		for _, c := range disj {
			switch v := c.(type) {
			case constraint.Falsy:
				if idx, ok := constraint.PlaceholderArgIndex(v.Path, len(info.Args)); ok && argPaths[idx].IsEmpty() {
					fallback := ce.ConditionFromExpr(info.Args[idx])
					if fallback.HasConstraints() {
						fallback = constraint.Not(fallback)
						cond = constraint.And(cond, fallback)
					}
				}
			case constraint.Truthy:
				if idx, ok := constraint.PlaceholderArgIndex(v.Path, len(info.Args)); ok && argPaths[idx].IsEmpty() {
					fallback := ce.ConditionFromExpr(info.Args[idx])
					if fallback.HasConstraints() {
						cond = constraint.And(cond, fallback)
					}
				}
			}
		}

		if cond.IsTrue() {
			return constraint.TrueCondition()
		}
		if cond.IsFalse() || !cond.HasConstraints() {
			continue
		}
		if !result.HasConstraints() {
			result = cond
		} else {
			result = constraint.Or(result, cond)
		}
	}

	return result
}

func siblingConstraintsFromOnReturn(disj []constraint.Constraint, inputs *flow.Inputs, bindings *bind.BindingTable, graph *cfg.Graph, p cfg.Point) []constraint.Constraint {
	if len(disj) == 0 {
		return nil
	}
	var out []constraint.Constraint
	for _, c := range disj {
		var cpath constraint.Path
		var wantNil bool
		switch v := c.(type) {
		case constraint.IsNil:
			cpath = v.Path
			wantNil = false
		case constraint.Falsy:
			cpath = v.Path
			wantNil = false
		case constraint.NotNil:
			cpath = v.Path
			wantNil = true
		case constraint.Truthy:
			cpath = v.Path
			wantNil = true
		default:
			continue
		}
		if cpath.Symbol == 0 {
			continue
		}
		version := graph.VisibleVersion(p, cpath.Symbol)
		raw := sibling.ConstraintsForSymbol(cpath.Symbol, version.ID, inputs, wantNil, bindings)
		if len(raw) == 0 {
			continue
		}
		for _, rc := range raw {
			switch v := rc.(type) {
			case constraint.IsNil:
				v.Path = path.WithVersion(v.Path, graph, p)
				out = append(out, v)
			case constraint.NotNil:
				v.Path = path.WithVersion(v.Path, graph, p)
				out = append(out, v)
			default:
				out = append(out, rc)
			}
		}
	}
	return out
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

// ExtractFunctionEffect extracts the function effect from a call using symbol-based lookup.
// All functions in CFG have symbols, so this is the canonical effect resolution path.
func ExtractFunctionEffect(
	info *cfg.CallInfo,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	effectLookupSym constraint.EffectLookupBySym,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) *constraint.FunctionEffect {
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
		effectLookupSym,
		synthFn,
		symResolver,
		checkeffects.EffectFromType,
	)
}

func ExtractEffectFromType(t typ.Type) *constraint.FunctionEffect {
	return checkeffects.EffectFromType(t)
}

// CallTerminates checks if a call is to a function that never returns.
// Uses symbol-based effect lookup - all functions have symbols.
func CallTerminates(
	info *cfg.CallInfo,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	effectLookupSym constraint.EffectLookupBySym,
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
		effectLookupSym,
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
	effectLookupSym constraint.EffectLookupBySym,
	moduleBindings *bind.BindingTable,
) bool {
	if graph == nil {
		return false
	}
	for _, callInfo := range graph.CallSitesAt(p) {
		if CallTerminates(callInfo, p, synthFn, symResolver, effectLookupSym, graph, moduleBindings) {
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
	effectLookupSym constraint.EffectLookupBySym,
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
		if cond := ConstraintsFromCallOnReturn(callInfo, p, sc, inputs, synthFn, typeKeyResolver, effectLookupSym, constResolver, symResolver, graph, moduleBindings); cond.HasConstraints() {
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
	effectLookupSym constraint.EffectLookupBySym,
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
	if len(callInfo.Args) == 0 {
		return nil
	}

	eff := ExtractFunctionEffect(callInfo, p, synthFn, effectLookupSym, symResolver, graph, moduleBindings)
	if eff == nil || !eff.HasPredicateSemantics() {
		return nil
	}

	bindings := resolve.GetBindings(inputs)
	constResolver := predicate.BuildConstResolver(inputs, p)
	argPaths := make([]constraint.Path, len(callInfo.Args))
	for i, arg := range callInfo.Args {
		argPaths[i] = path.FromExprWithBindingsAt(arg, constResolver, bindings, graph, p)
	}

	onTruthy := eff.OnTrue.Substitute(argPaths)
	onFalsy := eff.OnFalse.Substitute(argPaths)

	if !onTruthy.HasConstraints() && !onFalsy.HasConstraints() {
		return nil
	}

	return &flow.PredicateLink{
		OnTruthy: onTruthy,
		OnFalsy:  onFalsy,
	}
}

// ComputeDeadPoints computes dead points from a graph using effect-based termination analysis.
func ComputeDeadPoints(
	graph *cfg.Graph,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	effectLookupSym constraint.EffectLookupBySym,
	moduleBindings *bind.BindingTable,
) map[cfg.Point]bool {
	dead := make(map[cfg.Point]bool)
	for _, p := range graph.RPO() {
		if PointHasTerminatingCallSite(graph, p, synthFn, symResolver, effectLookupSym, moduleBindings) {
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
