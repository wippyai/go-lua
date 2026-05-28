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
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/numconst"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/abstract/sibling"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	checkeffects "github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractEdgeConstraints extracts type constraints from branch conditions.
func ExtractEdgeConstraints(fc *core.FlowContext, inputs *flow.Inputs) {
	for _, branch := range fc.Evidence.Branches {
		p := branch.Point
		info := branch.Info
		if info == nil {
			continue
		}
		succs := fc.Graph.Successors(p)
		if len(succs) < 2 {
			continue
		}

		trueEdge, falseEdge := FindBranchEdges(fc.Graph, p, succs)
		if trueEdge == 0 && falseEdge == 0 {
			continue
		}

		constResolver := predicate.BuildConstResolver(inputs, p)

		ce := &ConditionExtractor{
			P: p, SC: fc.Scopes[p], Inputs: inputs,
			Synth:           fc.Derived.Synth,
			SymResolver:     fc.Derived.SymResolver,
			TypeKeyRes:      fc.Derived.TypeKeyRes,
			ConstResolver:   constResolver,
			RefinementBySym: fc.Derived.RefinementBySym,
			ModuleBindings:  fc.ModuleBindings,
			Evidence:        fc.Evidence,
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
	}
}

// ExtractNumericConstraints extracts numeric constraints from branch conditions.
func ExtractNumericConstraints(fc *core.FlowContext, inputs *flow.Inputs) {
	anchors := buildLoopCounterAnchors(fc.Graph, inputs)
	for _, branch := range fc.Evidence.Branches {
		p := branch.Point
		info := branch.Info
		if info == nil {
			continue
		}
		succs := fc.Graph.Successors(p)
		if len(succs) < 2 {
			continue
		}

		trueEdge, falseEdge := FindBranchEdges(fc.Graph, p, succs)
		if trueEdge == 0 && falseEdge == 0 {
			continue
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
			continue
		}

		numConstraints := NumericBranchConstraintsFromExpr(info.Condition, p, inputs)
		onTrue := appendLoopCounterAnchors(numConstraints.OnTrue, anchors, p)
		onFalse := appendLoopCounterAnchors(numConstraints.OnFalse, anchors, p)
		if trueEdge != 0 && len(onTrue) > 0 {
			inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, flow.EdgeNumericConstraint{
				From:        p,
				To:          trueEdge,
				Constraints: onTrue,
			})
		}
		if falseEdge != 0 && len(onFalse) > 0 {
			inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, flow.EdgeNumericConstraint{
				From:        p,
				To:          falseEdge,
				Constraints: onFalse,
			})
		}
	}

	extractCallEnsuresNumericConstraints(fc, inputs)
}

// extractCallEnsuresNumericConstraints seeds length facts from a statement call
// whose refinement guarantees a length relation on a container argument. An
// assertion wrapper that ensures `actual == expected` over args `#arr` and a
// constant proves `len(arr) == const` on its normal-return edges, which the
// index-read proof then consumes. The relation is read structurally from the
// callee refinement (no name-matching); only length-over-container args produce
// a numeric fact.
func extractCallEnsuresNumericConstraints(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc == nil || fc.Graph == nil || fc.Derived == nil {
		return
	}
	bindings := resolve.GetBindings(inputs)
	for _, call := range fc.Evidence.Calls {
		if call.Origin != api.CallOriginStatement {
			continue
		}
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		callArgs := runtimeCallArgs(info)
		if len(callArgs) == 0 {
			continue
		}
		eff := ExtractFunctionRefinement(info, p, fc.Derived.Synth, fc.Derived.RefinementBySym, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings)
		if eff == nil || !eff.OnReturn.HasConstraints() {
			continue
		}
		constResolver := predicate.BuildConstResolver(inputs, p)
		var ncs []constraint.NumericConstraint
		for _, c := range eff.OnReturn.MustConstraints() {
			ncs = append(ncs, callEnsuresLengthConstraints(c, callArgs, p, inputs, bindings, constResolver)...)
		}
		if len(ncs) == 0 {
			continue
		}
		for _, succ := range fc.Graph.Successors(p) {
			if inputs.DeadPoints != nil && inputs.DeadPoints[succ] {
				continue
			}
			inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, flow.EdgeNumericConstraint{
				From:        p,
				To:          succ,
				Constraints: ncs,
			})
		}
	}
}

// callEnsuresLengthConstraints maps a single refinement constraint that relates
// two argument placeholders into length numeric constraints when one argument is
// a length expression `#container` and the other is an integer constant.
func callEnsuresLengthConstraints(
	c constraint.Constraint,
	callArgs []ast.Expr,
	p cfg.Point,
	inputs *flow.Inputs,
	bindings *bind.BindingTable,
	constResolver func(string) *flow.ConstValue,
) []constraint.NumericConstraint {
	left, right, equality, ok := placeholderRelationArgs(c)
	if !ok {
		return nil
	}
	lIdx, lOK := constraint.PlaceholderArgIndex(left, len(callArgs))
	rIdx, rOK := constraint.PlaceholderArgIndex(right, len(callArgs))
	if !lOK || !rOK || lIdx >= len(callArgs) || rIdx >= len(callArgs) {
		return nil
	}
	if !equality {
		return nil
	}
	if lenPath, c, ok := lenAndConstArgs(callArgs[lIdx], callArgs[rIdx], p, inputs, bindings, constResolver); ok {
		return lenEqConstraints(lenPath, c)
	}
	if lenPath, c, ok := lenAndConstArgs(callArgs[rIdx], callArgs[lIdx], p, inputs, bindings, constResolver); ok {
		return lenEqConstraints(lenPath, c)
	}
	return nil
}

func placeholderRelationArgs(c constraint.Constraint) (left, right constraint.Path, equality, ok bool) {
	switch v := c.(type) {
	case constraint.EqPath:
		return v.Left, v.Right, true, true
	case constraint.NotEqPath:
		return v.Left, v.Right, false, true
	}
	return constraint.Path{}, constraint.Path{}, false, false
}

func lenAndConstArgs(
	lenArg, constArg ast.Expr,
	p cfg.Point,
	inputs *flow.Inputs,
	bindings *bind.BindingTable,
	constResolver func(string) *flow.ConstValue,
) (constraint.Path, int64, bool) {
	lenOp, ok := lenArg.(*ast.UnaryLenOpExpr)
	if !ok || lenOp == nil {
		return constraint.Path{}, 0, false
	}
	container := path.FromExprWithBindingsAt(lenOp.Expr, constResolver, bindings, inputs.Graph, p)
	if container.IsEmpty() {
		return constraint.Path{}, 0, false
	}
	c, ok := numconst.IntConstFromExpr(constArg)
	if !ok || c < 0 {
		return constraint.Path{}, 0, false
	}
	return container, c, true
}

func lenEqConstraints(container constraint.Path, c int64) []constraint.NumericConstraint {
	return []constraint.NumericConstraint{
		constraint.LenGeConst{Array: container, C: c},
		constraint.LenLeConst{Array: container, C: c},
	}
}

// loopCounterAnchor is a loop counter's init-side numeric bound together with the
// loop body it holds in. The body bounds where the anchor may be re-asserted, so
// a counter reused outside its loop (a later reassignment, an unrelated branch on
// the same symbol) is never anchored with an invariant that holds only in-loop.
type loopCounterAnchor struct {
	constraint constraint.NumericConstraint
	body       map[cfg.Point]bool
}

// appendLoopCounterAnchors pairs each loop counter's init-side anchor with any
// numeric edge constraint that bounds that counter at a branch inside the
// counter's loop. Pairing the init anchor on the SAME edge as a guard bound seeds
// the counter's interval with both ends at once, so the body read sees a tight
// interval immediately instead of one whose init-side end was widened away at the
// loop header. The anchor is a sound loop invariant (a monotone counter never
// crosses its init on the init side), so re-asserting it on an in-loop guard edge
// never admits an out-of-range value.
func appendLoopCounterAnchors(constraints []constraint.NumericConstraint, anchors map[cfg.SymbolID]loopCounterAnchor, branch cfg.Point) []constraint.NumericConstraint {
	if len(constraints) == 0 || len(anchors) == 0 {
		return constraints
	}
	seen := make(map[cfg.SymbolID]struct{})
	out := constraints
	for _, nc := range constraints {
		for _, p := range nc.Paths() {
			if p.Symbol == 0 {
				continue
			}
			if _, ok := seen[p.Symbol]; ok {
				continue
			}
			anchor, ok := anchors[p.Symbol]
			if !ok || !anchor.body[branch] {
				continue
			}
			seen[p.Symbol] = struct{}{}
			out = append(out, anchor.constraint)
		}
	}
	return out
}

// buildLoopCounterAnchors maps each monotone loop counter to its init-side
// numeric anchor and the loop body it holds in, keyed by symbol. A counter
// qualifies when some loop header (LoopPreheaderSet with the counter in LoopVars)
// gives it a constant integer init at the header's preheader and a single
// constant-step self-update in the loop body. An incrementing counter (step > 0)
// anchors a lower bound (i >= init); a decrementing one (step < 0) anchors an
// upper bound (i <= init). The anchors let appendLoopCounterAnchors restore the
// init-side end on in-loop guard edges so the header widening cannot strand a
// sound bound. This covers while, while-true + break, and repeat-until uniformly,
// with no loop-kind special-casing.
func buildLoopCounterAnchors(graph *cfg.Graph, inputs *flow.Inputs) map[cfg.SymbolID]loopCounterAnchor {
	c := graph.CFG()
	var anchors map[cfg.SymbolID]loopCounterAnchor
	for pi := range c.Nodes {
		header := cfg.Point(pi)
		node := c.Node(header)
		if node == nil || !node.LoopPreheaderSet || len(node.LoopVars) == 0 {
			continue
		}
		constResolver := predicate.BuildConstResolver(inputs, node.LoopPreheader)
		if constResolver == nil {
			continue
		}
		body := loopBodyPoints(c, header)
		for _, counter := range node.LoopVars {
			nc, ok := loopCounterInitAnchor(graph, header, counter, body, constResolver)
			if !ok {
				continue
			}
			if anchors == nil {
				anchors = make(map[cfg.SymbolID]loopCounterAnchor)
			}
			anchors[counter] = loopCounterAnchor{constraint: nc, body: body}
		}
	}
	return anchors
}

// loopCounterInitAnchor builds the init-side numeric anchor for one loop counter,
// or ok=false when the counter has no constant init at the preheader or no
// constant-step self-update establishing a monotone direction in the loop body.
func loopCounterInitAnchor(graph *cfg.Graph, header cfg.Point, counter cfg.SymbolID, body map[cfg.Point]bool, constResolver func(string) *flow.ConstValue) (constraint.NumericConstraint, bool) {
	if counter == 0 {
		return nil, false
	}
	name := graph.NameOf(counter)
	if name == "" {
		return nil, false
	}
	counterPath := constraint.Path{Root: name, Symbol: counter}
	init, ok := initConstFor(counterPath, constResolver)
	if !ok {
		return nil, false
	}
	step, ok := loopCounterStep(graph, body, counter)
	if !ok || step == 0 {
		return nil, false
	}
	if step > 0 {
		return constraint.GeConst{X: counterPath, C: init}, true
	}
	return constraint.LeConst{X: counterPath, C: init}, true
}

// loopCounterStep returns the signed constant step of a loop counter's monotone
// self-update within the given loop body, or ok=false when the counter is not
// monotone there. Every assignment to the counter inside the loop body must be
// the same constant-step self-update (counter = counter +|- k); any other in-loop
// write (reset, non-constant step, differing step) voids the anchor so no
// monotone direction is assumed unsoundly. Assignments outside the loop body (the
// counter's pre-loop initializer) are ignored.
func loopCounterStep(graph *cfg.Graph, body map[cfg.Point]bool, counter cfg.SymbolID) (int64, bool) {
	c := graph.CFG()
	var step int64
	found := false
	for pi := range c.Nodes {
		if !body[cfg.Point(pi)] {
			continue
		}
		info, ok := graph.Info(cfg.Point(pi)).(*cfg.AssignInfo)
		if !ok {
			continue
		}
		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol != counter {
				continue
			}
			if i >= len(info.Sources) {
				return 0, false
			}
			s, ok := selfUpdateStep(info.Sources[i], counter, graph)
			if !ok {
				return 0, false
			}
			if found && s != step {
				return 0, false
			}
			step = s
			found = true
		}
	}
	return step, found
}

// loopBodyPoints returns the natural-loop body of header: header plus every node
// that reaches a latch (a back-edge predecessor of header) without passing
// through header or the loop preheader. The preheader (the unique entry edge,
// marked by the CFG) distinguishes the loop-entry predecessor from the latches;
// every other predecessor of header is a back edge whose source is inside the
// loop. Bounding the backward walk by header and the preheader excludes both the
// pre-loop initializer and any enclosing-loop body, so a nested loop's body
// contains only its own nodes and the counter's self-update is checked against
// the precise loop it belongs to.
func loopBodyPoints(c *cfg.CFG, header cfg.Point) map[cfg.Point]bool {
	body := make(map[cfg.Point]bool)
	body[header] = true
	node := c.Node(header)
	if node != nil && node.LoopPreheaderSet {
		body[node.LoopPreheader] = true
	}
	var stack []cfg.Point
	for _, pred := range c.PredecessorsReadOnly(header) {
		if !body[pred] {
			body[pred] = true
			stack = append(stack, pred)
		}
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, pred := range c.PredecessorsReadOnly(p) {
			if !body[pred] {
				body[pred] = true
				stack = append(stack, pred)
			}
		}
	}
	// The preheader is a boundary only, never a body member.
	if node != nil && node.LoopPreheaderSet {
		delete(body, node.LoopPreheader)
	}
	return body
}

// selfUpdateStep returns the signed constant step when expr is counter (+|-) k,
// resolving the counter operand by symbol identity. Any other shape is ok=false.
func selfUpdateStep(expr ast.Expr, counter cfg.SymbolID, graph *cfg.Graph) (int64, bool) {
	arith, ok := expr.(*ast.ArithmeticOpExpr)
	if !ok || (arith.Operator != "+" && arith.Operator != "-") {
		return 0, false
	}
	bindings := graph.Bindings()
	counterIsLhs := identIsSymbol(arith.Lhs, counter, bindings)
	counterIsRhs := identIsSymbol(arith.Rhs, counter, bindings)
	switch {
	case counterIsLhs:
		k, ok := numconst.IntConstFromExpr(arith.Rhs)
		if !ok {
			return 0, false
		}
		if arith.Operator == "-" {
			return -k, true
		}
		return k, true
	case counterIsRhs && arith.Operator == "+":
		// k + counter is also increasing by k.
		k, ok := numconst.IntConstFromExpr(arith.Lhs)
		if !ok {
			return 0, false
		}
		return k, true
	}
	return 0, false
}

func identIsSymbol(expr ast.Expr, sym cfg.SymbolID, bindings *bind.BindingTable) bool {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || bindings == nil {
		return false
	}
	s, ok := bindings.SymbolOf(ident)
	return ok && s == sym
}

// initConstFor returns the integer value the counter path holds at the loop
// preheader, or ok=false when it is not a compile-time integer constant there.
func initConstFor(p constraint.Path, constResolver func(string) *flow.ConstValue) (int64, bool) {
	if p.Root == "" {
		return 0, false
	}
	val := constResolver(p.Root)
	if val == nil || val.Kind != flow.ConstInt {
		return 0, false
	}
	return val.Int, true
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

	root := varName
	if varSymbol != 0 {
		if name := graph.NameOf(varSymbol); name != "" {
			root = name
		}
	}
	varPath := path.WithVersion(constraint.Path{Root: root, Symbol: varSymbol}, graph, branchPoint)

	// The Init bound is the loop-start end; Limit is the loop-stop end. A positive
	// step ascends from Init up to Limit, so Init is the lower bound and Limit the
	// upper; a negative step descends from Init down to Limit, so Init is the upper
	// bound and Limit the lower. A non-constant step has no provable direction and
	// emits no bounds (sound: no presence proof).
	step, stepOk := forStepValue(forInfo.Step)
	if !stepOk || step == 0 {
		return nil
	}
	var lowerExpr, upperExpr ast.Expr
	if step > 0 {
		lowerExpr, upperExpr = forInfo.Init, forInfo.Limit
	} else {
		lowerExpr, upperExpr = forInfo.Limit, forInfo.Init
	}

	lower, lowerOk := forLowerConstraint(varPath, lowerExpr)
	upper, upperOk := forUpperConstraint(varPath, upperExpr, branchPoint, graph)
	if !lowerOk || !upperOk {
		return nil
	}
	return []constraint.NumericConstraint{lower, upper}
}

// forStepValue returns the integer step of a numeric for-loop, defaulting to 1
// when the step is omitted. A non-integer-constant step yields ok=false.
func forStepValue(step ast.Expr) (int64, bool) {
	if step == nil {
		return 1, true
	}
	return numconst.IntConstFromExpr(step)
}

// forLowerConstraint builds the v >= X lower bound for a for-loop end. Only an
// integer-constant end is expressible as a lower bound (there is no symbolic
// length lower-bound constraint), so a length-expression lower end yields false.
func forLowerConstraint(varPath constraint.Path, expr ast.Expr) (constraint.NumericConstraint, bool) {
	c, ok := numconst.IntConstFromExpr(expr)
	if !ok {
		return nil, false
	}
	return constraint.GeConst{X: varPath, C: c}, true
}

// forUpperConstraint builds the v <= X upper bound for a for-loop end, as either
// a constant (LeConst) or a symbolic array-length relation (LeLenOf, e.g. #arr).
func forUpperConstraint(varPath constraint.Path, expr ast.Expr, branchPoint cfg.Point, graph *cfg.Graph) (constraint.NumericConstraint, bool) {
	if c, ok := numconst.IntConstFromExpr(expr); ok {
		return constraint.LeConst{X: varPath, C: c}, true
	}
	if arrPath, offset, ok := ExtractLenBound(expr, branchPoint, graph); ok {
		return constraint.LeLenOf{X: varPath, Array: arrPath, Offset: offset}, true
	}
	return nil, false
}

func ExtractLenPath(expr ast.Expr, p cfg.Point, graph *cfg.Graph) constraint.Path {
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

// ExtractLenOfPath returns only the path component of a length-bound expression.
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

	for _, p := range fc.Graph.RPO() {
		if !PointHasTerminatingCallEvidence(fc.Graph, fc.Evidence.Calls, p, fc.Derived.Synth, fc.Derived.SymResolver, fc.Derived.RefinementBySym, fc.ModuleBindings) {
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

	for _, call := range fc.Evidence.Calls {
		if call.Origin != api.CallOriginStatement {
			continue
		}
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		sc := fc.Scopes[p]
		constResolver := predicate.BuildConstResolver(inputs, p)

		cond := ConstraintsFromCallOnReturn(info, p, sc, inputs, fc.Derived.Synth, fc.Derived.TypeKeyRes, fc.Derived.RefinementBySym, constResolver, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings, fc.Evidence)
		if !cond.HasConstraints() {
			continue
		}
		for _, succ := range fc.Graph.Successors(p) {
			key := EdgeKey{From: p, To: succ}
			if existing, ok := out[key]; ok && existing.HasConstraints() {
				out[key] = constraint.And(existing, cond)
			} else {
				out[key] = cond
			}
		}
	}

	for _, assign := range fc.Evidence.Assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		sc := fc.Scopes[p]
		constResolver := predicate.BuildConstResolver(inputs, p)
		cond := ConstraintsFromAssignOnReturn(info, p, sc, inputs, fc.Derived.Synth, fc.Derived.TypeKeyRes, fc.Derived.RefinementBySym, constResolver, fc.Derived.SymResolver, fc.Graph, fc.ModuleBindings, fc.Evidence)
		if !cond.HasConstraints() {
			continue
		}
		for _, succ := range fc.Graph.Successors(p) {
			key := EdgeKey{From: p, To: succ}
			if existing, ok := out[key]; ok && existing.HasConstraints() {
				out[key] = constraint.And(existing, cond)
			} else {
				out[key] = cond
			}
		}
	}

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
	refinementLookupSym constraint.RefinementLookupBySym,
	constResolver func(string) *flow.ConstValue,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
	evidence api.FlowEvidence,
) constraint.Condition {
	if info == nil {
		return constraint.Condition{}
	}
	callArgs := runtimeCallArgs(info)
	if len(callArgs) == 0 {
		return constraint.Condition{}
	}

	bindings := resolve.GetBindings(inputs)

	// TypeName(x) pattern - check metatype
	if info.CalleeName != "" && typeKeyResolver != nil {
		if typeKey, ok := typeKeyResolver(info.CalleeName, sc); ok && !typeKey.IsZero() {
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
		argPaths[i] = fillCallArgPathSymbol(info, i, argPaths[i], graph)
	}

	ce := &ConditionExtractor{
		P: p, SC: sc, Inputs: inputs,
		Synth:           synthFn,
		SymResolver:     symResolver,
		TypeKeyRes:      typeKeyResolver,
		ConstResolver:   constResolver,
		RefinementBySym: refinementLookupSym,
		ModuleBindings:  moduleBindings,
		Evidence:        evidence,
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
		if returnKeyCollectionConstraint(c) {
			continue
		}
		switch v := c.(type) {
		case constraint.Falsy:
			if idx, ok := constraint.PlaceholderArgIndex(v.Path, len(callArgs)); ok && argPaths[idx].IsEmpty() {
				reconstructed := ce.ConditionFromExpr(callArgs[idx])
				if reconstructed.HasConstraints() {
					must = append(must, constraint.Not(reconstructed).MustConstraints()...)
				}
				continue
			}
		case constraint.Truthy:
			if idx, ok := constraint.PlaceholderArgIndex(v.Path, len(callArgs)); ok && argPaths[idx].IsEmpty() {
				reconstructed := ce.ConditionFromExpr(callArgs[idx])
				if reconstructed.HasConstraints() {
					must = append(must, reconstructed.MustConstraints()...)
				}
				continue
			}
		case constraint.EqPath:
			if reconstructed, ok := callConstraintFromOriginalArgs(ce, callArgs, argPaths, v, true); ok {
				must = append(must, reconstructed...)
				continue
			}
		case constraint.NotEqPath:
			if reconstructed, ok := callConstraintFromOriginalArgs(ce, callArgs, argPaths, v, false); ok {
				must = append(must, reconstructed...)
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
	must = append(must, siblingConstraintsFromOnReturn(must, inputs, bindings, graph, p)...)
	cond := constraint.FromConjunction(must)

	if cond.IsFalse() || !cond.HasConstraints() {
		return constraint.Condition{}
	}
	return cond
}

func returnKeyCollectionConstraint(c constraint.Constraint) bool {
	keyOf, ok := c.(constraint.KeyOf)
	return ok && keyOf.Table.IsPlaceholder() && constraint.IsReturnPath(keyOf.Key)
}

func fillCallArgPathSymbol(info *cfg.CallInfo, runtimeIdx int, p constraint.Path, graph *cfg.Graph) constraint.Path {
	if info == nil || p.Symbol != 0 || callsite.IsMethodCallInfo(info) {
		return p
	}
	if runtimeIdx < 0 || runtimeIdx >= len(info.ArgSymbols) {
		return p
	}
	sym := info.ArgSymbols[runtimeIdx]
	if sym == 0 {
		return p
	}
	p.Symbol = sym
	if p.Root == "" && graph != nil {
		p.Root = graph.NameOf(sym)
	}
	return p
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

// callConstraintFromOriginalArgs canonicalizes EqPath/NotEqPath placeholder
// constraints when one argument is non-path (for example literals or #expr).
// Direct path substitution cannot encode that relation, so the extractor
// rebuilds the equivalent condition from the original call arguments.
func callConstraintFromOriginalArgs(
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

// PointHasTerminatingCallEvidence reports whether the trace contains a
// definitely-terminating call at point p.
func PointHasTerminatingCallEvidence(
	graph *cfg.Graph,
	calls []api.CallEvidence,
	p cfg.Point,
	synthFn func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	refinementLookupSym constraint.RefinementLookupBySym,
	moduleBindings *bind.BindingTable,
) bool {
	if graph == nil {
		return false
	}
	for _, call := range calls {
		if call.Point != p {
			continue
		}
		if CallTerminates(call.Info, p, synthFn, symResolver, refinementLookupSym, graph, moduleBindings) {
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
	evidence api.FlowEvidence,
) constraint.Condition {
	if info == nil {
		return constraint.Condition{}
	}
	var combined constraint.Condition
	info.EachSourceCall(func(_ int, callInfo *cfg.CallInfo) {
		if cond := ConstraintsFromCallOnReturn(callInfo, p, sc, inputs, synthFn, typeKeyResolver, refinementLookupSym, constResolver, symResolver, graph, moduleBindings, evidence); cond.HasConstraints() {
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

	if !onTruthy.HasConstraints() && !onFalsy.HasConstraints() {
		return nil
	}

	return &flow.PredicateLink{
		OnTruthy: onTruthy,
		OnFalsy:  onFalsy,
	}
}
