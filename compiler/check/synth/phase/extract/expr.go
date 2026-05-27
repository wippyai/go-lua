// expr.go implements expression-level type synthesis helpers.
//
// This file contains specialized synthesis for complex expression patterns:
//   - Logical operators (and/or) with short-circuit narrowing
//   - Attribute access with field/method resolution
//   - Table constructors with bidirectional typing
//   - Arithmetic and unary operators
//   - Expected type propagation for contextual inference
//
// # LOGICAL OPERATOR NARROWING
//
// For `x and y`, if x is truthy then y is evaluated with x narrowed to truthy.
// For `x or y`, if x is falsy then y is evaluated with x narrowed to falsy.
// This enables patterns like: `x and x.field` where x may be nil.
//
// # EXPECTED TYPE HANDLING
//
// When an expected type is provided (from assignment or function parameter),
// expressions are synthesized with contextual typing. This is important for:
//   - Table literals: fields inferred from expected record type
//   - Function literals: parameters inferred from expected function type
//   - Union discrimination: selecting the best matching union member
package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/conditionexpr"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/indexread"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// synthAttrGetCore synthesizes type for attribute access using the shared core.
func (s *Synthesizer) synthAttrGetCore(ex *ast.AttrGetExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	return s.synthAttrGetCoreWithCaptureTypes(ex, p, sc, narrower, recurse, nil)
}

func (s *Synthesizer) synthAttrGetCoreWithCaptureTypes(
	ex *ast.AttrGetExpr,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
	recurse ExprSynth,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	objType := recurse(ex.Object)
	objType = s.indexedObjectType(ex.Object, objType, p, sc, narrower)
	if typ.IsAny(unwrap.Alias(objType)) {
		return typ.Any
	}

	if narrowed, ok := s.narrowedAttrGetType(ex, objType, p, sc, narrower, captureTypes); ok {
		return narrowed
	}

	switch key := ex.Key.(type) {
	case *ast.StringExpr:
		return s.stringKeyAttrType(ex, objType, key.Value, p, sc, captureTypes)
	case *ast.NumberExpr:
		return s.numberKeyAttrType(ex, objType, key.Value, p, sc, narrower, captureTypes)
	case *ast.IdentExpr:
		return s.identKeyAttrType(ex, objType, key, recurse(key), p, sc, narrower, captureTypes)
	default:
		return s.dynamicKeyAttrType(ex, objType, recurse(ex.Key), p, sc, narrower, captureTypes)
	}
}

func (s *Synthesizer) indexedObjectType(obj ast.Expr, static typ.Type, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	if narrower == nil || s == nil || s.deps == nil || s.deps.Paths == nil {
		return static
	}
	path := s.deps.Paths(p, obj, sc)
	if path.IsEmpty() {
		return static
	}
	flowType := narrower.NarrowedTypeAt(p, path)
	if flowType == nil || typ.IsNever(flowType) {
		return static
	}
	if static == nil || typ.IsUnknown(unwrap.Alias(static)) || static.Kind().IsPlaceholder() {
		return flowType
	}
	if subtype.IsSubtype(flowType, static) {
		return flowType
	}
	return static
}

func (s *Synthesizer) narrowedAttrGetType(
	ex *ast.AttrGetExpr,
	objType typ.Type,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
	captureTypes map[cfg.SymbolID]typ.Type,
) (typ.Type, bool) {
	if narrower == nil || s.deps.Paths == nil {
		return nil, false
	}
	path := s.deps.Paths(p, ex, sc)
	if path.IsEmpty() {
		return nil, false
	}
	narrowed := narrower.NarrowedTypeAt(p, path)
	if narrowed == nil {
		return nil, false
	}
	if refined, ok := s.refineIndexRead(objType, narrowed, ex.Object, ex.Key, p, sc, narrower); ok {
		return refined, true
	}
	if specialized := s.specializeAttrValue(ex, p, sc, narrowed, captureTypes); specialized != nil {
		return specialized, true
	}
	if typ.IsUnknown(unwrap.Alias(narrowed)) && !typ.IsUnknown(unwrap.Alias(objType)) {
		return nil, false
	}
	if key, ok := ex.Key.(*ast.StringExpr); ok {
		refined, ok := s.reconcileNarrowedFieldRead(objType, key.Value, narrowed)
		if !ok {
			return nil, false
		}
		narrowed = refined
	}
	return narrowed, true
}

func (s *Synthesizer) reconcileNarrowedFieldRead(objType typ.Type, field string, narrowed typ.Type) (typ.Type, bool) {
	declaredField, ok := s.deps.Types.Field(s.deps.Ctx, objType, field)
	if !ok || declaredField == nil {
		if querycore.MissingFieldReadsNil(objType) {
			return typ.Nil, true
		}
		return narrowed, true
	}
	return value.ReconcilePathFactWithDeclaredRead(narrowed, declaredField)
}

func (s *Synthesizer) stringKeyAttrType(
	ex *ast.AttrGetExpr,
	objType typ.Type,
	key string,
	p cfg.Point,
	sc *scope.State,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	if ft, ok := s.deps.Types.Field(s.deps.Ctx, objType, key); ok {
		if manifestPath := s.manifestPathForAttrObject(ex.Object); manifestPath != "" {
			ft = enrichWithManifest(s.deps.Manifests, ft, manifestPath, key)
		}
		return s.specializedOrOriginalAttrValue(ex, p, sc, ft, captureTypes)
	}
	if vt := mapValueType(objType); vt != nil {
		return vt
	}
	if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, typ.LiteralString(key)); ok {
		return s.specializedOrOriginalAttrValue(ex, p, sc, it, captureTypes)
	}
	if missingFieldIsTypo(objType) {
		return typ.Unknown
	}
	if querycore.MissingFieldReadsNil(objType) {
		return typ.Nil
	}
	return typ.Unknown
}

// missingFieldIsTypo reports whether reading a named field absent from objType
// is a likely typo against an exhaustive shape rather than a well-defined Lua
// nil read. A closed record without a map component declares its full field set,
// so an unlisted field resolves to unknown and drives the no-field diagnostic.
// Open records, maps, and other dynamic table shapes read missing keys as nil.
func missingFieldIsTypo(objType typ.Type) bool {
	rec, ok := unwrap.Alias(objType).(*typ.Record)
	if !ok {
		return false
	}
	return !rec.Open && !rec.HasMapComponent()
}

func (s *Synthesizer) manifestPathForAttrObject(obj ast.Expr) string {
	ident, ok := obj.(*ast.IdentExpr)
	if !ok || s.deps.Manifests == nil || s.deps.CheckCtx == nil {
		return ""
	}
	bindings := s.deps.CheckCtx.Bindings()
	if bindings == nil {
		return ""
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return ""
	}
	return s.deps.CheckCtx.ModuleAlias(sym)
}

func (s *Synthesizer) numberKeyAttrType(
	ex *ast.AttrGetExpr,
	objType typ.Type,
	key string,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	keyType := ops.ParseNumber(key)
	it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType)
	if !ok {
		return typ.Unknown
	}
	if narrower != nil {
		if refined, ok := s.refineIndexRead(objType, it, ex.Object, ex.Key, p, sc, narrower); ok {
			return s.specializedOrOriginalAttrValue(ex, p, sc, refined, captureTypes)
		}
	}
	return s.specializedOrOriginalAttrValue(ex, p, sc, it, captureTypes)
}

func (s *Synthesizer) identKeyAttrType(
	ex *ast.AttrGetExpr,
	objType typ.Type,
	key *ast.IdentExpr,
	keyType typ.Type,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType)
	if !ok {
		if derived := s.indexFromKeyOf(objType, ex.Object, key, p, sc, narrower); derived != nil {
			return derived
		}
		return typ.Unknown
	}
	if narrower != nil {
		it = s.refineIdentIndexResult(ex, objType, key, keyType, it, p, sc, narrower)
	}
	return s.specializedOrOriginalAttrValue(ex, p, sc, it, captureTypes)
}

func (s *Synthesizer) refineIdentIndexResult(
	ex *ast.AttrGetExpr,
	objType typ.Type,
	key *ast.IdentExpr,
	keyType typ.Type,
	indexResult typ.Type,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
) typ.Type {
	if refined, ok := s.refineIndexRead(objType, indexResult, ex.Object, ex.Key, p, sc, narrower); ok {
		return refined
	}
	result := s.refineIndexByKeyOf(ex.Object, key, indexResult, p, sc, narrower)
	if derived := s.indexFromKeyOf(objType, ex.Object, key, p, sc, narrower); derived != nil {
		if keyType != nil && keyType.Kind().IsPlaceholder() {
			return derived
		}
		if shouldPreferKeyOfIndex(result) {
			return derived
		}
	}
	return result
}

func (s *Synthesizer) refineIndexByKeyOf(obj ast.Expr, key *ast.IdentExpr, indexResult typ.Type, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	if !s.hasKeyOfIndex(obj, key, p, sc, narrower) {
		return indexResult
	}
	if opt, ok := indexResult.(*typ.Optional); ok {
		return opt.Inner
	}
	if refined := narrow.RemoveNil(indexResult); !typ.IsNever(refined) {
		return refined
	}
	return indexResult
}

func (s *Synthesizer) hasKeyOfIndex(obj ast.Expr, key *ast.IdentExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps) bool {
	if narrower == nil || s.deps.Paths == nil || s.deps.CheckCtx == nil {
		return false
	}
	tablePath := s.deps.Paths(p, obj, sc)
	if tablePath.IsEmpty() {
		return false
	}
	bindings := s.deps.CheckCtx.Bindings()
	if bindings == nil {
		return false
	}
	keySym, ok := bindings.SymbolOf(key)
	if !ok || keySym == 0 {
		return false
	}
	keyPath := s.identifierPath(key, p, sc, keySym)
	return narrower.HasKeyOf(p, tablePath, keyPath)
}

func (s *Synthesizer) dynamicKeyAttrType(
	ex *ast.AttrGetExpr,
	objType typ.Type,
	keyType typ.Type,
	p cfg.Point,
	sc *scope.State,
	narrower api.FlowOps,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType)
	if !ok {
		return typ.Unknown
	}
	if narrower != nil {
		if refined, ok := s.refineIndexRead(objType, it, ex.Object, ex.Key, p, sc, narrower); ok {
			return s.specializedOrOriginalAttrValue(ex, p, sc, refined, captureTypes)
		}
	}
	return s.specializedOrOriginalAttrValue(ex, p, sc, it, captureTypes)
}

func (s *Synthesizer) refineIndexRead(container, result typ.Type, obj, key ast.Expr, p cfg.Point, sc *scope.State, narrower api.FlowOps) (typ.Type, bool) {
	if narrower == nil || s == nil || s.deps == nil || s.deps.Paths == nil {
		return nil, false
	}
	return indexread.Refine(indexread.Query{
		Point:     p,
		Container: container,
		Result:    result,
		Object:    obj,
		Key:       key,
		Flow:      narrower,
		PathOf: func(expr ast.Expr) constraint.Path {
			return s.deps.Paths(p, expr, sc)
		},
	})
}

func (s *Synthesizer) specializedOrOriginalAttrValue(expr ast.Expr, p cfg.Point, sc *scope.State, t typ.Type, captureTypes map[cfg.SymbolID]typ.Type) typ.Type {
	if specialized := s.specializeAttrValue(expr, p, sc, t, captureTypes); specialized != nil {
		return specialized
	}
	return t
}

func (s *Synthesizer) specializeAttrValue(expr ast.Expr, p cfg.Point, sc *scope.State, t typ.Type, captureTypes map[cfg.SymbolID]typ.Type) typ.Type {
	return s.stableLocalFunctionValueType(expr, p, sc, t, captureTypes)
}

func (s *Synthesizer) indexFromKeyOf(objType typ.Type, objExpr ast.Expr, key *ast.IdentExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	if s == nil || key == nil || narrower == nil || s.deps.Paths == nil || s.deps.CheckCtx == nil {
		return nil
	}
	bindings := s.deps.CheckCtx.Bindings()
	if bindings == nil {
		return nil
	}
	tablePath := s.deps.Paths(p, objExpr, sc)
	if tablePath.IsEmpty() {
		return nil
	}
	keySym, ok := bindings.SymbolOf(key)
	if !ok || keySym == 0 {
		return nil
	}
	keyPath := s.identifierPath(key, p, sc, keySym)
	if !narrower.HasKeyOf(p, tablePath, keyPath) {
		return nil
	}
	tableType := objType
	if tableType == nil || tableType.Kind().IsPlaceholder() {
		if narrowed := narrower.NarrowedTypeAt(p, tablePath); narrowed != nil {
			tableType = narrowed
		}
	}
	derivedKey := querycore.EntryKeyType(tableType)
	if derivedKey == nil {
		return nil
	}
	if it, ok := s.deps.Types.Index(s.deps.Ctx, tableType, derivedKey); ok {
		if opt, ok := it.(*typ.Optional); ok {
			return opt.Inner
		}
		return it
	}
	return nil
}

func shouldPreferKeyOfIndex(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, m := range v.Members {
			if shouldPreferKeyOfIndex(m) {
				return true
			}
		}
		return false
	default:
		if t.Kind().IsPlaceholder() {
			return true
		}
		if t.Kind() == kind.Nil {
			return true
		}
		return false
	}
}

// mapValueType returns the value type for map-like types without adding optional.
func mapValueType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Map:
		return v.Value
	case *typ.Optional:
		return mapValueType(v.Inner)
	case *typ.Union:
		var types []typ.Type
		for _, m := range v.Members {
			if vt := mapValueType(m); vt != nil {
				types = append(types, vt)
			}
		}
		if len(types) == 1 {
			return types[0]
		}
		if len(types) > 1 {
			return typ.NewUnion(types...)
		}
		return nil
	case *typ.Instantiated:
		if resolved, err := querycore.ResolveInstantiated(v); err == nil {
			return mapValueType(resolved)
		}
	}
	return nil
}

// synthLogicalOpCore synthesizes type for logical operators.
func (s *Synthesizer) synthLogicalOpCore(ex *ast.LogicalOpExpr, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)
	right := recurse(ex.Rhs)

	switch ex.Operator {
	case "and":
		return ops.LogicalAndTyped(left, right)
	case "or":
		return ops.LogicalOrTyped(left, right)
	default:
		return typ.Unknown
	}
}

// synthRelationalOpCore synthesizes relational operators that can be proven
// from the current abstract value. Other comparisons stay boolean.
func (s *Synthesizer) synthRelationalOpCore(ex *ast.RelationalOpExpr, recurse ExprSynth) typ.Type {
	if cmp, ok := guard.ExtractTypeProbeComparison(ex); ok {
		observed := recurse(cmp.Probe.Expr)
		return guard.EvaluateTypeProbeComparison(observed, cmp)
	}
	return typ.Boolean
}

// synthLogicalOpWithNarrowing synthesizes logical op with LHS path narrowing in RHS.
// For `and`: RHS sees LHS narrowed to truthy.
// For `or`: RHS sees LHS narrowed to falsy.
func (s *Synthesizer) synthLogicalOpWithNarrowing(ex *ast.LogicalOpExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)

	if ex.Operator == "and" && ops.IsFalsy(left) {
		return left
	}
	if ex.Operator == "or" && ops.IsTruthy(left) {
		return left
	}

	if ex.Operator == "and" {
		if wrapped, ok := s.logicalBranchFlow(ex.Lhs, p, sc, narrower, true); ok {
			right := s.synthLogicalRHS(ex, p, sc, wrapped, recurse, true)
			return ops.LogicalAndTyped(left, right)
		}
	}
	if ex.Operator == "or" {
		if wrapped, ok := s.logicalBranchFlow(ex.Lhs, p, sc, narrower, false); ok {
			right := s.synthLogicalRHS(ex, p, sc, wrapped, recurse, false)
			return ops.LogicalOrTyped(left, right)
		}
	}

	return s.synthLogicalOpCore(ex, recurse)
}

// synthLogicalRHS synthesizes the RHS of a short-circuit logical operator with
// the LHS narrowed by its branch truthiness. A flow projection (wrapped) carries
// the narrowing when a solution is present. When no flow solution is available
// (spec-overlay/pre-flow synthesis), the RHS is synthesized through the original
// recurse closure so the active type context survives, and references to the LHS
// path are structurally refined to the truthy/falsy branch so a guarded operand
// such as `nl` in `nl and (nl - 1)` reads as non-nil rather than degrading to
// unknown.
func (s *Synthesizer) synthLogicalRHS(
	ex *ast.LogicalOpExpr,
	p cfg.Point,
	sc *scope.State,
	wrapped api.FlowOps,
	recurse ExprSynth,
	truthy bool,
) typ.Type {
	if wrapped != nil {
		if right := s.SynthExpr(ex.Rhs, p, wrapped); !typ.IsUnknown(right) {
			return right
		}
	}
	lhsSym := s.logicalGuardSymbol(ex.Lhs)
	if lhsSym == 0 {
		return recurse(ex.Rhs)
	}
	narrowed := func(t typ.Type) typ.Type {
		if truthy {
			return narrow.ToTruthy(t)
		}
		return narrow.ToFalsy(t)
	}
	guarded := func(inner ExprSynth) ExprSynth {
		var self ExprSynth
		self = func(child ast.Expr) typ.Type {
			if ident, ok := child.(*ast.IdentExpr); ok && s.LookupSymbol(ident) == lhsSym {
				if refined := narrowed(inner(child)); refined != nil {
					return refined
				}
			}
			return s.synthExprCore(child, sc, p, nil, self)
		}
		return self
	}
	return guarded(recurse)(ex.Rhs)
}

// logicalGuardSymbol returns the symbol guarded by a logical operator's LHS when
// it is a bare identifier whose truthiness refines later reads. Compound guards
// resolve through the flow projection instead.
func (s *Synthesizer) logicalGuardSymbol(expr ast.Expr) cfg.SymbolID {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return 0
	}
	return s.LookupSymbol(ident)
}

func (s *Synthesizer) logicalBranchFlow(expr ast.Expr, p cfg.Point, sc *scope.State, narrower api.FlowOps, truthy bool) (api.FlowOps, bool) {
	condition := s.logicalBranchCondition(expr, p, sc, narrower, truthy)
	if !condition.HasConstraints() && !condition.IsFalse() {
		return narrower, false
	}
	return api.ConditionFlow(narrower, condition), true
}

func (s *Synthesizer) logicalBranchCondition(expr ast.Expr, p cfg.Point, sc *scope.State, narrower api.FlowOps, truthy bool) constraint.Condition {
	if expr == nil {
		return constraint.TrueCondition()
	}
	inputs := s.conditionInputs()
	return (conditionexpr.Extractor{
		P:        p,
		SC:       sc,
		Inputs:   inputs,
		Bindings: s.conditionBindings(),
		Graph:    s.deps.Graph(),
	}).ConditionForTruth(expr, truthy)
}

func (s *Synthesizer) conditionBindings() *bind.BindingTable {
	if s == nil || s.deps == nil || s.deps.CheckCtx == nil {
		return nil
	}
	return s.deps.CheckCtx.Bindings()
}

func (s *Synthesizer) conditionInputs() *flow.Inputs {
	if s == nil || s.deps == nil {
		return nil
	}
	if s.deps.Inputs != nil {
		return s.deps.Inputs
	}
	if graph := s.deps.Graph(); graph != nil {
		return &flow.Inputs{Graph: graph}
	}
	return nil
}

func (s *Synthesizer) logicalNarrowPath(p cfg.Point, expr ast.Expr, sc *scope.State) constraint.Path {
	if s == nil {
		return constraint.Path{}
	}
	if s.deps.Paths != nil {
		return s.deps.Paths(p, expr, sc)
	}
	if ident, ok := expr.(*ast.IdentExpr); ok {
		if s.deps.CheckCtx != nil {
			if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
				if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
					return constraint.Path{Root: ident.Value, Symbol: sym}
				}
			}
		}
	}
	return constraint.Path{}
}

func (s *Synthesizer) synthLogicalOpWithExpected(ex *ast.LogicalOpExpr, sc *scope.State, p cfg.Point, narrower api.FlowOps, recurse ExprSynth, expected typ.Type) typ.Type {
	if ex == nil {
		return typ.Unknown
	}
	if expected == nil || ex.Operator != "or" && ex.Operator != "and" {
		return s.synthLogicalOpCore(ex, recurse)
	}

	branch := func(expr ast.Expr, truthy bool) typ.Type {
		if expr == nil {
			return typ.Unknown
		}
		branchRecurse := recurse
		branchFlow := narrower
		if wrapped, ok := s.logicalBranchFlow(ex.Lhs, p, sc, narrower, truthy); ok {
			branchFlow = wrapped
			branchRecurse = func(child ast.Expr) typ.Type {
				return s.SynthExpr(child, p, wrapped)
			}
		}
		return s.synthExprWithExpectedCoreFlow(expr, sc, p, branchFlow, branchRecurse, expected)
	}

	left := recurse(ex.Lhs)
	right := branch(ex.Rhs, ex.Operator == "and")
	switch ex.Operator {
	case "and":
		return ops.LogicalAndTyped(left, right)
	case "or":
		return ops.LogicalOrTyped(left, right)
	default:
		return typ.Unknown
	}
}

// synthArithmeticOpCore synthesizes type for arithmetic operators.
func (s *Synthesizer) synthArithmeticOpCore(ex *ast.ArithmeticOpExpr, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)
	right := recurse(ex.Rhs)
	return s.deps.Types.BinaryOp(s.deps.Ctx, left, ex.Operator, right)
}

// synthUnaryMinusCore synthesizes type for unary minus.
func (s *Synthesizer) synthUnaryMinusCore(ex *ast.UnaryMinusOpExpr, recurse ExprSynth) typ.Type {
	operand := recurse(ex.Expr)
	return s.deps.Types.UnaryOp(s.deps.Ctx, "-", operand)
}

// expandValuesCore expands expression list to types using provided synthesis functions.
func (s *Synthesizer) expandValuesCore(exprs []ast.Expr, needed int, single func(ast.Expr) typ.Type, multi func(ast.Expr) []typ.Type) []typ.Type {
	if len(exprs) == 0 {
		return nil
	}
	result := make([]typ.Type, 0, exprListResultCapacity(exprs, needed))

	for i, expr := range exprs {
		if i == len(exprs)-1 && ast.CanProduceMultipleValues(expr) {
			result = append(result, multi(expr)...)
		} else {
			result = append(result, single(expr))
		}
	}

	for len(result) < needed {
		result = append(result, typ.Nil)
	}

	return result
}

func exprListResultCapacity(exprs []ast.Expr, needed int) int {
	if needed > len(exprs) {
		return needed
	}
	return len(exprs)
}

// expandValues expands expression list to types.
func (s *Synthesizer) expandValues(exprs []ast.Expr, needed int, p cfg.Point, narrower api.FlowOps) []typ.Type {
	return s.expandValuesCore(exprs, needed,
		func(expr ast.Expr) typ.Type { return s.SynthExpr(expr, p, narrower) },
		func(expr ast.Expr) []typ.Type { return s.MultiTypeOf(expr, p) },
	)
}

// expandValuesWithSpec expands expression list with spec-narrowed type lookup.
func (s *Synthesizer) expandValuesWithSpec(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	return s.expandValuesCore(exprs, needed,
		func(expr ast.Expr) typ.Type { return s.synthExprWithSpec(expr, p, specTypes) },
		func(expr ast.Expr) []typ.Type { return s.synthMultiWithSpec(expr, p, specTypes) },
	)
}

// synthExprWithSpec synthesizes expression type with spec-narrowed lookup.
func (s *Synthesizer) synthExprWithSpec(expr ast.Expr, p cfg.Point, specTypes api.SpecTypes) typ.Type {
	if expr == nil {
		return typ.Nil
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		multi := s.synthMultiWithSpec(call, p, specTypes)
		if len(multi) == 0 || multi[0] == nil {
			return typ.Unknown
		}
		return multi[0]
	}
	if ident, ok := expr.(*ast.IdentExpr); ok {
		if sym := s.LookupSymbol(ident); sym != 0 {
			if t, exists := specTypes[sym]; exists {
				return t
			}
		}
	}
	sc := s.deps.ScopeAt(p)
	recurse := func(ex ast.Expr) typ.Type { return s.synthExprWithSpec(ex, p, specTypes) }
	if attr, ok := expr.(*ast.AttrGetExpr); ok {
		return s.synthAttrGetCoreWithCaptureTypes(attr, p, sc, nil, recurse, specTypes)
	}
	return s.synthExprCore(expr, sc, p, nil, recurse)
}

// synthMultiWithSpec synthesizes multi-return expression with spec-narrowed lookup.
func (s *Synthesizer) synthMultiWithSpec(expr ast.Expr, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	sc := s.deps.ScopeAt(p)
	recurse := func(ex ast.Expr) typ.Type { return s.synthExprWithSpec(ex, p, specTypes) }
	return s.synthMultiCore(expr, sc, recurse,
		func(call *ast.FuncCallExpr) []typ.Type {
			if call.Receiver != nil {
				if recvIdent, ok := call.Receiver.(*ast.IdentExpr); ok {
					if sym := s.LookupSymbol(recvIdent); sym != 0 {
						if recvType, exists := specTypes[sym]; exists {
							return s.SynthCallWithReceiverType(call, p, sc, recvType, recurse)
						}
					}
				}
			}
			return s.synthCallCoreWithCaptureTypes(call, p, sc, nil, recurse, nil, specTypes)
		},
	)
}

// enrichWithManifest enriches a field type with manifest information.
func enrichWithManifest(manifests io.ManifestQuerier, ft typ.Type, modulePath, fieldName string) typ.Type {
	if manifests == nil {
		return ft
	}
	manifest := io.LookupManifest(manifests, modulePath)
	if manifest == nil {
		return ft
	}

	if enriched, ok := manifest.LookupValue(fieldName); ok && enriched != nil {
		return enriched
	}
	return ft
}
