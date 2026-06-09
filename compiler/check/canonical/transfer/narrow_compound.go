package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// narrowByCompound decomposes a short-circuit logical guard (`A and B`, `A or B`)
// the CFG records as a single branch whose Condition is a *ast.LogicalOpExpr. The
// owner of this proof family is transfer: each operand is lowered to the same
// branch-edge narrowing machinery, and existential `or`/`and` edges join only when
// both operands narrow one identical value slot.
func (t *Transfer) narrowByCompound(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	logical, ok := info.Condition.(*ast.LogicalOpExpr)
	if !ok {
		return out, false
	}
	// The whole condition's truthiness on this edge: a CheckTruthy branch is truthy on
	// the taken edge and falsy on the not-taken; a CheckFalsy branch is the inverse.
	wantTruthy := taken
	if info.CondCheck.Kind == cfg.CheckFalsy {
		wantTruthy = !taken
	}
	if logical.Operator == "or" && wantTruthy {
		return t.narrowOrUnionTrueEdgeAtPoint(point, out, logical)
	}
	if logical.Operator == "and" && !wantTruthy {
		return t.narrowAndUnionFalseEdgeAtPoint(point, out, logical)
	}
	operands, decomposable := compoundOperands(logical, wantTruthy)
	if !decomposable {
		return out, false
	}
	state := out
	applied := false
	for _, operand := range operands {
		narrowed, ok := t.narrowOperand(point, state, operand)
		if !ok {
			continue
		}
		state = narrowed
		applied = true
	}
	return state, applied
}

func (t *Transfer) narrowOrUnionTrueEdgeAtPoint(point cfg.Point, out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowSameValueUnion(point, out,
		operandGuard{expr: logical.Lhs, truthy: true},
		operandGuard{expr: logical.Rhs, truthy: true},
	)
}

// narrowAndUnionFalseEdge narrows the FALSE edge of `A and B` when both operands
// constrain the same value. The edge proves `!A OR !B`; joining each operand's
// false-edge refinement gives the least value fact representable by the product
// value domain without path-splitting.
func (t *Transfer) narrowAndUnionFalseEdge(out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowAndUnionFalseEdgeAtPoint(0, out, logical)
}

func (t *Transfer) narrowAndUnionFalseEdgeAtPoint(point cfg.Point, out flow.PointState, logical *ast.LogicalOpExpr) (flow.PointState, bool) {
	return t.narrowSameValueUnion(point, out,
		operandGuard{expr: logical.Lhs, truthy: false},
		operandGuard{expr: logical.Rhs, truthy: false},
	)
}

func (t *Transfer) narrowSameValueUnion(point cfg.Point, out flow.PointState, lhsGuard, rhsGuard operandGuard) (flow.PointState, bool) {
	lhs, lslot, lok := t.operandNarrowsOneSlot(point, out, lhsGuard)
	if !lok {
		return out, false
	}
	rhs, rslot, rok := t.operandNarrowsOneSlot(point, out, rhsGuard)
	if !rok || !lslot.Equal(rslot) {
		return out, false
	}
	lv, lok := t.valueBySlot(lhs, lslot)
	rv, rok := t.valueBySlot(rhs, rslot)
	if !lok || !rok {
		return out, false
	}
	joined := product.Join(lv, rv)
	res := flow.ClonePointStateForEdgeFactEffect(out)
	t.setValueBySlot(&res, lslot, joined, false)
	return res, true
}

// operandNarrowsOneSlot narrows state by one logical operand and reports the
// single value slot it refined. It returns ok=true only when the operand refines
// exactly one slot, so the caller can join two disjuncts that agree on which value
// they constrain.
func (t *Transfer) operandNarrowsOneSlot(point cfg.Point, state flow.PointState, guard operandGuard) (flow.PointState, flow.ValueSlot, bool) {
	narrowed, ok := t.narrowOperand(point, state, guard)
	if !ok {
		return flow.PointState{}, flow.ValueSlot{}, false
	}
	changed, ok := flow.SingleChangedValueSlot(state, narrowed)
	if !ok {
		return flow.PointState{}, flow.ValueSlot{}, false
	}
	return narrowed, changed, true
}

// compoundOperands returns the operands a logical guard narrows on the chosen edge.
func compoundOperands(logical *ast.LogicalOpExpr, wantTruthy bool) ([]operandGuard, bool) {
	switch logical.Operator {
	case "and":
		if !wantTruthy {
			return nil, false
		}
		return []operandGuard{{logical.Lhs, true}, {logical.Rhs, true}}, true
	case "or":
		if wantTruthy {
			return nil, false
		}
		return []operandGuard{{logical.Lhs, false}, {logical.Rhs, false}}, true
	default:
		return nil, false
	}
}

// operandGuard is one operand of a decomposed logical guard paired with the
// truthiness the edge proves for it.
type operandGuard struct {
	expr   ast.Expr
	truthy bool
}

// narrowOperand narrows state by one decomposed logical operand asserted to the
// given truthiness. It classifies the operand the same way the CFG classifies a
// branch condition, then runs the per-edge narrowing machinery on a synthetic
// BranchInfo.
func (t *Transfer) narrowOperand(point cfg.Point, state flow.PointState, og operandGuard) (flow.PointState, bool) {
	condVar, check := extraction.ExtractCondition(og.expr)
	leaf := &cfg.BranchInfo{
		CondVar:    condVar,
		CondSymbol: t.condRootSymbol(og.expr, condVar),
		CondCheck:  check,
		Condition:  og.expr,
	}
	narrowed := t.narrowEdgeInner(point, state, leaf, og.truthy, false)
	if statesEqualForNarrow(narrowed, state) {
		return state, false
	}
	return narrowed, true
}

// condRootSymbol resolves the symbol a leaf condition tests, mirroring CFG branch
// symbol resolution for synthetic compound operands.
func (t *Transfer) condRootSymbol(expr ast.Expr, condVar string) cfg.SymbolID {
	if sym, ok := t.condRootSymbolFromExpr(expr); ok {
		return sym
	}
	return 0
}

func (t *Transfer) condRootSymbolFromExpr(expr ast.Expr) (cfg.SymbolID, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr, *ast.AttrGetExpr:
		return t.staticRootSymbolOfExpr(e)
	case *ast.UnaryNotOpExpr:
		return t.condRootSymbolFromExpr(e.Expr)
	case *ast.RelationalOpExpr:
		if sym, ok := t.relandRootSymbol(e.Lhs); ok {
			return sym, true
		}
		return t.relandRootSymbol(e.Rhs)
	case *ast.FuncCallExpr:
		if e.Method == "" && e.Receiver == nil && len(e.Args) == 1 {
			if fn, ok := e.Func.(*ast.IdentExpr); ok && fn.Value == "type" {
				return t.condRootSymbolFromExpr(e.Args[0])
			}
		}
	}
	return 0, false
}

func (t *Transfer) relandRootSymbol(expr ast.Expr) (cfg.SymbolID, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr, *ast.AttrGetExpr:
		return t.staticRootSymbolOfExpr(e)
	case *ast.FuncCallExpr:
		return t.condRootSymbolFromExpr(e)
	}
	return 0, false
}

// statesEqualForNarrow reports whether a narrowing left the abstract edge state
// unchanged. Compound guards compose every proven fact, not only Env/Cells
// refinements.
func statesEqualForNarrow(a, b flow.PointState) bool {
	return flow.PointStateDomain.Equal(a, b)
}
