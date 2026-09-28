// Package flowbuild implements flow constraint extraction from control flow graphs.
// This is the core of Phase B that transforms CFG structure into flow.Inputs,
// the constraint system that the flow solver uses to compute type narrowing.
//
// # EXTRACTION PIPELINE
//
// The Run function executes a multi-stage extraction pipeline:
//
//  1. Declarations: Extract type keys, declared types, and module aliases
//     from the CFG. This seeds the initial type information.
//
//  2. Const Propagation: Collect constant assignments and propagate constant
//     values through the CFG for use in branch analysis.
//
//  3. Assignments: Extract type assignments from local declarations,
//     assignments, and function definitions. This captures the flow of
//     types through variables.
//
//  4. Table Mutators: Extract assignments from table mutation operations
//     (table.insert, table.remove, etc.) that modify container types.
//
//  5. Return Classification: Classify return statements for multi-return
//     inference and nil-return detection.
//
//  6. Edge Constraints: Extract type constraints from branch conditions
//     (if, while, for) that narrow types on specific control flow edges.
//
//  7. Call Constraints: Extract OnReturn constraints from function calls
//     that narrow types based on call results (e.g., error returns).
//
//  8. Termination: Mark edges after terminating calls (error(), assert(false))
//     as unreachable with false conditions.
//
// # OUTPUT
//
// The output is a flow.Inputs struct containing all extracted constraints:
//   - DeclaredTypes: Symbol to type mappings
//   - EdgeConditions: Type constraints on CFG edges
//   - ConstValues: Constant value information
//   - PredicateLinks: Call-site predicate connections
//   - ReturnKinds: Return statement classifications
package flowbuild

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/constprop"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/decl"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// coreDecomposer implements flow.TypeDecomposer using query/core functions.
type coreDecomposer struct{}

func (coreDecomposer) ElementType(t typ.Type) typ.Type { return core.ElementType(t) }
func (coreDecomposer) KeyType(t typ.Type) typ.Type     { return core.KeyType(t) }
func (coreDecomposer) ValueType(t typ.Type) typ.Type   { return core.ValueType(t) }

// Run executes the complete flow constraint extraction pipeline.
// It processes the CFG to extract all type constraints that the flow solver
// needs to compute narrowed types at each program point.
//
// The extraction order is designed to build dependencies incrementally:
// declarations first, then assignments, then branch conditions. Each stage
// can use information from previous stages (e.g., const values in branches).
//
// Returns nil if the graph or its CFG is nil.
func Run(fc *fbcore.FlowContext) *flow.Inputs {
	if fc.Graph == nil || fc.Graph.CFG() == nil {
		return nil
	}

	inputs := initInputsFromContext(fc)

	// Declarations: type keys, declared types, module aliases.
	decl.ExtractTypeKeys(fc, inputs)
	decl.ExtractDeclaredTypes(fc, inputs)
	decl.ExtractModuleAliases(fc, inputs)

	// Compute derived resolvers and store in a separate derived bundle.
	derived := &fbcore.Derived{
		SymResolver:           resolve.BuildInputSymbolResolver(fc.CheckCtx, inputs),
		TypeKeyRes:            resolve.BuildContextTypeKeyResolver(fc.CheckCtx),
		RefinementBySym:       resolve.BuildRefinementLookup(fc.CheckCtx),
		CapturedReassignments: cond.CapturedReassignments(fc.Graph),
	}
	if fc.API != nil {
		derived.Synth = fc.API.TypeOf
	}
	fc.Derived = derived

	// Const propagation.
	constprop.CollectConstAssignments(fc, inputs)
	constprop.PropagateAllConstValues(fc, inputs)

	// Assignments with const resolution.
	assign.ExtractAssignments(fc, inputs, keyscoll.BuildKeysCollectorDetector(fc.Graph, fc.ModuleBindings))
	inputs.ClosedMapVars = assign.ClosedMapVars(fc.Graph, inputs)
	inputs.CallAliasRoots = collectCallAliasRoots(fc)
	if bindings := fc.Graph.Bindings(); bindings != nil {
		fresh := bindings.FreshTablePaths()
		capturedFresh := make(map[cfg.SymbolID]map[string]bool, len(fresh))
		captured := make(map[cfg.SymbolID]bool)
		if fn := fc.Graph.Func(); fn != nil {
			for _, sym := range bindings.CapturedSymbols(fn) {
				captured[sym] = true
			}
		}
		for _, nested := range fc.Graph.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			for _, sym := range bindings.CapturedSymbols(nested.Func) {
				captured[sym] = true
			}
		}
		for sym, paths := range fresh {
			if captured[sym] {
				capturedFresh[sym] = paths
			}
		}
		inputs.FreshLocalTablePaths = capturedFresh
	}
	derived.ReceiverRoots, derived.NilableRoots, derived.KnownNonNilPaths = cond.ReceiverRoots(inputs, fc.Graph)

	// Table mutator assignments (table.insert-like).
	mutator.ExtractTableMutatorAssignments(fc, inputs)

	// Container mutator assignments (channel.send-like).
	mutator.ExtractContainerMutatorAssignments(fc, inputs)

	// Function definitions on table fields (function M.add()).
	assign.ExtractFuncDefAssignments(fc, inputs)

	// Return classification.
	returns.ExtractReturnKinds(fc, inputs)

	// Edge constraints from branches.
	cond.ExtractEdgeConstraints(fc, inputs)

	// Call OnReturn constraints, merged into edges.
	callConstraints := cond.ExtractCallOnReturnConstraints(fc, inputs)
	MergeCallConstraintsIntoEdges(inputs, callConstraints)
	cond.ExtractEvaluationConstraints(fc, inputs)

	// Mark terminating call edges as unreachable (error(), etc.).
	for _, p := range fc.Graph.RPO() {
		if fc.Derived == nil {
			continue
		}
		if !cond.PointHasTerminatingCallSite(fc.Graph, p, fc.Derived.Synth, fc.Derived.SymResolver, fc.Derived.RefinementBySym, fc.ModuleBindings) {
			continue
		}
		for _, succ := range fc.Graph.Successors(p) {
			inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{
				From:      p,
				To:        succ,
				Condition: constraint.FalseCondition(),
			})
		}
	}

	// Mark return points with no predecessors as dead.
	markDeadReturns(fc.Graph, inputs)

	// Numeric constraints.
	cond.ExtractNumericConstraints(fc, inputs)

	return inputs
}

func collectCallAliasRoots(fc *fbcore.FlowContext) map[cfg.Point][]cfg.SymbolID {
	if fc == nil {
		return nil
	}
	graph := fc.Graph
	if graph == nil || graph.Bindings() == nil {
		return nil
	}
	bindings := graph.Bindings()
	byPoint := make(map[cfg.Point]map[cfg.SymbolID]bool)
	record := func(p cfg.Point, expr ast.Expr) {
		visitCalls(expr, func(call *ast.FuncCallExpr) {
			// A closed borrow-only callable cannot retain or mutate the
			// argument, so it cannot invalidate facts about its fields.
			if fc.Derived != nil && fc.Derived.Synth != nil {
				if callee := fc.Derived.Synth(call.Func, p); callee != nil {
					if _, single := unwrap.Alias(callee).(*typ.Function); single {
						if row, ok := core.EffectRowOf(callee); ok && row.IsClosed() && row.BorrowsAllParams() && !row.HasStore() && !row.HasMutate() {
							return
						}
					}
				}
			}
			roots := byPoint[p]
			if roots == nil {
				roots = make(map[cfg.SymbolID]bool)
				byPoint[p] = roots
			}
			collectAliasedRoots(call.Receiver, bindings, roots)
			for _, arg := range call.Args {
				collectAliasedRoots(arg, bindings, roots)
			}
		})
	}
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			record(p, info.Call)
		}
	})
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		for _, expr := range info.Sources {
			record(p, expr)
		}
		for _, expr := range info.IterExprs {
			record(p, expr)
		}
		for _, target := range info.Targets {
			record(p, target.Base)
			record(p, target.Key)
		}
	})
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		for _, expr := range info.Exprs {
			record(p, expr)
		}
	})
	graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info != nil {
			record(p, info.Condition)
		}
	})
	result := make(map[cfg.Point][]cfg.SymbolID, len(byPoint))
	for p, roots := range byPoint {
		for sym := range roots {
			result[p] = append(result[p], sym)
		}
		slices.Sort(result[p])
	}
	return result
}

func visitCalls(expr ast.Expr, visit func(*ast.FuncCallExpr)) {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		visit(e)
		visitCalls(e.Func, visit)
		visitCalls(e.Receiver, visit)
		for _, arg := range e.Args {
			visitCalls(arg, visit)
		}
	case *ast.AttrGetExpr:
		visitCalls(e.Object, visit)
		visitCalls(e.Key, visit)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field != nil {
				visitCalls(field.Key, visit)
				visitCalls(field.Value, visit)
			}
		}
	case *ast.LogicalOpExpr:
		visitCalls(e.Lhs, visit)
		visitCalls(e.Rhs, visit)
	case *ast.RelationalOpExpr:
		visitCalls(e.Lhs, visit)
		visitCalls(e.Rhs, visit)
	case *ast.ArithmeticOpExpr:
		visitCalls(e.Lhs, visit)
		visitCalls(e.Rhs, visit)
	case *ast.StringConcatOpExpr:
		visitCalls(e.Lhs, visit)
		visitCalls(e.Rhs, visit)
	case *ast.UnaryNotOpExpr:
		visitCalls(e.Expr, visit)
	case *ast.UnaryMinusOpExpr:
		visitCalls(e.Expr, visit)
	case *ast.UnaryLenOpExpr:
		visitCalls(e.Expr, visit)
	case *ast.UnaryBNotOpExpr:
		visitCalls(e.Expr, visit)
	case *ast.CastExpr:
		visitCalls(e.Expr, visit)
	case *ast.NonNilAssertExpr:
		visitCalls(e.Expr, visit)
	}
}

// Only expressions that can carry the table reference itself are recorded.
// Indexing and scalar operations yield values rather than the source table.
func collectAliasedRoots(expr ast.Expr, bindings *bind.BindingTable, roots map[cfg.SymbolID]bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym, ok := bindings.SymbolOf(e); ok {
			roots[sym] = true
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field != nil {
				collectAliasedRoots(field.Value, bindings, roots)
			}
		}
	case *ast.CastExpr:
		collectAliasedRoots(e.Expr, bindings, roots)
	case *ast.NonNilAssertExpr:
		collectAliasedRoots(e.Expr, bindings, roots)
	case *ast.LogicalOpExpr:
		collectAliasedRoots(e.Lhs, bindings, roots)
		collectAliasedRoots(e.Rhs, bindings, roots)
	}
}

// initInputsFromContext creates and seeds the Inputs struct from FlowContext.
func initInputsFromContext(fc *fbcore.FlowContext) *flow.Inputs {
	initialTypes := make(map[cfg.SymbolID]typ.Type)
	for sym, t := range fc.InitialDeclaredTypes {
		if sym != 0 && t != nil {
			initialTypes[sym] = t
		}
	}

	moduleAliases := make(map[cfg.SymbolID]string)
	for sym, path := range fc.ModuleAliases {
		moduleAliases[sym] = path
	}

	return &flow.Inputs{
		Graph:             fc.Graph,
		Decomposer:        coreDecomposer{},
		DeclaredTypes:     initialTypes,
		ConstValues:       make(map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue),
		TypeKeys:          make(map[uint64]typ.Type),
		ReturnKinds:       make(map[cfg.Point]flow.ReturnKind),
		ReturnConstraints: make(map[cfg.Point]flow.ReturnExprConstraints),
		PredicateLinks:    make(map[string]flow.PredicateLink),
		Facts:             make(map[cfg.Point]constraint.Condition),
		ModuleAliases:     moduleAliases,
		SiblingTypes:      fc.SiblingTypes,
		LiteralTypes:      fc.LiteralTypes,
	}
}

// MergeCallConstraintsIntoEdges merges call OnReturn conditions into edge constraints.
func MergeCallConstraintsIntoEdges(inputs *flow.Inputs, callConstraints map[cond.EdgeKey]constraint.Condition) {
	if len(callConstraints) == 0 {
		return
	}

	keys := make([]cond.EdgeKey, 0, len(callConstraints))
	for key := range callConstraints {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b cond.EdgeKey) int {
		if a.From != b.From {
			return int(a.From) - int(b.From)
		}
		return int(a.To) - int(b.To)
	})
	for _, key := range keys {
		c := callConstraints[key]
		if !c.HasConstraints() {
			continue
		}
		inputs.EdgeConditions = append(inputs.EdgeConditions, flow.EdgeCondition{
			From:      key.From,
			To:        key.To,
			Condition: c,
		})
	}
}

// markDeadReturns marks return points with no predecessors as dead.
func markDeadReturns(graph *cfg.Graph, inputs *flow.Inputs) {
	entry := graph.Entry()
	graph.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		if p == entry {
			return
		}
		if len(graph.Predecessors(p)) == 0 {
			if inputs.DeadPoints == nil {
				inputs.DeadPoints = make(map[cfg.Point]bool)
			}
			inputs.DeadPoints[p] = true
		}
	})
}
