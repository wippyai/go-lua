package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// lenseed.go carries two related but distinct size proofs plus the in-bounds
// index-read refinement. Numeric length facts model Lua's sequence border (`#x`)
// and live in out.Num. Container cardinality facts model statically-known
// definitely-present constructor entries for iteration/postcondition reasoning
// (`keys(data)` returning at least one key from a non-empty map) and live in
// out.Rel. Keeping the axes separate prevents string-keyed maps from polluting
// sequence length while giving call postconditions a normalized predicate to
// consume without function-name branches.

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

// tableLiteralCardinalityLowerBound returns a conservative lower bound on the
// number of definitely-present entries in a table constructor. It follows Lua's
// final-write semantics for repeated static keys: the last write decides whether
// the entry is present. Dynamic keys make the lower bound unknown because they
// may collide with a static key or write nil into it, so the sound lower bound is
// zero.
func (t *Transfer) tableLiteralCardinalityLowerBound(out *flow.PointState, e *ast.TableExpr) int64 {
	if e == nil || len(e.Fields) == 0 {
		return 0
	}
	type slot struct {
		present bool
	}
	final := make(map[tableCardinalityKey]slot, len(e.Fields))
	var positional int
	for _, field := range e.Fields {
		if field == nil {
			continue
		}
		key := tableCardinalityKey{}
		if field.Key == nil {
			positional++
			key = tableCardinalityKey{kind: tableCardinalityKeyInt, index: positional}
		} else {
			var ok bool
			key, ok = tableFieldCardinalityKey(field)
			if !ok {
				return 0
			}
		}
		present := false
		if av, ok := t.evalExpr(out, field.Value, nil); ok {
			present = av.DefinitelyPresent()
		}
		final[key] = slot{present: present}
	}
	if len(final) == 0 {
		return 0
	}
	var n int64
	for _, slot := range final {
		if slot.present {
			n++
		}
	}
	return n
}

type tableCardinalityKeyKind uint8

const (
	tableCardinalityKeyString tableCardinalityKeyKind = iota + 1
	tableCardinalityKeyInt
)

type tableCardinalityKey struct {
	kind  tableCardinalityKeyKind
	name  string
	index int
}

func tableFieldCardinalityKey(field *ast.Field) (tableCardinalityKey, bool) {
	seg, ok := fieldkey.FromTableField(field)
	if !ok {
		return tableCardinalityKey{}, false
	}
	if name, ok := fieldkey.StringKeyFromSegment(seg); ok {
		return tableCardinalityKey{kind: tableCardinalityKeyString, name: name}, true
	}
	if seg.Kind == constraint.SegmentIndexInt {
		return tableCardinalityKey{kind: tableCardinalityKeyInt, index: seg.Index}, true
	}
	return tableCardinalityKey{}, false
}

// seedArrayLiteralLength seeds size facts for a slot bound from a constructor.
// `local arr = {1, 2, 3}` proves #arr >= 3 on the numeric sequence axis. A
// string-keyed literal such as `{["a"] = 1, ["b"] = 2}` proves cardinality >= 2
// on the relation axis, not #table >= 2. A binding from any other source drops a
// prior length/cardinality floor (soundness: the new value's size is unknown),
// so a slot reused across assignments never carries stale proof.
func (t *Transfer) seedArrayLiteralLength(out *flow.PointState, sym cfg.SymbolID, src ast.Expr, cardinalityLower int64) {
	if out == nil {
		return
	}
	dropOp, ok := flow.NumericDropLenBoundSymbolOp(sym)
	if !ok {
		return
	}
	flow.ApplyRelationEffect(out, flow.RelationEffect{
		Kind:    flow.RelationKillLengthTargets,
		Symbols: []cfg.SymbolID{sym},
	})
	ops := []flow.NumericOp{dropOp}
	tbl, ok := src.(*ast.TableExpr)
	if !ok {
		flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: ops, RequireExisting: true})
		return
	}
	if n := arrayLiteralArity(tbl); n > 0 {
		if op, ok := flow.NumericLenGeConstSymbolOp(sym, n); ok {
			ops = append(ops, op)
		}
	}
	flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: ops, RequireExisting: true})
	if cardinalityLower > 0 {
		if effect, ok := flow.RelationContainerLowerBoundPathEffect(constraint.Path{Symbol: sym}, cardinalityLower); ok {
			flow.ApplyRelationEffect(out, effect)
		}
	}
}

// applyIndexWriteLength updates the length floor of an indexed write `base[k]=v`.
// An append at the border (`arr[#arr + c]` on the SAME base, c >= 1) raises the
// floor to the prior floor plus c: writing past the current length extends the
// sequence. Any other index write the transfer cannot prove is a same-base border
// append drops the floor (soundness default: the write may target a hole or an
// index the proven floor does not cover, so the prior floor no longer holds).
func (t *Transfer) applyIndexWriteLength(out *flow.PointState, target cfg.AssignTarget) {
	if out == nil || target.Kind != cfg.TargetIndex || target.BaseSymbol == 0 {
		return
	}
	arrKey, ok := flow.NumericVarKeyOfSymbol(target.BaseSymbol)
	if !ok {
		return
	}
	dropOp, ok := flow.NumericDropLenBoundSymbolOp(target.BaseSymbol)
	if !ok {
		return
	}
	lenIdent, offset, ok := t.lengthIndexOffset(target.Key)
	if !ok || offset < 1 {
		flow.ApplyNumericEffect(out, flow.NumericEffect{
			Ops:             []flow.NumericOp{dropOp},
			RequireExisting: true,
		})
		return
	}
	lenKey, ok := flow.NumericVarKeyOfSymbol(t.symbolOf(lenIdent))
	if !ok || lenKey != arrKey {
		flow.ApplyNumericEffect(out, flow.NumericEffect{
			Ops:             []flow.NumericOp{dropOp},
			RequireExisting: true,
		})
		return
	}
	incOp, ok := flow.NumericIncrementLenLowerSymbolOp(target.BaseSymbol, offset)
	if !ok {
		return
	}
	flow.ApplyNumericEffect(out, flow.NumericEffect{
		Ops:             []flow.NumericOp{incOp},
		RequireExisting: true,
	})
}

func (t *Transfer) incrementLenBound(out *flow.PointState, arrKey constraint.PathKey, delta int64) bool {
	return flow.ApplyNumericEffect(out, flow.NumericEffect{
		Ops:             []flow.NumericOp{{Kind: flow.NumericIncrementLenLower, Key: arrKey, Delta: delta}},
		RequireExisting: true,
	})
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
	place, ok := t.staticPlaceOfExpr(expr)
	if !ok {
		return "", false
	}
	return symbolPathKey(place)
}

// containerExprPath builds the constraint.Path for a tracked container expression
// (a bare identifier or a static field path), the path-typed counterpart of
// containerExprKey. The KeyOf production (applyGenericFor) and the index-read
// consumption (refineIndexRead) both derive their table/key paths through this
// helper, so a key drawn from `pairs(container)` and the same container indexed by
// that key resolve to Equal paths. A non-static or non-identifier-rooted base
// reports ok=false.
func (t *Transfer) containerExprPath(expr ast.Expr) (constraint.Path, bool) {
	place, ok := t.staticPlaceOfExpr(expr)
	if !ok {
		return constraint.Path{}, false
	}
	return place.StaticPath()
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

func (t *Transfer) lenExprContainerPlace(expr ast.Expr) (Place, constraint.PathKey, bool) {
	lenOp, ok := expr.(*ast.UnaryLenOpExpr)
	if !ok {
		return Place{}, "", false
	}
	place, ok := t.staticPlaceOfExpr(lenOp.Expr)
	if !ok {
		return Place{}, "", false
	}
	key, ok := symbolPathKey(place)
	if !ok {
		return Place{}, "", false
	}
	return place, key, true
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

// evalTableCreateCall lowers Luau's table.create allocation primitive into a
// product-domain allocation seed. Positive array capacity is a sequence seed;
// zero/unknown array capacity stays a fresh record seed, which dynamic writes
// and table.insert can still refine through the ordinary product transfer laws.
func (t *Transfer) evalTableCreateCall(out *flow.PointState, call *ast.FuncCallExpr) (product.AbstractValue, bool) {
	if !t.isTableCreateCall(call) {
		return product.AbstractValue{}, false
	}
	if call == nil || len(call.Args) == 0 {
		return product.AbstractValue{}, false
	}
	narray, hasConstArray := t.constInt(call.Args[0])
	if hasConstArray && narray > 0 {
		return product.FromType(typ.NewFreshArray()), true
	}
	return product.FromType(typ.NewFreshEmptyRecord()), true
}

func (t *Transfer) isTableCreateCall(call *ast.FuncCallExpr) bool {
	if t == nil || call == nil || call.Method != "" || call.Func == nil {
		return false
	}
	path, ok := t.staticPathOfExpr(call.Func)
	if !ok || path.Root != "table" || len(path.Segments) != 1 {
		return false
	}
	seg := path.Segments[0]
	return seg.Kind == constraint.SegmentField && seg.Name == "create"
}

// applyTableInsert lowers `table.insert` into a MutatorEffect. Direct sequence
// targets (`arr`, `saga.compensations`, `groups["default"]`) append to the exact
// Place and raise that sequence's length floor when it has a static numeric key.
// Dynamic map-element targets (`suites[suite]`) append into the map value slot
// through the base Place with an explicit abstract key; those per-key lengths are
// not represented in the numeric component.
func (t *Transfer) applyTableInsert(
	out *flow.PointState,
	info *cfg.CallInfo,
	demand func(int, paramevidence.ParamContract),
) {
	if !isTableInsertCallee(info) {
		return
	}
	args := info.Call.Args
	if len(args) < 2 || len(args) > 3 {
		return
	}
	target := args[0]
	elemExpr := args[len(args)-1]
	if out.Num == nil {
		effect, ok := t.tableInsertMutatorEffect(out, target, elemExpr, product.AbstractValue{}, demand)
		if ok {
			effect.LengthKey = ""
			effect.LengthIncrement = 0
			t.applyMutatorEffect(out, effect)
		}
		return
	}
	elemAV, hasElem := t.evalExpr(out, elemExpr, demand)
	if !hasElem || elemAV.IsZero() {
		elemAV = product.AbstractValue{}
	}
	t.demandTableInsertTarget(out, target, elemAV, demand)

	effect, ok := t.tableInsertMutatorEffect(out, target, elemExpr, elemAV, demand)
	if !ok {
		return
	}
	t.applyMutatorEffect(out, effect)
}

func (t *Transfer) demandTableInsertTarget(
	out *flow.PointState,
	target ast.Expr,
	elem product.AbstractValue,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil {
		return
	}
	elemContract := paramevidence.DemandFromType(typ.Any)
	if !elem.IsZero() {
		if elemType := elem.ProjectValue(); elemType != nil && !typ.IsAbsentOrUnknown(elemType) {
			elemContract = paramevidence.DemandFromType(elemType)
		}
	}
	t.demandExprContractCtx(out, target, paramevidence.DemandFromSequenceElement(elemContract), demand)
}

func (t *Transfer) tableInsertMutatorEffect(
	out *flow.PointState,
	target ast.Expr,
	elemExpr ast.Expr,
	elemAV product.AbstractValue,
	demand func(int, paramevidence.ParamContract),
) (MutatorEffect, bool) {
	elemPath, _ := t.staticPathOfExpr(elemExpr)
	if attr, isAttr := target.(*ast.AttrGetExpr); isAttr {
		if _, isStatic := staticMemberKey(attr); !isStatic {
			base, ok := t.placeOfExpr(out, attr.Object, demand)
			if !ok || base.Root == 0 {
				return MutatorEffect{}, false
			}
			key, _ := t.evalExpr(out, attr.Key, demand)
			return MutatorEffect{
				Place:       base,
				Kind:        MutatorAppendMapElement,
				Element:     elemAV,
				ElementExpr: elemExpr,
				ElementPath: elemPath,
				Key:         key,
			}, true
		}
	}

	place, ok := t.placeOfExpr(out, target, demand)
	if !ok || place.Root == 0 {
		return MutatorEffect{}, false
	}
	effect := MutatorEffect{
		Place:       place,
		Kind:        MutatorAppendElement,
		Element:     elemAV,
		ElementExpr: elemExpr,
		ElementPath: elemPath,
	}
	if arrKey, ok := t.containerExprKey(target); ok {
		effect.LengthKey = arrKey
		effect.LengthIncrement = 1
	}
	return effect, true
}

// seedNumericForBounds seeds the numeric component with a numeric-for loop's
// induction RANGE [floor, ceil]: the interval the control variable ranges over
// across the whole loop body. The step direction selects which control expression
// is the range floor and which is the range ceiling:
//   - step > 0: i ascends init..limit, so the floor is init and the ceiling limit
//     (`i >= init`, `i <= limit`).
//   - step < 0: i descends init..limit, so the floor is limit and the ceiling init
//     (`i >= limit`, `i <= init`).
//
// The ceiling is recorded as a constant (`i <= c`) when it is an integer constant,
// or as a symbolic length reference (`i <= #arr + offset`) when it is `#container`
// over a tracked sequence (a bare identifier or a static field path). The floor is
// recorded as a constant when it is one. Both bounds describe the body view of the
// induction variable on every iteration, so a body read `arr[i]` is provably in
// range exactly when the range lies within the container's length: a constant
// ceiling within a tuple's static arity, or a #arr-relative ceiling, proves the
// element present, while a ceiling that exceeds the length keeps the read optional
// (soundness). A non-constant, non-length ceiling, or a step with no provable
// direction, leaves the corresponding bound unseeded (the sound carry-forward).
func (t *Transfer) seedNumericForBounds(out *flow.PointState, idxSym cfg.SymbolID, info *cfg.NumericForInfo) {
	if out == nil || info == nil || idxSym == 0 {
		return
	}
	ceilEnd, floorEnd := info.Limit, info.Init
	if t.forStepIsNegative(info.Step) {
		ceilEnd, floorEnd = info.Init, info.Limit
	}
	ops := make([]flow.NumericOp, 0, 2)
	if c, ok := t.constInt(floorEnd); ok {
		if op, ok := flow.NumericVarGeConstSymbolOp(idxSym, c); ok {
			ops = append(ops, op)
		}
	}
	if c, ok := t.constInt(ceilEnd); ok {
		if op, ok := flow.NumericVarLeConstSymbolOp(idxSym, c); ok {
			ops = append(ops, op)
		}
		flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: ops, RequireExisting: true})
		return
	}
	if arrKey, ok := t.lenExprContainerKey(ceilEnd); ok {
		if op, ok := flow.NumericVarLeLenOffsetSymbolOp(idxSym, arrKey, 0); ok {
			ops = append(ops, op)
		}
	}
	flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: ops, RequireExisting: true})
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

// refineByKeyPresence strips the optional from an index read `container[key]`
// when the product-state KeyPresence axis proves that key came from the same
// container. A non-identifier key, an empty path, or a missing fact declines,
// leaving the optional intact. Removal is gated by the narrow laws: only a result
// whose nil is pure flow-uncertainty (an optional element value) is narrowed.
func (t *Transfer) refineByKeyPresence(
	out *flow.PointState,
	e *ast.AttrGetExpr,
	result typ.Type,
) (product.AbstractValue, bool) {
	tablePath, ok := t.containerExprPath(e.Object)
	if !ok || tablePath.IsEmpty() {
		return product.AbstractValue{}, false
	}
	keyPath, ok := t.dynamicIndexKeyPath(e.Key)
	if !ok {
		return product.AbstractValue{}, false
	}
	if !flow.PointFactsOf(*out).HasKeyPresence(tablePath, keyPath) {
		return product.AbstractValue{}, false
	}
	keyValue, ok := flow.PointFactsOf(*out).PathValue(keyPath)
	if ok && keyValue.DefinitelyAbsent() {
		return product.AbstractValue{}, false
	}
	if !narrow.NilPresenceIsOnlyFlowUncertainty(result) {
		return product.AbstractValue{}, false
	}
	refined := narrow.RemoveNil(result)
	if refined == nil || typ.IsNever(refined) || typ.TypeEquals(refined, result) {
		return product.AbstractValue{}, false
	}
	return product.FromType(refined), true
}

func (t *Transfer) dynamicIndexKeyPath(expr ast.Expr) (constraint.Path, bool) {
	path, ok := t.staticPathOfExpr(expr)
	if !ok || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

// dynamicWriteKey resolves the value-domain key of a dynamic-key write base[key] = v.
// It first reads the key expression's tracked value; when that does not resolve — a
// `pairs` key variable over a closed record is left untyped by the iteration typing,
// since a closed record is not a uniform keyed container — it synthesizes the
// key's sound domain from the base record's field names when KeyPresence proves
// the key came from that base. The synthesized domain is the union of the record's
// string field-name literals: a `pairs(base)` key ranges over exactly those
// names, so a write through it can land on any of them. A key the transfer can
// neither resolve nor prove a key of the base yields zero, so the write is left
// as the sound carry-forward.
func (t *Transfer) dynamicWriteKey(
	out *flow.PointState,
	target cfg.AssignTarget,
	base product.AbstractValue,
	demand func(int, paramevidence.ParamContract),
) product.AbstractValue {
	if key, ok := t.evalExpr(out, target.Key, demand); ok && !key.IsZero() {
		return key
	}
	keyIdent, ok := target.Key.(*ast.IdentExpr)
	if !ok {
		return product.AbstractValue{}
	}
	keySym := t.symbolOf(keyIdent)
	if keySym == 0 {
		return product.AbstractValue{}
	}
	basePath, ok := t.staticContainerPathOfAssignTarget(target)
	if !ok {
		return product.AbstractValue{}
	}
	keyPath := constraint.NewPath(keySym, keyIdent.Value)
	if !flow.PointFactsOf(*out).HasKeyPresence(basePath, keyPath) {
		return product.AbstractValue{}
	}
	names := recordFieldNameDomain(base)
	if names == nil {
		return product.AbstractValue{}
	}
	return product.FromType(names)
}

// recordFieldNameDomain returns the union of the string field-name literals of the
// record av carries, the key domain a `pairs` iteration over a closed record ranges
// over. A non-record value, or a record with no named string fields, yields nil so
// the caller declines to synthesize a key.
func recordFieldNameDomain(av product.AbstractValue) typ.Type {
	if av.IsZero() {
		return nil
	}
	t := av.ProjectValue()
	if t == nil {
		return nil
	}
	rec, ok := unwrapRecord(t)
	if !ok || len(rec.Fields) == 0 {
		return nil
	}
	names := make([]typ.Type, 0, len(rec.Fields))
	for _, f := range rec.Fields {
		names = append(names, typ.LiteralString(f.Name))
	}
	if len(names) == 0 {
		return nil
	}
	return typ.NewUnion(names...)
}

// unwrapRecord resolves t to its underlying record through an alias, reporting false
// for a non-record type.
func unwrapRecord(t typ.Type) (*typ.Record, bool) {
	switch r := unwrap.Alias(t).(type) {
	case *typ.Record:
		return r, true
	}
	return nil, false
}

// writeIsSelfDerived reports whether the dynamic-key write base[key] = src writes
// back a value provably already held by base at the key being written, so the
// write changes no field's value domain (the value at key K is stored back to
// key K). Such a SELF-write must not weaken the container's declared fields;
// only a FOREIGN write (a value not drawn from base at this key) can replace a
// field with a new type.
//
// Two self-derivation forms are recognized structurally from the source expression:
//
//   - src is `base[key]` itself (the same base symbol and the same key expression as
//     the target): writing a slot back to itself is identity.
//   - src is the VALUE variable of a keyed `pairs(base)` iteration whose current
//     product-state provenance says value = base[key]. This fact is killed by
//     assignment to base, key, or value, so reassignment cannot keep a stale
//     self-write proof alive.
//
// Any other source is treated as foreign (the sound default: a value the transfer
// cannot prove came from base at this key may differ from the field's type).
func (t *Transfer) writeIsSelfDerived(out *flow.PointState, target cfg.AssignTarget, src ast.Expr) bool {
	if target.Key == nil || target.BaseSymbol == 0 {
		return false
	}
	keyIdent, ok := target.Key.(*ast.IdentExpr)
	if !ok {
		return false
	}
	keySym := t.symbolOf(keyIdent)
	if keySym == 0 {
		return false
	}
	basePath, ok := t.staticContainerPathOfAssignTarget(target)
	if !ok {
		return false
	}
	// Form 1: base[key] = base[key] (same container path, same key symbol).
	if attr, ok := src.(*ast.AttrGetExpr); ok {
		if srcBasePath, ok := t.staticPathOfExpr(attr.Object); ok && srcBasePath.Equal(basePath) {
			if srcKeyIdent, isKeyIdent := attr.Key.(*ast.IdentExpr); isKeyIdent {
				if t.symbolOf(srcKeyIdent) == keySym {
					return true
				}
			}
		}
	}
	// Form 2: base[key] = value, where (key, value) are the loop variables of a keyed
	// iteration over base.
	srcIdent, ok := src.(*ast.IdentExpr)
	if !ok {
		return false
	}
	valueSym := t.symbolOf(srcIdent)
	if valueSym == 0 {
		return false
	}
	keyPath := constraint.NewPath(keySym, keyIdent.Value)
	valuePath := constraint.NewPath(valueSym, srcIdent.Value)
	return out != nil && flow.PointFactsOf(*out).HasKeyValuePresence(basePath, keyPath, valuePath)
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
	container := base.ProjectValue()
	result := ev.ProjectValue()
	if container == nil || result == nil {
		return ev
	}

	if refined, ok := t.refineByIndexWriteAdmission(out, e); ok {
		return refined
	}

	// Key-presence (KeyOf): a key drawn from `pairs(container)` indexing that same
	// container reads a present value, so the optional is stripped. The fact is
	// produced by iteration transfer into PointState.KeyPresence and matched here
	// for the exact (container, key) path pair, so a key from a different container
	// or an arbitrary key never matches and its read stays optional.
	if refined, ok := t.refineByKeyPresence(out, e, result); ok {
		return refined
	}

	if out.Num == nil {
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
	idxKey, ok := flow.NumericVarKeyOfSymbol(idxSym)
	if !ok {
		return ev
	}

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
