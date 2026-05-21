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
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/numparse"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type localTypeOverride struct {
	path constraint.Path
	t    typ.Type
}

// localNarrowOps wraps a api.FlowOps and overrides NarrowedTypeAt for
// expression-local facts proven by short-circuit evaluation.
type localNarrowOps struct {
	inner        api.FlowOps
	overridePath constraint.Path
	overrideType typ.Type
	overrides    []localTypeOverride
}

func (n *localNarrowOps) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if !n.overridePath.IsEmpty() && path.Equal(n.overridePath) {
		return n.overrideType
	}
	for i := len(n.overrides) - 1; i >= 0; i-- {
		if path.Equal(n.overrides[i].path) {
			return n.overrides[i].t
		}
	}
	if n.inner != nil {
		return n.inner.NarrowedTypeAt(p, path)
	}
	return nil
}

func (n *localNarrowOps) BoundsAt(p cfg.Point, name string) (int64, int64, bool) {
	if n.inner != nil {
		return n.inner.BoundsAt(p, name)
	}
	return 0, 0, false
}

func (n *localNarrowOps) ArrayLenBoundAt(p cfg.Point, varName string) (string, bool) {
	if n.inner != nil {
		return n.inner.ArrayLenBoundAt(p, varName)
	}
	return "", false
}

func (n *localNarrowOps) ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (string, int64, bool) {
	if n.inner != nil {
		return n.inner.ArrayLenBoundWithOffsetAt(p, varName)
	}
	return "", 0, false
}

func (n *localNarrowOps) LengthBoundsAt(p cfg.Point, path constraint.Path) (int64, int64, bool) {
	if n.inner != nil {
		return n.inner.LengthBoundsAt(p, path)
	}
	return 0, 0, false
}

func (n *localNarrowOps) IsPointDead(p cfg.Point) bool {
	if n.inner != nil {
		return n.inner.IsPointDead(p)
	}
	return false
}

func (n *localNarrowOps) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	if n.inner != nil {
		return n.inner.HasKeyOf(p, tablePath, keyPath)
	}
	return false
}

// synthAttrGetCore synthesizes type for attribute access using the shared core.
func (s *Synthesizer) synthAttrGetCore(ex *ast.AttrGetExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	objType := recurse(ex.Object)

	if narrower != nil && s.deps.Paths != nil {
		path := s.deps.Paths(p, ex, sc)
		if !path.IsEmpty() {
			narrowed := narrower.NarrowedTypeAt(p, path)
			if narrowed != nil {
				if key, ok := ex.Key.(*ast.NumberExpr); ok && narrow.NilPresenceIsOnlyFlowUncertainty(narrowed) && s.literalLengthBoundProvesIndex(objType, ex.Object, key.Value, p, sc, narrower) {
					goto skipNarrowedAttr
				}
				if refined := s.narrowArrayIndexByLengthExpr(objType, narrowed, ex.Object, ex.Key, p, sc, narrower); refined != nil {
					return refined
				}
				if specialized := s.stableLocalFunctionValueType(ex, p, sc, narrowed, nil); specialized != nil {
					return specialized
				}
				if typ.IsUnknown(unwrap.Alias(narrowed)) && typ.IsAny(unwrap.Alias(objType)) {
					goto skipNarrowedAttr
				}
				if key, ok := ex.Key.(*ast.StringExpr); ok {
					if declaredField, ok := s.deps.Types.Field(s.deps.Ctx, objType, key.Value); ok && declaredField != nil {
						refined, ok := s.refineNarrowedFieldFact(narrowed, declaredField)
						if !ok {
							goto skipNarrowedAttr
						}
						narrowed = refined
					}
				}
				return narrowed
			}
		}
	}

skipNarrowedAttr:

	var manifestPath string
	if ident, ok := ex.Object.(*ast.IdentExpr); ok && s.deps.Manifests != nil && s.deps.CheckCtx != nil {
		if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				manifestPath = s.deps.CheckCtx.ModuleAlias(sym)
			}
		}
	}

	switch key := ex.Key.(type) {
	case *ast.StringExpr:
		if ft, ok := s.deps.Types.Field(s.deps.Ctx, objType, key.Value); ok {
			if manifestPath != "" {
				ft = enrichWithManifest(s.deps.Manifests, ft, manifestPath, key.Value)
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, ft, nil); specialized != nil {
				return specialized
			}
			return ft
		}
		if ft := fieldOnPartialUnion(objType, key.Value, s.deps.Types, s.deps.Ctx); ft != nil {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, ft, nil); specialized != nil {
				return specialized
			}
			return ft
		}
		if vt := mapValueType(objType); vt != nil {
			return vt
		}
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, typ.LiteralString(key.Value)); ok {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
	case *ast.NumberExpr:
		keyType := ops.ParseNumber(key.Value)
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType); ok {
			if narrower != nil {
				if narrowedResult := s.narrowArrayIndexByLiteralLenBound(objType, it, ex.Object, key.Value, p, sc, narrower); narrowedResult != nil {
					return narrowedResult
				}
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
	case *ast.IdentExpr:
		keyType := recurse(key)
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType); ok {
			if narrower != nil {
				if narrowedResult := s.narrowTupleIndex(objType, key.Value, it, p, narrower); narrowedResult != nil {
					return narrowedResult
				}
				if narrowedResult := s.narrowArrayIndexByLenBound(objType, it, ex.Object, key.Value, 0, p, sc, narrower); narrowedResult != nil {
					return narrowedResult
				}
				// Check for KeyOf constraint to unwrap optional on map index
				if opt, ok := it.(*typ.Optional); ok && s.deps.Paths != nil && s.deps.CheckCtx != nil {
					if tablePath := s.deps.Paths(p, ex.Object, sc); !tablePath.IsEmpty() {
						if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
							if keySym, ok := bindings.SymbolOf(key); ok && keySym != 0 {
								keyPath := constraint.Path{Root: key.Value, Symbol: keySym}
								if narrower.HasKeyOf(p, tablePath, keyPath) {
									return opt.Inner
								}
							}
						}
					}
				}
				if s.deps.Paths != nil && s.deps.CheckCtx != nil {
					if tablePath := s.deps.Paths(p, ex.Object, sc); !tablePath.IsEmpty() {
						if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
							if keySym, ok := bindings.SymbolOf(key); ok && keySym != 0 {
								keyPath := constraint.Path{Root: key.Value, Symbol: keySym}
								if narrower.HasKeyOf(p, tablePath, keyPath) {
									if refined := narrow.RemoveNil(it); !typ.IsNever(refined) {
										it = refined
									}
								}
							}
						}
					}
				}
				if derived := s.indexFromKeyOf(objType, ex.Object, key, p, sc, narrower); derived != nil {
					if keyType != nil && keyType.Kind().IsPlaceholder() {
						return derived
					}
					if shouldPreferKeyOfIndex(it) {
						return derived
					}
				}
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
		if derived := s.indexFromKeyOf(objType, ex.Object, key, p, sc, narrower); derived != nil {
			return derived
		}
	default:
		keyType := recurse(ex.Key)
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType); ok {
			if narrower != nil {
				if varName, offset, ok := indexVarOffsetFromExpr(ex.Key); ok {
					if narrowedResult := s.narrowArrayIndexByLenBound(objType, it, ex.Object, varName, offset, p, sc, narrower); narrowedResult != nil {
						return narrowedResult
					}
				}
				if narrowedResult := s.narrowArrayIndexByLengthExpr(objType, it, ex.Object, ex.Key, p, sc, narrower); narrowedResult != nil {
					return narrowedResult
				}
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
	}

	return typ.Unknown
}

func (s *Synthesizer) refineNarrowedFieldFact(narrowed, declared typ.Type) (typ.Type, bool) {
	if narrowed == nil || declared == nil {
		return narrowed, true
	}
	declared = unwrap.Alias(declared)
	narrowed = unwrap.Alias(narrowed)
	if declared == nil || narrowed == nil {
		return narrowed, true
	}
	if declared.Kind().IsPlaceholder() {
		return narrowed, true
	}
	if s.deps.Types != nil {
		if s.deps.Types.IsSubtype(s.deps.Ctx, narrowed, declared) {
			return narrowed, true
		}
		declaredNonNil := narrow.RemoveNil(declared)
		if !typ.IsNever(declaredNonNil) {
			if s.deps.Types.IsSubtype(s.deps.Ctx, declaredNonNil, narrowed) {
				return declaredNonNil, true
			}
			if unwrap.Function(declaredNonNil) != nil && unwrap.Function(narrowed) != nil {
				return declaredNonNil, true
			}
		}
	}
	return nil, false
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
	keyPath := constraint.Path{Root: key.Value, Symbol: keySym}
	if !narrower.HasKeyOf(p, tablePath, keyPath) {
		return nil
	}
	tableType := objType
	if tableType == nil || tableType.Kind().IsPlaceholder() {
		if narrowed := narrower.NarrowedTypeAt(p, tablePath); narrowed != nil {
			tableType = narrowed
		}
	}
	derivedKey := querycore.KeyType(tableType)
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

// narrowTupleIndex checks if a tuple index can be narrowed using integer bounds.
func (s *Synthesizer) narrowTupleIndex(objType typ.Type, varName string, indexResult typ.Type, p cfg.Point, narrower api.FlowOps) typ.Type {
	tuple, ok := unwrap.Alias(objType).(*typ.Tuple)
	if !ok || len(tuple.Elements) == 0 {
		return nil
	}

	if narrower == nil {
		return nil
	}

	lower, upper, hasBounds := narrower.BoundsAt(p, varName)
	if !hasBounds {
		return nil
	}

	tupleLen := int64(len(tuple.Elements))
	if lower >= 1 && upper <= tupleLen {
		narrowed := narrow.RemoveNil(indexResult)
		if !typ.IsNever(narrowed) {
			return narrowed
		}
	}

	return nil
}

func (s *Synthesizer) narrowArrayIndexByLenBound(objType, indexResult typ.Type, objExpr ast.Expr, varName string, offset int64, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	if narrower == nil || s == nil || s.deps.Paths == nil {
		return nil
	}
	lower, _, hasBounds := narrower.BoundsAt(p, varName)
	if !hasBounds {
		return nil
	}
	if lower+offset < 1 {
		return nil
	}
	arrKey, lenOffset, hasLenRef := narrower.ArrayLenBoundWithOffsetAt(p, varName)
	if !hasLenRef {
		return nil
	}
	tablePath := s.deps.Paths(p, objExpr, sc)
	if tablePath.IsEmpty() {
		return nil
	}
	if string(tablePath.Key()) != arrKey {
		return nil
	}
	if lenOffset > -offset {
		return nil
	}
	return narrow.RefineSequenceIndex(objType, indexResult, lower+offset)
}

func (s *Synthesizer) narrowArrayIndexByLiteralLenBound(objType, indexResult typ.Type, objExpr ast.Expr, indexLiteral string, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	index, ok := numparse.ParseIntegerLiteral(indexLiteral)
	if !ok {
		return nil
	}
	if !s.literalLengthBoundProvesIndex(objType, objExpr, indexLiteral, p, sc, narrower) {
		return nil
	}
	return narrow.RefineSequenceIndex(objType, indexResult, index)
}

func (s *Synthesizer) narrowArrayIndexByLengthExpr(objType, indexResult typ.Type, objExpr, keyExpr ast.Expr, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	if narrower == nil || s == nil || s.deps.Paths == nil {
		return nil
	}
	tablePath := s.deps.Paths(p, objExpr, sc)
	if tablePath.IsEmpty() {
		return nil
	}
	lenPath, offset, ok := lenIndexPathFromExpr(keyExpr, p, s.deps.Paths, sc)
	if !ok || !lenPath.Equal(tablePath) {
		return nil
	}
	lower, _, ok := narrower.LengthBoundsAt(p, tablePath)
	if !ok {
		return nil
	}
	return narrow.RefineLengthIndex(objType, indexResult, lower, offset)
}

func lenIndexPathFromExpr(expr ast.Expr, p cfg.Point, paths func(cfg.Point, ast.Expr, *scope.State) constraint.Path, sc *scope.State) (constraint.Path, int64, bool) {
	switch e := expr.(type) {
	case *ast.UnaryLenOpExpr:
		path := paths(p, e.Expr, sc)
		return path, 0, !path.IsEmpty()
	case *ast.ArithmeticOpExpr:
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		path, offset, ok := lenIndexPathFromExpr(e.Lhs, p, paths, sc)
		if !ok {
			return constraint.Path{}, 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return path, offset + k, true
	}
	return constraint.Path{}, 0, false
}

func (s *Synthesizer) literalLengthBoundProvesIndex(objType typ.Type, objExpr ast.Expr, indexLiteral string, p cfg.Point, sc *scope.State, narrower api.FlowOps) bool {
	if narrower == nil || s == nil || s.deps.Paths == nil {
		return false
	}
	index, ok := numparse.ParseIntegerLiteral(indexLiteral)
	if !ok || index < 1 {
		return false
	}
	tablePath := s.deps.Paths(p, objExpr, sc)
	if tablePath.IsEmpty() {
		return false
	}
	lower, _, ok := narrower.LengthBoundsAt(p, tablePath)
	return ok && lower >= index && narrow.LengthBoundProvesSequenceIndex(objType, index)
}

func indexVarOffsetFromExpr(expr ast.Expr) (string, int64, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if e.Value == "" {
			return "", 0, false
		}
		return e.Value, 0, true
	case *ast.ArithmeticOpExpr:
		ident, ok := e.Lhs.(*ast.IdentExpr)
		if !ok || ident.Value == "" {
			return "", 0, false
		}
		if e.Operator != "+" && e.Operator != "-" {
			return "", 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return "", 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return ident.Value, k, true
	}
	return "", 0, false
}

func intConstFromExpr(expr ast.Expr) (int64, bool) {
	switch v := expr.(type) {
	case *ast.NumberExpr:
		return numparse.ParseIntegerLiteral(v.Value)
	case *ast.UnaryMinusOpExpr:
		if n, ok := intConstFromExpr(v.Expr); ok {
			return -n, true
		}
	}
	return 0, false
}

// fieldOnPartialUnion handles field access on unions where some but not all
// members have the field.
func fieldOnPartialUnion(t typ.Type, name string, types querycore.TypeOps, ctx *db.QueryContext) typ.Type {
	u, ok := unwrap.Alias(t).(*typ.Union)
	if !ok {
		return nil
	}

	var fieldTypes []typ.Type
	hasField := false

	for _, m := range u.Members {
		if ft, ok := types.Field(ctx, m, name); ok {
			fieldTypes = append(fieldTypes, ft)
			hasField = true
		} else {
			fieldTypes = append(fieldTypes, typ.Nil)
		}
	}

	if !hasField {
		return nil
	}

	return typ.NewUnion(fieldTypes...)
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

// synthLogicalOpWithNarrowing synthesizes logical op with LHS path narrowing in RHS.
// For `and`: RHS sees LHS narrowed to truthy.
// For `or`: RHS sees LHS narrowed to falsy.
func (s *Synthesizer) synthLogicalOpWithNarrowing(ex *ast.LogicalOpExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)

	if ex.Operator == "and" {
		if probe, ok := guard.ExtractTypeEqualityProbe(ex.Lhs); ok {
			probePath := s.logicalNarrowPath(p, probe.Expr, sc)
			if !probePath.IsEmpty() {
				wrapped := &localNarrowOps{
					inner:        narrower,
					overridePath: probePath,
					overrideType: guard.TypeForTypeKey(probe.Key),
				}
				right := s.SynthExpr(ex.Rhs, p, wrapped)
				return ops.LogicalAndTyped(left, right)
			}
		}
	}

	if ex.Operator == "and" {
		if overrides := s.truthyLocalOverrides(ex.Lhs, p, sc, narrower); len(overrides) > 0 {
			wrapped := &localNarrowOps{
				inner:     narrower,
				overrides: overrides,
			}
			right := s.SynthExpr(ex.Rhs, p, wrapped)
			return ops.LogicalAndTyped(left, right)
		}
	}
	if ex.Operator == "or" {
		if overrides := s.falsyLocalOverrides(ex.Lhs, p, sc, narrower); len(overrides) > 0 {
			wrapped := &localNarrowOps{
				inner:     narrower,
				overrides: overrides,
			}
			right := s.SynthExpr(ex.Rhs, p, wrapped)
			return ops.LogicalOrTyped(left, right)
		}
	}

	// Extract path for LHS expression
	lhsPath := s.logicalNarrowPath(p, ex.Lhs, sc)

	if !lhsPath.IsEmpty() && ops.CanBeFalsy(left) {
		var narrowedType typ.Type
		switch ex.Operator {
		case "and":
			narrowedType = narrow.ToTruthy(left)
		case "or":
			narrowedType = narrow.ToFalsy(left)
		}

		if !typ.IsNever(narrowedType) {
			wrapped := &localNarrowOps{
				inner:        narrower,
				overridePath: lhsPath,
				overrideType: narrowedType,
			}
			wrappedRecurse := func(expr ast.Expr) typ.Type {
				return s.SynthExpr(expr, p, wrapped)
			}
			right := wrappedRecurse(ex.Rhs)
			switch ex.Operator {
			case "and":
				return ops.LogicalAndTyped(left, right)
			case "or":
				return ops.LogicalOrTyped(left, right)
			}
		}
	}

	return s.synthLogicalOpCore(ex, recurse)
}

func (s *Synthesizer) truthyLocalOverrides(expr ast.Expr, p cfg.Point, sc *scope.State, narrower api.FlowOps) []localTypeOverride {
	return s.logicalLocalOverrides(expr, p, sc, narrower, true)
}

func (s *Synthesizer) falsyLocalOverrides(expr ast.Expr, p cfg.Point, sc *scope.State, narrower api.FlowOps) []localTypeOverride {
	return s.logicalLocalOverrides(expr, p, sc, narrower, false)
}

func (s *Synthesizer) logicalLocalOverrides(expr ast.Expr, p cfg.Point, sc *scope.State, narrower api.FlowOps, truthy bool) []localTypeOverride {
	if expr == nil {
		return nil
	}
	if logical, ok := expr.(*ast.LogicalOpExpr); ok {
		switch {
		case truthy && logical.Operator == "and":
			left := s.logicalLocalOverrides(logical.Lhs, p, sc, narrower, true)
			leftNarrower := composeLocalNarrower(narrower, left)
			right := s.logicalLocalOverrides(logical.Rhs, p, sc, leftNarrower, true)
			return append(left, right...)
		case !truthy && logical.Operator == "or":
			left := s.logicalLocalOverrides(logical.Lhs, p, sc, narrower, false)
			leftNarrower := composeLocalNarrower(narrower, left)
			right := s.logicalLocalOverrides(logical.Rhs, p, sc, leftNarrower, false)
			return append(left, right...)
		}
	}
	if truthy {
		if probe, ok := guard.ExtractTypeEqualityProbe(expr); ok {
			probePath := s.logicalNarrowPath(p, probe.Expr, sc)
			if !probePath.IsEmpty() {
				return []localTypeOverride{{
					path: probePath,
					t:    guard.TypeForTypeKey(probe.Key),
				}}
			}
		}
	}
	path := s.logicalNarrowPath(p, expr, sc)
	if path.IsEmpty() {
		return nil
	}
	t := s.SynthExpr(expr, p, narrower)
	if t == nil {
		return nil
	}
	var narrowed typ.Type
	if truthy {
		narrowed = narrow.ToTruthy(t)
	} else {
		narrowed = narrow.ToFalsy(t)
	}
	if typ.IsNever(narrowed) || typ.TypeEquals(narrowed, t) {
		return nil
	}
	return []localTypeOverride{{
		path: path,
		t:    narrowed,
	}}
}

func composeLocalNarrower(inner api.FlowOps, overrides []localTypeOverride) api.FlowOps {
	if len(overrides) == 0 {
		return inner
	}
	return &localNarrowOps{
		inner:     inner,
		overrides: overrides,
	}
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

func (s *Synthesizer) synthLogicalOpWithExpected(ex *ast.LogicalOpExpr, sc *scope.State, p cfg.Point, recurse ExprSynth, expected typ.Type) typ.Type {
	if ex == nil {
		return typ.Unknown
	}
	if expected == nil || ex.Operator != "or" && ex.Operator != "and" {
		return s.synthLogicalOpCore(ex, recurse)
	}

	branch := func(expr ast.Expr) typ.Type {
		if expr == nil {
			return typ.Unknown
		}
		return s.SynthExprWithExpectedCore(expr, sc, p, recurse, expected)
	}

	left := recurse(ex.Lhs)
	right := branch(ex.Rhs)
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
