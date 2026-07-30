package program

import (
	"github.com/wippyai/go-lua/__legacy/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type keyedFunction struct {
	funcExpr *ast.FunctionExpr
	key      summary.SummaryKey
}

type programKeys struct {
	rootKey                  summary.SummaryKey
	functions                []keyedFunction
	functionByKey            map[summary.SummaryKey]*ast.FunctionExpr
	functionKeys             map[symbol.ID]summary.SummaryKey
	functionIDs              map[identity.ID]summary.SummaryKey
	targetKeys               map[symbol.ID]summary.SummaryKey
	pathKeys                 map[factflow.CalleePathKey]summary.SummaryKey
	pathMultiKeys            map[factflow.CalleePathKey][]summary.SummaryKey
	functionTypes            map[summary.SummaryKey]*typ.Function
	metatableProof           metatableMethodProof
	metatableMethodReceivers map[symbol.ID]typ.Type
	bindings                 *bind.Result
}

// functionSymbol returns the function symbol owning fn.
func (k programKeys) functionSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if k.bindings == nil || fn == nil {
		return 0, false
	}
	return k.bindings.FunctionSymbol(fn)
}

func (k programKeys) summaryKeyForFunction(fn *ast.FunctionExpr) (summary.SummaryKey, bool) {
	sym, ok := k.functionSymbol(fn)
	if !ok || sym == 0 {
		return summary.SummaryKey{}, false
	}
	key, ok := k.functionKeys[sym]
	return key, ok
}

// functionSymbolsByKey inverts functionKeys so a resolved call summary key maps
// back to its callee function symbol for call-site parameter inference.
func collectKeys(bindings *bind.Result, root summary.SummaryKey, reg *axis.Registry, external typeannotation.Resolver, moduleExports importlookup.Source, stmts ...[]ast.Stmt) programKeys {
	return collectKeysWithPublicRoot(bindings, root, nil, reg, external, moduleExports, stmts...)
}

// collectFunctionKeys assigns the demanded function's public identity while
// collecting every binder-derived lookup. A RunFunction root is one lexical
// body, so its identity, target, and path bindings must all name the configured
// root key rather than also publishing the binder symbol key.
func collectFunctionKeys(bindings *bind.Result, root summary.SummaryKey, publicRoot *ast.FunctionExpr, reg *axis.Registry, external typeannotation.Resolver, moduleExports importlookup.Source, stmts ...[]ast.Stmt) programKeys {
	return collectKeysWithPublicRoot(bindings, root, publicRoot, reg, external, moduleExports, stmts...)
}

func collectKeysWithPublicRoot(bindings *bind.Result, root summary.SummaryKey, publicRoot *ast.FunctionExpr, reg *axis.Registry, external typeannotation.Resolver, moduleExports importlookup.Source, stmts ...[]ast.Stmt) programKeys {
	metatableContext := collectMetatableMethodContext(bindings, external, moduleExports, stmts...)
	out := programKeys{
		rootKey:                  root,
		functionByKey:            make(map[summary.SummaryKey]*ast.FunctionExpr),
		functionKeys:             make(map[symbol.ID]summary.SummaryKey),
		functionIDs:              make(map[identity.ID]summary.SummaryKey),
		targetKeys:               make(map[symbol.ID]summary.SummaryKey),
		pathKeys:                 make(map[factflow.CalleePathKey]summary.SummaryKey),
		pathMultiKeys:            make(map[factflow.CalleePathKey][]summary.SummaryKey),
		functionTypes:            make(map[summary.SummaryKey]*typ.Function),
		metatableProof:           metatableContext.proof,
		metatableMethodReceivers: metatableContext.methodReceivers,
		bindings:                 bindings,
	}
	if bindings == nil {
		return out
	}
	pathTargets := collectFunctionPathTargets(bindings, stmts...)
	ambiguousPathKeys := make(map[factflow.CalleePathKey]struct{})
	bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.Symbol == 0 || origin.Func == nil {
			return true
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		if origin.Func == publicRoot {
			key = root
		}
		out.functions = append(out.functions, keyedFunction{funcExpr: origin.Func, key: key})
		out.functionByKey[key] = origin.Func
		out.functionKeys[origin.Symbol] = key
		out.functionIDs[identity.LuaFunction(uint64(origin.Symbol))] = key
		if fnType, ok := lowerFunctionOriginType(origin, bindings, external, out.metatableProof); ok {
			out.functionTypes[key] = fnType
		}
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 && functionTargetCanUseDirectSymbolKey(bindings, origin.TargetSymbol) {
			out.targetKeys[origin.TargetSymbol] = key
		}
		targetPath, hasTargetPath := pathTargets[origin.Func]
		if !hasTargetPath && origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			targetPath = path.NewPath(origin.TargetSymbol, bindings.Name(origin.TargetSymbol))
			hasTargetPath = true
		}
		if hasTargetPath && (!origin.HasTargetSymbol || functionTargetCanUseStaticPathKey(bindings, origin.TargetSymbol)) {
			pathKey, ok := factflow.CalleePathKeyFromPath(targetPath)
			if !ok {
				return true
			}
			if existing, seen := out.pathKeys[pathKey]; seen && existing != key {
				ambiguousPathKeys[pathKey] = struct{}{}
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], existing)
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], key)
			} else {
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], key)
			}
			out.pathKeys[pathKey] = key
		}
		return true
	})
	// A path bound to more than one function definition is not a sound static
	// callee target: the call resolves through the current value identity instead.
	for pathKey := range ambiguousPathKeys {
		delete(out.pathKeys, pathKey)
	}
	return out
}

func functionTargetCanUseDirectSymbolKey(bindings *bind.Result, target symbol.ID) bool {
	if bindings == nil || target == 0 {
		return false
	}
	kind, ok := bindings.Kind(target)
	return ok && kind != symbol.Global
}

func functionTargetCanUseStaticPathKey(bindings *bind.Result, target symbol.ID) bool {
	if bindings == nil || target == 0 {
		return false
	}
	kind, ok := bindings.Kind(target)
	if !ok {
		return false
	}
	return kind != symbol.Global || len(bindings.WriteIdents(target)) <= 1
}

func appendSummaryKeyUnique(keys []summary.SummaryKey, key summary.SummaryKey) []summary.SummaryKey {
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func collectFunctionPathTargets(bindings *bind.Result, roots ...[]ast.Stmt) map[*ast.FunctionExpr]path.Path {
	if bindings == nil {
		return nil
	}
	out := make(map[*ast.FunctionExpr]path.Path)
	for _, stmts := range roots {
		collectFunctionPathTargetsInStmts(out, bindings, stmts)
	}
	bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.Func == nil {
			return true
		}
		collectFunctionPathTargetsInStmts(out, bindings, origin.Func.Stmts)
		return true
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectFunctionPathTargetsInStmts(out map[*ast.FunctionExpr]path.Path, bindings *bind.Result, stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.LocalAssignStmt:
			symbols := bindings.LocalSymbols(stmt)
			for i, expr := range stmt.Exprs {
				if i >= len(symbols) || symbols[i] == 0 {
					continue
				}
				root := path.NewPath(symbols[i], bindings.Name(symbols[i]))
				collectFunctionPathTargetsInExpr(out, root, expr)
			}
		case *ast.AssignStmt:
			for i, expr := range stmt.Rhs {
				if i >= len(stmt.Lhs) {
					continue
				}
				target, ok := pathexpr.Resolve(stmt.Lhs[i], bindings)
				if !ok || target.IsEmpty() {
					continue
				}
				collectFunctionPathTargetsInExpr(out, target, expr)
			}
		case *ast.FuncDefStmt:
			target, ok := pathexpr.ResolveFuncName(stmt.Name, bindings)
			if ok && !target.IsEmpty() && stmt.Func != nil {
				out[stmt.Func] = target
			}
		case *ast.DoBlockStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.IfStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Then)
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Else)
		case *ast.WhileStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.RepeatStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.NumberForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.GenericForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		}
	}
}

func collectFunctionPathTargetsInExpr(out map[*ast.FunctionExpr]path.Path, root path.Path, expr ast.Expr) {
	if root.IsEmpty() {
		return
	}
	expr = unwrapFunctionValueTarget(expr)
	switch expr := expr.(type) {
	case *ast.FunctionExpr:
		out[expr] = root
	case *ast.TableExpr:
		collectFunctionPathTargetsInTable(out, root, expr)
	}
}

func collectFunctionPathTargetsInTable(out map[*ast.FunctionExpr]path.Path, root path.Path, table *ast.TableExpr) {
	if table == nil {
		return
	}
	arrayIndex := 0
	for _, field := range table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok {
			continue
		}
		if !suffix.CanNameSummaryPath() {
			continue
		}
		target := appendPath(root, suffix.Path)
		collectFunctionPathTargetsInExpr(out, target, field.Value)
	}
}

func unwrapFunctionValueTarget(expr ast.Expr) ast.Expr {
	for {
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			expr = wrapped.Expr
		default:
			return expr
		}
	}
}

func appendPath(root path.Path, suffix path.Path) path.Path {
	return root.AppendSegments(suffix.Segments)
}

func rootKey(configured summary.SummaryKey) summary.SummaryKey {
	if !configured.Ref.IsZero() {
		return configured
	}
	return summary.DefaultSummaryKey(ref.Root())
}
