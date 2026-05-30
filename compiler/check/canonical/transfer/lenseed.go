package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// lenseed.go carries the array-length seeding and the in-bounds index-read
// refinement: the wiring that recovers a non-optional element type for a
// provably in-range sequence read. The length facts live only in the numeric
// component (out.Num), which the loop-header widen bounds, so seeding cannot
// affect fixpoint convergence; the refinement is a pure read of the converged
// numeric state and emits no new facts.

// arrayLiteralArity counts the positional (sequential, nil-key) elements of a
// table constructor, the runtime length of an array literal `{1, 2, 3}`. A
// keyed field (`{x = 1}`) or a dynamic-keyed field does not contribute to the
// positional length, so a literal mixing positional and keyed parts reports
// only the positional prefix count. A non-positional literal yields 0, so no
// length floor is seeded.
func arrayLiteralArity(e *ast.TableExpr) int64 {
	if e == nil {
		return 0
	}
	var n int64
	for _, field := range e.Fields {
		if field == nil {
			continue
		}
		if field.Key != nil {
			// A keyed entry breaks the positional sequence; Lua only borders the
			// array part at the first hole, so stop counting at the first key.
			continue
		}
		n++
	}
	return n
}

// seedArrayLiteralLength seeds the length floor of a slot bound from an array
// literal: `local arr = {1, 2, 3}` proves #arr >= 3, so a literal/length index
// within that floor reads the non-optional element. A binding from any other
// source drops a prior length floor (soundness: the new value's length is
// unknown), so a slot reused across assignments never carries a stale floor.
func (t *Transfer) seedArrayLiteralLength(out *flow.PointState, key string, src ast.Expr) {
	if out.Num == nil {
		return
	}
	arrKey := constraint.PathKey(key)
	tbl, ok := src.(*ast.TableExpr)
	if !ok {
		out.Num.DropLenBound(arrKey)
		return
	}
	out.Num.DropLenBound(arrKey)
	if n := arrayLiteralArity(tbl); n > 0 {
		out.Num.ApplyLenGeConst(arrKey, n)
	}
}

// applyIndexWriteLength updates the length floor of an indexed write `base[k]=v`.
// An append at the border (`arr[#arr + c]` on the SAME base, c >= 1) raises the
// floor to the prior floor plus c: writing past the current length extends the
// sequence. Any other index write the transfer cannot prove is a same-base border
// append drops the floor (soundness default: the write may target a hole or an
// index the proven floor does not cover, so the prior floor no longer holds).
func (t *Transfer) applyIndexWriteLength(out *flow.PointState, target cfg.AssignTarget, baseKey string) {
	if out.Num == nil {
		return
	}
	arrKey := constraint.PathKey(baseKey)
	lenIdent, offset, ok := t.lengthIndexOffset(target.Key)
	if !ok || offset < 1 {
		out.Num.DropLenBound(arrKey)
		return
	}
	if lenSym := t.symbolOf(lenIdent); lenSym == 0 || constraint.PathKey(symKey(lenSym)) != arrKey {
		out.Num.DropLenBound(arrKey)
		return
	}
	if lower, _, ok := out.Num.LenBoundsFor(arrKey); ok {
		out.Num.ApplyLenGeConst(arrKey, lower+offset)
	} else {
		out.Num.ApplyLenGeConst(arrKey, offset)
	}
}

// positionalTupleLiteral types a pure positional table literal (`{1, 2, 3}`) as a
// fixed-arity tuple of its element types. It applies only when every field is
// positional (nil key) and each element type resolves to a concrete type, the
// shape whose static arity proves an in-range index read present. A literal with
// any keyed field, no fields, or an unresolved element reports ok=false so
// evalTable keeps the record over-approximation.
func (t *Transfer) positionalTupleLiteral(
	out *flow.PointState,
	e *ast.TableExpr,
	demand func(int, paramevidence.ParamContract),
) (product.AbstractValue, bool) {
	if e == nil || len(e.Fields) == 0 {
		return product.AbstractValue{}, false
	}
	elems := make([]typ.Type, 0, len(e.Fields))
	for _, field := range e.Fields {
		if field == nil || field.Key != nil {
			return product.AbstractValue{}, false
		}
		fv, ok := t.evalExpr(out, field.Value, demand)
		if !ok || fv.IsZero() {
			return product.AbstractValue{}, false
		}
		ft := fv.ProjectValue()
		if ft == nil || typ.IsAbsentOrUnknown(ft) {
			return product.AbstractValue{}, false
		}
		elems = append(elems, ft)
	}
	return product.FromType(typ.NewTuple(elems...)), true
}

// lenExprBase reports whether expr is `#base` over an identifier and that
// identifier expression, the operand the length-seeding and the in-bounds
// length-index refinement key on.
func lenExprBase(expr ast.Expr) (*ast.IdentExpr, bool) {
	lenOp, ok := expr.(*ast.UnaryLenOpExpr)
	if !ok {
		return nil, false
	}
	ident, ok := lenOp.Expr.(*ast.IdentExpr)
	if !ok {
		return nil, false
	}
	return ident, true
}

// containerExprKey returns the numeric-component path key for a sequence
// container expression. A bare identifier keys on its symbol; a static field
// path rooted at an identifier (`saga.compensations`) keys on the root symbol
// plus its field-segment suffix. The key is internal to the transfer package's
// numeric component (it never crosses the package boundary), so any shape the
// seeding and the read both derive identically is sufficient; a non-static or
// non-identifier-rooted base reports ok=false so no length fact is keyed.
func (t *Transfer) containerExprKey(expr ast.Expr) (constraint.PathKey, bool) {
	switch obj := expr.(type) {
	case *ast.IdentExpr:
		if sym := t.symbolOf(obj); sym != 0 {
			return constraint.PathKey(symKey(sym)), true
		}
	case *ast.AttrGetExpr:
		segs := staticAttrPath(obj)
		if len(segs) == 0 {
			return "", false
		}
		root := attrRootIdent(obj)
		if root == nil {
			return "", false
		}
		sym := t.symbolOf(root)
		if sym == 0 {
			return "", false
		}
		return constraint.PathKey(symKey(sym) + constraint.FormatSegments(segs)), true
	}
	return "", false
}

// lenExprContainerKey reports whether expr is `#container` over a tracked
// sequence container (a bare identifier or a static field path) and returns that
// container's numeric path key. It generalizes lenExprBase from
// identifier-only bases to field-path bases (`#saga.compensations`).
func (t *Transfer) lenExprContainerKey(expr ast.Expr) (constraint.PathKey, bool) {
	lenOp, ok := expr.(*ast.UnaryLenOpExpr)
	if !ok {
		return "", false
	}
	return t.containerExprKey(lenOp.Expr)
}

// lengthIndexOffset reports whether key is `#base + c` or `c + #base` (the
// append-cursor and length-relative index shape) over an identifier base, and
// returns that base identifier and the constant offset c. A bare `#base` reports
// offset 0. Any other shape reports ok=false.
func (t *Transfer) lengthIndexOffset(key ast.Expr) (*ast.IdentExpr, int64, bool) {
	if ident, ok := lenExprBase(key); ok {
		return ident, 0, true
	}
	arith, ok := key.(*ast.ArithmeticOpExpr)
	if !ok || arith.Operator != "+" {
		return nil, 0, false
	}
	if ident, ok := lenExprBase(arith.Lhs); ok {
		if c, ok := t.constInt(arith.Rhs); ok {
			return ident, c, true
		}
	}
	if ident, ok := lenExprBase(arith.Rhs); ok {
		if c, ok := t.constInt(arith.Lhs); ok {
			return ident, c, true
		}
	}
	return nil, 0, false
}

// lengthIndexContainerOffset is the field-path-aware form of lengthIndexOffset:
// it reports whether key is `#container + c` / `c + #container` / bare
// `#container` over a tracked sequence container (a bare identifier or a static
// field path), returning that container's numeric path key and the constant
// offset c. Any other shape reports ok=false.
func (t *Transfer) lengthIndexContainerOffset(key ast.Expr) (constraint.PathKey, int64, bool) {
	if k, ok := t.lenExprContainerKey(key); ok {
		return k, 0, true
	}
	arith, ok := key.(*ast.ArithmeticOpExpr)
	if !ok || arith.Operator != "+" {
		return "", 0, false
	}
	if k, ok := t.lenExprContainerKey(arith.Lhs); ok {
		if c, ok := t.constInt(arith.Rhs); ok {
			return k, c, true
		}
	}
	if k, ok := t.lenExprContainerKey(arith.Rhs); ok {
		if c, ok := t.constInt(arith.Lhs); ok {
			return k, c, true
		}
	}
	return "", 0, false
}

// isTableInsertCallee reports whether info's callee is precisely the standard
// `table.insert`: the static path with root `table` and a single field segment
// `insert`, called as a function (not a method). The precise root-plus-field
// match keeps an unrelated `obj.insert(...)` from being treated as a sequence
// append.
func isTableInsertCallee(info *cfg.CallInfo) bool {
	if info == nil || info.Method != "" {
		return false
	}
	if info.CalleeName != "insert" {
		return false
	}
	p := info.CalleePath
	if p.Root != "table" || len(p.Segments) != 1 {
		return false
	}
	seg := p.Segments[0]
	return seg.Kind == constraint.SegmentField && seg.Name == "insert"
}

// applyTableInsert applies `table.insert(arr, v)` (and the positional
// `table.insert(arr, pos, v)`): the sequence gains one element, so the length
// floor rises by one and the array element widens to admit the inserted value.
// The first argument must be a tracked identifier; the inserted value is the
// last argument. A call that does not match leaves the state unchanged (the
// sound carry-forward).
func (t *Transfer) applyTableInsert(
	out *flow.PointState,
	info *cfg.CallInfo,
	demand func(int, paramevidence.ParamContract),
) {
	if out.Num == nil || !isTableInsertCallee(info) {
		return
	}
	args := info.Call.Args
	if len(args) < 2 || len(args) > 3 {
		return
	}
	arrIdent, ok := args[0].(*ast.IdentExpr)
	if !ok {
		return
	}
	arrSym := t.symbolOf(arrIdent)
	if arrSym == 0 {
		return
	}
	arrKey := constraint.PathKey(symKey(arrSym))
	if lower, _, ok := out.Num.LenBoundsFor(arrKey); ok {
		out.Num.ApplyLenGeConst(arrKey, lower+1)
	} else {
		out.Num.ApplyLenGeConst(arrKey, 1)
	}

	elemAV, ok := t.evalExpr(out, args[len(args)-1], demand)
	if !ok || elemAV.IsZero() {
		return
	}
	baseKey := symKey(arrSym)
	base, had := out.Env[baseKey]
	if !had || base.IsZero() {
		return
	}
	out.Env[baseKey] = product.AppendElement(base, elemAV)
}

// seedNumericForLength seeds the numeric component for a numeric-for loop whose
// range is bounded by a sequence length: a forward `for i = 1, #arr` or a
// backward `for i = #arr, 1, -1`. The step direction selects which control
// expression supplies the length limit and which supplies the constant floor:
//   - step > 0: i ascends init..limit, so #-bound is the LIMIT and the floor is
//     init (`i <= #arr`, `i >= init`).
//   - step < 0: i descends init..limit, so #-bound is the INIT and the floor is
//     limit (`i <= #arr`, `i >= limit`).
//
// The body executes only when the range is non-empty, which for either form
// implies `#arr >= floor`, so an in-body read `arr[i]` is provably in range. The
// length container may be a bare identifier or a static field path
// (`saga.compensations`). A range whose #-bounded end is not `#container`, or
// whose constant floor is absent, leaves the length reference unseeded (the
// sound carry-forward).
func (t *Transfer) seedNumericForLength(out *flow.PointState, idxSym cfg.SymbolID, info *cfg.NumericForInfo) {
	if out.Num == nil || info == nil || idxSym == 0 {
		return
	}
	lenEnd, floorEnd := info.Limit, info.Init
	if t.forStepIsNegative(info.Step) {
		lenEnd, floorEnd = info.Init, info.Limit
	}
	arrKey, ok := t.lenExprContainerKey(lenEnd)
	if !ok {
		return
	}
	idxKey := constraint.PathKey(symKey(idxSym))
	out.Num.ApplyLeLenOfWithOffset(idxKey, arrKey, 0)
	if c, ok := t.constInt(floorEnd); ok {
		out.Num.ApplyGeConst(idxKey, c)
	} else {
		out.Num.ApplyGeConst(idxKey, 1)
	}
}

// forStepIsNegative reports whether a numeric-for step expression is a negative
// integer constant (`-1`, `-2`). A nil step defaults to +1 (ascending); a
// non-constant or non-negative step is treated as ascending so the length bound
// is read off the limit. Only the descending case redirects the #-bound to the
// loop's init.
func (t *Transfer) forStepIsNegative(step ast.Expr) bool {
	if step == nil {
		return false
	}
	if neg, ok := step.(*ast.UnaryMinusOpExpr); ok {
		if c, ok := t.constInt(neg.Expr); ok && c > 0 {
			return true
		}
	}
	if c, ok := t.constInt(step); ok && c < 0 {
		return true
	}
	return false
}

// refineIndexRead recovers a non-optional element type for a provably in-bounds
// sequence read `base[key]`. It ports the four arms of the store-side
// refineIndexReadAt against the canonical numeric component (out.Num): a literal
// index within a proven length floor, an index variable bounded by the
// container's own length, a length-relative index (`#arr + k`), and a fixed-arity
// tuple. Each arm reads only the converged numeric state and the narrow laws
// self-gate soundness (returning nil when removal would be unsound), so a
// non-match falls through to the unchanged optional ev. The predicate uses the
// LOWER bound of the length and of the index, so a read that any run could take
// out of bounds keeps its nil.
func (t *Transfer) refineIndexRead(
	out *flow.PointState,
	e *ast.AttrGetExpr,
	base product.AbstractValue,
	ev product.AbstractValue,
) product.AbstractValue {
	if out.Num == nil {
		return ev
	}
	container := base.ProjectValue()
	result := ev.ProjectValue()
	if container == nil || result == nil {
		return ev
	}

	arrKey, _ := t.containerExprKey(e.Object)

	// Literal index within a proven length lower bound: arr[k] with #arr >= k.
	if lit, ok := t.constInt(e.Key); ok && lit >= 1 {
		// A fixed-arity tuple's runtime length is its static arity.
		if arity, ok := narrow.TupleArity(container); ok && arity >= lit {
			if refined := narrow.RefineSequenceIndex(container, result, lit); refined != nil {
				return product.FromType(refined)
			}
		}
		if arrKey != "" {
			if lower, _, ok := out.Num.LenBoundsFor(arrKey); ok && lower >= lit {
				if refined := narrow.RefineSequenceIndex(container, result, lit); refined != nil {
					return product.FromType(refined)
				}
			}
		}
		return ev
	}

	// Length-relative index `#base + k` (or bare `#base`) on the same container.
	if lenKey, offset, ok := t.lengthIndexContainerOffset(e.Key); ok {
		if lenKey != "" && lenKey == arrKey {
			// A fixed-arity tuple resolves #t to its static arity directly.
			if arity, ok := narrow.TupleArity(container); ok {
				if refined := narrow.RefineLengthIndex(container, result, arity, offset); refined != nil {
					return product.FromType(refined)
				}
			}
			if lower, _, ok := out.Num.LenBoundsFor(arrKey); ok {
				if refined := narrow.RefineLengthIndex(container, result, lower, offset); refined != nil {
					return product.FromType(refined)
				}
			}
		}
		return ev
	}

	// Index variable: a fixed-arity tuple indexed within [1, arity], or a sequence
	// indexed by a variable bounded by its own length (i <= #arr).
	idxIdent, ok := e.Key.(*ast.IdentExpr)
	if !ok {
		return ev
	}
	idxSym := t.symbolOf(idxIdent)
	if idxSym == 0 {
		return ev
	}
	idxKey := constraint.PathKey(symKey(idxSym))

	if arity, ok := narrow.TupleArity(container); ok {
		if lower, upper, ok := numeric.BoundsForWithTheory(out.Num, idxKey); ok && lower >= 1 && upper <= arity {
			if refined := narrow.RefineSequenceIndex(container, result, lower); refined != nil {
				return product.FromType(refined)
			}
		}
	}

	if arrKey != "" {
		refArr, offset, ok := out.Num.LenRefWithOffsetFor(idxKey)
		if ok && refArr == arrKey {
			if lower, _, ok := numeric.BoundsForWithTheory(out.Num, idxKey); ok && lower+offset >= 1 && offset <= 0 {
				if refined := narrow.RefineSequenceIndex(container, result, lower+offset); refined != nil {
					return product.FromType(refined)
				}
			}
		}
	}

	return ev
}
