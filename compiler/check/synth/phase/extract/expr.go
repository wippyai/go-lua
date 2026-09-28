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
	"math/big"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/numparse"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// keyTypeAt types index keys for path extraction at p.
func (s *Synthesizer) keyTypeAt(p cfg.Point, narrower api.FlowOps) func(ast.Expr) typ.Type {
	return func(key ast.Expr) typ.Type { return s.SynthExpr(key, p, narrower) }
}

// synthAttrGetCore synthesizes type for attribute access using the shared core.
func (s *Synthesizer) synthAttrGetCore(ex *ast.AttrGetExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	objType := recurse(ex.Object)
	var manifestPath string
	var importedSymbol compcfg.SymbolID
	if ident, ok := ex.Object.(*ast.IdentExpr); ok && s.deps.Manifests != nil && s.deps.CheckCtx != nil {
		if bindings := s.deps.CheckCtx.Bindings(); bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok {
					graph.EachAliasSymbol(sym, func(candidate compcfg.SymbolID) bool {
						if module := s.deps.CheckCtx.ModuleAlias(candidate); module != "" {
							manifestPath, importedSymbol = module, candidate
							return true
						}
						return false
					})
				} else {
					manifestPath, importedSymbol = s.deps.CheckCtx.ModuleAlias(sym), sym
				}
			}
		}
	}

	if narrower != nil && s.deps.Paths != nil {
		path := s.deps.Paths(p, ex, sc, recurse)
		if !path.IsEmpty() {
			narrowed := narrower.NarrowedTypeAt(p, path)
			if narrowed != nil {
				if _, cast := ex.Object.(*ast.CastExpr); cast && typ.IsAny(unwrap.Alias(objType)) && querycore.AssignabilityOf(s.deps.Ctx) != subtype.Strict {
					goto skipNarrowedAttr
				}
				if specialized := s.stableLocalFunctionValueType(ex, p, sc, narrowed, nil); specialized != nil {
					return specialized
				}
				if typ.IsUnknown(unwrap.Alias(narrowed)) && typ.IsAny(unwrap.Alias(objType)) {
					goto skipNarrowedAttr
				}
				if key, ok := ex.Key.(*ast.StringExpr); ok && manifestPath != "" && importedFieldWritten(s.deps.CheckCtx, importedSymbol, key.Value) {
					return widenImportedLiteral(narrowed)
				}
				return narrowed
			}
		}
	}

skipNarrowedAttr:

	switch key := ex.Key.(type) {
	case *ast.StringExpr:
		if ft, ok := s.deps.Types.Field(s.deps.Ctx, objType, key.Value); ok {
			if manifestPath != "" {
				ft = enrichWithManifest(s.deps.Manifests, ft, manifestPath, key.Value)
				if importedFieldWritten(s.deps.CheckCtx, importedSymbol, key.Value) {
					ft = widenImportedLiteral(ft)
				}
			}
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, ft, nil); specialized != nil {
				return specialized
			}
			return ft
		}
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, typ.LiteralString(key.Value)); ok {
			if specialized := s.stableLocalFunctionValueType(ex, p, sc, it, nil); specialized != nil {
				return specialized
			}
			return it
		}
		// A complete record lists every field its table holds, so a field it
		// lacks reads as nil.
		if rec, ok := unwrap.Alias(objType).(*typ.Record); ok && rec.Complete && !rec.Declared {
			return typ.Nil
		}
	case *ast.NumberExpr:
		keyType := ops.ParseNumber(key.Value)
		if it, ok := s.deps.Types.Index(s.deps.Ctx, objType, keyType); ok {
			// Lua's positive length is a border: the slot at exactly that
			// index exists, though earlier slots may still be holes.
			if narrower != nil && s.deps.Paths != nil {
				if index, valid := numparse.ParseIntegerLiteral(key.Value); valid && index > 0 {
					tablePath := s.deps.Paths(p, ex.Object, sc, recurse)
					if length, proved := narrower.ExactLengthAt(p, tablePath); proved && length == index {
						if present := narrow.RemoveNil(it); !typ.IsNever(present) {
							return present
						}
						// A stale empty-record projection contains no element type;
						// the assertion proves presence but not the value's shape.
						return typ.Unknown
					}
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
				if narrowedResult := s.narrowTupleIndex(objType, key, it, p, narrower, recurse); narrowedResult != nil {
					return narrowedResult
				}
				if narrowedResult := s.narrowArrayIndexByLenBound(it, ex.Object, key.Value, 0, p, sc, narrower); narrowedResult != nil {
					return narrowedResult
				}
				// Check for KeyOf constraint to unwrap optional on map index
				if opt, ok := it.(*typ.Optional); ok && s.deps.Paths != nil && s.deps.CheckCtx != nil {
					if tablePath := s.deps.Paths(p, ex.Object, sc, recurse); !tablePath.IsEmpty() {
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
					if tablePath := s.deps.Paths(p, ex.Object, sc, recurse); !tablePath.IsEmpty() {
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
				if narrowedResult := s.narrowTupleIndex(objType, ex.Key, it, p, narrower, recurse); narrowedResult != nil {
					return narrowedResult
				}
				// #t denotes a present last slot after a positive length
				// assertion for the same version of t.
				if length, ok := ex.Key.(*ast.UnaryLenOpExpr); ok && s.deps.Paths != nil {
					tablePath := s.deps.Paths(p, ex.Object, sc, recurse)
					lengthPath := s.deps.Paths(p, length.Expr, sc, recurse)
					if !tablePath.IsEmpty() && tablePath.Equal(lengthPath) && narrower.HasLengthAtLeast(p, tablePath, 1) {
						if present := narrow.RemoveNil(it); !typ.IsNever(present) {
							return present
						}
					}
				}
				if varName, offset, ok := indexVarOffsetFromExpr(ex.Key); ok {
					if narrowedResult := s.narrowArrayIndexByLenBound(it, ex.Object, varName, offset, p, sc, narrower); narrowedResult != nil {
						return narrowedResult
					}
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

// A writable imported field cannot remain a singleton after a write in the
// current function. Conservatively include writes on every CFG path, including
// loop back-edges, when resolving a literal carried by an imported manifest.
func importedFieldWritten(env api.BaseEnv, sym compcfg.SymbolID, field string) bool {
	if env == nil || env.Bindings() == nil || sym == 0 {
		return false
	}
	graph, ok := env.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return false
	}
	return ImportedFieldMayChange(graph, env.Bindings(), sym, field)
}

// ImportedFieldMayChange reports writes through stable aliases and escapes of
// an imported table. Escapes conservatively invalidate every singleton field.
func ImportedFieldMayChange(graph *compcfg.Graph, bindings *bind.BindingTable, sym compcfg.SymbolID, field string) bool {
	// DirectAliasSymbol is computed from the whole graph and follows stable
	// local alias chains. A write through any such alias reaches the import.
	aliases := make(map[compcfg.SymbolID]bool)
	for candidate := range graph.AllSymbolIDs() {
		graph.EachAliasSymbol(candidate, func(source compcfg.SymbolID) bool {
			if source == sym {
				aliases[candidate] = true
			}
			return false
		})
	}
	aliases[sym] = true
	written := false
	graph.EachAssign(func(_ compcfg.Point, info *compcfg.AssignInfo) {
		if written || info == nil {
			return
		}
		for _, target := range info.Targets {
			if !aliases[target.BaseSymbol] {
				continue
			}
			if target.Kind == compcfg.TargetField && len(target.FieldPath) > 0 && target.FieldPath[0] == field {
				written = true
				return
			}
			if target.Kind == compcfg.TargetIndex {
				if key, ok := target.Key.(*ast.StringExpr); !ok || key.Value == field {
					written = true
					return
				}
			}
		}
		// A local direct alias stays tracked above. Any other assignment
		// containing the table lets an untracked reference retain it.
		info.EachTargetSource(func(_ int, target compcfg.AssignTarget, source ast.Expr) {
			if written || !importedAliasInValue(source, bindings, aliases) {
				return
			}
			if target.Kind != compcfg.TargetIdent || !aliases[target.Symbol] || graph.DirectAliasSymbol(target.Symbol) == 0 {
				written = true
			}
		})
	})
	graph.EachCallSite(func(_ compcfg.Point, call *compcfg.CallInfo) {
		if written || call == nil {
			return
		}
		for _, arg := range call.Args {
			if importedAliasInValue(arg, bindings, aliases) {
				written = true
				return
			}
		}
	})
	graph.EachReturn(func(_ compcfg.Point, ret *compcfg.ReturnInfo) {
		if written || ret == nil {
			return
		}
		for _, expr := range ret.Exprs {
			if importedAliasInValue(expr, bindings, aliases) {
				written = true
				return
			}
		}
	})
	// A closure can retain the table and mutate it outside this graph.
	for _, nested := range graph.NestedFunctions() {
		if nested.Func == nil {
			continue
		}
		for _, captured := range bindings.CapturedSymbols(nested.Func) {
			if aliases[captured] {
				return true
			}
		}
	}
	return written
}

func importedAliasInValue(expr ast.Expr, bindings *bind.BindingTable, aliases map[compcfg.SymbolID]bool) bool {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym, ok := bindings.SymbolOf(e)
		return ok && aliases[sym]
	case *ast.TableExpr:
		for _, f := range e.Fields {
			if f != nil && (importedAliasInValue(f.Key, bindings, aliases) || importedAliasInValue(f.Value, bindings, aliases)) {
				return true
			}
		}
	case *ast.CastExpr:
		return importedAliasInValue(e.Expr, bindings, aliases)
	case *ast.NonNilAssertExpr:
		return importedAliasInValue(e.Expr, bindings, aliases)
	}
	return false
}

func widenImportedLiteral(t typ.Type) typ.Type {
	lit, ok := typ.UnwrapAnnotated(t).(*typ.Literal)
	if !ok {
		return t
	}
	switch lit.Base {
	case kind.String:
		return typ.String
	case kind.Integer:
		return typ.Integer
	case kind.Number:
		return typ.Number
	case kind.Boolean:
		return typ.Boolean
	default:
		return typ.Unknown
	}
}

func (s *Synthesizer) indexFromKeyOf(objType typ.Type, objExpr ast.Expr, key *ast.IdentExpr, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	if s == nil || key == nil || narrower == nil || s.deps.Paths == nil || s.deps.CheckCtx == nil {
		return nil
	}
	bindings := s.deps.CheckCtx.Bindings()
	if bindings == nil {
		return nil
	}
	tablePath := s.deps.Paths(p, objExpr, sc, s.keyTypeAt(p, narrower))
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
		if present := narrow.RemoveNil(it); !typ.IsNever(present) {
			return present
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

// narrowTupleIndex removes the absent case only when every value of the index
// expression lies within the tuple's one-based slots.
func (s *Synthesizer) narrowTupleIndex(objType typ.Type, key ast.Expr, indexResult typ.Type, p cfg.Point, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	tuple, ok := unwrap.Alias(objType).(*typ.Tuple)
	if !ok || len(tuple.Elements) == 0 {
		return nil
	}

	if narrower == nil {
		return nil
	}

	bounds, hasBounds := tupleIndexInterval(key, p, narrower, recurse)
	if !hasBounds {
		return nil
	}

	tupleLen := int64(len(tuple.Elements))
	if bounds.Lower >= 1 && bounds.Upper <= tupleLen {
		narrowed := narrow.RemoveNil(indexResult)
		if !typ.IsNever(narrowed) {
			return narrowed
		}
	}

	return nil
}

// tupleIndexInterval evaluates only integer expressions that the numeric flow
// solution can bound. Failed arithmetic or an unknown leaf leaves the index
// optional; overflow must never wrap into a seemingly valid tuple slot.
func tupleIndexInterval(expr ast.Expr, p cfg.Point, narrower api.FlowOps, recurse ExprSynth) (numeric.Interval, bool) {
	if value, ok := intConstFromExpr(expr); ok {
		return numeric.Interval{Lower: value, Upper: value}, true
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		lower, upper, ok := narrower.BoundsAt(p, e.Value)
		return numeric.Interval{Lower: lower, Upper: upper}, ok
	case *ast.UnaryLenOpExpr:
		if recurse != nil {
			if tuple, ok := unwrap.Alias(recurse(e.Expr)).(*typ.Tuple); ok {
				length := int64(len(tuple.Elements))
				return numeric.Interval{Lower: length, Upper: length}, true
			}
		}
	case *ast.UnaryMinusOpExpr:
		value, ok := tupleIndexInterval(e.Expr, p, narrower, recurse)
		if ok {
			lower, lowOK := checkedIndexOp(0, value.Upper, "-")
			upper, highOK := checkedIndexOp(0, value.Lower, "-")
			return numeric.Interval{Lower: lower, Upper: upper}, lowOK && highOK
		}
	case *ast.ArithmeticOpExpr:
		if e.Operator == "%" {
			right, ok := tupleIndexInterval(e.Rhs, p, narrower, recurse)
			if !ok || right.Lower != right.Upper || right.Lower <= 0 {
				return numeric.Interval{}, false
			}
			_, bounded := tupleIndexInterval(e.Lhs, p, narrower, recurse)
			if bounded || recurse != nil && subtype.IsSubtype(recurse(e.Lhs), typ.Integer) {
				// Lua modulo by a positive integer lies in [0, k-1] for
				// every integer dividend, even without finite input bounds.
				return numeric.Interval{Lower: 0, Upper: right.Lower - 1}, true
			}
			return numeric.Interval{}, false
		}
		if e.Operator != "+" && e.Operator != "-" && e.Operator != "*" {
			return numeric.Interval{}, false
		}
		left, leftOK := tupleIndexInterval(e.Lhs, p, narrower, recurse)
		right, rightOK := tupleIndexInterval(e.Rhs, p, narrower, recurse)
		if !leftOK || !rightOK {
			return numeric.Interval{}, false
		}
		var pairs [][2]int64
		switch e.Operator {
		case "+":
			pairs = [][2]int64{{left.Lower, right.Lower}, {left.Upper, right.Upper}}
		case "-":
			pairs = [][2]int64{{left.Lower, right.Upper}, {left.Upper, right.Lower}}
		case "*":
			pairs = [][2]int64{{left.Lower, right.Lower}, {left.Lower, right.Upper}, {left.Upper, right.Lower}, {left.Upper, right.Upper}}
		}
		var result numeric.Interval
		for i, pair := range pairs {
			value, ok := checkedIndexOp(pair[0], pair[1], e.Operator)
			if !ok {
				return numeric.Interval{}, false
			}
			if i == 0 || value < result.Lower {
				result.Lower = value
			}
			if i == 0 || value > result.Upper {
				result.Upper = value
			}
		}
		return result, true
	}
	return numeric.Interval{}, false
}

func checkedIndexOp(a, b int64, op string) (int64, bool) {
	value := big.NewInt(a)
	other := big.NewInt(b)
	switch op {
	case "+":
		value.Add(value, other)
	case "-":
		value.Sub(value, other)
	case "*":
		value.Mul(value, other)
	}
	return value.Int64(), value.IsInt64()
}

func (s *Synthesizer) narrowArrayIndexByLenBound(indexResult typ.Type, objExpr ast.Expr, varName string, offset int64, p cfg.Point, sc *scope.State, narrower api.FlowOps) typ.Type {
	opt, ok := indexResult.(*typ.Optional)
	if !ok || narrower == nil || s == nil || s.deps.Paths == nil {
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
	tablePath := s.deps.Paths(p, objExpr, sc, s.keyTypeAt(p, narrower))
	if tablePath.IsEmpty() {
		return nil
	}
	if string(tablePath.Key()) != arrKey {
		return nil
	}
	if lenOffset > -offset {
		return nil
	}
	return opt.Inner
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

// synthLogicalOpCore synthesizes type for logical operators.
func (s *Synthesizer) synthLogicalOpCore(ex *ast.LogicalOpExpr, recurse ExprSynth) typ.Type {
	left := recurse(ex.Lhs)
	right := recurse(ex.Rhs)

	switch ex.Operator {
	case "and":
		if querycore.AssignabilityOf(s.deps.Ctx) == subtype.Strict && typ.IsAny(left) {
			// The only way the left operand survives `and` is as nil or false.
			// In strict mode, preserve that fact instead of propagating any.
			return typ.JoinBranchOutcome(narrow.ToFalsy(left), right)
		}
		return ops.LogicalAndTyped(left, right)
	case "or":
		return ops.LogicalOrTyped(left, right)
	default:
		return typ.Unknown
	}
}

// assumingFlowOps views flow ops with an extra condition holding at every
// point.
type assumingFlowOps struct {
	inner api.FlowOps
	extra constraint.Condition
}

// assumeFlow returns flow ops in which cond holds at every point in addition to
// what ops already establishes.
func assumeFlow(ops api.FlowOps, cond constraint.Condition) api.FlowOps {
	if a, ok := ops.(*assumingFlowOps); ok {
		return &assumingFlowOps{inner: a.inner, extra: constraint.And(a.extra, cond)}
	}
	return &assumingFlowOps{inner: ops, extra: cond}
}

func (a *assumingFlowOps) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	return a.inner.NarrowedTypeAssuming(p, path, a.extra)
}

func (a *assumingFlowOps) NarrowedTypeAssuming(p cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type {
	return a.inner.NarrowedTypeAssuming(p, path, constraint.And(a.extra, extra))
}

func (a *assumingFlowOps) BoundsAt(p cfg.Point, name string) (int64, int64, bool) {
	return a.inner.BoundsAt(p, name)
}

func (a *assumingFlowOps) ArrayLenBoundAt(p cfg.Point, varName string) (string, bool) {
	return a.inner.ArrayLenBoundAt(p, varName)
}

func (a *assumingFlowOps) ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (string, int64, bool) {
	return a.inner.ArrayLenBoundWithOffsetAt(p, varName)
}
func (a *assumingFlowOps) HasLengthAtLeast(p cfg.Point, path constraint.Path, minimum int64) bool {
	return a.inner.HasLengthAtLeast(p, path, minimum)
}

func (a *assumingFlowOps) ExactLengthAt(p cfg.Point, path constraint.Path) (int64, bool) {
	return a.inner.ExactLengthAt(p, path)
}

func (a *assumingFlowOps) IsPointDead(p cfg.Point) bool {
	return a.inner.IsPointDead(p)
}

func (a *assumingFlowOps) HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool {
	return a.inner.HasKeyOfAssuming(p, tablePath, keyPath, a.extra)
}

func (a *assumingFlowOps) HasKeyOfAssuming(p cfg.Point, tablePath, keyPath constraint.Path, extra constraint.Condition) bool {
	return a.inner.HasKeyOfAssuming(p, tablePath, keyPath, constraint.And(a.extra, extra))
}

// synthLogicalOpWithNarrowing synthesizes a logical operator whose right
// operand is typed under the condition the left operand establishes: its truthy
// condition for `and`, its falsy condition for `or`. The condition is the one a
// branch on the left operand puts on its edge, applied by the flow solution the
// same way, so `type(x) == "table" and x.f` types x.f exactly as
// `if type(x) == "table" then ... x.f ... end` does.
func (s *Synthesizer) synthLogicalOpWithNarrowing(ex *ast.LogicalOpExpr, p cfg.Point, narrower api.FlowOps, recurse ExprSynth) typ.Type {
	if s.deps.Conditions == nil {
		return s.synthLogicalOpCore(ex, recurse)
	}
	onTrue, onFalse := s.deps.Conditions(p, ex.Lhs)
	var cond constraint.Condition
	switch ex.Operator {
	case "and":
		cond = onTrue
	case "or":
		cond = onFalse
	default:
		return s.synthLogicalOpCore(ex, recurse)
	}
	if !cond.HasConstraints() {
		return s.synthLogicalOpCore(ex, recurse)
	}
	left := recurse(ex.Lhs)
	right := s.SynthExpr(ex.Rhs, p, assumeFlow(narrower, cond))
	if ex.Operator == "and" {
		if querycore.AssignabilityOf(s.deps.Ctx) == subtype.Strict && typ.IsAny(left) {
			return typ.JoinBranchOutcome(narrow.ToFalsy(left), right)
		}
		return ops.LogicalAndTyped(left, right)
	}
	return ops.LogicalOrTyped(left, right)
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

// ExpandValuesUsing expands expression list with the supplied phase's synthesis functions.
func (s *Synthesizer) ExpandValuesUsing(exprs []ast.Expr, needed int, single func(ast.Expr) typ.Type, multi func(ast.Expr) []typ.Type, sc *scope.State) []typ.Type {
	if len(exprs) == 0 {
		return nil
	}
	result := make([]typ.Type, 0, needed)

	last := exprs[len(exprs)-1]
	for i, expr := range exprs {
		if i == len(exprs)-1 {
			result = append(result, multi(expr)...)
		} else {
			result = append(result, single(expr))
		}
	}

	pad := typ.Type(typ.Nil)
	if len(result) < needed {
		if rest := openValueRest(last, single, sc); rest != nil {
			pad = rest
		}
	}
	for len(result) < needed {
		result = append(result, pad)
	}

	return result
}

// openValueRest returns the type of the values an expression yields past the
// ones its type states, when their number is not known: a call to a function
// value typed any or unknown, or a vararg expression. It returns nil when the
// expression yields exactly the values its type states.
func openValueRest(expr ast.Expr, single func(ast.Expr) typ.Type, sc *scope.State) typ.Type {
	switch ex := expr.(type) {
	case *ast.FuncCallExpr:
		target := ex.Func
		if ex.Receiver != nil {
			target = ex.Receiver
		}
		if target == nil {
			return nil
		}
		callee := unwrap.Alias(single(target))
		if typ.IsAny(callee) || typ.IsUnknown(callee) {
			return callee
		}
	case *ast.Comma3Expr:
		vt := sc.VariadicType()
		if vt == nil {
			return typ.Unknown
		}
		return typ.NewOptional(vt)
	}
	return nil
}

// expandValues expands expression list to types.
func (s *Synthesizer) expandValues(exprs []ast.Expr, needed int, p cfg.Point, narrower api.FlowOps) []typ.Type {
	return s.ExpandValuesUsing(exprs, needed,
		func(expr ast.Expr) typ.Type { return s.SynthExpr(expr, p, narrower) },
		func(expr ast.Expr) []typ.Type { return s.MultiTypeOf(expr, p) },
		s.deps.ScopeAt(p),
	)
}

// expandValuesWithSpec expands expression list with spec-narrowed type lookup.
func (s *Synthesizer) expandValuesWithSpec(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	return s.ExpandValuesUsing(exprs, needed,
		func(expr ast.Expr) typ.Type { return s.synthExprWithSpec(expr, p, specTypes) },
		func(expr ast.Expr) []typ.Type { return s.synthMultiWithSpec(expr, p, specTypes) },
		s.deps.ScopeAt(p),
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
