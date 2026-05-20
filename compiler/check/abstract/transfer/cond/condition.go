// Package cond implements condition constraint extraction for the flow type system.
// It analyzes branch conditions (if/while/for) to extract type constraints that
// narrow variable types along true and false control flow edges.
//
// # CONDITION MODEL
//
// Branch conditions in Lua (if x then / while x do) produce type constraints:
//   - True branch: constraints satisfied when condition is truthy
//   - False branch: constraints satisfied when condition is falsy
//
// The extractor handles various condition patterns:
//
//	x == nil           -> IsNil{x} / NotNil{x}
//	x ~= nil           -> NotNil{x} / IsNil{x}
//	type(x) == "T"     -> HasType{x, T} / NotHasType{x, T}
//	x and y            -> And(constraints(x), constraints(y))
//	x or y             -> Or(constraints(x), constraints(y))
//	not x              -> negate(constraints(x))
//
// # SIBLING CONSTRAINT PROPAGATION
//
// For error-return patterns like `local ok, err = fn()`, checking one variable
// implies constraints on its sibling:
//
//	if err ~= nil then  -- err is truthy, implies ok is falsy
//	if ok then          -- ok is truthy, implies err is nil
//
// This is handled via SiblingConstraints lookups into the flow.Inputs.
//
// # PREDICATE FUNCTION INTEGRATION
//
// Functions with predicate semantics (returning boolean with type implications)
// are detected via PredicateLink lookups. When a predicate result is used as
// a condition, its OnTruthy/OnFalsy constraints are propagated to branch edges.
//
// # NUMERIC CONSTRAINTS
//
// Numeric comparisons (i < #arr, x >= 0) produce NumericConstraint values that
// enable bounds checking and index safety analysis separate from type narrowing.
package cond

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/literal"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/numconst"
	flowpath "github.com/wippyai/go-lua/compiler/check/abstract/transfer/path"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/predicate"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/resolve"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/sibling"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// BranchConditions holds predicate conditions for true and false branches.
// These are emitted as EdgeConditions to constrain types on control flow edges.
type BranchConditions struct {
	OnTrue  constraint.Condition // Constraints when condition is truthy
	OnFalse constraint.Condition // Constraints when condition is falsy
}

// ConditionExtractor holds shared context for recursive condition extraction.
// It processes AST condition expressions and emits type constraints for both
// true and false control flow edges.
//
// The extractor uses multiple resolution strategies:
//   - Synth: type synthesis for expressions (used for predicate detection)
//   - SymResolver: symbol-to-type resolution (declared and narrowed types)
//   - TypeKeyRes: type name to TypeKey mapping (for type(x) == "T" patterns)
//   - ConstResolver: constant value lookup (for const-folded conditions)
//   - RefinementBySym: function refinement lookup (for predicate/terminating functions)
type ConditionExtractor struct {
	P               cfg.Point                                         // Current CFG point
	SC              *scope.State                                      // Scope state at this point
	Inputs          *flow.Inputs                                      // Flow inputs being built
	Synth           func(ast.Expr, cfg.Point) typ.Type                // Expression type synthesis
	SymResolver     func(cfg.Point, cfg.SymbolID) (typ.Type, bool)    // Symbol type resolution
	TypeKeyRes      func(string, *scope.State) (narrow.TypeKey, bool) // Type name resolution
	ConstResolver   func(string) *flow.ConstValue                     // Constant value lookup
	RefinementBySym constraint.RefinementLookupBySym                  // Function refinement lookup
	ModuleBindings  *bind.BindingTable                                // Module-level fallback bindings for callee resolution
	Evidence        api.FlowEvidence                                  // Canonical graph event trace
}

// constraintsFromBranch extracts type constraints from branch info.
func (ce *ConditionExtractor) ConstraintsFromBranch(info *cfg.BranchInfo) BranchConditions {
	if info == nil {
		return BranchConditions{}
	}

	if info.Condition != nil {
		return ce.ConstraintsFromConditionExpr(info.Condition)
	}

	if info.CondVar == "" {
		return BranchConditions{}
	}

	root := info.CondVar
	if info.CondSymbol != 0 && ce.Inputs != nil {
		root = resolve.RootFromSymbol(ce.Inputs, info.CondSymbol, info.CondVar)
	}
	path := constraint.Path{Root: root, Symbol: info.CondSymbol}
	path = flowpath.WithVersion(path, ce.graph(), ce.P)
	if path.IsEmpty() {
		return BranchConditions{}
	}

	switch info.CondCheck.Kind {
	case cfg.CheckNil:
		return BranchConditions{
			OnTrue:  constraint.FromConstraints(constraint.IsNil{Path: path}),
			OnFalse: constraint.FromConstraints(constraint.NotNil{Path: path}),
		}
	case cfg.CheckNotNil:
		return BranchConditions{
			OnTrue:  constraint.FromConstraints(constraint.NotNil{Path: path}),
			OnFalse: constraint.FromConstraints(constraint.IsNil{Path: path}),
		}
	case cfg.CheckTruthy:
		return BranchConditions{
			OnTrue:  constraint.FromConstraints(constraint.Truthy{Path: path}),
			OnFalse: constraint.FromConstraints(constraint.Falsy{Path: path}),
		}
	case cfg.CheckFalsy:
		return BranchConditions{
			OnTrue:  constraint.FromConstraints(constraint.Falsy{Path: path}),
			OnFalse: constraint.FromConstraints(constraint.Truthy{Path: path}),
		}
	case cfg.CheckTypeEqual:
		if info.CondCheck.TypeName != "" && ce.TypeKeyRes != nil {
			if typeKey, ok := ce.TypeKeyRes(info.CondCheck.TypeName, ce.SC); ok && !typeKey.IsZero() {
				return BranchConditions{
					OnTrue:  constraint.FromConstraints(constraint.HasType{Path: path, Type: typeKey}),
					OnFalse: constraint.FromConstraints(constraint.NotHasType{Path: path, Type: typeKey}),
				}
			}
		}
	case cfg.CheckTypeNot:
		if info.CondCheck.TypeName != "" && ce.TypeKeyRes != nil {
			if typeKey, ok := ce.TypeKeyRes(info.CondCheck.TypeName, ce.SC); ok && !typeKey.IsZero() {
				return BranchConditions{
					OnTrue:  constraint.FromConstraints(constraint.NotHasType{Path: path, Type: typeKey}),
					OnFalse: constraint.FromConstraints(constraint.HasType{Path: path, Type: typeKey}),
				}
			}
		}
	}

	return BranchConditions{}
}

// bindings returns the binding table from inputs.
func (ce *ConditionExtractor) bindings() *bind.BindingTable {
	if ce.Inputs == nil {
		return nil
	}
	return resolve.GetBindings(ce.Inputs)
}

func (ce *ConditionExtractor) graph() interface {
	VisibleVersion(cfg.Point, cfg.SymbolID) cfg.Version
} {
	if ce.Inputs == nil {
		return nil
	}
	return ce.Inputs.Graph
}

func (ce *ConditionExtractor) cfgGraph() *cfg.Graph {
	if ce.Inputs == nil {
		return nil
	}
	graph, _ := ce.Inputs.Graph.(*cfg.Graph)
	return graph
}

// pathFromExpr extracts a path using bindings from inputs.
func (ce *ConditionExtractor) pathFromExpr(expr ast.Expr) constraint.Path {
	return flowpath.FromExprWithBindingsAt(expr, ce.ConstResolver, ce.bindings(), ce.graph(), ce.P)
}

// constraintsFromConditionExpr extracts predicate conditions from a full condition expression.
func (ce *ConditionExtractor) ConstraintsFromConditionExpr(expr ast.Expr) BranchConditions {
	return ce.branchConditionsFromExpr(expr)
}

// conditionFromExpr extracts predicate conditions from an expression (true branch).
func (ce *ConditionExtractor) ConditionFromExpr(expr ast.Expr) constraint.Condition {
	return ce.branchConditionsFromExpr(expr).OnTrue
}

func (ce *ConditionExtractor) branchConditionsFromExpr(expr ast.Expr) BranchConditions {
	switch e := expr.(type) {
	case *ast.TrueExpr:
		return BranchConditions{
			OnTrue:  constraint.TrueCondition(),
			OnFalse: constraint.FalseCondition(),
		}
	case *ast.FalseExpr:
		return BranchConditions{
			OnTrue:  constraint.FalseCondition(),
			OnFalse: constraint.TrueCondition(),
		}
	case *ast.UnaryNotOpExpr:
		inner := ce.branchConditionsFromExpr(e.Expr)
		return BranchConditions{
			OnTrue:  inner.OnFalse,
			OnFalse: inner.OnTrue,
		}

	case *ast.LogicalOpExpr:
		return ce.branchConditionsFromLogicalExpr(e)

	case *ast.RelationalOpExpr:
		return ce.branchConditionsFromRelationalExpr(e)

	case *ast.FuncCallExpr:
		if branches, ok := ce.branchConditionsFromPredicateCall(e); ok {
			return branches
		}
		return BranchConditions{
			OnTrue:  conditionFromOptionalConstraints(ce.constraintsFromCallExpr(e)),
			OnFalse: constraint.TrueCondition(),
		}

	case *ast.IdentExpr:
		if e.Value == "true" {
			return BranchConditions{
				OnTrue:  constraint.TrueCondition(),
				OnFalse: constraint.FalseCondition(),
			}
		}
		if e.Value == "false" {
			return BranchConditions{
				OnTrue:  constraint.FalseCondition(),
				OnFalse: constraint.TrueCondition(),
			}
		}
		return ce.branchConditionsFromIdent(e)

	case *ast.AttrGetExpr:
		path := ce.pathFromExpr(e)
		if path.IsEmpty() {
			return BranchConditions{
				OnTrue:  constraint.TrueCondition(),
				OnFalse: constraint.TrueCondition(),
			}
		}
		result := []constraint.Constraint{constraint.Truthy{Path: path}}
		basePath := ce.pathFromExpr(e.Object)
		if !basePath.IsEmpty() {
			if fieldName := ast.KeyName(e.Key); fieldName != "" {
				cpath := basePath
				result = append(result, constraint.HasField{
					Path:  cpath,
					Field: fieldName,
				})
			}
		}
		return BranchConditions{
			OnTrue:  constraint.FromConstraints(result...),
			OnFalse: constraint.FromConstraints(constraint.Falsy{Path: path}),
		}
	}
	return BranchConditions{
		OnTrue:  constraint.TrueCondition(),
		OnFalse: constraint.TrueCondition(),
	}
}

func (ce *ConditionExtractor) branchConditionsFromLogicalExpr(expr *ast.LogicalOpExpr) BranchConditions {
	if expr == nil {
		return BranchConditions{
			OnTrue:  constraint.TrueCondition(),
			OnFalse: constraint.TrueCondition(),
		}
	}
	left := ce.branchConditionsFromExpr(expr.Lhs)
	right := ce.branchConditionsFromExpr(expr.Rhs)
	switch expr.Operator {
	case "and":
		return BranchConditions{
			OnTrue:  constraint.And(left.OnTrue, right.OnTrue),
			OnFalse: constraint.Or(left.OnFalse, constraint.And(left.OnTrue, right.OnFalse)),
		}
	case "or":
		return BranchConditions{
			OnTrue:  constraint.Or(left.OnTrue, constraint.And(left.OnFalse, right.OnTrue)),
			OnFalse: constraint.And(left.OnFalse, right.OnFalse),
		}
	default:
		return BranchConditions{
			OnTrue:  constraint.TrueCondition(),
			OnFalse: constraint.TrueCondition(),
		}
	}
}

func (ce *ConditionExtractor) branchConditionsFromRelationalExpr(expr *ast.RelationalOpExpr) BranchConditions {
	if expr == nil {
		return BranchConditions{
			OnTrue:  constraint.TrueCondition(),
			OnFalse: constraint.TrueCondition(),
		}
	}
	switch expr.Operator {
	case "==":
		return BranchConditions{
			OnTrue:  ce.ConditionFromEquality(expr.Lhs, expr.Rhs),
			OnFalse: ce.ConditionFromInequality(expr.Lhs, expr.Rhs),
		}
	case "~=":
		return BranchConditions{
			OnTrue:  ce.ConditionFromInequality(expr.Lhs, expr.Rhs),
			OnFalse: ce.ConditionFromEquality(expr.Lhs, expr.Rhs),
		}
	case "<", "<=", ">", ">=":
		return BranchConditions{
			OnTrue:  ce.conditionFromOrderedComparison(expr.Lhs, expr.Rhs),
			OnFalse: constraint.TrueCondition(),
		}
	default:
		return BranchConditions{
			OnTrue:  constraint.TrueCondition(),
			OnFalse: constraint.TrueCondition(),
		}
	}
}

func (ce *ConditionExtractor) branchConditionsFromIdent(ident *ast.IdentExpr) BranchConditions {
	if ident == nil {
		return BranchConditions{
			OnTrue:  constraint.TrueCondition(),
			OnFalse: constraint.TrueCondition(),
		}
	}
	path := ce.pathFromExpr(ident)
	onTrue := constraint.TrueCondition()
	onFalse := constraint.TrueCondition()
	if !path.IsEmpty() {
		onTrue = constraint.FromConstraints(append([]constraint.Constraint{constraint.Truthy{Path: path}},
			versionSiblingConstraints(sibling.ConstraintsForIdent(ident, ce.P, ce.Inputs, true), ce.graph(), ce.P)...)...)
		onFalse = constraint.FromConstraints(append([]constraint.Constraint{constraint.Falsy{Path: path}},
			versionSiblingConstraints(sibling.ConstraintsForIdent(ident, ce.P, ce.Inputs, false), ce.graph(), ce.P)...)...)
	}
	if link := predicate.LookupPredicateLink(ident.Value, ce.Inputs); link != nil {
		if link.OnTruthy.HasConstraints() {
			onTrue = constraint.And(onTrue, link.OnTruthy)
		}
		if link.OnFalsy.HasConstraints() {
			onFalse = constraint.And(onFalse, link.OnFalsy)
		}
	}
	return BranchConditions{OnTrue: onTrue, OnFalse: onFalse}
}

func (ce *ConditionExtractor) branchConditionsFromPredicateCall(expr *ast.FuncCallExpr) (BranchConditions, bool) {
	link := ce.predicateLinkFromCallExpr(expr)
	if link == nil || (!link.OnTruthy.HasConstraints() && !link.OnFalsy.HasConstraints()) {
		return BranchConditions{}, false
	}
	return BranchConditions{
		OnTrue:  conditionOrTrue(link.OnTruthy),
		OnFalse: conditionOrTrue(link.OnFalsy),
	}, true
}

func (ce *ConditionExtractor) predicateLinkFromCallExpr(expr *ast.FuncCallExpr) *flow.PredicateLink {
	if expr == nil {
		return nil
	}
	info := cfg.BuildCallInfo(expr, false)
	if info == nil {
		return nil
	}
	ce.resolveSyntheticCallInfo(info)
	return ExtractPredicateLinkFromCallInfo(info, 0, ce.P, ce.SC, ce.Inputs, ce.TypeKeyRes, ce.Synth, ce.RefinementBySym, ce.SymResolver, ce.cfgGraph(), ce.ModuleBindings)
}

func (ce *ConditionExtractor) resolveSyntheticCallInfo(info *cfg.CallInfo) {
	if info == nil {
		return
	}
	bindings := ce.bindings()
	if bindings != nil {
		if ident, ok := info.Callee.(*ast.IdentExpr); ok {
			if sym, found := bindings.SymbolOf(ident); found {
				info.CalleeSymbol = sym
			}
		}
		if ident, ok := info.Receiver.(*ast.IdentExpr); ok {
			if sym, found := bindings.SymbolOf(ident); found {
				info.ReceiverSymbol = sym
			}
		}
		if len(info.Args) > 0 {
			for i, arg := range info.Args {
				ident, ok := arg.(*ast.IdentExpr)
				if !ok {
					continue
				}
				sym, found := bindings.SymbolOf(ident)
				if !found {
					continue
				}
				if info.ArgSymbols == nil {
					info.ArgSymbols = make([]cfg.SymbolID, len(info.Args))
				}
				info.ArgSymbols[i] = sym
			}
		}
	}
	if info.IsTypeCheck && len(info.Args) > 0 {
		info.TypeCheckPath = ce.pathFromExpr(info.Args[0])
	}
	if info.Method != "" {
		info.CalleePath = ce.pathFromExpr(info.Receiver)
	} else {
		info.CalleePath = ce.pathFromExpr(info.Callee)
	}
	if info.CalleeSymbol == 0 && !info.CalleePath.IsEmpty() && len(info.CalleePath.Segments) == 0 {
		info.CalleeSymbol = info.CalleePath.Symbol
	}
}

func conditionFromOptionalConstraints(items []constraint.Constraint) constraint.Condition {
	if len(items) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromConstraints(items...)
}

func conditionOrTrue(c constraint.Condition) constraint.Condition {
	if c.HasConstraints() {
		return c
	}
	return constraint.TrueCondition()
}

func (ce *ConditionExtractor) conditionFromOrderedComparison(lhs, rhs ast.Expr) constraint.Condition {
	var out []constraint.Constraint
	if path := ce.pathFromExpr(lhs); !path.IsEmpty() {
		if key, ok := ce.typeKeyFromOrderedOperand(rhs); ok {
			out = append(out, constraint.HasType{Path: path, Type: key})
		}
	}
	if path := ce.pathFromExpr(rhs); !path.IsEmpty() {
		if key, ok := ce.typeKeyFromOrderedOperand(lhs); ok {
			out = append(out, constraint.HasType{Path: path, Type: key})
		}
	}
	if len(out) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromConstraints(out...)
}

func (ce *ConditionExtractor) typeKeyFromOrderedOperand(expr ast.Expr) (narrow.TypeKey, bool) {
	lit, ok := ce.literalFromExpr(expr)
	if !ok || lit == nil {
		return narrow.TypeKey{}, false
	}
	switch lit.Base {
	case kind.Integer, kind.Number:
		return narrow.BuiltinTypeKey("number"), true
	case kind.String:
		return narrow.BuiltinTypeKey("string"), true
	default:
		return narrow.TypeKey{}, false
	}
}

// conditionFromEquality handles == comparisons.
func (ce *ConditionExtractor) ConditionFromEquality(lhs, rhs ast.Expr) constraint.Condition {
	// type(x) == "string"
	if path, ok := ce.typePredicatePath(lhs); ok {
		if key, ok := typeKeyFromStringExpr(rhs); ok {
			return constraint.FromConstraints(constraint.HasType{Path: path, Type: key})
		}
	}
	if path, ok := ce.typePredicatePath(rhs); ok {
		if key, ok := typeKeyFromStringExpr(lhs); ok {
			return constraint.FromConstraints(constraint.HasType{Path: path, Type: key})
		}
	}

	// x[y] == literal (dynamic index with literal value, including const-resolved identifiers)
	if lit, ok := ce.literalFromExpr(rhs); ok && lit != nil {
		if c := ce.constraintsFromDynamicIndexLiteral(lhs, lit); c != nil {
			return constraint.FromConstraints(c...)
		}
	}
	if lit, ok := ce.literalFromExpr(lhs); ok && lit != nil {
		if c := ce.constraintsFromDynamicIndexLiteral(rhs, lit); c != nil {
			return constraint.FromConstraints(c...)
		}
	}

	// x[y] == z (dynamic index with path value)
	if c := ce.constraintsFromDynamicIndexPath(lhs, rhs, true); c != nil {
		return constraint.FromConstraints(c...)
	}
	if c := ce.constraintsFromDynamicIndexPath(rhs, lhs, true); c != nil {
		return constraint.FromConstraints(c...)
	}

	// x == literal (including const-resolved identifiers and literal-typed variables)
	if lit, ok := ce.literalFromExpr(lhs); ok && lit != nil {
		return constraint.FromConstraints(ce.constraintsFromPathLiteral(rhs, lit)...)
	}
	if lit, ok := ce.literalFromExpr(rhs); ok && lit != nil {
		return constraint.FromConstraints(ce.constraintsFromPathLiteral(lhs, lit)...)
	}

	// x == nil
	if literal.IsNilExpr(lhs) {
		if ident, ok := rhs.(*ast.IdentExpr); ok {
			if link := predicate.LookupPredicateLink(ident.Value, ce.Inputs); link != nil && link.OnFalsy.HasConstraints() {
				path := ce.pathFromExpr(rhs)
				if path.IsEmpty() {
					return link.OnFalsy
				}
				return constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.IsNil{Path: path}))
			}
			path := ce.pathFromExpr(rhs)
			if !path.IsEmpty() {
				result := []constraint.Constraint{constraint.IsNil{Path: path}}
				result = append(result, versionSiblingConstraints(sibling.ConstraintsForIdent(ident, ce.P, ce.Inputs, false), ce.graph(), ce.P)...)
				return constraint.FromConstraints(result...)
			}
		}
		if path := ce.pathFromExpr(rhs); !path.IsEmpty() {
			return constraint.FromConstraints(constraint.IsNil{Path: path})
		}
	}
	if literal.IsNilExpr(rhs) {
		if ident, ok := lhs.(*ast.IdentExpr); ok {
			if link := predicate.LookupPredicateLink(ident.Value, ce.Inputs); link != nil && link.OnFalsy.HasConstraints() {
				path := ce.pathFromExpr(lhs)
				if path.IsEmpty() {
					return link.OnFalsy
				}
				return constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.IsNil{Path: path}))
			}
			path := ce.pathFromExpr(lhs)
			if !path.IsEmpty() {
				result := []constraint.Constraint{constraint.IsNil{Path: path}}
				result = append(result, versionSiblingConstraints(sibling.ConstraintsForIdent(ident, ce.P, ce.Inputs, false), ce.graph(), ce.P)...)
				return constraint.FromConstraints(result...)
			}
		}
		if path := ce.pathFromExpr(lhs); !path.IsEmpty() {
			return constraint.FromConstraints(constraint.IsNil{Path: path})
		}
	}

	// x == y (path equality)
	left := ce.pathFromExpr(lhs)
	right := ce.pathFromExpr(rhs)
	if left.IsEmpty() || right.IsEmpty() {
		return constraint.TrueCondition()
	}
	if target, field, ok := constraint.SplitFieldPath(left); ok {
		return constraint.FromConstraints(constraint.FieldEqualsPath{Target: target, Field: field, Value: right})
	}
	if target, field, ok := constraint.SplitFieldPath(right); ok {
		return constraint.FromConstraints(constraint.FieldEqualsPath{Target: target, Field: field, Value: left})
	}
	if target, key, ok := flowpath.SplitIndexPath(left); ok {
		return constraint.FromConstraints(constraint.IndexEqualsPath{Target: target, Key: key, Value: right})
	}
	if target, key, ok := flowpath.SplitIndexPath(right); ok {
		return constraint.FromConstraints(constraint.IndexEqualsPath{Target: target, Key: key, Value: left})
	}
	return constraint.FromConstraints(constraint.NewEqPath(left, right))
}

func (ce *ConditionExtractor) typePredicatePath(expr ast.Expr) (constraint.Path, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil {
		return constraint.Path{}, false
	}
	if checkcallsite.IsMethodLikeExpr(call) {
		return constraint.Path{}, false
	}
	if len(call.Args) != 1 {
		return constraint.Path{}, false
	}
	if !ce.calleeHasEffect(call, effect.Row.HasTypePredicate) {
		return constraint.Path{}, false
	}
	path := ce.pathFromExpr(call.Args[0])
	if path.IsEmpty() {
		return constraint.Path{}, false
	}
	return path, true
}

func (ce *ConditionExtractor) calleeHasEffect(call *ast.FuncCallExpr, want func(effect.Row) bool) bool {
	if call == nil {
		return false
	}
	// Try refinement lookup by symbol.
	if ident, ok := call.Func.(*ast.IdentExpr); ok {
		if bindings := ce.bindings(); bindings != nil && ce.RefinementBySym != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if eff := ce.RefinementBySym(sym); eff != nil {
					if row, ok := eff.Row.(effect.Row); ok && want(row) {
						return true
					}
				}
			}
		}
	}
	// Check for metatype callable.
	if ident, ok := call.Func.(*ast.IdentExpr); ok && ce.SC != nil {
		if meta := ce.SC.MetaForName(ident.Value); meta != nil {
			fn := typ.Func().
				Param("value", typ.Any).
				Returns(meta.Of).
				Effects(effect.WithCallableType()).
				Build()
			if row, ok := fn.Effects.(effect.Row); ok && want(row) {
				return true
			}
		}
	}
	// Fall back to extracting effect from synthesized type.
	if ce.Synth != nil {
		if t := ce.Synth(call.Func, ce.P); t != nil {
			if row, ok := effectRowFromType(t); ok {
				return want(row)
			}
		}
	}
	return false
}

func effectRowFromType(t typ.Type) (effect.Row, bool) {
	if t == nil {
		return effect.Row{}, false
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Function:
		row, ok := v.Effects.(effect.Row)
		return row, ok
	case *typ.Optional:
		return effectRowFromType(v.Inner)
	case *typ.Union:
		var merged effect.Row
		for _, m := range v.Members {
			if row, ok := effectRowFromType(m); ok {
				merged = merged.With(row.Labels...)
			}
		}
		if len(merged.Labels) > 0 {
			return merged, true
		}
		return effect.Row{}, false
	case *typ.Intersection:
		var merged effect.Row
		for _, m := range v.Members {
			if row, ok := effectRowFromType(m); ok {
				merged = merged.With(row.Labels...)
			}
		}
		if len(merged.Labels) > 0 {
			return merged, true
		}
		return effect.Row{}, false
	case *typ.Instantiated:
		if resolved, err := core.ResolveInstantiated(v); err == nil {
			return effectRowFromType(resolved)
		}
	}
	return effect.Row{}, false
}

// conditionFromInequality handles ~= comparisons.
func (ce *ConditionExtractor) ConditionFromInequality(lhs, rhs ast.Expr) constraint.Condition {
	// x ~= nil
	if literal.IsNilExpr(lhs) {
		if ident, ok := rhs.(*ast.IdentExpr); ok {
			if link := predicate.LookupPredicateLink(ident.Value, ce.Inputs); link != nil && link.OnTruthy.HasConstraints() {
				path := ce.pathFromExpr(rhs)
				if path.IsEmpty() {
					return link.OnTruthy
				}
				return constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.NotNil{Path: path}))
			}
			path := ce.pathFromExpr(rhs)
			if !path.IsEmpty() {
				result := []constraint.Constraint{constraint.NotNil{Path: path}}
				result = append(result, versionSiblingConstraints(sibling.ConstraintsForIdent(ident, ce.P, ce.Inputs, true), ce.graph(), ce.P)...)
				return constraint.FromConstraints(result...)
			}
		}
		if path := ce.pathFromExpr(rhs); !path.IsEmpty() {
			return constraint.FromConstraints(constraint.NotNil{Path: path})
		}
	}
	if literal.IsNilExpr(rhs) {
		if ident, ok := lhs.(*ast.IdentExpr); ok {
			if link := predicate.LookupPredicateLink(ident.Value, ce.Inputs); link != nil && link.OnTruthy.HasConstraints() {
				path := ce.pathFromExpr(lhs)
				if path.IsEmpty() {
					return link.OnTruthy
				}
				return constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.NotNil{Path: path}))
			}
			path := ce.pathFromExpr(lhs)
			if !path.IsEmpty() {
				result := []constraint.Constraint{constraint.NotNil{Path: path}}
				result = append(result, versionSiblingConstraints(sibling.ConstraintsForIdent(ident, ce.P, ce.Inputs, true), ce.graph(), ce.P)...)
				return constraint.FromConstraints(result...)
			}
		}
		if path := ce.pathFromExpr(lhs); !path.IsEmpty() {
			return constraint.FromConstraints(constraint.NotNil{Path: path})
		}
	}

	// x[y] ~= literal (dynamic index, including const-resolved identifiers)
	if lit, ok := ce.literalFromExpr(rhs); ok && lit != nil {
		if c := ce.constraintsFromDynamicIndexLiteral(lhs, lit); c != nil {
			return constraint.FromConstraints(numconst.NegateConstraints(c)...)
		}
	}
	if lit, ok := ce.literalFromExpr(lhs); ok && lit != nil {
		if c := ce.constraintsFromDynamicIndexLiteral(rhs, lit); c != nil {
			return constraint.FromConstraints(numconst.NegateConstraints(c)...)
		}
	}

	// x[y] ~= z (dynamic index with path value)
	if c := ce.constraintsFromDynamicIndexPath(lhs, rhs, false); c != nil {
		return constraint.FromConstraints(c...)
	}
	if c := ce.constraintsFromDynamicIndexPath(rhs, lhs, false); c != nil {
		return constraint.FromConstraints(c...)
	}

	// Fallback: negate equality constraints
	eq := ce.ConditionFromEquality(lhs, rhs)
	if !eq.HasConstraints() {
		return constraint.TrueCondition()
	}
	return constraint.Not(eq)
}

// constraintsFromPathLiteral handles path == literal for static paths.
func (ce *ConditionExtractor) constraintsFromPathLiteral(expr ast.Expr, lit *typ.Literal) []constraint.Constraint {
	if lit == nil {
		return nil
	}
	path := ce.pathFromExpr(expr)
	if path.IsEmpty() {
		return nil
	}
	if target, field, ok := constraint.SplitFieldPath(path); ok {
		return []constraint.Constraint{constraint.FieldEquals{Target: target, Field: field, Value: lit}}
	}
	if target, key, ok := flowpath.SplitIndexPath(path); ok {
		return []constraint.Constraint{constraint.IndexEquals{Target: target, Key: key, Value: lit}}
	}
	// Boolean literal equality on a path must remain portable across function
	// boundaries (effects are applied in callers with different TypeKeys maps).
	// Encode `x == false` as falsy + non-nil and `x == true` as truthy + boolean.
	if lit.Base == kind.Boolean {
		if b, ok := lit.Value.(bool); ok {
			if b {
				return []constraint.Constraint{
					constraint.Truthy{Path: path},
					constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("boolean")},
				}
			}
			return []constraint.Constraint{
				constraint.Falsy{Path: path},
				constraint.NotNil{Path: path},
			}
		}
	}
	if typeKey, ok := ce.literalTypeKey(lit); ok {
		return []constraint.Constraint{constraint.HasType{Path: path, Type: typeKey}}
	}
	return nil
}

func (ce *ConditionExtractor) literalTypeKey(lit *typ.Literal) (narrow.TypeKey, bool) {
	if lit == nil {
		return narrow.TypeKey{}, false
	}
	hash := lit.Hash()
	if hash == 0 {
		return narrow.TypeKey{}, false
	}
	if ce.Inputs != nil {
		if ce.Inputs.TypeKeys == nil {
			ce.Inputs.TypeKeys = make(map[uint64]typ.Type)
		}
		if _, exists := ce.Inputs.TypeKeys[hash]; !exists {
			ce.Inputs.TypeKeys[hash] = lit
		}
	}
	return narrow.HashTypeKey(hash), true
}

// constraintsFromCallExpr handles function call expressions.
func (ce *ConditionExtractor) constraintsFromCallExpr(expr *ast.FuncCallExpr) []constraint.Constraint {
	if expr == nil {
		return nil
	}

	if checkcallsite.IsMethodLikeExpr(expr) {
		return nil
	}
	if len(expr.Args) == 0 {
		return nil
	}

	if constraints := ce.constraintsFromCallableTypeCallExpr(expr); len(constraints) > 0 {
		return constraints
	}

	if path := ce.pathFromExpr(expr.Args[0]); !path.IsEmpty() {
		if typeKey, ok := ce.localTypePredicateKey(expr); ok {
			return []constraint.Constraint{
				constraint.NotNil{Path: path},
				constraint.HasType{Path: path, Type: typeKey},
			}
		}
	}

	return nil
}

func (ce *ConditionExtractor) localTypePredicateKey(call *ast.FuncCallExpr) (narrow.TypeKey, bool) {
	if call == nil {
		return narrow.TypeKey{}, false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident == nil {
		return narrow.TypeKey{}, false
	}
	bindings := ce.bindings()
	if bindings == nil {
		return narrow.TypeKey{}, false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return narrow.TypeKey{}, false
	}
	for _, pred := range ce.Evidence.LocalTypePredicates {
		if pred.Symbol != sym || pred.ParamIndex != 0 || pred.Kind == "" {
			continue
		}
		key := narrow.BuiltinTypeKey(pred.Kind)
		if key.IsZero() {
			continue
		}
		return key, true
	}
	return narrow.TypeKey{}, false
}

func (ce *ConditionExtractor) constraintsFromCallableTypeCallExpr(expr *ast.FuncCallExpr) []constraint.Constraint {
	if expr == nil || checkcallsite.IsMethodLikeExpr(expr) || len(expr.Args) == 0 || ce.TypeKeyRes == nil {
		return nil
	}
	fnIdent, ok := expr.Func.(*ast.IdentExpr)
	if !ok {
		return nil
	}
	if !ce.calleeHasEffect(expr, effect.Row.HasCallableType) {
		return nil
	}
	typeKey, ok := ce.TypeKeyRes(fnIdent.Value, ce.SC)
	if !ok || typeKey.IsZero() {
		return nil
	}
	path := ce.pathFromExpr(expr.Args[0])
	if path.IsEmpty() {
		return nil
	}
	return []constraint.Constraint{constraint.HasType{Path: path, Type: typeKey}}
}

// literalFromExpr resolves a literal from an expression using the extractor's context.
func (ce *ConditionExtractor) literalFromExpr(expr ast.Expr) (*typ.Literal, bool) {
	return literal.FromExprWithSymType(expr, ce.ConstResolver, ce.bindings(), ce.SymResolver, ce.P)
}

// typeKeyFromStringExpr extracts a type key from a string literal.
func typeKeyFromStringExpr(expr ast.Expr) (narrow.TypeKey, bool) {
	s, ok := expr.(*ast.StringExpr)
	if !ok {
		return narrow.TypeKey{}, false
	}
	return narrow.KnownBuiltinTypeKey(s.Value)
}

// NumericConstraintsFromExpr extracts numeric constraints from an expression.
func NumericConstraintsFromExpr(expr ast.Expr, p cfg.Point, inputs *flow.Inputs) []constraint.NumericConstraint {
	bindings := resolve.GetBindings(inputs)
	return numericConstraintsFromExprInternal(expr, p, inputs, bindings)
}

func numericConstraintsFromExprInternal(expr ast.Expr, p cfg.Point, inputs *flow.Inputs, bindings *bind.BindingTable) []constraint.NumericConstraint {
	switch e := expr.(type) {
	case *ast.UnaryNotOpExpr:
		inner := numericConstraintsFromExprInternal(e.Expr, p, inputs, bindings)
		var negated []constraint.NumericConstraint
		for _, nc := range inner {
			if neg := numconst.NegateNumericConstraint(nc); neg != nil {
				negated = append(negated, neg)
			}
		}
		return negated
	case *ast.LogicalOpExpr:
		return numericConstraintsFromLogicalExprInternal(e, p, inputs, bindings)
	case *ast.RelationalOpExpr:
		if nc := numconst.NumericConstraintFromComparisonWithBindings(e.Operator, e.Lhs, e.Rhs, p, inputs, bindings); nc != nil {
			return []constraint.NumericConstraint{nc}
		}
	}
	return nil
}

func numericConstraintsFromLogicalExprInternal(expr *ast.LogicalOpExpr, p cfg.Point, inputs *flow.Inputs, bindings *bind.BindingTable) []constraint.NumericConstraint {
	switch expr.Operator {
	case "and":
		left := numericConstraintsFromExprInternal(expr.Lhs, p, inputs, bindings)
		right := numericConstraintsFromExprInternal(expr.Rhs, p, inputs, bindings)
		if len(left) == 0 {
			return right
		}
		if len(right) == 0 {
			return left
		}
		out := make([]constraint.NumericConstraint, 0, len(left)+len(right))
		out = append(out, left...)
		out = append(out, right...)
		return out
	}
	return nil
}

// ExtractReturnExprConstraints extracts constraints from a return expression.
func ExtractReturnExprConstraints(expr ast.Expr, p cfg.Point, sc *scope.State, inputs *flow.Inputs, evidence api.FlowEvidence, typeKeyResolver func(string, *scope.State) (narrow.TypeKey, bool), synthFunc func(ast.Expr, cfg.Point) typ.Type, constResolver func(string) *flow.ConstValue, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool)) flow.ReturnExprConstraints {
	if ident, ok := expr.(*ast.IdentExpr); ok {
		link := predicate.LookupPredicateLink(ident.Value, inputs)
		if link == nil || (!link.OnTruthy.HasConstraints() && !link.OnFalsy.HasConstraints()) {
			return flow.ReturnExprConstraints{}
		}
	}

	ce := &ConditionExtractor{
		P: p, SC: sc, Inputs: inputs,
		Synth:         synthFunc,
		SymResolver:   symResolver,
		TypeKeyRes:    typeKeyResolver,
		ConstResolver: constResolver,
		Evidence:      evidence,
	}
	branches := ce.ConstraintsFromConditionExpr(expr)
	var onReturn constraint.Condition
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		onReturn = conditionFromOptionalConstraints(ce.constraintsFromCallableTypeCallExpr(call))
	}
	if !onReturn.HasConstraints() && !branches.OnTrue.HasConstraints() && !branches.OnFalse.HasConstraints() {
		return flow.ReturnExprConstraints{}
	}

	return flow.ReturnExprConstraints{OnReturn: onReturn, OnTrue: branches.OnTrue, OnFalse: branches.OnFalse}
}

// ChannelValueConstraint emits a HasType constraint for result.value when
// detecting the pattern result.channel == ch where ch is a channel type.
func ChannelValueConstraint(target constraint.Path, channelExpr ast.Expr, p cfg.Point, bindings *bind.BindingTable, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool), inputs *flow.Inputs) constraint.Constraint {
	if bindings == nil || symResolver == nil {
		return nil
	}

	ident, ok := channelExpr.(*ast.IdentExpr)
	if !ok {
		return nil
	}

	sym, found := bindings.SymbolOf(ident)
	if !found || sym == 0 {
		return nil
	}

	channelType, ok := symResolver(p, sym)
	if !ok || channelType == nil {
		return nil
	}

	elemType := ExtractChannelElementType(channelType)
	if elemType == nil {
		return nil
	}

	valuePath := constraint.Path{
		Root:   target.Root,
		Symbol: target.Symbol,
		Segments: append(
			append([]constraint.Segment{}, target.Segments...),
			constraint.Segment{Kind: constraint.SegmentField, Name: "value"},
		),
	}

	if inputs != nil && inputs.TypeKeys != nil {
		h := elemType.Hash()
		if h != 0 {
			inputs.TypeKeys[h] = elemType
		}
	}

	return constraint.HasType{
		Path: valuePath,
		Type: narrow.HashTypeKey(elemType.Hash()),
	}
}

// ExtractChannelElementType extracts T from Channel<T> or similar generic types.
func ExtractChannelElementType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}

	if alias, ok := t.(*typ.Alias); ok {
		return ExtractChannelElementType(alias.Target)
	}

	if inst, ok := t.(*typ.Instantiated); ok {
		if len(inst.TypeArgs) > 0 {
			return inst.TypeArgs[0]
		}
	}

	return nil
}

// constraintsFromDynamicIndexLiteral handles x[y] == literal where y is a variable.
func (ce *ConditionExtractor) constraintsFromDynamicIndexLiteral(lhs ast.Expr, lit *typ.Literal) []constraint.Constraint {
	bindings := ce.bindings()
	if bindings == nil || ce.SymResolver == nil || lit == nil {
		return nil
	}

	attr, ok := lhs.(*ast.AttrGetExpr)
	if !ok {
		return nil
	}

	basePath := ce.pathFromExpr(attr.Object)
	if basePath.IsEmpty() {
		return nil
	}

	keyIdent, ok := attr.Key.(*ast.IdentExpr)
	if !ok {
		return nil
	}

	keySym, found := bindings.SymbolOf(keyIdent)
	if !found || keySym == 0 {
		return nil
	}

	keyType, ok := ce.SymResolver(ce.P, keySym)
	if !ok || keyType == nil {
		return nil
	}

	return EmitIndexEqualsLiteral(basePath, keyType, lit)
}

// constraintsFromDynamicIndexPath handles x[y] == z where y is a variable and z is a path.
func (ce *ConditionExtractor) constraintsFromDynamicIndexPath(lhs, rhs ast.Expr, isEquals bool) []constraint.Constraint {
	bindings := ce.bindings()
	if bindings == nil || ce.SymResolver == nil {
		return nil
	}

	attr, ok := lhs.(*ast.AttrGetExpr)
	if !ok {
		return nil
	}

	basePath := ce.pathFromExpr(attr.Object)
	if basePath.IsEmpty() {
		return nil
	}

	valuePath := ce.pathFromExpr(rhs)
	if valuePath.IsEmpty() {
		return nil
	}

	keyIdent, ok := attr.Key.(*ast.IdentExpr)
	if !ok {
		return nil
	}

	keySym, found := bindings.SymbolOf(keyIdent)
	if !found || keySym == 0 {
		return nil
	}

	keyType, ok := ce.SymResolver(ce.P, keySym)
	if !ok || keyType == nil {
		return nil
	}

	return EmitIndexEqualsPath(basePath, keyType, valuePath, isEquals)
}

// EmitIndexEqualsLiteral creates IndexEquals constraints for the given key type.
func EmitIndexEqualsLiteral(target constraint.Path, keyType typ.Type, value *typ.Literal) []constraint.Constraint {
	if lit, ok := keyType.(*typ.Literal); ok {
		return []constraint.Constraint{constraint.IndexEquals{Target: target, Key: lit, Value: value}}
	}

	if union, ok := keyType.(*typ.Union); ok {
		var constraints []constraint.Constraint
		for _, member := range union.Members {
			if lit, ok := member.(*typ.Literal); ok {
				constraints = append(constraints, constraint.IndexEquals{Target: target, Key: lit, Value: value})
			}
		}
		if len(constraints) > 0 {
			return constraints
		}
	}

	return []constraint.Constraint{constraint.IndexEquals{Target: target, Key: keyType, Value: value}}
}

// EmitIndexEqualsPath creates IndexEqualsPath or IndexNotEqualsPath constraints.
func EmitIndexEqualsPath(target constraint.Path, keyType typ.Type, valuePath constraint.Path, isEquals bool) []constraint.Constraint {
	if lit, ok := keyType.(*typ.Literal); ok {
		if isEquals {
			return []constraint.Constraint{constraint.IndexEqualsPath{Target: target, Key: lit, Value: valuePath}}
		}
		return []constraint.Constraint{constraint.IndexNotEqualsPath{Target: target, Key: lit, Value: valuePath}}
	}

	if union, ok := keyType.(*typ.Union); ok {
		var constraints []constraint.Constraint
		for _, member := range union.Members {
			if lit, ok := member.(*typ.Literal); ok {
				if isEquals {
					constraints = append(constraints, constraint.IndexEqualsPath{Target: target, Key: lit, Value: valuePath})
				} else {
					constraints = append(constraints, constraint.IndexNotEqualsPath{Target: target, Key: lit, Value: valuePath})
				}
			}
		}
		if len(constraints) > 0 {
			return constraints
		}
	}

	if isEquals {
		return []constraint.Constraint{constraint.IndexEqualsPath{Target: target, Key: keyType, Value: valuePath}}
	}
	return []constraint.Constraint{constraint.IndexNotEqualsPath{Target: target, Key: keyType, Value: valuePath}}
}

func versionSiblingConstraints(constraints []constraint.Constraint, graph interface {
	VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version
}, p cfg.Point) []constraint.Constraint {
	if len(constraints) == 0 || graph == nil {
		return constraints
	}
	out := make([]constraint.Constraint, 0, len(constraints))
	for _, c := range constraints {
		switch v := c.(type) {
		case constraint.IsNil:
			v.Path = flowpath.WithVersion(v.Path, graph, p)
			out = append(out, v)
		case constraint.NotNil:
			v.Path = flowpath.WithVersion(v.Path, graph, p)
			out = append(out, v)
		case constraint.Truthy:
			v.Path = flowpath.WithVersion(v.Path, graph, p)
			out = append(out, v)
		case constraint.Falsy:
			v.Path = flowpath.WithVersion(v.Path, graph, p)
			out = append(out, v)
		default:
			out = append(out, c)
		}
	}
	return out
}
